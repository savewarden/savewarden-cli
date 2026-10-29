package sync

import (
	"context"
	"io/fs"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/entities/game"
)

// ManifestReader loads a decoded and validated manifest.
type ManifestReader interface {
	Read(path string) (game.Manifest, error)
}

// Inventories walks one directory tree without following anything. Its
// method has the exact signature of the inspect feature's Directory, which
// app passes here (ADR 0004).
type Inventories interface {
	Directory(ctx context.Context, volume string, segments []string) (backup.Directory, error)
}

// Files opens read-only scopes anchored on a volume root such as D:\. A
// missing volume yields an fs.ErrNotExist error.
type Files interface {
	OpenScope(root string) (Scope, error)
}

// Scope gives read-only access below its anchor, with slash-separated
// io/fs names. It never follows a link, junction or other reparse point,
// in the name or in its parents; such entries are reported through Lstat
// modes (fs.ModeSymlink or fs.ModeIrregular) and refused by ReadDir and
// OpenRegular. The plan reads the vault only through it.
type Scope interface {
	Lstat(name string) (fs.FileInfo, error)
	ReadDir(name string) ([]fs.DirEntry, error)
	OpenRegular(name string) (fs.File, error)
	SameFile(a, b fs.FileInfo) bool
	Close() error
}
