package sync

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/entities/game"
)

// RecoveryAction says what sync --recover --apply does, per the recovery
// tables of ADR 0006 §4.
type RecoveryAction string

// Recovery actions of a first publication.
const (
	RecoverNothing        RecoveryAction = "nothing to recover"
	RecoverInitialization RecoveryAction = "commit the vault marker; its temporaries stay in place"
	RecoverAbandon        RecoveryAction = "record abandoned; the staging, if any, stays in place"
	RecoverRolledBack     RecoveryAction = "record rolled-back; nothing to move"
	RecoverVerifyPublish  RecoveryAction = "verify current against the plan, then commit its receipt and published; if it differs, move it back to staging"
	RecoverUnpublish      RecoveryAction = "move current back to staging, then record rolled-back"
)

// Recovery is what sync --recover found, without writing anything.
type Recovery struct {
	ManifestPath string
	Game         game.Game
	Vault        string
	VaultState   VaultState
	Volume       backup.Volume
	// Operation is the interrupted operation, empty when there is none.
	Operation string
	Last      backup.Step
	// Observed holds the presences of current and of the staging, by
	// identity; nil when no operation is pending.
	Observed *backup.Observation
	Action   RecoveryAction

	vaultVolume   string
	vaultSegments []string
	observed      *vault
	journal       backup.Journal
}

// RecoverPlan finds the interrupted operation of the game, observes the
// identities under current and staging\<op>, and decides its recovery
// without writing anything. An ambiguity is an InterventionError: nothing
// is guessed, and --apply would do nothing either.
func (s *Service) RecoverPlan(ctx context.Context, request Request) (Recovery, error) {
	return s.recoverPlan(ctx, request, true)
}

// recoverPlan is RecoverPlan; a run that holds the lock does not probe it.
func (s *Service) recoverPlan(ctx context.Context, request Request, probe bool) (Recovery, error) {
	manifest, err := s.manifests.Read(request.ManifestPath)
	if err != nil {
		return Recovery{}, err
	}
	vaultVolume, vaultSegments, err := game.ParseWindowsAbsolute(request.VaultPath)
	if err != nil {
		return Recovery{}, fmt.Errorf("vault %s: %w", request.VaultPath, err)
	}
	rec := Recovery{ManifestPath: request.ManifestPath, Game: manifest.Game, Vault: request.VaultPath, Action: RecoverNothing,
		vaultVolume: vaultVolume, vaultSegments: vaultSegments}
	scope, err := s.files.OpenScope(vaultVolume)
	if errors.Is(err, fs.ErrNotExist) {
		return rec, errVault(request.VaultPath, "volume %s is not available", vaultVolume)
	}
	if err != nil {
		return rec, fmt.Errorf("vault %s: open volume %s: %w", request.VaultPath, vaultVolume, err)
	}
	defer func() { _ = scope.Close() }()
	v, err := resolveVault(scope, request.VaultPath, vaultSegments)
	if err != nil {
		return rec, err
	}
	if rec.Volume, err = s.describe(request.VaultPath, vaultVolume, vaultSegments, v.exists); err != nil {
		return rec, err
	}
	if err := v.readProbing(scope, request.VaultPath, vaultSegments, manifest.Game.ID, probe); err != nil {
		return rec, err
	}
	if err := ctx.Err(); err != nil {
		return rec, fmt.Errorf("recovery plan interrupted: %w", err)
	}
	rec.VaultState, rec.observed = v.state, v
	switch v.state {
	case VaultAbsent, VaultEmpty:
		return rec, nil
	case VaultInterrupted:
		if len(v.rootTaken) >= backup.MaxTemporaries {
			return rec, &InterventionError{Cause: backup.ErrEnvelopeExhausted.Error() + ": nine marker temporaries"}
		}
		rec.Action = RecoverInitialization
		return rec, nil
	}
	for _, intervention := range v.interventions {
		if intervention.Cause != CausePendingOperation {
			return rec, &InterventionError{Cause: string(intervention.Cause) + " " + intervention.Subject + ": " + intervention.Detail}
		}
	}
	if len(v.pending) == 0 {
		return rec, nil
	}
	// A receipt that cannot be read is never rewritten, completed or
	// replaced: recovery stops before writing anything (ADR 0006 A5).
	if len(v.invalid) > 0 {
		return rec, &InterventionError{Operation: v.pending[0].Operation, Cause: "receipt " + v.invalid[0] + " is not valid; it is never rewritten"}
	}
	rec.Operation = v.pending[0].Operation
	rec.journal = v.journals[rec.Operation]
	rec.Last = rec.journal.Last()
	if rec.journal.Rotation() {
		return rec, &InterventionError{Operation: rec.Operation, Cause: "the interrupted operation replaces a current generation; its recovery is not available in this build"}
	}
	game := join(strings.Join(vaultSegments, "/"), gamesDir+"/"+manifest.Game.ID)
	observed := backup.Observation{
		Current: presence(scope, game+"/"+currentDir, rec.journal.Staging),
		Staging: presence(scope, game+"/"+backup.StagingName(rec.Operation), rec.journal.Staging),
	}
	rec.Observed = &observed
	action, err := backup.Classify(rec.journal, observed)
	if err != nil {
		return rec, &InterventionError{Operation: rec.Operation, Cause: err.Error()}
	}
	switch action {
	case backup.ActionAbandon:
		rec.Action = RecoverAbandon
	case backup.ActionMarkRolledBack:
		rec.Action = RecoverRolledBack
	case backup.ActionVerifyThenPublish:
		_, published := v.receiptSum[rec.Operation]
		if !published && len(v.receiptTaken[rec.Operation]) >= backup.MaxTemporaries {
			return rec, &InterventionError{Operation: rec.Operation, Cause: backup.ErrEnvelopeExhausted.Error() + ": nine receipt temporaries"}
		}
		rec.Action = RecoverVerifyPublish
	case backup.ActionFinishUnpublish:
		rec.Action = RecoverUnpublish
	default:
		return rec, &InterventionError{Operation: rec.Operation, Cause: fmt.Sprintf("recovery %q is not available in this build", action)}
	}
	return rec, nil
}

// presence describes what a name holds, by identity, without following it.
func presence(scope Scope, name string, staged backup.FileID) backup.Presence {
	id, err := scope.Identity(name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return backup.Absent
	case err != nil:
		return backup.Other
	case staged != (backup.FileID{}) && id == staged:
		return backup.New
	case staged == (backup.FileID{}):
		if info, err := scope.Lstat(name); err == nil && info.IsDir() && !isLink(info.Mode()) {
			return backup.Unidentified
		}
	}
	return backup.Other
}

// Recover plans the recovery, then performs it under lock after reading
// the vault again: the same operation, the same observation and the same
// action, or nothing is done (ADR 0006 §4 and §9).
func (s *Service) Recover(ctx context.Context, request Request, w Writer) (Result, error) {
	rec, err := s.RecoverPlan(ctx, request)
	result := Result{Operation: rec.Operation, Recovery: &rec}
	var intervention *InterventionError
	if errors.As(err, &intervention) {
		result.Effect, result.Cause = EffectIntervention, intervention.Cause
		return result, err
	}
	if err != nil || rec.Action == RecoverNothing {
		return result, err
	}
	session, err := w.Vaults.Open(request.VaultPath)
	if err != nil {
		return result, errVault(request.VaultPath, "%w", err)
	}
	o := &operation{
		s: s, w: w, ctx: ctx, vault: session, op: rec.Operation, game: gamesDir + "/" + rec.Game.ID, recovering: true,
		plan:     Plan{Vault: rec.Vault, Game: rec.Game, vaultVolume: rec.vaultVolume, vaultSegments: rec.vaultSegments},
		attempts: map[int]map[int]bool{}, receipt: map[int]bool{}, marker: map[int]bool{},
	}
	err = o.unfinished(o.recover(&rec, request))
	if closeErr := session.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	result.Effect = o.effect
	if errors.As(err, &intervention) {
		result.Effect, result.Cause = EffectIntervention, intervention.Cause
	}
	return result, err
}

func (o *operation) recover(rec *Recovery, request Request) error {
	if rec.Action == RecoverInitialization {
		// The vault root is read again once the session pins it: still only
		// marker temporaries, the same ones (ADR 0006 I3).
		again, err := o.s.recoverPlan(o.ctx, request, false)
		if err != nil {
			return err
		}
		if again.Action != RecoverInitialization || len(again.observed.rootTaken) != len(rec.observed.rootTaken) {
			return errVault(rec.Vault, "vault-changed: the vault changed since the recovery plan; run sync --recover again")
		}
		// The marker alone; games and the rest come with the next --apply.
		for k := range again.observed.rootTaken {
			o.marker[k] = true
		}
		marker, err := backup.EncodeMarker(backup.Marker{CreatedAt: o.now(), ToolVersion: o.w.ToolVersion})
		if err != nil {
			return err
		}
		if err := o.commitFile(backup.MarkerName, marker, o.marker, o.wrote); err != nil {
			return errVault(rec.Vault, "marker: %w", err)
		}
		return nil
	}
	for _, dir := range []string{gamesDir, o.game, o.game + "/" + journalDir, o.journalDir()} {
		if err := o.vault.Hold(dir); err != nil {
			return errVault(rec.Vault, "%w", err)
		}
	}
	if err := o.vault.Lock(o.game + "/" + lockFile); err != nil {
		var busy interface{ Busy() bool }
		if errors.As(err, &busy) && busy.Busy() {
			return errVault(rec.Vault, "another sync is running on this game")
		}
		return errVault(rec.Vault, "lock: %w", err)
	}
	// Under lock, the decision must be the one just shown.
	again, err := o.s.recoverPlan(o.ctx, request, false)
	if err != nil {
		return err
	}
	if again.Operation != rec.Operation || again.Last != rec.Last || again.Action != rec.Action ||
		(again.Observed == nil) != (rec.Observed == nil) || (again.Observed != nil && *again.Observed != *rec.Observed) {
		return errVault(rec.Vault, "vault-changed: the vault changed since the recovery plan; run sync --recover again")
	}
	*rec = again
	j := rec.journal
	o.seq, o.prev = len(j.Records), rec.observed.lastRecord[o.op]
	o.attempts[o.seq+1] = map[int]bool{}
	for k := range j.Taken {
		o.attempts[o.seq+1][k] = true
	}
	for k := range rec.observed.receiptTaken[o.op] {
		o.receipt[k] = true
	}
	o.planned, o.planSum, o.staging = j.Plan, j.PlanSHA256, j.Staging
	for _, r := range j.Records {
		if r.Step == backup.StepPublishIntent {
			o.intentAt = r.At
		}
	}
	if rec.Action == RecoverVerifyPublish || rec.Action == RecoverUnpublish {
		// current may move back below it; it exists since staging-intent.
		if err := o.vault.Hold(o.game + "/" + stagingDir); err != nil {
			return errVault(rec.Vault, "%w", err)
		}
	}
	o.critical = true
	switch rec.Action {
	case RecoverAbandon:
		if err := o.record(backup.StepAbandoned, backup.StepValues{Text: "recovered: interrupted before publication"}); err != nil {
			return err
		}
		o.effect = EffectStaged
	case RecoverRolledBack:
		if err := o.record(backup.StepRolledBack, backup.StepValues{}); err != nil {
			return err
		}
		o.effect = EffectRolledBack
	case RecoverVerifyPublish:
		return o.verifyThenPublish(rec)
	case RecoverUnpublish:
		return o.unpublish(rec.Last == backup.StepRollbackFailed)
	}
	return nil
}

// verifyThenPublish handles a staging already renamed to current: current
// is verified in full against the plan; equal, its receipt and published
// are committed, the receipt rebuilt if missing; different, it is moved
// back to staging.
func (o *operation) verifyThenPublish(rec *Recovery) error {
	segments := append(append([]string{}, o.plan.vaultSegments...), gamesDir, o.plan.Game.ID, currentDir)
	dir, err := o.s.inventories.Directory(o.ctx, o.plan.vaultVolume, segments)
	if err != nil {
		return err
	}
	_, receiptCommitted := rec.observed.receiptSum[o.op]
	if differences := compareWithReceipt(dir, backup.Receipt{Sources: o.planned.Sources}); len(differences) > 0 {
		if receiptCommitted {
			// A5 and A6: a receipt already describes current, which no
			// longer matches; moving it back would leave the receipt orphan.
			return o.intervene(fmt.Sprintf("current no longer matches the plan of %s, whose receipt is committed: %s %s, %s",
				o.op, orDash(differences[0].source), orDash(differences[0].path), differences[0].detail))
		}
		if err := o.record(backup.StepUnpublishIntent, backup.StepValues{Current: o.staging}); err != nil {
			return err
		}
		return o.unpublish(false)
	}
	o.effect = EffectIntervention
	digest, ok := rec.observed.receiptSum[o.op]
	if ok {
		receipt := findReceipt(rec.observed, o.op)
		if receipt == nil || !o.matchesPlan(*receipt) {
			return o.intervene("the receipt of " + o.op + " does not match its plan")
		}
	} else {
		now := o.now()
		if digest, err = o.writeReceipt(nil, &now, o.w.ToolVersion); err != nil {
			return o.intervene("current is published but its receipt cannot be written: " + err.Error())
		}
	}
	if err := o.record(backup.StepPublished, backup.StepValues{Digest: digest}); err != nil {
		return o.intervene("current and its receipt are published but the journal is not closed: " + err.Error())
	}
	o.critical, o.effect = false, EffectPublished
	return nil
}

// unpublish moves current back to staging\<op> after unpublish-intent,
// then records rolled-back. A failure is recorded once as rollback-failed
// (envelope E3) and stops in code 3.
func (o *operation) unpublish(retried bool) error {
	moved, err := o.switchTo(o.game+"/"+currentDir, o.stagingDir(), o.staging)
	var unexplained *unexplainedError
	switch {
	case err == nil:
	case errors.As(err, &unexplained):
		// An object no record explains: nothing is recorded or moved.
		return o.intervene(err.Error())
	case moved:
		return o.intervene("the rollback did not end as recorded: " + err.Error())
	default:
		if !retried {
			if recordErr := o.record(backup.StepRollbackFailed, backup.StepValues{Text: oneLine(err.Error())}); recordErr != nil {
				return o.intervene(fmt.Sprintf("rollback failed (%v) and could not be recorded: %v", err, recordErr))
			}
		}
		return o.intervene("rollback failed: " + err.Error() + "; close the programs that use the vault, then run sync --recover --apply again")
	}
	if err := o.record(backup.StepRolledBack, backup.StepValues{}); err != nil {
		return o.intervene("current was moved back to staging but the journal is not closed: " + err.Error())
	}
	o.critical, o.effect = false, EffectRolledBack
	return nil
}

func findReceipt(v *vault, op string) *backup.Receipt {
	for i := range v.receipts {
		if v.receipts[i].Operation == op {
			return &v.receipts[i]
		}
	}
	return nil
}

// matchesPlan reports whether a committed receipt is the one this
// operation writes: everything but the moment of publication or recovery.
func (o *operation) matchesPlan(r backup.Receipt) bool {
	p := o.planned
	return r.Operation == o.op && r.ToolVersion == p.ToolVersion && r.GameID == p.GameID && r.GameName == p.GameName &&
		r.StartedAt.Equal(p.StartedAt) && r.PublishIntentAt.Equal(o.intentAt) && r.Previous == p.Previous &&
		r.PlanSHA256 == o.planSum && r.Changes == p.Changes && sameSources(r.Sources, p.Sources)
}

func sameSources(a, b []backup.ReceiptSource) bool {
	da, errA := backup.ContentDigest(a)
	db, errB := backup.ContentDigest(b)
	return errA == nil && errB == nil && da == db
}

func oneLine(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " ")
	if len(s) > 1000 {
		s = s[:1000]
	}
	return s
}
