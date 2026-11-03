package sync_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/features/inspect"
	"github.com/savewarden/savewarden-cli/internal/features/sync"
)

// restoreApplied is the operation of a restore run at restoreAt by
// restoreWriter with the random part 9a8b7c6d.
const restoreApplied = "20261107T101500-9a8b7c6d"

func restoreWriter(vaults sync.Vaults, clock time.Time, random string) sync.Writer {
	return sync.Writer{Vaults: vaults, Clock: &fakeClock{now: clock}, Random: strings.NewReader(random), ToolVersion: "dev"}
}

func restoreApplyWith(f *fakeFS, vaults sync.Vaults, destination string, clock time.Time, random string) (sync.RestoreResult, error) {
	return service(f, manifest()).RestoreApply(context.Background(), restoreRequest(destination), restoreWriter(vaults, clock, random))
}

func restoreApply(f *fakeFS, vaults sync.Vaults) (sync.RestoreResult, error) {
	return restoreApplyWith(f, vaults, restoreDir, restoreAt, "\x9a\x8b\x7c\x6d")
}

// restoreJournal lists the files of the journal of op in the area.
func restoreJournal(f *fakeFS, op string) string {
	journal := f.get(restoreDir + `\journal\` + op)
	if journal == nil {
		return ""
	}
	return strings.Join(names(journal), ",")
}

// outsideTheArea lists every volume but the content of the area.
func outsideTheArea(f *fakeFS) string {
	var lines []string
	for _, line := range strings.Split(everything(f), "\n") {
		// snapshot names a child of a volume root with a doubled backslash.
		if !strings.HasPrefix(line, `G:\\Restores\synthetic`) {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// ADR 0006 §11: a restore copies the snapshot exactly, with its dates, into
// a new marked area, and publishes it with a receipt chained to the
// snapshot and to its generation; nothing else is written anywhere.
func TestRestoreCopiesTheSnapshotExactly(t *testing.T) {
	f := restorable(t)
	snapReceipt := f.get(gameDir + `\snapshots\receipts\` + snapApplied + ".json").data
	sr, err := backup.DecodeSnapshotReceipt(snapReceipt)
	if err != nil {
		t.Fatal(err)
	}
	outside := outsideTheArea(f)
	result, err := restoreApply(f, newFakeVaults(f))
	if err != nil || result.Effect != sync.EffectPublished || result.Operation != restoreApplied || result.Files != 2 || result.Bytes != 6 {
		t.Fatalf("restore: %+v, %v", result, err)
	}
	published := restoreDir + `\` + restoreApplied
	if result.State() != "published restore "+restoreApplied || result.Restore != published ||
		result.Receipt != restoreDir+`\receipts\`+restoreApplied+".json" {
		t.Fatalf("state %q, restore %q, receipt %q", result.State(), result.Restore, result.Receipt)
	}
	for _, path := range []string{`saves\1.save`, `saves\Slot 1\2.save`} {
		a, b := f.get(snapDir+`\`+path), f.get(published+`\`+path)
		if b == nil || !bytes.Equal(a.data, b.data) || !a.modTime.Equal(b.modTime) || a.id == b.id {
			t.Fatalf("%s: not an independent copy", path)
		}
	}
	if f.get(published+".incomplete") != nil || f.get(published+`\saves\Slot 1`) == nil {
		t.Fatal("the restore must be the renamed copy, with its directories")
	}
	if got := restoreJournal(f, restoreApplied); got != "001-planned.rec,002-staging-intent.rec,003-staged.rec,004-verified.rec,005-publish-intent.rec,006-published.rec" {
		t.Fatalf("journal %s", got)
	}
	r, err := backup.DecodeRestoreReceipt(f.get(result.Receipt).data)
	if err != nil || r.Restore != restoreApplied || r.Snapshot != (backup.SnapshotSource{Operation: snapApplied, ReceiptSHA256: sha(snapReceipt)}) ||
		r.Source != sr.Source || r.Source.Operation != op1 || !sameContent(t, r.Sources, sr.Sources) || r.PublishedAt == nil || r.GameID != "synthetic-game" {
		t.Fatalf("receipt %+v, %v", r, err)
	}
	marker, err := backup.DecodeRestoreAreaMarker(f.get(restoreDir + `\savewarden-restore.json`).data)
	if err != nil || marker.GameID != "synthetic-game" || f.get(restoreDir+`\restore.lock`) == nil {
		t.Fatalf("marker %+v, %v", marker, err)
	}
	if outsideTheArea(f) != outside {
		t.Fatal("something was written outside the area")
	}
	p := restorePlanOf(t, f)
	if len(p.Restores) != 1 || p.Restores[0].State() != "present" || p.Restores[0].Snapshot != snapApplied ||
		!slices.Equal(restoreFindings(p), []string{"info snapshot-already-restored " + restoreApplied}) {
		t.Fatalf("plan after: %+v, %q", p.Restores, restoreFindings(p))
	}
}

// ADR 0006 §6 and §11: the only session is opened on the area, never on
// the vault, even when both are on one volume; every write is below it.
func TestRestoreNeverOpensTheVaultForWriting(t *testing.T) {
	for _, destination := range []string{restoreDir, `F:\Restores\synthetic`} {
		f := restorable(t)
		f.mkdir(`F:\Restores`)
		vault := snapshot(f, vaultPath)
		vaults := newFakeVaults(f)
		result, err := restoreApplyWith(f, vaults, destination, restoreAt, "\x9a\x8b\x7c\x6d")
		if err != nil || result.Effect != sync.EffectPublished {
			t.Fatalf("%s: %+v, %v", destination, result, err)
		}
		if !slices.Equal(vaults.sessions, []string{destination}) || snapshot(f, vaultPath) != vault {
			t.Fatalf("%s: sessions %q, vault changed %v", destination, vaults.sessions, snapshot(f, vaultPath) != vault)
		}
		for _, call := range vaults.calls {
			if strings.Contains(call, "sync.lock") || strings.Contains(call, "..") {
				t.Fatalf("%s: call %s", destination, call)
			}
		}
	}
}

// recordingInventories records the directories a run inventories.
type recordingInventories struct {
	inner sync.Inventories
	calls *[]string
}

func (r recordingInventories) Directory(ctx context.Context, volume string, segments []string) (backup.Directory, error) {
	*r.calls = append(*r.calls, windows(volume, segments))
	return r.inner.Directory(ctx, volume, segments)
}

// The content of the snapshot is read in full once, by the plan, never
// again under lock; the copy is read back from the area only.
func TestRestoreReadsTheContentOnce(t *testing.T) {
	f := restorable(t)
	var calls []string
	s := sync.New(fakeManifests{manifest()}, recordingInventories{inspect.New(nil, inspectFS{f}), &calls}, syncFiles{f}, fakeVolumes{f})
	result, err := s.RestoreApply(context.Background(), restoreRequest(restoreDir), restoreWriter(newFakeVaults(f), restoreAt, "\x9a\x8b\x7c\x6d"))
	if err != nil || result.Effect != sync.EffectPublished {
		t.Fatalf("restore: %+v, %v", result, err)
	}
	if want := []string{snapDir, restoreDir + `\` + restoreApplied + ".incomplete"}; !slices.Equal(calls, want) {
		t.Fatalf("inventories:\n got %q\nwant %q", calls, want)
	}
}

// R7, R8: each --apply publishes a new restore, the earlier one never
// touched again.
func TestSuccessiveRestoresAccumulate(t *testing.T) {
	f := restorable(t)
	if _, err := restoreApply(f, newFakeVaults(f)); err != nil {
		t.Fatal(err)
	}
	first := snapshot(f, restoreDir+`\`+restoreApplied)
	vaults := newFakeVaults(f)
	result, err := restoreApplyWith(f, vaults, restoreDir, restoreAt.Add(time.Hour), "\x0b\x1c\x2d\x3e")
	second := "20261107T111500-0b1c2d3e"
	if err != nil || result.Operation != second || result.Effect != sync.EffectPublished {
		t.Fatalf("second restore: %+v, %v", result, err)
	}
	if snapshot(f, restoreDir+`\`+restoreApplied) != first {
		t.Fatal("the first restore changed")
	}
	for _, call := range vaults.calls {
		if strings.Contains(call, restoreApplied) {
			t.Fatalf("a published restore was targeted again: %s", call)
		}
	}
	if slices.Contains(vaults.calls, "CreateDir .") || slices.ContainsFunc(vaults.calls, func(call string) bool { return strings.Contains(call, "savewarden-restore.json") }) {
		t.Fatalf("a marked area is never marked again: %q", vaults.calls)
	}
	p := restorePlanOf(t, f)
	if len(p.Restores) != 2 || p.Restores[0].State() != "present" || p.Restores[1].State() != "present" || len(p.Findings) != 2 {
		t.Fatalf("plan: %+v, %q", p.Restores, restoreFindings(p))
	}
}

// ADR 0006 §11: an id already used in the area, or equal to the snapshot
// or to its generation, is refused before any journal, code 1.
func TestRestoreIDIsChecked(t *testing.T) {
	tests := []struct {
		name   string
		clock  time.Time
		random string
		op     string
	}{
		{"the snapshot", snapAt, "\x5e\x6f\x7a\x8b", snapApplied},
		{"its generation", at, "\x1a\x2b\x3c\x4d", op1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := restorable(t)
			result, err := restoreApplyWith(f, newFakeVaults(f), restoreDir, tt.clock, tt.random)
			expectRestoreRefused(t, err, "operation id "+tt.op+" is already used in this area, or names the snapshot")
			if result.Effect != sync.EffectMetadata || f.get(restoreDir+`\journal\`+tt.op) != nil || f.get(restoreDir+`\`+tt.op+".incomplete") != nil {
				t.Fatalf("refused id: %+v, journal %q", result, restoreJournal(f, tt.op))
			}
			if result, err := restoreApply(f, newFakeVaults(f)); err != nil || result.Effect != sync.EffectPublished {
				t.Fatalf("next run: %+v, %v", result, err)
			}
		})
	}
	t.Run("used in the area", func(t *testing.T) {
		f := restorable(t)
		publishRestore(t, f, restoreDir, restoreApplied, restoreAt.Add(-time.Hour))
		f.put(restoreDir+`\restore.lock`, f.file(""))
		before := everything(f)
		result, err := restoreApply(f, newFakeVaults(f))
		expectRestoreRefused(t, err, "operation id "+restoreApplied+" is already used in this area")
		if result.Effect != sync.EffectNone || result.State() != "not modified" || everything(f) != before {
			t.Fatalf("collision: %+v", result)
		}
	})
}

// atSnapshotOpening runs change at the k-th opening of saves/1.save in the
// snapshot: the first is the plan's read, the second the copy's.
func atSnapshotOpening(f *fakeFS, k int, change func(*node)) func(string, *node) {
	opened := 0
	return func(name string, n *node) {
		if strings.HasSuffix(name, "/snapshots/"+snapApplied+"/saves/1.save") {
			if opened++; opened == k {
				f.onOpen = nil
				change(n)
			}
		}
	}
}

// ADR 0006 §11, creation step 2 and departure X2: a snapshot changed,
// replaced or emptied during the copy closes the operation in failed,
// changed-during-restore, code 2, its copy kept incomplete.
func TestSnapshotChangedDuringRestore(t *testing.T) {
	tests := map[string]func(f *fakeFS) func(*node){
		"content": func(*fakeFS) func(*node) { return func(n *node) { n.data = []byte("uno") } },
		"replaced": func(f *fakeFS) func(*node) {
			return func(*node) {
				snapshots := f.get(gameDir + `\snapshots`)
				snapshots.children[snapApplied] = copyTree(f, snapshots.children[snapApplied])
			}
		},
		"file removed": func(f *fakeFS) func(*node) {
			return func(*node) { delete(f.get(snapDir+`\saves\Slot 1`).children, "2.save") }
		},
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			f := restorable(t)
			f.onOpen = atSnapshotOpening(f, 2, change(f))
			result, err := restoreApply(f, newFakeVaults(f))
			var blocked *sync.BlockedError
			if !errors.As(err, &blocked) || blocked.Code != sync.CodeChangedDuringRestore || result.Effect != sync.EffectStaged ||
				result.State() != "restore "+restoreApplied+" kept incomplete" {
				t.Fatalf("restore: %+v, %v", result, err)
			}
			if name == "replaced" && !strings.Contains(blocked.Detail, "snapshot replaced") {
				t.Fatalf("a replaced snapshot is named as such: %s", blocked.Detail)
			}
			if got := restoreJournal(f, restoreApplied); got != "001-planned.rec,002-staging-intent.rec,003-failed.rec" ||
				f.get(restoreDir+`\`+restoreApplied+".incomplete") == nil || f.get(restoreDir+`\`+restoreApplied) != nil {
				t.Fatalf("journal %s", got)
			}
			if p := restorePlanOf(t, f); len(p.Interventions) != 0 || p.Incomplete != 1 {
				t.Fatalf("plan after: %q, %d incomplete", restoreCauses(p), p.Incomplete)
			}
		})
	}
}

// ADR 0006 §11, creation step 2: a vault gone during the copy is not
// verifiable, code 1, never a change of the snapshot.
func TestVaultUnavailableDuringRestore(t *testing.T) {
	f := restorable(t)
	f.onOpen = atSnapshotOpening(f, 2, func(*node) { delete(f.volumes[`F:\`].children, "Vault") })
	result, err := restoreApply(f, newFakeVaults(f))
	var blocked *sync.BlockedError
	var intervention *sync.InterventionError
	if err == nil || errors.As(err, &blocked) || errors.As(err, &intervention) || !strings.Contains(err.Error(), "snapshot "+snapApplied+" not verifiable") ||
		result.Effect != sync.EffectStaged || !strings.HasSuffix(restoreJournal(f, restoreApplied), "002-staging-intent.rec,003-failed.rec") {
		t.Fatalf("vault gone: %+v, %v, journal %s", result, err, restoreJournal(f, restoreApplied))
	}
}

// ADR 0006 §11, creation step 3: a copy that differs when read back is
// verification-mismatch, failed; a copy swapped while it is read back is
// code 3, nothing recorded.
func TestRestoreVerification(t *testing.T) {
	verifying := func(f *fakeFS, swap bool) func(string, *node) {
		return func(name string, n *node) {
			if strings.Contains(name, restoreApplied+".incomplete/saves/1.save") {
				f.onOpen = nil
				n.data = []byte("uno")
				if swap {
					area := f.get(restoreDir)
					area.children[restoreApplied+".incomplete"] = copyTree(f, area.children[restoreApplied+".incomplete"])
				}
			}
		}
	}
	f := restorable(t)
	f.onOpen = verifying(f, false)
	_, err := restoreApply(f, newFakeVaults(f))
	var blocked *sync.BlockedError
	if !errors.As(err, &blocked) || blocked.Code != sync.CodeVerificationMismatch || !strings.HasSuffix(restoreJournal(f, restoreApplied), "003-staged.rec,004-failed.rec") {
		t.Fatalf("content differs: %v, journal %s", err, restoreJournal(f, restoreApplied))
	}
	f = restorable(t)
	f.onOpen = verifying(f, true)
	result, err := restoreApply(f, newFakeVaults(f))
	var intervention *sync.InterventionError
	if !errors.As(err, &intervention) || result.Effect != sync.EffectIntervention || !strings.HasSuffix(restoreJournal(f, restoreApplied), "003-staged.rec") {
		t.Fatalf("swapped: %+v, %v, journal %s", result, err, restoreJournal(f, restoreApplied))
	}
}

// The space is checked at the plan, then under lock before any structure
// or journal.
func TestRestoreSpaceIsCheckedBeforeAnyWrite(t *testing.T) {
	f := restorable(t)
	f.volumeOf[`G:\`] = backup.Volume{FileSystem: "NTFS", Drive: backup.DriveFixed, Available: 1 << 20}
	before := everything(f)
	vaults := newFakeVaults(f)
	result, err := restoreApply(f, vaults)
	var blocked *sync.BlockedError
	if !errors.As(err, &blocked) || result.Effect != sync.EffectNone || len(vaults.sessions) != 0 || everything(f) != before {
		t.Fatalf("space at the plan: %+v, %v", result, err)
	}
	f = restorable(t)
	f.described, f.later = 0, &backup.Volume{FileSystem: "NTFS", Drive: backup.DriveFixed, Available: 1 << 20}
	result, err = restoreApply(f, newFakeVaults(f))
	if !errors.As(err, &blocked) || blocked.Code != sync.CodeInsufficientSpace || result.Effect != sync.EffectMetadata ||
		!slices.Equal(names(f.get(restoreDir)), []string{"restore.lock", "savewarden-restore.json"}) {
		t.Fatalf("space under lock: %+v, %v, area %q", result, err, names(f.get(restoreDir)))
	}
}

// restoreHooks runs a hook before a named call of the session, once.
type restoreHooks struct {
	*fakeVaults
	before map[string]func()
}

func hookedRestore(f *fakeFS) *restoreHooks {
	return &restoreHooks{fakeVaults: newFakeVaults(f), before: map[string]func(){}}
}

func (h *restoreHooks) Open(path string) (sync.Vault, error) {
	h.run("Open")
	v, err := h.fakeVaults.Open(path)
	if err != nil {
		return nil, err
	}
	return &restoreHookVault{Vault: v, h: h}, nil
}

func (h *restoreHooks) run(call string) {
	if hook := h.before[call]; hook != nil {
		delete(h.before, call)
		hook()
	}
}

type restoreHookVault struct {
	sync.Vault
	h *restoreHooks
}

func (v *restoreHookVault) CreateDir(name string) error {
	v.h.run("CreateDir " + name)
	return v.Vault.CreateDir(name)
}

func (v *restoreHookVault) NewFile(name string, modTime time.Time) (io.WriteCloser, error) {
	v.h.run("NewFile " + name)
	return v.Vault.NewFile(name, modTime)
}

func (v *restoreHookVault) Move(from, to string) error {
	v.h.run("Move " + from + " " + to)
	return v.Vault.Move(from, to)
}

func (v *restoreHookVault) Lock(name string) error {
	v.h.run("Lock " + name)
	return v.Vault.Lock(name)
}

// ADR 0006 §11, Z3: a file added in the destination before its marker
// leaves it unmarked, code 1; it is never adopted afterwards.
func TestRestoreDestinationFilledBeforeMarking(t *testing.T) {
	for _, existing := range []bool{false, true} {
		f := restorable(t)
		fill := func() { f.put(restoreDir+`\notes.txt`, f.file("x")) }
		var vaults sync.Vaults
		want := sync.EffectMetadata
		if existing {
			// An empty directory, filled before the session pins it.
			f.mkdir(restoreDir)
			hooks := hookedRestore(f)
			hooks.before["Open"] = fill
			vaults, want = hooks, sync.EffectNone
		} else {
			// The file appears right after the session created it.
			vaults = &fillAfterCreate{fakeVaults: newFakeVaults(f), fill: fill}
		}
		result, err := restoreApply(f, vaults)
		expectRestoreRefused(t, err, "destination-changed: it is no longer empty; nothing is marked")
		if result.Effect != want || !slices.Equal(names(f.get(restoreDir)), []string{"notes.txt"}) {
			t.Fatalf("existing %v: %+v, area %q", existing, result, names(f.get(restoreDir)))
		}
		expectRestoreRefused(t, restoreError(f, restoreDir), "is not empty and not a restore area")
	}
}

// fillAfterCreate adds a file in the destination right after the session
// created it.
type fillAfterCreate struct {
	*fakeVaults
	fill func()
}

func (v *fillAfterCreate) Open(path string) (sync.Vault, error) {
	session, err := v.fakeVaults.Open(path)
	if err != nil {
		return nil, err
	}
	return &fillingVault{Vault: session, fill: v.fill}, nil
}

type fillingVault struct {
	sync.Vault
	fill func()
}

func (v *fillingVault) CreateDir(name string) error {
	err := v.Vault.CreateDir(name)
	if name == "." && err == nil {
		v.fill()
	}
	return err
}

// ADR 0006 §11, concurrency of the marker validation (V1): a temporary
// another process took is skipped; a marker another process validated
// first makes this run lose, code 1, nothing more written; nine taken
// temporaries exhaust envelope E1, code 3.
func TestMarkerValidationRaceHasOneWinner(t *testing.T) {
	other := func(f *fakeFS) []byte {
		data, err := backup.EncodeRestoreAreaMarker(backup.RestoreAreaMarker{CreatedAt: restoreAt.Add(-time.Minute), ToolVersion: "other", GameID: "synthetic-game"})
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	// Validated by another process between the temporary and the rename.
	f := restorable(t)
	hooks := hookedRestore(f)
	hooks.before["Move savewarden-restore.json.tmp-1 savewarden-restore.json"] = func() {
		f.put(restoreDir+`\savewarden-restore.json`, f.file(string(other(f))))
	}
	result, err := restoreApply(f, hooks)
	expectRestoreRefused(t, err, "destination-changed: another run marked this area first")
	if result.Effect != sync.EffectMetadata || result.State() != "metadata only; nothing staged or published" ||
		!slices.Equal(names(f.get(restoreDir)), []string{"savewarden-restore.json", "savewarden-restore.json.tmp-1"}) ||
		string(f.get(restoreDir+`\savewarden-restore.json`).data) != string(other(f)) {
		t.Fatalf("loser: %+v, area %q", result, names(f.get(restoreDir)))
	}
	// The abandoned temporary is accepted beside the marker afterwards.
	if result, err := restoreApply(f, newFakeVaults(f)); err != nil || result.Effect != sync.EffectPublished {
		t.Fatalf("after the race: %+v, %v", result, err)
	}

	// A temporary taken between the choice of k and its creation.
	f = restorable(t)
	hooks = hookedRestore(f)
	hooks.before["NewFile savewarden-restore.json.tmp-1"] = func() { f.put(restoreDir+`\savewarden-restore.json.tmp-1`, f.file("{")) }
	if result, err := restoreApply(f, hooks); err != nil || result.Effect != sync.EffectPublished ||
		!slices.Contains(hooks.calls, "Move savewarden-restore.json.tmp-2 savewarden-restore.json") {
		t.Fatalf("k taken: %+v, %v, calls %q", result, err, hooks.calls)
	}

	// Nine temporaries taken by others: envelope E1 exhausted.
	f = restorable(t)
	hooks = hookedRestore(f)
	for k := 1; k <= backup.MaxTemporaries; k++ {
		name := fmt.Sprintf("savewarden-restore.json.tmp-%d", k)
		hooks.before["NewFile "+name] = func() { f.put(restoreDir+`\`+name, f.file("{")) }
	}
	result, err = restoreApply(f, hooks)
	var intervention *sync.InterventionError
	if !errors.As(err, &intervention) || result.Effect != sync.EffectIntervention || f.get(restoreDir+`\savewarden-restore.json`) != nil ||
		!strings.HasPrefix(result.State(), "intervention required for the restore area: ") {
		t.Fatalf("envelope: %+v, %v", result, err)
	}
}

// ADR 0006 §11, Z6: under lock, the area must be the one the plan read,
// the handed-over copies aside.
func TestRestoreAreaIsReadAgainUnderLock(t *testing.T) {
	f := restorable(t)
	publishRestore(t, f, restoreDir, restored1, restoreAt.Add(-time.Hour))
	hooks := hookedRestore(f)
	hooks.before["Lock restore.lock"] = func() { f.put(restoreDir+`\notes.txt`, f.file("x")) }
	result, err := restoreApply(f, hooks)
	expectRestoreRefused(t, err, "destination-changed: unexpected-entry notes.txt appeared since the plan")
	if result.Effect != sync.EffectMetadata || f.get(restoreDir+`\journal\`+restoreApplied) != nil {
		t.Fatalf("changed under lock: %+v", result)
	}
	// A handed-over copy removed between the plan and the lock blocks
	// nothing.
	f = restorable(t)
	publishRestore(t, f, restoreDir, restored1, restoreAt.Add(-time.Hour))
	hooks = hookedRestore(f)
	hooks.before["Lock restore.lock"] = func() { delete(f.get(restoreDir).children, restored1) }
	if result, err := restoreApply(f, hooks); err != nil || result.Effect != sync.EffectPublished {
		t.Fatalf("handed-over copy removed: %+v, %v", result, err)
	}
	// A restore running in the area holds its lock.
	f = restorable(t)
	publishRestore(t, f, restoreDir, restored1, restoreAt.Add(-time.Hour))
	f.put(restoreDir+`\restore.lock`, f.file("")).busy = true
	vaults := newFakeVaults(f)
	vaults.locks[restoreDir+`\restore.lock`] = true
	result, err = restoreApply(f, vaults)
	expectRestoreRefused(t, err, "another restore is running in this area")
	if result.Effect != sync.EffectNone {
		t.Fatalf("lock held: %+v", result)
	}
}

// An area that needs an intervention is refused by --apply, code 3, with
// nothing written.
func TestRestoreApplyRefusesAnAreaInIntervention(t *testing.T) {
	f := restorable(t)
	startRestore(t, f, restoreDir, restored2, restoreAt, backup.StepPlanned)
	before := everything(f)
	vaults := newFakeVaults(f)
	result, err := restoreApply(f, vaults)
	var intervention *sync.InterventionError
	if !errors.As(err, &intervention) || result.Effect != sync.EffectIntervention || len(vaults.sessions) != 0 || everything(f) != before ||
		result.State() != "intervention required for the restore area: pending-operation "+restored2 ||
		!strings.Contains(err.Error(), "intervention required for the restore area") || !strings.Contains(err.Error(), "restore --recover") {
		t.Fatalf("intervention: %+v, %v", result, err)
	}
}

// ADR 0006 §11, creation step 4: a rename refused, and confirmed not done
// by the source and the target, rolls back, code 1, the copy kept.
func TestRestoreRenameRefusedRollsBack(t *testing.T) {
	f := restorable(t)
	vaults := newFakeVaults(f)
	vaults.busy[restoreApplied+".incomplete"] = 1000
	result, err := restoreApply(f, vaults)
	var blocked *sync.BlockedError
	var intervention *sync.InterventionError
	if err == nil || errors.As(err, &blocked) || errors.As(err, &intervention) || result.Effect != sync.EffectRolledBack ||
		result.State() != "rolled back restore "+restoreApplied || !strings.HasSuffix(restoreJournal(f, restoreApplied), "005-publish-intent.rec,006-rolled-back.rec") ||
		f.get(restoreDir+`\`+restoreApplied+".incomplete") == nil || f.get(restoreDir+`\`+restoreApplied) != nil {
		t.Fatalf("rename refused: %+v, %v, journal %s", result, err, restoreJournal(f, restoreApplied))
	}
}

// X1: a restore runs while sync.lock is held and an operation of the vault
// is pending, without touching or blocking either.
func TestRestoreRunsBesideSyncAndSnapshot(t *testing.T) {
	f := restorable(t)
	f.put(gameDir+`\sync.lock`, f.file("")).busy = true
	f.mkdir(gameDir + `\snapshots\journal\` + snapOp2)
	writeJournal(t, f, op2, backup.StepPlanned)
	vault := snapshot(f, vaultPath)
	result, err := restoreApply(f, newFakeVaults(f))
	if err != nil || result.Effect != sync.EffectPublished || snapshot(f, vaultPath) != vault {
		t.Fatalf("beside sync and snapshot: %+v, %v", result, err)
	}
}

// A crash just before and just after every call of a restore leaves the
// vault untouched, the area in a state its plan recognises, a restore
// published only with its intent, and no restore.lock in an unmarked
// directory; once the area plans a new restore, the next run publishes.
func TestCrashAtEveryRestoreCall(t *testing.T) {
	clean := restorable(t)
	vaults := newFakeVaults(clean)
	if _, err := restoreApply(clean, vaults); err != nil {
		t.Fatal(err)
	}
	total := len(vaults.calls)
	for k := 1; k <= total; k++ {
		for _, after := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d %v %s", k, after, vaults.calls[k-1]), func(t *testing.T) {
				f := restorable(t)
				vault := snapshot(f, `F:\`)
				crashing := newFakeVaults(f)
				if after {
					crashing.crashAfter = k
				} else {
					crashing.crashBefore = k
				}
				if _, err := restoreApply(f, crashing); err == nil {
					t.Fatal("no crash")
				}
				if snapshot(f, `F:\`) != vault {
					t.Fatal("the vault changed")
				}
				area := f.get(restoreDir)
				if area != nil && area.children["savewarden-restore.json"] == nil && area.children["restore.lock"] != nil {
					t.Fatal("restore.lock in an unmarked directory")
				}
				journal := restoreJournal(f, restoreApplied)
				if f.get(restoreDir+`\`+restoreApplied) != nil && !strings.Contains(journal, "-publish-intent.rec") {
					t.Fatalf("restore published without its intent: %s", journal)
				}
				p, err := restorePlanWith(f, manifest(), restoreRequest(restoreDir))
				if err != nil {
					t.Fatalf("plan after the crash: %v", err)
				}
				for _, cause := range restoreCauses(p) {
					if !strings.HasPrefix(cause, "initialization-interrupted ") && cause != "pending-operation "+restoreApplied {
						t.Fatalf("plan after the crash: %q", restoreCauses(p))
					}
				}
				published := strings.HasSuffix(journal, "006-published.rec")
				if published != (len(p.Restores) == 1) || (published && p.Restores[0].State() != "present") {
					t.Fatalf("journal %s, restores %+v", journal, p.Restores)
				}
				if p.Outcome() == "new restore" {
					next, err := restoreApplyWith(f, newFakeVaults(f), restoreDir, restoreAt.Add(time.Hour), "\x0b\x1c\x2d\x3e")
					if err != nil || next.Effect != sync.EffectPublished {
						t.Fatalf("next run: %+v, %v", next, err)
					}
				}
			})
		}
	}
}
