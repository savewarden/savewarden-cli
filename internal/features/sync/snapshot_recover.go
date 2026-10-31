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

// Recovery actions of a snapshot operation (ADR 0006 §10).
const (
	RecoverSnapshotAbandon   RecoveryAction = "record abandoned; the incomplete copy, if any, stays in place"
	RecoverSnapshotPublish   RecoveryAction = "verify the snapshot against the plan and its provenance, then commit its receipt and published; if it differs, move it back to incomplete"
	RecoverSnapshotConfirm   RecoveryAction = "read the committed receipt and verify the snapshot against the plan, then commit published"
	RecoverSnapshotUnpublish RecoveryAction = "move the snapshot back to incomplete, then record rolled-back"
)

// SnapshotRecovery is what snapshot --recover found, without writing
// anything. It relies on the snapshot domain alone: current, its
// operations and its anomalies are never read nor judged (ADR 0006 §10).
type SnapshotRecovery struct {
	ManifestPath string
	Game         game.Game
	Vault        string
	Volume       backup.Volume
	// Operation is the interrupted snapshot operation, empty when there is
	// none.
	Operation string
	Last      backup.Step
	Observed  *backup.SnapshotObservation
	Action    RecoveryAction

	vaultVolume   string
	vaultSegments []string
	domain        *snapshotVault
	journal       backup.Journal
}

// SnapshotRecoverPlan finds the interrupted snapshot operation of the
// game, observes its two names and its receipt, and decides its recovery
// without writing anything. An ambiguity is an InterventionError.
func (s *Service) SnapshotRecoverPlan(ctx context.Context, request SnapshotRequest) (SnapshotRecovery, error) {
	manifest, err := s.manifests.Read(request.ManifestPath)
	if err != nil {
		return SnapshotRecovery{}, err
	}
	vaultVolume, vaultSegments, err := game.ParseWindowsAbsolute(request.VaultPath)
	if err != nil {
		return SnapshotRecovery{}, fmt.Errorf("vault %s: %w", request.VaultPath, err)
	}
	installVolume, installSegments, err := game.ParseWindowsAbsolute(manifest.Installation.Path)
	if err != nil {
		return SnapshotRecovery{}, err
	}
	if err := distinctByName(request.VaultPath, vaultVolume, vaultSegments, installVolume, installSegments); err != nil {
		return SnapshotRecovery{}, err
	}
	rec := SnapshotRecovery{ManifestPath: request.ManifestPath, Game: manifest.Game, Vault: request.VaultPath, Action: RecoverNothing,
		vaultVolume: vaultVolume, vaultSegments: vaultSegments, domain: &snapshotVault{}}
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
	if !v.exists {
		return rec, errVault(request.VaultPath, "does not exist")
	}
	if rec.Volume, err = s.describe(request.VaultPath, vaultVolume, vaultSegments, true); err != nil {
		return rec, err
	}
	if strings.EqualFold(vaultVolume, installVolume) {
		install, complete := chainOf(scope, installSegments)
		if overlapping(scope, v.chain, true, install, complete) {
			return rec, errVault(request.VaultPath, "is the installation, contains it or lies in it under another name; the vault must be distinct")
		}
	}
	if err := s.checkSources(manifest, installVolume, vaultVolume, scope); err != nil {
		return rec, err
	}
	root := strings.Join(vaultSegments, "/")
	data, err := readFile(scope, join(root, backup.MarkerName), markerReadLimit)
	if err != nil {
		return rec, errVault(request.VaultPath, "not a Savewarden vault: %v", err)
	}
	if _, err := backup.DecodeMarker(data); err != nil {
		return rec, errVault(request.VaultPath, "%v; a marker is never rewritten", err)
	}
	// Only the path to the journals is required; the rest of the vault,
	// the current domain included, is never read.
	gameName := join(root, gamesDir+"/"+manifest.Game.ID)
	for _, dir := range []string{join(root, gamesDir), gameName, join(gameName, snapshotsDir), join(gameName, snapshotsDir+"/"+journalDir)} {
		info, err := scope.Lstat(dir)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return rec, nil
		case err != nil:
			return rec, errVault(request.VaultPath, "%v", err)
		case isLink(info.Mode()) || !info.IsDir():
			return rec, &InterventionError{Cause: string(CauseUnexpectedEntry) + " " + strings.TrimPrefix(dir, root+"/") + ": not a plain directory"}
		}
	}
	if err := ctx.Err(); err != nil {
		return rec, fmt.Errorf("recovery plan interrupted: %w", err)
	}
	prefix := gamesDir + "/" + manifest.Game.ID + "/"
	if rec.domain, err = readSnapshots(scope, request.VaultPath, gameName, prefix, manifest.Game.ID, nil); err != nil {
		return rec, err
	}
	for _, intervention := range rec.domain.interventions {
		if intervention.Cause != CausePendingOperation {
			return rec, &InterventionError{Cause: string(intervention.Cause) + " " + intervention.Subject + ": " + intervention.Detail}
		}
	}
	if len(rec.domain.pending) == 0 {
		return rec, nil
	}
	rec.Operation = rec.domain.pending[0].Operation
	rec.journal = rec.domain.journals[rec.Operation]
	rec.Last = rec.journal.Last()
	base := join(gameName, snapshotsDir)
	observed := backup.SnapshotObservation{
		Published:  incompletePresence(scope, join(base, rec.Operation), rec.journal),
		Incomplete: incompletePresence(scope, join(base, rec.Operation+".incomplete"), rec.journal),
		Receipt:    rec.domain.receipts[rec.Operation] != nil,
	}
	rec.Observed = &observed
	action, err := backup.ClassifySnapshot(rec.journal, observed)
	if err != nil {
		return rec, &InterventionError{Operation: rec.Operation, Cause: err.Error()}
	}
	switch action {
	case backup.ActionAbandon:
		rec.Action = RecoverSnapshotAbandon
	case backup.ActionMarkRolledBack:
		rec.Action = RecoverRolledBack
	case backup.ActionVerifyThenPublish:
		if len(rec.domain.receiptTaken[rec.Operation]) >= backup.MaxTemporaries {
			return rec, &InterventionError{Operation: rec.Operation, Cause: backup.ErrEnvelopeExhausted.Error() + ": nine receipt temporaries"}
		}
		rec.Action = RecoverSnapshotPublish
	case backup.ActionConfirmReceipt:
		rec.Action = RecoverSnapshotConfirm
	case backup.ActionFinishUnpublish:
		rec.Action = RecoverSnapshotUnpublish
	default:
		return rec, &InterventionError{Operation: rec.Operation, Cause: fmt.Sprintf("recovery %q is not available for a snapshot", action)}
	}
	return rec, nil
}

// incompletePresence describes a name of the snapshot domain by identity,
// without following it: New is the copy staged recorded; before staged, a
// plain directory is Unidentified.
func incompletePresence(scope Scope, name string, j backup.Journal) backup.Presence {
	id, err := scope.Identity(name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return backup.Absent
	case err != nil:
		return backup.Other
	case j.Staging != (backup.FileID{}) && id == j.Staging:
		return backup.New
	case j.Staging == (backup.FileID{}):
		if info, err := scope.Lstat(name); err == nil && info.IsDir() && !isLink(info.Mode()) {
			return backup.Unidentified
		}
	}
	return backup.Other
}

// SnapshotRecover plans the recovery of a snapshot operation, then
// performs it under the game lock after reading the snapshot domain again:
// the same operation, observation and action, or nothing is done.
func (s *Service) SnapshotRecover(ctx context.Context, request SnapshotRequest, w Writer) (SnapshotResult, error) {
	rec, err := s.SnapshotRecoverPlan(ctx, request)
	result := SnapshotResult{Operation: rec.Operation, Recovery: &rec}
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
		s: s, w: w, ctx: ctx, vault: session, domain: snapshotDomain, op: rec.Operation, game: gamesDir + "/" + rec.Game.ID, recovering: true,
		plan:     Plan{Vault: rec.Vault, Game: rec.Game, vaultVolume: rec.vaultVolume, vaultSegments: rec.vaultSegments},
		attempts: map[int]map[int]bool{}, receipt: map[int]bool{}, marker: map[int]bool{},
	}
	err = o.unfinished(o.recoverSnapshot(&rec, request))
	if closeErr := session.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	result.Effect = o.effect
	if errors.As(err, &intervention) {
		result.Cause = intervention.Cause
		if o.effect == EffectNone || o.effect == EffectStaged {
			result.Effect = EffectIntervention
		}
	}
	return result, err
}

func (o *operation) recoverSnapshot(rec *SnapshotRecovery, request SnapshotRequest) error {
	for _, dir := range []string{gamesDir, o.game, o.game + "/" + snapshotsDir, o.game + "/" + o.domain.journals, o.journalDir()} {
		if err := o.vault.Hold(dir); err != nil {
			return errVault(rec.Vault, "%w", err)
		}
	}
	// The lock is taken as sync takes it: its absence never makes a
	// snapshot recovery depend on the current domain.
	if err := o.vault.Lock(o.game + "/" + lockFile); err != nil {
		var busy interface{ Busy() bool }
		if errors.As(err, &busy) && busy.Busy() {
			return errVault(rec.Vault, "%s", o.domain.busy)
		}
		return errVault(rec.Vault, "lock: %w", err)
	}
	again, err := o.s.SnapshotRecoverPlan(o.ctx, request)
	if err != nil {
		return err
	}
	if again.Operation != rec.Operation || again.Last != rec.Last || again.Action != rec.Action ||
		(again.Observed == nil) != (rec.Observed == nil) || (again.Observed != nil && *again.Observed != *rec.Observed) {
		return errVault(rec.Vault, "vault-changed: the vault changed since the recovery plan; run snapshot --recover again")
	}
	*rec = again
	j := rec.journal
	o.seq, o.prev = len(j.Records), rec.domain.lastRecord[o.op]
	o.attempts[o.seq+1] = map[int]bool{}
	for k := range j.Taken {
		o.attempts[o.seq+1][k] = true
	}
	for k := range rec.domain.receiptTaken[o.op] {
		o.receipt[k] = true
	}
	o.snapshotPlan, o.planSum, o.staging = j.SnapshotPlan, j.PlanSHA256, j.Staging
	o.source, o.sourceIdentity = j.SnapshotSource, j.SourceIdentity
	if j.SnapshotPlan != nil {
		o.content = j.SnapshotPlan.Sources
	}
	for _, r := range j.Records {
		if r.Step == backup.StepPublishIntent {
			o.intentAt = r.At
		}
	}
	if rec.Action == RecoverSnapshotPublish {
		if err := o.vault.Hold(o.game + "/" + snapshotsDir + "/" + receiptsDir); err != nil {
			return errVault(rec.Vault, "%w", err)
		}
	}
	o.critical = true
	switch rec.Action {
	case RecoverSnapshotAbandon:
		if err := o.record(backup.StepAbandoned, backup.StepValues{Text: "recovered: interrupted before publication"}); err != nil {
			return err
		}
		o.effect = EffectStaged
	case RecoverRolledBack:
		if err := o.record(backup.StepRolledBack, backup.StepValues{}); err != nil {
			return err
		}
		o.effect = EffectRolledBack
	case RecoverSnapshotPublish, RecoverSnapshotConfirm:
		return o.verifySnapshotThenPublish(rec)
	case RecoverSnapshotUnpublish:
		return o.unpublish(rec.Last == backup.StepRollbackFailed)
	}
	return nil
}

// verifySnapshotThenPublish handles a copy already renamed to its snapshot:
// verified in full against the plan, its identity read again after the
// content. Without a committed receipt, the provenance gate decides; with
// one (A5, A6), that receipt is the proof and nothing is ever moved back.
func (o *operation) verifySnapshotThenPublish(rec *SnapshotRecovery) error {
	published := o.game + "/" + o.domain.published(o.op)
	segments := append(append([]string{}, o.plan.vaultSegments...), strings.Split(published, "/")...)
	dir, err := o.s.inventories.Directory(o.ctx, o.plan.vaultVolume, segments)
	if err != nil {
		return err
	}
	if id, err := o.vault.Identity(published); err != nil || id != o.staging {
		return o.intervene(fmt.Sprintf("the snapshot was replaced while it was read, or cannot be checked: %v %v; nothing recorded", id, err))
	}
	differences := compareWithReceipt(dir, backup.Receipt{Sources: o.content})
	if rec.Action == RecoverSnapshotConfirm {
		if len(differences) > 0 {
			return o.intervene(fmt.Sprintf("the snapshot no longer matches the plan of %s, whose receipt is committed: %s %s, %s",
				o.op, orDash(differences[0].source), orDash(differences[0].path), differences[0].detail))
		}
		data := rec.domain.receipts[o.op]
		if _, err := backup.CheckSnapshotReceipt(data, rec.journal); err != nil {
			return o.intervene("the receipt of " + o.op + " does not match its plan: " + err.Error())
		}
		if err := o.record(backup.StepPublished, backup.StepValues{Digest: sha256.Sum256(data)}); err != nil {
			return o.intervene("the snapshot and its receipt are published but the journal is not closed: " + err.Error())
		}
		o.critical, o.effect = false, EffectPublished
		return nil
	}
	if len(differences) > 0 {
		if err := o.record(backup.StepUnpublishIntent, backup.StepValues{Staging: o.staging}); err != nil {
			return err
		}
		return o.unpublish(false)
	}
	if err := o.snapshotProvenance(); err != nil {
		return err
	}
	now := o.now()
	digest, err := o.writeSnapshotReceipt(nil, &now, o.w.ToolVersion)
	if err != nil {
		return o.intervene("the snapshot is published but its receipt cannot be written: " + err.Error())
	}
	if err := o.record(backup.StepPublished, backup.StepValues{Digest: digest}); err != nil {
		return o.intervene("the snapshot and its receipt are published but the journal is not closed: " + err.Error())
	}
	o.critical, o.effect = false, EffectPublished
	return nil
}
