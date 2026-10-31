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
// tables of ADR 0006 §4 and §5.
type RecoveryAction string

// Recovery actions of a first publication and of a rotation.
const (
	RecoverNothing        RecoveryAction = "nothing to recover"
	RecoverInitialization RecoveryAction = "commit the vault marker; its temporaries stay in place"
	RecoverAbandon        RecoveryAction = "record abandoned; the staging, if any, stays in place"
	RecoverRolledBack     RecoveryAction = "record rolled-back; nothing to move"
	RecoverVerifyPublish  RecoveryAction = "verify current against the plan, then commit its receipt and published; if it differs, move it back to staging"
	RecoverUnpublish      RecoveryAction = "move current back to staging, then record rolled-back"
	RecoverVerifyPromote  RecoveryAction = "verify current against the plan, then commit its receipt and published; if it differs, move it back to staging and restore the retained generation"
	RecoverRestore        RecoveryAction = "verify the retained generation, record restore-intent, move it back to current, then record rolled-back"
	RecoverFinishRestore  RecoveryAction = "verify the retained generation, move it back to current, then record rolled-back"
	RecoverUnpromote      RecoveryAction = "move current back to staging, verify the retained generation and move it back to current, then record rolled-back"
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
	// Previous is the generation an interrupted rotation replaces, empty
	// for a first publication.
	Previous string
	// Observed holds the presences of current, of the retained generation of
	// a rotation and of the staging, by identity; nil when no operation is
	// pending.
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
	j := rec.journal
	rec.Previous = j.Previous
	game := join(strings.Join(vaultSegments, "/"), gamesDir+"/"+manifest.Game.ID)
	var head *backup.Receipt
	if j.Rotation() {
		if head = findReceipt(v, j.Previous); head == nil {
			return rec, &InterventionError{Operation: rec.Operation, Cause: "the receipt of " + j.Previous + ", the generation this rotation replaces, is missing"}
		}
		// The receipt a restoration checks retained against must be the one
		// the journal of that generation published, for the identity
		// planned fixed (ADR 0006 §4).
		previous, ok := v.journals[j.Previous]
		if !ok {
			return rec, &InterventionError{Operation: rec.Operation, Cause: "the journal of " + j.Previous + ", the generation this rotation replaces, is missing"}
		}
		if err := backup.RecognizePrevious(*head, v.receiptSum[j.Previous], previous, j.Old); err != nil {
			return rec, &InterventionError{Operation: rec.Operation, Cause: "the generation this rotation replaces is not proven: " + err.Error()}
		}
	}
	observed := backup.Observation{
		Current: presence(scope, game+"/"+currentDir, j),
		Staging: presence(scope, game+"/"+backup.StagingName(rec.Operation), j),
	}
	if j.Rotation() {
		observed.Retained = presence(scope, game+"/"+retainedDir+"/"+j.Previous, j)
	}
	if len(j.Records) == 0 {
		// Rule M3: nothing was fixed, so current must prove it is the
		// published generation, or be absent when none is expected.
		observed.Expected = v.generationExpected()
		if observed.Current != backup.Absent {
			if observed.Current, err = s.previousGeneration(ctx, scope, v, rec, game); err != nil {
				return rec, err
			}
		}
	}
	rec.Observed = &observed
	action, err := backup.Classify(rec.journal, observed)
	if err != nil {
		return rec, &InterventionError{Operation: rec.Operation, Cause: err.Error()}
	}
	if _, committed := v.receiptSum[rec.Operation]; committed && action != backup.ActionVerifyThenPublish {
		// A receipt is committed only after its switch: without the switch
		// observed, journal and vault contradict each other (ADR 0006 §3).
		return rec, &InterventionError{Operation: rec.Operation, Cause: "the receipt of " + rec.Operation + " is committed but its generation is not current; nothing written"}
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
		if j.Rotation() {
			rec.Action = RecoverVerifyPromote
		}
	case backup.ActionFinishUnpublish:
		rec.Action = RecoverUnpublish
	case backup.ActionRestore:
		rec.Action = RecoverRestore
	case backup.ActionFinishRestore:
		rec.Action = RecoverFinishRestore
	case backup.ActionFinishUnpromote:
		rec.Action = RecoverUnpromote
	default:
		return rec, &InterventionError{Operation: rec.Operation, Cause: fmt.Sprintf("recovery %q is not available in this build", action)}
	}
	if rec.Action == RecoverRestore || rec.Action == RecoverFinishRestore {
		// Verified restoration: an altered generation is never put back.
		segments := append(append([]string{}, vaultSegments...), gamesDir, manifest.Game.ID, retainedDir, j.Previous)
		dir, err := s.inventories.Directory(ctx, vaultVolume, segments)
		if err != nil {
			return rec, err
		}
		if differences := compareWithReceipt(dir, *head); len(differences) > 0 {
			first := differences[0]
			return rec, &InterventionError{Operation: rec.Operation, Cause: fmt.Sprintf(
				"%s/%s/%s no longer matches the receipt of %s: %s %s, %s; %d difference(s) in all; nothing restored",
				gamesDir+"/"+manifest.Game.ID, retainedDir, j.Previous, j.Previous, orDash(first.source), orDash(first.path),
				strings.ReplaceAll(first.detail, "current", "retained"), len(differences))}
		}
	}
	return rec, nil
}

// generationExpected reports whether the game has a receipt, valid or not,
// or a journal terminal in published: a published generation should then
// exist (ADR 0006 §4, rule M3).
func (v *vault) generationExpected() bool {
	if len(v.receipts) > 0 || len(v.invalid) > 0 {
		return true
	}
	for _, j := range v.journals {
		if j.Last() == backup.StepPublished {
			return true
		}
	}
	return false
}

// previousGeneration applies the whole proof chain of ADR 0006 §4 to the
// current found beside an operation without any record: Old when it is the
// published generation, its content included, Other otherwise.
func (s *Service) previousGeneration(ctx context.Context, scope Scope, v *vault, rec Recovery, game string) (backup.Presence, error) {
	head, err := v.head()
	if err != nil {
		return backup.Other, nil
	}
	j, ok := v.journals[head.Operation]
	if !ok {
		return backup.Other, nil
	}
	id, err := scope.Identity(game + "/" + currentDir)
	if err != nil || backup.RecognizePrevious(head, v.receiptSum[head.Operation], j, id) != nil {
		return backup.Other, nil
	}
	segments := append(append([]string{}, rec.vaultSegments...), gamesDir, rec.Game.ID, currentDir)
	dir, err := s.inventories.Directory(ctx, rec.vaultVolume, segments)
	if err != nil {
		return backup.Other, err
	}
	if len(compareWithReceipt(dir, head)) > 0 {
		return backup.Other, nil
	}
	if again, err := scope.Identity(game + "/" + currentDir); err != nil || again != id {
		return backup.Other, nil
	}
	return backup.Old, nil
}

// presence describes what a name holds, by identity, without following
// it: Old is the current the journal's planned record fixed, New its
// staging; before staged, a plain directory is Unidentified, never Old.
func presence(scope Scope, name string, j backup.Journal) backup.Presence {
	id, err := scope.Identity(name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return backup.Absent
	case err != nil:
		return backup.Other
	case j.Old != (backup.FileID{}) && id == j.Old:
		return backup.Old
	case j.Staging != (backup.FileID{}) && id == j.Staging:
		return backup.New
	case j.Staging == (backup.FileID{}):
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
		s: s, w: w, ctx: ctx, vault: session, domain: &currentDomain, op: rec.Operation, game: gamesDir + "/" + rec.Game.ID, recovering: true,
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
	for _, dir := range []string{gamesDir, o.game, o.game + "/" + o.domain.journals, o.journalDir()} {
		if err := o.vault.Hold(dir); err != nil {
			return errVault(rec.Vault, "%w", err)
		}
	}
	if err := o.vault.Lock(o.game + "/" + lockFile); err != nil {
		var busy interface{ Busy() bool }
		if errors.As(err, &busy) && busy.Busy() {
			return errVault(rec.Vault, "%s", o.domain.busy)
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
	if j.Plan != nil {
		o.content = j.Plan.Sources
	}
	if j.Rotation() {
		o.previous, o.old, o.head = j.Previous, j.Old, *findReceipt(rec.observed, j.Previous)
	}
	for _, r := range j.Records {
		if r.Step == backup.StepPublishIntent || r.Step == backup.StepPromoteIntent {
			o.intentAt = r.At
		}
	}
	switch rec.Action {
	case RecoverVerifyPublish, RecoverUnpublish, RecoverVerifyPromote, RecoverUnpromote, RecoverRestore, RecoverFinishRestore:
		// current may move back below it, and a restoration checks the
		// staging by its identity; it exists since staging-intent.
		if err := o.vault.Hold(o.game + "/" + o.domain.stagings); err != nil {
			return errVault(rec.Vault, "%w", err)
		}
	}
	switch rec.Action {
	case RecoverVerifyPromote, RecoverUnpromote, RecoverRestore, RecoverFinishRestore:
		// The old generation moves back from it; it exists since M1.
		if err := o.vault.Hold(o.game + "/" + retainedDir); err != nil {
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
	case RecoverVerifyPublish, RecoverVerifyPromote:
		return o.verifyThenPublish(rec)
	case RecoverUnpublish:
		return o.unpublish(rec.Last == backup.StepRollbackFailed)
	case RecoverRestore:
		return o.restoreOld("", true, false)
	case RecoverFinishRestore:
		return o.restoreOld("", false, rec.Last == backup.StepRollbackFailed)
	case RecoverUnpromote:
		return o.unpromote(rec.Last == backup.StepRollbackFailed)
	}
	return nil
}

// verifyThenPublish handles a staging already renamed to current: current
// is verified in full against the plan; equal, its receipt and published
// are committed, the receipt rebuilt if missing; different, it is moved
// back to staging, and a rotation restores its retained generation.
func (o *operation) verifyThenPublish(rec *Recovery) error {
	segments := append(append([]string{}, o.plan.vaultSegments...), gamesDir, o.plan.Game.ID, currentDir)
	dir, err := o.s.inventories.Directory(o.ctx, o.plan.vaultVolume, segments)
	if err != nil {
		return err
	}
	if id, err := o.vault.Identity(o.game + "/" + currentDir); err != nil || id != o.staging {
		// Swapped while it was read: nothing is recorded on its strength.
		return o.intervene(fmt.Sprintf("current was replaced while it was read, or cannot be checked: %v %v; nothing recorded", id, err))
	}
	_, receiptCommitted := rec.observed.receiptSum[o.op]
	if differences := compareWithReceipt(dir, backup.Receipt{Sources: o.planned.Sources}); len(differences) > 0 {
		if receiptCommitted {
			// A5 and A6: a receipt already describes current, which no
			// longer matches; moving it back would leave the receipt orphan.
			return o.intervene(fmt.Sprintf("current no longer matches the plan of %s, whose receipt is committed: %s %s, %s",
				o.op, orDash(differences[0].source), orDash(differences[0].path), differences[0].detail))
		}
		if o.rotation() {
			if err := o.record(backup.StepUnpromoteIntent, backup.StepValues{Current: o.staging}); err != nil {
				return err
			}
			return o.unpromote(false)
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
	moved, err := o.switchTo(o.game+"/"+o.domain.published(o.op), o.stagingDir(), o.staging)
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
		return o.intervene("rollback failed: " + err.Error() + "; close the programs that use the vault, then run " + o.domain.command + " --recover --apply again")
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
