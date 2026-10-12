//go:build unix

package filesystem_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/platform/filesystem"
)

func TestWindowsVolumeIsUnavailableOnUnix(t *testing.T) {
	_, err := filesystem.OpenScope(`D:\`)
	if err == nil || errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "Windows installations only") {
		t.Fatalf("error = %v; must be explicit and must not read as a missing source", err)
	}
}

func TestUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	anchor := tempAnchor(t)
	write(t, filepath.Join(anchor, "locked.save"), "x")
	if err := os.Chmod(filepath.Join(anchor, "locked.save"), 0); err != nil {
		t.Fatal(err)
	}
	scope := open(t, anchor)
	if _, err := scope.OpenRegular("locked.save"); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("error = %v, want permission denied", err)
	}
}

func TestFifoIsNeverOpenedForReading(t *testing.T) {
	anchor := tempAnchor(t)
	if err := mkfifo(filepath.Join(anchor, "pipe")); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
	scope := open(t, anchor)
	info, err := scope.Lstat("pipe")
	if err != nil || info.Mode()&fs.ModeNamedPipe == 0 {
		t.Fatalf("lstat pipe: %v %v", info, err)
	}
	if _, err := scope.OpenRegular("pipe"); err == nil {
		t.Fatal("a named pipe was opened as a regular file")
	}
}
