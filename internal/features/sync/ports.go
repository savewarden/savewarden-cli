package sync

import (
	"context"
	"io"
	"io/fs"
	"time"

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
	// Identity returns the volume serial number and file index of name,
	// refusing links like Lstat reports them.
	Identity(name string) (backup.FileID, error)
	Close() error
}

// Volumes describes, without writing, the volume holding an existing
// directory: file system, drive type and bytes available to the caller.
type Volumes interface {
	Describe(path string) (backup.Volume, error)
}

// Vaults opens create-only sessions on a vault path, or on a restore area,
// which a session serves as it serves a vault; a restore never opens one on
// the vault (ADR 0006 §11). Only --apply and --recover receive it: the plan
// never does (ADR 0007, ADR 0009).
type Vaults interface {
	Open(path string) (Vault, error)
}

// Vault creates below one vault directory or restore area, with
// slash-separated names relative to it, "." being that directory itself
// (ADR 0005). It never replaces, truncates, deletes or follows anything. A
// creation, rename or identity check needs its parent directory held; the
// directory's own parent always is.
type Vault interface {
	// CreateDir creates a directory and holds it; fs.ErrExist if the name
	// is taken.
	CreateDir(name string) error
	// Hold pins an existing plain directory until Release or Close.
	Hold(name string) error
	// Release unpins dir and everything held below it, so it can be moved.
	Release(dir string) error
	// NewFile creates a regular file, fs.ErrExist if the name is taken.
	// Close dates it unless modTime is zero, flushes and closes it.
	NewFile(name string, modTime time.Time) (io.WriteCloser, error)
	// Move renames without ever replacing; a transient sharing or lock
	// violation has a Busy method that returns true.
	Move(from, to string) error
	// Identity returns the volume serial number and file index of name.
	Identity(name string) (backup.FileID, error)
	// Lock takes a lock file exclusively until Close, sync.lock in a vault,
	// restore.lock in a restore area, created empty if missing; a second
	// holder gets a busy error.
	Lock(name string) error
	Close() error

	// The reads below write nothing. They are the checks under the session
	// of ADR 0006 §11: what the session holds, observed through it rather
	// than through a letter resolved again by the plan's adapters.

	// Chain returns the identities of the directories the session holds,
	// from the volume root down to its directory, or down to that
	// directory's parent while it does not exist, read on the held handles.
	Chain() ([]backup.FileID, error)
	// Stat reports what name is without following it: fs.ModeDir, 0 for a
	// regular file, fs.ModeIrregular for a link or reparse point;
	// fs.ErrNotExist if absent, "." included.
	Stat(name string) (fs.FileMode, error)
	// ReadDir lists a plain directory without following anything.
	ReadDir(name string) ([]fs.DirEntry, error)
	// ReadFile reads a regular file without following it, refusing more
	// than limit bytes.
	ReadFile(name string, limit int64) ([]byte, error)
	// MarkedAncestor returns the depth, 0 for the volume root, of the first
	// held ancestor of the directory that contains one of names, or -1.
	MarkedAncestor(names ...string) (int, error)
}

// Clock dates operations and paces the bounded retries of ADR 0006 §7.
type Clock interface {
	Now() time.Time
	Sleep(ctx context.Context, d time.Duration) error
}
