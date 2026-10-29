package backup

import (
	"fmt"
	"regexp"
	"strconv"
)

// FileID identifies a file or directory on its volume: the volume serial
// number and the file index. A rename keeps it; a copy or a replacement
// does not (ADR 0006 §4).
type FileID struct {
	Volume uint32
	Index  uint64
}

// String returns the journal form: eight then sixteen lowercase hexadecimal
// digits, separated by a hyphen.
func (id FileID) String() string { return fmt.Sprintf("%08x-%016x", id.Volume, id.Index) }

var fileIDForm = regexp.MustCompile(`^([0-9a-f]{8})-([0-9a-f]{16})$`)

// ParseFileID reads the form written by String, and nothing else.
func ParseFileID(s string) (FileID, error) {
	m := fileIDForm.FindStringSubmatch(s)
	if m == nil {
		return FileID{}, fmt.Errorf("file identity %q is not of the form vvvvvvvv-iiiiiiiiiiiiiiii", s)
	}
	volume, _ := strconv.ParseUint(m[1], 16, 32)
	index, _ := strconv.ParseUint(m[2], 16, 64)
	return FileID{Volume: uint32(volume), Index: index}, nil
}

// Drive is the kind of drive that holds a volume.
type Drive int

// Drive kinds, as Windows reports them.
const (
	DriveUnknown Drive = iota
	DriveRemovable
	DriveFixed
	DriveRemote
	DriveOptical
	DriveRAM
)

func (d Drive) String() string {
	switch d {
	case DriveRemovable:
		return "removable"
	case DriveFixed:
		return "fixed"
	case DriveRemote:
		return "network"
	case DriveOptical:
		return "optical"
	case DriveRAM:
		return "RAM disk"
	}
	return "unknown"
}

// Volume describes the volume of a vault, read without writing anything.
type Volume struct {
	// FileSystem is the name the volume reports, such as NTFS.
	FileSystem string
	Drive      Drive
	// Available is the number of bytes the calling user may still write,
	// quotas included.
	Available uint64
}
