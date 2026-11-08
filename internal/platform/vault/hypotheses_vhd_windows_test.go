//go:build windows

package vault_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/savewarden/savewarden-cli/internal/platform/vault"
)

// HW7 and HW11 need a volume that the test attaches, detaches and mounts in a
// temporary folder: a VHD that diskpart creates in t.TempDir(). diskpart
// needs administrator rights, which an elevated CI runner has; a workstation
// skips them. On a real removable volume, HW7 is proven by the owner's
// protocol, docs/validation/b5a-protocoles.md.

type hwVHD struct {
	file  string // the .vhdx, in t.TempDir()
	mount string // an empty folder of t.TempDir() where the volume is mounted
	guid  string // \\?\Volume{GUID}\ of the volume
}

func hwDiskpart(t *testing.T, script string) (string, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "diskpart.txt")
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("diskpart", "/s", path).CombinedOutput()
	return string(output), err
}

// hwNewVHD creates a 64 MiB VHD, formats it in NTFS and mounts it in a
// temporary folder. Cleanup removes the mount point and detaches the VHD;
// t.TempDir() then removes the file.
func hwNewVHD(t *testing.T, label string) *hwVHD {
	t.Helper()
	if !hwElevated() {
		if hwOnCI() {
			t.Fatalf("%s needs an elevated runner for diskpart", label)
		}
		t.Skipf("%s needs administrator rights for diskpart: proven on CI", label)
	}
	// diskpart finds an attached VHD by the path it was attached with, in
	// its long form: on the CI runner, TEMP holds the 8.3 name RUNNER~1, and
	// a second diskpart given the short path reports the VHD as detached.
	dir := hwLongPath(t, hwTemp(t))
	v := &hwVHD{file: filepath.Join(dir, label+".vhdx"), mount: filepath.Join(dir, label+"-mount")}
	hwMkdir(t, v.mount)
	t.Cleanup(func() {
		if output, err := exec.Command("mountvol", v.mount, "/D").CombinedOutput(); err != nil {
			t.Logf("mountvol /D: %v %s", err, output)
		}
		if output, err := hwDiskpart(t, fmt.Sprintf("select vdisk file=\"%s\"\ndetach vdisk\n", v.file)); err != nil {
			t.Logf("diskpart detach at cleanup: %v\n%s", err, output)
		}
	})
	script := fmt.Sprintf("create vdisk file=\"%s\" maximum=64 type=expandable\nselect vdisk file=\"%s\"\nattach vdisk\n"+
		"create partition primary\nformat fs=ntfs quick label=%s\nassign mount=\"%s\"\n", v.file, v.file, label, v.mount)
	if output, err := hwDiskpart(t, script); err != nil {
		t.Fatalf("diskpart: %v\n%s", err, output)
	}
	guid, err := hwVolumeGUID(v.mount + `\`)
	if err != nil {
		t.Fatalf("volume mounted in %s: %v", v.mount, err)
	}
	v.guid = guid
	return v
}

// hwLongPath returns the long form of an existing path, every 8.3 name
// expanded; the buffer is sized by a first call.
func hwLongPath(t *testing.T, path string) string {
	t.Helper()
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	var probe [1]uint16
	size, err := windows.GetLongPathName(pointer, &probe[0], 0)
	if err != nil {
		t.Fatalf("long path of %s: %v", path, err)
	}
	buffer := make([]uint16, size)
	n, err := windows.GetLongPathName(pointer, &buffer[0], size)
	if err != nil || n >= size {
		t.Fatalf("long path of %s: %d characters for %d, %v", path, n, size, err)
	}
	return windows.UTF16ToString(buffer[:n])
}

func (v *hwVHD) detach(t *testing.T) {
	t.Helper()
	if output, err := hwDiskpart(t, fmt.Sprintf("select vdisk file=\"%s\"\ndetach vdisk\n", v.file)); err != nil {
		t.Fatalf("diskpart detach: %v\n%s", err, output)
	}
}

func (v *hwVHD) attach(t *testing.T) {
	t.Helper()
	if output, err := hwDiskpart(t, fmt.Sprintf("select vdisk file=\"%s\"\nattach vdisk\n", v.file)); err != nil {
		t.Fatalf("diskpart attach: %v\n%s", err, output)
	}
}

// hwCode reports an error with its system code, for the record.
func hwCode(err error) string {
	if err == nil {
		return "success"
	}
	return fmt.Sprintf("error %d (%v)", errno(err), err)
}

// HW7, on a fixed volume that is detached: (a) a volume removed during a
// session makes the calls through its volume path fail, never redirected;
// (b) removed then attached again, the handles of the session stay dead and
// nothing is written on the new mount; (c) the GUID path reopened while the
// volume is away fails. The codes observed are recorded: they complete the
// classification "volume not available" of plan B5 §4.1. This proves a fixed
// volume taken away, never a removable one: the owner's protocol does.
func TestHW7VolumeDetachedDuringTheSession(t *testing.T) {
	v := hwNewVHD(t, "HW7")
	vaultPath := v.guid + "savewarden-hw7"
	if err := hwCreateDir(vaultPath); err != nil {
		t.Fatal(err)
	}
	pin, err := hwOpen(vaultPath, windows.GENERIC_READ, hwSharePin)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = windows.CloseHandle(pin) }()
	base := hwGUIDPath(t, pin)
	if base != vaultPath {
		t.Fatalf("final path %q, want %q", base, vaultPath)
	}
	if err := hwCreateDir(base + `\before`); err != nil {
		t.Fatal(err)
	}

	v.detach(t)
	errDir := hwCreateDir(base + `\after`)
	errFile := hwCreateFile(base+`\f`, "f")
	_, _, errHandle := hwIdentity(pin)
	reopened, errReopen := hwOpen(vaultPath, windows.FILE_READ_ATTRIBUTES, hwShareAll)
	if errReopen == nil {
		_ = windows.CloseHandle(reopened)
	}
	t.Logf("HW7a detached: CreateDirectory %s; CreateFile %s; handle %s", hwCode(errDir), hwCode(errFile), hwCode(errHandle))
	t.Logf("HW7c GUID path reopened while detached: %s", hwCode(errReopen))
	if errDir == nil || errFile == nil || errReopen == nil {
		t.Fatal("a call through the volume path succeeded while the volume was detached")
	}
	for _, name := range []string{"after", "f"} {
		if _, err := os.Lstat(filepath.Join(v.mount, "savewarden-hw7", name)); err == nil {
			t.Fatalf("%s appeared below the mount folder", name)
		}
	}

	v.attach(t)
	_, _, errAfter := hwIdentity(pin)
	again, errGUID := hwVolumeGUID(v.mount + `\`)
	t.Logf("HW7b attached again: old handle %s; volume %q (%v), same GUID %t", hwCode(errAfter), again, errGUID, again == v.guid)
	if errAfter == nil {
		t.Fatal("the handle of the session works on the new mount")
	}
	if errGUID == nil {
		for _, name := range []string{"after", "f"} {
			if _, err := os.Lstat(again + `savewarden-hw7\` + name); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("%s on the new mount: %v, want absent", name, err)
			}
		}
		if _, err := os.Lstat(again + `savewarden-hw7\before`); err != nil {
			t.Fatalf("the volume attached again lost its content: %v", err)
		}
	}
}

// HW11: a volume mount point among the ancestors of a vault path is refused
// as a reparse point: through the letter by vault.Open, with
// NotFollowedError; through the volume path of the host, the mount folder
// opened without following it is a reparse point.
func TestHW11MountPointAmongAncestorsIsRefused(t *testing.T) {
	v := hwNewVHD(t, "HW11")
	if err := hwCreateDir(v.guid + "vault"); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(v.mount, "vault")
	for _, path := range []string{inner, filepath.Join(inner, "Vault")} {
		s, err := vault.Open(path)
		if err == nil {
			_ = s.Close()
			t.Fatalf("vault.Open(%s) accepted a mount point among the ancestors", path)
		}
		if !notFollowed(err) {
			t.Fatalf("vault.Open(%s): %v, want NotFollowedError", path, err)
		}
	}
	host := hwGUIDPath(t, hwHandle(t, filepath.Dir(v.mount)))
	handle := hwHandle(t, host+`\`+filepath.Base(v.mount))
	_, attributes, err := hwIdentity(handle)
	if err != nil {
		t.Fatal(err)
	}
	if attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT == 0 {
		t.Fatalf("mount folder through the volume path: attributes %#x, want a reparse point", attributes)
	}
	t.Logf("mount point refused through the letter and seen as a reparse point through %s", host)
}
