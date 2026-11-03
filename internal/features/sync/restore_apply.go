package sync

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/entities/game"
)

// restoreDomain holds the restores of a restore area and their operations,
// at its root (ADR 0006 §11).
var restoreDomain = domain{
	command:   "restore",
	journals:  backup.RestoreJournalDir,
	stagings:  ".",
	staging:   backup.RestoreStagingName,
	published: backup.RestoreName,
	newRecord: backup.NewRestoreRecord,
	changed:   CodeChangedDuringRestore,
	busy:      "another restore is running in this area",
}

// areaSubject names a restore area in an intervention no operation explains.
const areaSubject = "the restore area"

// areaTarget is the target of a restore: the restore area at path, whose
// operations live at its root (ADR 0006 §11).
func areaTarget(path string) (target, error) {
	volume, segments, err := game.ParseWindowsAbsolute(path)
	if err != nil {
		return target{}, errDestination(path, "%v", err)
	}
	return target{label: "destination", path: path, volume: volume, segments: segments, dir: ".", lock: backup.RestoreLockName}, nil
}

// RestoreResult reports a restore --apply run, complete or not. Its State
// says what the restore area now holds (ADR 0009); the vault is never
// written.
type RestoreResult struct {
	Plan      RestorePlan
	Operation string
	Effect    Effect
	// Cause explains EffectIntervention, or a run closed in code 3.
	Cause string
	// Restore and Receipt are the Windows paths of a published restore;
	// Files and Bytes count what was verified.
	Restore string
	Receipt string
	Files   int
	Bytes   int64
}

// State is the state of the restore area, as "savewarden: destination:
// <state>" announces it (ADR 0009).
func (r RestoreResult) State() string {
	switch r.Effect {
	case EffectMetadata:
		return "metadata only; nothing staged or published"
	case EffectStaged:
		return "restore " + r.Operation + " kept incomplete"
	case EffectPublished:
		return "published restore " + r.Operation
	case EffectRolledBack:
		return "rolled back restore " + r.Operation
	case EffectIntervention:
		subject := r.Operation
		if subject == "" {
			subject = areaSubject
		}
		return "intervention required for " + subject + ": " + r.Cause
	}
	return "not modified"
}

// RestoreApply plans, then marks the restore area if it is new, copies the
// snapshot into <dest>\<op>.incomplete, verifies the copy and publishes it
// as <dest>\<op> with its receipt (ADR 0006 §11). It never opens a session
// on the vault. Its Result says what the area now holds even when it fails.
func (s *Service) RestoreApply(ctx context.Context, request RestoreRequest, w Writer) (RestoreResult, error) {
	plan, err := s.RestorePlan(ctx, request)
	if err != nil {
		return RestoreResult{}, err
	}
	result := RestoreResult{Plan: plan}
	switch {
	case len(plan.Interventions) > 0:
		first := plan.Interventions[0]
		result.Effect, result.Cause = EffectIntervention, string(first.Cause)+" "+first.Subject
		return result, &InterventionError{Subject: areaSubject, Cause: result.Cause + "; see restore --recover"}
	case plan.Blocking() > 0:
		return result, &BlockedError{Detail: fmt.Sprintf("%d blocking finding(s)", plan.Blocking())}
	}
	t, err := areaTarget(plan.Destination)
	if err != nil {
		return result, err
	}
	reads, err := vaultReadRoot(plan.Vault, plan.Game.ID)
	if err != nil {
		return result, err
	}
	o, err := s.open(ctx, t, restoreDomain, w)
	if err != nil {
		return result, err
	}
	o.reads = reads
	err = o.unfinished(o.restoreSnapshot(request, plan))
	if closeErr := o.vault.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	result.Operation, result.Effect = o.op, o.effect
	if result.Effect == EffectIntervention && !o.open {
		// Refused before its directory existed: the area is the subject.
		result.Operation = ""
	}
	var intervention *InterventionError
	if errors.As(err, &intervention) {
		result.Cause = intervention.Cause
	}
	if o.effect == EffectPublished {
		result.Restore = t.windowsPath(o.op)
		result.Receipt = t.windowsPath(backup.RestoreReceiptsDir + "/" + o.op + ".json")
		for _, source := range o.content {
			result.Files += len(source.Entries)
			result.Bytes += source.Bytes()
		}
	}
	return result, err
}

// restoreSnapshot performs the operation; the caller closes the session.
func (o *operation) restoreSnapshot(request RestoreRequest, plan RestorePlan) error {
	if err := o.restoreUnderLock(request, plan); err != nil {
		return err
	}
	for _, phase := range []func() error{o.copySnapshot, o.verifyIncomplete} {
		if err := phase(); err != nil {
			return o.fail(err)
		}
	}
	return o.publishRestore()
}

// restoreUnderLock marks a new area (Z2 to Z4), takes its lock (Z5), plans
// again under it (Z6), then creates the structure and the operation
// directory (Z7, Z8) and commits the planned record (Z9).
func (o *operation) restoreUnderLock(request RestoreRequest, before RestorePlan) error {
	if before.State != DestinationArea {
		if err := o.initializeArea(before); err != nil {
			return err
		}
	}
	// Z5: the lock, taken as sync.lock is, created empty if it is missing.
	if err := o.vault.Lock(o.at(o.target.lock)); err != nil {
		var busy interface{ Busy() bool }
		if errors.As(err, &busy) && busy.Busy() {
			return o.target.errorf("%s", o.domain.busy)
		}
		return o.target.errorf("lock: %w", err)
	}
	if before.area == nil || !before.area.root.Lock {
		o.wrote()
	}
	// Z6: under lock, the area and the snapshot are read again, the
	// content of the snapshot excepted, and the space checked again.
	again, err := o.s.restorePlan(o.ctx, request, true)
	if err != nil {
		return err
	}
	if err := o.sameArea(before, again); err != nil {
		return err
	}
	for _, finding := range again.Findings {
		if finding.Severity == backup.Blocking {
			return &BlockedError{Code: finding.Code, Detail: finding.Detail + " (under lock)"}
		}
	}
	src := again.snapshot
	if src == nil || before.snapshot == nil || src.receiptSum != before.snapshot.receiptSum || src.identity != before.snapshot.identity ||
		again.From != before.From {
		return o.target.errorf("snapshot %s changed since the plan; run restore again", before.Snapshot)
	}
	// The id is checked before any journal: never shared in the area, nor
	// equal to the snapshot or to the generation it copies.
	if again.area.used(o.op) || o.op == before.Snapshot || o.op == before.From {
		return o.target.errorf("operation id %s is already used in this area, or names the snapshot or its generation; run restore again", o.op)
	}
	// Z7: the structure, made of exclusive creations, after the space check.
	for _, dir := range []string{backup.RestoreReceiptsDir, backup.RestoreJournalDir} {
		if err := o.ensureDir(o.at(dir)); err != nil {
			return o.target.errorf("%w", err)
		}
	}
	// Only the ancestors of the journal stay pinned across the switch;
	// receipts is held again to be written.
	if err := o.vault.Release(o.at(backup.RestoreReceiptsDir)); err != nil {
		return o.target.errorf("%w", err)
	}
	// Z8: the operation directory, exclusive.
	if err := o.vault.CreateDir(o.journalDir()); err != nil {
		return o.target.errorf("operation directory: %w", err)
	}
	// Metadata only until a copy exists to be kept (ADR 0009).
	o.effect, o.open = EffectMetadata, true
	// Z9: the planned record, which starts the operation.
	o.started = o.now()
	o.restorePlan = &backup.RestorePlan{ToolVersion: o.w.ToolVersion, GameID: before.Game.ID, GameName: before.Game.Name,
		StartedAt: o.started, Source: src.receipt.Source, Sources: src.receipt.Sources}
	_, sum, err := o.restorePlan.Encode()
	if err != nil {
		return err
	}
	o.planSum, o.content = sum, o.restorePlan.Sources
	o.restored = backup.SnapshotSource{Operation: before.Snapshot, ReceiptSHA256: src.receiptSum}
	o.snapshotIdentity = src.identity
	return o.record(backup.StepPlanned, backup.StepValues{RestoredSnapshot: o.restored, SnapshotIdentity: o.snapshotIdentity, RestorePlan: o.restorePlan})
}

// sameArea requires the area under lock to be the one the plan read, the
// handed-over copies aside: destination-changed, code 1, otherwise (Z6).
func (o *operation) sameArea(before, again RestorePlan) error {
	changed := func(why string) error {
		return o.target.errorf("destination-changed: %s since the plan; run restore again", why)
	}
	switch {
	case again.State != DestinationArea || again.area == nil:
		return changed("the restore area is no longer marked")
	case len(again.Interventions) > 0:
		first := again.Interventions[0]
		return changed(fmt.Sprintf("%s %s appeared", first.Cause, first.Subject))
	case before.State == DestinationArea && !sameMarker(before.area.marker, again.area.marker):
		return changed("the marker of the area changed")
	case before.State != DestinationArea && (o.areaMarker == nil || !sameMarker(*o.areaMarker, again.area.marker)):
		return changed("the marker of the area is not the one this run validated")
	case before.area.signature() != again.area.signature():
		return changed("the restore area changed")
	}
	return nil
}

func sameMarker(a, b backup.RestoreAreaMarker) bool {
	return a.GameID == b.GameID && a.ToolVersion == b.ToolVersion && a.CreatedAt.Equal(b.CreatedAt)
}

// initializeArea marks a new or empty destination, as steps Z2 to Z4 of
// ADR 0006 §11: the destination created if it is missing, read again while
// the session pins it, then the marker committed by rename.
func (o *operation) initializeArea(before RestorePlan) error {
	// Z2: the destination itself, never adopted if it appeared meanwhile.
	if before.State == DestinationNew {
		if err := o.vault.CreateDir("."); err != nil {
			if errors.Is(err, fs.ErrExist) {
				return o.target.errorf("destination-changed: it appeared since the plan; run restore again")
			}
			return o.target.errorf("%w", err)
		}
		o.wrote()
	}
	// Z3: pinned by the session, read again: it must still be empty.
	if err := o.stillEmpty(); err != nil {
		return err
	}
	marker := backup.RestoreAreaMarker{CreatedAt: o.now(), ToolVersion: o.w.ToolVersion, GameID: before.Game.ID}
	data, err := backup.EncodeRestoreAreaMarker(marker)
	if err != nil {
		return err
	}
	// Z4: validated by a rename that never replaces.
	if err := o.commitAreaMarker(data); err != nil {
		return err
	}
	o.areaMarker = &marker
	return nil
}

// stillEmpty reads the destination again, without following anything: a
// file added since the plan leaves it unmarked, nothing more written.
func (o *operation) stillEmpty() error {
	scope, err := o.s.files.OpenScope(o.target.volume)
	if err != nil {
		return o.target.errorf("open volume %s: %v", o.target.volume, err)
	}
	defer func() { _ = scope.Close() }()
	entries, err := scope.ReadDir(strings.Join(o.target.segments, "/"))
	switch {
	case err != nil:
		return o.target.errorf("cannot be read again before its marker: %v", err)
	case len(entries) > 0:
		return o.target.errorf("destination-changed: it is no longer empty; nothing is marked, run restore again")
	}
	return nil
}

// commitAreaMarker writes the marker of the area to the smallest free
// temporary and renames it, never replacing (ADR 0006 §11, concurrency of
// the marker validation): a temporary another process took is skipped for
// the next one, within envelope E1; a marker another process validated
// first makes this run the loser, code 1, nothing more written.
func (o *operation) commitAreaMarker(data []byte) error {
	for {
		k := 1
		for k <= backup.MaxTemporaries && o.marker[k] {
			k++
		}
		if k > backup.MaxTemporaries {
			o.effect = EffectIntervention
			return &InterventionError{Subject: areaSubject, Cause: backup.ErrEnvelopeExhausted.Error() + ": the marker of the area cannot be validated"}
		}
		o.marker[k] = true
		temporary := fmt.Sprintf("%s.tmp-%d", backup.RestoreAreaMarkerName, k)
		file, err := o.vault.NewFile(temporary, time.Time{})
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return o.target.errorf("marker: %w", err)
		}
		o.wrote()
		_, err = file.Write(data)
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return o.target.errorf("marker: %w", err)
		}
		err = o.move(temporary, backup.RestoreAreaMarkerName)
		switch {
		case errors.Is(err, fs.ErrExist):
			return o.target.errorf("destination-changed: another run marked this area first; its temporary %s is left as an abandoned attempt, run restore again", temporary)
		case err != nil:
			return o.target.errorf("marker: %w", err)
		}
		return nil
	}
}

// copySnapshot creates <op>.incomplete after its intent, copies the
// snapshot into it as the plan describes it, then reads the identity of the
// snapshot again and closes the read scope of the vault (ADR 0006 §11,
// creation step 2). A snapshot changed or replaced is changed-during-restore,
// code 2 (departure X2); a vault gone or unreadable, not verifiable, code 1.
func (o *operation) copySnapshot() error {
	if err := o.record(backup.StepStagingIntent, backup.StepValues{}); err != nil {
		return err
	}
	if err := o.vault.CreateDir(o.stagingDir()); err != nil {
		return fmt.Errorf("incomplete copy: %w", err)
	}
	o.effect = EffectStaged
	scope, err := o.s.files.OpenScope(o.reads.volume)
	if err != nil {
		return o.snapshotUnavailable(err)
	}
	snapshot := o.reads.scopeName(snapshotsDir + "/" + o.restored.Operation)
	copyErr := o.copyContent(scope, snapshot)
	id, idErr := scope.Identity(snapshot)
	// The vault is no longer read after the copy.
	_ = scope.Close()
	if copyErr != nil || idErr != nil {
		if err := o.vaultReadable(); err != nil {
			return o.snapshotUnavailable(err)
		}
	}
	switch {
	case errors.Is(idErr, fs.ErrNotExist), idErr == nil && id != o.snapshotIdentity:
		// Judged before the content: a replaced snapshot is named as such.
		return &BlockedError{Code: CodeChangedDuringRestore, Detail: fmt.Sprintf("snapshot replaced during the copy: %s is no longer the directory %s the plan authenticated",
			o.restored.Operation, o.snapshotIdentity)}
	case idErr != nil:
		return fmt.Errorf("snapshot %s: identity: %w", o.restored.Operation, idErr)
	case copyErr != nil:
		return copyErr
	}
	staging, err := o.vault.Identity(o.stagingDir())
	if err != nil {
		return fmt.Errorf("incomplete copy identity: %w", err)
	}
	digest, err := backup.ContentDigest(o.content)
	if err != nil {
		return err
	}
	o.staging = staging
	return o.record(backup.StepStaged, backup.StepValues{Staging: staging, Digest: digest})
}

// snapshotUnavailable reports a vault that cannot be read during the copy:
// never a change of the snapshot, code 1.
func (o *operation) snapshotUnavailable(err error) error {
	return fmt.Errorf("snapshot %s not verifiable: the vault %s cannot be read now: %w; the copy is kept incomplete", o.restored.Operation, o.reads.path, err)
}

// vaultReadable checks again, without following anything, the root and the
// marker of the vault: a file of the snapshot found absent is a change only
// if the vault itself is still there.
func (o *operation) vaultReadable() error {
	scope, err := o.s.files.OpenScope(o.reads.volume)
	if err != nil {
		return err
	}
	defer func() { _ = scope.Close() }()
	v, err := resolveVault(scope, o.reads.path, o.reads.segments)
	if err != nil {
		return err
	}
	if !v.exists {
		return fmt.Errorf("%s does not exist", o.reads.path)
	}
	data, err := readFile(scope, join(strings.Join(o.reads.segments, "/"), backup.MarkerName), markerReadLimit)
	if err != nil {
		return err
	}
	_, err = backup.DecodeMarker(data)
	return err
}

// publishRestore renames the copy to <dest>\<op> after its intent, then
// commits the receipt and the published record (ADR 0006 §11, creation
// steps 4 and 5). From publish-intent on, nothing is recorded as failed: an
// interrupted switch is left to recovery.
func (o *operation) publishRestore() error {
	published := o.at(o.domain.published(o.op))
	if err := o.record(backup.StepPublishIntent, backup.StepValues{Staging: o.staging}); err != nil {
		return err
	}
	o.critical = true
	moved, err := o.switchTo(o.stagingDir(), published, o.staging)
	var unexplained *unexplainedError
	switch {
	case err == nil:
	case errors.As(err, &unexplained):
		return o.intervene(err.Error())
	case moved:
		return o.intervene("the switch did not end as recorded: " + err.Error())
	default:
		if recordErr := o.record(backup.StepRolledBack, backup.StepValues{}); recordErr != nil {
			return o.intervene(fmt.Sprintf("publication failed (%v) and could not be recorded: %v", err, recordErr))
		}
		o.critical, o.effect = false, EffectRolledBack
		return fmt.Errorf("publication failed, the copy is kept incomplete: %w", err)
	}
	publishedAt := o.now()
	receipt, err := o.writeRestoreReceipt(&publishedAt, nil, "")
	if err != nil {
		return o.intervene("the restore is published but its receipt is missing: " + err.Error() + "; do not use it yet, run restore --recover")
	}
	if err := o.record(backup.StepPublished, backup.StepValues{Digest: receipt}); err != nil {
		return o.intervene("the restore and its receipt are published but the journal is not closed: " + err.Error() + "; run restore --recover")
	}
	o.critical, o.effect = false, EffectPublished
	return nil
}

// writeRestoreReceipt commits receipts\<op>.json from the plan and returns
// its SHA-256. A receipt written at publication has publishedAt; one rebuilt
// by recovery has recoveredAt and recoveredBy (ADR 0009).
func (o *operation) writeRestoreReceipt(publishedAt, recoveredAt *time.Time, recoveredBy string) ([32]byte, error) {
	r, err := o.restorePlan.Receipt(o.op, o.planSum, o.restored, o.intentAt, publishedAt, recoveredAt, recoveredBy)
	if err != nil {
		return [32]byte{}, err
	}
	data, err := backup.EncodeRestoreReceipt(r)
	if err != nil {
		return [32]byte{}, err
	}
	receipts := o.at(backup.RestoreReceiptsDir)
	if err := o.vault.Hold(receipts); err != nil {
		return [32]byte{}, err
	}
	if err := o.commitFile(receipts+"/"+o.op+".json", data, o.receipt, nil); err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(data), nil
}
