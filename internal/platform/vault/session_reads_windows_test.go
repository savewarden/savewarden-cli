//go:build windows

package vault_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

// idOf reads the identity of path, opened without following it.
func idOf(t *testing.T, path string) backup.FileID {
	t.Helper()
	handle := hwHandle(t, path)
	var data windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &data); err != nil {
		t.Fatal(err)
	}
	return backup.FileID{Volume: data.VolumeSerialNumber, Index: uint64(data.FileIndexHigh)<<32 | uint64(data.FileIndexLow)}
}

// chainOf lists the identities of path and of every ancestor, volume root
// first.
func chainOf(t *testing.T, path string) []backup.FileID {
	t.Helper()
	volume := filepath.VolumeName(path)
	ids := []backup.FileID{idOf(t, volume+`\`)}
	current := volume + `\`
	for _, segment := range strings.Split(strings.TrimPrefix(path, volume+`\`), `\`) {
		current = filepath.Join(current, segment)
		ids = append(ids, idOf(t, current))
	}
	return ids
}

// The held chain is read on the session's own handles: from the volume
// root down to the vault, or to its parent while the vault is absent, then
// extended by the root the session creates.
func TestSessionChainIsReadOnTheHeldHandles(t *testing.T) {
	parent := filepath.Join(filepath.Clean(t.TempDir()), "Parent")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	vault := filepath.Join(parent, "Vault")
	s := open(t, vault)
	held, err := s.Chain()
	if err != nil {
		t.Fatal(err)
	}
	if want := chainOf(t, parent); !slices.Equal(held, want) {
		t.Fatalf("absent vault: held chain %v, want its parent's %v", held, want)
	}
	if err := s.CreateDir("."); err != nil {
		t.Fatal(err)
	}
	if held, err = s.Chain(); err != nil {
		t.Fatal(err)
	}
	if want := chainOf(t, vault); !slices.Equal(held, want) {
		t.Fatalf("created vault: held chain %v, want %v", held, want)
	}
}

// A letter given to another directory after the session opened does not
// change the chain it holds: the session reads its pins, never the letter.
func TestSessionChainIgnoresALetterRemappedAfterOpen(t *testing.T) {
	root := hwTemp(t)
	for _, side := range []string{"A", "B"} {
		hwMkdir(t, filepath.Join(root, side, "Vault"))
	}
	letter := hwReserveLetter(t)
	letter.subst(t, filepath.Join(root, "A"))
	s := open(t, letter.name+`\Vault`)
	letter.remove(t)
	letter.subst(t, filepath.Join(root, "B"))
	held, err := s.Chain()
	if err != nil {
		t.Fatal(err)
	}
	want := []backup.FileID{idOf(t, filepath.Join(root, "A")), idOf(t, filepath.Join(root, "A", "Vault"))}
	if !slices.Equal(held, want) {
		t.Fatalf("held chain %v, want the directories pinned at Open %v", held, want)
	}
	if now := idOf(t, letter.name+`\Vault`); slices.Contains(held, now) {
		t.Fatalf("the held chain follows the letter, now %v", now)
	}
}

// The decision reads of the session follow nothing and write nothing: what
// a name is, the entries of a directory, the content of a file within a
// limit, and the depth of a marked ancestor.
func TestSessionDecisionReads(t *testing.T) {
	base := filepath.Clean(t.TempDir())
	outside := filepath.Join(base, "outside")
	marked := filepath.Join(base, "Marked")
	vault := filepath.Join(marked, "Inner", "Vault")
	for _, dir := range []string{outside, filepath.Join(vault, "dir")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for path, data := range map[string]string{
		filepath.Join(vault, "f"):                        "content",
		filepath.Join(outside, "secret"):                 "outside",
		filepath.Join(marked, "savewarden-restore.json"): "{}",
	} {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	junction(t, filepath.Join(vault, "jn"), outside)
	before := fingerprint(t, base)
	s := open(t, vault)

	for name, want := range map[string]fs.FileMode{".": fs.ModeDir, "dir": fs.ModeDir, "f": 0, "jn": fs.ModeIrregular} {
		if got, err := s.Stat(name); err != nil || got != want {
			t.Errorf("Stat(%s): %v, %v; want %v", name, got, err, want)
		}
	}
	if _, err := s.Stat("absent"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(absent): %v, want fs.ErrNotExist", err)
	}

	entries, err := s.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]fs.FileMode{}
	for _, entry := range entries {
		kinds[entry.Name()] = entry.Type()
	}
	if len(kinds) != 3 || !kinds["dir"].IsDir() || !kinds["f"].IsRegular() || kinds["jn"]&(fs.ModeSymlink|fs.ModeIrregular) == 0 {
		t.Errorf("ReadDir: %v", kinds)
	}
	if _, err := s.ReadDir("jn"); !notFollowed(err) {
		t.Errorf("ReadDir through a junction: %v, want a refusal", err)
	}

	if data, err := s.ReadFile("f", 64); err != nil || string(data) != "content" {
		t.Errorf("ReadFile: %q, %v", data, err)
	}
	if _, err := s.ReadFile("f", 3); err == nil {
		t.Error("ReadFile beyond its limit succeeded")
	}
	if _, err := s.ReadFile("jn/secret", 64); err == nil {
		t.Error("ReadFile through a junction succeeded")
	}
	if _, err := s.ReadFile("dir", 64); err == nil {
		t.Error("ReadFile of a directory succeeded")
	}

	depth, err := s.MarkedAncestor("savewarden-vault.json", "savewarden-restore.json")
	if want := len(strings.Split(strings.TrimPrefix(marked, filepath.VolumeName(marked)+`\`), `\`)); err != nil || depth != want {
		t.Errorf("MarkedAncestor: %d, %v; want %d", depth, err, want)
	}
	if depth, err := s.MarkedAncestor("absent-marker"); err != nil || depth != -1 {
		t.Errorf("MarkedAncestor without marker: %d, %v", depth, err)
	}

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if fingerprint(t, base) != before {
		t.Fatal("a read of the session changed the tree")
	}
}

// An absent vault reads as absent below its held parent.
func TestSessionStatOfAnAbsentVault(t *testing.T) {
	s := open(t, filepath.Join(filepath.Clean(t.TempDir()), "Vault"))
	if _, err := s.Stat("."); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Stat(.) of an absent vault: %v", err)
	}
}
