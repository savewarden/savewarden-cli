package inspect_test

import (
	"bytes"
	"errors"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/game"
	"github.com/savewarden/savewarden-cli/internal/features/inspect"
)

// node is an in-memory file-system object. Hooks simulate concurrent
// changes at precise moments so race handling is tested on every OS.
type node struct {
	mode     fs.FileMode
	data     []byte
	modTime  time.Time
	id       int
	children map[string]*node
	lstatErr error
	listErr  error
	openErr  error
	onOpen   func() // runs inside OpenRegular, before the object is opened
	onRead   func() // runs once, after the first Read
}

var epoch = time.Date(2026, 10, 9, 18, 32, 8, 123456700, time.FixedZone("CEST", 2*3600))

type fakeFS struct {
	volumes  map[string]*node
	scopeErr error
	nextID   int
	opened   []string
	listed   []string
}

func newFakeFS() *fakeFS {
	f := &fakeFS{volumes: map[string]*node{}}
	f.volumes[`D:\`] = f.dir()
	return f
}

func (f *fakeFS) id() int { f.nextID++; return f.nextID }

func (f *fakeFS) dir() *node {
	return &node{mode: fs.ModeDir | 0o755, modTime: epoch, id: f.id(), children: map[string]*node{}}
}

func (f *fakeFS) file(data string) *node {
	return &node{mode: 0o644, data: []byte(data), modTime: epoch, id: f.id()}
}

func (f *fakeFS) special(mode fs.FileMode) *node {
	return &node{mode: mode, modTime: epoch, id: f.id(), children: map[string]*node{}}
}

// put creates parents as needed and places n at a slash path below D:\.
func (f *fakeFS) put(path string, n *node) *node {
	parent := f.volumes[`D:\`]
	parts := strings.Split(path, "/")
	for _, part := range parts[:len(parts)-1] {
		child, ok := parent.children[part]
		if !ok {
			child = f.dir()
			parent.children[part] = child
		}
		parent = child
	}
	parent.children[parts[len(parts)-1]] = n
	return n
}

func (f *fakeFS) remove(path string) {
	parent, name := f.parentOf(path)
	delete(parent.children, name)
}

func (f *fakeFS) parentOf(path string) (*node, string) {
	parent := f.volumes[`D:\`]
	parts := strings.Split(path, "/")
	for _, part := range parts[:len(parts)-1] {
		parent = parent.children[part]
	}
	return parent, parts[len(parts)-1]
}

func (f *fakeFS) OpenScope(root string) (inspect.Scope, error) {
	if f.scopeErr != nil {
		return nil, f.scopeErr
	}
	volume, ok := f.volumes[root]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return &fakeScope{fs: f, root: volume}, nil
}

type notFollowedError struct{ name string }

func (e *notFollowedError) Error() string     { return e.name + ": link not followed" }
func (e *notFollowedError) NotFollowed() bool { return true }

type fakeScope struct {
	fs   *fakeFS
	root *node
}

func isLinkMode(mode fs.FileMode) bool { return mode&(fs.ModeSymlink|fs.ModeIrregular) != 0 }

// walkTo resolves the parent directories of name, refusing links as the
// Scope contract requires, and returns the parent and the final element.
func (s *fakeScope) walkTo(name string) (*node, string, error) {
	if name == "." {
		return nil, "", errors.New("anchor has no parent")
	}
	parent := s.root
	parts := strings.Split(name, "/")
	for _, part := range parts[:len(parts)-1] {
		child, ok := parent.children[part]
		switch {
		case !ok:
			return nil, "", fs.ErrNotExist
		case isLinkMode(child.mode):
			return nil, "", &notFollowedError{part}
		case !child.mode.IsDir():
			return nil, "", errors.New(part + ": not a directory")
		}
		parent = child
	}
	return parent, parts[len(parts)-1], nil
}

func (s *fakeScope) Lstat(name string) (fs.FileInfo, error) {
	parent, base, err := s.walkTo(name)
	if err != nil {
		return nil, err
	}
	n, ok := parent.children[base]
	if !ok {
		return nil, fs.ErrNotExist
	}
	if n.lstatErr != nil {
		return nil, n.lstatErr
	}
	return info(base, n), nil
}

func (s *fakeScope) ReadDir(name string) ([]fs.DirEntry, error) {
	s.fs.listed = append(s.fs.listed, name)
	parent, base, err := s.walkTo(name)
	if err != nil {
		return nil, err
	}
	n, ok := parent.children[base]
	switch {
	case !ok:
		return nil, fs.ErrNotExist
	case isLinkMode(n.mode):
		return nil, &notFollowedError{name}
	case n.listErr != nil:
		return nil, n.listErr
	}
	entries := make([]fs.DirEntry, 0, len(n.children))
	for childName, child := range n.children {
		entries = append(entries, fs.FileInfoToDirEntry(info(childName, child)))
	}
	// Reverse order proves the service sorts by itself.
	sort.Slice(entries, func(a, b int) bool { return entries[a].Name() > entries[b].Name() })
	return entries, nil
}

func (s *fakeScope) OpenRegular(name string) (fs.File, error) {
	parent, base, err := s.walkTo(name)
	if err != nil {
		return nil, err
	}
	if n, ok := parent.children[base]; ok && n.onOpen != nil {
		n.onOpen()
	}
	n, ok := parent.children[base]
	switch {
	case !ok:
		return nil, fs.ErrNotExist
	case isLinkMode(n.mode):
		return nil, &notFollowedError{name}
	case !n.mode.IsRegular():
		return nil, errors.New(name + ": not a regular file")
	case n.openErr != nil:
		return nil, n.openErr
	}
	s.fs.opened = append(s.fs.opened, name)
	return &fakeFile{name: base, node: n, reader: bytes.NewReader(append([]byte(nil), n.data...))}, nil
}

func (s *fakeScope) SameFile(a, b fs.FileInfo) bool {
	return a.Sys().(int) == b.Sys().(int)
}

func (s *fakeScope) Close() error { return nil }

type fakeFile struct {
	name   string
	node   *node
	reader *bytes.Reader
	reads  int
}

func (f *fakeFile) Read(p []byte) (int, error) {
	n, err := f.reader.Read(p)
	f.reads++
	if f.reads == 1 && f.node.onRead != nil {
		f.node.onRead()
	}
	return n, err
}

func (f *fakeFile) Stat() (fs.FileInfo, error) { return info(f.name, f.node), nil }
func (f *fakeFile) Close() error               { return nil }

type fakeInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
	id      int
}

func info(name string, n *node) fs.FileInfo {
	return fakeInfo{name, int64(len(n.data)), n.mode, n.modTime, n.id}
}

func (i fakeInfo) Name() string       { return i.name }
func (i fakeInfo) Size() int64        { return i.size }
func (i fakeInfo) Mode() fs.FileMode  { return i.mode }
func (i fakeInfo) ModTime() time.Time { return i.modTime }
func (i fakeInfo) IsDir() bool        { return i.mode.IsDir() }
func (i fakeInfo) Sys() any           { return i.id }

type fakeManifests struct {
	manifest game.Manifest
	err      error
}

func (f fakeManifests) Read(string) (game.Manifest, error) {
	if f.err != nil {
		return game.Manifest{}, f.err
	}
	return f.manifest, f.manifest.Validate()
}
