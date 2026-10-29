package sync_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/features/sync"
)

const gameDir = vaultPath + `\games\synthetic-game`

// snapshot lists every node below path with its identity, mode, date and
// content, so that a test can prove nothing changed.
func snapshot(f *fakeFS, path string) string {
	var lines []string
	var walk func(prefix string, n *node)
	walk = func(prefix string, n *node) {
		lines = append(lines, fmt.Sprintf("%s|%d|%v|%d|%q", prefix, n.id, n.mode, n.modTime.UnixNano(), n.data))
		for name, child := range n.children {
			walk(prefix+`\`+name, child)
		}
	}
	n := f.get(path)
	if len(path) == 3 {
		n = f.volumes[path]
	}
	if n == nil {
		panic("snapshot of a missing path " + path)
	}
	walk(path, n)
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

type run struct {
	f      *fakeFS
	vaults *fakeVaults
	clock  *fakeClock
	result sync.Result
	err    error
}

func apply(f *fakeFS, vault string) *run {
	r := &run{f: f, vaults: newFakeVaults(f), clock: &fakeClock{now: at}}
	return r.again(vault)
}

func (r *run) again(vault string) *run {
	r.result, r.err = service(r.f, manifest()).Apply(context.Background(),
		sync.Request{ManifestPath: "m.yaml", VaultPath: vault}, writer(r.vaults, r.clock))
	return r
}

func names(n *node) []string {
	var out []string
	for name := range n.children {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func TestFirstPublicationIntoANewVault(t *testing.T) {
	f := world()
	f.mkdir(`F:\Backups`)
	sources := snapshot(f, `D:\`)
	r := apply(f, `F:\Backups\Vault`)
	if r.err != nil {
		t.Fatal(r.err)
	}
	if r.result.Effect != sync.EffectPublished || r.result.Operation != op1 || r.result.State() != "published "+op1 {
		t.Fatalf("result %+v", r.result)
	}
	if snapshot(f, `D:\`) != sources {
		t.Fatal("the source changed")
	}
	vault := f.get(`F:\Backups\Vault`)
	if got := names(vault); strings.Join(got, ",") != "games,savewarden-vault.json" {
		t.Fatalf("vault root %v", got)
	}
	if _, err := backup.DecodeMarker(vault.children["savewarden-vault.json"].data); err != nil {
		t.Fatal(err)
	}
	game := f.get(`F:\Backups\Vault\games\synthetic-game`)
	if got := names(game); strings.Join(got, ",") != "current,journal,receipts,staging,sync.lock" {
		t.Fatalf("game directory %v", got)
	}
	if len(game.children["staging"].children) != 0 {
		t.Fatalf("staging left %v", names(game.children["staging"]))
	}
	journal := game.children["journal"].children[op1]
	want := []string{"001-planned.rec", "002-staging-intent.rec", "003-staged.rec", "004-verified.rec", "005-publish-intent.rec", "006-published.rec"}
	if got := names(journal); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("journal %v", got)
	}
	receipt, err := backup.DecodeReceipt(game.children["receipts"].children[op1+".json"].data)
	if err != nil || receipt.Recovered() || receipt.PublishedAt == nil || len(receipt.Sources) != 1 || len(receipt.Sources[0].Entries) != 2 {
		t.Fatalf("receipt %+v, %v", receipt, err)
	}
	current := game.children["current"].children["saves"]
	if string(current.children["1.save"].data) != "one" || string(current.children["Slot 1"].children["2.save"].data) != "two" ||
		!current.children["1.save"].modTime.Equal(epoch) {
		t.Fatalf("current %+v", current)
	}
	again := plan(t, f, manifest(), `F:\Backups\Vault`)
	if again.Outcome() != "up to date" || again.Current != op1 {
		t.Fatalf("plan after publication: %q, findings %q", again.Outcome(), findings(again))
	}
	before := snapshot(f, `F:\`)
	calls := len(r.vaults.calls)
	if r.again(`F:\Backups\Vault`); r.err != nil || r.result.Effect != sync.EffectNone || len(r.vaults.calls) != calls {
		t.Fatalf("second apply: %+v, %v, %d calls", r.result, r.err, len(r.vaults.calls)-calls)
	}
	if snapshot(f, `F:\`) != before {
		t.Fatal("an up-to-date apply wrote")
	}
}

func TestApplyIntoAnEmptyOrInitializedVault(t *testing.T) {
	f := world()
	f.mkdir(vaultPath)
	if r := apply(f, vaultPath); r.err != nil || r.result.Effect != sync.EffectPublished {
		t.Fatalf("empty vault: %+v, %v", r.result, r.err)
	}
	f = world()
	markVault(t, f, vaultPath)
	marker := snapshot(f, vaultPath+`\savewarden-vault.json`)
	if r := apply(f, vaultPath); r.err != nil || r.result.Effect != sync.EffectPublished {
		t.Fatalf("initialized vault: %+v, %v", r.result, r.err)
	}
	if snapshot(f, vaultPath+`\savewarden-vault.json`) != marker {
		t.Fatal("marker rewritten")
	}
}

// B2b publishes a first generation only: a different current is refused.
func TestExistingCurrentIsRefused(t *testing.T) {
	f := world()
	markVault(t, f, vaultPath)
	publish(t, f, manifest(), op2, "")
	f.put(installPath+`\saves\new.save`, f.file("new"))
	before := snapshot(f, `F:\`)
	r := apply(f, vaultPath)
	var blocked *sync.BlockedError
	if !errors.As(r.err, &blocked) || blocked.Code != sync.CodeCurrentExists || r.result.Effect != sync.EffectNone {
		t.Fatalf("result %+v, %v", r.result, r.err)
	}
	if snapshot(f, `F:\`) != before || len(r.vaults.calls) != 0 {
		t.Fatal("the refused apply wrote")
	}
}

func TestApplyRefusals(t *testing.T) {
	t.Run("interrupted initialization", func(t *testing.T) {
		f := world()
		f.put(vaultPath+`\savewarden-vault.json.tmp-1`, f.file("{"))
		before := snapshot(f, `F:\`)
		r := apply(f, vaultPath)
		var intervention *sync.InterventionError
		if !errors.As(r.err, &intervention) || r.result.Effect != sync.EffectIntervention || snapshot(f, `F:\`) != before {
			t.Fatalf("result %+v, %v", r.result, r.err)
		}
	})
	t.Run("blocked plan", func(t *testing.T) {
		f := world()
		f.put(installPath+`\saves\secret.save`, f.file("s")).crypted = true
		r := apply(f, vaultPath)
		var blocked *sync.BlockedError
		if !errors.As(r.err, &blocked) || blocked.Code != "" || len(r.vaults.calls) != 0 {
			t.Fatalf("result %+v, %v", r.result, r.err)
		}
	})
	t.Run("another sync holds the lock", func(t *testing.T) {
		f := world()
		markVault(t, f, vaultPath)
		f.mkdir(gameDir)
		f.put(gameDir+`\sync.lock`, f.file(""))
		vaults := newFakeVaults(f)
		vaults.locks[gameDir+`\sync.lock`] = true
		r := &run{f: f, vaults: vaults, clock: &fakeClock{now: at}}
		r.again(vaultPath)
		if r.err == nil || !strings.Contains(r.err.Error(), "another sync is running") || r.result.Effect != sync.EffectNone {
			t.Fatalf("result %+v, %v", r.result, r.err)
		}
	})
	t.Run("space gone under lock", func(t *testing.T) {
		f := world()
		f.later = &backup.Volume{FileSystem: "NTFS", Drive: backup.DriveFixed, Available: 1000}
		r := apply(f, vaultPath)
		var blocked *sync.BlockedError
		if !errors.As(r.err, &blocked) || blocked.Code != sync.CodeInsufficientSpace ||
			r.result.State() != "metadata only; nothing staged or published" {
			t.Fatalf("result %+v, %v", r.result, r.err)
		}
		if game := f.get(gameDir); game == nil || game.children["journal"] != nil || game.children["staging"] != nil {
			t.Fatal("journal or staging created despite the lack of space")
		}
	})
	t.Run("vault appears after the plan", func(t *testing.T) {
		f := world()
		vaults := newFakeVaults(f)
		r := &run{f: f, vaults: vaults, clock: &fakeClock{now: at}}
		f.onOpen = nil
		var once bool
		vaults.failWrite = nil
		// The plan sees no vault; the session finds one.
		real := f.volumes[`F:\`]
		r.result, r.err = service(f, manifest()).Apply(context.Background(), sync.Request{ManifestPath: "m.yaml", VaultPath: vaultPath},
			sync.Writer{Vaults: appearing{vaults, func() {
				if !once {
					once = true
					real.children["Vault"] = f.dir()
				}
			}}, Clock: r.clock, Random: strings.NewReader("abcd"), ToolVersion: "dev"})
		if r.err == nil || !strings.Contains(r.err.Error(), "vault-changed") || r.result.Effect != sync.EffectNone {
			t.Fatalf("result %+v, %v", r.result, r.err)
		}
	})
}

// appearing runs a change of the vault just before the session opens.
type appearing struct {
	*fakeVaults
	change func()
}

func (a appearing) Open(path string) (sync.Vault, error) {
	a.change()
	return a.fakeVaults.Open(path)
}

// failed reads the last record of the only operation, which must be
// terminal failed, with current absent and staging kept.
func expectFailed(t *testing.T, r *run) {
	t.Helper()
	if r.result.Effect != sync.EffectStaged || r.result.State() != "current unchanged; operation "+op1+" kept in staging" {
		t.Fatalf("result %+v, %v", r.result, r.err)
	}
	game := r.f.get(gameDir)
	if game.children["current"] != nil {
		t.Fatal("current created by a failed operation")
	}
	journal := game.children["journal"].children[op1]
	got := names(journal)
	if !strings.HasSuffix(got[len(got)-1], "-failed.rec") {
		t.Fatalf("journal %v", got)
	}
	if p := plan(t, r.f, manifest(), vaultPath); len(p.Interventions) != 0 {
		t.Fatalf("plan after a failed operation: %q", causes(p))
	}
}

func TestSourceChangedDuringTheCopy(t *testing.T) {
	f := world()
	sources := snapshot(f, `D:\`)
	opened := 0
	f.onOpen = func(name string, n *node) {
		// The plan reads the file first; the copy opens it second.
		if strings.HasSuffix(name, "/1.save") && strings.HasPrefix(name, "Games/") {
			if opened++; opened == 2 {
				n.data, n.modTime = []byte("ONE"), n.modTime.Add(time.Second)
			}
		}
	}
	r := apply(f, vaultPath)
	var blocked *sync.BlockedError
	if !errors.As(r.err, &blocked) || blocked.Code != sync.CodeChangedDuringSync {
		t.Fatalf("result %+v, %v", r.result, r.err)
	}
	expectFailed(t, r)
	f.onOpen = nil
	if snapshot(f, `D:\`) == sources {
		t.Fatal("premise: the source should have changed")
	}
}

func TestSourceChangedBetweenCopyAndRecheck(t *testing.T) {
	f := world()
	opened := 0
	f.onOpen = func(name string, n *node) {
		// Two files: the plan opens them first, the copy next.
		if strings.HasPrefix(name, "Games/") {
			if opened++; opened == 4 {
				f.put(installPath+`\saves\late.save`, f.file("late"))
			}
		}
	}
	r := apply(f, vaultPath)
	var blocked *sync.BlockedError
	if !errors.As(r.err, &blocked) || blocked.Code != sync.CodeChangedDuringSync || !strings.Contains(blocked.Detail, "changed during the copy") {
		t.Fatalf("result %+v, %v", r.result, r.err)
	}
	expectFailed(t, r)
}

func TestWriteFailureKeepsTheStaging(t *testing.T) {
	f := world()
	vaults := newFakeVaults(f)
	vaults.failWrite = func(call string) error {
		if strings.HasPrefix(call, "Write ") && strings.HasSuffix(call, "2.save") {
			return errors.New("disk full")
		}
		return nil
	}
	r := &run{f: f, vaults: vaults, clock: &fakeClock{now: at}}
	r.again(vaultPath)
	if r.err == nil || !strings.Contains(r.err.Error(), "disk full") {
		t.Fatalf("result %+v, %v", r.result, r.err)
	}
	var blocked *sync.BlockedError
	if errors.As(r.err, &blocked) {
		t.Fatal("a write error is exit code 1, not 2")
	}
	expectFailed(t, r)
	if f.get(gameDir+`\staging\`+op1+`\saves\1.save`) == nil {
		t.Fatal("staging not kept")
	}
}

func TestVerificationMismatch(t *testing.T) {
	f := world()
	f.onOpen = func(name string, n *node) {
		if strings.Contains(name, "/staging/") && strings.HasSuffix(name, "1.save") {
			n.data = []byte("oNe")
		}
	}
	r := apply(f, vaultPath)
	var blocked *sync.BlockedError
	if !errors.As(r.err, &blocked) || blocked.Code != sync.CodeVerificationMismatch {
		t.Fatalf("result %+v, %v", r.result, r.err)
	}
	expectFailed(t, r)
}

// ADR 0006 §7: sharing and lock violations are retried with a backoff for
// five seconds at most; the switch then fails with current unchanged.
func TestRenameRetries(t *testing.T) {
	staging := "games/synthetic-game/staging/" + op1
	t.Run("transient", func(t *testing.T) {
		f := world()
		vaults := newFakeVaults(f)
		vaults.busy[staging] = 3
		r := &run{f: f, vaults: vaults, clock: &fakeClock{now: at}}
		if r.again(vaultPath); r.err != nil || r.result.Effect != sync.EffectPublished {
			t.Fatalf("result %+v, %v", r.result, r.err)
		}
		if fmt.Sprint(r.clock.slept) != "[50ms 100ms 200ms]" {
			t.Fatalf("slept %v", r.clock.slept)
		}
	})
	t.Run("permanent", func(t *testing.T) {
		f := world()
		vaults := newFakeVaults(f)
		vaults.busy[staging] = 1000
		r := &run{f: f, vaults: vaults, clock: &fakeClock{now: at}}
		r.again(vaultPath)
		if r.err == nil || r.result.Effect != sync.EffectRolledBack || r.result.State() != "rolled back "+op1+"; current unchanged" {
			t.Fatalf("result %+v, %v", r.result, r.err)
		}
		var total time.Duration
		for _, d := range r.clock.slept {
			total += d
		}
		if total > 5*time.Second || len(r.clock.slept) == 0 {
			t.Fatalf("slept %v", r.clock.slept)
		}
		journal := names(f.get(gameDir + `\journal\` + op1))
		if journal[len(journal)-1] != "006-rolled-back.rec" || f.get(gameDir+`\current`) != nil {
			t.Fatalf("journal %v", journal)
		}
		if p := plan(t, f, manifest(), vaultPath); len(p.Interventions) != 0 || p.Outcome() != "changes" {
			t.Fatalf("plan after rollback: %q %q", p.Outcome(), causes(p))
		}
	})
}

// A rename that fails for another reason, such as access denied, is never
// retried.
func TestOtherRenameErrorsAreNotRetried(t *testing.T) {
	f := world()
	vaults := newFakeVaults(f)
	vaults.failWrite = nil
	r := &run{f: f, vaults: vaults, clock: &fakeClock{now: at}}
	denied := &denyingVaults{fakeVaults: vaults, from: "games/synthetic-game/staging/" + op1}
	r.result, r.err = service(f, manifest()).Apply(context.Background(), sync.Request{ManifestPath: "m.yaml", VaultPath: vaultPath},
		sync.Writer{Vaults: denied, Clock: r.clock, Random: strings.NewReader("\x1a\x2b\x3c\x4d"), ToolVersion: "dev"})
	if r.err == nil || !errors.Is(r.err, fs.ErrPermission) || len(r.clock.slept) != 0 || r.result.Effect != sync.EffectRolledBack {
		t.Fatalf("result %+v, %v, slept %v", r.result, r.err, r.clock.slept)
	}
}

type denyingVaults struct {
	*fakeVaults
	from string
}

func (d *denyingVaults) Open(path string) (sync.Vault, error) {
	v, err := d.fakeVaults.Open(path)
	if err != nil {
		return nil, err
	}
	return &denyingVault{Vault: v, from: d.from}, nil
}

type denyingVault struct {
	sync.Vault
	from string
}

func (d *denyingVault) Move(from, to string) error {
	if from == d.from {
		return fmt.Errorf("move %s: %w", from, fs.ErrPermission)
	}
	return d.Vault.Move(from, to)
}

// crashRun applies into a new vault and kills the process at mutating call
// k, before or after performing it.
func crashRun(k int, after bool) *run {
	f := world()
	vaults := newFakeVaults(f)
	if after {
		vaults.crashAfter = k
	} else {
		vaults.crashBefore = k
	}
	r := &run{f: f, vaults: vaults, clock: &fakeClock{now: at}}
	return r.again(vaultPath)
}

// Plan B2 §9.2: a crash just before and just after every vault call of a
// first publication leaves the source untouched and a vault that the plan
// reads without error; current never appears without its intent.
func TestCrashAtEveryVaultCall(t *testing.T) {
	clean := apply(world(), vaultPath)
	if clean.err != nil {
		t.Fatal(clean.err)
	}
	total := len(clean.vaults.calls)
	if total < 20 {
		t.Fatalf("premise: %d calls", total)
	}
	sources := snapshot(world(), `D:\`)
	for k := 1; k <= total; k++ {
		for _, after := range []bool{false, true} {
			name := fmt.Sprintf("%s %d %s", map[bool]string{false: "before", true: "after"}[after], k, clean.vaults.calls[k-1])
			r := crashRun(k, after)
			if r.err == nil || !strings.Contains(r.err.Error(), errCrash.Error()) {
				t.Fatalf("%s: %v", name, r.err)
			}
			if snapshot(r.f, `D:\`) != sources {
				t.Fatalf("%s: the source changed", name)
			}
			p, err := service(r.f, manifest()).Plan(context.Background(), sync.Request{ManifestPath: "m.yaml", VaultPath: vaultPath})
			if err != nil {
				t.Fatalf("%s: plan after the crash: %v", name, err)
			}
			if current := r.f.get(gameDir + `\current`); current != nil {
				journal := r.f.get(gameDir + `\journal\` + op1)
				if journal == nil || journal.children["005-publish-intent.rec"] == nil {
					t.Fatalf("%s: current without publish-intent", name)
				}
				if current.id != clean.f.get(gameDir+`\current`).id && current.id == 0 {
					t.Fatalf("%s: current of unknown identity", name)
				}
			}
			if p.Outcome() == "changes" && r.f.get(gameDir+`\journal\`+op1) != nil {
				t.Fatalf("%s: an interrupted operation was not reported: %q", name, causes(p))
			}
		}
	}
}

// ADR 0006 §4, step 6: a staging directory substituted between the
// pre-check and the rename is published under the right name but with the
// wrong identity; the post-check catches it, nothing else is moved, and
// the run ends in code 3.
func TestSwitchRaceIsCaughtAfterTheRename(t *testing.T) {
	f := world()
	vaults := newFakeVaults(f)
	staging := "games/synthetic-game/staging/" + op1
	racing := &racingVaults{fakeVaults: vaults, from: staging, swap: func() {
		parent := f.get(gameDir + `\staging`)
		parent.children[op1+"-real"] = parent.children[op1]
		parent.children[op1] = f.dir()
	}}
	clock := &fakeClock{now: at}
	result, err := service(f, manifest()).Apply(context.Background(), sync.Request{ManifestPath: "m.yaml", VaultPath: vaultPath},
		sync.Writer{Vaults: racing, Clock: clock, Random: strings.NewReader("\x1a\x2b\x3c\x4d"), ToolVersion: "dev"})
	var intervention *sync.InterventionError
	if !errors.As(err, &intervention) || result.Effect != sync.EffectIntervention {
		t.Fatalf("result %+v, %v", result, err)
	}
	moves := 0
	for _, call := range vaults.calls {
		if strings.HasPrefix(call, "Move games/synthetic-game/staging/") {
			moves++
		}
	}
	if moves != 1 || f.get(gameDir+`\receipts`) != nil {
		t.Fatalf("after the race: %d moves of the staging, receipts %v", moves, f.get(gameDir+`\receipts`))
	}
	journal := names(f.get(gameDir + `\journal\` + op1))
	if journal[len(journal)-1] != "005-publish-intent.rec" {
		t.Fatalf("journal %v", journal)
	}
}

type racingVaults struct {
	*fakeVaults
	from string
	swap func()
}

func (r *racingVaults) Open(path string) (sync.Vault, error) {
	v, err := r.fakeVaults.Open(path)
	if err != nil {
		return nil, err
	}
	return &racingVault{Vault: v, r: r}, nil
}

type racingVault struct {
	sync.Vault
	r *racingVaults
}

func (v *racingVault) Move(from, to string) error {
	if from == v.r.from && v.r.swap != nil {
		v.r.swap()
		v.r.swap = nil
	}
	return v.Vault.Move(from, to)
}
