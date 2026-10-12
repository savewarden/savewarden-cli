//go:build !windows && !unix

package filesystem

import (
	"errors"
	"io/fs"
)

// Scope is unavailable on this platform.
type Scope struct{}

func openScope(string) (*Scope, error) { return nil, errors.ErrUnsupported }

// Lstat is unavailable on this platform.
func (*Scope) Lstat(string) (fs.FileInfo, error) { return nil, errors.ErrUnsupported }

// ReadDir is unavailable on this platform.
func (*Scope) ReadDir(string) ([]fs.DirEntry, error) { return nil, errors.ErrUnsupported }

// OpenRegular is unavailable on this platform.
func (*Scope) OpenRegular(string) (fs.File, error) { return nil, errors.ErrUnsupported }

// SameFile is unavailable on this platform.
func (*Scope) SameFile(fs.FileInfo, fs.FileInfo) bool { return false }

// Close is unavailable on this platform.
func (*Scope) Close() error { return nil }
