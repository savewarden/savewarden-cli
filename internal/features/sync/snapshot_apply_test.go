package sync_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"maps"
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

// Openings of current/saves/1.save by a snapshot: the plan, the plan under
// lock, the reading before staging-intent, then the copy.
const (
	beforeStagingIntent = 3
	duringTheCopy       = 4
)

// atOpening runs change at the k-th opening of current/saves/1.save.
func atOpening(f *fakeFS, k int, change func(*node)) func(string, *node) {
	opened := 0
	return func(name string, n *node) {
		if strings.Contains(name, "/current/saves/1.save") {
			if opened++; opened == k {
				change(n)
				f.onOpen = nil
			}
		}
	}
}

// duringCopy runs change when the copy opens current/saves/1.save.
func duringCopy(f *fakeFS, change func(*node)) func(string, *node) {
	return atOpening(f, duringTheCopy, change)
}

// ADR 0006 §10, creation step 2: current is read again in full after
// planned; a change of content closes the operation in failed, code 2,
// before any copy; a swapped current is code 3, nothing recorded.
func TestCurrentIsReadAgainBeforeStagingIntent(t *testing.T) {
	f := generation(t)
	f.onOpen = atOpening(f, beforeStagingIntent, func(n *node) { n.data = []byte("uno") })
	result, err := snapshotApply(f, newFakeVaults(f))
	var blocked *sync.BlockedError
	if !errors.As(err, &blocked) || blocked.Code != sync.CodeCurrentModified || snapshotJournal(f, snapApplied) != "001-planned.rec,002-failed.rec" ||
		result.State() != "metadata only; nothing staged or published" ||
		f.get(gameDir+`\snapshots\`+snapApplied+".incomplete") != nil {
		t.Fatalf("content changed: %+v, %v, journal %s", result, err, snapshotJournal(f, snapApplied))
	}
	f = generation(t)
	f.onOpen = atOpening(f, beforeStagingIntent, func(n *node) {
		n.data = []byte("uno")
		game := f.get(gameDir)
		game.children["current"] = copyTree(f, game.children["current"])
	})
	result, err = snapshotApply(f, newFakeVaults(f))
	var intervention *sync.InterventionError
	if !errors.As(err, &intervention) || result.Effect != sync.EffectIntervention || snapshotJournal(f, snapApplied) != "001-planned.rec" {
		t.Fatalf("identity and content changed: %+v, %v, journal %s", result, err, snapshotJournal(f, snapApplied))
	}
}

func TestSnapshotOfCurrentChangedDuringCopy(t *testing.T) {
	f := generation(t)
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
	// Identity and content both changed: the identity is judged first.
	f = generation(t)
	f.onOpen = duringCopy(f, func(n *node) {
		n.data = []byte("uno")
		game := f.get(gameDir)
		game.children["current"] = copyTree(f, game.children["current"])
	})
	result, err = snapshotApply(f, newFakeVaults(f))
	if !errors.As(err, &intervention) || result.Effect != sync.EffectIntervention ||
		snapshotJournal(f, snapApplied) != "001-planned.rec,002-staging-intent.rec" {
		t.Fatalf("identity and content changed: %+v, %v, journal %s", result, err, snapshotJournal(f, snapApplied))
	}
}

// ADR 0006 §10, creation step 3: an incomplete copy swapped while it is
// read back is code 3 with nothing recorded, even when its content differs
// too; a content that differs alone is verification-mismatch, failed.
func TestIncompleteCopySwappedDuringVerification(t *testing.T) {
	verifying := func(f *fakeFS, swap bool) func(string, *node) {
		return func(name string, n *node) {
			if strings.Contains(name, "/snapshots/"+snapApplied+".incomplete/saves/1.save") {
				f.onOpen = nil
				n.data = []byte("uno")
				if swap {
					snapshots := f.get(gameDir + `\snapshots`)
					snapshots.children[snapApplied+".incomplete"] = copyTree(f, snapshots.children[snapApplied+".incomplete"])
				}
			}
		}
	}
	f := generation(t)
	f.onOpen = verifying(f, false)
	_, err := snapshotApply(f, newFakeVaults(f))
	var blocked *sync.BlockedError
	if !errors.As(err, &blocked) || blocked.Code != sync.CodeVerificationMismatch || !strings.HasSuffix(snapshotJournal(f, snapApplied), "003-staged.rec,004-failed.rec") {
		t.Fatalf("content differs: %v, journal %s", err, snapshotJournal(f, snapApplied))
	}
	f = generation(t)
	f.onOpen = verifying(f, true)
	result, err := snapshotApply(f, newFakeVaults(f))
	var intervention *sync.InterventionError
	if !errors.As(err, &intervention) || result.Effect != sync.EffectIntervention || !strings.HasSuffix(snapshotJournal(f, snapApplied), "003-staged.rec") {
		t.Fatalf("swapped: %+v, %v, journal %s", result, err, snapshotJournal(f, snapApplied))
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

// Security review of B3: a source receipt larger than any receipt the plan
// could have read is contradictory evidence, rolled back, never left
// pending as unavailable (ADR 0006 §10).
func TestOversizedSourceReceiptIsContradictory(t *testing.T) {
	f := generation(t)
	h := hooked(f)
	h.after[snapMove] = func() { receiptNode(f).data = bytes.Repeat([]byte(" "), 64<<20+1) }
	result, err := snapshotApply(f, h)
	var intervention *sync.InterventionError
	if !errors.As(err, &intervention) || !strings.Contains(intervention.Cause, "rolled back") || result.Effect != sync.EffectRolledBack ||
		!strings.HasSuffix(snapshotJournal(f, snapApplied), "006-unpublish-intent.rec,007-rolled-back.rec") {
		t.Fatalf("oversized source receipt: %+v, %v, journal %s", result, err, snapshotJournal(f, snapApplied))
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

// I8 by domain: a pending operation of current blocks a new snapshot, code
// 3, before the lock is taken or anything written.
func TestPendingCurrentOperationBlocksSnapshotApply(t *testing.T) {
	f := generation(t)
	writeJournal(t, f, op2, backup.StepPlanned)
	before := snapshot(f, `F:\`)
	vaults := newFakeVaults(f)
	result, err := snapshotApply(f, vaults)
	var intervention *sync.InterventionError
	if !errors.As(err, &intervention) || !strings.Contains(intervention.Cause, "pending-operation") || result.Effect != sync.EffectIntervention ||
		len(vaults.calls) != 0 || snapshot(f, `F:\`) != before {
		t.Fatalf("pending current operation: %+v, %v, calls %q", result, err, vaults.calls)
	}
}

// ADR 0006 §10, structure: a crash between the creations of the structure
// leaves a state the next run recognises: --apply completes it, after
// --recover --apply when the operation directory was created.
func TestSnapshotStructureCreationIsResumable(t *testing.T) {
	calls := snapshotCalls(t)
	structure := slices.Index(calls, snapCommit(1, "planned")) - 3 // NewFile, Write, Close of planned
	for k := 1; k <= structure; k++ {
		for _, crashAfter := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d %v %s", k, crashAfter, calls[k-1]), func(t *testing.T) {
				f := interrupted(t, k, crashAfter)
				if p := snapshotPlanOf(t, f); len(p.Pending) > 0 {
					// Nothing was copied: nothing is announced as kept.
					if result, err := snapshotRecoverWith(f, newFakeVaults(f)); err != nil || result.Effect != sync.EffectMetadata {
						t.Fatalf("recovery: %+v, %v", result, err)
					}
				} else if len(p.Interventions) != 0 {
					t.Fatalf("plan after the crash: %q", snapshotCauses(p))
				}
				result, err := service(f, manifest()).SnapshotApply(context.Background(), sync.SnapshotRequest{ManifestPath: "m.yaml", VaultPath: vaultPath},
					snapWriter(newFakeVaults(f), "\x6f\x7a\x8b\x9c"))
				if err != nil || result.Effect != sync.EffectPublished {
					t.Fatalf("next --apply: %+v, %v", result, err)
				}
			})
		}
	}
}

// pinsAtSwitch records, at every rename of a snapshot copy, the
// directories its session holds, and runs after[call] once that rename is
// done.
type pinsAtSwitch struct {
	*fakeVaults
	held  map[string][]string
	after map[string]func()
}

func newPinsAtSwitch(f *fakeFS) *pinsAtSwitch {
	return &pinsAtSwitch{fakeVaults: newFakeVaults(f), held: map[string][]string{}, after: map[string]func(){}}
}

func (p *pinsAtSwitch) Open(path string) (sync.Vault, error) {
	v, err := p.fakeVaults.Open(path)
	if err != nil {
		return nil, err
	}
	return &pinsVault{Vault: v, p: p}, nil
}

type pinsVault struct {
	sync.Vault
	p *pinsAtSwitch
}

func (v *pinsVault) Move(from, to string) error {
	call := "Move " + from + " " + to
	if !strings.Contains(from, "/journal/") && !strings.Contains(from, "/receipts/") {
		v.p.held[call] = slices.Sorted(maps.Keys(v.Vault.(*fakeVault).held))
	}
	err := v.Vault.Move(from, to)
	if hook := v.p.after[call]; hook != nil && err == nil {
		delete(v.p.after, call)
		hook()
	}
	return err
}

// ADR 0006 §10, creation step 5: across the switch, and across the move
// back of a rollback, only the ancestors of the operation's journal and
// that directory stay pinned; receipts is held only to be written.
func TestOnlyJournalAncestorsStayPinnedAcrossTheSwitch(t *testing.T) {
	journal := snapRel + "/journal/" + snapApplied
	unmove := "Move " + snapRel + "/" + snapApplied + " " + snapRel + "/" + snapApplied + ".incomplete"
	check := func(p *pinsAtSwitch, moves ...string) {
		t.Helper()
		for _, move := range moves {
			held, ok := p.held[move]
			if !ok {
				t.Fatalf("no %s among %v", move, p.held)
			}
			for _, name := range held {
				if name != "." && name != journal && !strings.HasPrefix(journal, name+"/") {
					t.Errorf("%s: %s is held", move, name)
				}
			}
		}
	}
	f := generation(t)
	p := newPinsAtSwitch(f)
	if result, err := snapshotApply(f, p); err != nil || result.Effect != sync.EffectPublished {
		t.Fatalf("apply: %+v, %v", result, err)
	}
	check(p, snapMove)
	f = generation(t)
	p = newPinsAtSwitch(f)
	p.after[snapMove] = func() { delete(f.get(gameDir+`\receipts`).children, op1+".json") }
	if result, err := snapshotApply(f, p); err == nil || result.Effect != sync.EffectRolledBack {
		t.Fatalf("contradictory provenance: %+v, %v", result, err)
	}
	check(p, snapMove, unmove)
	// The same move back, decided by recovery.
	f = interruptedAt(t, snapMove, true)
	delete(f.get(gameDir+`\receipts`).children, op1+".json")
	p = newPinsAtSwitch(f)
	if result, err := snapshotRecoverWith(f, p); err == nil || result.Effect != sync.EffectRolledBack {
		t.Fatalf("recovery: %+v, %v", result, err)
	}
	check(p, unmove)
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
