package sync_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/features/sync"
)

// snapApplied is the operation of a snapshot taken at snapAt by
// snapWriter.
const snapApplied = "20261106T101500-5e6f7a8b"

const snapRel = gameRel + "/snapshots"

func sha(data []byte) [32]byte { return sha256.Sum256(data) }

func snapWriter(vaults sync.Vaults, random string) sync.Writer {
	return sync.Writer{Vaults: vaults, Clock: &fakeClock{now: snapAt}, Random: strings.NewReader(random), ToolVersion: "dev"}
}

func snapshotApply(f *fakeFS, vaults sync.Vaults) (sync.SnapshotResult, error) {
	return service(f, manifest()).SnapshotApply(context.Background(), sync.SnapshotRequest{ManifestPath: "m.yaml", VaultPath: vaultPath},
		snapWriter(vaults, "\x5e\x6f\x7a\x8b"))
}

// currentDomain lists everything of the game directory outside the
// snapshot domain.
func currentDomain(f *fakeFS) string {
	var lines []string
	for _, line := range strings.Split(snapshot(f, gameDir), "\n") {
		if !strings.HasPrefix(line, gameDir+`\snapshots`) {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func snapshotJournal(f *fakeFS, op string) string {
	journal := f.get(gameDir + `\snapshots\journal\` + op)
	if journal == nil {
		return ""
	}
	return strings.Join(names(journal), ",")
}

func TestSnapshotCopiesCurrentExactly(t *testing.T) {
	f := generation(t)
	head, headData := headReceipt(t, f)
	before := currentDomain(f)
	result, err := snapshotApply(f, newFakeVaults(f))
	if err != nil || result.Effect != sync.EffectPublished || result.Operation != snapApplied || result.Files != 2 || result.Bytes != 6 {
		t.Fatalf("snapshot: %+v, %v", result, err)
	}
	if result.State() != "published snapshot "+snapApplied || !strings.HasSuffix(result.Snapshot, `\snapshots\`+snapApplied) {
		t.Fatalf("state %q, snapshot %q", result.State(), result.Snapshot)
	}
	current, published := f.get(gameDir+`\current`), f.get(gameDir+`\snapshots\`+snapApplied)
	for _, path := range []string{`saves\1.save`, `saves\Slot 1\2.save`} {
		a, b := f.get(gameDir+`\current\`+path), f.get(gameDir+`\snapshots\`+snapApplied+`\`+path)
		if b == nil || !bytes.Equal(a.data, b.data) || !a.modTime.Equal(b.modTime) || a.id == b.id {
			t.Fatalf("%s: not an independent copy", path)
		}
	}
	if current.id == published.id || f.get(gameDir+`\snapshots\`+snapApplied+".incomplete") != nil {
		t.Fatal("the snapshot must be a renamed copy, the incomplete copy gone")
	}
	data := f.get(gameDir + `\snapshots\receipts\` + snapApplied + ".json").data
	r, err := backup.DecodeSnapshotReceipt(data)
	if err != nil || r.Snapshot != snapApplied || r.Source.Operation != op1 || r.Source.ReceiptSHA256 != sha(headData) ||
		!sameContent(t, r.Sources, head.Sources) || r.PublishedAt == nil {
		t.Fatalf("receipt %+v, %v", r, err)
	}
	if got := snapshotJournal(f, snapApplied); got != "001-planned.rec,002-staging-intent.rec,003-staged.rec,004-verified.rec,005-publish-intent.rec,006-published.rec" {
		t.Fatalf("journal %s", got)
	}
	if currentDomain(f) != before {
		t.Fatal("the current domain changed")
	}
	if p := snapshotPlanOf(t, f); p.UpToDate != snapApplied {
		t.Fatalf("plan after: %s %q", p.Outcome(), p.UpToDate)
	}
}

func sameContent(t *testing.T, a, b []backup.ReceiptSource) bool {
	t.Helper()
	da, err := backup.ContentDigest(a)
	if err != nil {
		t.Fatal(err)
	}
	db, err := backup.ContentDigest(b)
	if err != nil {
		t.Fatal(err)
	}
	return da == db
}

func TestSnapshotWritesOnlyUnderSnapshots(t *testing.T) {
	f := generation(t)
	vaults := newFakeVaults(f)
	if _, err := snapshotApply(f, vaults); err != nil {
		t.Fatal(err)
	}
	for _, call := range vaults.calls {
		_, target, _ := strings.Cut(call, " ")
		if !strings.HasPrefix(target, snapRel+"/") && target != snapRel && call != "Lock "+gameRel+"/sync.lock" {
			t.Errorf("call outside the snapshot domain: %s", call)
		}
	}
	// A plan writes nothing, an up-to-date --apply neither.
	before := snapshot(f, `F:\`)
	result, err := snapshotApply(f, newFakeVaults(f))
	if err != nil || result.Effect != sync.EffectNone || result.State() != "not modified" || snapshot(f, `F:\`) != before {
		t.Fatalf("second --apply: %+v, %v", result, err)
	}
}

func TestNewSnapshotAfterRotationIsPublished(t *testing.T) {
	f := generation(t)
	if _, err := snapshotApply(f, newFakeVaults(f)); err != nil {
		t.Fatal(err)
	}
	if _, err := rotateWith(context.Background(), f, newFakeVaults(f)); err != nil {
		t.Fatal(err)
	}
	result, err := service(f, manifest()).SnapshotApply(context.Background(), sync.SnapshotRequest{ManifestPath: "m.yaml", VaultPath: vaultPath},
		snapWriter(newFakeVaults(f), "\x6f\x7a\x8b\x9c"))
	if err != nil || result.Effect != sync.EffectPublished {
		t.Fatalf("second snapshot: %+v, %v", result, err)
	}
	p := snapshotPlanOf(t, f)
	if len(p.Snapshots) != 2 || p.Degraded() != 0 || p.UpToDate != result.Operation || p.Snapshots[0].Source != op1 || p.Snapshots[1].Source != op3 {
		t.Fatalf("plan %+v", p)
	}
}

func TestSnapshotTakesTheLockLikeSync(t *testing.T) {
	f := generation(t)
	delete(f.get(gameDir).children, "sync.lock")
	vaults := newFakeVaults(f)
	result, err := snapshotApply(f, vaults)
	lock := f.get(gameDir + `\sync.lock`)
	if err != nil || result.Effect != sync.EffectPublished || lock == nil || len(lock.data) != 0 || !slices.Contains(vaults.calls, "Lock "+gameRel+"/sync.lock") {
		t.Fatalf("snapshot without lock file: %+v, %v", result, err)
	}
}

// duringCopy runs change at the third opening of current/saves/1.save.
func duringCopy(f *fakeFS, change func(*node)) func(string, *node) {
	opened := 0
	return func(name string, n *node) {
		if strings.Contains(name, "/current/saves/1.save") {
			if opened++; opened == 3 {
				change(n)
				f.onOpen = nil
			}
		}
	}
}

func TestSnapshotOfCurrentChangedDuringCopy(t *testing.T) {
	f := generation(t)
	// The plan and the plan under lock read current first: the third
	// opening of the file is the copy.
	f.onOpen = duringCopy(f, func(n *node) { n.data = []byte("uno") })
	result, err := snapshotApply(f, newFakeVaults(f))
	var blocked *sync.BlockedError
	if !errors.As(err, &blocked) || blocked.Code != sync.CodeCurrentModified || result.Effect != sync.EffectStaged ||
		!strings.HasSuffix(snapshotJournal(f, snapApplied), "003-failed.rec") || f.get(gameDir+`\snapshots\`+snapApplied) != nil ||
		f.get(gameDir+`\snapshots\`+snapApplied+".incomplete") == nil {
		t.Fatalf("content changed: %+v, %v, journal %s", result, err, snapshotJournal(f, snapApplied))
	}
	f = generation(t)
	f.onOpen = duringCopy(f, func(*node) {
		game := f.get(gameDir)
		game.children["current"] = copyTree(f, game.children["current"])
	})
	result, err = snapshotApply(f, newFakeVaults(f))
	var intervention *sync.InterventionError
	if !errors.As(err, &intervention) || result.Effect != sync.EffectIntervention ||
		snapshotJournal(f, snapApplied) != "001-planned.rec,002-staging-intent.rec" {
		t.Fatalf("identity changed: %+v, %v, journal %s", result, err, snapshotJournal(f, snapApplied))
	}
}

func TestSnapshotIDCollisionIsRefused(t *testing.T) {
	f := generation(t)
	head, data := headReceipt(t, f)
	writeSnapshotRecords(t, f, snapApplied, snapAt.Add(-1), []backup.Step{backup.StepPlanned, backup.StepAbandoned}, func(step backup.Step) backup.StepValues {
		if step == backup.StepAbandoned {
			return backup.StepValues{Text: "earlier"}
		}
		return backup.StepValues{SnapshotSource: backup.SnapshotSource{Operation: head.Operation, ReceiptSHA256: sha(data)},
			SourceIdentity: identityOf(f.get(gameDir + `\current`)),
			SnapshotPlan:   &backup.SnapshotPlan{ToolVersion: "dev", GameID: head.GameID, GameName: head.GameName, StartedAt: snapAt.Add(-1), Sources: head.Sources}}
	})
	before := snapshot(f, gameDir+`\snapshots`)
	result, err := snapshotApply(f, newFakeVaults(f))
	if err == nil || !strings.Contains(err.Error(), "already used") || result.Effect == sync.EffectPublished || snapshot(f, gameDir+`\snapshots`) != before {
		t.Fatalf("collision: %+v, %v", result, err)
	}
}

func TestSnapshotSpaceIsCheckedBeforeAnyWrite(t *testing.T) {
	f := generation(t)
	f.described, f.later = 0, &backup.Volume{FileSystem: "NTFS", Drive: backup.DriveFixed, Available: 1 << 20}
	result, err := snapshotApply(f, newFakeVaults(f))
	var blocked *sync.BlockedError
	if !errors.As(err, &blocked) || blocked.Code != sync.CodeInsufficientSpace || f.get(gameDir+`\snapshots`) != nil || result.Effect != sync.EffectNone {
		t.Fatalf("space under lock: %+v, %v", result, err)
	}
}

func receiptNode(f *fakeFS) *node { return f.get(gameDir + `\receipts\` + op1 + ".json") }

func TestClosedGateBeforePublishFails(t *testing.T) {
	f := generation(t)
	h := hooked(f)
	commit := snapRel + "/journal/" + snapApplied + "/004-verified.rec"
	h.after["Move "+commit+".tmp-1 "+commit] = func() { receiptNode(f).data = append(receiptNode(f).data, ' ') }
	result, err := snapshotApply(f, h)
	var intervention *sync.InterventionError
	if !errors.As(err, &intervention) || !strings.HasPrefix(intervention.Cause, string(sync.CauseSourceUnverified)) ||
		result.Effect != sync.EffectStaged || !strings.HasSuffix(snapshotJournal(f, snapApplied), "005-failed.rec") ||
		f.get(gameDir+`\snapshots\`+snapApplied) != nil {
		t.Fatalf("gate before publish: %+v, %v, journal %s", result, err, snapshotJournal(f, snapApplied))
	}
}

func TestContradictoryProvenanceRollsBack(t *testing.T) {
	f := generation(t)
	h := hooked(f)
	h.after["Move "+snapRel+"/"+snapApplied+".incomplete "+snapRel+"/"+snapApplied] = func() {
		delete(f.get(gameDir+`\receipts`).children, op1+".json")
	}
	result, err := snapshotApply(f, h)
	var intervention *sync.InterventionError
	if !errors.As(err, &intervention) || !strings.Contains(intervention.Cause, "rolled back") || result.Effect != sync.EffectRolledBack ||
		!strings.HasSuffix(snapshotJournal(f, snapApplied), "006-unpublish-intent.rec,007-rolled-back.rec") ||
		f.get(gameDir+`\snapshots\`+snapApplied) != nil || f.get(gameDir+`\snapshots\`+snapApplied+".incomplete") == nil ||
		f.get(gameDir+`\snapshots\receipts\`+snapApplied+".json") != nil {
		t.Fatalf("contradictory provenance: %+v, %v, journal %s", result, err, snapshotJournal(f, snapApplied))
	}
	// sync now refuses its own vault: its receipt is gone. Snapshot does not hide it.
	if p := plan(t, f, manifest(), vaultPath); p.Outcome() != "blocked" {
		t.Fatalf("sync plan: %s", p.Outcome())
	}
}

func TestUnavailableProvenanceWritesNothing(t *testing.T) {
	f := generation(t)
	h := hooked(f)
	h.after["Move "+snapRel+"/"+snapApplied+".incomplete "+snapRel+"/"+snapApplied] = func() { receiptNode(f).busy = true }
	result, err := snapshotApply(f, h)
	var intervention *sync.InterventionError
	if !errors.As(err, &intervention) || !strings.HasPrefix(intervention.Cause, string(sync.CauseSourceUnverified)) ||
		result.Effect != sync.EffectIntervention || !strings.HasSuffix(snapshotJournal(f, snapApplied), "005-publish-intent.rec") ||
		f.get(gameDir+`\snapshots\`+snapApplied) == nil || f.get(gameDir+`\snapshots\receipts\`+snapApplied+".json") != nil {
		t.Fatalf("unavailable provenance: %+v, %v, journal %s", result, err, snapshotJournal(f, snapApplied))
	}
}

func TestSnapshotRenameRefusedRollsBack(t *testing.T) {
	f := generation(t)
	h := hooked(f)
	h.deny[snapRel+"/"+snapApplied+".incomplete"] = errors.New("access denied")
	result, err := snapshotApply(f, h)
	var intervention *sync.InterventionError
	if err == nil || errors.As(err, &intervention) || result.Effect != sync.EffectRolledBack || result.State() != "rolled back snapshot "+snapApplied ||
		!strings.HasSuffix(snapshotJournal(f, snapApplied), "005-publish-intent.rec,006-rolled-back.rec") {
		t.Fatalf("refused rename: %+v, %v", result, err)
	}
}

// A crash just before and just after every vault call of a snapshot leaves
// the current domain untouched, a vault sync still plans, and a snapshot
// never published without its intent.
func TestCrashAtEverySnapshotCall(t *testing.T) {
	clean := generation(t)
	vaults := newFakeVaults(clean)
	if _, err := snapshotApply(clean, vaults); err != nil {
		t.Fatal(err)
	}
	total := len(vaults.calls)
	for k := 1; k <= total; k++ {
		for _, after := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d %v %s", k, after, vaults.calls[k-1]), func(t *testing.T) {
				f := generation(t)
				before, syncBefore := currentDomain(f), plan(t, f, manifest(), vaultPath).Outcome()
				crashing := newFakeVaults(f)
				if after {
					crashing.crashAfter = k
				} else {
					crashing.crashBefore = k
				}
				if _, err := snapshotApply(f, crashing); err == nil {
					t.Fatal("no crash")
				}
				if currentDomain(f) != before {
					t.Fatal("the current domain changed")
				}
				if p := plan(t, f, manifest(), vaultPath); p.Outcome() != syncBefore || len(p.Interventions) != 0 {
					t.Fatalf("sync plan after the crash: %s %q", p.Outcome(), causes(p))
				}
				journal := snapshotJournal(f, snapApplied)
				if f.get(gameDir+`\snapshots\`+snapApplied) != nil && !strings.Contains(journal, "-publish-intent.rec") {
					t.Fatalf("snapshot published without its intent: %s", journal)
				}
				if _, err := service(f, manifest()).SnapshotPlan(context.Background(), sync.SnapshotRequest{ManifestPath: "m.yaml", VaultPath: vaultPath}); err != nil {
					t.Fatalf("snapshot plan after the crash: %v", err)
				}
			})
		}
	}
}
