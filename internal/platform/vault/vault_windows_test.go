//go:build windows

package vault_test

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/platform/vault"
)

func open(t *testing.T, path string) *vault.Session {
	t.Helper()
	s, err := vault.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// newVault opens a session on an absent vault and creates it.
func newVault(t *testing.T) (string, *vault.Session) {
	t.Helper()
	base := filepath.Join(filepath.Clean(t.TempDir()), "Vault")
	s := open(t, base)
	if err := s.CreateDir("."); err != nil {
		t.Fatal(err)
	}
	return base, s
}

func create(t *testing.T, s *vault.Session, name, content string, modTime time.Time) {
	t.Helper()
	w, err := s.CreateFile(name, modTime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, content); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func dirs(t *testing.T, s *vault.Session, names ...string) {
	t.Helper()
	for _, name := range names {
		if err := s.CreateDir(name); err != nil {
			t.Fatal(err)
		}
	}
}

func content(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func errno(err error) windows.Errno {
	var code windows.Errno
	if errors.As(err, &code) {
		return code
	}
	return 0
}

func notFollowed(err error) bool {
	var refusal interface{ NotFollowed() bool }
	return errors.As(err, &refusal) && refusal.NotFollowed()
}

func junction(t *testing.T, link, target string) {
	t.Helper()
	output, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		t.Fatalf("mklink /J: %v: %s", err, output)
	}
}

// fingerprint lists every entry below root with its mode, size, date and
// content, without following anything.
func fingerprint(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		line := fmt.Sprintf("%s|%v|%d|%d", path, info.Mode(), info.Size(), info.ModTime().UnixNano())
		if info.Mode().IsRegular() {
			line += "|" + content(t, path)
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

func TestCreationIsExclusive(t *testing.T) {
	base, s := newVault(t)
	if err := s.CreateDir("."); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("vault created twice: %v", err)
	}
	create(t, s, "a.save", "one", time.Time{})
	if _, err := s.CreateFile("a.save", time.Time{}); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("CREATE_NEW over an existing file: %v", err)
	}
	dirs(t, s, "d")
	if err := os.Mkdir(filepath.Join(base, "made-elsewhere"), 0o700); err != nil {
		t.Fatal(err)
	}
	before := fingerprint(t, base)
	for name, err := range map[string]error{
		"directory twice":          s.CreateDir("d"),
		"directory over a file":    s.CreateDir("a.save"),
		"directory made elsewhere": s.CreateDir("made-elsewhere"),
	} {
		if !errors.Is(err, fs.ErrExist) {
			t.Errorf("%s: %v, want fs.ErrExist", name, err)
		}
	}
	// Windows answers CREATE_NEW on a directory name with access denied,
	// error 5: refused all the same.
	for name, err := range map[string]error{
		"file over a directory":      func() error { _, err := s.CreateFile("d", time.Time{}); return err }(),
		"file over a made directory": func() error { _, err := s.CreateFile("made-elsewhere", time.Time{}); return err }(),
	} {
		if err == nil {
			t.Errorf("%s: created", name)
		}
		t.Logf("%s refused with error %d", name, errno(err))
	}
	if after := fingerprint(t, base); after != before {
		t.Fatalf("refused creations changed the vault:\n%s\n---\n%s", before, after)
	}
	if got := content(t, filepath.Join(base, "a.save")); got != "one" {
		t.Fatalf("existing file changed: %q", got)
	}
}

func TestFileIsFlushedDatedAndClosedOnce(t *testing.T) {
	base, s := newVault(t)
	modTime := time.Date(2026, 10, 9, 16, 32, 8, 123456700, time.UTC)
	w, err := s.CreateFile("slot.save", modTime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "progress"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("more")); !errors.Is(err, fs.ErrClosed) {
		t.Fatalf("write after close: %v", err)
	}
	if err := w.Close(); !errors.Is(err, fs.ErrClosed) {
		t.Fatalf("second close: %v", err)
	}
	info, err := os.Stat(filepath.Join(base, "slot.save"))
	if err != nil {
		t.Fatal(err)
	}
	// NTFS keeps dates at 100 ns.
	if !info.ModTime().Equal(modTime) {
		t.Fatalf("date %v, want %v", info.ModTime().UTC(), modTime)
	}
	if got := content(t, filepath.Join(base, "slot.save")); got != "progress" {
		t.Fatalf("content %q", got)
	}
}

// MoveFileEx with flags 0 never replaces a target, file or directory.
func TestMoveNeverReplaces(t *testing.T) {
	base, s := newVault(t)
	create(t, s, "a.save", "one", time.Time{})
	create(t, s, "b.save", "two", time.Time{})
	dirs(t, s, "d", "empty", "full")
	create(t, s, "d/inside.save", "d", time.Time{})
	create(t, s, "full/inside.save", "full", time.Time{})
	for _, name := range []string{"d", "empty", "full"} {
		if err := s.Release(name); err != nil {
			t.Fatal(err)
		}
	}
	before := fingerprint(t, base)
	for _, move := range [][2]string{
		{"a.save", "b.save"}, {"a.save", "empty"}, {"d", "empty"}, {"d", "full"}, {"d", "a.save"},
	} {
		err := s.Move(move[0], move[1])
		if !errors.Is(err, fs.ErrExist) {
			t.Fatalf("%s onto %s: %v, want fs.ErrExist", move[0], move[1], err)
		}
		t.Logf("move %s onto existing %s refused with error %d", move[0], move[1], errno(err))
	}
	if after := fingerprint(t, base); after != before {
		t.Fatalf("vault changed by refused moves:\n%s\n---\n%s", before, after)
	}
	if err := s.Move("a.save", "c.save"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(base, "a.save")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("source still present: %v", err)
	}
	if got := content(t, filepath.Join(base, "c.save")); got != "one" {
		t.Fatalf("moved content %q", got)
	}
}

// ADR 0006 §4, P1-3: a parent pinned without FILE_SHARE_DELETE does not
// prevent renaming a child, and the identity survives the rename.
func TestParentPinsDoNotBlockChildRename(t *testing.T) {
	base, s := newVault(t)
	dirs(t, s, "games", "games/g", "games/g/staging", "games/g/staging/op")
	create(t, s, "games/g/staging/op/slot.save", "s", time.Time{})
	if err := s.Move("games/g/staging/op", "games/g/current"); err == nil || !strings.Contains(err.Error(), "held by the session") {
		t.Fatalf("held source moved: %v", err)
	}
	if err := s.Release("games/g/staging/op"); err != nil {
		t.Fatal(err)
	}
	staged, err := s.Identity("games/g/staging/op")
	if err != nil {
		t.Fatal(err)
	}
	if staged == (backup.FileID{}) {
		t.Fatal("empty identity")
	}
	if err := s.Move("games/g/staging/op", "games/g/current"); err != nil {
		t.Fatalf("rename below pinned parents: %v", err)
	}
	published, err := s.Identity("games/g/current")
	if err != nil || published != staged {
		t.Fatalf("identity after rename %v, %v; want %v", published, err, staged)
	}
	if _, err := s.Identity("games/g/staging/op"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("source name still present: %v", err)
	}
	if got := content(t, filepath.Join(base, "games", "g", "current", "slot.save")); got != "s" {
		t.Fatalf("content %q", got)
	}
}

// openPin opens a directory as a read scope does, without FILE_SHARE_DELETE.
func openPin(t *testing.T, path string) windows.Handle {
	t.Helper()
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(pointer, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		t.Fatal(err)
	}
	return handle
}

// A read scope left open on the directory to move blocks the rename with a
// sharing violation, reported as busy; an open file inside it blocks the
// rename too. The codes observed are recorded for ADR 0006.
func TestOpenHandlesBlockTheRename(t *testing.T) {
	base, s := newVault(t)
	dirs(t, s, "staging")
	create(t, s, "staging/slot.save", "s", time.Time{})
	if err := s.Release("staging"); err != nil {
		t.Fatal(err)
	}

	pin := openPin(t, filepath.Join(base, "staging"))
	err := s.Move("staging", "current")
	_ = windows.CloseHandle(pin)
	var busy *vault.BusyError
	if !errors.As(err, &busy) || errno(err) != windows.ERROR_SHARING_VIOLATION {
		t.Fatalf("rename with the directory pinned: error %d, %v; want busy, 32", errno(err), err)
	}
	t.Logf("directory held without FILE_SHARE_DELETE: rename refused with error %d", errno(err))

	file, err := os.Open(filepath.Join(base, "staging", "slot.save"))
	if err != nil {
		t.Fatal(err)
	}
	err = s.Move("staging", "current")
	_ = file.Close()
	// Observed: access denied, error 5, which ADR 0006 §7 never retries.
	if err == nil || errno(err) != windows.ERROR_ACCESS_DENIED || errors.As(err, &busy) {
		t.Fatalf("rename with a file open inside: error %d, %v; want 5, not busy", errno(err), err)
	}
	t.Logf("file open inside the directory: rename refused with error %d", errno(err))

	if err := s.Move("staging", "current"); err != nil {
		t.Fatalf("rename once every handle is closed: %v", err)
	}
}

// Held directories, and the ancestors of the vault, cannot be renamed or
// deleted while the session is open.
func TestHeldDirectoriesStayInPlace(t *testing.T) {
	base, s := newVault(t)
	dirs(t, s, "games")
	for _, path := range []string{filepath.Join(base, "games"), base} {
		if err := os.Rename(path, path+"-moved"); err == nil {
			t.Fatalf("%s renamed while held", path)
		}
		if err := os.Remove(filepath.Join(base, "games")); err == nil {
			t.Fatal("held directory deleted")
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(base, "games"), filepath.Join(base, "games-moved")); err != nil {
		t.Fatalf("rename after Close: %v", err)
	}
}

func requireOnCI(t *testing.T, what string, err error) {
	t.Helper()
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Fatalf("%s is required on CI: %v", what, err)
	}
	t.Skipf("%s unavailable: %v", what, err)
}

// Junctions and links are refused wherever they appear, and nothing is ever
// written through them.
func TestLinksAreNeverFollowed(t *testing.T) {
	root := filepath.Clean(t.TempDir())
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(filepath.Join(outside, "games"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "keep.save"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := fingerprint(t, outside)

	t.Run("in the vault path", func(t *testing.T) {
		junction(t, filepath.Join(root, "via"), outside)
		for _, path := range []string{filepath.Join(root, "via", "Vault"), filepath.Join(root, "via")} {
			if _, err := vault.Open(path); !notFollowed(err) {
				t.Errorf("%s: %v, want a not-followed refusal", path, err)
			}
		}
	})
	t.Run("below the vault", func(t *testing.T) {
		base, s := newVault(t)
		junction(t, filepath.Join(base, "games"), filepath.Join(outside, "games"))
		junction(t, filepath.Join(base, "staging"), outside)
		if err := s.Hold("games"); !notFollowed(err) {
			t.Errorf("Hold through a junction: %v", err)
		}
		if _, err := s.Identity("games"); !notFollowed(err) {
			t.Errorf("Identity of a junction: %v", err)
		}
		if _, err := s.CreateFile("games/new.save", time.Time{}); err == nil || !strings.Contains(err.Error(), "not held") {
			t.Errorf("file created below an unheld junction: %v", err)
		}
		if err := s.CreateDir("staging"); !errors.Is(err, fs.ErrExist) {
			t.Errorf("directory created over a junction: %v", err)
		}
		if err := s.Lock("staging"); err == nil {
			t.Error("lock taken on a junction")
		}
	})
	t.Run("relative symbolic link that os.Root follows", func(t *testing.T) {
		base, s := newVault(t)
		dirs(t, s, "real")
		create(t, s, "real/slot.save", "s", time.Time{})
		if err := os.Symlink("real", filepath.Join(base, "dirlink")); err != nil {
			requireOnCI(t, "symbolic link creation", err)
		}
		r, err := os.OpenRoot(base)
		if err != nil {
			t.Fatal(err)
		}
		file, err := r.Open(filepath.Join("dirlink", "slot.save"))
		_ = r.Close()
		if err != nil {
			t.Fatalf("premise: os.Root should follow a link that stays inside: %v", err)
		}
		_ = file.Close()
		if err := s.Hold("dirlink"); !notFollowed(err) {
			t.Errorf("Hold through a symbolic link: %v", err)
		}
		if _, err := s.CreateFile("dirlink/new.save", time.Time{}); err == nil {
			t.Error("file created through a symbolic link")
		}
		if _, err := os.Lstat(filepath.Join(base, "real", "new.save")); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("file written through the link: %v", err)
		}
	})
	if after := fingerprint(t, outside); after != before {
		t.Fatalf("written outside the vault:\n%s\n---\n%s", before, after)
	}
}

func TestOpenRefusals(t *testing.T) {
	root := filepath.Clean(t.TempDir())
	if err := os.WriteFile(filepath.Join(root, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		`relative\Vault`, root + `\Vault\`, root + `\a\..\Vault`, `\\server\share\Vault`,
		filepath.Join(root, "missing", "Vault"), filepath.Join(root, "file"), filepath.Join(root, "CON"),
		filepath.Join(root, "trailing."),
	} {
		if s, err := vault.Open(path); err == nil {
			_ = s.Close()
			t.Errorf("%q accepted", path)
		}
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 1 {
		t.Fatalf("refusals wrote something: %v %v", entries, err)
	}
}

func TestNamesAreChecked(t *testing.T) {
	base, s := newVault(t)
	before := fingerprint(t, base)
	for _, name := range []string{"", "/abs", "a/", "a/../b", `a\b`, "a:b", "CON", "trailing.", "trailing ", "a//b"} {
		if err := s.CreateDir(name); err == nil {
			t.Errorf("directory %q created", name)
		}
		if _, err := s.CreateFile(name, time.Time{}); err == nil {
			t.Errorf("file %q created", name)
		}
	}
	if _, err := s.CreateFile(".", time.Time{}); err == nil {
		t.Error("the vault itself opened as a file")
	}
	if err := s.Move(".", "elsewhere"); err == nil {
		t.Error("the vault itself moved")
	}
	if after := fingerprint(t, base); after != before {
		t.Fatalf("invalid names wrote something:\n%s", after)
	}
}

func TestLongPaths(t *testing.T) {
	base, s := newVault(t)
	name := "."
	for i := range 12 {
		next := fmt.Sprintf("very long directory name %02d", i)
		if name != "." {
			next = name + "/" + next
		}
		dirs(t, s, next)
		name = next
	}
	file := name + "/deep.save"
	if len(filepath.Join(base, filepath.FromSlash(file))) <= windows.MAX_PATH {
		t.Fatal("premise: path not beyond MAX_PATH")
	}
	create(t, s, file, "deep", time.Time{})
	if err := s.Move(file, name+"/moved.save"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Identity(name + "/moved.save"); err != nil {
		t.Fatal(err)
	}
	if got := content(t, `\\?\`+filepath.Join(base, filepath.FromSlash(name), "moved.save")); got != "deep" {
		t.Fatalf("content %q", got)
	}
}

const lockHelper = "SAVEWARDEN_VAULT_LOCK_HELPER"

// TestLockHelperProcess holds the lock of the vault named by lockHelper
// until it is killed. It does nothing in a normal test run.
func TestLockHelperProcess(t *testing.T) {
	path := os.Getenv(lockHelper)
	if path == "" {
		t.Skip("helper process only")
	}
	s, err := vault.Open(path)
	if err == nil {
		err = s.Lock("sync.lock")
	}
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	fmt.Println("locked")
	time.Sleep(time.Minute)
	os.Exit(0)
}

// The lock admits one session at a time, is never written, and is released
// by the system when its holder dies.
func TestLockIsExclusive(t *testing.T) {
	base, first := newVault(t)
	if err := first.Lock("sync.lock"); err != nil {
		t.Fatal(err)
	}
	second := open(t, base)
	var busy *vault.BusyError
	if err := second.Lock("sync.lock"); !errors.As(err, &busy) {
		t.Fatalf("second lock: %v, want busy", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := second.Lock("sync.lock"); err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}

	child := exec.Command(os.Args[0], "-test.run=^TestLockHelperProcess$", "-test.count=1") //nolint:gosec // G702: re-runs this test binary as the lock holder.
	child.Env = append(os.Environ(), lockHelper+"="+base)
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "locked\n" {
		_ = child.Process.Kill()
		t.Fatalf("helper: %q, %v", line, err)
	}
	third := open(t, base)
	if err := third.Lock("sync.lock"); !errors.As(err, &busy) {
		_ = child.Process.Kill()
		t.Fatalf("lock held by another process: %v, want busy", err)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	if err := third.Lock("sync.lock"); err != nil {
		t.Fatalf("lock after the holder died: %v", err)
	}
	info, err := os.Stat(filepath.Join(base, "sync.lock"))
	if err != nil || info.Size() != 0 {
		t.Fatalf("lock file %v, %v", info, err)
	}
}

func TestDescribeTheTemporaryVolume(t *testing.T) {
	volume, err := vault.Describe(filepath.Clean(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("temporary volume: %s, %s, %d bytes available", volume.FileSystem, volume.Drive, volume.Available)
	if volume.FileSystem == "" || volume.Available == 0 {
		t.Fatalf("volume %+v", volume)
	}
	if volume.Drive != backup.DriveFixed && volume.Drive != backup.DriveRemovable {
		t.Fatalf("temporary directory on a %s drive", volume.Drive)
	}
	if volume.FileSystem != "NTFS" {
		t.Skipf("temporary volume is %s, not NTFS", volume.FileSystem)
	}
	for _, path := range []string{`relative`, `\\server\share`, filepath.Join(t.TempDir(), "missing")} {
		if _, err := vault.Describe(path); err == nil {
			t.Errorf("%q described", path)
		}
	}
}
