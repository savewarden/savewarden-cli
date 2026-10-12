package game

import (
	"errors"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Windows path grammar for manifest v1. It is deliberately independent of
// path/filepath so that a manifest gets the same verdict on every host:
// filepath follows the host's rules and would accept or misread Windows
// paths on Linux and macOS. Only canonical forms are accepted; nothing is
// cleaned, expanded or corrected silently.

const maxSegmentUTF16 = 255

// ParseWindowsAbsolute splits an absolute drive path such as D:\Games\X into
// its volume root ("D:\") and its segments. The volume root alone is valid.
func ParseWindowsAbsolute(path string) (volume string, segments []string, err error) {
	if err := checkCommon(path); err != nil {
		return "", nil, err
	}
	switch {
	case strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, `\??\`):
		return "", nil, errors.New(`UNC and device paths are not supported; use a drive path such as D:\Games`)
	case len(path) < 2 || !isDriveLetter(path[0]) || path[1] != ':':
		return "", nil, errors.New(`must be an absolute path starting with a drive letter, such as D:\Games`)
	case len(path) == 2 || path[2] != '\\':
		return "", nil, errors.New(`drive-relative path; write the backslash after the drive, such as D:\Games`)
	}
	volume = path[:3]
	if len(path) == 3 {
		return volume, nil, nil
	}
	segments, err = splitSegments(path[3:])
	if err != nil {
		return "", nil, err
	}
	return volume, segments, nil
}

// ParseWindowsRelative splits a relative path such as CPY_SAVES\Profile into
// its segments.
func ParseWindowsRelative(path string) ([]string, error) {
	if err := checkCommon(path); err != nil {
		return nil, err
	}
	switch {
	case path == "":
		return nil, errors.New("must not be empty")
	case strings.HasPrefix(path, `\`):
		return nil, errors.New("must be relative; remove the leading backslash")
	case len(path) >= 2 && isDriveLetter(path[0]) && path[1] == ':':
		return nil, errors.New("must be relative; drive letters are not allowed")
	}
	return splitSegments(path)
}

func checkCommon(path string) error {
	if !utf8.ValidString(path) {
		return errors.New("must be valid UTF-8")
	}
	if strings.Contains(path, "/") {
		return errors.New(`use backslashes (\) as separators; forward slashes are not accepted`)
	}
	if start := strings.IndexByte(path, '%'); start >= 0 {
		if end := strings.IndexByte(path[start+1:], '%'); end > 0 {
			return errors.New("environment variables are not expanded in manifest v1")
		}
	}
	return nil
}

func splitSegments(path string) ([]string, error) {
	segments := strings.Split(path, `\`)
	for _, segment := range segments {
		if err := checkSegment(segment); err != nil {
			return nil, err
		}
	}
	return segments, nil
}

func checkSegment(segment string) error {
	switch segment {
	case "":
		return errors.New(`empty segment; remove doubled or trailing backslashes`)
	case ".", "..":
		return errors.New(`"." and ".." segments are not allowed`)
	}
	for _, r := range segment {
		if r < 0x20 {
			return errors.New("control characters are not allowed")
		}
		if strings.ContainsRune(`<>:"|?*`, r) {
			if r == ':' {
				return errors.New(`":" is not allowed in a segment (alternate data streams are refused)`)
			}
			return errors.New(`characters < > : " | ? * are not allowed`)
		}
	}
	if last := segment[len(segment)-1]; last == '.' || last == ' ' {
		return errors.New("a segment must not end with a dot or a space; Windows strips them")
	}
	if isReservedName(segment) {
		return errors.New("reserved Windows device name " + segment)
	}
	if len(utf16.Encode([]rune(segment))) > maxSegmentUTF16 {
		return errors.New("segment longer than 255 UTF-16 code units")
	}
	return nil
}

var reservedNames = func() map[string]bool {
	names := map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true, "CONIN$": true, "CONOUT$": true}
	for _, prefix := range []string{"COM", "LPT"} {
		for _, suffix := range []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "¹", "²", "³"} {
			names[prefix+suffix] = true
		}
	}
	return names
}()

// isReservedName applies Windows' rule: the part before the first dot,
// with trailing spaces removed, must not be a device name in any case.
func isReservedName(segment string) bool {
	base, _, _ := strings.Cut(segment, ".")
	return reservedNames[strings.ToUpper(strings.TrimRight(base, " "))]
}

func isDriveLetter(c byte) bool { return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' }

// SameSegments compares segment lists the way a case-insensitive Windows
// volume would. It reports whether a equals b or is an ancestor of b.
func SameSegments(a, b []string) bool {
	if len(a) > len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(a[i], b[i]) {
			return false
		}
	}
	return true
}
