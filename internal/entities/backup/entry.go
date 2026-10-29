// Package backup holds the types shared by the features that observe,
// copy and verify saves: inventory entries, findings, receipts, the vault
// marker and the operation journal of ADR 0006. It depends on the standard
// library only and performs no I/O: callers read and write, this package
// encodes, decodes, compares and classifies.
package backup

import (
	"crypto/sha256"
	"time"
)

// Entry describes one regular file read in full.
type Entry struct {
	Path    string // slash-separated, relative to the inventoried directory
	Size    int64
	ModTime time.Time // UTC
	SHA256  [sha256.Size]byte
}

// Status says what happened to an inventoried directory.
type Status string

// Directory statuses.
const (
	StatusInventoried Status = "inventoried" // walked; findings may still apply
	StatusMissing     Status = "missing"     // the directory or a parent does not exist
	StatusRefused     Status = "refused"     // not walked: link, non-directory or unreadable parent
)

// Directory is the no-follow inventory of one directory tree.
type Directory struct {
	Status Status
	// Missing is the Windows path of the first missing component when
	// Status is StatusMissing.
	Missing string
	// Entries are the regular files, sorted by Path in byte order.
	Entries []Entry
	// Directories are every subdirectory, empty ones included, sorted and
	// slash-separated.
	Directories []string
	// Encrypted lists the entries the file system reports as encrypted
	// (EFS), sorted.
	Encrypted []string
	Bytes     int64
	// Findings are the problems met, sorted by Path then Code; their Source
	// is empty.
	Findings []Finding
}
