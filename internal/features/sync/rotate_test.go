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

// op3 is the rotation of op1: one hour later, another random part.
const op3 = "20261105T111500-2b3c4d5e"

const gameRel = "games/synthetic-game"

// Calls of a rotation of op1 by op3: M1, M2 and M3, and the commit of a
// record by its first temporary.
var (
	m1 = "Move " + gameRel + "/current " + gameRel + "/retained/" + op1
	m2 = "Move " + gameRel + "/staging/" + op3 + " " + gameRel + "/current"
	m3 = "Move " + gameRel + "/retained/" + op1 + " " + gameRel + "/current"
)

func commitOf(op string, seq int, step backup.Step) string {
	name := gameRel + "/journal/" + op + "/" + backup.RecordName(seq, step)
	return "Move " + name + ".tmp-1 " + name
}

// generation publishes op1 by a real first publication, then changes the
// source: a rotation then has a new file to publish.
func generation(t *testing.T) *fakeFS {
	t.Helper()
	f := world()
	if r := apply(f, vaultPath); r.err != nil || r.result.Effect != sync.EffectPublished {
		t.Fatalf("first publication: %+v, %v", r.result, r.err)
	}
	f.put(installPath+`\saves\new.save`, f.file("new"))
	return f
}

// rotateWith runs the rotation of op1 by op3 through vaults.
func rotateWith(ctx context.Context, f *fakeFS, vaults sync.Vaults) (sync.Result, error) {
	return service(f, manifest()).Apply(ctx, sync.Request{ManifestPath: "m.yaml", VaultPath: vaultPath},
		sync.Writer{Vaults: vaults, Clock: &fakeClock{now: at.Add(timeLater)}, Random: strings.NewReader("\x2b\x3c\x4d\x5e"), ToolVersion: "dev"})
}

// rotated checks what a completed rotation of old by next leaves: the new
// generation as current, the old one retained and identical to its
// receipt, the staging gone, a receipt naming old as previous, and a plan
// that finds the vault up to date.
func rotated(t *testing.T, f *fakeFS, old, next string) {
	t.Helper()
	game := f.get(gameDir)
	retained := game.children["retained"].children[old]
	if retained == nil {
		t.Fatalf("nothing retained for %s: %v", old, names(game.children["retained"]))
	}
	oldSaves := retained.children["saves"]
	if string(oldSaves.children["1.save"].data) != "one" || oldSaves.children["new.save"] != nil {
		t.Fatalf("retained %s: %v", old, names(oldSaves))
	}
	if string(game.children["current"].children["saves"].children["new.save"].data) != "new" {
		t.Fatalf("current: %v", names(game.children["current"].children["saves"]))
	}
	if len(game.children["staging"].children) != 0 {
		t.Fatalf("staging left %v", names(game.children["staging"]))
	}
	want := "001-planned.rec,002-staging-intent.rec,003-staged.rec,004-verified.rec,005-retire-intent.rec,006-promote-intent.rec,007-published.rec"
	if got := strings.Join(names(game.children["journal"].children[next]), ","); got != want {
		t.Fatalf("journal %s", got)
	}
	receipt, err := backup.DecodeReceipt(game.children["receipts"].children[next+".json"].data)
	if err != nil || receipt.Previous != old || receipt.PublishedAt == nil || receipt.Recovered() {
		t.Fatalf("receipt %+v, %v", receipt, err)
	}
	if p := plan(t, f, manifest(), vaultPath); p.Outcome() != "up to date" || p.Current != next {
		t.Fatalf("plan after the rotation: %q, current %q, %q %q", p.Outcome(), p.Current, causes(p), findings(p))
	}
}

// hookVaults runs a change of the vault once, before or after the named
// vault call, and refuses the moves of the named sources: it stands for
// another process or for a file kept open.
type hookVaults struct {
	*fakeVaults
	before, after map[string]func()
	deny          map[string]error
	// identity runs a change before the n-th identity check of a name.
	identity map[string]identityHook
	checked  map[string]int
}

type identityHook struct {
	n      int
	change func()
}

func hooked(f *fakeFS) *hookVaults {
	return &hookVaults{fakeVaults: newFakeVaults(f), before: map[string]func(){}, after: map[string]func(){}, deny: map[string]error{},
		identity: map[string]identityHook{}, checked: map[string]int{}}
}

func (h *hookVaults) Open(path string) (sync.Vault, error) {
	v, err := h.fakeVaults.Open(path)
	if err != nil {
		return nil, err
	}
	return &hookVault{Vault: v, h: h}, nil
}

func (h *hookVaults) run(hooks map[string]func(), call string) {
	if hook := hooks[call]; hook != nil {
		delete(hooks, call)
		hook()
	}
}

type hookVault struct {
	sync.Vault
	h *hookVaults
}

func (v *hookVault) Move(from, to string) error {
	call := "Move " + from + " " + to
	v.h.run(v.h.before, call)
	if err := v.h.deny[from]; err != nil {
		return fmt.Errorf("move %s: %w", from, err)
	}
	err := v.Vault.Move(from, to)
	if err == nil {
		v.h.run(v.h.after, call)
	}
	return err
}

func (v *hookVault) Identity(name string) (backup.FileID, error) {
	v.h.checked[name]++
	if hook, ok := v.h.identity[name]; ok && hook.n == v.h.checked[name] {
		delete(v.h.identity, name)
		hook.change()
	}
	return v.Vault.Identity(name)
}

// afterRetirement is the identity check of name by point 5, after the
// post-check of M1: a rotation checks current there for the fourth time,
// retained\<prev-op> for the third.
func afterRetirement(name string) int {
	if name == gameRel+"/current" {
		return 4
	}
	return 3
}

func (v *hookVault) Release(dir string) error {
	v.h.run(v.h.before, "Release "+dir)
	return v.Vault.Release(dir)
}

// journalOf lists the record files of op.
func journalOf(f *fakeFS, op string) string {
	journal := f.get(gameDir + `\journal\` + op)
	if journal == nil {
		return ""
	}
	return strings.Join(names(journal), ",")
}

func lastRecord(f *fakeFS, op string) string {
	all := strings.Split(journalOf(f, op), ",")
	return all[len(all)-1]
}

// expectIntervention requires code 3 with the journal of op3 ending at last.
func expectIntervention(t *testing.T, f *fakeFS, result sync.Result, err error, last, mention string) {
	t.Helper()
	var intervention *sync.InterventionError
	if !errors.As(err, &intervention) || result.Effect != sync.EffectIntervention || !strings.Contains(err.Error(), mention) {
		t.Fatalf("result %+v, %v; want an intervention mentioning %q", result, err, mention)
	}
	if got := lastRecord(f, op3); got != last {
		t.Fatalf("journal ends at %q, want %q: %s", got, last, journalOf(f, op3))
	}
}

func TestExistingCurrentIsRotatedByARealPublication(t *testing.T) {
	f := generation(t)
	old := f.get(gameDir + `\current`)
	result, err := rotateWith(context.Background(), f, newFakeVaults(f))
	if err != nil || result.Effect != sync.EffectPublished || result.State() != "published "+op3 || result.Files != 3 {
		t.Fatalf("result %+v, %v", result, err)
	}
	if f.get(gameDir+`\retained\`+op1) != old {
		t.Fatal("the old current was not renamed into retained")
	}
	rotated(t, f, op1, op3)
	planned, err := backup.DecodeRecord(f.get(gameDir + `\journal\` + op3 + `\001-planned.rec`).data)
	if err != nil {
		t.Fatal(err)
	}
	if values, err := planned.Values(); err != nil || values.Previous != op1 || values.Current != fakeID(old) {
		t.Fatalf("planned %+v, %v", values, err)
	}
}

// R2: a first publication and a rotation both leave a current that the
// next plan recognises, B2b's journals included.
func TestPreviousGenerationIsRecognized(t *testing.T) {
	f := world()
	if r := apply(f, vaultPath); r.err != nil {
		t.Fatal(r.err)
	}
	if p := plan(t, f, manifest(), vaultPath); p.Outcome() != "up to date" || p.Current != op1 || len(p.Interventions) != 0 {
		t.Fatalf("after a first publication: %q %q", p.Outcome(), causes(p))
	}
	f = world()
	markVault(t, f, vaultPath)
	publish(t, f, manifest(), op2, "")
	if p := plan(t, f, manifest(), vaultPath); p.Outcome() != "up to date" || p.Current != op2 {
		t.Fatalf("after a synthetic B2b publication: %q %q %q", p.Outcome(), causes(p), findings(p))
	}
}

func TestPreviousGenerationIsRecognizedAfterRotation(t *testing.T) {
	f := generation(t)
	if _, err := rotateWith(context.Background(), f, newFakeVaults(f)); err != nil {
		t.Fatal(err)
	}
	f.put(installPath+`\saves\newer.save`, f.file("newer"))
	next := "20261105T121500-3c4d5e6f"
	result, err := service(f, manifest()).Apply(context.Background(), sync.Request{ManifestPath: "m.yaml", VaultPath: vaultPath},
		sync.Writer{Vaults: newFakeVaults(f), Clock: &fakeClock{now: at.Add(2 * timeLater)}, Random: strings.NewReader("\x3c\x4d\x5e\x6f"), ToolVersion: "dev"})
	if err != nil || result.Effect != sync.EffectPublished || result.Operation != next {
		t.Fatalf("second rotation: %+v, %v", result, err)
	}
	if p := plan(t, f, manifest(), vaultPath); p.Outcome() != "up to date" || p.Current != next {
		t.Fatalf("plan: %q %q", p.Outcome(), causes(p))
	}
}

// A3 then recovery rebuilds the receipt of op1: its generation is still
// recognised, and rotated.
func TestPreviousGenerationFromReconstructedReceipt(t *testing.T) {
	r := crashAt(t, "Move games/synthetic-game/staging/"+op1+" games/synthetic-game/current", true)
	if _, err := recoverWith(r.f, newFakeVaults(r.f)); err != nil {
		t.Fatal(err)
	}
	receipt, err := backup.DecodeReceipt(r.f.get(gameDir + `\receipts\` + op1 + `.json`).data)
	if err != nil || !receipt.Recovered() {
		t.Fatalf("receipt %+v, %v", receipt, err)
	}
	r.f.put(installPath+`\saves\new.save`, r.f.file("new"))
	if result, err := rotateWith(context.Background(), r.f, newFakeVaults(r.f)); err != nil || result.Effect != sync.EffectPublished {
		t.Fatalf("rotation: %+v, %v", result, err)
	}
	rotated(t, r.f, op1, op3)
}

// copyCurrent replaces current by an identical copy, as a copied or moved
// vault would have it: same content, another identity.
func copyCurrent(f *fakeFS) {
	game := f.get(gameDir)
	game.children["current"] = copyTree(f, game.children["current"])
}

// A copied current is refused in code 3 without any write, by a
// diagnostic that claims no corruption and adopts no identity.
func TestCurrentWithAnotherIdentityIsNotRecognized(t *testing.T) {
	for _, changed := range []bool{false, true} {
		f := world()
		if r := apply(f, vaultPath); r.err != nil {
			t.Fatal(r.err)
		}
		copyCurrent(f)
		if changed {
			f.put(installPath+`\saves\new.save`, f.file("new"))
		}
		p := plan(t, f, manifest(), vaultPath)
		if strings.Join(causes(p), ",") != "unexpected-entry "+gameRel+"/current" || p.Outcome() != "intervention required" {
			t.Fatalf("source changed %v: plan %q %q", changed, p.Outcome(), causes(p))
		}
		detail := p.Interventions[0].Detail
		if !strings.Contains(detail, "copied or moved vault is not supported yet") || !strings.Contains(detail, "nothing was written") ||
			strings.Contains(strings.ToLower(detail), "corrupt") || strings.Contains(strings.ToLower(detail), "compromis") {
			t.Fatalf("diagnostic %q", detail)
		}
		before := snapshot(f, `F:\`)
		r := apply(f, vaultPath)
		var intervention *sync.InterventionError
		if !errors.As(r.err, &intervention) || len(r.vaults.calls) != 0 || snapshot(f, `F:\`) != before {
			t.Fatalf("source changed %v: apply %+v, %v, %d calls", changed, r.result, r.err, len(r.vaults.calls))
		}
	}
}

// The same identity with another content is current-modified, code 2.
func TestCurrentWithAlteredContentIsNotRecognized(t *testing.T) {
	f := generation(t)
	f.get(gameDir + `\current\saves\1.save`).data = []byte("ONE")
	p := plan(t, f, manifest(), vaultPath)
	if len(p.Interventions) != 0 || p.Outcome() != "blocked" || !strings.Contains(strings.Join(findings(p), ","), "blocking current-modified saves 1.save") {
		t.Fatalf("plan %q %q %q", p.Outcome(), causes(p), findings(p))
	}
	before := snapshot(f, `F:\`)
	r := apply(f, vaultPath)
	var blocked *sync.BlockedError
	if !errors.As(r.err, &blocked) || len(r.vaults.calls) != 0 || snapshot(f, `F:\`) != before {
		t.Fatalf("apply %+v, %v", r.result, r.err)
	}
}

// The plan applies the recognition of --apply: another identity is
// unexpected-entry whatever the content; a receipt that is missing keeps
// current-modified, as in B2b.
func TestPlanReportsACurrentNotPublishedByItsOperation(t *testing.T) {
	f := world()
	if r := apply(f, vaultPath); r.err != nil {
		t.Fatal(r.err)
	}
	copyCurrent(f)
	f.get(gameDir + `\current\saves\1.save`).data = []byte("ONE")
	if p := plan(t, f, manifest(), vaultPath); strings.Join(causes(p), ",") != "unexpected-entry "+gameRel+"/current" {
		t.Fatalf("copied and altered: %q", causes(p))
	}
	f = world()
	if r := apply(f, vaultPath); r.err != nil {
		t.Fatal(r.err)
	}
	delete(f.get(gameDir).children, "receipts")
	if p := plan(t, f, manifest(), vaultPath); len(p.Interventions) != 0 || strings.Join(findings(p), ",") != "blocking current-modified  current" {
		t.Fatalf("receipt missing: %q %q", causes(p), findings(p))
	}
}

// A2, between the plan and the lock: a content change is current-modified,
// code 2, before any journal.
func TestRotationRefusesCurrentChangedBeforeTheLock(t *testing.T) {
	f := generation(t)
	vaults := appearing{newFakeVaults(f), func() { f.get(gameDir + `\current\saves\1.save`).data = []byte("ONE") }}
	result, err := rotateWith(context.Background(), f, vaults)
	var blocked *sync.BlockedError
	if !errors.As(err, &blocked) || blocked.Code != sync.CodeCurrentModified || journalOf(f, op3) != "" {
		t.Fatalf("result %+v, %v, journal %q", result, err, journalOf(f, op3))
	}
	if result.Effect != sync.EffectNone && result.Effect != sync.EffectMetadata {
		t.Fatalf("effect %v", result.State())
	}
}

// A2, under lock before planned: current replaced, by a copy or a
// junction, is code 3 before any journal.
func TestRotationRefusesAnUnrecognizedCurrentUnderLock(t *testing.T) {
	for name, change := range map[string]func(f *fakeFS){
		"copy":                copyCurrent,
		"junction":            func(f *fakeFS) { f.get(gameDir).children["current"] = f.link() },
		"unreadable identity": func(f *fakeFS) { f.get(gameDir + `\current`).busy = true },
		"current gone":        func(f *fakeFS) { delete(f.get(gameDir).children, "current") },
	} {
		f := generation(t)
		result, err := rotateWith(context.Background(), f, appearing{newFakeVaults(f), func() { change(f) }})
		var intervention *sync.InterventionError
		if !errors.As(err, &intervention) || result.Effect != sync.EffectIntervention || journalOf(f, op3) != "" ||
			!strings.HasPrefix(result.State(), "intervention required for the vault: ") {
			t.Fatalf("%s: result %+v, %v, journal %q, state %q", name, result, err, journalOf(f, op3), result.State())
		}
	}
	// Any other change of the vault under lock stays vault-changed, code 1.
	f := generation(t)
	result, err := rotateWith(context.Background(), f, appearing{newFakeVaults(f), func() { writeJournal(t, f, op2, backup.StepPlanned) }})
	var intervention *sync.InterventionError
	if err == nil || errors.As(err, &intervention) || !strings.Contains(err.Error(), "vault-changed") || journalOf(f, op3) != "" {
		t.Fatalf("pending operation under lock: %+v, %v", result, err)
	}
}

// A2, point 3: just before retire-intent, a content change closes the
// operation as failed with current-modified, code 2, nothing moved.
func TestRotationFailsWhenCurrentChangesBeforeRetirement(t *testing.T) {
	f := generation(t)
	vaults := hooked(f)
	vaults.after[commitOf(op3, 4, backup.StepVerified)] = func() { f.get(gameDir + `\current\saves\1.save`).data = []byte("ONE") }
	result, err := rotateWith(context.Background(), f, vaults)
	var blocked *sync.BlockedError
	if !errors.As(err, &blocked) || blocked.Code != sync.CodeCurrentModified || result.Effect != sync.EffectStaged ||
		lastRecord(f, op3) != "005-failed.rec" {
		t.Fatalf("result %+v, %v, journal %s", result, err, journalOf(f, op3))
	}
	if f.get(gameDir+`\staging\`+op3) == nil || f.get(gameDir+`\retained`) != nil {
		t.Fatal("staging lost, or something retained")
	}
}

func TestRotationStopsWhenCurrentIsReplacedBeforeRetirement(t *testing.T) {
	f := generation(t)
	vaults := hooked(f)
	vaults.after[commitOf(op3, 4, backup.StepVerified)] = func() { copyCurrent(f) }
	result, err := rotateWith(context.Background(), f, vaults)
	expectIntervention(t, f, result, err, "004-verified.rec", "not the directory")
	if f.get(gameDir+`\retained`) != nil {
		t.Fatal("something was retained")
	}
}

// Point 4: a current swapped between retire-intent and M1 fails the
// pre-check; nothing is moved.
func TestRetirementPrecheckCatchesASwap(t *testing.T) {
	f := generation(t)
	vaults := hooked(f)
	vaults.before["Release "+gameRel+"/current"] = func() { copyCurrent(f) }
	result, err := rotateWith(context.Background(), f, vaults)
	expectIntervention(t, f, result, err, "005-retire-intent.rec", "not the directory")
	if f.get(gameDir+`\retained\`+op1) != nil || f.get(gameDir+`\current`) == nil {
		t.Fatal("something was moved")
	}
	// A state the pre-check cannot confirm is never recorded as rolled back.
	for name, change := range map[string]func(f *fakeFS){
		"current gone":         func(f *fakeFS) { delete(f.get(gameDir).children, "current") },
		"current a junction":   func(f *fakeFS) { f.get(gameDir).children["current"] = f.link() },
		"junction at retained": func(f *fakeFS) { f.put(gameDir+`\retained\`+op1, f.link()) },
	} {
		f := generation(t)
		vaults := hooked(f)
		vaults.before["Release "+gameRel+"/current"] = func() { change(f) }
		result, err := rotateWith(context.Background(), f, vaults)
		expectIntervention(t, f, result, err, "005-retire-intent.rec", "before")
		if strings.Contains(journalOf(f, op3), "rolled-back") {
			t.Fatalf("%s: recorded as rolled back", name)
		}
	}
}

// F2: a check before M3 that cannot be read spends no rollback-failed.
func TestRestorationPrecheckSpendsNoRollbackFailed(t *testing.T) {
	f := generation(t)
	vaults := hooked(f)
	vaults.deny[gameRel+"/staging/"+op3] = fs.ErrPermission
	vaults.after[commitOf(op3, 7, backup.StepRestoreIntent)] = func() { f.put(gameDir+`\current`, f.link()) }
	result, err := rotateWith(context.Background(), f, vaults)
	expectIntervention(t, f, result, err, "007-restore-intent.rec", "before the rename")
	if f.get(gameDir+`\retained\`+op1) == nil {
		t.Fatal("the old generation moved")
	}
}

// A retained\<prev-op> that already stands where M1 would retire current
// is refused by the plan itself: code 3, no journal, nothing moved.
func TestRotationRefusesAnExistingRetained(t *testing.T) {
	f := generation(t)
	f.mkdir(gameDir + `\retained\` + op1)
	if p := plan(t, f, manifest(), vaultPath); strings.Join(causes(p), ",") != "unexpected-entry "+gameRel+"/retained/"+op1 {
		t.Fatalf("plan %q", causes(p))
	}
	before := snapshot(f, `F:\`)
	result, err := rotateWith(context.Background(), f, newFakeVaults(f))
	var intervention *sync.InterventionError
	if !errors.As(err, &intervention) || !strings.Contains(err.Error(), "unexpected-entry "+gameRel+"/retained/"+op1) ||
		journalOf(f, op3) != "" || snapshot(f, `F:\`) != before {
		t.Fatalf("result %+v, %v, journal %q", result, err, journalOf(f, op3))
	}
}

// R3, the window between M1 and M2: retained changed or replaced, or an
// object under current, stops in code 3 with nothing recorded or moved.
func TestPromotionRefusedWhenRetainedChangedAfterRetirement(t *testing.T) {
	for name, change := range map[string]func(f *fakeFS){
		"file modified": func(f *fakeFS) { f.get(gameDir + `\retained\` + op1 + `\saves\1.save`).data = []byte("ONE") },
		"file added":    func(f *fakeFS) { f.put(gameDir+`\retained\`+op1+`\saves\extra.save`, f.file("x")) },
		"file removed":  func(f *fakeFS) { delete(f.get(gameDir+`\retained\`+op1+`\saves`).children, "1.save") },
	} {
		f := generation(t)
		vaults := hooked(f)
		vaults.after[m1] = func() { change(f) }
		result, err := rotateWith(context.Background(), f, vaults)
		expectIntervention(t, f, result, err, "005-retire-intent.rec", "no longer matches the receipt of "+op1)
		if f.get(gameDir+`\current`) != nil || f.get(gameDir+`\staging\`+op3) == nil {
			t.Fatalf("%s: promoted or restored", name)
		}
	}
}

func TestPromotionRefusedWhenRetainedReplacedAfterRetirement(t *testing.T) {
	f := generation(t)
	vaults := hooked(f)
	name := gameRel + "/retained/" + op1
	vaults.identity[name] = identityHook{afterRetirement(name), func() {
		retained := f.get(gameDir + `\retained`)
		retained.children[op1] = copyTree(f, retained.children[op1])
	}}
	result, err := rotateWith(context.Background(), f, vaults)
	expectIntervention(t, f, result, err, "005-retire-intent.rec", "not the directory")
	if f.get(gameDir+`\current`) != nil {
		t.Fatal("promoted or restored")
	}
}

func TestPromotionRefusedWhenCurrentReappearsAfterRetirement(t *testing.T) {
	f := generation(t)
	vaults := hooked(f)
	var appeared *node
	name := gameRel + "/current"
	vaults.identity[name] = identityHook{afterRetirement(name), func() { appeared = f.mkdir(gameDir + `\current`) }}
	result, err := rotateWith(context.Background(), f, vaults)
	expectIntervention(t, f, result, err, "005-retire-intent.rec", "appeared under current")
	if f.get(gameDir+`\current`) != appeared || len(appeared.children) != 0 || f.get(gameDir+`\retained\`+op1) == nil {
		t.Fatal("the object under current was overwritten, or retained moved")
	}
}

// A refused M2 is followed by a new observation: an altered retained is
// never restored.
func TestFailedPromotionDoesNotRestoreAnAlteredRetained(t *testing.T) {
	f := generation(t)
	vaults := hooked(f)
	vaults.deny[gameRel+"/staging/"+op3] = fs.ErrPermission
	vaults.after[commitOf(op3, 6, backup.StepPromoteIntent)] = func() {
		f.get(gameDir + `\retained\` + op1 + `\saves\1.save`).data = []byte("ONE")
	}
	result, err := rotateWith(context.Background(), f, vaults)
	expectIntervention(t, f, result, err, "006-promote-intent.rec", "nothing restored")
	if f.get(gameDir+`\current`) != nil {
		t.Fatal("an altered generation was restored")
	}
}

func TestRetirementRetriesSharingViolations(t *testing.T) {
	f := generation(t)
	vaults := newFakeVaults(f)
	vaults.busy[gameRel+"/current"] = 3
	if result, err := rotateWith(context.Background(), f, vaults); err != nil || result.Effect != sync.EffectPublished {
		t.Fatalf("result %+v, %v", result, err)
	}
	rotated(t, f, op1, op3)
}

// A file kept open in current refuses M1 with access denied: rolled back
// at once, code 1, current unchanged.
func TestRetirementRefusedByAnOpenFileRollsBack(t *testing.T) {
	f := generation(t)
	old := f.get(gameDir + `\current`)
	vaults := hooked(f)
	vaults.deny[gameRel+"/current"] = fs.ErrPermission
	result, err := rotateWith(context.Background(), f, vaults)
	var intervention *sync.InterventionError
	if err == nil || errors.As(err, &intervention) || result.Effect != sync.EffectRolledBack ||
		result.State() != "rolled back "+op3+"; current unchanged" || lastRecord(f, op3) != "006-rolled-back.rec" {
		t.Fatalf("result %+v, %v, journal %s", result, err, journalOf(f, op3))
	}
	if f.get(gameDir+`\current`) != old || f.get(gameDir+`\retained\`+op1) != nil {
		t.Fatal("current moved")
	}
	if p := plan(t, f, manifest(), vaultPath); len(p.Interventions) != 0 || p.Outcome() != "changes" || p.Current != op1 {
		t.Fatalf("plan after the rollback: %q %q", p.Outcome(), causes(p))
	}
}

// M2 refused: verified restoration, restore-intent, M3, rolled-back, code 1.
func TestPromotionRefusedRestoresCurrent(t *testing.T) {
	f := generation(t)
	old := f.get(gameDir + `\current`)
	vaults := hooked(f)
	vaults.deny[gameRel+"/staging/"+op3] = fs.ErrPermission
	result, err := rotateWith(context.Background(), f, vaults)
	var intervention *sync.InterventionError
	if err == nil || errors.As(err, &intervention) || result.Effect != sync.EffectRolledBack {
		t.Fatalf("result %+v, %v", result, err)
	}
	if got := journalOf(f, op3); !strings.HasSuffix(got, "005-retire-intent.rec,006-promote-intent.rec,007-restore-intent.rec,008-rolled-back.rec") {
		t.Fatalf("journal %s", got)
	}
	if f.get(gameDir+`\current`) != old || f.get(gameDir+`\retained\`+op1) != nil || f.get(gameDir+`\staging\`+op3) == nil {
		t.Fatal("current not restored, or staging lost")
	}
	if p := plan(t, f, manifest(), vaultPath); len(p.Interventions) != 0 || p.Current != op1 {
		t.Fatalf("plan after the rollback: %q %q", p.Outcome(), causes(p))
	}
}

// M2 then M3 refused: rollback-failed, code 3, both generations intact.
func TestRestorationRefusedIsRecordedOnce(t *testing.T) {
	f := generation(t)
	vaults := hooked(f)
	vaults.deny[gameRel+"/staging/"+op3] = fs.ErrPermission
	vaults.deny[gameRel+"/retained/"+op1] = fs.ErrPermission
	result, err := rotateWith(context.Background(), f, vaults)
	expectIntervention(t, f, result, err, "008-rollback-failed.rec", "the old generation is intact")
	if f.get(gameDir+`\retained\`+op1+`\saves\1.save`) == nil || f.get(gameDir+`\staging\`+op3+`\saves\new.save`) == nil {
		t.Fatal("a generation was lost")
	}
	if strings.Count(journalOf(f, op3), "rollback-failed") != 1 {
		t.Fatalf("journal %s", journalOf(f, op3))
	}
}

// A current that appears after promote-intent makes M2 refuse; it is never
// overwritten, and the old generation is not put back over it.
func TestCurrentReappearingBetweenMovesIsNeverOverwritten(t *testing.T) {
	f := generation(t)
	vaults := hooked(f)
	var appeared *node
	vaults.before["Release "+gameRel+"/staging/"+op3] = func() { appeared = f.mkdir(gameDir + `\current`) }
	result, err := rotateWith(context.Background(), f, vaults)
	expectIntervention(t, f, result, err, "006-promote-intent.rec", "nothing restored")
	if f.get(gameDir+`\current`) != appeared || f.get(gameDir+`\retained\`+op1) == nil || f.get(gameDir+`\staging\`+op3) == nil {
		t.Fatal("something was overwritten or moved")
	}
	for _, call := range vaults.calls {
		if call == m3 {
			t.Fatal("a restoration was attempted")
		}
	}
	if p := plan(t, f, manifest(), vaultPath); strings.Contains(p.Interventions[0].Detail, "current published") {
		t.Fatalf("a foreign current is announced as published: %q", p.Interventions[0].Detail)
	}
}

// A staging swapped just before M2 is promoted under the right name with
// the wrong identity: the post-check stops, no other move.
func TestPromotionPostcheckMismatchStops(t *testing.T) {
	f := generation(t)
	vaults := hooked(f)
	vaults.before[m2] = func() {
		parent := f.get(gameDir + `\staging`)
		parent.children[op3+"-real"] = parent.children[op3]
		parent.children[op3] = f.dir()
	}
	result, err := rotateWith(context.Background(), f, vaults)
	expectIntervention(t, f, result, err, "006-promote-intent.rec", "after the rename")
	for _, call := range vaults.calls {
		if call == m3 {
			t.Fatal("a restoration was attempted")
		}
	}
	if f.get(gameDir+`\receipts\`+op3+`.json`) != nil {
		t.Fatal("a receipt was written")
	}
}

// From retire-intent on, a cancellation waits for the end of the switch.
func TestCancellationDuringRotationIsDeferred(t *testing.T) {
	f := generation(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	vaults := hooked(f)
	vaults.after[commitOf(op3, 5, backup.StepRetireIntent)] = cancel
	vaults.busy[gameRel+"/current"] = 2
	if result, err := rotateWith(ctx, f, vaults); err != nil || result.Effect != sync.EffectPublished {
		t.Fatalf("result %+v, %v", result, err)
	}
	rotated(t, f, op1, op3)
}

// A4: the old current is only renamed, so a rotation needs the space of the
// whole new generation plus the margin, as a first publication.
func TestRotationSpaceNeedsTheWholeGeneration(t *testing.T) {
	f := generation(t)
	p := plan(t, f, manifest(), vaultPath)
	if want := uint64(64<<20) + 9; p.Needed != want {
		t.Fatalf("needed %d, want %d", p.Needed, want)
	}
	f.volumeOf[`F:\`] = backup.Volume{FileSystem: "NTFS", Drive: backup.DriveFixed, Available: p.Needed - 1}
	if p := plan(t, f, manifest(), vaultPath); !strings.Contains(strings.Join(findings(p), ","), "blocking insufficient-space") {
		t.Fatalf("findings %q", findings(p))
	}
}

// A4: generations accumulate in retained, none deleted or rewritten.
func TestRetainedGenerationsAccumulate(t *testing.T) {
	f := generation(t)
	if _, err := rotateWith(context.Background(), f, newFakeVaults(f)); err != nil {
		t.Fatal(err)
	}
	first := snapshot(f, gameDir+`\retained\`+op1)
	f.put(installPath+`\saves\newer.save`, f.file("newer"))
	_, err := service(f, manifest()).Apply(context.Background(), sync.Request{ManifestPath: "m.yaml", VaultPath: vaultPath},
		sync.Writer{Vaults: newFakeVaults(f), Clock: &fakeClock{now: at.Add(2 * timeLater)}, Random: strings.NewReader("\x3c\x4d\x5e\x6f"), ToolVersion: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	retained := f.get(gameDir + `\retained`)
	if got := strings.Join(names(retained), ","); got != op1+","+op3 {
		t.Fatalf("retained %s", got)
	}
	if snapshot(f, gameDir+`\retained\`+op1) != first {
		t.Fatal("an older generation changed")
	}
	if retained.children[op3].children["saves"].children["new.save"] == nil {
		t.Fatal("the second generation was not retained whole")
	}
}
