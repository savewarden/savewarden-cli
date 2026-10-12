//go:build unix

package filesystem

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

// Scope is a read-only view below an anchor directory. On Unix it exists to
// test the scope contract; inspect only supports Windows installations.
type Scope struct {
	base string
	root *os.Root
}

func openScope(root string) (*Scope, error) {
	if !filepath.IsAbs(root) {
		// A Windows volume such as D:\ is a relative name here; never let it
		// resolve against the working directory.
		return nil, fmt.Errorf("volume %s is not available on %s: inspect supports Windows installations only", root, runtime.GOOS)
	}
	scopeRoot, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	return &Scope{base: root, root: scopeRoot}, nil
}

// parents checks every directory between the anchor and name without
// following links, and returns their descriptions.
func (s *Scope) parents(name string) ([]fs.FileInfo, error) {
	var chain []fs.FileInfo
	for _, prefix := range prefixes(name) {
		info, err := s.root.Lstat(prefix)
		switch {
		case err != nil:
			return nil, err
		case info.Mode()&fs.ModeSymlink != 0:
			return nil, &NotFollowedError{Name: prefix, Reason: "symbolic link"}
		case !info.IsDir():
			return nil, &fs.PathError{Op: "lstat", Path: prefix, Err: errNotDirectory}
		}
		chain = append(chain, info)
	}
	return chain, nil
}

// unchanged verifies, after an open, that no parent was exchanged.
func (s *Scope) unchanged(name string, chain []fs.FileInfo) error {
	again, err := s.parents(name)
	if err != nil {
		return err
	}
	for i := range chain {
		if !os.SameFile(chain[i], again[i]) {
			return &NotFollowedError{Name: name, Reason: "a parent directory changed while opening"}
		}
	}
	return nil
}

// Lstat describes name without following it or any parent.
func (s *Scope) Lstat(name string) (fs.FileInfo, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	if _, err := s.parents(name); err != nil {
		return nil, err
	}
	return s.root.Lstat(name)
}

// ReadDir lists a directory that is not, and is not reached through, a link.
func (s *Scope) ReadDir(name string) ([]fs.DirEntry, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	chain, err := s.parents(name)
	if err != nil {
		return nil, err
	}
	before, err := s.root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if before.Mode()&fs.ModeSymlink != 0 {
		return nil, &NotFollowedError{Name: name, Reason: "symbolic link"}
	}
	dir, err := s.root.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = dir.Close() }()
	opened, err := dir.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(before, opened) {
		return nil, &NotFollowedError{Name: name, Reason: "replaced while opening"}
	}
	if err := s.unchanged(name, chain); err != nil {
		return nil, err
	}
	return dir.ReadDir(-1)
}

// OpenRegular opens name with O_NOFOLLOW, then checks the object and parents.
func (s *Scope) OpenRegular(name string) (fs.File, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	chain, err := s.parents(name)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(s.base, filepath.FromSlash(name))
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if errors.Is(err, syscall.ELOOP) || errors.Is(err, syscall.EMLINK) {
		return nil, &NotFollowedError{Name: name, Reason: "symbolic link"}
	}
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = &fs.PathError{Op: "open", Path: name, Err: errors.New("not a regular file")}
	}
	if err == nil {
		err = s.unchanged(name, chain)
	}
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

// SameFile compares device and inode numbers.
func (s *Scope) SameFile(a, b fs.FileInfo) bool { return os.SameFile(a, b) }

// Close releases the anchor.
func (s *Scope) Close() error { return s.root.Close() }
