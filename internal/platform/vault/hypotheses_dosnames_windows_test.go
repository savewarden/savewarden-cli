//go:build windows

package vault_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// Drive letters and DOS names for HW1, HW3, HW4, HW8, HW12 and HW13, on a
// bounded bench (plan B5 §2.3, §5.1, decisions 23 and 25):
//
//   - a test takes a free letter from L down to G, reserved for these
//     tests: absent from GetLogicalDrives and unknown to QueryDosDevice. The
//     B4 tests take theirs from Z down to M, possibly at the same time, in
//     other packages;
//   - the state of the letters A to L, with their QueryDosDevice targets, is
//     recorded first; the test removes only the definition it made, by exact
//     match, and fails if that state is not back;
//   - no letter that existed before is touched, and no letter defined at
//     boot is aimed at;
//   - outside t.TempDir(), only the metadata needed to identify X:\ and its
//     volume root is queried: no content read, no listing, no write.
//
// subst and DefineDosDevice define names in the logon session of the test:
// HW8 and HW13 therefore run on CI only, never on a workstation.

const hwLetters = "ABCDEFGHIJKL"

// hwQueryDosDevice returns the targets of a DOS name, or nil when the name
// is unknown.
func hwQueryDosDevice(name string) ([]string, error) {
	pointer, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	const size uint32 = 32768
	buffer := make([]uint16, size)
	n, err := windows.QueryDosDevice(pointer, &buffer[0], size)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// The targets are separated by NUL and end with two.
	var targets []string
	start := 0
	for i := 0; i < int(n); i++ {
		if buffer[i] == 0 {
			if i > start {
				targets = append(targets, windows.UTF16ToString(buffer[start:i]))
			}
			start = i + 1
		}
	}
	return targets, nil
}

// hwLetterState records the letters A to L: the logical drive bit and the
// DOS targets of each.
func hwLetterState(t *testing.T) string {
	t.Helper()
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for i, letter := range hwLetters {
		targets, err := hwQueryDosDevice(string(letter) + ":")
		if err != nil {
			t.Fatalf("QueryDosDevice %c: %v", letter, err)
		}
		lines = append(lines, fmt.Sprintf("%c:%t=%s", letter, mask&(1<<i) != 0, strings.Join(targets, "|")))
	}
	return strings.Join(lines, " ")
}

// hwLetter is a drive letter reserved by one test.
type hwLetter struct {
	name   string
	kind   string // "", "subst" or "dos"
	target string
}

// hwReserveLetter takes a free letter from L down to G, records the state of
// A to L, and at cleanup removes its own definition, then requires that
// state back.
func hwReserveLetter(t *testing.T) *hwLetter {
	t.Helper()
	initial := hwLetterState(t)
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		t.Fatal(err)
	}
	for i := 11; i >= 6; i-- {
		name := string(rune('A'+i)) + ":"
		targets, err := hwQueryDosDevice(name)
		if err != nil || mask&(1<<i) != 0 || targets != nil {
			continue
		}
		l := &hwLetter{name: name}
		t.Cleanup(func() {
			if l.kind != "" {
				if err := l.removeErr(); err != nil {
					t.Errorf("remove %s: %v", l.name, err)
				}
			}
			if got := hwLetterState(t); got != initial {
				t.Errorf("drive letters not restored:\nbefore %s\nafter  %s", initial, got)
			}
		})
		return l
	}
	if hwOnCI() {
		t.Fatal("no free drive letter from L to G on CI")
	}
	t.Skip("no free drive letter from L to G")
	return nil
}

// subst gives the letter to dir with subst.
func (l *hwLetter) subst(t *testing.T, dir string) {
	t.Helper()
	if l.kind != "" {
		t.Fatalf("%s is still defined", l.name)
	}
	if output, err := exec.Command("subst", l.name, dir).CombinedOutput(); err != nil { //nolint:gosec // G702: a letter this test reserved, to a directory of its own.
		t.Fatalf("subst %s %s: %v %s", l.name, dir, err, output)
	}
	l.kind, l.target = "subst", dir
}

// define gives the letter to the raw NT target with DefineDosDevice, in the
// logon session of the test, without broadcasting the change.
func (l *hwLetter) define(t *testing.T, target string) {
	t.Helper()
	if l.kind != "" {
		t.Fatalf("%s is still defined", l.name)
	}
	if err := hwDefineDosDevice(windows.DDD_RAW_TARGET_PATH|windows.DDD_NO_BROADCAST_SYSTEM, l.name, target); err != nil {
		t.Fatalf("DefineDosDevice %s %s: %v", l.name, target, err)
	}
	l.kind, l.target = "dos", target
}

func (l *hwLetter) remove(t *testing.T) {
	t.Helper()
	if err := l.removeErr(); err != nil {
		t.Fatalf("remove %s: %v", l.name, err)
	}
}

func (l *hwLetter) removeErr() error {
	kind := l.kind
	l.kind = ""
	switch kind {
	case "subst":
		if output, err := exec.Command("subst", l.name, "/D").CombinedOutput(); err != nil {
			return fmt.Errorf("subst /D: %w %s", err, output)
		}
	case "dos":
		return hwDefineDosDevice(windows.DDD_REMOVE_DEFINITION|windows.DDD_EXACT_MATCH_ON_REMOVE|
			windows.DDD_RAW_TARGET_PATH|windows.DDD_NO_BROADCAST_SYSTEM, l.name, l.target)
	}
	return nil
}

func hwDefineDosDevice(flags uint32, name, target string) error {
	namePointer, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	targetPointer, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.DefineDosDevice(flags, namePointer, targetPointer)
}

// hwRoot is what the S-02 detection of plan B5 §3.2 observes on X:\.
type hwRoot struct {
	final        string // final GUID path of a handle on X:\
	finalErr     error
	volume       string // the \\?\Volume{GUID}\ that starts final
	mount        string // GetVolumeNameForVolumeMountPoint(X:\)
	mountErr     error
	sameIdentity bool // X:\ has the identity of volume
}

// strict reports the three conditions: a final path without remainder, the
// same GUID by GetVolumeNameForVolumeMountPoint, the same identity.
func (r hwRoot) strict() bool {
	return r.finalErr == nil && r.final == r.volume && r.mountErr == nil && r.mount == r.volume && r.sameIdentity
}

func (r hwRoot) String() string {
	return fmt.Sprintf("final %q (%v), mount point %q (%v), same identity %t", r.final, r.finalErr, r.mount, r.mountErr, r.sameIdentity)
}

// hwCheckRoot queries the metadata of root, X:\, and of its volume root:
// handles for attributes only, nothing listed or read.
func hwCheckRoot(t *testing.T, root string) hwRoot {
	t.Helper()
	var r hwRoot
	handle, err := hwOpen(root, windows.FILE_READ_ATTRIBUTES, hwShareAll)
	if err != nil {
		t.Fatalf("open %s: %v", root, err)
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	r.final, r.finalErr = hwFinalPath(handle, volumeNameGUID)
	r.mount, r.mountErr = hwVolumeGUID(root)
	if r.finalErr == nil {
		r.volume = hwVolumePrefix(t, r.final)
		rootID, _, err := hwIdentity(handle)
		if err != nil {
			t.Fatal(err)
		}
		r.sameIdentity = hwIdentityOf(t, r.volume) == rootID
	}
	return r
}

func hwCIOnly(t *testing.T, what string) {
	t.Helper()
	if !hwOnCI() {
		t.Skipf("%s runs on CI only (plan B5 §5.1)", what)
	}
}

func hwElevated() bool { return windows.GetCurrentProcessToken().IsElevated() }

// HW12: the S-02 detection. For a real volume letter, the final GUID path of
// the handle on X:\ is \\?\Volume{GUID}\ without remainder,
// GetVolumeNameForVolumeMountPoint gives the same GUID, and X:\ has the
// identity of \\?\Volume{GUID}\. For a subst alias and a DefineDosDevice
// alias of a subdirectory, the final path keeps a remainder and
// GetVolumeNameForVolumeMountPoint fails.
func TestHW12LetterRootDetection(t *testing.T) {
	dir := hwTemp(t)
	real := hwCheckRoot(t, filepath.VolumeName(dir)+`\`)
	if !real.strict() {
		t.Fatalf("real letter %s: %s; want the three conditions", filepath.VolumeName(dir), real)
	}
	t.Logf("real letter %s: %s", filepath.VolumeName(dir), real)

	sub := filepath.Join(dir, "HW12", "sub")
	hwMkdir(t, sub)
	letter := hwReserveLetter(t)
	for _, alias := range []struct {
		kind  string
		apply func()
	}{
		{"subst", func() { letter.subst(t, sub) }},
		{"DefineDosDevice", func() { letter.define(t, `\??\`+sub) }},
	} {
		alias.apply()
		r := hwCheckRoot(t, letter.name+`\`)
		letter.remove(t)
		t.Logf("%s alias of a subdirectory: %s", alias.kind, r)
		if r.finalErr != nil || r.final == r.volume || r.volume != real.volume {
			t.Errorf("%s alias: final path %q (%v), want a remainder below %s", alias.kind, r.final, r.finalErr, real.volume)
		}
		if r.mountErr == nil {
			t.Errorf("%s alias: GetVolumeNameForVolumeMountPoint gave %q, want a failure", alias.kind, r.mount)
		}
		if r.strict() {
			t.Errorf("%s alias passes the three conditions", alias.kind)
		}
	}
}

// HW8: can a local DOS name Volume{GUID}, defined by DefineDosDevice in the
// logon session of the test, shadow the global one for \\?\Volume{GUID}\...?
// The test records, primitive by primitive, where CreateDirectory,
// CreateFile and MoveFileEx land when the shadow is defined before the call,
// and for the rename between the check of its source and the rename itself;
// and whether the final path of a new handle, or the identity of the rename
// target, then detects it. Either outcome is recorded: only an unexpected
// error fails. CI only (plan B5 §5.1).
func TestHW8LocalVolumeNameShadowing(t *testing.T) {
	hwCIOnly(t, "HW8")
	root := hwTemp(t)
	held := filepath.Join(root, "HW8", "held")
	hwMkdir(t, held)
	base := hwGUIDPath(t, hwPin(t, held))
	volume := hwVolumePrefix(t, base)
	rel := strings.TrimPrefix(base, volume)
	name := strings.TrimSuffix(strings.TrimPrefix(volume, `\\?\`), `\`)
	decoy := filepath.Join(root, "HW8", "decoy")
	decoyHeld := filepath.Join(decoy, rel)
	hwMkdir(t, decoyHeld)
	global, err := hwQueryDosDevice(name)
	if err != nil || len(global) == 0 {
		t.Fatalf("QueryDosDevice %s: %v %v", name, global, err)
	}
	shadow := `\??\` + decoy
	defined := false
	define := func() {
		t.Helper()
		if err := hwDefineDosDevice(windows.DDD_RAW_TARGET_PATH|windows.DDD_NO_BROADCAST_SYSTEM, name, shadow); err != nil {
			t.Fatalf("DefineDosDevice %s: %v", name, err)
		}
		defined = true
	}
	remove := func() error {
		if !defined {
			return nil
		}
		defined = false
		err := hwDefineDosDevice(windows.DDD_REMOVE_DEFINITION|windows.DDD_EXACT_MATCH_ON_REMOVE|
			windows.DDD_RAW_TARGET_PATH|windows.DDD_NO_BROADCAST_SYSTEM, name, shadow)
		if after, qerr := hwQueryDosDevice(name); qerr != nil || strings.Join(after, "|") != strings.Join(global, "|") {
			err = errors.Join(err, fmt.Errorf("%s not restored: %v, was %v (%w)", name, after, global, qerr))
		}
		return err
	}
	t.Cleanup(func() {
		if err := remove(); err != nil {
			t.Errorf("remove the shadow: %v", err)
		}
	})
	where := func(entry string) string {
		var found []string
		for _, place := range []struct{ label, dir string }{{"held", held}, {"decoy", decoyHeld}} {
			if _, err := os.Lstat(filepath.Join(place.dir, entry)); err == nil {
				found = append(found, place.label)
			}
		}
		if len(found) == 0 {
			return "nowhere"
		}
		return strings.Join(found, "+")
	}

	// a and b: the shadow defined before CreateDirectory and CreateFile.
	define()
	targets, err := hwQueryDosDevice(name)
	if err != nil {
		t.Fatal(err)
	}
	effective := len(targets) > 0 && targets[0] == shadow
	errDir := hwCreateDir(base + `\a`)
	var detected string
	pointer, err := windows.UTF16PtrFromString(base + `\f`)
	if err != nil {
		t.Fatal(err)
	}
	handle, errFile := windows.CreateFile(pointer, windows.GENERIC_WRITE|windows.FILE_READ_ATTRIBUTES, 0, nil,
		windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if errFile == nil {
		final, err := hwFinalPath(handle, volumeNameGUID)
		detected = fmt.Sprintf("final path %q (%v), detection %t", final, err, final != base+`\f`)
		_ = windows.CloseHandle(handle)
	}
	if err := remove(); err != nil {
		t.Fatal(err)
	}
	t.Logf("HW8a CreateDirectory under the shadow (effective %t): %v (%d), landed %s", effective, errDir, errno(errDir), where("a"))
	t.Logf("HW8b CreateFile under the shadow: %v (%d), landed %s; %s", errFile, errno(errFile), where("f"), detected)

	// c: the source of a rename checked without the shadow, renamed under it.
	if err := hwCreateFile(base+`\src`, "held"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(decoyHeld, "src"), []byte("decoy"), 0o600); err != nil {
		t.Fatal(err)
	}
	sourceHandle := hwHandle(t, base+`\src`)
	if final := hwGUIDPath(t, sourceHandle); final != base+`\src` {
		t.Fatalf("source final path %q", final)
	}
	sourceID, _, err := hwIdentity(sourceHandle)
	if err != nil {
		t.Fatal(err)
	}
	_ = windows.CloseHandle(sourceHandle)
	define()
	errMove := hwMove(base+`\src`, base+`\dst`)
	if err := remove(); err != nil {
		t.Fatal(err)
	}
	postcondition := "target absent: detected"
	if h, err := hwOpen(base+`\dst`, windows.FILE_READ_ATTRIBUTES, hwShareAll); err == nil {
		id, _, _ := hwIdentity(h)
		_ = windows.CloseHandle(h)
		postcondition = fmt.Sprintf("target identity matches the source: %t", id == sourceID)
	}
	t.Logf("HW8c MoveFileEx under the shadow: %v (%d), source left %s, target landed %s; postcondition: %s",
		errMove, errno(errMove), where("src"), where("dst"), postcondition)
	t.Logf("HW8 summary: shadow effective %t; CreateDirectory landed %s; CreateFile landed %s; rename target %s",
		effective, where("a"), where("f"), where("dst"))
}

// HW13: a feasibility hypothesis, not a guarantee (decision 23). A letter
// strictly equivalent to a volume root, admitted by decision 24 under the
// three conditions of the detection, is created, remapped and removed by the
// test in its logon session. Candidate routes: subst of a volume root; a
// DefineDosDevice name for the volume. For each, the test records the three
// conditions, then remaps the letter to a subdirectory, which the detection
// must refuse, and, when the runner has another volume, to that volume's
// root. CI only, on the bounded bench above. If no route passes, HW13 falls:
// stop and the owner's arbitration (T3).
func TestHW13RootEquivalentLetterIsRemappable(t *testing.T) {
	hwCIOnly(t, "HW13")
	t.Logf("process elevated: %t; the routes need no administrator right, but an elevated runner does not prove it", hwElevated())
	dir := hwTemp(t)
	volume := filepath.VolumeName(dir)
	guidRoot, err := hwVolumeGUID(volume + `\`)
	if err != nil {
		t.Fatal(err)
	}
	devices, err := hwQueryDosDevice(volume)
	if err != nil || len(devices) == 0 {
		t.Fatalf("QueryDosDevice %s: %v %v", volume, devices, err)
	}
	sub := filepath.Join(dir, "HW13", "sub")
	hwMkdir(t, sub)
	other := os.Getenv("RUNNER_TEMP")
	otherVolume := filepath.VolumeName(other)
	if otherVolume == "" || strings.EqualFold(otherVolume, volume) {
		otherVolume = ""
	}

	letter := hwReserveLetter(t)
	type route struct {
		name   string
		toRoot func(volume string)
		toDir  func(dir string)
	}
	routes := []route{
		{"subst", func(v string) { letter.subst(t, v+`\`) }, func(d string) { letter.subst(t, d) }},
		{"DefineDosDevice", func(v string) {
			targets, err := hwQueryDosDevice(v)
			if err != nil || len(targets) == 0 {
				t.Fatalf("QueryDosDevice %s: %v %v", v, targets, err)
			}
			letter.define(t, targets[0])
		}, func(d string) { letter.define(t, `\??\`+d) }},
	}
	passed := 0
	for _, r := range routes {
		r.toRoot(volume)
		atRoot := hwCheckRoot(t, letter.name+`\`)
		letter.remove(t)
		r.toDir(sub)
		atDir := hwCheckRoot(t, letter.name+`\`)
		letter.remove(t)
		t.Logf("HW13 %s to %s\\: %s; strict %t", r.name, volume, atRoot, atRoot.strict())
		t.Logf("HW13 %s remapped to a subdirectory: %s; strict %t", r.name, atDir, atDir.strict())
		ok := atRoot.strict() && atRoot.volume == guidRoot && !atDir.strict()
		if otherVolume != "" {
			r.toRoot(otherVolume)
			atOther := hwCheckRoot(t, letter.name+`\`)
			letter.remove(t)
			t.Logf("HW13 %s remapped to %s\\: %s; strict %t, other volume %t", r.name, otherVolume, atOther,
				atOther.strict(), atOther.volume != guidRoot)
		}
		if ok {
			passed++
		}
		t.Logf("HW13 route %s: %t", r.name, ok)
	}
	if passed == 0 {
		t.Fatal("HW13 falls: no route gives a letter strictly equivalent to a volume root that the test can remap; stop and the owner's arbitration (T3)")
	}
}
