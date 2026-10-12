//go:build windows || unix

package filesystem_test

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/features/inspect"
	"github.com/savewarden/savewarden-cli/internal/platform/filesystem"
)

// The concrete scope must satisfy the feature port.
var _ inspect.Scope = (*filesystem.Scope)(nil)

// tempAnchor returns a clean absolute temporary directory; anchors must be
// clean, and TMP may hold mixed separators on Windows.
func tempAnchor(t *testing.T) string {
	t.Helper()
	return filepath.Clean(t.TempDir())
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func open(t *testing.T, anchor string) *filesystem.Scope {
	t.Helper()
	scope, err := filesystem.OpenScope(anchor)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = scope.Close() })
	return scope
}

// requireOnCI skips a test locally when the OS refuses a fixture, such as a
// symbolic link without privilege, but fails it on CI where it must run.
func requireOnCI(t *testing.T, what string, err error) {
	t.Helper()
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Fatalf("%s is required on CI: %v", what, err)
	}
	t.Skipf("%s unavailable here: %v", what, err)
}

func notFollowed(err error) bool {
	var refusal interface{ NotFollowed() bool }
	return errors.As(err, &refusal) && refusal.NotFollowed()
}

// fingerprint records every path, mode, size, time and content below root.
// It is taken once the fixture is written, so it measures only the scope.
func fingerprint(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Lstat reads the object itself: on NTFS, the copy of a directory's
		// times held in its parent's listing is updated lazily.
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		line := fmt.Sprintf("%s|%v|%d|%d", path, info.Mode(), info.Size(), info.ModTime().UnixNano())
		if info.Mode().IsRegular() {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			line += fmt.Sprintf("|%x", sha256.Sum256(content))
		}
		lines = append(lines, line)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func TestReadsRegularFilesWithoutMutation(t *testing.T) {
	anchor := tempAnchor(t)
	write(t, filepath.Join(anchor, "jeu synthétique", "saves", "Profil été", "1.save"), "one")
	write(t, filepath.Join(anchor, "jeu synthétique", "saves", "オ.save"), "two")
	write(t, filepath.Join(anchor, "jeu synthétique", "saves", "empty.save"), "")
	before := fingerprint(t, anchor)

	scope := open(t, anchor)
	entries, err := scope.ReadDir("jeu synthétique/saves")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	if strings.Join(names, "|") != "Profil été|empty.save|オ.save" {
		t.Fatalf("names = %q", names)
	}
	name := "jeu synthétique/saves/Profil été/1.save"
	info, err := scope.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Size() != 3 {
		t.Fatalf("lstat: %v %v", info, err)
	}
	file, err := scope.OpenRegular(name)
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(file)
	if err != nil || string(content) != "one" {
		t.Fatalf("content %q %v", content, err)
	}
	opened, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if !scope.SameFile(info, opened) {
		t.Fatal("same file not recognised")
	}
	other, err := scope.Lstat("jeu synthétique/saves/オ.save")
	if err != nil || scope.SameFile(info, other) {
		t.Fatalf("different files confused: %v", err)
	}
	if err := scope.Close(); err != nil {
		t.Fatal(err)
	}
	if after := fingerprint(t, anchor); after != before {
		t.Fatalf("tree changed:\n%s\n---\n%s", before, after)
	}
}

func TestRefusesInvalidNamesAndWrongTypes(t *testing.T) {
	anchor := tempAnchor(t)
	write(t, filepath.Join(anchor, "dir", "file.save"), "x")
	scope := open(t, anchor)
	for _, name := range []string{"", "/abs", "../outside", "dir/../file", `dir\file.save`, "dir/file.save:stream", "dir//file.save", "dir/"} {
		if _, err := scope.Lstat(name); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("Lstat(%q) = %v, want fs.ErrInvalid", name, err)
		}
		if _, err := scope.OpenRegular(name); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("OpenRegular(%q) = %v, want fs.ErrInvalid", name, err)
		}
	}
	if _, err := scope.OpenRegular("dir"); err == nil {
		t.Error("a directory was opened as a regular file")
	}
	if _, err := scope.ReadDir("dir/file.save"); err == nil {
		t.Error("a file was listed as a directory")
	}
	if _, err := scope.Lstat("dir/missing.save"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing file: %v", err)
	}
	if _, err := filesystem.OpenScope(filepath.Join(anchor, "missing")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing anchor: %v", err)
	}
}

// os.Root confines names but follows links that stay inside the root. The
// scope must refuse them anyway: this test fails with os.Root alone.
func TestRefusesSymlinksInsideTheAnchor(t *testing.T) {
	anchor := tempAnchor(t)
	write(t, filepath.Join(anchor, "real", "file.save"), "target")
	// Relative targets stay inside the root, which os.Root follows.
	if err := os.Symlink("real", filepath.Join(anchor, "dirlink")); err != nil {
		requireOnCI(t, "symbolic link creation", err)
	}
	if err := os.Symlink(filepath.Join("real", "file.save"), filepath.Join(anchor, "filelink")); err != nil {
		requireOnCI(t, "symbolic link creation", err)
	}
	root, err := os.OpenRoot(anchor)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	for _, name := range []string{"dirlink/file.save", "filelink"} {
		if file, err := root.Open(filepath.FromSlash(name)); err != nil {
			t.Fatalf("premise: os.Root should follow %s inside the root: %v", name, err)
		} else {
			_ = file.Close()
		}
	}

	scope := open(t, anchor)
	if info, err := scope.Lstat("filelink"); err != nil || info.Mode()&(fs.ModeSymlink|fs.ModeIrregular) == 0 {
		t.Fatalf("filelink not reported as a link: %v %v", info, err)
	}
	if _, err := scope.OpenRegular("filelink"); !notFollowed(err) {
		t.Fatalf("file link opened: %v", err)
	}
	if _, err := scope.OpenRegular("dirlink/file.save"); !notFollowed(err) {
		t.Fatalf("file reached through a directory link: %v", err)
	}
	if _, err := scope.ReadDir("dirlink"); !notFollowed(err) {
		t.Fatalf("directory link listed: %v", err)
	}
	if _, err := scope.Lstat("dirlink/file.save"); !notFollowed(err) {
		t.Fatalf("lstat through a directory link: %v", err)
	}
}
