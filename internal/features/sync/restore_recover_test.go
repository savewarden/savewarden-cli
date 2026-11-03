package sync_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/entities/game"
	"github.com/savewarden/savewarden-cli/internal/features/sync"
)

var (
	restoreMove        = "Move " + restoreApplied + ".incomplete " + restoreApplied
	restoreReceiptMove = "Move receipts/" + restoreApplied + ".json.tmp-1 receipts/" + restoreApplied + ".json"
)

func restoreRecoverRequest() sync.RestoreRequest {
	return sync.RestoreRequest{ManifestPath: "m.yaml", Destination: restoreDir}
}

func restoreRecoverPlan(f *fakeFS) (sync.RestoreRecovery, error) {
	return service(f, manifest()).RestoreRecoverPlan(context.Background(), restoreRecoverRequest())
}

func restoreRecoverWith(f *fakeFS, vaults sync.Vaults) (sync.RestoreResult, error) {
	return service(f, manifest()).RestoreRecover(context.Background(), restoreRecoverRequest(),
		restoreWriter(vaults, restoreAt.Add(timeLater), "\x00\x00\x00\x00"))
}

// restoreCalls lists the calls of a clean restore.
func restoreCalls(t *testing.T) []string {
	t.Helper()
	f := restorable(t)
	vaults := newFakeVaults(f)
	if _, err := restoreApply(f, vaults); err != nil {
		t.Fatal(err)
	}
	return vaults.calls
}

// interruptedRestore returns an area whose restore was killed before or
// after call k.
func interruptedRestore(t *testing.T, k int, after bool) *fakeFS {
	t.Helper()
	f := restorable(t)
	vaults := newFakeVaults(f)
	if after {
		vaults.crashAfter = k
	} else {
		vaults.crashBefore = k
	}
	if _, err := restoreApply(f, vaults); err == nil {
		t.Fatal("no crash")
	}
	return f
}

func interruptedRestoreAt(t *testing.T, call string, after bool) *fakeFS {
	t.Helper()
	k := slices.Index(restoreCalls(t), call)
	if k < 0 {
		t.Fatalf("no call %q", call)
	}
	return interruptedRestore(t, k+1, after)
}

// lastRestoreStep returns the file name, without its number, of the last
// record of restoreApplied; temporaries are abandoned attempts.
func lastRestoreStep(f *fakeFS) string {
	last := ""
	for _, name := range strings.Split(restoreJournal(f, restoreApplied), ",") {
		if strings.HasSuffix(name, ".rec") {
			last = name[strings.Index(name, "-")+1:]
		}
	}
	return last
}

// recoverAll runs recoveries until nothing is left to recover, at most
// three times; a recovery that rolls back a copy that differs is code 2.
func recoverAll(t *testing.T, f *fakeFS) {
	t.Helper()
	for range 3 {
		rec, err := restoreRecoverPlan(f)
		if err != nil {
			t.Fatalf("recovery plan: %v", err)
		}
		if rec.Action == sync.RecoverNothing {
			return
		}
		result, err := restoreRecoverWith(f, newFakeVaults(f))
		var blocked *sync.BlockedError
		movedBack := errors.As(err, &blocked) && blocked.Code == sync.CodeVerificationMismatch
		if err != nil && !movedBack {
			t.Fatalf("recovery %q: %+v, %v", rec.Action, result, err)
		}
	}
	t.Fatal("recovery does not converge")
}

// settledArea checks an area after recovery: its operation, if any, closed;
// nothing left to recover; a plan without intervention; the restore
// published only if its journal says so.
func settledArea(t *testing.T, f *fakeFS) string {
	t.Helper()
	last := lastRestoreStep(f)
	if restoreJournal(f, restoreApplied) != "" && !slices.Contains([]string{"published.rec", "abandoned.rec", "rolled-back.rec", "failed.rec"}, last) {
		t.Fatalf("not terminal: %s", restoreJournal(f, restoreApplied))
	}
	if rec, err := restoreRecoverPlan(f); err != nil || rec.Action != sync.RecoverNothing {
		t.Fatalf("second recovery: %q, %v", rec.Action, err)
	}
	p := restorePlanOf(t, f)
	if len(p.Interventions) != 0 {
		t.Fatalf("plan after: %q", restoreCauses(p))
	}
	published := f.get(restoreDir+`\`+restoreApplied) != nil
	if published != (last == "published.rec") || (published && (len(p.Restores) != 1 || p.Restores[0].State() != "present")) {
		t.Fatalf("published %v, journal %s, restores %+v", published, restoreJournal(f, restoreApplied), p.Restores)
	}
	if published {
		r, err := backup.DecodeRestoreReceipt(f.get(restoreDir + `\receipts\` + restoreApplied + ".json").data)
		if err != nil || (r.PublishedAt == nil) == (r.RecoveredAt == nil) {
			t.Fatalf("receipt %+v, %v", r, err)
		}
	}
	return last
}

// noVaultRead fails if the vault volume was read since reads was reset.
func noVaultRead(t *testing.T, f *fakeFS) {
	t.Helper()
	for _, read := range f.reads {
		if strings.Contains(read, ` F:\`) {
			t.Fatalf("a recovery read the vault: %s", read)
		}
	}
}

// ADR 0006 §11: after a crash before or after any call of a restore,
// recovery closes the operation, published or not, never reading the
// vault; a crash after the switch ends published.
func TestRestoreRecoveryAfterEveryCrash(t *testing.T) {
	calls := restoreCalls(t)
	switchAt := slices.Index(calls, restoreMove)
	for k := 1; k <= len(calls); k++ {
		for _, after := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d %v %s", k, after, calls[k-1]), func(t *testing.T) {
				f := interruptedRestore(t, k, after)
				vault := snapshot(f, `F:\`)
				f.reads = nil
				recoverAll(t, f)
				noVaultRead(t, f)
				last := settledArea(t, f)
				if (k-1 > switchAt || (k-1 == switchAt && after)) && last != "published.rec" {
					t.Fatalf("a crash after the switch must end published, got %s", last)
				}
				if snapshot(f, `F:\`) != vault {
					t.Fatal("the vault changed")
				}
				// The area takes a new restore afterwards.
				if result, err := restoreApplyWith(f, newFakeVaults(f), restoreDir, restoreAt.Add(2*timeLater), "\x0b\x1c\x2d\x3e"); err != nil || result.Effect != sync.EffectPublished {
					t.Fatalf("next restore: %+v, %v", result, err)
				}
			})
		}
	}
}

// A recovery killed at any of its own calls is recovered again to the
// same end (ADR 0006 §9, interruptible and convergent).
func TestRestoreRecoveryIsInterruptible(t *testing.T) {
	for _, tt := range []struct {
		name   string
		call   string
		change func(f *fakeFS)
		want   string
	}{
		{"publish", restoreMove, func(*fakeFS) {}, "published.rec"},
		{"move back", restoreMove, func(f *fakeFS) { f.get(restoreDir + `\` + restoreApplied + `\saves\1.save`).data = []byte("uno") }, "rolled-back.rec"},
		{"confirm", restoreReceiptMove, func(*fakeFS) {}, "published.rec"},
		{"abandon", "CreateDir " + restoreApplied + ".incomplete", func(*fakeFS) {}, "abandoned.rec"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			base := interruptedRestoreAt(t, tt.call, true)
			tt.change(base)
			clean := newFakeVaults(base)
			_, _ = restoreRecoverWith(base, clean)
			if len(clean.calls) == 0 {
				t.Fatal("the recovery wrote nothing")
			}
			for k := 1; k <= len(clean.calls); k++ {
				for _, after := range []bool{false, true} {
					f := interruptedRestoreAt(t, tt.call, true)
					tt.change(f)
					crashing := newFakeVaults(f)
					if after {
						crashing.crashAfter = k
					} else {
						crashing.crashBefore = k
					}
					if _, err := restoreRecoverWith(f, crashing); err == nil {
						t.Fatalf("%d %v: no crash", k, after)
					}
					recoverAll(t, f)
					if got := settledArea(t, f); got != tt.want {
						t.Fatalf("%d %v: journal %s, want ...%s", k, after, restoreJournal(f, restoreApplied), tt.want)
					}
				}
			}
		})
	}
}

// ADR 0006 §11, publish-intent without receipt: a copy renamed but
// different from the plan is moved back, rolled-back, code 2.
func TestRestoreRecoveryMovesBackACopyThatDiffers(t *testing.T) {
	f := interruptedRestoreAt(t, restoreMove, true)
	f.get(restoreDir + `\` + restoreApplied + `\saves\1.save`).data = []byte("uno")
	result, err := restoreRecoverWith(f, newFakeVaults(f))
	var blocked *sync.BlockedError
	if !errors.As(err, &blocked) || blocked.Code != sync.CodeVerificationMismatch || result.Effect != sync.EffectRolledBack ||
		result.State() != "rolled back restore "+restoreApplied ||
		!strings.HasSuffix(restoreJournal(f, restoreApplied), "005-publish-intent.rec,006-unpublish-intent.rec,007-rolled-back.rec") ||
		f.get(restoreDir+`\`+restoreApplied) != nil || f.get(restoreDir+`\`+restoreApplied+".incomplete") == nil {
		t.Fatalf("move back: %+v, %v, journal %s", result, err, restoreJournal(f, restoreApplied))
	}
	// With a committed receipt nothing moves back: code 3, nothing written.
	f = interruptedRestoreAt(t, restoreReceiptMove, true)
	f.get(restoreDir + `\` + restoreApplied + `\saves\1.save`).data = []byte("uno")
	before := everything(f)
	result, err = restoreRecoverWith(f, newFakeVaults(f))
	var intervention *sync.InterventionError
	if !errors.As(err, &intervention) || result.Effect != sync.EffectIntervention || strings.Contains(restoreJournal(f, restoreApplied), "unpublish") {
		t.Fatalf("confirm refused: %+v, %v", result, err)
	}
	if everything(f) != before {
		t.Fatal("a refused confirmation wrote something")
	}
}

// Three recoveries of an ambiguity leave the area identical, byte for byte,
// with the same diagnostic (ADR 0006 §9).
func TestRepeatedRestoreRecoveryIsStable(t *testing.T) {
	f := interruptedRestoreAt(t, restoreMove, true)
	f.mkdir(restoreDir + `\` + restoreApplied + ".incomplete")
	var first string
	for i := range 3 {
		before := everything(f)
		_, err := restoreRecoverWith(f, newFakeVaults(f))
		var intervention *sync.InterventionError
		if !errors.As(err, &intervention) || everything(f) != before {
			t.Fatalf("run %d: %v", i, err)
		}
		if i == 0 {
			first = err.Error()
		} else if err.Error() != first {
			t.Fatalf("run %d: %q, first %q", i, err, first)
		}
	}
}

// The recovery plan receives no writer and writes nothing.
func TestRestoreRecoverPlanWritesNothing(t *testing.T) {
	f := interruptedRestoreAt(t, restoreMove, true)
	before := everything(f)
	rec, err := restoreRecoverPlan(f)
	if err != nil || rec.Action != sync.RecoverRestorePublish || rec.Operation != restoreApplied || rec.Last != backup.StepPublishIntent ||
		rec.Observed == nil || *rec.Observed != (backup.RestoreObservation{Published: backup.New, Incomplete: backup.Absent}) {
		t.Fatalf("recovery plan: %+v, %v", rec, err)
	}
	if !strings.Contains(rec.Warning(), "do not use "+restoreDir+`\`+restoreApplied+" yet") || everything(f) != before {
		t.Fatalf("warning %q", rec.Warning())
	}
}

// ADR 0006 §11: the recovery never opens the vault: absent, renamed or
// altered, it changes nothing, A3 included.
func TestRestoreRecoveryNeedsNoVault(t *testing.T) {
	for name, change := range map[string]func(f *fakeFS){
		"vault absent":  func(f *fakeFS) { delete(f.volumes[`F:\`].children, "Vault") },
		"volume absent": func(f *fakeFS) { delete(f.volumes, `F:\`) },
		"snapshot altered": func(f *fakeFS) {
			f.get(snapDir + `\saves\1.save`).data = []byte("uno")
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := interruptedRestoreAt(t, restoreMove, true)
			change(f)
			f.reads = nil
			vaults := newFakeVaults(f)
			result, err := restoreRecoverWith(f, vaults)
			if err != nil || result.Effect != sync.EffectPublished || !slices.Equal(vaults.sessions, []string{restoreDir}) {
				t.Fatalf("recovery: %+v, %v", result, err)
			}
			noVaultRead(t, f)
		})
	}
}

// ADR 0006 §11, destination without operation: nothing to recover
// concludes at once, without a session or any write, not even a missing
// restore.lock.
func TestNothingToRecoverOpensNoSession(t *testing.T) {
	for name, change := range map[string]func(t *testing.T, f *fakeFS){
		"absent":        func(*testing.T, *fakeFS) {},
		"parent absent": func(_ *testing.T, f *fakeFS) { delete(f.volumes[`G:\`].children, "Restores") },
		"empty":         func(_ *testing.T, f *fakeFS) { f.mkdir(restoreDir) },
		"marker only": func(t *testing.T, f *fakeFS) {
			markArea(t, f, restoreDir, "synthetic-game")
			delete(f.get(restoreDir).children, "journal")
			delete(f.get(restoreDir).children, "receipts")
		},
		"structure":   func(t *testing.T, f *fakeFS) { markArea(t, f, restoreDir, "synthetic-game") },
		"handed over": func(t *testing.T, f *fakeFS) { publishRestore(t, f, restoreDir, restored1, restoreAt) },
		"handed-over copy removed": func(t *testing.T, f *fakeFS) {
			publishRestore(t, f, restoreDir, restored1, restoreAt)
			delete(f.get(restoreDir).children, restored1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := restorable(t)
			change(t, f)
			before := everything(f)
			vaults := newFakeVaults(f)
			result, err := restoreRecoverWith(f, vaults)
			if err != nil || result.Recovery.Action != sync.RecoverNothing || result.State() != "not modified" || len(vaults.sessions) != 0 || everything(f) != before {
				t.Fatalf("nothing to recover: %+v, %v, sessions %q", result, err, vaults.sessions)
			}
		})
	}
	// P2-01 of the protocol review, the order of ADR 0006 §11: nothing to
	// recover is concluded before the manifest is read, even an invalid
	// one; a recovery that may write requires it.
	invalid := manifest()
	invalid.Game.ID = ""
	recoverWithInvalid := func(f *fakeFS) (sync.RestoreResult, error) {
		return service(f, invalid).RestoreRecover(context.Background(), restoreRecoverRequest(),
			restoreWriter(newFakeVaults(f), restoreAt.Add(timeLater), "\x00\x00\x00\x00"))
	}
	for name, change := range map[string]func(t *testing.T, f *fakeFS){
		"absent":    func(*testing.T, *fakeFS) {},
		"empty":     func(_ *testing.T, f *fakeFS) { f.mkdir(restoreDir) },
		"structure": func(t *testing.T, f *fakeFS) { markArea(t, f, restoreDir, "synthetic-game") },
	} {
		f := restorable(t)
		change(t, f)
		if result, err := recoverWithInvalid(f); err != nil || result.Recovery.Action != sync.RecoverNothing || result.Recovery.Game.ID != "" {
			t.Fatalf("%s with an invalid manifest: %+v, %v", name, result, err)
		}
	}
	f := interruptedRestoreAt(t, restoreMove, true)
	before := everything(f)
	if _, err := recoverWithInvalid(f); err == nil || everything(f) != before {
		t.Fatalf("a recovery that may write requires the manifest: %v", err)
	}
	// A destination that is not an area is refused, code 1.
	for name, change := range map[string]func(f *fakeFS){
		"file":      func(f *fakeFS) { f.put(restoreDir, f.file("x")) },
		"link":      func(f *fakeFS) { f.put(restoreDir, f.link()) },
		"not empty": func(f *fakeFS) { f.put(restoreDir+`\notes.txt`, f.file("x")) },
	} {
		f := restorable(t)
		change(f)
		_, err := restoreRecoverWith(f, newFakeVaults(f))
		expectRestoreRefused(t, err, "destination "+restoreDir)
		_ = name
	}
}

// ADR 0006 §11, C2: an operation object, a receipt or a receipt temporary
// that no journal explains is an ambiguity, code 3, nothing written.
func TestAreaWithoutJournalIsEmptyOrAmbiguous(t *testing.T) {
	for name, change := range map[string]func(f *fakeFS){
		"copy":              func(f *fakeFS) { f.mkdir(restoreDir + `\` + restored2) },
		"incomplete":        func(f *fakeFS) { f.mkdir(restoreDir + `\` + restored2 + ".incomplete") },
		"receipt":           func(f *fakeFS) { f.put(restoreDir+`\receipts\`+restored2+".json", f.file("{}")) },
		"receipt temporary": func(f *fakeFS) { f.put(restoreDir+`\receipts\`+restored2+".json.tmp-1", f.file("{")) },
		"other entry":       func(f *fakeFS) { f.put(restoreDir+`\notes.txt`, f.file("x")) },
	} {
		t.Run(name, func(t *testing.T) {
			f := restorable(t)
			markArea(t, f, restoreDir, "synthetic-game")
			change(f)
			before := everything(f)
			vaults := newFakeVaults(f)
			result, err := restoreRecoverWith(f, vaults)
			var intervention *sync.InterventionError
			if !errors.As(err, &intervention) || result.Effect != sync.EffectIntervention || len(vaults.sessions) != 0 || everything(f) != before ||
				!strings.HasPrefix(result.State(), "intervention required for the restore area: ") {
				t.Fatalf("ambiguity: %+v, %v", result, err)
			}
		})
	}
}

// ADR 0006 §11, Z3: an initialization interrupted with marker temporaries
// only is resumed by --recover --apply, without any lock, at the next free
// attempt, the temporaries kept; the area then takes a restore.
func TestRestoreAreaInitializationIsResumable(t *testing.T) {
	for _, call := range []string{"NewFile savewarden-restore.json.tmp-1", "Write savewarden-restore.json.tmp-1", "Close savewarden-restore.json.tmp-1"} {
		t.Run(call, func(t *testing.T) {
			f := interruptedRestoreAt(t, call, true)
			p := restorePlanOf(t, f)
			if !slices.Equal(restoreCauses(p), []string{"initialization-interrupted " + restoreDir}) {
				t.Fatalf("plan: %q", restoreCauses(p))
			}
			if _, err := restoreApply(f, newFakeVaults(f)); err == nil {
				t.Fatal("--apply resumes no initialization")
			}
			vaults := newFakeVaults(f)
			result, err := restoreRecoverWith(f, vaults)
			if err != nil || result.Recovery.Action != sync.RecoverAreaMarker || result.Effect != sync.EffectMetadata ||
				slices.ContainsFunc(vaults.calls, func(c string) bool { return strings.HasPrefix(c, "Lock ") }) {
				t.Fatalf("resume: %+v, %v, calls %q", result, err, vaults.calls)
			}
			if !slices.Equal(names(f.get(restoreDir)), []string{"savewarden-restore.json", "savewarden-restore.json.tmp-1"}) {
				t.Fatalf("area %q", names(f.get(restoreDir)))
			}
			if result, err := restoreApply(f, newFakeVaults(f)); err != nil || result.Effect != sync.EffectPublished {
				t.Fatalf("restore after the resume: %+v, %v", result, err)
			}
		})
	}
	// Another run validated a marker during the resume: this one loses,
	// code 1; a run that reads the area afterwards has nothing to do.
	f := interruptedRestoreAt(t, "Close savewarden-restore.json.tmp-1", true)
	hooks := hookedRestore(f)
	hooks.before["Move savewarden-restore.json.tmp-2 savewarden-restore.json"] = func() {
		data, err := backup.EncodeRestoreAreaMarker(backup.RestoreAreaMarker{CreatedAt: restoreAt, ToolVersion: "other", GameID: "synthetic-game"})
		if err != nil {
			t.Fatal(err)
		}
		f.put(restoreDir+`\savewarden-restore.json`, f.file(string(data)))
	}
	result, err := restoreRecoverWith(f, hooks)
	expectRestoreRefused(t, err, `destination-changed: another run marked this area first; read again, it is a restore area of game "synthetic-game"`)
	if result.Effect != sync.EffectMetadata {
		t.Fatalf("loser: %+v", result)
	}
	if rec, err := restoreRecoverPlan(f); err != nil || rec.Action != sync.RecoverNothing {
		t.Fatalf("after the race: %q, %v", rec.Action, err)
	}
	// P1-03 of the protocol review: a marker validated between the plan and
	// the resume is read strictly. Only a marker of this game over an area
	// without operation leaves nothing to do; anything else is refused,
	// code 1, nothing written.
	markerOf := func(gameID string) string {
		data, err := backup.EncodeRestoreAreaMarker(backup.RestoreAreaMarker{CreatedAt: restoreAt, ToolVersion: "other", GameID: gameID})
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	for _, tt := range []struct {
		name    string
		change  func(f *fakeFS)
		mention string
	}{
		{"same game", func(f *fakeFS) { f.put(restoreDir+`\savewarden-restore.json`, f.file(markerOf("synthetic-game"))) }, ""},
		{"same game, structure", func(f *fakeFS) {
			f.put(restoreDir+`\savewarden-restore.json`, f.file(markerOf("synthetic-game")))
			f.mkdir(restoreDir + `\receipts`)
			f.mkdir(restoreDir + `\journal`)
		}, ""},
		{"other game", func(f *fakeFS) { f.put(restoreDir+`\savewarden-restore.json`, f.file(markerOf("other-game"))) }, `is a restore area of game "other-game"`},
		{"invalid marker", func(f *fakeFS) { f.put(restoreDir+`\savewarden-restore.json`, f.file(`{"schema":"savewarden.vault"}`)) }, "a marker is never rewritten"},
		{"operation object", func(f *fakeFS) {
			f.put(restoreDir+`\savewarden-restore.json`, f.file(markerOf("synthetic-game")))
			f.mkdir(restoreDir + `\` + restored2)
		}, "destination-changed: another run marked this area, which now holds unexpected-entry " + restored2},
		{"operation journal", func(f *fakeFS) {
			f.put(restoreDir+`\savewarden-restore.json`, f.file(markerOf("synthetic-game")))
			f.mkdir(restoreDir + `\receipts`)
			f.mkdir(restoreDir + `\journal\` + restored2)
		}, "destination-changed: another run marked this area, which now holds pending-operation " + restored2},
	} {
		t.Run("validated meanwhile, "+tt.name, func(t *testing.T) {
			f := interruptedRestoreAt(t, "Close savewarden-restore.json.tmp-1", true)
			hooks := hookedRestore(f)
			var before string
			hooks.before["Open"] = func() { tt.change(f); before = everything(f) }
			result, err := restoreRecoverWith(f, hooks)
			if len(hooks.calls) != 0 || everything(f) != before {
				t.Fatalf("written: %q", hooks.calls)
			}
			if tt.mention == "" {
				if err != nil || result.Effect != sync.EffectNone {
					t.Fatalf("nothing to do: %+v, %v", result, err)
				}
				return
			}
			expectRestoreRefused(t, err, tt.mention)
		})
	}
	// Nine temporaries exhaust envelope E1: code 3, nothing written.
	f = restorable(t)
	f.mkdir(restoreDir)
	for k := 1; k <= backup.MaxTemporaries; k++ {
		f.put(fmt.Sprintf(`%s\savewarden-restore.json.tmp-%d`, restoreDir, k), f.file("{"))
	}
	before := everything(f)
	result, err = restoreRecoverWith(f, newFakeVaults(f))
	var intervention *sync.InterventionError
	if !errors.As(err, &intervention) || result.Effect != sync.EffectIntervention || everything(f) != before {
		t.Fatalf("envelope: %+v, %v", result, err)
	}
}

// ADR 0006 §11: a recovery that may write redoes the distinctions, the
// paths of its plan included, read in the area without the vault.
func TestRestoreRecoveryRedoesTheDistinctions(t *testing.T) {
	move := func(f *fakeFS, to string) {
		f.mkdir(to[:strings.LastIndex(to, `\`)]).children[to[strings.LastIndex(to, `\`)+1:]] = f.get(restoreDir)
		delete(f.get(`G:\Restores`).children, "synthetic")
	}
	tests := []struct {
		name    string
		to      string
		change  func(m *game.Manifest)
		mention string
	}{
		{"under the vault", vaultPath + `\inner\synthetic`, nil, "lies in a Savewarden vault or restore area"},
		{"in a source", installPath + `\saves\inner\synthetic`, nil, "overlaps the installation"},
		{"in a backed-up path", installPath + `\saves\inner\synthetic`, func(m *game.Manifest) { m.Installation.Path = `D:\Elsewhere` },
			"overlaps the backed-up source saves " + installPath + `\saves`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := interruptedRestoreAt(t, restoreMove, true)
			f.mkdir(`D:\Elsewhere\saves`)
			move(f, tt.to)
			m := manifest()
			if tt.change != nil {
				tt.change(&m)
			}
			before := everything(f)
			vaults := newFakeVaults(f)
			_, err := service(f, m).RestoreRecover(context.Background(), sync.RestoreRequest{ManifestPath: "m.yaml", Destination: tt.to},
				restoreWriter(vaults, restoreAt.Add(timeLater), "\x00\x00\x00\x00"))
			expectRestoreRefused(t, err, tt.mention)
			if len(vaults.sessions) != 0 || everything(f) != before {
				t.Fatal("a refused recovery wrote something")
			}
		})
	}
}

// Under the lock, the recovery reads the area again: an observation that
// changed since its plan is destination-changed, code 1, nothing recorded.
func TestRestoreRecoveryRereadsUnderLock(t *testing.T) {
	f := interruptedRestoreAt(t, restoreMove, true)
	hooks := hookedRestore(f)
	// The copy goes back under its incomplete name: (new, absent) becomes
	// (absent, new), another valid row with another action.
	hooks.before["Lock restore.lock"] = func() {
		area := f.get(restoreDir)
		area.children[restoreApplied+".incomplete"] = area.children[restoreApplied]
		delete(area.children, restoreApplied)
	}
	journal := restoreJournal(f, restoreApplied)
	_, err := restoreRecoverWith(f, hooks)
	expectRestoreRefused(t, err, "destination-changed: the restore area changed since the recovery plan")
	if restoreJournal(f, restoreApplied) != journal || f.get(restoreDir+`\receipts\`+restoreApplied+".json") != nil {
		t.Fatalf("a changed area was recovered: %s", restoreJournal(f, restoreApplied))
	}
}

// P1-01 of the protocol review: a receipt temporary is written only after
// publish-intent. Beside a journal that never reached it, it is an
// ambiguity, code 3, nothing written; after a rollback that followed
// publish-intent, it stays an accepted abandoned attempt.
func TestReceiptTemporaryNeedsPublishIntent(t *testing.T) {
	f := restorable(t)
	startRestore(t, f, restoreDir, restored2, restoreAt, backup.StepPlanned)
	f.put(restoreDir+`\receipts\`+restored2+".json.tmp-1", f.file("{"))
	before := everything(f)
	vaults := newFakeVaults(f)
	result, err := restoreRecoverWith(f, vaults)
	var intervention *sync.InterventionError
	if !errors.As(err, &intervention) || !strings.Contains(err.Error(), "never reached publish-intent") || result.Effect != sync.EffectIntervention ||
		len(vaults.sessions) != 0 || everything(f) != before {
		t.Fatalf("receipt temporary after planned: %+v, %v", result, err)
	}
	if p := restorePlanOf(t, f); !slices.Contains(restoreCauses(p), "journal-ambiguous receipts/"+restored2+".json.tmp-*") {
		t.Fatalf("plan: %q", restoreCauses(p))
	}
	// Written after the switch, then rolled back: accepted afterwards.
	f = interruptedRestoreAt(t, "NewFile receipts/"+restoreApplied+".json.tmp-1", true)
	f.get(restoreDir + `\` + restoreApplied + `\saves\1.save`).data = []byte("uno")
	var blocked *sync.BlockedError
	if _, err := restoreRecoverWith(f, newFakeVaults(f)); !errors.As(err, &blocked) || blocked.Code != sync.CodeVerificationMismatch {
		t.Fatalf("move back: %v", err)
	}
	if f.get(restoreDir+`\receipts\`+restoreApplied+".json.tmp-1") == nil || lastRestoreStep(f) != "rolled-back.rec" {
		t.Fatalf("journal %s", restoreJournal(f, restoreApplied))
	}
	if p := restorePlanOf(t, f); len(p.Interventions) != 0 {
		t.Fatalf("plan after the rollback: %q", restoreCauses(p))
	}
}

// P2-02 of the protocol review: the diagnostics of a restore never speak of
// current or staging.
func TestRestoreDiagnosticsNameTheRestore(t *testing.T) {
	f := interruptedRestoreAt(t, restoreMove, true)
	f.get(restoreDir + `\` + restoreApplied + `\saves\1.save`).data = []byte("uno")
	vaults := newFakeVaults(f)
	vaults.failWrite = func(call string) error {
		if strings.Contains(call, "-rolled-back.rec") {
			return errors.New("disk full")
		}
		return nil
	}
	_, err := restoreRecoverWith(f, vaults)
	if err == nil || !strings.Contains(err.Error(), "the restore was moved back to its incomplete copy but the journal is not closed") ||
		strings.Contains(err.Error(), "current") || strings.Contains(err.Error(), "staging") {
		t.Fatalf("move back not recorded: %v", err)
	}
	f = restorable(t)
	vaults = newFakeVaults(f)
	vaults.failWrite = func(call string) error {
		if strings.HasPrefix(call, "Write "+restoreApplied+".incomplete/") {
			return errors.New("disk full")
		}
		return nil
	}
	result, err := restoreApply(f, vaults)
	if err == nil || !strings.Contains(err.Error(), "incomplete copy "+restoreApplied+".incomplete/") || strings.Contains(err.Error(), "staging") ||
		result.Effect != sync.EffectStaged || lastRestoreStep(f) != "failed.rec" {
		t.Fatalf("write failure: %+v, %v", result, err)
	}
}

// A restore running in the area holds its lock: its pending operation is
// never taken for an interrupted one, code 1.
func TestRunningRestoreIsNotInterrupted(t *testing.T) {
	f := interruptedRestoreAt(t, restoreMove, true)
	f.get(restoreDir + `\restore.lock`).busy = true
	_, err := restoreRecoverPlan(f)
	expectRestoreRefused(t, err, "another restore is running in this area")
	f.get(restoreDir + `\restore.lock`).busy = false
	if rec, err := restoreRecoverPlan(f); err != nil || rec.Action != sync.RecoverRestorePublish {
		t.Fatalf("lock free: %q, %v", rec.Action, err)
	}
}

// Envelope E1 for a record: nine temporaries of the next record make the
// operation an ambiguity, code 3, nothing written.
func TestRestoreEnvelopeIsBounded(t *testing.T) {
	f := interruptedRestoreAt(t, "CreateDir journal/"+restoreApplied, true)
	writeAreaRecords(t, f, restoreDir, restoreApplied, restoreAt, nil, nil)
	for k := 1; k <= backup.MaxTemporaries; k++ {
		f.put(fmt.Sprintf(`%s\journal\%s\001-planned.rec.tmp-%d`, restoreDir, restoreApplied, k), f.file("{"))
	}
	before := everything(f)
	_, err := restoreRecoverWith(f, newFakeVaults(f))
	var intervention *sync.InterventionError
	if !errors.As(err, &intervention) || !strings.Contains(err.Error(), backup.ErrEnvelopeExhausted.Error()) || everything(f) != before {
		t.Fatalf("envelope: %v", err)
	}
}

// After published, any object of any type under P and I is classified by
// the handover, never blocking and never read; the same objects under the
// name of an operation that is not handed over are code 3.
func TestHandedOverObjectsOfAnyType(t *testing.T) {
	published := restoreDir + `\` + restored1
	for name, change := range map[string]func(f *fakeFS){
		"file": func(f *fakeFS) { f.put(published, f.file("x")) },
		"link": func(f *fakeFS) { f.put(published, f.link()) },
		"incomplete link": func(f *fakeFS) {
			delete(f.get(restoreDir).children, restored1)
			f.put(published+".incomplete", f.link())
		},
		"unreadable": func(f *fakeFS) { f.get(published).statErr = fs.ErrPermission },
	} {
		t.Run(name, func(t *testing.T) {
			f := restorable(t)
			publishRestore(t, f, restoreDir, restored1, restoreAt)
			change(f)
			f.reads = nil
			rec, err := restoreRecoverPlan(f)
			if err != nil || rec.Action != sync.RecoverNothing || len(rec.Restores) != 1 {
				t.Fatalf("handed over: %+v, %v", rec, err)
			}
			for _, read := range f.reads {
				if call, path, _ := strings.Cut(read, " "); strings.HasPrefix(path, published+`\`) || strings.HasPrefix(path, published+`.incomplete\`) ||
					(strings.HasPrefix(path, published) && call != "Lstat" && call != "Identity") {
					t.Fatalf("a handed-over object was read: %s", read)
				}
			}
			if result, err := restoreApplyWith(f, newFakeVaults(f), restoreDir, restoreAt.Add(time.Hour), "\x0b\x1c\x2d\x3e"); err != nil || result.Effect != sync.EffectPublished {
				t.Fatalf("a new restore beside it: %+v, %v", result, err)
			}
		})
	}
	// An I of another identity beside a closed operation: code 3.
	f := restorable(t)
	startRestore(t, f, restoreDir, restored2, restoreAt, backup.StepPlanned, backup.StepStagingIntent, backup.StepFailed)
	f.put(restoreDir+`\`+restored2+".incomplete", f.file("x"))
	if _, err := restoreRecoverPlan(f); err == nil || !strings.Contains(err.Error(), "intervention required") {
		t.Fatalf("closed operation: %v", err)
	}
}
