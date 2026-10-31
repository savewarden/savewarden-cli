package sync_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/features/sync"
)

const snapOp2 = "20261106T090000-6f7a8b9c"

func snapshotPlanOf(t *testing.T, f *fakeFS) sync.SnapshotPlan {
	t.Helper()
	p, err := service(f, manifest()).SnapshotPlan(context.Background(), sync.SnapshotRequest{ManifestPath: "m.yaml", VaultPath: vaultPath})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func snapshotPlanError(f *fakeFS, vault string) error {
	_, err := service(f, manifest()).SnapshotPlan(context.Background(), sync.SnapshotRequest{ManifestPath: "m.yaml", VaultPath: vault})
	return err
}

func identityOf(n *node) backup.FileID {
	return backup.FileID{Volume: 0x1a2b3c4d, Index: uint64(n.id)} //nolint:gosec // G115: node ids count up from 1.
}

func snapshotCodes(p sync.SnapshotPlan) []string {
	var out []string
	for _, finding := range p.Findings {
		out = append(out, string(finding.Code)+" "+finding.Path)
	}
	return out
}

func snapshotCauses(p sync.SnapshotPlan) []string {
	var out []string
	for _, i := range p.Interventions {
		out = append(out, string(i.Cause)+" "+i.Subject)
	}
	return out
}

// writeSnapshotRecords commits the records of snapshot operation id,
// chained and one second apart from start.
func writeSnapshotRecords(t *testing.T, f *fakeFS, id string, start time.Time, steps []backup.Step, values func(backup.Step) backup.StepValues) {
	t.Helper()
	f.mkdir(gameDir + `\snapshots\receipts`)
	dir := f.mkdir(gameDir + `\snapshots\journal\` + id)
	prev := ""
	for i, step := range steps {
		r, err := backup.NewSnapshotRecord(i+1, step, id, start.Add(time.Duration(i)*time.Second), prev, values(step))
		if err != nil {
			t.Fatalf("record %s: %v", step, err)
		}
		data, err := backup.EncodeRecord(r)
		if err != nil {
			t.Fatal(err)
		}
		dir.children[backup.RecordName(i+1, step)] = f.file(string(data))
		sum := sha256.Sum256(data)
		prev = hex.EncodeToString(sum[:])
	}
}

// publishSnapshot leaves snapshot id of the current generation as the
// protocol would publish it: a copy with its own identity, its receipt and
// a journal terminal in published.
func publishSnapshot(t *testing.T, f *fakeFS, id string, start time.Time) {
	t.Helper()
	game := f.get(gameDir)
	head, receiptData := headReceipt(t, f)
	plan := &backup.SnapshotPlan{ToolVersion: "dev", GameID: head.GameID, GameName: head.GameName, StartedAt: start, Sources: head.Sources}
	_, planSum, err := plan.Encode()
	if err != nil {
		t.Fatal(err)
	}
	copied := copyTree(f, game.children["current"])
	f.mkdir(gameDir + `\snapshots`).children[id] = copied
	content, err := backup.ContentDigest(plan.Sources)
	if err != nil {
		t.Fatal(err)
	}
	source := backup.SnapshotSource{Operation: head.Operation, ReceiptSHA256: sha256.Sum256(receiptData)}
	published := start.Add(5 * time.Second)
	receipt, err := plan.Receipt(id, planSum, source, start.Add(4*time.Second), &published, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	data, err := backup.EncodeSnapshotReceipt(receipt)
	if err != nil {
		t.Fatal(err)
	}
	f.put(gameDir+`\snapshots\receipts\`+id+".json", f.file(string(data)))
	steps := []backup.Step{backup.StepPlanned, backup.StepStagingIntent, backup.StepStaged, backup.StepVerified, backup.StepPublishIntent, backup.StepPublished}
	writeSnapshotRecords(t, f, id, start, steps, func(step backup.Step) backup.StepValues {
		switch step {
		case backup.StepPlanned:
			return backup.StepValues{SnapshotSource: source, SourceIdentity: identityOf(game.children["current"]), SnapshotPlan: plan}
		case backup.StepStaged:
			return backup.StepValues{Staging: identityOf(copied), Digest: content}
		case backup.StepVerified:
			return backup.StepValues{Digest: content}
		case backup.StepPublishIntent:
			return backup.StepValues{Staging: identityOf(copied)}
		case backup.StepPublished:
			return backup.StepValues{Digest: sha256.Sum256(data)}
		}
		return backup.StepValues{}
	})
}

// headReceipt returns the receipt of the current generation and its bytes.
func headReceipt(t *testing.T, f *fakeFS) (backup.Receipt, []byte) {
	t.Helper()
	p := plan(t, f, manifest(), vaultPath)
	data := f.get(gameDir + `\receipts\` + p.Current + ".json").data
	r, err := backup.DecodeReceipt(data)
	if err != nil {
		t.Fatal(err)
	}
	return r, data
}

var snapAt = at.Add(24 * time.Hour)

func TestSnapshotPlanWritesNothing(t *testing.T) {
	f := generation(t)
	before := snapshot(f, `F:\`)
	p := snapshotPlanOf(t, f)
	if p.Outcome() != "new snapshot" || !p.Recognized || p.Current != op1 || p.Files != 2 || p.Bytes != 6 || p.Needed == 0 ||
		len(p.Snapshots) != 0 || len(p.Findings) != 0 {
		t.Fatalf("plan %+v", p)
	}
	if after := snapshot(f, `F:\`); after != before {
		t.Fatal("the snapshot plan changed the vault")
	}
	publishSnapshot(t, f, snapOp, snapAt)
	before = snapshot(f, `F:\`)
	if p := snapshotPlanOf(t, f); p.Outcome() != "up to date" || p.UpToDate != snapOp {
		t.Fatalf("plan %s %q", p.Outcome(), p.UpToDate)
	}
	if after := snapshot(f, `F:\`); after != before {
		t.Fatal("the snapshot plan changed the vault")
	}
}

// S7: one generation gives one snapshot; the plan is then up to date, and
// --apply writes nothing.
func TestSnapshotIsIdempotentForTheSameGeneration(t *testing.T) {
	f := generation(t)
	publishSnapshot(t, f, snapOp, snapAt)
	p := snapshotPlanOf(t, f)
	if p.UpToDate != snapOp || p.Degraded() != 0 || len(p.Snapshots) != 1 || p.Snapshots[0].Source != op1 ||
		p.Snapshots[0].Files != 2 || p.Snapshots[0].Bytes != 6 || p.Files != 0 || p.Needed != 0 {
		t.Fatalf("plan %+v", p)
	}
	before := snapshot(f, `F:\`)
	vaults := newFakeVaults(f)
	if result, err := snapshotApply(f, vaults); err != nil || result.Effect != sync.EffectNone || len(vaults.calls) != 0 || snapshot(f, `F:\`) != before {
		t.Fatalf("--apply: %+v, %v, calls %q", result, err, vaults.calls)
	}
}

// ADR 0008: content is read only for the snapshot that would make the plan
// up to date; the others are judged by receipt and identity.
func TestOnlyTheUpToDateSnapshotIsReadInFull(t *testing.T) {
	f := generation(t)
	publishSnapshot(t, f, snapOp, snapAt)
	if result, err := rotateWith(context.Background(), f, newFakeVaults(f)); err != nil || result.Effect != sync.EffectPublished {
		t.Fatalf("rotation: %+v, %v", result, err)
	}
	publishSnapshot(t, f, snapOp2, snapAt.Add(timeLater))
	var opened []string
	f.onOpen = func(name string, _ *node) { opened = append(opened, name) }
	p := snapshotPlanOf(t, f)
	if p.UpToDate != snapOp2 || len(p.Snapshots) != 2 {
		t.Fatalf("plan: up to date %q, %d snapshots", p.UpToDate, len(p.Snapshots))
	}
	read := func(op string) int {
		count := 0
		for _, name := range opened {
			if strings.Contains(name, "/snapshots/"+op+"/") {
				count++
			}
		}
		return count
	}
	if read(snapOp2) != 3 || read(snapOp) != 0 {
		t.Fatalf("files read: %d of %s, %d of %s; opened %q", read(snapOp2), snapOp2, read(snapOp), snapOp, opened)
	}
}

// ADR 0006 §10: an operation id is never shared between the two domains;
// a snapshot journal named after an operation of current is ambiguous for
// snapshot, never for sync.
func TestSnapshotIDSharedWithCurrentIsAmbiguous(t *testing.T) {
	f := generation(t)
	if result, err := rotateWith(context.Background(), f, newFakeVaults(f)); err != nil || result.Effect != sync.EffectPublished {
		t.Fatalf("rotation: %+v, %v", result, err)
	}
	head, data := headReceipt(t, f)
	writeSnapshotRecords(t, f, op1, snapAt, []backup.Step{backup.StepPlanned, backup.StepAbandoned}, func(step backup.Step) backup.StepValues {
		if step == backup.StepAbandoned {
			return backup.StepValues{Text: "interrupted"}
		}
		return backup.StepValues{SnapshotSource: backup.SnapshotSource{Operation: head.Operation, ReceiptSHA256: sha256.Sum256(data)},
			SourceIdentity: identityOf(f.get(gameDir + `\current`)),
			SnapshotPlan:   &backup.SnapshotPlan{ToolVersion: "dev", GameID: head.GameID, GameName: head.GameName, StartedAt: snapAt, Sources: head.Sources}}
	})
	if want := "journal-ambiguous " + gameRel + "/snapshots/journal/" + op1; !slices.Equal(snapshotCauses(snapshotPlanOf(t, f)), []string{want}) {
		t.Fatalf("causes %q", snapshotCauses(snapshotPlanOf(t, f)))
	}
	if p := plan(t, f, manifest(), vaultPath); len(p.Interventions) != 0 || p.Outcome() != "up to date" {
		t.Fatalf("sync plan: %s %q", p.Outcome(), causes(p))
	}
}

func TestNewSnapshotAfterRotation(t *testing.T) {
	f := generation(t)
	publishSnapshot(t, f, snapOp, snapAt)
	if result, err := rotateWith(context.Background(), f, newFakeVaults(f)); err != nil || result.Effect != sync.EffectPublished {
		t.Fatalf("rotation: %+v, %v", result, err)
	}
	p := snapshotPlanOf(t, f)
	if p.Outcome() != "new snapshot" || p.Current != op3 || p.Files != 3 || len(p.Snapshots) != 1 || p.Snapshots[0].Degraded() {
		t.Fatalf("plan %s %+v", p.Outcome(), p)
	}
}

func TestSnapshotRequiresRecognizedCurrent(t *testing.T) {
	f := generation(t)
	f.get(gameDir + `\current\saves\1.save`).data = []byte("uno")
	p := snapshotPlanOf(t, f)
	if p.Outcome() != "blocked" || p.Recognized || !slices.Contains(snapshotCodes(p), "current-modified 1.save") {
		t.Fatalf("modified current: %s %q", p.Outcome(), snapshotCodes(p))
	}
	f = generation(t)
	copyCurrent(f)
	if p := snapshotPlanOf(t, f); strings.Join(snapshotCauses(p), ",") != "unexpected-entry "+gameRel+"/current" || p.Outcome() != "intervention required" {
		t.Fatalf("copied current: %q", snapshotCauses(p))
	}
}

func TestSnapshotWithoutCurrentIsBlocked(t *testing.T) {
	f := world()
	markVault(t, f, vaultPath)
	f.mkdir(gameDir)
	if p := snapshotPlanOf(t, f); strings.Join(snapshotCodes(p), ",") != "no-current current" || p.Outcome() != "blocked" {
		t.Fatalf("no current: %s %q", p.Outcome(), snapshotCodes(p))
	}
	f = world()
	markVault(t, f, vaultPath)
	if p := snapshotPlanOf(t, f); strings.Join(snapshotCodes(p), ",") != "no-current current" {
		t.Fatalf("no game directory: %q", snapshotCodes(p))
	}
	f = world()
	f.mkdir(`F:\Backups`)
	if err := snapshotPlanError(f, `F:\Backups\Vault`); err == nil || !strings.Contains(err.Error(), "snapshot never does") {
		t.Fatalf("absent vault: %v", err)
	}
	f.mkdir(`F:\Backups\Vault`)
	if err := snapshotPlanError(f, `F:\Backups\Vault`); err == nil || !strings.Contains(err.Error(), "snapshot never does") {
		t.Fatalf("empty vault: %v", err)
	}
}

func TestSnapshotChecksDistinction(t *testing.T) {
	f := world()
	markVault(t, f, installPath+`\Vault`)
	if err := snapshotPlanError(f, installPath+`\Vault`); err == nil || !strings.Contains(err.Error(), "distinct") {
		t.Fatalf("vault in the installation: %v", err)
	}
}

func TestPendingCurrentOperationBlocksSnapshot(t *testing.T) {
	f := generation(t)
	writeJournal(t, f, op2, backup.StepPlanned)
	p := snapshotPlanOf(t, f)
	if !slices.Contains(snapshotCauses(p), "pending-operation "+op2) || p.Outcome() != "intervention required" {
		t.Fatalf("causes %q", snapshotCauses(p))
	}
}

func TestPendingSnapshotOperationBlocksSnapshot(t *testing.T) {
	f := generation(t)
	writeSnapshotRecords(t, f, snapOp, snapAt, []backup.Step{backup.StepPlanned}, func(backup.Step) backup.StepValues {
		head, data := headReceipt(t, f)
		return backup.StepValues{SnapshotSource: backup.SnapshotSource{Operation: head.Operation, ReceiptSHA256: sha256.Sum256(data)},
			SourceIdentity: identityOf(f.get(gameDir + `\current`)),
			SnapshotPlan:   &backup.SnapshotPlan{ToolVersion: "dev", GameID: head.GameID, GameName: head.GameName, StartedAt: snapAt, Sources: head.Sources}}
	})
	p := snapshotPlanOf(t, f)
	if strings.Join(snapshotCauses(p), ",") != "pending-operation "+snapOp || len(p.Pending) != 1 || p.Pending[0].Last != backup.StepPlanned {
		t.Fatalf("causes %q, pending %+v", snapshotCauses(p), p.Pending)
	}
	// The current domain does not see it.
	if causes := causes(plan(t, f, manifest(), vaultPath)); len(causes) != 0 {
		t.Fatalf("sync causes %q", causes)
	}
}

// degradedLines lists what the vault holds of snapshot op: its names, its
// receipt and its journal.
func degradedLines(f *fakeFS, op string) string {
	var lines []string
	for _, line := range strings.Split(snapshot(f, gameDir+`\snapshots`), "\n") {
		if strings.Contains(line, op) {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// ADR 0006 §10 and ADR 0008: whatever is observed of an approved snapshot,
// even a file or a link under its name or its receipt's, degrades it; it
// is named in every outcome, counted by a recovery, never blocking, and
// nothing is written or renamed for it; a new snapshot of the same
// generation is published beside it.
func TestDegradedSnapshotsAreVisible(t *testing.T) {
	tests := []struct {
		name   string
		change func(f *fakeFS)
		want   string
	}{
		{"replaced by a file", func(f *fakeFS) { f.get(gameDir + `\snapshots`).children[snapOp] = f.file("x") }, "snapshot-modified"},
		{"replaced by a link", func(f *fakeFS) { f.get(gameDir + `\snapshots`).children[snapOp] = f.link() }, "snapshot-modified"},
		{"file named as its copy", func(f *fakeFS) { f.get(gameDir + `\snapshots`).children[snapOp+".incomplete"] = f.file("x") }, "snapshot-modified"},
		{"receipt replaced by a directory", func(f *fakeFS) { f.get(gameDir + `\snapshots\receipts`).children[snapOp+".json"] = f.dir() }, "snapshot-modified"},
		{"receipt replaced by a link", func(f *fakeFS) { f.get(gameDir + `\snapshots\receipts`).children[snapOp+".json"] = f.link() }, "snapshot-modified"},
		{"missing", func(f *fakeFS) { delete(f.get(gameDir+`\snapshots`).children, snapOp) }, "snapshot-missing"},
		{"rename lost", func(f *fakeFS) {
			snapshots := f.get(gameDir + `\snapshots`)
			snapshots.children[snapOp+".incomplete"] = snapshots.children[snapOp]
			delete(snapshots.children, snapOp)
		}, "snapshot-rename-lost"},
		{"replaced", func(f *fakeFS) {
			snapshots := f.get(gameDir + `\snapshots`)
			snapshots.children[snapOp] = copyTree(f, snapshots.children[snapOp])
		}, "snapshot-modified"},
		{"receipt altered", func(f *fakeFS) {
			receipt := f.get(gameDir + `\snapshots\receipts\` + snapOp + ".json")
			receipt.data = append(receipt.data, ' ')
		}, "snapshot-modified"},
		{"receipt missing", func(f *fakeFS) { delete(f.get(gameDir+`\snapshots\receipts`).children, snapOp+".json") }, "snapshot-modified"},
		{"content altered", func(f *fakeFS) { f.get(gameDir + `\snapshots\` + snapOp + `\saves\1.save`).data = []byte("uno") }, "snapshot-modified"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := generation(t)
			publishSnapshot(t, f, snapOp, snapAt)
			tt.change(f)
			p := snapshotPlanOf(t, f)
			want := tt.want + " snapshots/" + snapOp
			if p.Outcome() != "new snapshot" || p.Degraded() != 1 || !slices.Equal(snapshotCodes(p), []string{want}) || p.Blocking() != 0 {
				t.Fatalf("plan %s, degraded %d, findings %q; want %q", p.Outcome(), p.Degraded(), snapshotCodes(p), want)
			}
			// Content is read only for the snapshot that would make the plan
			// up to date; recovery judges receipts and identities alone.
			byIdentity := tt.name != "content altered"
			wantRecovery := 0
			if byIdentity {
				wantRecovery = 1
			}
			if rec, err := snapshotRecoverPlan(f); err != nil || rec.Action != sync.RecoverNothing || rec.Degraded() != wantRecovery {
				t.Fatalf("recovery plan: %q, degraded %d, %v", rec.Action, rec.Degraded(), err)
			}
			degraded := degradedLines(f, snapOp)
			result, err := snapshotApply(f, newFakeVaults(f))
			if err != nil || result.Effect != sync.EffectPublished || result.Plan.Degraded() != 1 {
				t.Fatalf("apply: %+v, %v", result, err)
			}
			if after := degradedLines(f, snapOp); after != degraded {
				t.Fatalf("the degraded snapshot changed:\n%s\nwant\n%s", after, degraded)
			}
			if !byIdentity {
				return
			}
			// Beside an intervention, the degradation is still named.
			f.put(gameDir+`\snapshots\notes.txt`, f.file("x"))
			if p := snapshotPlanOf(t, f); p.Outcome() != "intervention required" || p.Degraded() != 1 || !slices.Contains(snapshotCodes(p), want) {
				t.Fatalf("beside an intervention: %s, degraded %d, findings %q", p.Outcome(), p.Degraded(), snapshotCodes(p))
			}
		})
	}
}

// Only the name of an approved snapshot escapes the grammar: a file named
// as the copy of an operation closed otherwise stays unexpected.
func TestNameOfAnUnapprovedSnapshotMustBeADirectory(t *testing.T) {
	f := generation(t)
	head, data := headReceipt(t, f)
	writeSnapshotRecords(t, f, snapOp, snapAt, []backup.Step{backup.StepPlanned, backup.StepAbandoned}, func(step backup.Step) backup.StepValues {
		if step == backup.StepAbandoned {
			return backup.StepValues{Text: "interrupted"}
		}
		return backup.StepValues{SnapshotSource: backup.SnapshotSource{Operation: head.Operation, ReceiptSHA256: sha256.Sum256(data)},
			SourceIdentity: identityOf(f.get(gameDir + `\current`)),
			SnapshotPlan:   &backup.SnapshotPlan{ToolVersion: "dev", GameID: head.GameID, GameName: head.GameName, StartedAt: snapAt, Sources: head.Sources}}
	})
	f.put(gameDir+`\snapshots\`+snapOp+".incomplete", f.file("x"))
	p := snapshotPlanOf(t, f)
	if want := "unexpected-entry " + gameRel + "/snapshots/" + snapOp + ".incomplete"; !slices.Equal(snapshotCauses(p), []string{want}) || p.Incomplete != 0 {
		t.Fatalf("causes %q, incomplete %d", snapshotCauses(p), p.Incomplete)
	}
}

// ADR 0006 §10, structure: receipts is created before journal; operations
// without it are a partial state, code 3, for the plan as for recovery;
// an empty structure is completed.
func TestPartialSnapshotStructureIsAmbiguous(t *testing.T) {
	f := generation(t)
	publishSnapshot(t, f, snapOp, snapAt)
	delete(f.get(gameDir+`\snapshots`).children, "receipts")
	want := "journal-ambiguous " + gameRel + "/snapshots/receipts"
	if p := snapshotPlanOf(t, f); !slices.Equal(snapshotCauses(p), []string{want}) {
		t.Fatalf("plan causes %q", snapshotCauses(p))
	}
	f = interruptedAt(t, snapCommit(5, "publish-intent"), true)
	delete(f.get(gameDir+`\snapshots`).children, "receipts")
	if _, err := snapshotRecoverPlan(f); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("recovery plan: %v", err)
	}
	f = generation(t)
	f.mkdir(gameDir + `\snapshots\journal`)
	if result, err := snapshotApply(f, newFakeVaults(f)); err != nil || result.Effect != sync.EffectPublished || f.get(gameDir+`\snapshots\receipts`) == nil {
		t.Fatalf("empty structure: %+v, %v", result, err)
	}
}

// ADR 0008: a pending operation whose lock another process holds is
// running: code 1, never an intervention, for the plan as for recovery.
func TestRunningSnapshotIsNotInterrupted(t *testing.T) {
	f := interruptedAt(t, snapCommit(2, "staging-intent"), true)
	f.get(gameDir + `\sync.lock`).busy = true
	// What the running operation writes is not judged: a receipt it would
	// be committing does not make the domain ambiguous.
	f.put(gameDir+`\snapshots\receipts\`+snapApplied+".json", f.file("{}"))
	for name, run := range map[string]func() error{
		"plan":          func() error { return snapshotPlanError(f, vaultPath) },
		"recovery plan": func() error { _, err := snapshotRecoverPlan(f); return err },
	} {
		var intervention *sync.InterventionError
		if err := run(); err == nil || errors.As(err, &intervention) || !strings.Contains(err.Error(), "another operation is running on this game") {
			t.Fatalf("%s: %v", name, err)
		}
	}
	f.get(gameDir + `\sync.lock`).busy = false
	delete(f.get(gameDir+`\snapshots\receipts`).children, snapApplied+".json")
	if p := snapshotPlanOf(t, f); !slices.Equal(snapshotCauses(p), []string{"pending-operation " + snapApplied}) {
		t.Fatalf("interrupted: %q", snapshotCauses(p))
	}
}

func TestIncompleteBeforeStagedIsExplainedByName(t *testing.T) {
	f := generation(t)
	head, data := headReceipt(t, f)
	writeSnapshotRecords(t, f, snapOp, snapAt, []backup.Step{backup.StepPlanned, backup.StepStagingIntent, backup.StepAbandoned},
		func(step backup.Step) backup.StepValues {
			switch step {
			case backup.StepPlanned:
				return backup.StepValues{SnapshotSource: backup.SnapshotSource{Operation: head.Operation, ReceiptSHA256: sha256.Sum256(data)},
					SourceIdentity: identityOf(f.get(gameDir + `\current`)),
					SnapshotPlan:   &backup.SnapshotPlan{ToolVersion: "dev", GameID: head.GameID, GameName: head.GameName, StartedAt: snapAt, Sources: head.Sources}}
			case backup.StepAbandoned:
				return backup.StepValues{Text: "interrupted"}
			}
			return backup.StepValues{}
		})
	f.put(gameDir+`\snapshots\`+snapOp+`.incomplete\saves\1.save`, f.file("one"))
	p := snapshotPlanOf(t, f)
	if len(p.Interventions) != 0 || p.Incomplete != 1 || p.Outcome() != "new snapshot" {
		t.Fatalf("plan %s, incomplete %d, causes %q", p.Outcome(), p.Incomplete, snapshotCauses(p))
	}
}

func TestSnapshotDomainAnomaliesAreInterventions(t *testing.T) {
	tests := []struct {
		name  string
		plant func(f *fakeFS)
		want  string
	}{
		{"stray file", func(f *fakeFS) { f.put(gameDir+`\snapshots\notes.txt`, f.file("x")) }, "unexpected-entry " + gameRel + "/snapshots/notes.txt"},
		{"unexplained snapshot", func(f *fakeFS) { f.mkdir(gameDir + `\snapshots\` + snapOp2) }, "unexpected-entry " + gameRel + "/snapshots/" + snapOp2},
		{"orphan receipt", func(f *fakeFS) { f.put(gameDir+`\snapshots\receipts\`+snapOp2+".json", f.file("{}")) },
			"journal-ambiguous " + gameRel + "/snapshots/receipts/" + snapOp2 + ".json"},
		{"corrupt journal", func(f *fakeFS) {
			f.put(gameDir+`\snapshots\journal\`+snapOp2+`\001-planned.rec`, f.file("not a record"))
		}, "journal-ambiguous " + gameRel + "/snapshots/journal/" + snapOp2},
		{"records of another operation", func(f *fakeFS) {
			journal := f.get(gameDir + `\snapshots\journal`)
			journal.children[op1] = journal.children[snapOp]
		}, "journal-ambiguous " + gameRel + "/snapshots/journal/" + op1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := generation(t)
			publishSnapshot(t, f, snapOp, snapAt)
			tt.plant(f)
			p := snapshotPlanOf(t, f)
			if !slices.Contains(snapshotCauses(p), tt.want) || p.Outcome() != "intervention required" {
				t.Fatalf("causes %q, want %q", snapshotCauses(p), tt.want)
			}
		})
	}
}

func TestSnapshotSpaceIsChecked(t *testing.T) {
	f := generation(t)
	f.volumeOf[`F:\`] = backup.Volume{FileSystem: "NTFS", Drive: backup.DriveFixed, Available: 1 << 20}
	p := snapshotPlanOf(t, f)
	if p.Outcome() != "blocked" || !slices.Contains(snapshotCodes(p), "insufficient-space "+vaultPath) {
		t.Fatalf("plan %s %q", p.Outcome(), snapshotCodes(p))
	}
}

func TestSnapshotLongPathIsReported(t *testing.T) {
	f := world()
	long := strings.Repeat("a", 210) + ".save"
	f.put(installPath+`\saves\`+long, f.file("long"))
	if r := apply(f, vaultPath); r.err != nil {
		t.Fatal(r.err)
	}
	p := snapshotPlanOf(t, f)
	if !slices.Contains(snapshotCodes(p), "long-path "+long) || p.Blocking() != 0 {
		t.Fatalf("findings %q", snapshotCodes(p))
	}
}
