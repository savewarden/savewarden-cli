package sync_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/features/sync"
)

func recoverPlan(f *fakeFS) (sync.Recovery, error) {
	return service(f, manifest()).RecoverPlan(context.Background(), sync.Request{ManifestPath: "m.yaml", VaultPath: vaultPath})
}

// recoverWith runs --recover --apply in a new process on the same disk,
// later than the runs it recovers: a first publication at, a rotation one
// hour later.
func recoverWith(f *fakeFS, vaults *fakeVaults) (sync.Result, error) {
	return service(f, manifest()).Recover(context.Background(), sync.Request{ManifestPath: "m.yaml", VaultPath: vaultPath},
		writer(vaults, &fakeClock{now: at.Add(recoveryLater)}))
}

const (
	timeLater     = 3600e9
	recoveryLater = 5400e9
)

// lastStep returns the last committed step of op, or "" without journal.
func lastStep(f *fakeFS, op string) string {
	journal := f.get(gameDir + `\journal\` + op)
	if journal == nil {
		return ""
	}
	last := ""
	for _, name := range names(journal) {
		if strings.HasSuffix(name, ".rec") {
			last = strings.TrimSuffix(name[4:], ".rec")
		}
	}
	return last
}

// terminal checks the invariants after a recovery: the operation, if any,
// is closed; a current exists only when published, with a valid receipt;
// and a new --apply then succeeds.
func terminal(t *testing.T, name string, f *fakeFS) {
	t.Helper()
	step := lastStep(f, op1)
	switch step {
	case "", "published", "abandoned", "failed", "rolled-back":
	default:
		t.Fatalf("%s: operation left at %q", name, step)
	}
	if current := f.get(gameDir + `\current`); (current != nil) != (step == "published") {
		t.Fatalf("%s: current %v after %q", name, current != nil, step)
	}
	if step == "published" {
		receipt, err := backup.DecodeReceipt(f.get(gameDir + `\receipts\` + op1 + `.json`).data)
		if err != nil {
			t.Fatalf("%s: receipt: %v", name, err)
		}
		if receipt.Recovered() && (receipt.PublishedAt != nil || receipt.RecoveredBy != "dev") {
			t.Fatalf("%s: receipt provenance %+v", name, receipt)
		}
	}
	next := &run{f: f, vaults: newFakeVaults(f), clock: &fakeClock{now: at.Add(2 * timeLater)}}
	next.result, next.err = service(f, manifest()).Apply(context.Background(), sync.Request{ManifestPath: "m.yaml", VaultPath: vaultPath},
		sync.Writer{Vaults: next.vaults, Clock: next.clock, Random: strings.NewReader("\x2b\x3c\x4d\x5e"), ToolVersion: "dev"})
	want := sync.EffectPublished
	if step == "published" {
		want = sync.EffectNone
	}
	if next.err != nil || next.result.Effect != want {
		t.Fatalf("%s: sync after recovery: %+v, %v", name, next.result, next.err)
	}
}

// Plan B2 §9.2: after a crash before or after any vault call of a first
// publication, --recover --apply closes the operation, and a new sync
// works. A3 is the crash after the rename and before the receipt.
func TestRecoveryAfterEveryCrash(t *testing.T) {
	total := len(apply(world(), vaultPath).vaults.calls)
	reached := map[sync.RecoveryAction]bool{}
	for k := 1; k <= total; k++ {
		for _, after := range []bool{false, true} {
			r := crashRun(k, after)
			name := fmt.Sprintf("%s %d %s", map[bool]string{false: "before", true: "after"}[after], k, r.vaults.calls[len(r.vaults.calls)-1])
			before := snapshot(r.f, `F:\`)
			rec, err := recoverPlan(r.f)
			if err != nil {
				t.Fatalf("%s: recovery plan: %v", name, err)
			}
			if snapshot(r.f, `F:\`) != before {
				t.Fatalf("%s: the recovery plan wrote", name)
			}
			reached[rec.Action] = true
			want := ""
			switch last := lastStep(r.f, op1); {
			case last == "published", last == "publish-intent" && r.f.get(gameDir+`\current`) != nil:
				want = "published" // A3 to A7
			case last == "publish-intent":
				want = "rolled-back" // A2
			case r.f.get(gameDir+`\journal\`+op1) != nil:
				want = "abandoned" // up to A1, I8 included
			}
			result, err := recoverWith(r.f, newFakeVaults(r.f))
			if err != nil {
				t.Fatalf("%s: recovery %q: %+v, %v", name, rec.Action, result, err)
			}
			if got := lastStep(r.f, op1); got != want {
				t.Fatalf("%s: recovery ended at %q, want %q", name, got, want)
			}
			terminal(t, name, r.f)
		}
	}
	for _, action := range []sync.RecoveryAction{sync.RecoverNothing, sync.RecoverInitialization, sync.RecoverAbandon,
		sync.RecoverRolledBack, sync.RecoverVerifyPublish} {
		if !reached[action] {
			t.Errorf("action %q never reached", action)
		}
	}
}

// A3: the rename happened, no receipt: current is verified against the
// plan, and the receipt is rebuilt with its provenance (P1-4).
func TestRecoveryRebuildsTheReceipt(t *testing.T) {
	r := crashAt(t, "Move games/synthetic-game/staging/"+op1+" games/synthetic-game/current", true)
	if p := plan(t, r.f, manifest(), vaultPath); !strings.Contains(p.Interventions[0].Detail, "current published, receipt missing") {
		t.Fatalf("plan %q", p.Interventions)
	}
	rec, err := recoverPlan(r.f)
	if err != nil || rec.Action != sync.RecoverVerifyPublish || *rec.Observed != (backup.Observation{Current: backup.New, Staging: backup.Absent}) {
		t.Fatalf("recovery plan %+v, %v", rec, err)
	}
	if _, err := recoverWith(r.f, newFakeVaults(r.f)); err != nil {
		t.Fatal(err)
	}
	receipt, err := backup.DecodeReceipt(r.f.get(gameDir + `\receipts\` + op1 + `.json`).data)
	intent, decodeErr := backup.DecodeRecord(r.f.get(gameDir + `\journal\` + op1 + `\005-publish-intent.rec`).data)
	if decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if err != nil || receipt.PublishedAt != nil || receipt.RecoveredAt == nil || receipt.RecoveredAt.Before(at.Add(timeLater)) ||
		receipt.RecoveredBy != "dev" || !receipt.PublishIntentAt.Equal(intent.At) {
		t.Fatalf("receipt %+v, %v", receipt, err)
	}
	if p := plan(t, r.f, manifest(), vaultPath); p.Outcome() != "up to date" {
		t.Fatalf("plan after recovery %q %q", p.Outcome(), findings(p))
	}
}

// crashAt kills the first publication at the named call.
func crashAt(t *testing.T, call string, after bool) *run {
	t.Helper()
	calls := apply(world(), vaultPath).vaults.calls
	for k, c := range calls {
		if c == call {
			return crashRun(k+1, after)
		}
	}
	t.Fatalf("no call %q in %v", call, calls)
	return nil
}

// A current that no longer matches the plan after the rename is moved
// back to staging, never published.
func TestRecoveryMovesBackACurrentThatDiffers(t *testing.T) {
	r := crashAt(t, "Move games/synthetic-game/staging/"+op1+" games/synthetic-game/current", true)
	r.f.get(gameDir + `\current\saves\1.save`).data = []byte("ONE")
	if _, err := recoverWith(r.f, newFakeVaults(r.f)); err != nil {
		t.Fatal(err)
	}
	if lastStep(r.f, op1) != "rolled-back" || r.f.get(gameDir+`\current`) != nil || r.f.get(gameDir+`\staging\`+op1) == nil {
		t.Fatalf("after recovery: %q, current %v", lastStep(r.f, op1), r.f.get(gameDir+`\current`) != nil)
	}
	journal := strings.Join(names(r.f.get(gameDir+`\journal\`+op1)), ",")
	if !strings.Contains(journal, "006-unpublish-intent.rec,007-rolled-back.rec") {
		t.Fatalf("journal %s", journal)
	}
}

// E3: a failed move back is recorded once as rollback-failed, code 3; a
// second failure adds nothing; once possible, the rollback completes.
func TestRollbackFailureIsRecordedOnce(t *testing.T) {
	r := crashAt(t, "Move games/synthetic-game/staging/"+op1+" games/synthetic-game/current", true)
	r.f.get(gameDir + `\current\saves\1.save`).data = []byte("ONE")
	vaults := newFakeVaults(r.f)
	vaults.busy["games/synthetic-game/current"] = 1000
	result, err := recoverWith(r.f, vaults)
	var intervention *sync.InterventionError
	if !errors.As(err, &intervention) || result.Effect != sync.EffectIntervention || lastStep(r.f, op1) != "rollback-failed" {
		t.Fatalf("first failure: %+v, %v, %q", result, err, lastStep(r.f, op1))
	}
	before := snapshot(r.f, `F:\`)
	vaults = newFakeVaults(r.f)
	vaults.busy["games/synthetic-game/current"] = 1000
	if _, err := recoverWith(r.f, vaults); !errors.As(err, &intervention) || snapshot(r.f, `F:\`) != before {
		t.Fatalf("second failure: %v, vault changed %v", err, snapshot(r.f, `F:\`) != before)
	}
	if _, err := recoverWith(r.f, newFakeVaults(r.f)); err != nil || lastStep(r.f, op1) != "rolled-back" {
		t.Fatalf("rollback once possible: %v, %q", err, lastStep(r.f, op1))
	}
}

// Plan B2 §5.9: a crash at any call of the recovery itself, then a new
// recovery, ends in the same terminal state as an uninterrupted one.
func TestRecoveryIsInterruptible(t *testing.T) {
	tamper := func(f *fakeFS) { f.get(gameDir + `\current\saves\1.save`).data = []byte("ONE") }
	for _, point := range []struct {
		call  string
		after bool
		edit  func(*fakeFS)
	}{
		{"Move games/synthetic-game/staging/" + op1 + " games/synthetic-game/current", true, nil},
		{"Move games/synthetic-game/staging/" + op1 + " games/synthetic-game/current", true, tamper},
		{"Close games/synthetic-game/journal/" + op1 + "/005-publish-intent.rec.tmp-1", true, nil},
		{"CreateDir games/synthetic-game/staging/" + op1, true, nil},
	} {
		prepare := func() *run {
			r := crashAt(t, point.call, point.after)
			if point.edit != nil {
				point.edit(r.f)
			}
			return r
		}
		reference := prepare()
		clean := newFakeVaults(reference.f)
		if _, err := recoverWith(reference.f, clean); err != nil {
			t.Fatal(err)
		}
		want := lastStep(reference.f, op1)
		for k := 1; k <= len(clean.calls); k++ {
			for _, after := range []bool{false, true} {
				name := fmt.Sprintf("%s, then recovery crash %v at %d %s", point.call, after, k, clean.calls[k-1])
				r := prepare()
				crashing := newFakeVaults(r.f)
				if after {
					crashing.crashAfter = k
				} else {
					crashing.crashBefore = k
				}
				if _, err := recoverWith(r.f, crashing); err == nil {
					t.Fatalf("%s: no crash", name)
				}
				if _, err := recoverWith(r.f, newFakeVaults(r.f)); err != nil {
					t.Fatalf("%s: second recovery: %v", name, err)
				}
				if got := lastStep(r.f, op1); got != want {
					t.Fatalf("%s: ended at %q, want %q", name, got, want)
				}
				terminal(t, name, r.f)
			}
		}
	}
}

func TestRecoveryOfAnInterruptedInitialization(t *testing.T) {
	r := crashAt(t, "Close savewarden-vault.json.tmp-1", false)
	rec, err := recoverPlan(r.f)
	if err != nil || rec.Action != sync.RecoverInitialization || rec.VaultState != sync.VaultInterrupted {
		t.Fatalf("recovery plan %+v, %v", rec, err)
	}
	// Only --recover --apply resumes it; --apply refuses.
	if refused := (&run{f: r.f, vaults: newFakeVaults(r.f), clock: &fakeClock{now: at}}).again(vaultPath); refused.err == nil {
		t.Fatal("--apply resumed an interrupted initialization")
	}
	result, err := recoverWith(r.f, newFakeVaults(r.f))
	if err != nil || result.Effect != sync.EffectMetadata {
		t.Fatalf("recovery %+v, %v", result, err)
	}
	vault := r.f.get(vaultPath)
	if got := strings.Join(names(vault), ","); got != "savewarden-vault.json,savewarden-vault.json.tmp-1" {
		t.Fatalf("vault root %s", got)
	}
	terminal(t, "interrupted initialization", r.f)
}

// I8: an operation directory without any record is abandoned.
func TestRecoveryOfAnUnstartedOperation(t *testing.T) {
	for _, call := range []string{
		"NewFile games/synthetic-game/journal/" + op1 + "/001-planned.rec.tmp-1",
		"Write games/synthetic-game/journal/" + op1 + "/001-planned.rec.tmp-1",
	} {
		r := crashAt(t, call, false)
		rec, err := recoverPlan(r.f)
		if err != nil || rec.Action != sync.RecoverAbandon || rec.Last != "" {
			t.Fatalf("%s: %+v, %v", call, rec, err)
		}
		if _, err := recoverWith(r.f, newFakeVaults(r.f)); err != nil {
			t.Fatal(err)
		}
		if got := lastStep(r.f, op1); got != "abandoned" {
			t.Fatalf("%s: %q", call, got)
		}
		terminal(t, call, r.f)
	}
}

// E1: nine temporaries of the next record, of the receipt or of the marker
// exhaust the envelope: code 3, nothing written; eight leave one attempt.
func TestRecoveryEnvelope(t *testing.T) {
	cases := []struct {
		name string
		call string
		add  func(f *fakeFS, k int)
	}{
		{"next record", "Move games/synthetic-game/journal/" + op1 + "/005-publish-intent.rec.tmp-1 games/synthetic-game/journal/" + op1 + "/005-publish-intent.rec",
			func(f *fakeFS, k int) {
				f.put(fmt.Sprintf(`%s\journal\%s\006-rolled-back.rec.tmp-%d`, gameDir, op1, k), f.file(""))
			}},
		{"receipt", "Move games/synthetic-game/staging/" + op1 + " games/synthetic-game/current",
			func(f *fakeFS, k int) {
				f.mkdir(gameDir + `\receipts`)
				f.put(fmt.Sprintf(`%s\receipts\%s.json.tmp-%d`, gameDir, op1, k), f.file("{"))
			}},
		{"marker", "Close savewarden-vault.json.tmp-1",
			func(f *fakeFS, k int) {
				if k > 1 {
					f.put(fmt.Sprintf(`%s\savewarden-vault.json.tmp-%d`, vaultPath, k), f.file(""))
				}
			}},
	}
	for _, tt := range cases {
		for _, count := range []int{8, 9} {
			r := crashAt(t, tt.call, tt.name != "marker")
			for k := 1; k <= count; k++ {
				tt.add(r.f, k)
			}
			before := snapshot(r.f, `F:\`)
			_, err := recoverWith(r.f, newFakeVaults(r.f))
			switch count {
			case 9:
				if err == nil || !strings.Contains(err.Error(), "recovery envelope exhausted") || snapshot(r.f, `F:\`) != before {
					t.Fatalf("%s, nine: %v, vault changed %v", tt.name, err, snapshot(r.f, `F:\`) != before)
				}
			default:
				if err != nil {
					t.Fatalf("%s, eight: %v", tt.name, err)
				}
				terminal(t, tt.name, r.f)
			}
		}
	}
}

// Plan B2 §5.9: an ambiguity stops every run before any write, with the
// same code and the same diagnostic, and never becomes repaired.
func TestAmbiguityIsStable(t *testing.T) {
	r := crashAt(t, "Move games/synthetic-game/staging/"+op1+" games/synthetic-game/current", true)
	// A staging reappears beside current: (new, other) is in no table.
	r.f.mkdir(gameDir + `\staging\` + op1)
	before := snapshot(r.f, `F:\`)
	var first string
	for i := range 3 {
		_, err := recoverWith(r.f, newFakeVaults(r.f))
		var intervention *sync.InterventionError
		if !errors.As(err, &intervention) || snapshot(r.f, `F:\`) != before {
			t.Fatalf("run %d: %v", i, err)
		}
		if i == 0 {
			first = err.Error()
		} else if err.Error() != first {
			t.Fatalf("run %d: %q differs from %q", i, err, first)
		}
	}
}

func TestNothingToRecover(t *testing.T) {
	f := world()
	if rec, err := recoverPlan(f); err != nil || rec.Action != sync.RecoverNothing {
		t.Fatalf("absent vault: %+v, %v", rec, err)
	}
	r := apply(f, vaultPath)
	if r.err != nil {
		t.Fatal(r.err)
	}
	before := snapshot(f, `F:\`)
	vaults := newFakeVaults(f)
	result, err := recoverWith(f, vaults)
	if err != nil || result.Effect != sync.EffectNone || len(vaults.calls) != 0 || snapshot(f, `F:\`) != before {
		t.Fatalf("terminal: %+v, %v, %d calls", result, err, len(vaults.calls))
	}
}

// P1-1 and P1-5: states that no step of the protocol produces stop every
// recovery in code 3, before any write.
func TestRecoveryRefusesWhatTheProtocolCannotProduce(t *testing.T) {
	journal := "games/synthetic-game/journal/" + op1 + "/"
	commit := func(record string) string {
		return "Move " + journal + record + ".tmp-1 " + journal + record
	}
	tests := []struct {
		name  string
		call  string
		after bool
		edit  func(f *fakeFS)
	}{
		{"staging before staging-intent", commit("001-planned.rec"), true, func(f *fakeFS) {
			f.mkdir(gameDir + `\staging\` + op1)
		}},
		{"staging gone after staged", commit("003-staged.rec"), true, func(f *fakeFS) {
			delete(f.get(gameDir+`\staging`).children, op1)
		}},
		{"current published without publish-intent", commit("004-verified.rec"), true, func(f *fakeFS) {
			staging := f.get(gameDir + `\staging`)
			f.get(gameDir).children["current"] = staging.children[op1]
			delete(staging.children, op1)
		}},
		{"final record truncated by hand", commit("003-staged.rec"), true, func(f *fakeFS) {
			n := f.get(gameDir + `\journal\` + op1 + `\003-staged.rec`)
			n.data = n.data[:len(n.data)-10]
		}},
		{"current of another identity", commit("005-publish-intent.rec"), true, func(f *fakeFS) {
			f.mkdir(gameDir + `\current`)
			delete(f.get(gameDir+`\staging`).children, op1)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := crashAt(t, tt.call, tt.after)
			tt.edit(r.f)
			before := snapshot(r.f, `F:\`)
			vaults := newFakeVaults(r.f)
			result, err := recoverWith(r.f, vaults)
			var intervention *sync.InterventionError
			if !errors.As(err, &intervention) || result.Effect != sync.EffectIntervention {
				t.Fatalf("result %+v, %v", result, err)
			}
			if snapshot(r.f, `F:\`) != before || len(vaults.calls) != 0 {
				t.Fatalf("the refused recovery wrote: %v", vaults.calls)
			}
		})
	}
}

// A5 and A6: once a receipt is committed, a current that no longer matches
// the plan is never moved back: that would leave the receipt orphan.
func TestRecoveryKeepsAPublishedReceiptWithItsCurrent(t *testing.T) {
	receipt := "Move games/synthetic-game/receipts/" + op1 + ".json.tmp-1 games/synthetic-game/receipts/" + op1 + ".json"
	published := "Close games/synthetic-game/journal/" + op1 + "/006-published.rec.tmp-1"
	for _, point := range []string{receipt, published} {
		r := crashAt(t, point, point == receipt)
		r.f.get(gameDir + `\current\saves\1.save`).data = []byte("ONE")
		before := snapshot(r.f, `F:\`)
		vaults := newFakeVaults(r.f)
		result, err := recoverWith(r.f, vaults)
		var intervention *sync.InterventionError
		if !errors.As(err, &intervention) || result.Effect != sync.EffectIntervention || !strings.Contains(err.Error(), "receipt is committed") {
			t.Fatalf("%s: %+v, %v", point, result, err)
		}
		for _, call := range vaults.calls {
			if !strings.HasPrefix(call, "Lock ") {
				t.Fatalf("%s: the refused recovery wrote: %v", point, vaults.calls)
			}
		}
		if snapshot(r.f, `F:\`) != before {
			t.Fatalf("%s: vault changed", point)
		}
	}
}

// I1: a committed receipt that does not decode is never rewritten, and no
// run adds a temporary next to it.
func TestRecoveryRefusesAnInvalidReceipt(t *testing.T) {
	r := crashAt(t, "Move games/synthetic-game/receipts/"+op1+".json.tmp-1 games/synthetic-game/receipts/"+op1+".json", true)
	r.f.get(gameDir + `\receipts\` + op1 + `.json`).data = []byte("{")
	before := snapshot(r.f, `F:\`)
	for i := range 3 {
		vaults := newFakeVaults(r.f)
		_, err := recoverWith(r.f, vaults)
		var intervention *sync.InterventionError
		if !errors.As(err, &intervention) || !strings.Contains(err.Error(), "is not valid") || len(vaults.calls) != 0 {
			t.Fatalf("run %d: %v, calls %v", i, err, vaults.calls)
		}
	}
	if snapshot(r.f, `F:\`) != before {
		t.Fatal("vault changed")
	}
}

// Plan B2 §4.6: an operation whose lock is held is running, not
// interrupted: every mode says so in code 1 and writes nothing.
func TestARunningSyncIsNotAnInterruptedOne(t *testing.T) {
	r := crashAt(t, "CreateDir games/synthetic-game/staging/"+op1, true)
	holder := newFakeVaults(r.f)
	session, err := holder.Open(vaultPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"games", "games/synthetic-game"} {
		if err := session.Hold(dir); err != nil {
			t.Fatal(err)
		}
	}
	if err := session.Lock("games/synthetic-game/sync.lock"); err != nil {
		t.Fatal(err)
	}
	before := snapshot(r.f, `F:\`)
	if err := planError(r.f, manifest(), vaultPath); err == nil || !strings.Contains(err.Error(), "another sync is running") {
		t.Fatalf("plan: %v", err)
	}
	if _, err := recoverPlan(r.f); err == nil || !strings.Contains(err.Error(), "another sync is running") {
		t.Fatalf("recover plan: %v", err)
	}
	second := &run{f: r.f, vaults: holder, clock: &fakeClock{now: at}}
	second.again(vaultPath)
	var intervention *sync.InterventionError
	if second.err == nil || errors.As(second.err, &intervention) || !strings.Contains(second.err.Error(), "another sync is running") ||
		second.result.Effect != sync.EffectNone {
		t.Fatalf("apply: %+v, %v", second.result, second.err)
	}
	if result, err := recoverWith(r.f, holder); err == nil || errors.As(err, &intervention) || result.Effect != sync.EffectNone {
		t.Fatalf("recover: %+v, %v", result, err)
	}
	if snapshot(r.f, `F:\`) != before {
		t.Fatal("vault changed")
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if rec, err := recoverPlan(r.f); err != nil || rec.Action != sync.RecoverAbandon {
		t.Fatalf("once the lock is free: %+v, %v", rec, err)
	}
}

// I3: a run that leaves its operation open says so: code 3, recover.
func TestAnOpenOperationIsAnIntervention(t *testing.T) {
	journal := "games/synthetic-game/journal/" + op1 + "/"
	for name, failing := range map[string]func(call string) bool{
		"planned record not committed": func(call string) bool { return call == "Close "+journal+"001-planned.rec.tmp-1" },
		"failure not recorded": func(call string) bool {
			return strings.HasPrefix(call, "Write games/synthetic-game/staging/") || strings.HasPrefix(call, "Close "+journal+"003-failed")
		},
	} {
		f := world()
		vaults := newFakeVaults(f)
		vaults.failWrite = func(call string) error {
			if failing(call) {
				return errors.New("disk full")
			}
			return nil
		}
		r := &run{f: f, vaults: vaults, clock: &fakeClock{now: at}}
		r.again(vaultPath)
		var intervention *sync.InterventionError
		if !errors.As(r.err, &intervention) || r.result.Effect != sync.EffectIntervention || !strings.Contains(r.err.Error(), "run sync --recover") {
			t.Fatalf("%s: %+v, %v", name, r.result, r.err)
		}
		if _, err := recoverWith(f, newFakeVaults(f)); err != nil {
			t.Fatalf("%s: recovery: %v", name, err)
		}
		terminal(t, name, f)
	}
}

// The move back of recovery checks identities first, like the switch: an
// object that appears under staging\<op> after the plan under lock is an
// ambiguity, and nothing is recorded or moved.
func TestRecoveryMoveBackRefusesAnUnexplainedStaging(t *testing.T) {
	r := crashAt(t, "Move games/synthetic-game/staging/"+op1+" games/synthetic-game/current", true)
	r.f.get(gameDir + `\current\saves\1.save`).data = []byte("ONE")
	vaults := newFakeVaults(r.f)
	racing := &racingVaults{fakeVaults: vaults, from: "never"}
	appearing := &appearingCurrent{racingVaults: racing, staging: "games/synthetic-game/current", appear: func() {
		r.f.mkdir(gameDir + `\staging\` + op1)
	}}
	_, err := service(r.f, manifest()).Recover(context.Background(), sync.Request{ManifestPath: "m.yaml", VaultPath: vaultPath},
		sync.Writer{Vaults: appearing, Clock: &fakeClock{now: at.Add(timeLater)}, Random: strings.NewReader("abcd"), ToolVersion: "dev"})
	var intervention *sync.InterventionError
	if !errors.As(err, &intervention) || !strings.Contains(err.Error(), "no record explains") {
		t.Fatalf("recovery: %v", err)
	}
	if got := lastStep(r.f, op1); got != "unpublish-intent" {
		t.Fatalf("journal ends at %q, want unpublish-intent and nothing after", got)
	}
}

// Resuming an initialization that fails again says so in its own words.
func TestFailedInitializationResumeIsAnIntervention(t *testing.T) {
	r := crashAt(t, "Close savewarden-vault.json.tmp-1", false)
	vaults := newFakeVaults(r.f)
	vaults.failWrite = func(call string) error {
		if strings.HasPrefix(call, "Close savewarden-vault.json.tmp-") {
			return errors.New("disk full")
		}
		return nil
	}
	result, err := recoverWith(r.f, vaults)
	var intervention *sync.InterventionError
	if !errors.As(err, &intervention) || result.Effect != sync.EffectIntervention ||
		!strings.Contains(err.Error(), "the vault initialization is still interrupted") {
		t.Fatalf("%+v, %v", result, err)
	}
}
