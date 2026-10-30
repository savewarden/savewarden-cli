package sync_test

import (
	"bytes"
	"errors"
	"io/fs"
	"strings"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/entities/game"
	"github.com/savewarden/savewarden-cli/internal/features/inspect"
	"github.com/savewarden/savewarden-cli/internal/features/sync"
)

// node is an in-memory file-system object. The same node may appear under
// two names, like an 8.3 short name; links are irregular nodes.
type node struct {
	mode     fs.FileMode
	data     []byte
	modTime  time.Time
	id       int
	crypted  bool
	listErr  error
	busy     bool // held without sharing: cannot be opened
	children map[string]*node
}

var epoch = time.Date(2026, 10, 9, 16, 32, 8, 123456700, time.UTC)

type fakeFS struct {
	volumes map[string]*node
	nextID  int
	opened  int
	// described overrides, per volume root, what fakeVolumes reports: by
	// default a fixed NTFS drive with 1 TiB available.
	volumeOf map[string]backup.Volume
	// onOpen, if set, runs when a file is opened for reading, before its
	// content is read: tests change the source there.
	onOpen func(name string, n *node)
	// later, if set, is what fakeVolumes reports from its second call on,
	// such as the check under lock.
	later     *backup.Volume
	described int
}

// fakeVolumes describes the fake volumes; a path on a missing volume fails.
type fakeVolumes struct{ f *fakeFS }

func (v fakeVolumes) Describe(path string) (backup.Volume, error) {
	root := path[:3]
	if _, ok := v.f.volumes[root]; !ok {
		return backup.Volume{}, fs.ErrNotExist
	}
	v.f.described++
	if v.f.later != nil && v.f.described > 1 {
		return *v.f.later, nil
	}
	if described, ok := v.f.volumeOf[root]; ok {
		return described, nil
	}
	return backup.Volume{FileSystem: "NTFS", Drive: backup.DriveFixed, Available: 1 << 40}, nil
}

func newFakeFS(volumes ...string) *fakeFS {
	f := &fakeFS{volumes: map[string]*node{}, volumeOf: map[string]backup.Volume{}}
	for _, volume := range volumes {
		f.volumes[volume] = f.dir()
	}
	return f
}

func (f *fakeFS) dir() *node {
	f.nextID++
	return &node{mode: fs.ModeDir | 0o755, modTime: epoch, id: f.nextID, children: map[string]*node{}}
}

func (f *fakeFS) file(data string) *node {
	f.nextID++
	return &node{mode: 0o644, data: []byte(data), modTime: epoch, id: f.nextID}
}

func (f *fakeFS) link() *node {
	f.nextID++
	return &node{mode: fs.ModeIrregular, modTime: epoch, id: f.nextID, children: map[string]*node{}}
}

// put places n at a Windows path such as D:\Games\x, creating parents.
func (f *fakeFS) put(path string, n *node) *node {
	parent, name := f.parentOf(path, true)
	parent.children[name] = n
	return n
}

// mkdir creates a directory and its parents and returns it.
func (f *fakeFS) mkdir(path string) *node {
	if n := f.get(path); n != nil {
		return n
	}
	return f.put(path, f.dir())
}

func (f *fakeFS) get(path string) *node {
	parent, name := f.parentOf(path, false)
	if parent == nil {
		return nil
	}
	return parent.children[name]
}

func (f *fakeFS) parentOf(path string, create bool) (*node, string) {
	volume, rest := path[:3], strings.Split(path[3:], `\`)
	parent := f.volumes[volume]
	for _, part := range rest[:len(rest)-1] {
		child, ok := parent.children[part]
		if !ok {
			if !create {
				return nil, ""
			}
			child = f.dir()
			parent.children[part] = child
		}
		parent = child
	}
	return parent, rest[len(rest)-1]
}

func (f *fakeFS) scope(root string) (*fakeScope, error) {
	volume, ok := f.volumes[root]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return &fakeScope{fs: f, root: volume}, nil
}

// inspectFS and syncFiles adapt the fake to each feature's port, as app
// adapts the real file system.
type inspectFS struct{ f *fakeFS }

func (a inspectFS) OpenScope(root string) (inspect.Scope, error) {
	scope, err := a.f.scope(root)
	if err != nil {
		return nil, err
	}
	return scope, nil
}

type syncFiles struct{ f *fakeFS }

func (a syncFiles) OpenScope(root string) (sync.Scope, error) {
	scope, err := a.f.scope(root)
	if err != nil {
		return nil, err
	}
	return scope, nil
}

type notFollowedError struct{ name string }

func (e *notFollowedError) Error() string     { return e.name + ": link not followed" }
func (e *notFollowedError) NotFollowed() bool { return true }

type fakeScope struct {
	fs   *fakeFS
	root *node
}

func isLink(mode fs.FileMode) bool { return mode&(fs.ModeSymlink|fs.ModeIrregular) != 0 }

func (s *fakeScope) lookup(name string) (*node, string, error) {
	if name == "." {
		return s.root, ".", nil
	}
	n := s.root
	parts := strings.Split(name, "/")
	for i, part := range parts {
		child, ok := n.children[part]
		switch {
		case !ok:
			return nil, "", fs.ErrNotExist
		case i < len(parts)-1 && isLink(child.mode):
			return nil, "", &notFollowedError{part}
		case i < len(parts)-1 && !child.mode.IsDir():
			return nil, "", errors.New(part + ": not a directory")
		}
		n = child
	}
	return n, parts[len(parts)-1], nil
}

func (s *fakeScope) Lstat(name string) (fs.FileInfo, error) {
	n, base, err := s.lookup(name)
	if err != nil {
		return nil, err
	}
	return info(base, n), nil
}

func (s *fakeScope) ReadDir(name string) ([]fs.DirEntry, error) {
	n, _, err := s.lookup(name)
	switch {
	case err != nil:
		return nil, err
	case isLink(n.mode):
		return nil, &notFollowedError{name}
	case !n.mode.IsDir():
		return nil, errors.New(name + ": not a directory")
	case n.listErr != nil:
		return nil, n.listErr
	}
	entries := make([]fs.DirEntry, 0, len(n.children))
	for childName, child := range n.children {
		entries = append(entries, fs.FileInfoToDirEntry(info(childName, child)))
	}
	return entries, nil
}

func (s *fakeScope) OpenRegular(name string) (fs.File, error) {
	n, base, err := s.lookup(name)
	switch {
	case err != nil:
		return nil, err
	case isLink(n.mode):
		return nil, &notFollowedError{name}
	case !n.mode.IsRegular():
		return nil, errors.New(name + ": not a regular file")
	case n.busy:
		return nil, busyError{}
	}
	s.fs.opened++
	opened := info(base, n)
	if s.fs.onOpen != nil {
		s.fs.onOpen(name, n)
	}
	return &fakeFile{info: opened, node: n, reader: bytes.NewReader(append([]byte(nil), n.data...))}, nil
}

func (s *fakeScope) SameFile(a, b fs.FileInfo) bool { return a.Sys().(int) == b.Sys().(int) }

func (s *fakeScope) Identity(name string) (backup.FileID, error) {
	n, _, err := s.lookup(name)
	switch {
	case err != nil:
		return backup.FileID{}, err
	case isLink(n.mode):
		return backup.FileID{}, &notFollowedError{name}
	case n.busy:
		return backup.FileID{}, errors.New(name + ": held without sharing, its identity cannot be read")
	}
	return backup.FileID{Volume: 0x1a2b3c4d, Index: uint64(n.id)}, nil //nolint:gosec // G115: node ids count up from 1.
}
func (s *fakeScope) Close() error { return nil }

type fakeFile struct {
	info   fs.FileInfo
	node   *node
	reader *bytes.Reader
	read   bool
}

func (f *fakeFile) Read(p []byte) (int, error) { f.read = true; return f.reader.Read(p) }

// Stat describes the file as opened, then, once read, as it is now.
func (f *fakeFile) Stat() (fs.FileInfo, error) {
	if f.read {
		return info(f.info.Name(), f.node), nil
	}
	return f.info, nil
}
func (f *fakeFile) Close() error { return nil }

type fakeInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
	id      int
	crypted bool
}

func info(name string, n *node) fs.FileInfo {
	return fakeInfo{name, int64(len(n.data)), n.mode, n.modTime, n.id, n.crypted}
}

func (i fakeInfo) Name() string       { return i.name }
func (i fakeInfo) Size() int64        { return i.size }
func (i fakeInfo) Mode() fs.FileMode  { return i.mode }
func (i fakeInfo) ModTime() time.Time { return i.modTime }
func (i fakeInfo) IsDir() bool        { return i.mode.IsDir() }
func (i fakeInfo) Sys() any           { return i.id }
func (i fakeInfo) Encrypted() bool    { return i.crypted }

type fakeManifests struct{ manifest game.Manifest }

func (f fakeManifests) Read(string) (game.Manifest, error) { return f.manifest, f.manifest.Validate() }
