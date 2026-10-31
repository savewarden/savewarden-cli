package sync_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/features/sync"
)

const snapOp = "20261106T080000-5e6f7a8b"

// withSnapshots plants under games\<id>\snapshots what no rule of the
// current domain explains: a journal that does not parse, a published
// record, a staging, a stray file. sync must read none of it.
func withSnapshots(f *fakeFS) string {
	base := gameDir + `\snapshots`
	f.put(base+`\journal\`+snapOp+`\001-planned.rec`, f.file("not a record"))
	f.put(base+`\journal\`+op2+`\001-published.rec`, f.file("not a record either"))
	f.put(base+`\receipts\`+snapOp+`.json`, f.file("{"))
	f.put(base+`\`+snapOp+`.incomplete\saves\1.save`, f.file("one"))
	f.put(base+`\notes.txt`, f.file("x"))
	f.put(base+`\link`, f.link())
	return base
}

// The snapshot domain is accepted as a plain directory and never read: a
// plan, a rotation and a recovery of current are unchanged beside it, and
// nothing under it changes (ADR 0007, B3 amendment).
func TestSyncAcceptsSnapshotsDirectory(t *testing.T) {
	f := generation(t)
	before := plan(t, f, manifest(), vaultPath)
	base := withSnapshots(f)
	planted := snapshot(f, base)
	p := plan(t, f, manifest(), vaultPath)
	if len(p.Interventions) != 0 || p.Outcome() != before.Outcome() || p.Current != before.Current ||
		!slices.Equal(findings(p), findings(before)) {
		t.Fatalf("plan beside snapshots: %q, %s; want %s", causes(p), p.Outcome(), before.Outcome())
	}
	result, err := rotateWith(context.Background(), f, newFakeVaults(f))
	if err != nil || result.Effect != sync.EffectPublished {
		t.Fatalf("rotation beside snapshots: %+v, %v", result, err)
	}
	rotated(t, f, op1, op3)
	if rec, err := recoverPlan(f); err != nil || rec.Action != sync.RecoverNothing {
		t.Fatalf("recovery plan beside snapshots: %q, %v", rec.Action, err)
	}
	if after := snapshot(f, base); after != planted {
		t.Fatalf("snapshots changed:\n%s\nwant\n%s", after, planted)
	}
}

func TestSnapshotsAsFileOrLinkIsUnexpectedForSync(t *testing.T) {
	for _, tt := range []struct {
		name  string
		plant func(f *fakeFS) *node
	}{
		{"file", func(f *fakeFS) *node { return f.file("x") }},
		{"link", func(f *fakeFS) *node { return f.link() }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := generation(t)
			f.put(gameDir+`\snapshots`, tt.plant(f))
			want := "unexpected-entry " + gameRel + "/snapshots"
			if got := strings.Join(causes(plan(t, f, manifest(), vaultPath)), ","); got != want {
				t.Fatalf("causes %q, want %q", got, want)
			}
		})
	}
}

// moveIndex returns the 1-based position of call among the mutating calls
// of a completed rotation, so that a crash can be placed right after it.
func moveIndex(t *testing.T, call string) int {
	t.Helper()
	f := generation(t)
	vaults := newFakeVaults(f)
	if _, err := rotateWith(context.Background(), f, vaults); err != nil {
		t.Fatal(err)
	}
	k := slices.Index(vaults.calls, call)
	if k < 0 {
		t.Fatalf("no call %q in %q", call, vaults.calls)
	}
	return k + 1
}

// An interrupted rotation leaves current absent; its recovery is that of
// the current domain alone, whatever the snapshot domain holds.
func TestSnapshotAnomaliesNeverBlockSync(t *testing.T) {
	k := moveIndex(t, m1)
	f := generation(t)
	base := withSnapshots(f)
	planted := snapshot(f, base)
	vaults := newFakeVaults(f)
	vaults.crashAfter = k
	if _, err := rotateWith(context.Background(), f, vaults); err == nil {
		t.Fatal("the rotation was not interrupted")
	}
	if f.get(gameDir+`\current`) != nil {
		t.Fatal("current should be retired, absent")
	}
	if p := plan(t, f, manifest(), vaultPath); strings.Join(causes(p), ",") != "pending-operation "+op3 {
		t.Fatalf("plan causes %q", causes(p))
	}
	rec, err := recoverPlan(f)
	if err != nil || rec.Action != sync.RecoverRestore {
		t.Fatalf("recovery plan: %q, %v", rec.Action, err)
	}
	if result, err := recoverWith(f, newFakeVaults(f)); err != nil || result.Effect != sync.EffectRolledBack {
		t.Fatalf("recovery: %+v, %v", result, err)
	}
	if f.get(gameDir+`\current`) == nil {
		t.Fatal("current was not restored")
	}
	if after := snapshot(f, base); after != planted {
		t.Fatalf("snapshots changed:\n%s\nwant\n%s", after, planted)
	}
}

// I8 by domain: an incoherent snapshot journal stops snapshot in code 3,
// and sync plans and rotates as before.
func TestSnapshotDomainAmbiguityNeverBlocksSync(t *testing.T) {
	f := generation(t)
	before := plan(t, f, manifest(), vaultPath)
	f.mkdir(gameDir + `\snapshots\receipts`)
	f.put(gameDir+`\snapshots\journal\`+snapOp+`\001-planned.rec`, f.file("not a record"))
	if p := snapshotPlanOf(t, f); p.Outcome() != "intervention required" {
		t.Fatalf("snapshot plan: %s %q", p.Outcome(), snapshotCauses(p))
	}
	var intervention *sync.InterventionError
	if _, err := snapshotApply(f, newFakeVaults(f)); !errors.As(err, &intervention) {
		t.Fatalf("snapshot --apply: %v", err)
	}
	if p := plan(t, f, manifest(), vaultPath); len(p.Interventions) != 0 || p.Outcome() != before.Outcome() || p.Current != before.Current {
		t.Fatalf("sync plan: %s %q", p.Outcome(), causes(p))
	}
	if result, err := rotateWith(context.Background(), f, newFakeVaults(f)); err != nil || result.Effect != sync.EffectPublished {
		t.Fatalf("rotation: %+v, %v", result, err)
	}
}

// Journals of the snapshot domain never explain an object of current: a
// staging named after a snapshot operation stays unexplained, and rule M3
// does not count a published snapshot as a published generation.
func TestSnapshotJournalsStayOutOfSyncSets(t *testing.T) {
	f := world()
	markVault(t, f, vaultPath)
	withSnapshots(f)
	f.mkdir(gameDir + `\staging\` + snapOp)
	if got := strings.Join(causes(plan(t, f, manifest(), vaultPath)), ","); got != "unexpected-entry "+gameRel+"/staging/"+snapOp {
		t.Fatalf("causes %q", got)
	}
	f = world()
	markVault(t, f, vaultPath)
	withSnapshots(f)
	f.mkdir(gameDir + `\journal\` + op1)
	// No receipt and no journal of current published anything: the
	// operation never started is abandoned, current being absent.
	rec, err := recoverPlan(f)
	if err != nil || rec.Action != sync.RecoverAbandon || rec.Observed == nil || rec.Observed.Expected {
		t.Fatalf("recovery plan: %q, %+v, %v", rec.Action, rec.Observed, err)
	}
}

func TestRotationLeavesSnapshotsUntouched(t *testing.T) {
	k := moveIndex(t, m2)
	f := generation(t)
	base := withSnapshots(f)
	planted := snapshot(f, base)
	vaults := newFakeVaults(f)
	vaults.crashBefore = k
	if _, err := rotateWith(context.Background(), f, vaults); err == nil {
		t.Fatal("the rotation was not interrupted")
	}
	if _, err := recoverWith(f, newFakeVaults(f)); err != nil {
		t.Fatalf("recovery: %v", err)
	}
	// A new rotation, by another operation, completes.
	again, err := service(f, manifest()).Apply(context.Background(), sync.Request{ManifestPath: "m.yaml", VaultPath: vaultPath},
		sync.Writer{Vaults: newFakeVaults(f), Clock: &fakeClock{now: at.Add(2 * recoveryLater)}, Random: strings.NewReader("\x3c\x4d\x5e\x6f"), ToolVersion: "dev"})
	if err != nil || again.Effect != sync.EffectPublished {
		t.Fatalf("rotation after recovery: %+v, %v", again, err)
	}
	if after := snapshot(f, base); after != planted {
		t.Fatalf("snapshots changed:\n%s\nwant\n%s", after, planted)
	}
}
