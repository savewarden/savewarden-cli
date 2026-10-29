// Package vault is the only package of Savewarden allowed to write
// (ADR 0005). A Session creates directories and files below one vault
// directory, and renames with MoveFileEx and flags 0. It never replaces,
// truncates, deletes or follows anything:
//
//   - a directory or file is created only if its name is free: CreateDirectory,
//     or CreateFile with CREATE_NEW;
//   - a rename never replaces an existing target and never copies across
//     volumes;
//   - every directory from the volume root down to the vault, and every
//     directory the session creates or holds below it, is opened with
//     FILE_FLAG_OPEN_REPARSE_POINT, checked on its handle and kept open
//     without FILE_SHARE_DELETE until released, so it cannot be renamed or
//     swapped for a junction while names are created in it. os.Root is not
//     used: it follows links that stay inside its tree;
//   - every name is checked against the Windows grammar of entities/game, and
//     paths are passed with the \\?\ prefix, literally.
//
// What to create, commit or move, and when, is decided by the sync feature
// according to ADR 0006; this package offers the primitives and nothing
// else. It is Windows only: POSIX rename(2) replaces its target, so other
// systems get errors.ErrUnsupported.
package vault

import (
	"io/fs"
	"strings"

	"github.com/savewarden/savewarden-cli/internal/entities/game"
)

// BusyError reports a sharing or lock violation, Windows errors 32 and 33:
// the only failures the caller may retry (ADR 0006 §7).
type BusyError struct {
	Op   string
	Name string
	Err  error
}

func (e *BusyError) Error() string {
	return e.Op + " " + e.Name + ": in use by another process: " + e.Err.Error()
}

// Unwrap returns the system error.
func (e *BusyError) Unwrap() error { return e.Err }

// Busy marks the error as transient for callers that cannot import this
// package.
func (e *BusyError) Busy() bool { return true }

// NotFollowedError refuses a link, junction, reparse point or placeholder.
type NotFollowedError struct {
	Name   string
	Reason string
}

func (e *NotFollowedError) Error() string { return e.Name + ": " + e.Reason + "; not followed" }

// NotFollowed marks the refusal for callers that cannot import this package.
func (e *NotFollowedError) NotFollowed() bool { return true }

// checkName accepts "." for the vault itself, or an io/fs name whose every
// segment Windows keeps as written: no trailing dot or space, no reserved
// device name, no stream separator.
func checkName(name string) error {
	if name == "." {
		return nil
	}
	if !fs.ValidPath(name) || strings.Contains(name, `\`) {
		return &fs.PathError{Op: "vault", Path: name, Err: fs.ErrInvalid}
	}
	for _, segment := range strings.Split(name, "/") {
		if err := game.CheckWindowsName(segment); err != nil {
			return &fs.PathError{Op: "vault", Path: name, Err: err}
		}
	}
	return nil
}

// parent returns the directory holding name: "." for a name at the vault
// root, and "" for the vault itself, whose parent is outside the session.
func parent(name string) string {
	if name == "." {
		return ""
	}
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		return name[:i]
	}
	return "."
}

// below reports whether name is dir or lies inside it.
func below(name, dir string) bool {
	return dir == "." || name == dir || strings.HasPrefix(name, dir+"/")
}
