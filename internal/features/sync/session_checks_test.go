package sync_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/entities/game"
	"github.com/savewarden/savewarden-cli/internal/features/inspect"
	"github.com/savewarden/savewarden-cli/internal/features/sync"
)

// The checks under the session of plan B5 §4.3 (T4), on the fake: the fake
// session pins its nodes at Open like the real one, so what a letter
// resolves to afterwards does not change the chain it holds.

// beforeOpen runs hook just before the session opens: what changes between
// the plan and the session.
type beforeOpen struct {
	*fakeVaults
	hook func()
}

func (b beforeOpen) Open(path string) (sync.Vault, error) {
	b.hook()
	return b.fakeVaults.Open(path)
}

func applyThrough(f *fakeFS, m game.Manifest, vault string, vaults sync.Vaults) (sync.Result, error) {
	return service(f, m).Apply(context.Background(), sync.Request{ManifestPath: "m.yaml", VaultPath: vault},
		sync.Writer{Vaults: vaults, Clock: &fakeClock{now: at}, Random: bytes.NewReader([]byte{0x1a, 0x2b, 0x3c, 0x4d}), ToolVersion: "dev"})
}

// aliasRoot gives letter the very root of volume: a letter strictly
// equivalent to a volume root (decision 24), the only alias S-02 admits.
func aliasRoot(f *fakeFS, letter, volume string) { f.volumes[letter] = f.volumes[volume] }

// expectRefusedUnderSession requires a refusal, code 1, made once the
// session was open and before it wrote anything, mentioning every mention.
func expectRefusedUnderSession(t *testing.T, err error, vaults *fakeVaults, mentions ...string) {
	t.Helper()
	expectRestoreRefused(t, err, mentions...)
	if len(vaults.sessions) == 0 {
		t.Fatalf("refused before any session: %v", err)
	}
	if len(vaults.calls) != 0 {
		t.Fatalf("the session wrote before its checks: %q", vaults.calls)
	}
}

func optionalSaves() game.Manifest { return manifest(source("saves", "saves", false)) }

// A vault absent beside the installation, or whose parent is the volume
// root, the common ancestor of every reference, is accepted: the parent of
// an absent target only anchors it and is never searched in a reference.
// So is an absent restore area beside the installation.
func TestAbsentTargetBesideAReferenceIsAccepted(t *testing.T) {
	for _, vault := range []string{`D:\Games\Vault`, `D:\Vault`} {
		t.Run("vault "+vault, func(t *testing.T) {
			f := world()
			if r := apply(f, vault); r.err != nil || r.result.Effect != sync.EffectPublished {
				t.Fatalf("%+v, %v", r.result, r.err)
			}
		})
	}
	t.Run("restore area beside the installation", func(t *testing.T) {
		f := restorable(t)
		result, err := restoreApplyWith(f, newFakeVaults(f), `D:\Games\Restauration`, restoreAt, "\x9a\x8b\x7c\x6d")
		if err != nil || result.Effect != sync.EffectPublished {
			t.Fatalf("%+v, %v", result, err)
		}
	})
}

// A vault absent below the installation or a source, named through another
// letter strictly equivalent to the volume root, passes the plan, whose
// comparison of the vault with the installation is still by letter until
// S-02; the session refuses it on its held chain, before any write. A
// restore area so placed is refused by the plan of B4, by identity.
func TestAbsentTargetUnderAReferenceIsRefused(t *testing.T) {
	for _, vault := range []string{`H:\Games\Synthetic Game\Vault`, `H:\Games\Synthetic Game\saves\Vault`} {
		t.Run("vault "+vault, func(t *testing.T) {
			f := world()
			aliasRoot(f, `H:\`, `D:\`)
			before := everything(f)
			if _, err := service(f, manifest()).Plan(context.Background(), sync.Request{ManifestPath: "m.yaml", VaultPath: vault}); err != nil {
				t.Fatalf("the plan refused already, the session guard is not the one tested: %v", err)
			}
			vaults := newFakeVaults(f)
			_, err := applyThrough(f, manifest(), vault, vaults)
			expectRefusedUnderSession(t, err, vaults, "vault "+vault, "overlaps the installation "+installPath)
			if everything(f) != before {
				t.Fatal("something was written")
			}
		})
	}
	t.Run("restore area under a source", func(t *testing.T) {
		f := restorable(t)
		aliasRoot(f, `H:\`, `D:\`)
		vaults := newFakeVaults(f)
		before := everything(f)
		_, err := restoreApplyWith(f, vaults, `H:\Games\Synthetic Game\saves\Restauration`, restoreAt, "\x9a\x8b\x7c\x6d")
		expectRestoreRefused(t, err, "overlaps the")
		if len(vaults.sessions) != 0 || everything(f) != before {
			t.Fatalf("sessions %q, or something was written", vaults.sessions)
		}
	})
}

// An existing vault that contains the installation, absent, is refused
// under the session: the identity of the vault lies in the chain of the
// installation, observed down to its deepest existing ancestor.
func TestExistingTargetContainingAReferenceIsRefused(t *testing.T) {
	f := newFakeFS(`D:\`, `F:\`)
	markVault(t, f, `D:\Box`)
	aliasRoot(f, `H:\`, `D:\`)
	m := optionalSaves()
	m.Installation.Path = `D:\Box\Absent Game`
	before := everything(f)
	if _, err := service(f, m).Plan(context.Background(), sync.Request{ManifestPath: "m.yaml", VaultPath: `H:\Box`}); err != nil {
		t.Fatalf("the plan refused already: %v", err)
	}
	vaults := newFakeVaults(f)
	_, err := applyThrough(f, m, `H:\Box`, vaults)
	expectRefusedUnderSession(t, err, vaults, `vault H:\Box`, `overlaps the installation D:\Box\Absent Game`)
	if everything(f) != before {
		t.Fatal("something was written")
	}
}

// The session decides on what it holds: a vault in a source through
// another letter, a held chain that cannot be read, a reference that
// changed since the plan, a marker that changed, a vault that appeared.
// Each is refused, code 1, by its own guard, the session open and nothing
// written.
func TestChecksUnderTheSessionUseTheHeldChain(t *testing.T) {
	for _, tt := range []struct {
		name    string
		vault   string
		prepare func(t *testing.T, f *fakeFS)
		change  func(f *fakeFS, v *fakeVaults)
		mention string
	}{
		{"vault in a source through another letter", `H:\Games\Synthetic Game\saves\Vault`,
			func(_ *testing.T, f *fakeFS) { aliasRoot(f, `H:\`, `D:\`) }, nil, "overlaps the"},
		{"held chain unreadable", vaultPath, nil,
			func(_ *fakeFS, v *fakeVaults) { v.chainErr = errors.New("held chain unreadable") }, "cannot be checked under the session"},
		{"reference changed since the plan", vaultPath, nil,
			func(f *fakeFS, _ *fakeVaults) { f.put(installPath+`\saves`, copyTree(f, f.get(installPath+`\saves`))) },
			`the source saves ` + installPath + `\saves changed since the plan`},
		{"marker changed since the plan", vaultPath, func(t *testing.T, f *fakeFS) { markVault(t, f, vaultPath) },
			func(f *fakeFS, _ *fakeVaults) {
				data, _ := backup.EncodeMarker(backup.Marker{CreatedAt: at.Add(1), ToolVersion: "dev"})
				f.get(vaultPath + `\savewarden-vault.json`).data = data
			}, "vault-changed: its marker changed since the plan"},
		{"vault appeared since the plan", vaultPath, nil,
			func(f *fakeFS, _ *fakeVaults) { f.mkdir(vaultPath) }, "vault-changed: it appeared since the plan"},
		{"vault replaced since the plan", vaultPath, func(t *testing.T, f *fakeFS) { markVault(t, f, vaultPath) },
			func(f *fakeFS, _ *fakeVaults) { f.put(vaultPath, copyTree(f, f.get(vaultPath))) },
			"the session holds other directories than the plan read"},
		{"vault disappeared since the plan", vaultPath, func(t *testing.T, f *fakeFS) { markVault(t, f, vaultPath) },
			func(f *fakeFS, _ *fakeVaults) { delete(f.volumes[`F:\`].children, "Vault") },
			"vault-changed: it disappeared since the plan"},
		{"reference unobservable under the session", vaultPath, nil,
			func(f *fakeFS, _ *fakeVaults) { f.get(installPath).statErr = errors.New("access denied") },
			"separation from the installation " + installPath + " cannot be proven under the session: access denied"},
		// A reference that cannot be observed is never searched for an
		// overlap: the held chain, which differs, is what is reported.
		{"reference unobservable and held chain different", vaultPath, func(t *testing.T, f *fakeFS) { markVault(t, f, vaultPath) },
			func(f *fakeFS, _ *fakeVaults) {
				f.put(vaultPath, copyTree(f, f.get(vaultPath)))
				f.get(installPath).statErr = errors.New("access denied")
			}, "the session holds other directories than the plan read"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := world()
			if tt.prepare != nil {
				tt.prepare(t, f)
			}
			vaults := newFakeVaults(f)
			hook := beforeOpen{fakeVaults: vaults, hook: func() {}}
			if tt.change != nil {
				hook.hook = func() { tt.change(f, vaults) }
			}
			_, err := applyThrough(f, manifest(), tt.vault, hook)
			// The session wrote nothing: no mutating call at all.
			expectRefusedUnderSession(t, err, vaults, "vault "+tt.vault, tt.mention)
			if strings.Contains(err.Error(), "overlaps") && !strings.Contains(tt.mention, "overlaps") {
				t.Fatalf("an overlap reported from an observation that does not support it: %v", err)
			}
		})
	}
}

// Every command and every recovery reads its references again under the
// session: a source replaced between the plan and the session is refused,
// code 1, the session open and nothing written, restore --recover without
// opening the vault.
func TestEveryPathChecksItsReferencesUnderTheSession(t *testing.T) {
	replaceSaves := func(f *fakeFS) { f.put(installPath+`\saves`, copyTree(f, f.get(installPath+`\saves`))) }
	changed := `the source saves ` + installPath + `\saves changed since the plan`
	for _, tt := range []struct {
		name    string
		fixture func(t *testing.T) *fakeFS
		run     func(f *fakeFS, vaults sync.Vaults) error
		subject string
	}{
		{"sync --apply", func(*testing.T) *fakeFS { return world() }, func(f *fakeFS, v sync.Vaults) error {
			_, err := applyThrough(f, manifest(), vaultPath, v)
			return err
		}, "vault " + vaultPath},
		{"sync --recover --apply", func(*testing.T) *fakeFS {
			return crashRun(len(apply(world(), vaultPath).vaults.calls), false).f
		}, func(f *fakeFS, v sync.Vaults) error {
			_, err := service(f, manifest()).Recover(context.Background(), sync.Request{ManifestPath: "m.yaml", VaultPath: vaultPath},
				sync.Writer{Vaults: v, Clock: &fakeClock{now: at.Add(recoveryLater)}, Random: bytes.NewReader(make([]byte, 4)), ToolVersion: "dev"})
			return err
		}, "vault " + vaultPath},
		{"snapshot --apply", generation, func(f *fakeFS, v sync.Vaults) error {
			_, err := snapshotApply(f, v)
			return err
		}, "vault " + vaultPath},
		{"snapshot --recover --apply", func(t *testing.T) *fakeFS { return interrupted(t, len(snapshotCalls(t)), false) },
			func(f *fakeFS, v sync.Vaults) error {
				_, err := snapshotRecoverWith(f, v)
				return err
			}, "vault " + vaultPath},
		{"restore --apply", restorable, func(f *fakeFS, v sync.Vaults) error {
			_, err := restoreApply(f, v)
			return err
		}, "destination " + restoreDir},
		{"restore --recover --apply", func(t *testing.T) *fakeFS { return interruptedRestore(t, len(restoreCalls(t)), false) },
			func(f *fakeFS, v sync.Vaults) error {
				_, err := restoreRecoverWith(f, v)
				return err
			}, "destination " + restoreDir},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.fixture(t)
			vaults := newFakeVaults(f)
			reads := len(f.reads)
			err := tt.run(f, beforeOpen{fakeVaults: vaults, hook: func() {
				replaceSaves(f)
				reads = len(f.reads)
			}})
			expectRefusedUnderSession(t, err, vaults, tt.subject, changed)
			if strings.HasPrefix(tt.name, "restore --recover") {
				for _, read := range f.reads[reads:] {
					if strings.Contains(read, `F:\`) {
						t.Fatalf("the vault was read under the session: %q", read)
					}
				}
			}
		})
	}
}

// The creation of an absent root is the only mutation before the checks
// resume on the chain that includes it. A reference that now lies below the
// new root, or an observation that cannot be made, is refused, code 1: the
// root stays, empty, named by the message, and nothing else is written, no
// marker, games, lock or journal, nothing deleted.
func TestCreatedRootIsCheckedBeforeTheLock(t *testing.T) {
	t.Run("installation below the new vault", func(t *testing.T) {
		f := newFakeFS(`D:\`, `F:\`)
		f.mkdir(`D:\Box`)
		aliasRoot(f, `H:\`, `D:\`)
		m := optionalSaves()
		m.Installation.Path = `D:\Box\Vault\Game`
		vaults := newFakeVaults(f)
		_, err := applyThrough(f, m, `H:\Box\Vault`, vaults)
		expectRestoreRefused(t, err, `vault H:\Box\Vault`, `overlaps the installation D:\Box\Vault\Game`, `H:\Box\Vault this run created stays in place`)
		createdOnly(t, vaults, f.get(`D:\Box\Vault`))
	})
	t.Run("held chain unreadable after the creation", func(t *testing.T) {
		f := world()
		vaults := newFakeVaults(f)
		vaults.onCall = func(call string) {
			if call == "CreateDir ." {
				vaults.chainErr = errors.New("held chain unreadable")
			}
		}
		_, err := applyThrough(f, manifest(), vaultPath, vaults)
		expectRestoreRefused(t, err, "vault "+vaultPath, "cannot be checked under the session", "this run created stays in place")
		createdOnly(t, vaults, f.get(vaultPath))
	})
	// F2: an ancestor marked just after the creation is refused by the
	// decision reads of the resumed checks, before the marker of the area,
	// the lock or any other mutation.
	t.Run("restore area, ancestor marked just after the creation", func(t *testing.T) {
		f := restorable(t)
		vaults := newFakeVaults(f)
		data, err := backup.EncodeMarker(backup.Marker{CreatedAt: at, ToolVersion: "dev"})
		if err != nil {
			t.Fatal(err)
		}
		vaults.onCall = func(call string) {
			if call == "CreateDir ." {
				f.put(`G:\Restores\savewarden-vault.json`, f.file(string(data)))
			}
		}
		_, err = restoreApplyWith(f, vaults, restoreDir, restoreAt, "\x9a\x8b\x7c\x6d")
		expectRestoreRefused(t, err, "destination "+restoreDir, `lies in a Savewarden vault or restore area, G:\Restores`, "this run created stays in place")
		createdOnly(t, vaults, f.get(restoreDir))
	})
	t.Run("restore area, held chain unreadable after the creation", func(t *testing.T) {
		f := restorable(t)
		vaults := newFakeVaults(f)
		vaults.onCall = func(call string) {
			if call == "CreateDir ." {
				vaults.chainErr = errors.New("held chain unreadable")
			}
		}
		_, err := restoreApplyWith(f, vaults, restoreDir, restoreAt, "\x9a\x8b\x7c\x6d")
		expectRestoreRefused(t, err, "destination "+restoreDir, "cannot be checked under the session", "this run created stays in place")
		createdOnly(t, vaults, f.get(restoreDir))
	})
}

// createdOnly requires the run to have created its root and nothing else,
// the root left empty.
func createdOnly(t *testing.T, vaults *fakeVaults, root *node) {
	t.Helper()
	if !slices.Equal(vaults.calls, []string{"CreateDir ."}) {
		t.Fatalf("calls %q, want the creation of the root alone", vaults.calls)
	}
	if root == nil || !root.mode.IsDir() || len(root.children) != 0 {
		t.Fatalf("root %+v, want an empty directory left in place", root)
	}
}

// mutating reports a call of the session that writes.
func mutating(call string) bool {
	for _, prefix := range []string{"CreateDir ", "NewFile ", "Write ", "Close ", "Move ", "Lock "} {
		if strings.HasPrefix(call, prefix) {
			return true
		}
	}
	return false
}

// TestSessionOrderIsOpenChecksLockWrites checks the order of plan B5 §4.1
// on the trace of every command and every recovery: Open; the checks,
// starting with the held chain; the creation of an absent root, at once
// followed by the held chain again; the initialization of ADR 0006; the
// lock; the mutations of the operation. No mutation comes before the
// checks, and the lock never before a marker the run validates.
func TestSessionOrderIsOpenChecksLockWrites(t *testing.T) {
	for _, tt := range []struct {
		name  string
		run   func(t *testing.T) *fakeVaults
		reads []string // the decision reads the checks must make first
	}{
		{"sync --apply, new vault", func(*testing.T) *fakeVaults {
			r := apply(world(), vaultPath)
			return r.vaults
		}, []string{"Stat ."}},
		{"sync --apply, rotation", func(t *testing.T) *fakeVaults {
			f := generation(t)
			r := &run{f: f, vaults: newFakeVaults(f), clock: &fakeClock{now: at.Add(timeLater)}}
			return r.again(vaultPath).vaults
		}, []string{"ReadFile savewarden-vault.json"}},
		{"sync --recover --apply", func(t *testing.T) *fakeVaults {
			r := crashRun(len(apply(world(), vaultPath).vaults.calls), false)
			vaults := newFakeVaults(r.f)
			if _, err := recoverWith(r.f, vaults); err != nil {
				t.Fatal(err)
			}
			return vaults
		}, []string{"ReadFile savewarden-vault.json"}},
		{"snapshot --apply", func(t *testing.T) *fakeVaults {
			f := generation(t)
			vaults := newFakeVaults(f)
			if _, err := snapshotApply(f, vaults); err != nil {
				t.Fatal(err)
			}
			return vaults
		}, []string{"ReadFile savewarden-vault.json"}},
		{"snapshot --recover --apply", func(t *testing.T) *fakeVaults {
			f := interrupted(t, len(snapshotCalls(t)), false)
			vaults := newFakeVaults(f)
			if _, err := snapshotRecoverWith(f, vaults); err != nil {
				t.Fatal(err)
			}
			return vaults
		}, []string{"ReadFile savewarden-vault.json"}},
		{"restore --apply, new area", func(t *testing.T) *fakeVaults {
			f := restorable(t)
			vaults := newFakeVaults(f)
			if _, err := restoreApply(f, vaults); err != nil {
				t.Fatal(err)
			}
			return vaults
		}, []string{"MarkedAncestor", "Stat ."}},
		{"restore --recover --apply", func(t *testing.T) *fakeVaults {
			f := interruptedRestore(t, len(restoreCalls(t)), false)
			vaults := newFakeVaults(f)
			if _, err := restoreRecoverWith(f, vaults); err != nil {
				t.Fatal(err)
			}
			return vaults
		}, []string{"MarkedAncestor", "ReadFile savewarden-restore.json"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			trace := tt.run(t).trace
			if len(trace) < 2 || !strings.HasPrefix(trace[0], "Open ") || trace[1] != "Chain" {
				t.Fatalf("the trace does not start with Open, then the held chain: %q", trace)
			}
			first := slices.IndexFunc(trace, mutating)
			if first < 0 {
				t.Fatalf("no write: %q", trace)
			}
			for _, read := range tt.reads {
				if k := slices.Index(trace, read); k < 0 || k > first {
					t.Fatalf("decision read %q missing before the first write: %q", read, trace)
				}
			}
			if k := slices.Index(trace, "CreateDir ."); k >= 0 && (k+1 >= len(trace) || trace[k+1] != "Chain") {
				t.Fatalf("the creation of the root is not followed at once by the held chain: %q", trace)
			}
			lock := slices.IndexFunc(trace, func(call string) bool { return strings.HasPrefix(call, "Lock ") })
			for k, call := range trace {
				marker := strings.Contains(call, backup.MarkerName) || strings.Contains(call, backup.RestoreAreaMarkerName)
				if lock >= 0 && k > lock && marker && mutating(call) {
					t.Fatalf("the lock comes before the marker: %q", trace)
				}
			}
		})
	}
}

// readsSince lists the reads of the plan's adapters made after mark.
func readsSince(f *fakeFS, mark int) []string { return append([]string(nil), f.reads[mark:]...) }

// noReadBelow requires no read of the plan's adapters below path.
func noReadBelow(t *testing.T, reads []string, path string) {
	t.Helper()
	for _, read := range reads {
		if strings.Contains(read, path) {
			t.Fatalf("read through the letter under the session: %q", read)
		}
	}
}

// D3-a: the resumed initialization of a vault reads its root again through
// the session, and requires the very temporaries of the plan, numbers
// included: one added, one removed, or one replaced by another number at the
// same count, is refused, code 1, nothing written; the root is never read
// again through the letter, and an unmarked vault never receives a lock.
func TestVaultInitializationResumeReadsTheRootThroughTheSession(t *testing.T) {
	temporary := func(k int) string { return vaultPath + `\savewarden-vault.json.tmp-` + strconv.Itoa(k) }
	for _, tt := range []struct {
		name    string
		change  func(f *fakeFS)
		mention string
	}{
		{"unchanged", func(*fakeFS) {}, ""},
		{"temporary added", func(f *fakeFS) { f.put(temporary(3), f.file("{")) }, "vault-changed: the vault changed since the recovery plan"},
		{"temporary removed", func(f *fakeFS) { delete(f.get(vaultPath).children, "savewarden-vault.json.tmp-2") },
			"vault-changed: the vault changed since the recovery plan"},
		{"temporary replaced, same count", func(f *fakeFS) {
			delete(f.get(vaultPath).children, "savewarden-vault.json.tmp-2")
			f.put(temporary(4), f.file("{"))
		}, "vault-changed: the vault changed since the recovery plan"},
		{"root unreadable", func(f *fakeFS) { f.get(vaultPath).listErr = errors.New("access denied") },
			"cannot be read again under the session before its marker: access denied"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := world()
			f.mkdir(vaultPath)
			f.put(temporary(1), f.file("{"))
			f.put(temporary(2), f.file("{"))
			vaults := newFakeVaults(f)
			mark := 0
			_, err := service(f, manifest()).Recover(context.Background(), sync.Request{ManifestPath: "m.yaml", VaultPath: vaultPath},
				sync.Writer{Vaults: beforeOpen{fakeVaults: vaults, hook: func() { tt.change(f); mark = len(f.reads) }},
					Clock: &fakeClock{now: at.Add(recoveryLater)}, Random: bytes.NewReader(make([]byte, 4)), ToolVersion: "dev"})
			noReadBelow(t, readsSince(f, mark), vaultPath)
			if slices.IndexFunc(vaults.calls, func(call string) bool { return strings.HasPrefix(call, "Lock ") }) >= 0 {
				t.Fatalf("a lock in an unmarked vault: %q", vaults.calls)
			}
			if tt.mention == "" {
				if err != nil || f.get(vaultPath+`\savewarden-vault.json`) == nil ||
					!slices.Contains(vaults.calls, "Move savewarden-vault.json.tmp-3 savewarden-vault.json") {
					t.Fatalf("resume: %v, calls %q", err, vaults.calls)
				}
				return
			}
			expectRefusedUnderSession(t, err, vaults, "vault "+vaultPath, tt.mention)
		})
	}
}

// D3-b: an area marked by another run between the recovery plan and the
// resume is read through the session, never through the letter, as
// strictly and as stably as readStableArea reads any area: a directory that
// cannot be read is refused, an area that keeps changing is refused, and
// nothing is written in either case.
func TestAreaMarkedMeanwhileIsReadThroughTheSession(t *testing.T) {
	marker := func(t *testing.T) string {
		data, err := backup.EncodeRestoreAreaMarker(backup.RestoreAreaMarker{CreatedAt: restoreAt, ToolVersion: "other", GameID: "synthetic-game"})
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	for _, tt := range []struct {
		name    string
		change  func(t *testing.T, f *fakeFS, v *fakeVaults)
		mention string
	}{
		{"same game, nothing to do", func(t *testing.T, f *fakeFS, _ *fakeVaults) {
			f.put(restoreDir+`\savewarden-restore.json`, f.file(marker(t)))
		}, ""},
		{"receipts unreadable", func(t *testing.T, f *fakeFS, _ *fakeVaults) {
			f.put(restoreDir+`\savewarden-restore.json`, f.file(marker(t)))
			f.mkdir(restoreDir + `\receipts`).listErr = errors.New("access denied")
			f.mkdir(restoreDir + `\journal`)
		}, "access denied"},
		// F1: the marker classified first disappears before the stable
		// reading, which agrees twice on an area of temporaries alone: no
		// marker is established, nothing is concluded.
		{"marker gone before the stable reading", func(t *testing.T, f *fakeFS, v *fakeVaults) {
			f.put(restoreDir+`\savewarden-restore.json`, f.file(marker(t)))
			gone := false
			v.onRead = func(call string) {
				if call == "Stat ." && !gone {
					gone = true
					delete(f.get(restoreDir).children, "savewarden-restore.json")
				}
			}
		}, "which no longer holds a marker"},
		{"keeps changing", func(t *testing.T, f *fakeFS, v *fakeVaults) {
			f.put(restoreDir+`\savewarden-restore.json`, f.file(marker(t)))
			n := 0
			v.onRead = func(call string) {
				if call == "ReadDir ." {
					n++
					f.put(restoreDir+`\`+fmt.Sprintf("stray-%d", n), f.file(""))
				}
			}
		}, "keeps changing while it is read"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := interruptedRestoreAt(t, "Close savewarden-restore.json.tmp-1", true)
			hooks := hookedRestore(f)
			mark := 0
			hooks.before["Open"] = func() { tt.change(t, f, hooks.fakeVaults); mark = len(f.reads) }
			result, err := restoreRecoverWith(f, hooks)
			if len(hooks.calls) != 0 {
				t.Fatalf("written: %q", hooks.calls)
			}
			noReadBelow(t, readsSince(f, mark), restoreDir)
			if tt.mention == "" {
				if err != nil || result.Effect != sync.EffectNone {
					t.Fatalf("nothing to do: %+v, %v", result, err)
				}
				return
			}
			expectRestoreRefused(t, err, "destination "+restoreDir, tt.mention)
		})
	}
}

// afterLock runs change once the session took its lock: what changes
// between the checks before the first write and Z6.
func afterLock(v *fakeVaults, change func()) {
	v.onCall = func(call string) {
		if strings.HasPrefix(call, "Lock ") {
			change()
		}
	}
}

// atRecheck, called once the lock is taken, runs change at the next read of
// the held chain: the reading again of Z6 through the letter is done, and
// the checks under the session start again.
func atRecheck(v *fakeVaults, change func()) {
	done := false
	v.onRead = func(call string) {
		if !done && call == "Chain" {
			done = true
			change()
		}
	}
}

// D3-c: at Z6, under the lock and before Z7, a restore checks the held
// chain, the references and the separation again, and reads its marker
// again, on the trusted observations: those of the checks before the first
// write, extended by the root and the marker this run created. A plan read
// again under the lock never becomes the reference. A change is refused,
// code 1, nothing more written: the marker and the lock already written
// stay, and the state says metadata only.
func TestRestoreChecksAgainUnderTheLock(t *testing.T) {
	replaceSaves := func(f *fakeFS) { f.put(installPath+`\saves`, copyTree(f, f.get(installPath+`\saves`))) }
	for _, tt := range []struct {
		name    string
		marked  bool
		change  func(t *testing.T, f *fakeFS, v *fakeVaults)
		mention string
	}{
		{"new area, reference changed", false, func(_ *testing.T, f *fakeFS, _ *fakeVaults) { replaceSaves(f) },
			`the source saves ` + installPath + `\saves changed since the plan`},
		{"marked area, reference changed", true, func(_ *testing.T, f *fakeFS, _ *fakeVaults) { replaceSaves(f) },
			`the source saves ` + installPath + `\saves changed since the plan`},
		// The marker changes after the reading again of Z6 through the letter,
		// when the checks under the session start again: only they see it.
		// Until the volume base of commit 5, the session reads by the path of
		// its letter: a marker changed behind a remapped letter is not seen.
		{"marked area, marker changed after the reading again", true, func(t *testing.T, f *fakeFS, v *fakeVaults) {
			atRecheck(v, func() {
				data, err := backup.EncodeRestoreAreaMarker(backup.RestoreAreaMarker{CreatedAt: restoreAt.Add(1), ToolVersion: "dev", GameID: "synthetic-game"})
				if err != nil {
					t.Fatal(err)
				}
				f.get(restoreDir + `\savewarden-restore.json`).data = data
			})
		}, "its marker changed since the plan"},
		{"new area, own marker changed after the reading again", false, func(_ *testing.T, f *fakeFS, v *fakeVaults) {
			atRecheck(v, func() { f.get(restoreDir + `\savewarden-restore.json`).data = []byte("{}") })
		}, "its marker changed since the plan"},
		{"new area, held chain unreadable", false, func(_ *testing.T, _ *fakeFS, v *fakeVaults) { v.chainErr = errors.New("held chain unreadable") },
			"cannot be checked under the session"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := restorable(t)
			if tt.marked {
				markArea(t, f, restoreDir, "synthetic-game")
			}
			vaults := newFakeVaults(f)
			afterLock(vaults, func() { tt.change(t, f, vaults) })
			result, err := restoreApply(f, vaults)
			expectRestoreRefused(t, err, "destination "+restoreDir, tt.mention, "refused under the lock, nothing more is written")
			if !strings.HasPrefix(vaults.calls[len(vaults.calls)-1], "Lock ") {
				t.Fatalf("written after the lock: %q", vaults.calls)
			}
			if result.Effect != sync.EffectMetadata {
				t.Fatalf("effect %v, want metadata only: the marker or the lock was written", result.Effect)
			}
		})
	}
	t.Run("witness: the marker this run validated is read again, not taken for a change", func(t *testing.T) {
		f := restorable(t)
		vaults := newFakeVaults(f)
		if result, err := restoreApply(f, vaults); err != nil || result.Effect != sync.EffectPublished {
			t.Fatalf("%+v, %v", result, err)
		}
		lock := slices.Index(vaults.trace, "Lock restore.lock")
		structure := slices.Index(vaults.trace, "CreateDir receipts")
		read := slices.Index(vaults.trace[lock+1:], "ReadFile savewarden-restore.json")
		if lock < 0 || structure < 0 || read < 0 || lock+1+read > structure || !slices.Contains(vaults.trace[lock+1:structure], "Chain") {
			t.Fatalf("no check between the lock and Z7: %q", vaults.trace)
		}
	})
}

// D3-d: the recovery of a restore operation checks again, under the lock,
// on the trusted observations, before the recovery plan read again under
// it replaces the plan; it never reads the vault.
func TestRestoreRecoveryChecksAgainUnderTheLock(t *testing.T) {
	f := interruptedRestore(t, len(restoreCalls(t)), false)
	vaults := newFakeVaults(f)
	mark := 0
	afterLock(vaults, func() {
		f.put(installPath+`\saves`, copyTree(f, f.get(installPath+`\saves`)))
		mark = len(f.reads)
	})
	_, err := restoreRecoverWith(f, vaults)
	expectRestoreRefused(t, err, "destination "+restoreDir, `the source saves `+installPath+`\saves changed since the plan`,
		"refused under the lock, nothing more is written")
	if !slices.Equal(vaults.calls, []string{"Lock restore.lock"}) {
		t.Fatalf("calls %q, want the lock alone", vaults.calls)
	}
	noReadBelow(t, readsSince(f, mark), vaultPath)
}

// switchingManifests serves *current, which a test changes between the
// recovery plan and its reading again under the lock.
type switchingManifests struct{ current *game.Manifest }

func (s switchingManifests) Read(string) (game.Manifest, error) {
	return *s.current, s.current.Validate()
}

// F3: the recovery plan read again under the lock rebuilds its references
// from the manifest, read again, and from the plans journaled in the area.
// A set that differs from the one the checks validated, a source added to
// the manifest or removed from it, is refused before the plan read again is
// adopted: nothing more is written than the lock, and the vault is never
// read.
func TestRestoreRecoveryRefusesReferencesChangedUnderTheLock(t *testing.T) {
	withExtra := manifest(source("saves", "saves", true), source("extra", "extra", false))
	for _, tt := range []struct {
		name          string
		first, second game.Manifest
	}{
		{"source added to the manifest", manifest(), withExtra},
		{"source removed from the manifest", withExtra, manifest()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := interruptedRestore(t, len(restoreCalls(t)), false)
			current := tt.first
			service := sync.New(switchingManifests{&current}, inspect.New(nil, inspectFS{f}), syncFiles{f}, fakeVolumes{f})
			vaults := newFakeVaults(f)
			mark := 0
			afterLock(vaults, func() { current, mark = tt.second, len(f.reads) })
			_, err := service.RestoreRecover(context.Background(), restoreRecoverRequest(),
				restoreWriter(vaults, restoreAt.Add(timeLater), "\x00\x00\x00\x00"))
			expectRestoreRefused(t, err, "destination "+restoreDir, "the references of the recovery, from the manifest or the plans journaled in the area, changed")
			if !slices.Equal(vaults.calls, []string{"Lock restore.lock"}) {
				t.Fatalf("calls %q, want the lock alone", vaults.calls)
			}
			noReadBelow(t, readsSince(f, mark), vaultPath)
		})
	}
}

// F3-b: at Z6, restore --apply reads the plan again under the lock, its
// references rebuilt from the manifest, read again, and from the receipt of
// the snapshot. A source added to the manifest or removed from it between
// the plan and Z6 is refused before Z7, nothing more written than the
// marker and the lock; the same set in another order is accepted.
func TestRestoreRefusesReferencesChangedUnderTheLock(t *testing.T) {
	withExtra := manifest(source("saves", "saves", true), source("extra", "extra", false))
	reordered := manifest(source("extra", "extra", false), source("saves", "saves", true))
	for _, tt := range []struct {
		name          string
		first, second game.Manifest
		refused       bool
	}{
		{"source added to the manifest", manifest(), withExtra, true},
		{"source removed from the manifest", withExtra, manifest(), true},
		{"same sources in another order", withExtra, reordered, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := restorable(t)
			current := tt.first
			service := sync.New(switchingManifests{&current}, inspect.New(nil, inspectFS{f}), syncFiles{f}, fakeVolumes{f})
			vaults := newFakeVaults(f)
			afterLock(vaults, func() { current = tt.second })
			result, err := service.RestoreApply(context.Background(), restoreRequest(restoreDir), restoreWriter(vaults, restoreAt, "\x9a\x8b\x7c\x6d"))
			if !tt.refused {
				if err != nil || result.Effect != sync.EffectPublished {
					t.Fatalf("same references in another order: %+v, %v", result, err)
				}
				return
			}
			expectRestoreRefused(t, err, "destination "+restoreDir, "the references of the restore, from the manifest or the snapshot, changed since the plan")
			if !strings.HasPrefix(vaults.calls[len(vaults.calls)-1], "Lock ") || slices.Contains(vaults.calls, "CreateDir receipts") {
				t.Fatalf("written after the lock: %q", vaults.calls)
			}
			if result.Effect != sync.EffectMetadata {
				t.Fatalf("effect %v, want metadata only: the marker and the lock were written", result.Effect)
			}
		})
	}
}

// The marked ancestors are a decision read of the session: a vault marker
// that appears above the area between the plan and the session is refused
// under the session, nothing written.
func TestMarkedAncestorIsCheckedUnderTheSession(t *testing.T) {
	f := restorable(t)
	vaults := newFakeVaults(f)
	data, err := backup.EncodeMarker(backup.Marker{CreatedAt: at, ToolVersion: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = restoreApplyWith(f, beforeOpen{fakeVaults: vaults, hook: func() { f.put(`G:\Restores\savewarden-vault.json`, f.file(string(data))) }},
		restoreDir, restoreAt, "\x9a\x8b\x7c\x6d")
	expectRefusedUnderSession(t, err, vaults, "destination "+restoreDir, `lies in a Savewarden vault or restore area, G:\Restores`)
}
