// Package filesystem implements read-only, anchored scopes on the local
// file system for the inspect feature.
//
// Confinement and no-follow are different guarantees. os.Root confines
// names to a directory tree but follows symbolic links that stay inside it,
// so it is never relied upon alone. Each implementation additionally checks
// every component below the anchor and the object actually opened:
//
//   - Windows (supported): every directory traversed is opened with
//     FILE_FLAG_OPEN_REPARSE_POINT, verified on its handle and kept open
//     without FILE_SHARE_DELETE for the scope's lifetime, so it cannot be
//     renamed or replaced by a junction while the scope is used. Files are
//     opened the same way and refused if the handle shows a reparse point,
//     a directory or an offline or cloud placeholder.
//   - Unix (tests only, not a supported platform): parents are checked
//     with Lstat, files are opened with O_NOFOLLOW and the parents are
//     verified again after opening. An exchange-and-restore race between
//     the two checks remains possible and is documented.
package filesystem

import (
	"errors"
	"io/fs"
	"strings"
)

// OpenScope anchors a read-only scope on root, which must be an absolute
// directory. A missing volume or directory yields an fs.ErrNotExist error.
// The returned *Scope uses standard-library types only and satisfies the
// inspect feature's port without importing it.
func OpenScope(root string) (*Scope, error) {
	return openScope(root)
}

// NotFollowedError refuses a link, junction, reparse point or placeholder.
type NotFollowedError struct {
	Name   string
	Reason string
}

func (e *NotFollowedError) Error() string { return e.Name + ": " + e.Reason + "; not followed" }

// NotFollowed marks the refusal for the inspect feature.
func (e *NotFollowedError) NotFollowed() bool { return true }

// checkName accepts io/fs names whose elements cannot be reinterpreted by
// Windows as separators or stream names.
func checkName(name string) error {
	if !fs.ValidPath(name) || strings.ContainsAny(name, `\:`) {
		return &fs.PathError{Op: "scope", Path: name, Err: fs.ErrInvalid}
	}
	return nil
}

// prefixes lists the directories leading to name, from the anchor down,
// excluding the anchor itself: "a/b/c" gives "a", "a/b".
func prefixes(name string) []string {
	var out []string
	for i := 0; i < len(name); i++ {
		if name[i] == '/' {
			out = append(out, name[:i])
		}
	}
	return out
}

var errNotDirectory = errors.New("not a directory")
