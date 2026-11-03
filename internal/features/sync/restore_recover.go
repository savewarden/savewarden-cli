package sync

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/entities/game"
)

// Recovery actions of a restore area (ADR 0006 §11).
const (
	RecoverAreaMarker       RecoveryAction = "commit the marker of the area; its temporaries stay in place"
	RecoverRestoreAbandon   RecoveryAction = "record abandoned; the incomplete copy, if any, stays in place"
	RecoverRestorePublish   RecoveryAction = "verify the restore against the plan, then commit its receipt and published; if it differs, move it back to incomplete"
	RecoverRestoreConfirm   RecoveryAction = "read the committed receipt and verify the restore against the plan, then commit published"
	RecoverRestoreUnpublish RecoveryAction = "move the restore back to incomplete, then record rolled-back"
)

// RestoreRecovery is what restore --recover found, without writing
// anything. It relies on the restore area and the manifest alone: the
// vault is never opened, read or named (ADR 0006 §11).
type RestoreRecovery struct {
	ManifestPath string
	Game         game.Game
	Destination  string
	State        string
	// Volume is described only for a recovery that may write.
	Volume backup.Volume
	// Operation is the interrupted restore operation, empty when there is
	// none; Observed, its two names and its receipt.
	Operation string
	Last      backup.Step
	Observed  *backup.RestoreObservation
	Action    RecoveryAction
	// Restores are the handed-over restores of the area, oldest first.
	Restores []HandedOverRestore

	area         *area
	journal      backup.Journal
	destVolume   string
	destSegments []string
}

// Warning is the notice of a restore renamed but not yet published, steps
// A3 to A6 (ADR 0009): its copy must not be used before recovery ends.
func (r RestoreRecovery) Warning() string {
	if r.Last != backup.StepPublishIntent || r.Observed == nil || r.Observed.Published != backup.New {
		return ""
	}
	return fmt.Sprintf(`operation %s pending recovery: copy published, receipt missing; do not use %s\%s yet`, r.Operation, r.Destination, r.Operation)
}

// RestoreRecoverPlan classifies the destination, finds the interrupted
// restore operation of the area, observes its two names and its receipt,
// and decides its recovery without writing anything (ADR 0006 §11). A
// destination with nothing to recover concludes at once; for any other,
// the destination is checked as at creation, the vault aside. An ambiguity
// is an InterventionError.
func (s *Service) RestoreRecoverPlan(ctx context.Context, request RestoreRequest) (RestoreRecovery, error) {
	return s.restoreRecoverPlan(ctx, request, false)
}

// restoreRecoverPlan is RestoreRecoverPlan; a run that holds the lock of
// the area does not probe it.
func (s *Service) restoreRecoverPlan(ctx context.Context, request RestoreRequest, underLock bool) (RestoreRecovery, error) {
	manifest, err := s.manifests.Read(request.ManifestPath)
	if err != nil {
		return RestoreRecovery{}, err
	}
	path := request.Destination
	volume, segments, err := game.ParseWindowsAbsolute(path)
	if err != nil {
		return RestoreRecovery{}, errDestination(path, "%v", err)
	}
	rec := RestoreRecovery{ManifestPath: request.ManifestPath, Game: manifest.Game, Destination: path, Action: RecoverNothing,
		destVolume: volume, destSegments: segments}
	if len(segments) == 0 {
		return rec, errDestination(path, "is the root of a volume; choose a directory")
	}
	scope, err := s.files.OpenScope(volume)
	if errors.Is(err, fs.ErrNotExist) {
		return rec, errDestination(path, "volume %s is not available", volume)
	}
	if err != nil {
		return rec, errDestination(path, "open volume %s: %v", volume, err)
	}
	defer func() { _ = scope.Close() }()
	name := strings.Join(segments, "/")
	a, err := readArea(scope, path, name, manifest.Game.ID)
	if err != nil {
		return rec, err
	}
	rec.area = a
	switch {
	case !a.exists:
		rec.State = DestinationNew
		return rec, nil
	case a.root.State == backup.RestoreRootEmpty:
		rec.State = DestinationEmpty
		return rec, nil
	}
	rec.State, rec.Restores = DestinationArea, a.restores
	if err := ctx.Err(); err != nil {
		return rec, fmt.Errorf("recovery plan interrupted: %w", err)
	}
	if a.root.State == backup.RestoreRootInterrupted {
		if a.root.Temporaries >= backup.MaxTemporaries {
			return rec, &InterventionError{Subject: areaSubject, Cause: backup.ErrEnvelopeExhausted.Error() + ": nine marker temporaries; the area cannot be marked"}
		}
		rec.Action = RecoverAreaMarker
	} else {
		// While another process holds the lock, a restore is running: what it
		// is writing is not judged.
		running := len(a.pending) > 0 || len(a.interventions) > 0
		if !underLock && a.root.Lock && running && lockBusy(scope, join(name, backup.RestoreLockName)) {
			return rec, errDestination(path, "another restore is running in this area; run it again once it ends")
		}
		for _, intervention := range a.interventions {
			if intervention.Cause != CausePendingOperation {
				return rec, &InterventionError{Subject: areaSubject, Cause: string(intervention.Cause) + " " + intervention.Subject + ": " + intervention.Detail}
			}
		}
		if len(a.pending) == 0 {
			// The structure alone, or handed-over restores: nothing to recover.
			return rec, nil
		}
		if err := rec.classify(scope, name); err != nil {
			return rec, err
		}
	}
	// The recovery may write: the destination is checked as at creation,
	// against the installation, the sources and the paths the area's plans
	// backed up, never against the vault, which it does not name.
	if rec.Volume, err = s.volumes.Describe(windowsPath(volume, segments)); err != nil {
		return rec, errDestination(path, "volume cannot be described: %v", err)
	}
	if err := qualifyVolume(rec.Volume, func(format string, args ...any) error { return errDestination(path, format, args...) }); err != nil {
		return rec, err
	}
	refs := manifestReferences(manifest)
	for _, j := range a.journals {
		if j.RestorePlan != nil {
			refs = append(refs, backedUpReferences(j.RestorePlan.Sources)...)
		}
	}
	return rec, s.distinctDestination(path, volume, segments, refs)
}

// classify decides the recovery of the pending operation of the area from
// its journal and the observation of its names, per the table of ADR 0006
// §11.
func (r *RestoreRecovery) classify(scope Scope, name string) error {
	a := r.area
	r.Operation = a.pending[0].Operation
	r.journal = a.journals[r.Operation]
	r.Last = r.journal.Last()
	observed := backup.RestoreObservation{
		Published:  incompletePresence(scope, join(name, r.Operation), r.journal),
		Incomplete: incompletePresence(scope, join(name, backup.RestoreStagingName(r.Operation)), r.journal),
		Receipt:    a.receipts[r.Operation] != nil,
	}
	r.Observed = &observed
	action, err := backup.ClassifyRestore(r.journal, observed)
	if err != nil {
		return &InterventionError{Operation: r.Operation, Cause: err.Error()}
	}
	switch action {
	case backup.ActionAbandon:
		r.Action = RecoverRestoreAbandon
	case backup.ActionMarkRolledBack:
		r.Action = RecoverRolledBack
	case backup.ActionVerifyThenPublish:
		if len(a.receiptTaken[r.Operation]) >= backup.MaxTemporaries {
			return &InterventionError{Operation: r.Operation, Cause: backup.ErrEnvelopeExhausted.Error() + ": nine receipt temporaries"}
		}
		r.Action = RecoverRestorePublish
	case backup.ActionConfirmReceipt:
		r.Action = RecoverRestoreConfirm
	case backup.ActionFinishUnpublish:
		r.Action = RecoverRestoreUnpublish
	default:
		return &InterventionError{Operation: r.Operation, Cause: fmt.Sprintf("recovery %q is not available for a restore", action)}
	}
	return nil
}

// RestoreRecover plans the recovery of the area, then performs it: an
// interrupted marker is validated without the lock; an operation is
// recovered under the lock of the area after reading it again, the same
// operation, observation and action, or nothing is done. The vault is
// never opened.
func (s *Service) RestoreRecover(ctx context.Context, request RestoreRequest, w Writer) (RestoreResult, error) {
	rec, err := s.RestoreRecoverPlan(ctx, request)
	result := RestoreResult{Operation: rec.Operation, Recovery: &rec}
	var intervention *InterventionError
	if errors.As(err, &intervention) {
		result.Effect, result.Cause = EffectIntervention, intervention.Cause
		return result, err
	}
	if err != nil || rec.Action == RecoverNothing {
		return result, err
	}
	t, err := areaTarget(rec.Destination)
	if err != nil {
		return result, err
	}
	session, err := w.Vaults.Open(rec.Destination)
	if err != nil {
		return result, errDestination(rec.Destination, "%v", err)
	}
	o := &operation{
		s: s, w: w, ctx: ctx, vault: session, domain: restoreDomain, op: rec.Operation, target: t, recovering: true,
		attempts: map[int]map[int]bool{}, receipt: map[int]bool{}, marker: map[int]bool{},
	}
	err = o.unfinished(o.recoverRestore(&rec, request))
	if closeErr := session.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	result.Effect = o.effect
	if errors.As(err, &intervention) {
		result.Cause = intervention.Cause
		// Code 3 announces its cause, whatever was written before (ADR 0009).
		if o.effect == EffectNone || o.effect == EffectMetadata || o.effect == EffectStaged {
			result.Effect = EffectIntervention
		}
	}
	return result, err
}

func (o *operation) recoverRestore(rec *RestoreRecovery, request RestoreRequest) error {
	if rec.Action == RecoverAreaMarker {
		return o.resumeAreaMarker(rec)
	}
	for _, dir := range []string{o.at(o.domain.journals), o.journalDir()} {
		if err := o.vault.Hold(dir); err != nil {
			return o.target.errorf("%w", err)
		}
	}
	// The lock is taken only in a marked area, created empty if missing.
	if err := o.vault.Lock(o.at(o.target.lock)); err != nil {
		var busy interface{ Busy() bool }
		if errors.As(err, &busy) && busy.Busy() {
			return o.target.errorf("%s", o.domain.busy)
		}
		return o.target.errorf("lock: %w", err)
	}
	if !rec.area.root.Lock {
		o.wrote()
	}
	again, err := o.s.restoreRecoverPlan(o.ctx, request, true)
	if err != nil {
		return err
	}
	if again.Operation != rec.Operation || again.Last != rec.Last || again.Action != rec.Action ||
		(again.Observed == nil) != (rec.Observed == nil) || (again.Observed != nil && *again.Observed != *rec.Observed) {
		return o.target.errorf("destination-changed: the restore area changed since the recovery plan; run restore --recover again")
	}
	*rec = again
	j := rec.journal
	o.seq, o.prev = len(j.Records), rec.area.lastRecord[o.op]
	o.attempts[o.seq+1] = map[int]bool{}
	for k := range j.Taken {
		o.attempts[o.seq+1][k] = true
	}
	for k := range rec.area.receiptTaken[o.op] {
		o.receipt[k] = true
	}
	o.restorePlan, o.planSum, o.staging = j.RestorePlan, j.PlanSHA256, j.Staging
	o.restored, o.snapshotIdentity = j.RestoredSnapshot, j.SnapshotIdentity
	if j.RestorePlan != nil {
		o.content = j.RestorePlan.Sources
	}
	for _, r := range j.Records {
		if r.Step == backup.StepPublishIntent {
			o.intentAt = r.At
		}
	}
	o.critical = true
	switch rec.Action {
	case RecoverRestoreAbandon:
		if err := o.record(backup.StepAbandoned, backup.StepValues{Text: "recovered: interrupted before publication"}); err != nil {
			return err
		}
		// "kept incomplete" only when a copy is there to be kept (ADR 0009).
		o.effect = EffectMetadata
		if rec.Observed != nil && rec.Observed.Incomplete != backup.Absent {
			o.effect = EffectStaged
		}
	case RecoverRolledBack:
		if err := o.record(backup.StepRolledBack, backup.StepValues{}); err != nil {
			return err
		}
		o.effect = EffectRolledBack
	case RecoverRestorePublish, RecoverRestoreConfirm:
		return o.verifyRestoreThenPublish(rec)
	case RecoverRestoreUnpublish:
		return o.unpublish(rec.Last == backup.StepRollbackFailed)
	}
	return nil
}

// verifyRestoreThenPublish handles a copy already renamed to <dest>\<op>,
// steps A3 to A6: verified in full against the plan journaled in the area,
// its identity read again after the content, without reading the vault.
// Without a committed receipt, a copy that differs is moved back, code 2;
// with one, that receipt is the proof and nothing is ever moved back.
func (o *operation) verifyRestoreThenPublish(rec *RestoreRecovery) error {
	published := o.at(o.domain.published(o.op))
	dir, err := o.readBack(published)
	if err != nil {
		return err
	}
	if id, err := o.vault.Identity(published); err != nil || id != o.staging {
		return o.intervene(fmt.Sprintf("the restore was replaced while it was read, or cannot be checked: %v %v; nothing recorded", id, err))
	}
	differences := compareWithReceipt(dir, backup.Receipt{Sources: o.content})
	if rec.Action == RecoverRestoreConfirm {
		if len(differences) > 0 {
			return o.intervene(fmt.Sprintf("the restore no longer matches the plan of %s, whose receipt is committed: %s %s, %s",
				o.op, orDash(differences[0].source), orDash(differences[0].path), differences[0].detail))
		}
		data := rec.area.receipts[o.op]
		if _, err := backup.CheckRestoreReceipt(data, rec.journal); err != nil {
			return o.intervene("the receipt of " + o.op + " does not match its plan: " + err.Error())
		}
		if err := o.record(backup.StepPublished, backup.StepValues{Digest: sha256.Sum256(data)}); err != nil {
			return o.intervene("the restore and its receipt are published but the journal is not closed: " + err.Error())
		}
		o.critical, o.effect = false, EffectPublished
		return nil
	}
	if len(differences) > 0 {
		first := differences[0]
		if err := o.record(backup.StepUnpublishIntent, backup.StepValues{Staging: o.staging}); err != nil {
			return err
		}
		if err := o.unpublish(false); err != nil {
			return err
		}
		return &BlockedError{Code: CodeVerificationMismatch, Detail: fmt.Sprintf("restore %s %s %s: %s; %d difference(s) in all, moved back to %s",
			o.op, orDash(first.source), orDash(first.path), strings.ReplaceAll(first.detail, "its receipt", "the plan"), len(differences),
			backup.RestoreStagingName(o.op))}
	}
	now := o.now()
	digest, err := o.writeRestoreReceipt(nil, &now, o.w.ToolVersion)
	if err != nil {
		return o.intervene("the restore is published but its receipt cannot be written: " + err.Error())
	}
	if err := o.record(backup.StepPublished, backup.StepValues{Digest: digest}); err != nil {
		return o.intervene("the restore and its receipt are published but the journal is not closed: " + err.Error())
	}
	o.critical, o.effect = false, EffectPublished
	return nil
}

// resumeAreaMarker resumes an interrupted initialization, step Z3: the
// destination, pinned by the session, is read again and must hold only
// marker temporaries; a new marker is validated at the next free attempt,
// without any lock, the temporaries kept (ADR 0006 §11). A marker another
// run validated meanwhile leaves nothing to do.
func (o *operation) resumeAreaMarker(rec *RestoreRecovery) error {
	scope, err := o.s.files.OpenScope(o.target.volume)
	if err != nil {
		return o.target.errorf("open volume %s: %v", o.target.volume, err)
	}
	entries, err := list(scope, strings.Join(o.target.segments, "/"))
	_ = scope.Close()
	if err != nil {
		return o.target.errorf("cannot be read again before its marker: %v", err)
	}
	root := backup.ClassifyRestoreRoot(entries)
	switch root.State {
	case backup.RestoreRootMarked:
		// Validated by another run: a marked area without operation.
		return nil
	case backup.RestoreRootInterrupted:
	default:
		return o.target.errorf("destination-changed: it no longer holds only marker temporaries; nothing is marked, run restore --recover again")
	}
	for k := range root.Taken {
		o.marker[k] = true
	}
	marker := backup.RestoreAreaMarker{CreatedAt: o.now(), ToolVersion: o.w.ToolVersion, GameID: rec.Game.ID}
	data, err := backup.EncodeRestoreAreaMarker(marker)
	if err != nil {
		return err
	}
	return o.commitAreaMarker(data)
}
