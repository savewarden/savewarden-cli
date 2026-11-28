package sync

import (
	"bytes"
	"io/fs"
	"path"
	"strings"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

// sessionScope reads the directory a session holds as a Scope anchored on
// its volume root would, but through the session: a decision on that
// directory then rests on what the session holds, not on a drive letter
// resolved again (plan B5 §4.3, step 5). Scope names below root, the scope
// name of the session's directory, map to names of the session; any other
// name is refused. Every directory is held before anything in it is read,
// so that no name resolves through a component the session does not pin.
// It writes nothing: Hold pins, it never creates.
type sessionScope struct {
	vault Vault
	root  string
}

// rel maps a scope name to a name of the session.
func (s sessionScope) rel(name string) (string, error) {
	switch {
	case name == s.root:
		return ".", nil
	case strings.HasPrefix(name, s.root+"/"):
		return name[len(s.root)+1:], nil
	}
	return "", &fs.PathError{Op: "session scope", Path: name, Err: fs.ErrPermission}
}

// holdParent holds every directory from the session's directory down to the
// parent of rel, which must be held before rel is read.
func (s sessionScope) holdParent(rel string) error {
	dir := path.Dir(rel)
	if rel == "." || dir == "." {
		return nil
	}
	parts := strings.Split(dir, "/")
	for k := range parts {
		if err := s.vault.Hold(strings.Join(parts[:k+1], "/")); err != nil {
			return err
		}
	}
	return nil
}

func (s sessionScope) Lstat(name string) (fs.FileInfo, error) {
	rel, err := s.rel(name)
	if err != nil {
		return nil, err
	}
	if err := s.holdParent(rel); err != nil {
		return nil, err
	}
	mode, err := s.vault.Stat(rel)
	if err != nil {
		return nil, err
	}
	info := sessionInfo{name: path.Base(name), mode: mode}
	if !isLink(mode) {
		if id, err := s.vault.Identity(rel); err == nil {
			info.id = id
		}
	}
	return info, nil
}

func (s sessionScope) ReadDir(name string) ([]fs.DirEntry, error) {
	rel, err := s.rel(name)
	if err != nil {
		return nil, err
	}
	if err := s.holdParent(rel); err != nil {
		return nil, err
	}
	return s.vault.ReadDir(rel)
}

// OpenRegular reads the whole file through the session, at most
// recordReadLimit bytes and one more, the largest a reading of an area
// accepts; readFile then applies its own, smaller limit.
func (s sessionScope) OpenRegular(name string) (fs.File, error) {
	rel, err := s.rel(name)
	if err != nil {
		return nil, err
	}
	if err := s.holdParent(rel); err != nil {
		return nil, err
	}
	data, err := s.vault.ReadFile(rel, recordReadLimit+1)
	if err != nil {
		return nil, err
	}
	return &sessionFile{info: sessionInfo{name: path.Base(name), size: int64(len(data))}, reader: bytes.NewReader(data)}, nil
}

// SameFile compares the identities Lstat read; an identity it could not
// read is never the same as another.
func (s sessionScope) SameFile(a, b fs.FileInfo) bool {
	x, okA := a.(sessionInfo)
	y, okB := b.(sessionInfo)
	return okA && okB && x.id != (backup.FileID{}) && x.id == y.id
}

func (s sessionScope) Identity(name string) (backup.FileID, error) {
	rel, err := s.rel(name)
	if err != nil {
		return backup.FileID{}, err
	}
	if err := s.holdParent(rel); err != nil {
		return backup.FileID{}, err
	}
	return s.vault.Identity(rel)
}

// Close closes nothing: the session belongs to its operation.
func (s sessionScope) Close() error { return nil }

type sessionInfo struct {
	name string
	mode fs.FileMode
	size int64
	id   backup.FileID
}

func (i sessionInfo) Name() string       { return i.name }
func (i sessionInfo) Size() int64        { return i.size }
func (i sessionInfo) Mode() fs.FileMode  { return i.mode }
func (i sessionInfo) ModTime() time.Time { return time.Time{} }
func (i sessionInfo) IsDir() bool        { return i.mode.IsDir() }
func (i sessionInfo) Sys() any           { return nil }

type sessionFile struct {
	info   sessionInfo
	reader *bytes.Reader
}

func (f *sessionFile) Read(p []byte) (int, error) { return f.reader.Read(p) }
func (f *sessionFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *sessionFile) Close() error               { return nil }

// The scope must keep the contract of the port.
var _ Scope = sessionScope{}
