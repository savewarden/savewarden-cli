//go:build windows

package filesystem_test

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/platform/filesystem"
)

func junction(t *testing.T, link, target string) {
	t.Helper()
	output, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		t.Fatalf("mklink /J: %v: %s", err, output)
	}
}

// A junction inside the anchor points back inside it. Junction targets are
// absolute, which os.Root already treats as an escape; the scope must refuse
// the junction itself at every entry point, whatever os.Root does.
func TestRefusesJunctionInsideTheAnchor(t *testing.T) {
	anchor := tempAnchor(t)
	write(t, filepath.Join(anchor, "real", "file.save"), "target")
	junction(t, filepath.Join(anchor, "jn"), filepath.Join(anchor, "real"))
	before := fingerprint(t, filepath.Join(anchor, "real"))

	root, err := os.OpenRoot(anchor)
	if err != nil {
		t.Fatal(err)
	}
	if file, err := root.Open(`jn\file.save`); err == nil {
		_ = file.Close()
		t.Log("os.Root alone reads through the junction; the scope must not")
	} else {
		t.Logf("os.Root refused the junction itself: %v", err)
	}
	_ = root.Close()

	scope := open(t, anchor)
	info, err := scope.Lstat("jn")
	if err != nil || info.Mode()&(fs.ModeSymlink|fs.ModeIrregular) == 0 || info.IsDir() {
		t.Fatalf("junction not reported as a link: %v %v", info, err)
	}
	for name, call := range map[string]func() error{
		"ReadDir jn":            func() error { _, err := scope.ReadDir("jn"); return err },
		"Lstat jn/file.save":    func() error { _, err := scope.Lstat("jn/file.save"); return err },
		"OpenRegular jn/file":   func() error { _, err := scope.OpenRegular("jn/file.save"); return err },
		"OpenRegular jn itself": func() error { _, err := scope.OpenRegular("jn"); return err },
	} {
		if err := call(); !notFollowed(err) {
			t.Errorf("%s: %v, want a not-followed refusal", name, err)
		}
	}
	if after := fingerprint(t, filepath.Join(anchor, "real")); after != before {
		t.Fatal("target changed")
	}
}

// A directory checked by Lstat is swapped for a junction before it is used.
func TestDirectorySwappedForJunctionAfterLstat(t *testing.T) {
	anchor := tempAnchor(t)
	write(t, filepath.Join(anchor, "d", "file.save"), "original")
	write(t, filepath.Join(anchor, "elsewhere", "file.save"), "attacker")
	scope := open(t, anchor)
	if info, err := scope.Lstat("d"); err != nil || !info.IsDir() {
		t.Fatalf("lstat d: %v %v", info, err)
	}
	if err := os.Rename(filepath.Join(anchor, "d"), filepath.Join(anchor, "d-moved")); err != nil {
		t.Fatalf("swap: %v", err)
	}
	junction(t, filepath.Join(anchor, "d"), filepath.Join(anchor, "elsewhere"))
	if _, err := scope.OpenRegular("d/file.save"); !notFollowed(err) {
		t.Fatalf("read through a swapped junction: %v", err)
	}
	if _, err := scope.ReadDir("d"); !notFollowed(err) {
		t.Fatalf("listed a swapped junction: %v", err)
	}
}

// Once traversed, a directory is pinned: it cannot be renamed, so it cannot
// be exchanged for a junction while the scope is open.
func TestTraversedDirectoriesArePinned(t *testing.T) {
	anchor := tempAnchor(t)
	write(t, filepath.Join(anchor, "p", "q", "file.save"), "x")
	scope, err := filesystem.OpenScope(anchor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scope.Lstat("p/q/file.save"); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"p", filepath.Join("p", "q")} {
		if err := os.Rename(filepath.Join(anchor, dir), filepath.Join(anchor, "moved")); err == nil {
			t.Fatalf("pinned directory %s was renamed", dir)
		}
	}
	if err := os.WriteFile(filepath.Join(anchor, "p", "q", "new.save"), []byte("game writes"), 0o600); err != nil {
		t.Fatalf("pins must not block writes inside directories: %v", err)
	}
	if err := scope.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(anchor, "p"), filepath.Join(anchor, "moved")); err != nil {
		t.Fatalf("pin not released by Close: %v", err)
	}
}

// A file checked by Lstat is replaced before being opened.
func TestFileReplacedAfterLstat(t *testing.T) {
	anchor := tempAnchor(t)
	write(t, filepath.Join(anchor, "a.save"), "original")
	write(t, filepath.Join(anchor, "b.save"), "other")
	scope := open(t, anchor)
	before, err := scope.Lstat("a.save")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(anchor, "a.save"), filepath.Join(anchor, "a-old.save")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(anchor, "b.save"), filepath.Join(anchor, "a.save")); err == nil {
		if _, err := scope.OpenRegular("a.save"); !notFollowed(err) {
			t.Fatalf("opened a file swapped for a symbolic link: %v", err)
		}
		if err := os.Remove(filepath.Join(anchor, "a.save")); err != nil {
			t.Fatal(err)
		}
	} else if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Fatalf("symbolic link creation is required on CI: %v", err)
	} else {
		t.Logf("symbolic links unavailable, link swap skipped: %v", err)
	}
	if err := os.Link(filepath.Join(anchor, "b.save"), filepath.Join(anchor, "a.save")); err != nil {
		t.Fatal(err)
	}
	file, err := scope.OpenRegular("a.save")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if scope.SameFile(before, opened) {
		t.Fatal("a replaced file kept the identity of the original")
	}
}

func TestLockedFileIsReportedNotRead(t *testing.T) {
	anchor := tempAnchor(t)
	path := filepath.Join(anchor, "locked.save")
	write(t, path, "x")
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.CloseHandle(handle) }()
	scope := open(t, anchor)
	_, err = scope.OpenRegular("locked.save")
	if err == nil || notFollowed(err) || !strings.Contains(err.Error(), "locked by another process") {
		t.Fatalf("error = %v", err)
	}
}

// Cloud placeholders cannot be created in a test; the offline attribute
// exercises the same refusal path.
func TestOfflinePlaceholderIsNeverOpened(t *testing.T) {
	anchor := tempAnchor(t)
	path := filepath.Join(anchor, "placeholder.save")
	write(t, path, "remote")
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	const offline = 0x00001000
	if err := syscall.SetFileAttributes(name, syscall.FILE_ATTRIBUTE_ARCHIVE|offline); err != nil {
		t.Skipf("offline attribute unavailable: %v", err)
	}
	t.Cleanup(func() { _ = syscall.SetFileAttributes(name, syscall.FILE_ATTRIBUTE_NORMAL) })
	scope := open(t, anchor)
	info, err := scope.Lstat("placeholder.save")
	if err != nil || info.Mode()&fs.ModeIrregular == 0 {
		t.Fatalf("placeholder not reported as irregular: %v %v", info, err)
	}
	if _, err := scope.OpenRegular("placeholder.save"); !notFollowed(err) {
		t.Fatalf("placeholder opened: %v", err)
	}
}

func TestLongPathsAndCaseInsensitiveIdentity(t *testing.T) {
	anchor := tempAnchor(t)
	segment := strings.Repeat("répertoire-long-", 6)
	deep := filepath.Join(anchor, segment, segment, segment, "Dir")
	write(t, filepath.Join(deep, "file.save"), "deep")
	if len(filepath.Join(deep, "file.save")) <= 260 {
		t.Fatalf("fixture path too short: %d", len(deep))
	}
	scope := open(t, anchor)
	name := strings.Join([]string{segment, segment, segment, "Dir", "file.save"}, "/")
	file, err := scope.OpenRegular(name)
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(file)
	_ = file.Close()
	if err != nil || string(content) != "deep" {
		t.Fatalf("content %q %v", content, err)
	}
	upper, err := scope.Lstat(strings.Join([]string{segment, segment, segment, "Dir"}, "/"))
	if err != nil {
		t.Fatal(err)
	}
	lower, err := scope.Lstat(strings.Join([]string{segment, segment, segment, "dir"}, "/"))
	if err != nil {
		t.Fatal(err)
	}
	if !scope.SameFile(upper, lower) {
		t.Fatal("two spellings of one directory not recognised as the same")
	}
}

func TestUnavailableVolumeAndJunctionAnchor(t *testing.T) {
	for letter := 'Z'; letter >= 'D'; letter-- {
		volume := string(letter) + `:\`
		if _, err := os.Stat(volume); err == nil {
			continue
		}
		if _, err := filesystem.OpenScope(volume); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("missing volume %s: %v", volume, err)
		}
		break
	}
	anchor := tempAnchor(t)
	if err := os.Mkdir(filepath.Join(anchor, "real"), 0o700); err != nil {
		t.Fatal(err)
	}
	junction(t, filepath.Join(anchor, "jn"), filepath.Join(anchor, "real"))
	if _, err := filesystem.OpenScope(filepath.Join(anchor, "jn")); !notFollowed(err) {
		t.Fatalf("junction accepted as an anchor: %v", err)
	}
	if _, err := filesystem.OpenScope(`relative\path`); err == nil {
		t.Fatal("relative anchor accepted")
	}
}
