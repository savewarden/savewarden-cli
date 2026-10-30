package sync

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

// rotate replaces the recognised current by the verified staging (ADR 0006
// §5): current read again in full, retire-intent, M1 current to
// retained\<prev-op>, the exact observation checked again with retained
// read in full, promote-intent, M2 staging to current, then the receipt and
// published. A refused M2 restores the old generation by M3 only after a
// new observation finds it intact; any anomaly after M1 stops in code 3,
// without a conjectural rollback.
func (o *operation) rotate() error {
	// A2, point 3: just before retire-intent, current is read again in full,
	// by the identity planned fixed.
	switch id, err := o.vault.Identity(o.game + "/" + currentDir); {
	case err != nil:
		return o.intervene("current cannot be checked before its retirement: " + err.Error() + "; nothing recorded or moved")
	case id != o.old:
		return o.intervene(fmt.Sprintf("current is %s, not the directory %s recorded by planned; nothing recorded or moved", id, o.old))
	}
	if err := o.oldUnchanged(currentDir); err != nil {
		return err
	}
	current, retained := o.game+"/"+currentDir, o.retainedDir()
	if err := o.record(backup.StepRetireIntent, backup.StepValues{Current: o.old}); err != nil {
		return err
	}
	o.critical = true
	// M1. The retained directory is created under the intent, like staging.
	moved, err := false, o.ensureDir(o.game+"/"+retainedDir)
	if err != nil {
		// Only a creation that failed leaves nothing there; anything else,
		// a link, a file, an entry that cannot be read, is unexplained.
		if _, idErr := o.vault.Identity(o.game + "/" + retainedDir); !errors.Is(idErr, fs.ErrNotExist) {
			err = &unexplainedError{fmt.Sprintf("%s cannot be held before the retirement: %v", retainedDir, err)}
		}
	}
	if err == nil {
		moved, err = o.switchTo(current, retained, o.old)
	}
	var unexplained *unexplainedError
	switch {
	case err == nil:
	case errors.As(err, &unexplained):
		return o.intervene(err.Error())
	case moved:
		return o.intervene("the retirement did not end as recorded: " + err.Error())
	default:
		// Nothing moved: current is still the old generation.
		if recordErr := o.record(backup.StepRolledBack, backup.StepValues{}); recordErr != nil {
			return o.intervene(fmt.Sprintf("retirement failed (%v) and could not be recorded: %v", err, recordErr))
		}
		o.critical, o.effect = false, EffectRolledBack
		return fmt.Errorf("retirement failed, current unchanged: %w", err)
	}
	// Point 5: the exact observation (—, old, new) before any promotion.
	if err := o.checkRetired(); err != nil {
		return o.intervene(err.Error() + "; nothing promoted or restored")
	}
	if err := o.record(backup.StepPromoteIntent, backup.StepValues{Staging: o.staging}); err != nil {
		return err
	}
	// M2.
	moved, err = o.switchTo(o.stagingDir(), current, o.staging)
	switch {
	case err == nil:
	case errors.As(err, &unexplained):
		// A current that reappeared, or another staging: neither is moved,
		// and the old generation is not put back over them.
		return o.intervene(err.Error() + "; nothing restored")
	case moved:
		return o.intervene("the promotion did not end as recorded: " + err.Error())
	default:
		return o.restore(err)
	}
	published := o.now()
	receipt, err := o.writeReceipt(&published, nil, "")
	if err != nil {
		return o.intervene("current is promoted but its receipt is missing: " + err.Error() + "; run sync --recover")
	}
	if err := o.record(backup.StepPublished, backup.StepValues{Digest: receipt}); err != nil {
		return o.intervene("current and its receipt are published but the journal is not closed: " + err.Error() + "; run sync --recover")
	}
	o.critical, o.effect = false, EffectPublished
	return nil
}

// restore puts the old generation back after a refused promotion: a new
// observation first, then restore-intent and M3, verified restoration of
// ADR 0006 §5. A refused M3 is recorded once as rollback-failed (E3).
func (o *operation) restore(cause error) error {
	if err := o.restoreOld(fmt.Sprintf("promotion failed (%v), then ", cause), true, false); err != nil {
		return err
	}
	return fmt.Errorf("promotion failed, current restored: %w", cause)
}

// restoreOld is the verified restoration of ADR 0006 §5, during a run as
// during recovery: the observation (—, old, new) with retained read in
// full, restore-intent unless already committed, M3, then rolled-back. An
// altered retained is never put back: code 3, nothing moved or recorded.
// A refused M3 is recorded as rollback-failed unless one already follows
// the intent (E3). why prefixes the diagnostics.
func (o *operation) restoreOld(why string, intent, retried bool) error {
	if err := o.checkRetired(); err != nil {
		return o.intervene(fmt.Sprintf("%s%v; nothing restored", why, err))
	}
	if intent {
		if err := o.record(backup.StepRestoreIntent, backup.StepValues{Retained: o.old}); err != nil {
			return o.intervene(fmt.Sprintf("%sthe restoration could not be recorded: %v", why, err))
		}
	}
	moved, err := o.switchTo(o.retainedDir(), o.game+"/"+currentDir, o.old)
	var unexplained *unexplainedError
	switch {
	case err == nil:
	case errors.As(err, &unexplained):
		return o.intervene(err.Error())
	case moved:
		return o.intervene("the restoration did not end as recorded: " + err.Error())
	default:
		if !retried {
			if recordErr := o.record(backup.StepRollbackFailed, backup.StepValues{Text: oneLine(err.Error())}); recordErr != nil {
				return o.intervene(fmt.Sprintf("%sthe restoration failed (%v) and could not be recorded: %v", why, err, recordErr))
			}
		}
		return o.intervene(fmt.Sprintf("%sthe restoration failed (%v); the old generation is intact in %s, the new one in %s; "+
			"close the programs that use the vault, then run sync --recover --apply", why, err, o.retainedDir(), o.stagingDir()))
	}
	if err := o.record(backup.StepRolledBack, backup.StepValues{}); err != nil {
		return o.intervene("current was restored but the journal is not closed: " + err.Error())
	}
	o.critical, o.effect = false, EffectRolledBack
	return nil
}

// unpromote moves the new current back to staging\<op> after
// unpromote-intent (M4), then restores the old generation. A refused M4 is
// recorded as rollback-failed unless one already follows the intent (E3).
func (o *operation) unpromote(retried bool) error {
	moved, err := o.switchTo(o.game+"/"+currentDir, o.stagingDir(), o.staging)
	var unexplained *unexplainedError
	switch {
	case err == nil:
	case errors.As(err, &unexplained):
		return o.intervene(err.Error())
	case moved:
		return o.intervene("the move back to staging did not end as recorded: " + err.Error())
	default:
		if !retried {
			if recordErr := o.record(backup.StepRollbackFailed, backup.StepValues{Text: oneLine(err.Error())}); recordErr != nil {
				return o.intervene(fmt.Sprintf("the move back to staging failed (%v) and could not be recorded: %v", err, recordErr))
			}
		}
		return o.intervene("the move back to staging failed: " + err.Error() + "; close the programs that use the vault, then run sync --recover --apply again")
	}
	return o.restoreOld("", true, false)
}

// checkRetired requires the observation (—, old, new) after M1: current
// absent, the staging by its identity, retained\<prev-op> by the identity
// planned fixed and read in full against its receipt (ADR 0006 §5, point
// 5). It moves nothing.
func (o *operation) checkRetired() error {
	switch _, err := o.vault.Identity(o.game + "/" + currentDir); {
	case err == nil:
		return errors.New("an object appeared under current after the retirement; its provenance is not proven")
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("current cannot be checked after the retirement: %w", err)
	}
	switch id, err := o.vault.Identity(o.stagingDir()); {
	case err != nil:
		return fmt.Errorf("%s cannot be checked: %w", o.stagingDir(), err)
	case id != o.staging:
		return fmt.Errorf("%s is %s, not the directory %s recorded by staged", o.stagingDir(), id, o.staging)
	}
	switch id, err := o.vault.Identity(o.retainedDir()); {
	case err != nil:
		return fmt.Errorf("%s cannot be checked: %w", o.retainedDir(), err)
	case id != o.old:
		return fmt.Errorf("%s is %s, not the directory %s recorded by planned", o.retainedDir(), id, o.old)
	}
	differences, err := o.oldDifferences(retainedDir + "/" + o.previous)
	if err != nil {
		return err
	}
	switch id, err := o.vault.Identity(o.retainedDir()); {
	case err != nil:
		return fmt.Errorf("%s cannot be checked after it was read: %w", o.retainedDir(), err)
	case id != o.old:
		return fmt.Errorf("%s was replaced while it was read: it is %s, not the directory %s recorded by planned", o.retainedDir(), id, o.old)
	}
	if len(differences) > 0 {
		first := differences[0]
		return fmt.Errorf("%s no longer matches the receipt of %s: %s %s, %s; %d difference(s) in all", o.retainedDir(), o.previous,
			orDash(first.source), orDash(first.path), strings.ReplaceAll(first.detail, "current", "retained"), len(differences))
	}
	return nil
}

// oldUnchanged requires current to match the receipt of the generation it
// holds, in full: a difference is current-modified, code 2, and nothing is
// moved. Its identity is read again after the content: a directory swapped
// during the read is code 3, never taken for a content change.
func (o *operation) oldUnchanged(name string) error {
	differences, err := o.oldDifferences(name)
	if err != nil {
		return err
	}
	if id, err := o.vault.Identity(o.game + "/" + name); err != nil || id != o.old {
		return o.refuse(fmt.Sprintf("%s was replaced while it was read, or cannot be checked: %v %v; nothing recorded or moved", name, id, err))
	}
	if len(differences) > 0 {
		first := differences[0]
		return &BlockedError{Code: CodeCurrentModified, Detail: fmt.Sprintf("current changed since the plan: %s %s, %s; %d difference(s) in all",
			orDash(first.source), orDash(first.path), first.detail, len(differences))}
	}
	return nil
}

// oldDifferences reads the old generation under name, relative to the game
// directory, through a read-only no-follow walk closed before returning,
// and lists how it differs from the receipt of previous.
func (o *operation) oldDifferences(name string) ([]difference, error) {
	segments := append(append([]string{}, o.plan.vaultSegments...), gamesDir, o.plan.Game.ID)
	segments = append(segments, strings.Split(name, "/")...)
	ctx := o.ctx
	if o.critical {
		// Between the moves, a check is part of the switch: it is finished.
		ctx = context.WithoutCancel(ctx)
	}
	dir, err := o.s.inventories.Directory(ctx, o.plan.vaultVolume, segments)
	if err != nil {
		return nil, err
	}
	return compareWithReceipt(dir, o.head), nil
}

func (o *operation) retainedDir() string { return o.game + "/" + retainedDir + "/" + o.previous }

// refuse stops in code 3 with nothing recorded: for the operation once its
// directory exists, for the vault before.
func (o *operation) refuse(cause string) error {
	if o.open {
		return o.intervene(cause)
	}
	o.effect = EffectIntervention
	return &InterventionError{Cause: cause}
}
