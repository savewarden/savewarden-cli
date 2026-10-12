package inspect

import (
	"io"
	"io/fs"

	"github.com/savewarden/savewarden-cli/internal/entities/game"
)

// ManifestReader loads a decoded and validated manifest.
type ManifestReader interface {
	Read(path string) (game.Manifest, error)
}

// FileSystem opens a read-only scope anchored on a directory, normally a
// volume root such as D:\.
type FileSystem interface {
	OpenScope(root string) (Scope, error)
}

// Scope gives read-only access below its anchor. Names are slash-separated
// and relative to the anchor, in io/fs form ("." is the anchor itself).
//
// Implementations must never follow a symbolic link, junction or other
// reparse point, in the name itself or in any parent below the anchor,
// including when one appears between two calls. They report such entries
// through Lstat modes (fs.ModeSymlink or fs.ModeIrregular) and refuse to
// traverse or open them with an error whose NotFollowed method returns true.
type Scope interface {
	// Lstat describes name without following it.
	Lstat(name string) (fs.FileInfo, error)
	// ReadDir lists a directory that is not, and is not reached through, a link.
	ReadDir(name string) ([]fs.DirEntry, error)
	// OpenRegular opens a regular file for reading only. It fails closed
	// if the object actually opened is not a regular file reached without
	// following any link.
	OpenRegular(name string) (File, error)
	// SameFile reports whether two descriptions denote the same object.
	SameFile(a, b fs.FileInfo) bool
	Close() error
}

// File is a regular file opened for reading.
type File interface {
	io.Reader
	Stat() (fs.FileInfo, error)
	Close() error
}
