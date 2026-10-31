package sync_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/features/sync"
)

var (
	snapMove    = "Move " + snapRel + "/" + snapApplied + ".incomplete " + snapRel + "/" + snapApplied
	snapReceipt = "Move " + snapRel + "/receipts/" + snapApplied + ".json.tmp-1 " + snapRel + "/receipts/" + snapApplied + ".json"
)

func snapCommit(seq int, step string) string {
	name := fmt.Sprintf("%s/journal/%s/%03d-%s.rec", snapRel, snapApplied, seq, step)
	return "Move " + name + ".tmp-1 " + name
}

// snapshotCalls lists the mutating vault calls of a clean snapshot.
func snapshotCalls(t *testing.T) []string {
	t.Helper()
	f := generation(t)
	vaults := newFakeVaults(f)
	if _, err := snapshotApply(f, vaults); err != nil {
		t.Fatal(err)
	}
	return vaults.calls
}

// interrupted returns a vault whose snapshot was killed before or after
// call k.
func interrupted(t *testing.T, k int, after bool) *fakeFS {
	t.Helper()
	f := generation(t)
	vaults := newFakeVaults(f)
	if after {
		vaults.crashAfter = k
	} else {
		vaults.crashBefore = k
	}
	if _, err := snapshotApply(f, vaults); err == nil {
		t.Fatal("no crash")
	}
	return f
}

// interruptedAt is interrupted at the named call.
func interruptedAt(t *testing.T, call string, after bool) *fakeFS {
	t.Helper()
	k := slices.Index(snapshotCalls(t), call)
	if k < 0 {
		t.Fatalf("no call %q", call)
	}
	return interrupted(t, k+1, after)
}

func snapshotRecoverWith(f *fakeFS, vaults sync.Vaults) (sync.SnapshotResult, error) {
	return service(f, manifest()).SnapshotRecover(context.Background(), sync.SnapshotRequest{ManifestPath: "m.yaml", VaultPath: vaultPath},
		sync.Writer{Vaults: vaults, Clock: &fakeClock{now: snapAt.Add(timeLater)}, Random: strings.NewReader("\x00\x00\x00\x00"), ToolVersion: "dev"})
}

func snapshotRecoverPlan(f *fakeFS) (sync.SnapshotRecovery, error) {
	return service(f, manifest()).SnapshotRecoverPlan(context.Background(), sync.SnapshotRequest{ManifestPath: "m.yaml", VaultPath: vaultPath})
}

// lastSnapshotStep returns the file name, without its number, of the last
// record of the snapshot operation; temporaries left beside records are
// abandoned attempts, ignored.
func lastSnapshotStep(f *fakeFS) string {
	last := ""
	for _, name := range strings.Split(snapshotJournal(f, snapApplied), ",") {
		if strings.HasSuffix(name, ".rec") {
			last = name[strings.Index(name, "-")+1:]
		}
	}
	return last
}

// settled checks a snapshot operation closed by recovery: terminal, the
// snapshot domain without any intervention, a second recovery with
// nothing to do, and the snapshot published only if its journal says so.
func settled(t *testing.T, f *fakeFS) string {
	t.Helper()
	journal := snapshotJournal(f, snapApplied)
	last := lastSnapshotStep(f)
	if journal != "" && !slices.Contains([]string{"published.rec", "abandoned.rec", "rolled-back.rec", "failed.rec"}, last) {
		t.Fatalf("not terminal: %s", journal)
	}
	if rec, err := snapshotRecoverPlan(f); err != nil || rec.Action != sync.RecoverNothing {
		t.Fatalf("second recovery: %q, %v", rec.Action, err)
	}
	p := snapshotPlanOf(t, f)
	if len(p.Interventions) != 0 || p.Degraded() != 0 {
		t.Fatalf("snapshot plan: %q, degraded %d", snapshotCauses(p), p.Degraded())
	}
	published := f.get(gameDir+`\snapshots\`+snapApplied) != nil
	if published != (last == "published.rec") || (published && p.UpToDate != snapApplied) {
		t.Fatalf("published %v, journal %s, up to date %q", published, journal, p.UpToDate)
	}
	return last
}

// ADR 0006 §10: after a crash before or after any vault call of a
// snapshot, recovery closes the operation, published or not, without
// touching the current domain.
func TestSnapshotRecoveryAfterEveryCrash(t *testing.T) {
	calls := snapshotCalls(t)
	for k := 1; k <= len(calls); k++ {
		for _, after := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d %v %s", k, after, calls[k-1]), func(t *testing.T) {
				f := interrupted(t, k, after)
				before := currentDomain(f)
				if rec, err := snapshotRecoverPlan(f); err == nil && rec.Action != sync.RecoverNothing {
					if result, err := snapshotRecoverWith(f, newFakeVaults(f)); err != nil {
						t.Fatalf("recovery %q: %+v, %v", rec.Action, result, err)
					}
				} else if err != nil {
					t.Fatalf("recovery plan: %v", err)
				}
				last := settled(t, f)
				if k > slices.Index(calls, snapMove) && (k > slices.Index(calls, snapMove)+1 || after) && last != "published.rec" {
					t.Fatalf("a crash after the switch must end published, got %s", last)
				}
				if currentDomain(f) != before {
					t.Fatal("recovery changed the current domain")
				}
			})
		}
	}
}

// A recovery killed at any of its own calls is recovered again to the
// same end (ADR 0006 §9, interruptible and convergent).
func TestSnapshotRecoveryIsInterruptible(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(f *fakeFS)
		want   string
	}{
		{"publish", func(*fakeFS) {}, "published.rec"},
		{"move back", func(f *fakeFS) { f.get(gameDir + `\snapshots\` + snapApplied + `\saves\1.save`).data = []byte("uno") }, "rolled-back.rec"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			base := interruptedAt(t, snapMove, true)
			tt.change(base)
			clean := newFakeVaults(base)
			if _, err := snapshotRecoverWith(base, clean); err != nil {
				t.Fatal(err)
			}
			for k := 1; k <= len(clean.calls); k++ {
				for _, after := range []bool{false, true} {
					f := interruptedAt(t, snapMove, true)
					tt.change(f)
					crashing := newFakeVaults(f)
					if after {
						crashing.crashAfter = k
					} else {
						crashing.crashBefore = k
					}
					if _, err := snapshotRecoverWith(f, crashing); err == nil {
						t.Fatalf("%d %v: no crash", k, after)
					}
					if rec, err := snapshotRecoverPlan(f); err != nil {
						t.Fatalf("%d %v: recovery plan after a crash: %v", k, after, err)
					} else if rec.Action != sync.RecoverNothing {
						if _, err := snapshotRecoverWith(f, newFakeVaults(f)); err != nil {
							t.Fatalf("%d %v: second recovery: %v", k, after, err)
						}
					}
					if got := lastSnapshotStep(f); got != tt.want {
						t.Fatalf("%d %v: journal %s, want ...%s", k, after, snapshotJournal(f, snapApplied), tt.want)
					}
				}
			}
		})
	}
}

func TestRepeatedSnapshotRecoveryIsStable(t *testing.T) {
	f := interruptedAt(t, snapMove, true)
	// The snapshot and a copy under its incomplete name: no row explains it.
	f.get(gameDir + `\snapshots`).children[snapApplied+".incomplete"] = copyTree(f, f.get(gameDir+`\snapshots\`+snapApplied))
	before := snapshot(f, `F:\`)
	var first string
	for run := 0; run < 3; run++ {
		result, err := snapshotRecoverWith(f, newFakeVaults(f))
		var intervention *sync.InterventionError
		if !errors.As(err, &intervention) || result.Effect != sync.EffectIntervention {
			t.Fatalf("run %d: %+v, %v", run, result, err)
		}
		if run == 0 {
			first = err.Error()
		} else if err.Error() != first {
			t.Fatalf("run %d: diagnostic %q, first %q", run, err, first)
		}
		if snapshot(f, `F:\`) != before {
			t.Fatalf("run %d changed the vault", run)
		}
	}
}

// lockThen runs then once the lock is taken: another change of the vault,
// before the recovery reads it again under lock.
type lockThen struct {
	*fakeVaults
	then func()
}

func (l *lockThen) Open(path string) (sync.Vault, error) {
	v, err := l.fakeVaults.Open(path)
	if err != nil {
		return nil, err
	}
	return &lockThenVault{Vault: v, then: l.then}, nil
}

type lockThenVault struct {
	sync.Vault
	then func()
}

func (v *lockThenVault) Lock(name string) error {
	if err := v.Vault.Lock(name); err != nil {
		return err
	}
	v.then()
	return nil
}

// Security review of B3: a recovery that creates the missing lock, then
// stops because the vault changed, says that it wrote.
func TestSnapshotRecoveryReportsTheLockItCreated(t *testing.T) {
	f := interruptedAt(t, snapCommit(2, "staging-intent"), true)
	delete(f.get(gameDir).children, "sync.lock")
	vaults := &lockThen{fakeVaults: newFakeVaults(f), then: func() { f.mkdir(gameDir + `\snapshots\` + snapApplied + ".incomplete") }}
	result, err := snapshotRecoverWith(f, vaults)
	if err == nil || !strings.Contains(err.Error(), "vault-changed") || result.State() != "metadata only; nothing staged or published" ||
		f.get(gameDir+`\sync.lock`) == nil {
		t.Fatalf("recovery: %q, %v", result.State(), err)
	}
}

func TestSnapshotRecoverPlanWritesNothing(t *testing.T) {
	f := interruptedAt(t, snapMove, true)
	before := snapshot(f, `F:\`)
	rec, err := snapshotRecoverPlan(f)
	if err != nil || rec.Action != sync.RecoverSnapshotPublish || rec.Operation != snapApplied || rec.Observed == nil ||
		rec.Observed.Published.String() != "new" || rec.Observed.Incomplete.String() != "absent" {
		t.Fatalf("recovery plan: %+v, %v", rec, err)
	}
	if snapshot(f, `F:\`) != before {
		t.Fatal("the recovery plan changed the vault")
	}
}

func TestSnapshotRecoveryWithoutJournalHasNothingToDo(t *testing.T) {
	f := generation(t)
	if rec, err := snapshotRecoverPlan(f); err != nil || rec.Action != sync.RecoverNothing {
		t.Fatalf("no snapshots: %q, %v", rec.Action, err)
	}
	f.mkdir(gameDir + `\snapshots`)
	if result, err := snapshotRecoverWith(f, newFakeVaults(f)); err != nil || result.Effect != sync.EffectNone {
		t.Fatalf("no journal: %+v, %v", result, err)
	}
}

// Q1: a snapshot interrupted after its switch, then a rotation interrupted
// with current absent. Each domain recovers alone, in either order, and
// never touches the other.
func TestSnapshotAndCurrentRecoveriesAreIndependent(t *testing.T) {
	k := moveIndex(t, m1)
	for _, snapshotFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("snapshot first %v", snapshotFirst), func(t *testing.T) {
			f := interruptedAt(t, snapMove, true)
			f.put(installPath+`\saves\new.save`, f.file("new"))
			vaults := newFakeVaults(f)
			vaults.crashAfter = k
			if _, err := rotateWith(context.Background(), f, vaults); err == nil || f.get(gameDir+`\current`) != nil {
				t.Fatalf("the rotation was not interrupted after M1: %v", err)
			}
			recoverSnapshot := func() {
				before := currentDomain(f)
				if result, err := snapshotRecoverWith(f, newFakeVaults(f)); err != nil || result.Effect != sync.EffectPublished {
					t.Fatalf("snapshot recovery: %+v, %v", result, err)
				}
				if currentDomain(f) != before {
					t.Fatal("the snapshot recovery touched the current domain")
				}
			}
			recoverCurrent := func() {
				before := snapshot(f, gameDir+`\snapshots`)
				if result, err := recoverWith(f, newFakeVaults(f)); err != nil || result.Effect != sync.EffectRolledBack {
					t.Fatalf("current recovery: %+v, %v", result, err)
				}
				if snapshot(f, gameDir+`\snapshots`) != before {
					t.Fatal("the current recovery touched the snapshots")
				}
			}
			if snapshotFirst {
				recoverSnapshot()
				recoverCurrent()
			} else {
				recoverCurrent()
				recoverSnapshot()
			}
			if p := snapshotPlanOf(t, f); p.UpToDate != snapApplied || len(p.Interventions) != 0 {
				t.Fatalf("snapshot plan after both: %s %q", p.Outcome(), snapshotCauses(p))
			}
		})
	}
}

func TestSnapshotRecoveryIgnoresCurrentDomainAnomalies(t *testing.T) {
	f := interruptedAt(t, snapMove, true)
	game := f.get(gameDir)
	delete(game.children, "current")
	delete(game.children, "sync.lock")
	f.put(gameDir+`\notes.txt`, f.file("x"))
	f.put(gameDir+`\journal\`+op2+`\001-planned.rec`, f.file("not a record"))
	f.put(vaultPath+`\stray.txt`, f.file("x"))
	result, err := snapshotRecoverWith(f, newFakeVaults(f))
	if err != nil || result.Effect != sync.EffectPublished || f.get(gameDir+`\sync.lock`) == nil {
		t.Fatalf("recovery beside a broken current domain: %+v, %v", result, err)
	}
}

// Q2: the receipt of the generation is no longer the head receipt once
// current rotated; the gate still passes, without reading current.
func TestProvenanceSurvivesRotation(t *testing.T) {
	f := interruptedAt(t, snapMove, true)
	f.put(installPath+`\saves\new.save`, f.file("new"))
	if result, err := rotateWith(context.Background(), f, newFakeVaults(f)); err != nil || result.Effect != sync.EffectPublished {
		t.Fatalf("rotation beside a pending snapshot: %+v, %v", result, err)
	}
	result, err := snapshotRecoverWith(f, newFakeVaults(f))
	if err != nil || result.Effect != sync.EffectPublished {
		t.Fatalf("recovery after rotation: %+v, %v", result, err)
	}
	p := snapshotPlanOf(t, f)
	if len(p.Snapshots) != 1 || p.Snapshots[0].Source != op1 || p.Snapshots[0].Degraded() || p.UpToDate != "" {
		t.Fatalf("snapshot of the replaced generation: %+v", p.Snapshots)
	}
}

func TestValidatedSnapshotReceiptNeedsNoGate(t *testing.T) {
	f := interruptedAt(t, snapReceipt, true)
	delete(f.get(gameDir+`\receipts`).children, op1+".json")
	rec, err := snapshotRecoverPlan(f)
	if err != nil || rec.Action != sync.RecoverSnapshotConfirm {
		t.Fatalf("plan: %q, %v", rec.Action, err)
	}
	if result, err := snapshotRecoverWith(f, newFakeVaults(f)); err != nil || result.Effect != sync.EffectPublished {
		t.Fatalf("A6 without the source receipt: %+v, %v", result, err)
	}
}

func TestRollbackNeedsNoProvenance(t *testing.T) {
	for _, tt := range []struct {
		call string
		want string
	}{
		{snapCommit(4, "verified"), "abandoned.rec"},
		{snapCommit(5, "publish-intent"), "rolled-back.rec"},
	} {
		f := interruptedAt(t, tt.call, true)
		delete(f.get(gameDir+`\receipts`).children, op1+".json")
		if _, err := snapshotRecoverWith(f, newFakeVaults(f)); err != nil || !strings.HasSuffix(snapshotJournal(f, snapApplied), tt.want) {
			t.Fatalf("%s: %v, journal %s", tt.call, err, snapshotJournal(f, snapApplied))
		}
	}
}

func TestContradictoryProvenanceRollsBackInRecovery(t *testing.T) {
	f := interruptedAt(t, snapMove, true)
	receipt := f.get(gameDir + `\receipts\` + op1 + ".json")
	receipt.data = append(receipt.data, ' ')
	result, err := snapshotRecoverWith(f, newFakeVaults(f))
	var intervention *sync.InterventionError
	if !errors.As(err, &intervention) || !strings.HasPrefix(intervention.Cause, string(sync.CauseSourceUnverified)) || result.Effect != sync.EffectRolledBack ||
		!strings.HasSuffix(snapshotJournal(f, snapApplied), "unpublish-intent.rec,007-rolled-back.rec") || f.get(gameDir+`\snapshots\`+snapApplied) != nil {
		t.Fatalf("contradictory provenance in recovery: %+v, %v, journal %s", result, err, snapshotJournal(f, snapApplied))
	}
}

func TestUnavailableProvenanceIsRetried(t *testing.T) {
	f := interruptedAt(t, snapMove, true)
	receiptNode(f).busy = true
	before := snapshot(f, gameDir+`\snapshots`)
	result, err := snapshotRecoverWith(f, newFakeVaults(f))
	var intervention *sync.InterventionError
	if !errors.As(err, &intervention) || result.Effect != sync.EffectIntervention || snapshot(f, gameDir+`\snapshots`) != before {
		t.Fatalf("unavailable provenance: %+v, %v", result, err)
	}
	receiptNode(f).busy = false
	if result, err := snapshotRecoverWith(f, newFakeVaults(f)); err != nil || result.Effect != sync.EffectPublished {
		t.Fatalf("retry: %+v, %v", result, err)
	}
}

func TestSnapshotEnvelopeIsBounded(t *testing.T) {
	f := interruptedAt(t, snapMove, true)
	receipts := f.get(gameDir + `\snapshots\receipts`)
	for k := 1; k <= 9; k++ {
		receipts.children[fmt.Sprintf("%s.json.tmp-%d", snapApplied, k)] = f.file("")
	}
	before := snapshot(f, `F:\`)
	if _, err := snapshotRecoverWith(f, newFakeVaults(f)); err == nil || !strings.Contains(err.Error(), "envelope") || snapshot(f, `F:\`) != before {
		t.Fatalf("E1, receipt: %v", err)
	}
	// published is prepared only beside the committed receipt.
	f = interruptedAt(t, snapReceipt, true)
	journal := f.get(gameDir + `\snapshots\journal\` + snapApplied)
	for k := 1; k <= 9; k++ {
		journal.children[fmt.Sprintf("006-published.rec.tmp-%d", k)] = f.file("")
	}
	before = snapshot(f, `F:\`)
	if _, err := snapshotRecoverWith(f, newFakeVaults(f)); err == nil || !strings.Contains(err.Error(), "envelope") || snapshot(f, `F:\`) != before {
		t.Fatalf("E1, record: %v", err)
	}
}

func TestOrphanSnapshotReceiptIsAmbiguous(t *testing.T) {
	f := interruptedAt(t, snapCommit(5, "publish-intent"), true)
	if _, err := snapshotRecoverWith(f, newFakeVaults(f)); err != nil {
		t.Fatal(err)
	}
	// rolled-back, then a receipt appears for it.
	f.put(gameDir+`\snapshots\receipts\`+snapApplied+".json", f.file("{}"))
	if _, err := snapshotRecoverPlan(f); err == nil || !strings.Contains(err.Error(), "journal-ambiguous") {
		t.Fatalf("orphan receipt: %v", err)
	}
}

// A5, A6: a committed snapshot receipt that no longer matches its plan
// stops the recovery; nothing is moved back, nothing written.
func TestConfirmRefusesAnAlteredReceipt(t *testing.T) {
	f := interruptedAt(t, snapReceipt, true)
	receipt := f.get(gameDir + `\snapshots\receipts\` + snapApplied + ".json")
	receipt.data = bytes.Replace(receipt.data, []byte(`"tool_version": "dev"`), []byte(`"tool_version": "v9"`), 1)
	before := snapshot(f, `F:\`)
	result, err := snapshotRecoverWith(f, newFakeVaults(f))
	var intervention *sync.InterventionError
	if !errors.As(err, &intervention) || result.Effect != sync.EffectIntervention || snapshot(f, `F:\`) != before {
		t.Fatalf("altered committed receipt: %+v, %v", result, err)
	}
}
