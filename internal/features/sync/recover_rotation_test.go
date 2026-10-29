package sync_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/features/sync"
)

// recoverThrough runs --recover --apply in a new process, through vaults.
func recoverThrough(f *fakeFS, vaults sync.Vaults) (sync.Result, error) {
	return service(f, manifest()).Recover(context.Background(), sync.Request{ManifestPath: "m.yaml", VaultPath: vaultPath},
		sync.Writer{Vaults: vaults, Clock: &fakeClock{now: at.Add(recoveryLater)}, Random: strings.NewReader("\x1a\x2b\x3c\x4d"), ToolVersion: "dev"})
}

// callIndex is the 1-based position of call among calls.
func callIndex(t *testing.T, calls []string, call string) int {
	t.Helper()
	for k, c := range calls {
		if c == call {
			return k + 1
		}
	}
	t.Fatalf("no call %q in %v", call, calls)
	return 0
}

// crashRotation runs the rotation of op1 by op3, configured by configure,
// and kills it at the named vault call, before or after it.
func crashRotation(t *testing.T, call string, after bool, configure func(*hookVaults)) *fakeFS {
	t.Helper()
	reference := generation(t)
	vaults := hooked(reference)
	if configure != nil {
		configure(vaults)
	}
	_, _ = rotateWith(context.Background(), reference, vaults)
	k := callIndex(t, vaults.calls, call)
	f := generation(t)
	vaults = hooked(f)
	if configure != nil {
		configure(vaults)
	}
	if after {
		vaults.crashAfter = k
	} else {
		vaults.crashBefore = k
	}
	if _, err := rotateWith(context.Background(), f, vaults); !errors.Is(err, errCrash) && (err == nil || !strings.Contains(err.Error(), errCrash.Error())) {
		t.Fatalf("%s: no crash: %v", call, err)
	}
	return f
}

// tamperCurrent changes a byte of what current holds.
func tamperCurrent(f *fakeFS) { f.get(gameDir + `\current\saves\1.save`).data = []byte("ONE") }

func alterRetained(f *fakeFS) {
	f.get(gameDir + `\retained\` + op1 + `\saves\1.save`).data = []byte("ONE")
}

// oldIntact reports whether node holds the generation of op1, unchanged.
func oldIntact(n *node) bool {
	saves := n.children["saves"]
	return saves != nil && string(saves.children["1.save"].data) == "one" && saves.children["new.save"] == nil
}

// rotationTerminal checks the invariants after recovering a rotation: the
// operation, if any, is closed; the old generation is intact, in current or
// in retained\<op1>; a published rotation is up to date, and otherwise a
// new --apply publishes.
func rotationTerminal(t *testing.T, name string, f *fakeFS) {
	t.Helper()
	step := lastStep(f, op3)
	switch step {
	case "", "published", "abandoned", "failed", "rolled-back":
	default:
		t.Fatalf("%s: operation left at %q", name, step)
	}
	current, retained := f.get(gameDir+`\current`), f.get(gameDir+`\retained\`+op1)
	if step == "published" {
		if retained == nil || !oldIntact(retained) || current == nil || current.children["saves"].children["new.save"] == nil {
			t.Fatalf("%s: published rotation without both generations", name)
		}
		if p := plan(t, f, manifest(), vaultPath); p.Outcome() != "up to date" || p.Current != op3 {
			t.Fatalf("%s: plan %q %q", name, p.Outcome(), causes(p))
		}
		return
	}
	if current == nil || !oldIntact(current) || retained != nil {
		t.Fatalf("%s: after %q the old generation is not back in current", name, step)
	}
	result, err := service(f, manifest()).Apply(context.Background(), sync.Request{ManifestPath: "m.yaml", VaultPath: vaultPath},
		sync.Writer{Vaults: newFakeVaults(f), Clock: &fakeClock{now: at.Add(2 * timeLater)}, Random: strings.NewReader("\x3c\x4d\x5e\x6f"), ToolVersion: "dev"})
	if err != nil || result.Effect != sync.EffectPublished {
		t.Fatalf("%s: sync after recovery: %+v, %v", name, result, err)
	}
}

// Plan B2 §9.3, P0-3: a crash before and after every vault call of a
// rotation, then --recover --apply, ends at the row the table names; the
// old generation is never lost.
func TestCrashAtEveryRotationCall(t *testing.T) {
	reference := generation(t)
	clean := newFakeVaults(reference)
	if _, err := rotateWith(context.Background(), reference, clean); err != nil {
		t.Fatal(err)
	}
	reached := map[sync.RecoveryAction]bool{}
	for k := 1; k <= len(clean.calls); k++ {
		for _, after := range []bool{false, true} {
			name := fmt.Sprintf("%s %d %s", map[bool]string{false: "before", true: "after"}[after], k, clean.calls[k-1])
			f := generation(t)
			vaults := newFakeVaults(f)
			if after {
				vaults.crashAfter = k
			} else {
				vaults.crashBefore = k
			}
			if _, err := rotateWith(context.Background(), f, vaults); err == nil {
				t.Fatalf("%s: no crash", name)
			}
			before := snapshot(f, `F:\`)
			rec, err := recoverPlan(f)
			if err != nil {
				t.Fatalf("%s: recovery plan: %v", name, err)
			}
			if snapshot(f, `F:\`) != before {
				t.Fatalf("%s: the recovery plan wrote", name)
			}
			reached[rec.Action] = true
			if result, err := recoverWith(f, newFakeVaults(f)); err != nil {
				t.Fatalf("%s: recovery %q: %+v, %v", name, rec.Action, result, err)
			}
			rotationTerminal(t, name, f)
		}
	}
	for _, action := range []sync.RecoveryAction{sync.RecoverNothing, sync.RecoverAbandon, sync.RecoverRolledBack,
		sync.RecoverRestore, sync.RecoverVerifyPromote} {
		if !reached[action] {
			t.Errorf("action %q never reached", action)
		}
	}
}

// commitOfReceipt is the commit of the receipt of op by its first temporary.
func commitOfReceipt(op string) string {
	name := gameRel + "/receipts/" + op + ".json"
	return "Move " + name + ".tmp-1 " + name
}

// denyMoves refuses the moves of the named sources.
func denyMoves(sources ...string) func(*hookVaults) {
	return func(v *hookVaults) {
		for _, s := range sources {
			v.deny[s] = fs.ErrPermission
		}
	}
}

// The closed table of ADR 0006 §5, row by row: each state is produced by
// the protocol itself, then recovered.
func TestRotationRecoveryRows(t *testing.T) {
	staging, retained, current := gameRel+"/staging/"+op3, gameRel+"/retained/"+op1, gameRel+"/current"
	rows := []struct {
		name   string
		state  func(t *testing.T) *fakeFS
		action sync.RecoveryAction
		last   string // empty: an intervention, nothing written
	}{
		{"none, beside the recognised generation (M3)", func(t *testing.T) *fakeFS {
			return crashRotation(t, "CreateDir "+gameRel+"/journal/"+op3, true, nil)
		}, sync.RecoverAbandon, "abandoned"},
		{"planned", func(t *testing.T) *fakeFS { return crashRotation(t, commitOf(op3, 1, backup.StepPlanned), true, nil) },
			sync.RecoverAbandon, "abandoned"},
		{"staged", func(t *testing.T) *fakeFS { return crashRotation(t, commitOf(op3, 3, backup.StepStaged), true, nil) },
			sync.RecoverAbandon, "abandoned"},
		{"verified", func(t *testing.T) *fakeFS { return crashRotation(t, commitOf(op3, 4, backup.StepVerified), true, nil) },
			sync.RecoverAbandon, "abandoned"},
		{"retire-intent, M1 not done", func(t *testing.T) *fakeFS {
			return crashRotation(t, commitOf(op3, 5, backup.StepRetireIntent), true, nil)
		}, sync.RecoverRolledBack, "rolled-back"},
		{"retire-intent, M1 done", func(t *testing.T) *fakeFS { return crashRotation(t, m1, true, nil) },
			sync.RecoverRestore, "rolled-back"},
		{"promote-intent, M2 not done", func(t *testing.T) *fakeFS {
			return crashRotation(t, commitOf(op3, 6, backup.StepPromoteIntent), true, nil)
		}, sync.RecoverRestore, "rolled-back"},
		{"promote-intent, M2 done", func(t *testing.T) *fakeFS { return crashRotation(t, m2, true, nil) },
			sync.RecoverVerifyPromote, "published"},
		{"promote-intent, M2 done, current differs", func(t *testing.T) *fakeFS {
			f := crashRotation(t, m2, true, nil)
			tamperCurrent(f)
			return f
		}, sync.RecoverVerifyPromote, "rolled-back"},
		{"promote-intent, receipt committed", func(t *testing.T) *fakeFS { return crashRotation(t, commitOfReceipt(op3), true, nil) },
			sync.RecoverVerifyPromote, "published"},
		{"promote-intent, receipt committed, current differs", func(t *testing.T) *fakeFS {
			f := crashRotation(t, commitOfReceipt(op3), true, nil)
			tamperCurrent(f)
			return f
		}, sync.RecoverVerifyPromote, ""},
		{"restore-intent, M3 not done", func(t *testing.T) *fakeFS {
			return crashRotation(t, commitOf(op3, 7, backup.StepRestoreIntent), true, denyMoves(staging))
		}, sync.RecoverFinishRestore, "rolled-back"},
		{"restore-intent, M3 done", func(t *testing.T) *fakeFS { return crashRotation(t, m3, true, denyMoves(staging)) },
			sync.RecoverRolledBack, "rolled-back"},
		{"rollback-failed after restore-intent", func(t *testing.T) *fakeFS {
			f := generation(t)
			vaults := hooked(f)
			denyMoves(staging, retained)(vaults)
			_, _ = rotateWith(context.Background(), f, vaults)
			return f
		}, sync.RecoverFinishRestore, "rolled-back"},
		{"unpromote-intent, M4 not done", func(t *testing.T) *fakeFS {
			f := crashRotation(t, m2, true, nil)
			tamperCurrent(f)
			crashed := newFakeVaults(f)
			crashed.crashAfter = callIndex(t, dryRecovery(t, f), commitOf(op3, 7, backup.StepUnpromoteIntent))
			_, _ = recoverWith(f, crashed)
			return f
		}, sync.RecoverUnpromote, "rolled-back"},
		{"unpromote-intent, M4 done", func(t *testing.T) *fakeFS {
			f := crashRotation(t, m2, true, nil)
			tamperCurrent(f)
			crashed := newFakeVaults(f)
			crashed.crashAfter = callIndex(t, dryRecovery(t, f), "Move "+current+" "+staging)
			_, _ = recoverWith(f, crashed)
			return f
		}, sync.RecoverRestore, "rolled-back"},
		{"rollback-failed after unpromote-intent", func(t *testing.T) *fakeFS {
			f := crashRotation(t, m2, true, nil)
			tamperCurrent(f)
			vaults := hooked(f)
			denyMoves(current)(vaults)
			_, _ = recoverThrough(f, vaults)
			return f
		}, sync.RecoverUnpromote, "rolled-back"},
		{"published", func(t *testing.T) *fakeFS {
			f := generation(t)
			if _, err := rotateWith(context.Background(), f, newFakeVaults(f)); err != nil {
				t.Fatal(err)
			}
			return f
		}, sync.RecoverNothing, "published"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			f := row.state(t)
			rec, err := recoverPlan(f)
			if rec.Action != row.action && !(row.last == "" && err != nil) {
				t.Fatalf("recovery plan %q, %v; want %q", rec.Action, err, row.action)
			}
			before := snapshot(f, `F:\`)
			_, err = recoverWith(f, newFakeVaults(f))
			if row.last == "" {
				var intervention *sync.InterventionError
				if !errors.As(err, &intervention) || snapshot(f, `F:\`) != before {
					t.Fatalf("want code 3 with nothing written: %v", err)
				}
				return
			}
			if err != nil || lastStep(f, op3) != row.last {
				t.Fatalf("recovery: %v, ended at %q", err, lastStep(f, op3))
			}
			rotationTerminal(t, row.name, f)
		})
	}
}

// dryRecovery lists the vault calls a recovery of f would make, on a copy.
func dryRecovery(t *testing.T, f *fakeFS) []string {
	t.Helper()
	copied := &fakeFS{volumes: map[string]*node{}, nextID: f.nextID, volumeOf: f.volumeOf}
	for name, n := range f.volumes {
		copied.volumes[name] = shareIDs(n)
	}
	vaults := newFakeVaults(copied)
	_, _ = recoverWith(copied, vaults)
	return vaults.calls
}

// shareIDs copies a tree keeping its identities, as a dry run needs.
func shareIDs(n *node) *node {
	out := *n
	out.data = append([]byte(nil), n.data...)
	if n.children != nil {
		out.children = map[string]*node{}
		for name, child := range n.children {
			out.children[name] = shareIDs(child)
		}
	}
	return &out
}

// Plan B2 §5.9: a crash at any call of a rotation's recovery, then a new
// recovery, ends where an uninterrupted recovery ends.
func TestRotationRecoveryIsInterruptible(t *testing.T) {
	for _, point := range []struct {
		name  string
		state func(t *testing.T) *fakeFS
	}{
		{"restore after M1", func(t *testing.T) *fakeFS { return crashRotation(t, m1, true, nil) }},
		{"verify then receipt after M2", func(t *testing.T) *fakeFS { return crashRotation(t, m2, true, nil) }},
		{"unpromote then restore", func(t *testing.T) *fakeFS {
			f := crashRotation(t, m2, true, nil)
			tamperCurrent(f)
			return f
		}},
	} {
		reference := point.state(t)
		calls := dryRecovery(t, reference)
		if _, err := recoverWith(reference, newFakeVaults(reference)); err != nil {
			t.Fatal(err)
		}
		want := lastStep(reference, op3)
		for k := 1; k <= len(calls); k++ {
			for _, after := range []bool{false, true} {
				name := fmt.Sprintf("%s, recovery crash %v at %d %s", point.name, after, k, calls[k-1])
				f := point.state(t)
				crashing := newFakeVaults(f)
				if after {
					crashing.crashAfter = k
				} else {
					crashing.crashBefore = k
				}
				if _, err := recoverWith(f, crashing); err == nil {
					t.Fatalf("%s: no crash", name)
				}
				if _, err := recoverWith(f, newFakeVaults(f)); err != nil {
					t.Fatalf("%s: second recovery: %v", name, err)
				}
				if got := lastStep(f, op3); got != want {
					t.Fatalf("%s: ended at %q, want %q", name, got, want)
				}
				rotationTerminal(t, name, f)
			}
		}
	}
}

// Interruptions one after the other between M4 and M3, then relaunches.
func TestRepeatedInterruptionsBetweenUnpromoteAndRestore(t *testing.T) {
	f := crashRotation(t, m2, true, nil)
	tamperCurrent(f)
	for _, call := range []string{
		"Move " + gameRel + "/current " + gameRel + "/staging/" + op3,
		commitOf(op3, 8, backup.StepRestoreIntent),
	} {
		crashing := newFakeVaults(f)
		crashing.crashAfter = callIndex(t, dryRecovery(t, f), call)
		if _, err := recoverWith(f, crashing); err == nil {
			t.Fatalf("%s: no crash", call)
		}
	}
	crashing := newFakeVaults(f)
	crashing.crashBefore = callIndex(t, dryRecovery(t, f), m3)
	if _, err := recoverWith(f, crashing); err == nil {
		t.Fatal("no crash before M3")
	}
	if _, err := recoverWith(f, newFakeVaults(f)); err != nil || lastStep(f, op3) != "rolled-back" {
		t.Fatalf("last recovery: %v, %q", err, lastStep(f, op3))
	}
	rotationTerminal(t, "repeated interruptions", f)
}

// E1 to E3 on a rotation: nine temporaries of the next record or of the
// receipt exhaust the envelope, eight leave one attempt; the longest path
// of eleven records is reached; a second rollback-failed is refused.
func TestRotationEnvelope(t *testing.T) {
	temporaries := func(f *fakeFS, name string, n int) {
		for k := 1; k <= n; k++ {
			f.put(fmt.Sprintf(`%s.tmp-%d`, name, k), f.file("{"))
		}
	}
	record := gameDir + `\journal\` + op3 + `\` + backup.RecordName(6, backup.StepRestoreIntent)
	f := crashRotation(t, m1, true, nil)
	temporaries(f, record, backup.MaxTemporaries)
	before := snapshot(f, `F:\`)
	if _, err := recoverWith(f, newFakeVaults(f)); err == nil || !strings.Contains(err.Error(), "envelope exhausted") || snapshot(f, `F:\`) != before {
		t.Fatalf("nine record temporaries: %v", err)
	}
	f = crashRotation(t, m1, true, nil)
	temporaries(f, record, backup.MaxTemporaries-1)
	if _, err := recoverWith(f, newFakeVaults(f)); err != nil || lastStep(f, op3) != "rolled-back" {
		t.Fatalf("eight record temporaries: %v, %q", err, lastStep(f, op3))
	}
	f = crashRotation(t, m2, true, nil)
	temporaries(f, gameDir+`\receipts\`+op3+`.json`, backup.MaxTemporaries)
	before = snapshot(f, `F:\`)
	if _, err := recoverWith(f, newFakeVaults(f)); err == nil || !strings.Contains(err.Error(), "envelope exhausted") || snapshot(f, `F:\`) != before {
		t.Fatalf("nine receipt temporaries: %v", err)
	}

	// The longest path: M4 refused once, then M3 refused twice; the second
	// refusal of M3 adds nothing (E3).
	current, retained := gameRel+"/current", gameRel+"/retained/"+op1
	f = crashRotation(t, m2, true, nil)
	tamperCurrent(f)
	for i, refused := range []string{current, retained, retained} {
		vaults := hooked(f)
		denyMoves(refused)(vaults)
		before := snapshot(f, `F:\`)
		_, err := recoverThrough(f, vaults)
		var intervention *sync.InterventionError
		if !errors.As(err, &intervention) {
			t.Fatalf("refused %s: %v", refused, err)
		}
		if i == 2 && snapshot(f, `F:\`) != before {
			t.Fatalf("a second refusal of M3 wrote: %s", journalOf(f, op3))
		}
	}
	if got := strings.Count(journalOf(f, op3), "rollback-failed"); got != 2 || lastRecord(f, op3) != "010-rollback-failed.rec" {
		t.Fatalf("journal %s", journalOf(f, op3))
	}
	if _, err := recoverWith(f, newFakeVaults(f)); err != nil || lastRecord(f, op3) != "011-rolled-back.rec" {
		t.Fatalf("longest path: %v, %s", err, journalOf(f, op3))
	}
	rotationTerminal(t, "longest path", f)
}

// stable runs --recover --apply three times on an ambiguous state: code 3
// each time, the same diagnostic, the vault identical byte for byte.
func stable(t *testing.T, f *fakeFS) string {
	t.Helper()
	before := snapshot(f, `F:\`)
	var first string
	for i := range 3 {
		result, err := recoverWith(f, newFakeVaults(f))
		var intervention *sync.InterventionError
		if !errors.As(err, &intervention) || result.Effect != sync.EffectIntervention {
			t.Fatalf("run %d: %+v, %v", i+1, result, err)
		}
		if i == 0 {
			first = err.Error()
		} else if err.Error() != first {
			t.Fatalf("run %d: diagnostic %q, then %q", i+1, first, err.Error())
		}
		if snapshot(f, `F:\`) != before {
			t.Fatalf("run %d wrote", i+1)
		}
	}
	return first
}

func TestRotationAmbiguityIsStable(t *testing.T) {
	f := crashRotation(t, m1, true, nil)
	f.mkdir(gameDir + `\staging\` + op3 + `-unknown`)
	stable(t, f)
}

// R3: an unexpected object under current after M1, (other, old, new),
// matches no row: --recover --apply stops in code 3 three times, nothing
// written, the object, retained and the staging left in place.
func TestRepeatedRecoveryWithAnUnexpectedCurrentAfterRetirement(t *testing.T) {
	for _, last := range []string{m1, commitOf(op3, 6, backup.StepPromoteIntent)} {
		f := crashRotation(t, last, true, nil)
		object := f.mkdir(gameDir + `\current`)
		f.put(gameDir+`\current\other.save`, f.file("?"))
		if _, err := recoverPlan(f); err == nil {
			t.Fatalf("%s: the recovery plan accepted an unexpected current", last)
		}
		if diagnostic := stable(t, f); !strings.Contains(diagnostic, "does not match") {
			t.Fatalf("%s: diagnostic %q", last, diagnostic)
		}
		if f.get(gameDir+`\current`) != object || f.get(gameDir+`\retained\`+op1) == nil || f.get(gameDir+`\staging\`+op3) == nil {
			t.Fatalf("%s: something moved", last)
		}
	}
}

func TestRecoveryRefusesAReplacedRetained(t *testing.T) {
	f := crashRotation(t, m1, true, nil)
	retained := f.get(gameDir + `\retained`)
	retained.children[op1] = copyTree(f, retained.children[op1])
	stable(t, f)
}

// Once the unexpected object is gone, outside Savewarden, the observation
// matches the row (—, old, new) again, with all its checks.
func TestRecoveryResumesOnlyOnAFullyConformingObservation(t *testing.T) {
	f := crashRotation(t, m1, true, nil)
	f.mkdir(gameDir + `\current`)
	stable(t, f)
	delete(f.get(gameDir).children, "current")
	if rec, err := recoverPlan(f); err != nil || rec.Action != sync.RecoverRestore {
		t.Fatalf("recovery plan %q, %v", rec.Action, err)
	}
	if _, err := recoverWith(f, newFakeVaults(f)); err != nil || lastStep(f, op3) != "rolled-back" {
		t.Fatalf("recovery: %v, %q", err, lastStep(f, op3))
	}
	rotationTerminal(t, "after the object was removed", f)
}

// retainedStates are the journals whose row (—, old, new) restores.
func retainedStates(t *testing.T) map[string]func() *fakeFS {
	staging := gameRel + "/staging/" + op3
	return map[string]func() *fakeFS{
		"retire-intent":  func() *fakeFS { return crashRotation(t, m1, true, nil) },
		"promote-intent": func() *fakeFS { return crashRotation(t, commitOf(op3, 6, backup.StepPromoteIntent), true, nil) },
		"unpromote-intent": func() *fakeFS {
			f := crashRotation(t, m2, true, nil)
			tamperCurrent(f)
			crashed := newFakeVaults(f)
			crashed.crashAfter = callIndex(t, dryRecovery(t, f), "Move "+gameRel+"/current "+staging)
			_, _ = recoverWith(f, crashed)
			return f
		},
		"restore-intent": func() *fakeFS {
			return crashRotation(t, commitOf(op3, 7, backup.StepRestoreIntent), true, denyMoves(staging))
		},
	}
}

func TestRecoveryDoesNotRestoreAnAlteredRetained(t *testing.T) {
	for name, state := range retainedStates(t) {
		f := state()
		alterRetained(f)
		if diagnostic := stable(t, f); !strings.Contains(diagnostic, "nothing restored") {
			t.Fatalf("%s: diagnostic %q", name, diagnostic)
		}
		if f.get(gameDir+`\current`) != nil {
			t.Fatalf("%s: an altered generation was restored", name)
		}
	}
}

func TestRecoveryRestoresAVerifiedRetained(t *testing.T) {
	for name, state := range retainedStates(t) {
		f := state()
		if _, err := recoverWith(f, newFakeVaults(f)); err != nil || lastStep(f, op3) != "rolled-back" {
			t.Fatalf("%s: %v, %q", name, err, lastStep(f, op3))
		}
		if !strings.Contains(journalOf(f, op3), "restore-intent") {
			t.Fatalf("%s: journal %s", name, journalOf(f, op3))
		}
		rotationTerminal(t, name, f)
	}
}

// unstarted leaves the operation directory of the rotation, empty or with
// temporaries 001 only, beside the recognised generation of op1.
func unstarted(t *testing.T, temporaries bool) *fakeFS {
	t.Helper()
	if temporaries {
		return crashRotation(t, "Write "+gameRel+"/journal/"+op3+"/001-planned.rec.tmp-1", false, nil)
	}
	return crashRotation(t, "CreateDir "+gameRel+"/journal/"+op3, true, nil)
}

// expectAbandoned recovers an unstarted operation into abandoned.
func expectAbandoned(t *testing.T, f *fakeFS, op string) {
	t.Helper()
	rec, err := recoverPlan(f)
	if err != nil || rec.Action != sync.RecoverAbandon || rec.Last != "" {
		t.Fatalf("recovery plan %+v, %v", rec, err)
	}
	if _, err := recoverWith(f, newFakeVaults(f)); err != nil || lastStep(f, op) != "abandoned" {
		t.Fatalf("recovery: %v, %q", err, lastStep(f, op))
	}
}

// expectRefused requires code 3 and nothing written, plan and run alike.
func expectRefused(t *testing.T, name string, f *fakeFS) {
	t.Helper()
	before := snapshot(f, `F:\`)
	var intervention *sync.InterventionError
	if _, err := recoverPlan(f); !errors.As(err, &intervention) {
		t.Fatalf("%s: recovery plan %v", name, err)
	}
	if _, err := recoverWith(f, newFakeVaults(f)); !errors.As(err, &intervention) || snapshot(f, `F:\`) != before {
		t.Fatalf("%s: recovery %v", name, err)
	}
}

// R1, rule M3.
func TestUnstartedOperationWithoutAnyGenerationIsAbandoned(t *testing.T) {
	for _, call := range []string{"CreateDir " + gameRel + "/journal/" + op1, "Write " + gameRel + "/journal/" + op1 + "/001-planned.rec.tmp-1"} {
		r := crashAt(t, call, call[0] == 'C')
		expectAbandoned(t, r.f, op1)
		terminal(t, call, r.f)
	}
}

func TestUnstartedOperationBesideTheRecognizedGenerationIsAbandoned(t *testing.T) {
	for _, temporaries := range []bool{false, true} {
		f := unstarted(t, temporaries)
		expectAbandoned(t, f, op3)
		rotationTerminal(t, "unstarted", f)
	}
}

func TestUnstartedOperationWithPublishedGenerationButNoCurrentIsAmbiguous(t *testing.T) {
	for _, temporaries := range []bool{false, true} {
		for name, change := range map[string]func(f *fakeFS){
			"valid receipt":          func(f *fakeFS) {},
			"corrupt receipt only":   func(f *fakeFS) { r := f.get(gameDir + `\receipts\` + op1 + `.json`); r.data = r.data[:len(r.data)-5] },
			"published journal only": func(f *fakeFS) { delete(f.get(gameDir).children, "receipts") },
		} {
			f := unstarted(t, temporaries)
			delete(f.get(gameDir).children, "current")
			change(f)
			expectRefused(t, name, f)
		}
	}
}

func TestUnstartedOperationWithCurrentButNoPublishedGenerationIsAmbiguous(t *testing.T) {
	for _, call := range []string{"CreateDir " + gameRel + "/journal/" + op1, "Write " + gameRel + "/journal/" + op1 + "/001-planned.rec.tmp-1"} {
		r := crashAt(t, call, call[0] == 'C')
		r.f.mkdir(gameDir + `\current`)
		expectRefused(t, call, r.f)
	}
}

func TestUnstartedOperationBesideAnUnrecognizedCurrentIsAmbiguous(t *testing.T) {
	for _, temporaries := range []bool{false, true} {
		for name, change := range map[string]func(f *fakeFS){
			"another identity": copyCurrent,
			"altered content":  tamperCurrent,
			"journal of op1 missing": func(f *fakeFS) {
				delete(f.get(gameDir+`\journal`).children, op1)
			},
			"receipt that differs from its published record": func(f *fakeFS) {
				r := f.get(gameDir + `\receipts\` + op1 + `.json`)
				receipt, err := backup.DecodeReceipt(r.data)
				if err != nil {
					t.Fatal(err)
				}
				receipt.ToolVersion = "dev2"
				if r.data, err = backup.EncodeReceipt(receipt); err != nil {
					t.Fatal(err)
				}
			},
		} {
			f := unstarted(t, temporaries)
			change(f)
			expectRefused(t, name, f)
		}
	}
}

func TestUnstartedOperationWithAStagingIsAmbiguous(t *testing.T) {
	for _, temporaries := range []bool{false, true} {
		f := unstarted(t, temporaries)
		f.mkdir(gameDir + `\staging\` + op3)
		expectRefused(t, "staging", f)
	}
}

func TestUnstartedOperationWithAnUnexplainedRetainedIsAmbiguous(t *testing.T) {
	for _, temporaries := range []bool{false, true} {
		f := unstarted(t, temporaries)
		f.mkdir(gameDir + `\retained\` + op2)
		expectRefused(t, "retained", f)
	}
}
