package sync_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/features/sync"
)

// snapApplied2 is the operation of a second snapshot, taken at snapAt
// with another random part.
const snapApplied2 = "20261106T101500-6f7a8b9c"

var (
	snapUnmove     = "Move " + snapRel + "/" + snapApplied + " " + snapRel + "/" + snapApplied + ".incomplete"
	snapIncomplete = "CreateDir " + snapRel + "/" + snapApplied + ".incomplete"
)

// contradicted returns a vault whose source receipt disappears just after
// the switch of the snapshot: provenance is then contradictory.
func contradicted(f *fakeFS) *hookVaults {
	h := hooked(f)
	h.after[snapMove] = func() { delete(f.get(gameDir+`\receipts`).children, op1+".json") }
	return h
}

// ADR 0006 §3: every move of the snapshot domain follows the commit of its
// intent, in creation, in rollback and in recovery.
func TestSnapshotWriteAheadOrder(t *testing.T) {
	after := func(t *testing.T, calls []string, action, intent string) {
		t.Helper()
		a, i := slices.Index(calls, action), slices.Index(calls, intent)
		if a < 0 || i < 0 || a < i {
			t.Fatalf("%s at %d, its intent %s at %d", action, a, intent, i)
		}
	}
	calls := snapshotCalls(t)
	after(t, calls, snapIncomplete, snapCommit(2, "staging-intent"))
	after(t, calls, snapMove, snapCommit(5, "publish-intent"))
	f := generation(t)
	h := contradicted(f)
	if _, err := snapshotApply(f, h); err == nil {
		t.Fatal("not rolled back")
	}
	after(t, h.calls, snapUnmove, snapCommit(6, "unpublish-intent"))
	// The same rollback, decided by recovery.
	f = interruptedAt(t, snapMove, true)
	delete(f.get(gameDir+`\receipts`).children, op1+".json")
	vaults := newFakeVaults(f)
	if result, err := snapshotRecoverWith(f, vaults); err == nil || result.Effect != sync.EffectRolledBack {
		t.Fatalf("recovery: %+v, %v", result, err)
	}
	after(t, vaults.calls, snapUnmove, snapCommit(6, "unpublish-intent"))
}

// A crash before or after every call of the rollback that contradictory
// provenance starts leaves a state recovery closes in rolled-back, the
// copy kept incomplete, nothing published.
func TestCrashDuringSnapshotRollback(t *testing.T) {
	f := generation(t)
	h := contradicted(f)
	if _, err := snapshotApply(f, h); err == nil {
		t.Fatal("not rolled back")
	}
	calls, switched := h.calls, slices.Index(h.calls, snapMove)+1
	for k := switched + 1; k <= len(calls); k++ {
		for _, crashAfter := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d %v %s", k, crashAfter, calls[k-1]), func(t *testing.T) {
				f := generation(t)
				h := contradicted(f)
				if crashAfter {
					h.crashAfter = k
				} else {
					h.crashBefore = k
				}
				if _, err := snapshotApply(f, h); err == nil {
					t.Fatal("no crash")
				}
				// Killed after its last commit, the rollback is already closed.
				if rec, err := snapshotRecoverPlan(f); err != nil {
					t.Fatalf("recovery plan: %v", err)
				} else if rec.Action != sync.RecoverNothing {
					if result, err := snapshotRecoverWith(f, newFakeVaults(f)); result.Effect != sync.EffectRolledBack {
						t.Fatalf("recovery: %+v, %v", result, err)
					}
				}
				if last := settled(t, f); last != "rolled-back.rec" || f.get(gameDir+`\snapshots\`+snapApplied+".incomplete") == nil {
					t.Fatalf("settled in %s", last)
				}
			})
		}
	}
}

// A recovery that rolls back on contradictory provenance, killed at any of
// its calls, is recovered again to rolled-back (ADR 0006 §9).
func TestSnapshotRecoveryRollbackIsInterruptible(t *testing.T) {
	prepare := func(t *testing.T) *fakeFS {
		f := interruptedAt(t, snapMove, true)
		delete(f.get(gameDir+`\receipts`).children, op1+".json")
		return f
	}
	clean := newFakeVaults(prepare(t))
	if _, err := snapshotRecoverWith(clean.f, clean); err == nil {
		t.Fatal("not rolled back")
	}
	for k := 1; k <= len(clean.calls); k++ {
		for _, crashAfter := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d %v %s", k, crashAfter, clean.calls[k-1]), func(t *testing.T) {
				f := prepare(t)
				crashing := newFakeVaults(f)
				if crashAfter {
					crashing.crashAfter = k
				} else {
					crashing.crashBefore = k
				}
				if _, err := snapshotRecoverWith(f, crashing); err == nil {
					t.Fatal("no crash")
				}
				if rec, err := snapshotRecoverPlan(f); err != nil {
					t.Fatalf("second recovery plan: %v", err)
				} else if rec.Action != sync.RecoverNothing {
					if result, err := snapshotRecoverWith(f, newFakeVaults(f)); result.Effect != sync.EffectRolledBack {
						t.Fatalf("second recovery: %+v, %v", result, err)
					}
				}
				if last := settled(t, f); last != "rolled-back.rec" {
					t.Fatalf("settled in %s", last)
				}
			})
		}
	}
}

// A move back refused by an open file records rollback-failed once (E3):
// a second refusal records nothing, and a recovery once the file is closed
// ends rolled-back.
func TestSnapshotRollbackFailedIsRecordedOnce(t *testing.T) {
	published := snapRel + "/" + snapApplied
	f := generation(t)
	h := contradicted(f)
	h.deny[published] = errors.New("access denied")
	var intervention *sync.InterventionError
	if _, err := snapshotApply(f, h); !errors.As(err, &intervention) ||
		!strings.HasSuffix(snapshotJournal(f, snapApplied), "006-unpublish-intent.rec,007-rollback-failed.rec") {
		t.Fatalf("refused move back: %v, journal %s", err, snapshotJournal(f, snapApplied))
	}
	for range 2 {
		denied := hooked(f)
		denied.deny[published] = errors.New("access denied")
		before := snapshot(f, `F:\`)
		if _, err := snapshotRecoverWith(f, denied); !errors.As(err, &intervention) || snapshot(f, `F:\`) != before {
			t.Fatalf("refused again: %v, journal %s", err, snapshotJournal(f, snapApplied))
		}
	}
	result, err := snapshotRecoverWith(f, newFakeVaults(f))
	if err != nil || result.Effect != sync.EffectRolledBack || !strings.HasSuffix(snapshotJournal(f, snapApplied), "007-rollback-failed.rec,008-rolled-back.rec") {
		t.Fatalf("recovery: %+v, %v, journal %s", result, err, snapshotJournal(f, snapApplied))
	}
}

// Plan B3 §6: once published, a snapshot, its receipt and its journal are
// never created, written or moved again, through snapshot, sync, a
// rotation and its recovery, and the recovery of another snapshot.
func TestPublishedSnapshotIsNeverMovedOrReopenedForWriting(t *testing.T) {
	f := generation(t)
	if _, err := snapshotApply(f, newFakeVaults(f)); err != nil {
		t.Fatal(err)
	}
	approved := degradedLines(f, snapApplied)
	var runs []*fakeVaults
	use := func() *fakeVaults {
		v := newFakeVaults(f)
		runs = append(runs, v)
		return v
	}
	if result, err := snapshotApply(f, use()); err != nil || result.Effect != sync.EffectNone {
		t.Fatalf("up to date: %+v, %v", result, err)
	}
	if _, err := snapshotRecoverWith(f, use()); err != nil {
		t.Fatal(err)
	}
	// A rotation of current killed after its retirement, then recovered.
	crashing := use()
	crashing.crashAfter = moveIndex(t, m1)
	if _, err := rotateWith(context.Background(), f, crashing); err == nil {
		t.Fatal("the rotation was not interrupted")
	}
	if _, err := recoverWith(f, use()); err != nil {
		t.Fatal(err)
	}
	again, err := service(f, manifest()).Apply(context.Background(), sync.Request{ManifestPath: "m.yaml", VaultPath: vaultPath},
		sync.Writer{Vaults: use(), Clock: &fakeClock{now: at.Add(2 * recoveryLater)}, Random: strings.NewReader("\x3c\x4d\x5e\x6f"), ToolVersion: "dev"})
	if err != nil || again.Effect != sync.EffectPublished {
		t.Fatalf("rotation: %+v, %v", again, err)
	}
	// A second snapshot, killed just after its switch, then recovered.
	h := hooked(f)
	h.after["Move "+snapRel+"/"+snapApplied2+".incomplete "+snapRel+"/"+snapApplied2] = func() { h.dead = true }
	if _, err := service(f, manifest()).SnapshotApply(context.Background(), sync.SnapshotRequest{ManifestPath: "m.yaml", VaultPath: vaultPath},
		snapWriter(h, "\x6f\x7a\x8b\x9c")); err == nil {
		t.Fatal("the second snapshot was not interrupted")
	}
	runs = append(runs, h.fakeVaults)
	if result, err := snapshotRecoverWith(f, use()); err != nil || result.Effect != sync.EffectPublished {
		t.Fatalf("recovery of the second snapshot: %+v, %v", result, err)
	}
	var calls []string
	for _, v := range runs {
		calls = append(calls, v.calls...)
	}
	for _, protected := range []string{snapRel + "/" + snapApplied, snapRel + "/receipts/" + snapApplied + ".json", snapRel + "/journal/" + snapApplied} {
		for _, call := range calls {
			if strings.Contains(call, " "+protected+" ") || strings.HasSuffix(call, " "+protected) || strings.Contains(call, " "+protected+"/") {
				t.Errorf("call on the approved snapshot: %s", call)
			}
		}
	}
	if after := degradedLines(f, snapApplied); after != approved {
		t.Fatalf("the approved snapshot changed:\n%s\nwant\n%s", after, approved)
	}
	if p := snapshotPlanOf(t, f); len(p.Snapshots) != 2 || p.Degraded() != 0 || p.UpToDate != snapApplied2 {
		t.Fatalf("plan: %+v", p)
	}
}

// An operation id used by current is refused under lock, code 1, before
// any snapshot journal.
func TestSnapshotIDUsedByCurrentIsRefused(t *testing.T) {
	f := generation(t)
	rotation, err := service(f, manifest()).Apply(context.Background(), sync.Request{ManifestPath: "m.yaml", VaultPath: vaultPath},
		sync.Writer{Vaults: newFakeVaults(f), Clock: &fakeClock{now: snapAt}, Random: strings.NewReader("\x5e\x6f\x7a\x8b"), ToolVersion: "dev"})
	if err != nil || rotation.Operation != snapApplied {
		t.Fatalf("rotation: %+v, %v", rotation, err)
	}
	before := snapshot(f, `F:\`)
	result, err := snapshotApply(f, newFakeVaults(f))
	var intervention *sync.InterventionError
	if err == nil || errors.As(err, &intervention) || !strings.Contains(err.Error(), "already used") || result.Effect != sync.EffectNone {
		t.Fatalf("collision: %+v, %v", result, err)
	}
	if after := snapshot(f, `F:\`); after != before {
		t.Fatalf("the vault changed:\n%s\nwant\n%s", after, before)
	}
}
