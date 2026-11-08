//go:build windows

package vault_test

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// hwKeyMarker is the empty file the owner puts at the root of a removable
// test key, holding no real data, to designate it for HW7.
const hwKeyMarker = "SAVEWARDEN-HW7-CLE-DE-TEST"

// hwSay prints an instruction to the owner at once, outside the test log.
func hwSay(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "\n>>> "+format+"\n\n", args...)
}

// hwWait polls ready every second until it holds, or fails the test after
// limit.
func hwWait(t *testing.T, what string, limit time.Duration, ready func() bool) {
	t.Helper()
	for deadline := time.Now().Add(limit); time.Now().Before(deadline); time.Sleep(time.Second) {
		if ready() {
			return
		}
	}
	t.Fatalf("%s: nothing observed within %s", what, limit)
}

// TestHW7OwnerRemovableVolume is the owner's protocol for HW7 on a real
// removable volume (plan B5 §5.1, decision 13), described in
// docs/validation/b5a-protocoles.md. It runs only when SAVEWARDEN_HW7_VOLUME
// names the letter of a removable key carrying hwKeyMarker at its root, and
// never on CI. It creates one new folder on the key through the volume path,
// holds it, asks the owner to pull the key out, checks that every call
// fails, asks to plug it back, then checks that the old handle stays dead and
// that nothing was written. It deletes nothing: the folder stays on the key.
func TestHW7OwnerRemovableVolume(t *testing.T) {
	letter := strings.ToUpper(os.Getenv("SAVEWARDEN_HW7_VOLUME"))
	if letter == "" || hwOnCI() {
		t.Skip("owner's protocol: set SAVEWARDEN_HW7_VOLUME to the letter of the test key, outside CI")
	}
	if len(letter) != 2 || letter[1] != ':' || letter[0] < 'A' || letter[0] > 'Z' {
		t.Fatalf("SAVEWARDEN_HW7_VOLUME %q: a drive letter such as F: is required", letter)
	}
	root := letter + `\`
	rootPointer, err := windows.UTF16PtrFromString(root)
	if err != nil {
		t.Fatal(err)
	}
	if kind := windows.GetDriveType(rootPointer); kind != windows.DRIVE_REMOVABLE {
		t.Fatalf("%s is not a removable volume (drive type %d)", root, kind)
	}
	if info, err := os.Lstat(root + hwKeyMarker); err != nil || !info.Mode().IsRegular() { //nolint:gosec // G703: the owner's test key, its marker checked by metadata only.
		t.Fatalf("%s carries no %s file: %v; the protocol only runs on a designated test key", root, hwKeyMarker, err)
	}
	guid, err := hwVolumeGUID(root)
	if err != nil {
		t.Fatal(err)
	}
	name := "savewarden-hw7-" + time.Now().UTC().Format("20060102T150405")
	folder := guid + name
	if err := hwCreateDir(folder); err != nil {
		t.Fatal(err)
	}
	pin, err := hwOpen(folder, windows.GENERIC_READ, hwSharePin)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = windows.CloseHandle(pin) }()
	if final := hwGUIDPath(t, pin); final != folder {
		t.Fatalf("final path %q, want %q", final, folder)
	}
	if err := hwCreateDir(folder + `\before`); err != nil {
		t.Fatal(err)
	}

	hwSay("Retirez maintenant la clé %s, sans l'éjecter. Attente : 5 minutes au plus.", letter)
	hwWait(t, "removal", 5*time.Minute, func() bool {
		_, _, err := hwIdentity(pin)
		return err != nil
	})
	errDir := hwCreateDir(folder + `\after`)
	errFile := hwCreateFile(folder+`\f`, "f")
	_, _, errHandle := hwIdentity(pin)
	reopened, errReopen := hwOpen(folder, windows.FILE_READ_ATTRIBUTES, hwShareAll)
	if errReopen == nil {
		_ = windows.CloseHandle(reopened)
	}
	t.Logf("HW7a removed: CreateDirectory %s; CreateFile %s; handle %s", hwCode(errDir), hwCode(errFile), hwCode(errHandle))
	t.Logf("HW7c GUID path reopened while removed: %s", hwCode(errReopen))
	if errDir == nil || errFile == nil || errReopen == nil {
		t.Fatal("a call through the volume path succeeded while the key was out")
	}

	hwSay("Rebranchez maintenant la clé. Attente : 5 minutes au plus.")
	var again string
	hwWait(t, "reinsertion", 5*time.Minute, func() bool {
		g, err := hwVolumeGUID(root)
		again = g
		return err == nil
	})
	_, _, errAfter := hwIdentity(pin)
	t.Logf("HW7b plugged back: old handle %s; volume %q, same GUID %t", hwCode(errAfter), again, again == guid)
	if errAfter == nil {
		t.Fatal("the handle of the session works on the new mount")
	}
	for _, entry := range []string{"after", "f"} {
		if _, err := os.Lstat(again + name + `\` + entry); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s on the key plugged back: %v, want absent", entry, err)
		}
	}
	if _, err := os.Lstat(again + name + `\before`); err != nil {
		t.Fatalf("the key plugged back lost the folder of the protocol: %v", err)
	}
	hwSay("Protocole HW7 terminé. Le dossier %s reste sur la clé : supprimez-le vous-même.", letter+`\`+name)
}
