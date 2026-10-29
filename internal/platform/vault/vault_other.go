//go:build !windows

package vault

import (
	"errors"
	"io"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

// Session is unavailable on this platform: POSIX rename(2) replaces its
// target, which the protocol of ADR 0006 forbids.
type Session struct{}

// Open is unavailable on this platform.
func Open(string) (*Session, error) { return nil, errors.ErrUnsupported }

// CreateDir is unavailable on this platform.
func (*Session) CreateDir(string) error { return errors.ErrUnsupported }

// Hold is unavailable on this platform.
func (*Session) Hold(string) error { return errors.ErrUnsupported }

// Release is unavailable on this platform.
func (*Session) Release(string) error { return errors.ErrUnsupported }

// NewFile is unavailable on this platform.
func (*Session) NewFile(string, time.Time) (io.WriteCloser, error) {
	return nil, errors.ErrUnsupported
}

// Move is unavailable on this platform.
func (*Session) Move(string, string) error { return errors.ErrUnsupported }

// Identity is unavailable on this platform.
func (*Session) Identity(string) (backup.FileID, error) {
	return backup.FileID{}, errors.ErrUnsupported
}

// Lock is unavailable on this platform.
func (*Session) Lock(string) error { return errors.ErrUnsupported }

// Close is unavailable on this platform.
func (*Session) Close() error { return nil }

// Describe is unavailable on this platform.
func Describe(string) (backup.Volume, error) { return backup.Volume{}, errors.ErrUnsupported }
