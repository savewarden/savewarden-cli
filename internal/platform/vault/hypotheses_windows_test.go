//go:build windows

package vault_test

// Hypotheses HW1 to HW13 of plan B5 §5.1, confronted with real Windows
// before any production code depends on them (T3): commit 1 of B5a, tests
// only. They exercise Windows itself, through golang.org/x/sys/windows, and
// the vault package only where a hypothesis names its current behaviour.
// Every file lies in t.TempDir(); drive letters and DOS names are created,
// checked and removed by the tests themselves (plan §2.3). A hypothesis that
// fails stops B5a: report, then the owner's arbitration.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

// volumeNameGUID is VOLUME_NAME_GUID with FILE_NAME_NORMALIZED, the flags
// plan B5 §4.1 gives GetFinalPathNameByHandle; x/sys names no such constant.
const volumeNameGUID = 0x1

// volumeNameDOS is VOLUME_NAME_DOS with FILE_NAME_NORMALIZED.
const volumeNameDOS = 0x0

const (
	hwNoFollow = windows.FILE_FLAG_OPEN_REPARSE_POINT | windows.FILE_FLAG_BACKUP_SEMANTICS | windows.FILE_FLAG_OPEN_NO_RECALL
	hwShareAll = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE
	// hwSharePin is the sharing of a Session pin: no deletion, so no rename.
	hwSharePin = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE
)

type hwID struct {
	volume uint32
	index  uint64
}

func hwTemp(t *testing.T) string {
	t.Helper()
	return filepath.Clean(t.TempDir())
}

func hwMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

// hwOpen opens path without following it, nor recalling a cloud file.
func hwOpen(path string, access, share uint32) (windows.Handle, error) {
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return windows.InvalidHandle, err
	}
	return windows.CreateFile(pointer, access, share, nil, windows.OPEN_EXISTING, hwNoFollow, 0)
}

// hwHandle opens path for its metadata only, sharing everything, and closes
// it at cleanup.
func hwHandle(t *testing.T, path string) windows.Handle {
	t.Helper()
	handle, err := hwOpen(path, windows.FILE_READ_ATTRIBUTES, hwShareAll)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(handle) })
	return handle
}

// hwPin opens a directory as a Session pin does: for reading, without
// following it, sharing reading and writing but not deletion.
func hwPin(t *testing.T, path string) windows.Handle {
	t.Helper()
	handle, err := hwOpen(path, windows.GENERIC_READ, hwSharePin)
	if err != nil {
		t.Fatalf("pin %s: %v", path, err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(handle) })
	return handle
}

// hwFinalPath returns the final path of handle. The buffer is sized by a
// first call, and a result as long as the buffer is a truncation, refused:
// the x/sys wrapper reports no error when the buffer is too small.
func hwFinalPath(handle windows.Handle, flags uint32) (string, error) {
	var probe [1]uint16
	size, err := windows.GetFinalPathNameByHandle(handle, &probe[0], 0, flags)
	if err != nil {
		return "", err
	}
	buffer := make([]uint16, size)
	n, err := windows.GetFinalPathNameByHandle(handle, &buffer[0], size, flags)
	if err != nil {
		return "", err
	}
	if n >= size {
		return "", fmt.Errorf("final path truncated: %d characters for a buffer of %d", n, size)
	}
	return windows.UTF16ToString(buffer[:n]), nil
}

func hwGUIDPath(t *testing.T, handle windows.Handle) string {
	t.Helper()
	path, err := hwFinalPath(handle, volumeNameGUID)
	if err != nil {
		t.Fatalf("final GUID path: %v", err)
	}
	return path
}

func hwIdentity(handle windows.Handle) (hwID, uint32, error) {
	var data windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &data); err != nil {
		return hwID{}, 0, err
	}
	return hwID{volume: data.VolumeSerialNumber, index: uint64(data.FileIndexHigh)<<32 | uint64(data.FileIndexLow)}, data.FileAttributes, nil
}

// hwIdentityOf opens path for its metadata only and returns its identity.
func hwIdentityOf(t *testing.T, path string) hwID {
	t.Helper()
	handle, err := hwOpen(path, windows.FILE_READ_ATTRIBUTES, hwShareAll)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	id, _, err := hwIdentity(handle)
	if err != nil {
		t.Fatalf("identity of %s: %v", path, err)
	}
	return id
}

// hwVolumeGUID returns \\?\Volume{GUID}\ for a mount point such as C:\.
func hwVolumeGUID(root string) (string, error) {
	pointer, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return "", err
	}
	var name [64]uint16
	if err := windows.GetVolumeNameForVolumeMountPoint(pointer, &name[0], uint32(len(name))); err != nil {
		return "", err
	}
	return windows.UTF16ToString(name[:]), nil
}

// hwVolumePrefix returns the \\?\Volume{GUID}\ that starts a final GUID path.
func hwVolumePrefix(t *testing.T, final string) string {
	t.Helper()
	const head = `\\?\Volume{`
	end := strings.Index(final, `}\`)
	if !strings.HasPrefix(final, head) || end < 0 {
		t.Fatalf("%q is not a volume GUID path", final)
	}
	return final[:end+2]
}

func hwCreateDir(path string) error {
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return windows.CreateDirectory(pointer, nil)
}

// hwCreateFile creates path with CREATE_NEW, writes content and closes it.
func hwCreateFile(path, content string) error {
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(pointer, windows.GENERIC_WRITE, 0, nil, windows.CREATE_NEW,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(handle), path)
	_, err = io.WriteString(file, content)
	return errors.Join(err, file.Close())
}

func hwMove(from, to string) error {
	source, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	target, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(source, target, 0)
}

// hwExpectedGUIDPath derives the final GUID path a handle should give from
// its final DOS path and the volume GUID of its letter, never from the name
// the test used: that name may hold 8.3 short names.
func hwExpectedGUIDPath(t *testing.T, handle windows.Handle, volume string) string {
	t.Helper()
	dos, err := hwFinalPath(handle, volumeNameDOS)
	if err != nil {
		t.Fatalf("final DOS path: %v", err)
	}
	prefix := `\\?\` + volume + `\`
	if len(dos) < len(prefix) || !strings.EqualFold(dos[:len(prefix)], prefix) {
		t.Fatalf("final DOS path %q is not below %s", dos, prefix)
	}
	root, err := hwVolumeGUID(volume + `\`)
	if err != nil {
		t.Fatalf("volume GUID of %s: %v", volume, err)
	}
	return root + dos[len(prefix):]
}

// hwOnCI reports whether the test runs on GitHub Actions, where a proof
// that cannot be made fails instead of skipping.
func hwOnCI() bool { return os.Getenv("GITHUB_ACTIONS") == "true" }

// HW1: GetFinalPathNameByHandle with flags 0x1 gives, for a pinned
// directory of a fixed volume, \\?\Volume{GUID}\ followed by its path in the
// volume; through a subst letter, the path in the real volume. Reopened
// without following it, that path denotes the same identity. A buffer too
// small is detected by the size the call returns.
func TestHW1FinalPathOfAPinIsItsVolumePath(t *testing.T) {
	root := hwTemp(t)
	dir := filepath.Join(root, "HW1")
	hwMkdir(t, dir)
	volume := filepath.VolumeName(dir)
	pin := hwPin(t, dir)
	final := hwGUIDPath(t, pin)
	if want := hwExpectedGUIDPath(t, pin, volume); final != want {
		t.Fatalf("final GUID path %q, want %q", final, want)
	}
	pinID, _, err := hwIdentity(pin)
	if err != nil {
		t.Fatal(err)
	}
	if got := hwIdentityOf(t, final); got != pinID {
		t.Fatalf("%s reopened: identity %+v, want %+v", final, got, pinID)
	}
	t.Logf("final GUID path of a pin: %s", final)

	// Through a subst letter, the same directory gives the same final path.
	letter := hwReserveLetter(t)
	letter.subst(t, root)
	aliased := hwPin(t, letter.name+`\HW1`)
	if got := hwGUIDPath(t, aliased); got != final {
		t.Fatalf("through %s: final GUID path %q, want %q", letter.name, got, final)
	}

	// A buffer too small: the call returns the size it needs, larger than
	// the buffer, and no error.
	var small [8]uint16
	n, err := windows.GetFinalPathNameByHandle(pin, &small[0], uint32(len(small)), volumeNameGUID)
	if err != nil || n <= uint32(len(small)) {
		t.Fatalf("buffer of %d: returned %d, %v; want a larger size and no error", len(small), n, err)
	}
	t.Logf("buffer of %d characters: size %d returned, truncation detected", len(small), n)
}

// HW2: through the volume path, CreateDirectory, CreateFile with CREATE_NEW,
// MoveFileEx with flags 0 and GetFileInformationByHandle keep the semantics
// the vault relies on: error 183 for an existing directory or rename
// target, 80 for an existing file, 17 across volumes; a refusal changes
// nothing.
func TestHW2PrimitivesKeepTheirSemanticsThroughTheVolumePath(t *testing.T) {
	dir := filepath.Join(hwTemp(t), "HW2")
	hwMkdir(t, dir)
	base := hwGUIDPath(t, hwPin(t, dir))

	if err := hwCreateDir(base + `\a`); err != nil {
		t.Fatal(err)
	}
	if err := hwCreateFile(base+`\f`, "f"); err != nil {
		t.Fatal(err)
	}
	if err := hwCreateFile(base+`\g`, "g"); err != nil {
		t.Fatal(err)
	}
	if err := hwCreateDir(base + `\c`); err != nil {
		t.Fatal(err)
	}
	before := fingerprint(t, dir)
	for _, tt := range []struct {
		name string
		err  error
		want windows.Errno
	}{
		{"CreateDirectory on an existing directory", hwCreateDir(base + `\a`), windows.ERROR_ALREADY_EXISTS},
		{"CreateFile CREATE_NEW on an existing file", hwCreateFile(base+`\f`, "x"), windows.ERROR_FILE_EXISTS},
		{"MoveFileEx 0 onto an existing directory", hwMove(base+`\a`, base+`\c`), windows.ERROR_ALREADY_EXISTS},
		{"MoveFileEx 0 onto an existing file", hwMove(base+`\f`, base+`\g`), windows.ERROR_ALREADY_EXISTS},
	} {
		if !errors.Is(tt.err, tt.want) {
			t.Errorf("%s: %v (%d), want %d", tt.name, tt.err, errno(tt.err), tt.want)
		}
	}
	if fingerprint(t, dir) != before {
		t.Fatal("a refused operation changed the directory")
	}

	if err := hwMove(base+`\a`, base+`\b`); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(filepath.Join(dir, "b")); err != nil || !info.IsDir() {
		t.Fatalf("renamed directory through the letter: %v", err)
	}
	if hwIdentityOf(t, base+`\b`) != hwIdentityOf(t, filepath.Join(dir, "b")) {
		t.Fatal("GetFileInformationByHandle differs between the volume path and the letter")
	}

	// Across volumes, outside any Session: on CI, RUNNER_TEMP lies on
	// another volume than TEMP; nothing is created there.
	other := os.Getenv("RUNNER_TEMP")
	if other == "" || strings.EqualFold(filepath.VolumeName(other), filepath.VolumeName(dir)) {
		if hwOnCI() {
			t.Fatalf("a second volume is required on CI for error 17: RUNNER_TEMP %q", other)
		}
		t.Log("error 17 across volumes: no second volume here, proven on CI")
		return
	}
	otherBase := hwGUIDPath(t, hwHandle(t, other))
	target := otherBase + `\savewarden-hw2-` + filepath.Base(dir) + "-" + fmt.Sprint(os.Getpid())
	if err := hwMove(base+`\g`, target); !errors.Is(err, windows.ERROR_NOT_SAME_DEVICE) {
		t.Fatalf("MoveFileEx 0 across volumes: %v (%d), want 17", err, errno(err))
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) { //nolint:gosec // G703: a name the test built below RUNNER_TEMP, checked absent.
		t.Fatalf("target across volumes: %v, want absent", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "g")); err != nil {
		t.Fatalf("source after the refused move: %v", err)
	}
}

// HW3: every prefix of the volume path, reopened without following it,
// denotes the identity of the pin taken for that prefix through the letter,
// and each final path extends the previous one by one segment. A letter
// remapped while the chain is pinned gives a final path that no longer
// extends its parent's: the incoherence is detectable.
func TestHW3PrefixesOfTheVolumePathMatchThePins(t *testing.T) {
	root := hwTemp(t)
	chain := filepath.Join(root, "HW3", "x", "y")
	hwMkdir(t, chain)
	volume := filepath.VolumeName(chain)
	segments := strings.Split(chain[len(volume)+1:], `\`)
	current := volume + `\`
	var finals []string
	for k := 0; k <= len(segments); k++ {
		if k > 0 {
			current = filepath.Join(current, segments[k-1])
		}
		pin := hwPin(t, current)
		final := hwGUIDPath(t, pin)
		if k > 0 {
			parentFinal := strings.TrimSuffix(finals[k-1], `\`) + `\`
			rest := strings.TrimPrefix(final, parentFinal)
			if !strings.HasPrefix(final, parentFinal) || rest == "" || strings.Contains(rest, `\`) {
				t.Fatalf("final path %q does not extend %q by one segment", final, parentFinal)
			}
			if !strings.EqualFold(rest, segments[k-1]) {
				t.Logf("segment %q has the final name %q (8.3 name or case)", segments[k-1], rest)
			}
		}
		pinID, _, err := hwIdentity(pin)
		if err != nil {
			t.Fatal(err)
		}
		if got := hwIdentityOf(t, final); got != pinID {
			t.Fatalf("prefix %s reopened: identity %+v, want the pin's %+v", final, got, pinID)
		}
		finals = append(finals, final)
	}

	// A letter remapped between two pins of the chain.
	for _, side := range []string{"A", "B"} {
		hwMkdir(t, filepath.Join(root, side, "x", "y"))
	}
	letter := hwReserveLetter(t)
	letter.subst(t, filepath.Join(root, "A"))
	parentPin := hwPin(t, letter.name+`\x`)
	parentFinal := hwGUIDPath(t, parentPin)
	letter.remove(t)
	letter.subst(t, filepath.Join(root, "B"))
	childFinal := hwGUIDPath(t, hwPin(t, letter.name+`\x\y`))
	if strings.HasPrefix(childFinal, parentFinal+`\`) {
		t.Fatalf("remapped letter: %q still extends %q", childFinal, parentFinal)
	}
	if hwIdentityOf(t, parentFinal+`\y`) == hwIdentityOf(t, childFinal) {
		t.Fatal("remapped letter: the held prefix and the new pin have the same identity")
	}
	t.Logf("remapped letter detected: %q does not extend %q", childFinal, parentFinal)
}

// HW4: a subst letter removed then given to another directory while its
// target is held: creations and renames made through the volume path of
// the pins stay in the held target, and the other directory, a witness,
// keeps its fingerprint. The letter itself now names the witness.
func TestHW4WritesThroughTheVolumePathStayInTheHeldTarget(t *testing.T) {
	root := hwTemp(t)
	held := filepath.Join(root, "held")
	witness := filepath.Join(root, "witness")
	hwMkdir(t, filepath.Join(held, "vault"))
	hwMkdir(t, filepath.Join(witness, "vault"))
	letter := hwReserveLetter(t)
	letter.subst(t, held)
	hwPin(t, letter.name+`\`)
	base := hwGUIDPath(t, hwPin(t, letter.name+`\vault`))
	before := fingerprint(t, witness)

	letter.remove(t)
	letter.subst(t, witness)
	if hwIdentityOf(t, letter.name+`\vault`) != hwIdentityOf(t, filepath.Join(witness, "vault")) {
		t.Fatal("the letter was not given to the witness")
	}

	if err := hwCreateDir(base + `\d`); err != nil {
		t.Fatal(err)
	}
	if err := hwCreateFile(base+`\f`, "x"); err != nil {
		t.Fatal(err)
	}
	if err := hwMove(base+`\f`, base+`\g`); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(filepath.Join(held, "vault", "d")); err != nil || !info.IsDir() {
		t.Fatalf("directory created through the volume path: %v, want it in the held target", err)
	}
	if got := content(t, filepath.Join(held, "vault", "g")); got != "x" {
		t.Fatalf("renamed file in the held target: %q", got)
	}
	if _, err := os.Lstat(filepath.Join(held, "vault", "f")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source of the rename: %v, want absent", err)
	}
	if fingerprint(t, witness) != before {
		t.Fatal("the witness changed: a write followed the letter")
	}
}

// HW5: a pin taken through the volume path still forbids renaming the held
// directory; a handle opened through the letter and one opened through the
// volume path denote the same object and conflict the same way.
func TestHW5PinsTakenThroughTheVolumePathHoldTheDirectory(t *testing.T) {
	parent := filepath.Join(hwTemp(t), "HW5")
	dir := filepath.Join(parent, "held")
	hwMkdir(t, dir)
	file := filepath.Join(dir, "slot.save")
	if err := os.WriteFile(file, []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := hwGUIDPath(t, hwHandle(t, dir))

	pin, err := hwOpen(base, windows.GENERIC_READ, hwSharePin)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Rename(dir, dir+"-moved")
	_ = windows.CloseHandle(pin)
	if err == nil {
		t.Fatal("directory renamed while pinned through its volume path")
	}
	t.Logf("pinned through the volume path: rename refused with error %d", errno(err))
	if err := os.Rename(dir, dir+"-moved"); err != nil {
		t.Fatalf("rename once the pin is closed: %v", err)
	}
	if err := os.Rename(dir+"-moved", dir); err != nil {
		t.Fatal(err)
	}

	if hwIdentityOf(t, base+`\slot.save`) != hwIdentityOf(t, file) {
		t.Fatal("the letter and the volume path denote different objects")
	}
	for _, tt := range []struct{ first, second string }{{base + `\slot.save`, file}, {file, base + `\slot.save`}} {
		exclusive, err := hwOpen(tt.first, windows.GENERIC_READ, 0)
		if err != nil {
			t.Fatal(err)
		}
		second, err := hwOpen(tt.second, windows.GENERIC_READ, hwShareAll)
		if err == nil {
			_ = windows.CloseHandle(second)
		}
		_ = windows.CloseHandle(exclusive)
		if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			t.Fatalf("%s held without sharing, %s opened: %v, want 32", tt.first, tt.second, err)
		}
	}
}

// HW6: long paths and Unicode names behind the volume path: directories and
// a file created beyond MAX_PATH read back with the same names.
func TestHW6LongAndUnicodeNamesBehindTheVolumePath(t *testing.T) {
	root := filepath.Join(hwTemp(t), "HW6")
	hwMkdir(t, root)
	base := hwGUIDPath(t, hwPin(t, root))
	const segment = "Sauvegarde éàü 日本語 セーブデータ"
	path, letterPath := base, root
	for hwUTF16Len(letterPath) <= windows.MAX_PATH+40 {
		path += `\` + segment
		letterPath = filepath.Join(letterPath, segment)
		if err := hwCreateDir(path); err != nil {
			t.Fatalf("create directory at %d characters: %v", hwUTF16Len(letterPath), err)
		}
	}
	const name = "Ünïcödé 存档.save"
	if err := hwCreateFile(path+`\`+name, "u"); err != nil {
		t.Fatal(err)
	}
	if got := content(t, filepath.Join(letterPath, name)); got != "u" {
		t.Fatalf("content through the letter: %q", got)
	}
	if got := hwGUIDPath(t, hwHandle(t, path+`\`+name)); got != path+`\`+name {
		t.Fatalf("final path %q, want %q", got, path+`\`+name)
	}
	t.Logf("file created at %d UTF-16 characters through the letter", hwUTF16Len(filepath.Join(letterPath, name)))
}

// hwUTF16Len counts UTF-16 code units, the unit of MAX_PATH.
func hwUTF16Len(s string) int { return len(utf16.Encode([]rune(s))) }

// HW9: an absent target is created through its held parent: the base is the
// parent's final path followed by the segment; the root created exclusively
// through that base is pinned, its final path equals the base and its
// identity is read on its handle. A name already taken is never adopted.
func TestHW9AbsentTargetIsCreatedThroughTheHeldParent(t *testing.T) {
	parent := filepath.Join(hwTemp(t), "HW9")
	hwMkdir(t, parent)
	base := hwGUIDPath(t, hwPin(t, parent)) + `\Vault`
	if err := hwCreateDir(base); err != nil {
		t.Fatal(err)
	}
	if err := hwCreateDir(base); !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		t.Fatalf("second exclusive creation: %v, want 183", err)
	}
	pin := hwPin(t, base)
	if got := hwGUIDPath(t, pin); got != base {
		t.Fatalf("final path of the created root %q, want %q", got, base)
	}
	id, attributes, err := hwIdentity(pin)
	if err != nil {
		t.Fatal(err)
	}
	if attributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		t.Fatalf("created root attributes %#x", attributes)
	}
	if got := hwIdentityOf(t, filepath.Join(parent, "Vault")); got != id {
		t.Fatalf("identity through the letter %+v, on the pin %+v", got, id)
	}
}

// HW10: the volume of the root pin is described on its handle and through
// \\?\Volume{GUID}\ as through the letter: file system, serial number,
// drive type and capacity.
func TestHW10VolumeOfTheRootPinIsDescribedByHandle(t *testing.T) {
	dir := hwTemp(t)
	letterRoot := filepath.VolumeName(dir) + `\`
	rootPin := hwPin(t, letterRoot)
	guidRoot := hwGUIDPath(t, rootPin)
	if want, err := hwVolumeGUID(letterRoot); err != nil || guidRoot != want {
		t.Fatalf("final path of the root pin %q, want %q (%v)", guidRoot, want, err)
	}

	const size = windows.MAX_PATH + 1
	var byHandle, byLetter [size]uint16
	var serialByHandle, serialByLetter uint32
	if err := windows.GetVolumeInformationByHandle(rootPin, nil, 0, &serialByHandle, nil, nil, &byHandle[0], size); err != nil {
		t.Fatal(err)
	}
	pointer, err := windows.UTF16PtrFromString(letterRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.GetVolumeInformation(pointer, nil, 0, &serialByLetter, nil, nil, &byLetter[0], size); err != nil {
		t.Fatal(err)
	}
	fsHandle, fsLetter := windows.UTF16ToString(byHandle[:]), windows.UTF16ToString(byLetter[:])
	if fsHandle != fsLetter || serialByHandle != serialByLetter {
		t.Fatalf("by handle %s %08x, by letter %s %08x", fsHandle, serialByHandle, fsLetter, serialByLetter)
	}

	guidPointer, err := windows.UTF16PtrFromString(guidRoot)
	if err != nil {
		t.Fatal(err)
	}
	if a, b := windows.GetDriveType(guidPointer), windows.GetDriveType(pointer); a != b {
		t.Fatalf("drive type through the volume path %d, through the letter %d", a, b)
	}
	directory := hwGUIDPath(t, hwHandle(t, dir)) + `\`
	directoryPointer, err := windows.UTF16PtrFromString(directory)
	if err != nil {
		t.Fatal(err)
	}
	var availableGUID, totalGUID, freeGUID, availableLetter, totalLetter, freeLetter uint64
	if err := windows.GetDiskFreeSpaceEx(directoryPointer, &availableGUID, &totalGUID, &freeGUID); err != nil {
		t.Fatalf("free space through the volume path: %v", err)
	}
	letterPointer, err := windows.UTF16PtrFromString(dir + `\`)
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.GetDiskFreeSpaceEx(letterPointer, &availableLetter, &totalLetter, &freeLetter); err != nil {
		t.Fatal(err)
	}
	if totalGUID != totalLetter || availableGUID == 0 || availableLetter == 0 {
		t.Fatalf("capacity through the volume path %d, through the letter %d; available %d and %d",
			totalGUID, totalLetter, availableGUID, availableLetter)
	}
	t.Logf("%s %s, serial %08x, drive type %d; available %d and %d bytes", guidRoot, fsHandle, serialByHandle,
		windows.GetDriveType(pointer), availableGUID, availableLetter)
}
