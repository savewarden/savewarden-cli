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
)

// snapshotDomain holds the snapshots of a game and their operations (ADR
// 0006 §10).
var snapshotDomain = domain{
	command:   "snapshot",
	journals:  snapshotsDir + "/" + journalDir,
	stagings:  snapshotsDir,
	staging:   backup.SnapshotStagingName,
	published: backup.SnapshotName,
	newRecord: backup.NewSnapshotRecord,
	changed:   CodeCurrentModified,
	busy:      "another operation is running on this game",
}

// CauseSourceUnverified: the receipt of the generation a snapshot copies
// is unavailable or contradicts the plan at the provenance gate (ADR 0006
// §10). Nothing is approved on its strength.
const CauseSourceUnverified Cause = "source-unverified"

// SnapshotResult reports a snapshot --apply or --recover run, complete or
// not. Its State says what the vault now holds (ADR 0008).
type SnapshotResult struct {
	Plan      SnapshotPlan
	Operation string
	Effect    Effect
	// Cause explains EffectIntervention, or a run closed in code 3.
	Cause string
	// Snapshot and Receipt are the Windows paths of a published snapshot;
	// Files and Bytes count what was verified.
	Snapshot string
	Receipt  string
	Files    int
	Bytes    int64
	// Recovery is what --recover found and did, nil for --apply.
	Recovery *SnapshotRecovery
}

// State is the vault state of the run, as "savewarden: vault: <state>"
// announces it.
func (r SnapshotResult) State() string {
	switch r.Effect {
	case EffectMetadata:
		return "metadata only; nothing staged or published"
	case EffectStaged:
		return "snapshot " + r.Operation + " kept incomplete"
	case EffectPublished:
		return "published snapshot " + r.Operation
	case EffectRolledBack:
		return "rolled back snapshot " + r.Operation
	case EffectIntervention:
		subject := r.Operation
		if subject == "" {
			subject = "the vault"
		}
		return "intervention required for " + subject + ": " + r.Cause
	}
	return "not modified"
}

// SnapshotApply plans, then copies the recognised current into
// snapshots\<op>.incomplete, verifies the copy and its provenance, and
// publishes it as snapshots\<op> with its receipt (ADR 0006 §10). Its
// Result says what the vault now holds even when it fails.
func (s *Service) SnapshotApply(ctx context.Context, request SnapshotRequest, w Writer) (SnapshotResult, error) {
	plan, err := s.SnapshotPlan(ctx, request)
	if err != nil {
		return SnapshotResult{}, err
	}
	result := SnapshotResult{Plan: plan}
	switch {
	case len(plan.Interventions) > 0:
		first := plan.Interventions[0]
		result.Effect, result.Cause = EffectIntervention, string(first.Cause)+" "+first.Subject
		return result, &InterventionError{Cause: result.Cause}
	case plan.Blocking() > 0:
		return result, &BlockedError{Detail: fmt.Sprintf("%d blocking finding(s)", plan.Blocking())}
	case plan.UpToDate != "":
		return result, nil
	}
	o, err := s.begin(ctx, Plan{Vault: plan.Vault, Game: plan.Game, vaultVolume: plan.vaultVolume, vaultSegments: plan.vaultSegments}, w)
	if err != nil {
		return result, err
	}
	o.domain = snapshotDomain
	err = o.unfinished(o.snapshot(request, plan))
	if closeErr := o.vault.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	result.Operation, result.Effect = o.op, o.effect
	if result.Effect == EffectIntervention && !o.open {
		// Refused before its directory existed: the vault is the subject.
		result.Operation = ""
	}
	var intervention *InterventionError
	if errors.As(err, &intervention) {
		result.Cause = intervention.Cause
	}
	if o.effect == EffectPublished {
		game := append(append([]string{}, plan.vaultSegments...), gamesDir, plan.Game.ID)
		result.Snapshot = windowsPath(plan.vaultVolume, append(append([]string{}, game...), snapshotsDir, o.op))
		result.Receipt = windowsPath(plan.vaultVolume, append(append([]string{}, game...), snapshotsDir, receiptsDir, o.op+".json"))
		for _, source := range o.content {
			result.Files += len(source.Entries)
			result.Bytes += source.Bytes()
		}
	}
	return result, err
}

// snapshot performs the operation; the caller closes the session.
func (o *operation) snapshot(request SnapshotRequest, plan SnapshotPlan) error {
	if err := o.snapshotUnderLock(request, plan); err != nil {
		return err
	}
	for _, phase := range []func() error{o.copyCurrent, o.verifyIncomplete, o.provenanceBeforePublish} {
		if err := phase(); err != nil {
			return o.fail(err)
		}
	}
	return o.publishSnapshot()
}

// snapshotUnderLock takes the lock, plans again under it, then creates the
// structure, the operation directory and the planned record (ADR 0006
// §10, creation step 1).
func (o *operation) snapshotUnderLock(request SnapshotRequest, before SnapshotPlan) error {
	for _, dir := range []string{gamesDir, o.game} {
		if err := o.vault.Hold(dir); err != nil {
			return errVault(o.plan.Vault, "%w", err)
		}
	}
	// The lock is taken as sync takes it: created empty if it is missing.
	if err := o.vault.Lock(o.game + "/" + lockFile); err != nil {
		var busy interface{ Busy() bool }
		if errors.As(err, &busy) && busy.Busy() {
			return errVault(o.plan.Vault, "%s", o.domain.busy)
		}
		return errVault(o.plan.Vault, "lock: %w", err)
	}
	if !before.observed.lock {
		o.wrote()
	}
	// Under lock, current is read again in full against its receipt, and
	// the space checked again, before any journal.
	again, err := o.s.snapshotPlan(o.ctx, request, false)
	if err != nil {
		return err
	}
	if len(again.Interventions) > 0 || again.UpToDate != "" || !sameSnapshotPlan(before, again) {
		return errVault(o.plan.Vault, "vault-changed: the vault changed since the plan; run snapshot again")
	}
	for _, finding := range again.Findings {
		if finding.Severity == backup.Blocking {
			return &BlockedError{Code: finding.Code, Detail: finding.Detail + " (under lock)"}
		}
	}
	if again.observed.journals[o.op].Op != "" || again.observed.receiptSum[o.op] != ([32]byte{}) ||
		again.domain.journals[o.op].Op != "" || again.domain.receipts[o.op] != nil ||
		again.domain.present[o.op] || again.domain.present[o.op+".incomplete"] {
		return errVault(o.plan.Vault, "operation id %s is already used in this vault; run snapshot again", o.op)
	}
	// The structure, made of exclusive creations, after the space check.
	for _, dir := range []string{snapshotsDir, snapshotsDir + "/" + receiptsDir, snapshotsDir + "/" + journalDir} {
		if err := o.ensureDir(o.game + "/" + dir); err != nil {
			return errVault(o.plan.Vault, "%w", err)
		}
	}
	// Only the ancestors of the journal stay pinned across the switch
	// (ADR 0006 §10, creation step 5); receipts is held again to be written.
	if err := o.vault.Release(o.game + "/" + snapshotsDir + "/" + receiptsDir); err != nil {
		return errVault(o.plan.Vault, "%w", err)
	}
	if err := o.vault.CreateDir(o.journalDir()); err != nil {
		return errVault(o.plan.Vault, "operation directory: %w", err)
	}
	// Metadata only until a copy exists to be kept (ADR 0008).
	o.effect, o.open = EffectMetadata, true
	head := again.head
	o.started = o.now()
	o.snapshotPlan = &backup.SnapshotPlan{ToolVersion: o.w.ToolVersion, GameID: head.GameID, GameName: head.GameName,
		StartedAt: o.started, Sources: head.Sources}
	_, sum, err := o.snapshotPlan.Encode()
	if err != nil {
		return err
	}
	o.planSum, o.content = sum, o.snapshotPlan.Sources
	o.source = backup.SnapshotSource{Operation: head.Operation, ReceiptSHA256: again.observed.receiptSum[head.Operation]}
	o.sourceIdentity = again.observed.currentID
	return o.record(backup.StepPlanned, backup.StepValues{SnapshotSource: o.source, SourceIdentity: o.sourceIdentity, SnapshotPlan: o.snapshotPlan})
}

// sameSnapshotPlan compares what a snapshot decides on: the generation,
// its receipt, the approved snapshots and the incomplete copies.
func sameSnapshotPlan(a, b SnapshotPlan) bool {
	if a.Current != b.Current || (a.head == nil) != (b.head == nil) || a.Incomplete != b.Incomplete || len(a.Snapshots) != len(b.Snapshots) {
		return false
	}
	if a.head != nil && a.observed.receiptSum[a.head.Operation] != b.observed.receiptSum[b.head.Operation] {
		return false
	}
	for i := range a.Snapshots {
		if a.Snapshots[i] != b.Snapshots[i] {
			return false
		}
	}
	return true
}

// copyCurrent reads current again, creates snapshots\<op>.incomplete after
// its intent, copies current into it as its receipt describes it, and
// records staged. The identity of current is read again after the copy,
// and judged before its content (ADR 0006 §10, creation step 2).
func (o *operation) copyCurrent() error {
	if err := o.currentUnchanged(); err != nil {
		return err
	}
	if err := o.record(backup.StepStagingIntent, backup.StepValues{}); err != nil {
		return err
	}
	if err := o.vault.CreateDir(o.stagingDir()); err != nil {
		return fmt.Errorf("incomplete copy: %w", err)
	}
	o.effect = EffectStaged
	scope, err := o.s.files.OpenScope(o.plan.vaultVolume)
	if err != nil {
		return fmt.Errorf("open volume %s: %w", o.plan.vaultVolume, err)
	}
	defer func() { _ = scope.Close() }()
	current := strings.Join(append(append([]string{}, o.plan.vaultSegments...), gamesDir, o.plan.Game.ID, currentDir), "/")
	copyErr := o.copyContent(scope, current)
	// A swapped current is code 3, never taken for a change of content.
	if id, err := scope.Identity(current); err != nil || id != o.sourceIdentity {
		return o.intervene(fmt.Sprintf("current was replaced during the copy, or cannot be checked: %v %v; nothing recorded", id, err))
	}
	if copyErr != nil {
		return copyErr
	}
	id, err := o.vault.Identity(o.stagingDir())
	if err != nil {
		return fmt.Errorf("incomplete copy identity: %w", err)
	}
	digest, err := backup.ContentDigest(o.content)
	if err != nil {
		return err
	}
	o.staging = id
	return o.record(backup.StepStaged, backup.StepValues{Staging: id, Digest: digest})
}

// copyContent copies every source of the plan from current, the directory
// current names in scope, into the incomplete copy.
func (o *operation) copyContent(scope Scope, current string) error {
	for _, source := range o.content {
		if source.State != backup.SourceCopied {
			continue
		}
		target := o.stagingDir() + "/" + source.ID
		if err := o.vault.CreateDir(target); err != nil {
			return fmt.Errorf("incomplete copy: %w", err)
		}
		for _, dir := range source.Directories {
			if err := o.vault.CreateDir(target + "/" + dir); err != nil {
				return fmt.Errorf("incomplete copy: %w", err)
			}
		}
		for _, entry := range source.Entries {
			if err := o.ctx.Err(); err != nil {
				return fmt.Errorf("interrupted: %w", err)
			}
			if err := o.copyFile(scope, source.ID, current+"/"+source.ID+"/"+entry.Path, target+"/"+entry.Path, entry); err != nil {
				return err
			}
		}
	}
	return nil
}

// currentUnchanged reads current again in full against the plan, before
// staging-intent; its identity, read after the content, is judged first:
// a swapped current is code 3 with nothing recorded, a change of content
// current-modified, code 2 (ADR 0006 §10, creation step 2).
func (o *operation) currentUnchanged() error {
	segments := append(append([]string{}, o.plan.vaultSegments...), gamesDir, o.plan.Game.ID, currentDir)
	dir, err := o.s.inventories.Directory(o.ctx, o.plan.vaultVolume, segments)
	if err != nil {
		return err
	}
	if id, err := o.vault.Identity(o.game + "/" + currentDir); err != nil || id != o.sourceIdentity {
		return o.intervene(fmt.Sprintf("current was replaced while it was read, or cannot be checked: %v %v; nothing recorded", id, err))
	}
	if differences := compareWithReceipt(dir, backup.Receipt{Sources: o.content}); len(differences) > 0 {
		first := differences[0]
		return &BlockedError{Code: o.domain.changed, Detail: fmt.Sprintf("current changed since the plan: %s %s, %s; %d difference(s) in all",
			orDash(first.source), orDash(first.path), first.detail, len(differences))}
	}
	return nil
}

// verifyIncomplete reads the copy back from disk through a no-follow walk
// and compares it with the plan; its identity is read again after the
// content (ADR 0006 §10, creation step 3).
func (o *operation) verifyIncomplete() error {
	segments := append(append([]string{}, o.plan.vaultSegments...), strings.Split(o.stagingDir(), "/")...)
	dir, err := o.s.inventories.Directory(o.ctx, o.plan.vaultVolume, segments)
	if err != nil {
		return err
	}
	// The identity, read after the content, is judged first.
	if id, err := o.vault.Identity(o.stagingDir()); err != nil || id != o.staging {
		return o.intervene(fmt.Sprintf("the incomplete copy was replaced while it was read, or cannot be checked: %v %v; nothing recorded", id, err))
	}
	if differences := compareWithReceipt(dir, backup.Receipt{Sources: o.content}); len(differences) > 0 {
		first := differences[0]
		return &BlockedError{Code: CodeVerificationMismatch,
			Detail: fmt.Sprintf("incomplete copy %s %s: %s; %d difference(s) in all", orDash(first.source), orDash(first.path),
				strings.ReplaceAll(strings.ReplaceAll(first.detail, "its receipt", "the plan"), "current", "the copy"), len(differences))}
	}
	digest, err := backup.ContentDigest(o.content)
	if err != nil {
		return err
	}
	return o.record(backup.StepVerified, backup.StepValues{Digest: digest})
}

// provenanceBeforePublish is the provenance gate before publish-intent: a
// closed gate closes the operation in failed, nothing being published,
// and exits in code 3 (ADR 0006 §10).
func (o *operation) provenanceBeforePublish() error {
	if _, err := o.checkProvenance(); err != nil {
		cause := string(CauseSourceUnverified) + ": " + err.Error()
		if recordErr := o.record(backup.StepFailed, backup.StepValues{Text: oneLine(cause)}); recordErr != nil {
			return errors.Join(errors.New(cause), fmt.Errorf("the failure could not be recorded, operation %s needs recovery: %w", o.op, recordErr))
		}
		return &InterventionError{Operation: o.op, Cause: oneLine(cause) + "; nothing published"}
	}
	return nil
}

// publishSnapshot renames the copy to snapshots\<op> after its intent,
// passes the provenance gate again, then commits the receipt and the
// published record. From publish-intent on, nothing is recorded as
// failed: an interrupted switch is left to recovery.
func (o *operation) publishSnapshot() error {
	published := o.game + "/" + o.domain.published(o.op)
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
	if err := o.snapshotProvenance(); err != nil {
		return err
	}
	publishedAt := o.now()
	receipt, err := o.writeSnapshotReceipt(&publishedAt, nil, "")
	if err != nil {
		return o.intervene("the snapshot is published but its receipt is missing: " + err.Error() + "; run snapshot --recover")
	}
	if err := o.record(backup.StepPublished, backup.StepValues{Digest: receipt}); err != nil {
		return o.intervene("the snapshot and its receipt are published but the journal is not closed: " + err.Error() + "; run snapshot --recover")
	}
	o.critical, o.effect = false, EffectPublished
	return nil
}

// snapshotProvenance is the provenance gate after the switch, before any
// snapshot receipt: contradictory evidence rolls the switch back, as
// nothing is approved yet; unavailable evidence writes nothing and leaves
// the operation to a later run (ADR 0006 §10).
func (o *operation) snapshotProvenance() error {
	contradictory, err := o.checkProvenance()
	switch {
	case err == nil:
		return nil
	case !contradictory:
		return o.intervene(string(CauseSourceUnverified) + ": " + oneLine(err.Error()) + "; nothing written, run snapshot --recover once it can be read")
	}
	cause := string(CauseSourceUnverified) + ": " + oneLine(err.Error())
	if err := o.record(backup.StepUnpublishIntent, backup.StepValues{Staging: o.staging}); err != nil {
		return o.intervene(cause + "; the rollback could not be recorded: " + err.Error())
	}
	if err := o.unpublish(false); err != nil {
		return err
	}
	return &InterventionError{Operation: o.op, Cause: cause + "; rolled back, the copy is kept incomplete"}
}

// checkProvenance reads, without following anything, the receipt of the
// generation the snapshot copies and checks it against the plan (ADR 0006
// §10). contradictory tells contradictory evidence from evidence that
// cannot be read now.
func (o *operation) checkProvenance() (contradictory bool, err error) {
	scope, err := o.s.files.OpenScope(o.plan.vaultVolume)
	if err != nil {
		return false, fmt.Errorf("open volume %s: %w", o.plan.vaultVolume, err)
	}
	defer func() { _ = scope.Close() }()
	receipts := strings.Join(append(append([]string{}, o.plan.vaultSegments...), gamesDir, o.plan.Game.ID, receiptsDir), "/")
	name := receipts + "/" + o.source.Operation + ".json"
	for _, path := range []string{receipts, name} {
		info, err := scope.Lstat(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return true, fmt.Errorf("%s is missing", path)
		case err != nil:
			return false, err
		case isLink(info.Mode()):
			return true, fmt.Errorf("%s is a link, junction or reparse point", path)
		case path == receipts && !info.IsDir(), path == name && !info.Mode().IsRegular():
			return true, fmt.Errorf("%s is not of its kind", path)
		}
	}
	data, err := readFile(scope, name, recordReadLimit)
	var tooLarge *tooLargeError
	var notFollowed interface{ NotFollowed() bool }
	switch {
	case err == nil:
	case errors.As(err, &tooLarge), errors.Is(err, fs.ErrNotExist), errors.As(err, &notFollowed) && notFollowed.NotFollowed():
		// Larger than the plan could have read, gone or a link since its
		// Lstat: evidence against the plan, not evidence unavailable.
		return true, err
	default:
		if info, lstatErr := scope.Lstat(name); errors.Is(lstatErr, fs.ErrNotExist) || (lstatErr == nil && (isLink(info.Mode()) || !info.Mode().IsRegular())) {
			return true, fmt.Errorf("%s changed while it was opened: %w", name, err)
		}
		return false, err
	}
	j := backup.Journal{Op: o.op, Snapshot: true, SnapshotPlan: o.snapshotPlan, SnapshotSource: o.source}
	if err := backup.CheckSourceReceipt(data, o.plan.Game.ID, j); err != nil {
		return true, err
	}
	return false, nil
}

// writeSnapshotReceipt commits snapshots\receipts\<op>.json from the plan
// and returns its SHA-256. A receipt written at publication has
// publishedAt; one rebuilt by recovery has recoveredAt and recoveredBy.
func (o *operation) writeSnapshotReceipt(publishedAt, recoveredAt *time.Time, recoveredBy string) ([32]byte, error) {
	r, err := o.snapshotPlan.Receipt(o.op, o.planSum, o.source, o.intentAt, publishedAt, recoveredAt, recoveredBy)
	if err != nil {
		return [32]byte{}, err
	}
	data, err := backup.EncodeSnapshotReceipt(r)
	if err != nil {
		return [32]byte{}, err
	}
	receipts := o.game + "/" + snapshotsDir + "/" + receiptsDir
	if err := o.vault.Hold(receipts); err != nil {
		return [32]byte{}, err
	}
	if err := o.commitFile(receipts+"/"+o.op+".json", data, o.receipt, nil); err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(data), nil
}
