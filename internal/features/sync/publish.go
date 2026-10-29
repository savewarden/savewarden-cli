package sync

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

// publish moves staging\<op> to current after its intent, then commits the
// receipt and the published record (ADR 0006 §4). From publish-intent on,
// nothing is recorded as failed: an interrupted switch is left to recovery.
func (o *operation) publish() error {
	current := o.game + "/" + currentDir
	if err := o.record(backup.StepPublishIntent, backup.StepValues{Staging: o.staging}); err != nil {
		return err
	}
	o.critical = true
	moved, err := o.switchTo(o.stagingDir(), current, o.staging)
	var unexplained *unexplainedError
	switch {
	case err == nil:
	case errors.As(err, &unexplained):
		// An object no record explains: nothing is recorded or moved.
		return o.intervene(err.Error())
	case moved:
		return o.intervene("the switch did not end as recorded: " + err.Error())
	default:
		// Nothing moved: the intent is closed by a rollback record.
		if recordErr := o.record(backup.StepRolledBack, backup.StepValues{}); recordErr != nil {
			return o.intervene(fmt.Sprintf("publication failed (%v) and could not be recorded: %v", err, recordErr))
		}
		o.critical, o.effect = false, EffectRolledBack
		return fmt.Errorf("publication failed, current unchanged: %w", err)
	}
	published := o.now()
	receipt, err := o.writeReceipt(&published, nil, "")
	if err != nil {
		return o.intervene("current is published but its receipt is missing: " + err.Error() + "; run sync --recover")
	}
	if err := o.record(backup.StepPublished, backup.StepValues{Digest: receipt}); err != nil {
		return o.intervene("current and its receipt are published but the journal is not closed: " + err.Error() + "; run sync --recover")
	}
	o.critical, o.effect = false, EffectPublished
	return nil
}

// intervene leaves the operation to recovery: exit code 3.
func (o *operation) intervene(cause string) error {
	o.effect = EffectIntervention
	return &InterventionError{Operation: o.op, Cause: cause}
}

// switchTo renames from to to, in the order of ADR 0006 §4: every pin on
// from released, identity checked before, target absent, MoveFileEx with
// retries, identity checked after. moved reports whether the rename may
// have happened, in which case only recovery decides. A check before the
// rename that fails or cannot be read is an unexplainedError: nothing may
// then be recorded as if the state were known (ADR 0006 §3).
func (o *operation) switchTo(from, to string, want backup.FileID) (moved bool, err error) {
	if err := o.vault.Release(from); err != nil {
		return false, &unexplainedError{fmt.Sprintf("%s cannot be released before its rename: %v", from, err)}
	}
	id, err := o.vault.Identity(from)
	switch {
	case err != nil:
		return false, &unexplainedError{fmt.Sprintf("%s cannot be checked before its rename: %v", from, err)}
	case id != want:
		return false, &unexplainedError{fmt.Sprintf("%s is %s, not the directory %s the journal recorded", from, id, want)}
	}
	switch _, err := o.vault.Identity(to); {
	case err == nil:
		return false, &unexplainedError{fmt.Sprintf("%s exists, and no record explains it", to)}
	case !errors.Is(err, fs.ErrNotExist):
		return false, &unexplainedError{fmt.Sprintf("%s cannot be checked before the rename: %v", to, err)}
	}
	if err := o.move(from, to); err != nil {
		// MoveFileEx renames a name or fails; the source and the target
		// say which, and both must confirm that nothing moved.
		if id, idErr := o.vault.Identity(from); idErr != nil || id != want {
			return true, err
		}
		if _, toErr := o.vault.Identity(to); !errors.Is(toErr, fs.ErrNotExist) {
			return false, &unexplainedError{fmt.Sprintf("the rename of %s failed (%v), and %s is no longer absent", from, err, to)}
		}
		return false, err
	}
	switch id, err := o.vault.Identity(to); {
	case err != nil:
		return true, fmt.Errorf("%s cannot be checked after the rename: %w", to, err)
	case id != want:
		return true, fmt.Errorf("%s is %s after the rename, not %s; an unknown object is neither read nor moved", to, id, want)
	}
	switch _, err := o.vault.Identity(from); {
	case err == nil:
		return true, fmt.Errorf("%s still exists after the rename", from)
	case !errors.Is(err, fs.ErrNotExist):
		return true, fmt.Errorf("%s cannot be checked after the rename: %w", from, err)
	}
	return true, nil
}

// unexplainedError reports, before a rename, an object that no record
// explains or a state that cannot be observed: an ambiguity of ADR 0006 §3,
// code 3 with nothing recorded.
type unexplainedError struct{ detail string }

func (e *unexplainedError) Error() string { return e.detail }

// writeReceipt commits receipts\<op>.json from the plan and returns its
// SHA-256. A receipt written at publication has publishedAt; one rebuilt by
// recovery has recoveredAt and recoveredBy instead (ADR 0007).
func (o *operation) writeReceipt(publishedAt, recoveredAt *time.Time, recoveredBy string) ([32]byte, error) {
	r, err := o.planned.Receipt(o.op, o.planSum, o.intentAt, publishedAt, recoveredAt, recoveredBy)
	if err != nil {
		return [32]byte{}, err
	}
	data, err := backup.EncodeReceipt(r)
	if err != nil {
		return [32]byte{}, err
	}
	if err := o.ensureDir(o.game + "/" + receiptsDir); err != nil {
		return [32]byte{}, err
	}
	if err := o.commitFile(o.game+"/"+receiptsDir+"/"+o.op+".json", data, o.receipt, nil); err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(data), nil
}
