//go:build windows

package filesystem

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

// Attributes and flags missing from package syscall.
const (
	fileAttributeOffline            = 0x00001000
	fileAttributeEncrypted          = 0x00004000
	fileAttributeRecallOnOpen       = 0x00040000
	fileAttributeRecallOnDataAccess = 0x00400000
	fileFlagOpenNoRecall            = 0x00100000

	errorSharingViolation syscall.Errno = 32
	errorInvalidDrive     syscall.Errno = 15
	errorNotReady         syscall.Errno = 21
)

const placeholderAttributes = fileAttributeOffline | fileAttributeRecallOnOpen | fileAttributeRecallOnDataAccess

// Scope is a read-only view below an anchor directory. Directories it
// traverses stay pinned until Close.
type Scope struct {
	base string
	root *os.Root
	pins map[string]syscall.Handle
}

func openScope(root string) (*Scope, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return nil, fmt.Errorf("scope anchor %q must be an absolute, clean directory path", root)
	}
	handle, err := openNoFollow(root, root, true)
	if err != nil {
		return nil, err
	}
	scopeRoot, err := os.OpenRoot(root)
	if err != nil {
		_ = syscall.CloseHandle(handle)
		return nil, err
	}
	return &Scope{base: root, root: scopeRoot, pins: map[string]syscall.Handle{".": handle}}, nil
}

func (s *Scope) native(name string) string { return filepath.FromSlash(name) }

func (s *Scope) absolute(name string) string {
	if name == "." {
		return s.base
	}
	return filepath.Join(s.base, filepath.FromSlash(name))
}

// pin opens and keeps every directory from the anchor down to dir, refusing
// any that is a reparse point. Held handles deny deletion and renaming, so
// a pinned directory cannot be swapped for a junction afterwards.
func (s *Scope) pin(dir string) error {
	if dir == "." {
		return nil
	}
	for _, prefix := range append(prefixes(dir), dir) {
		if _, ok := s.pins[prefix]; ok {
			continue
		}
		handle, err := openNoFollow(prefix, s.absolute(prefix), true)
		if err != nil {
			return err
		}
		s.pins[prefix] = handle
	}
	return nil
}

// Lstat describes name without following it. Reparse points that Go would
// report as regular files or directories, for example when GODEBUG
// restores older behaviour, are reported as fs.ModeIrregular.
func (s *Scope) Lstat(name string) (fs.FileInfo, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	if err := s.pin(parent(name)); err != nil {
		return nil, err
	}
	info, err := s.root.Lstat(s.native(name))
	if errors.Is(err, errorSharingViolation) {
		// A file held without sharing, such as a save locked by its game or
		// the lock of a running sync, cannot be opened. Its directory entry
		// is read instead, through parents that are pinned and verified.
		info, err = os.Lstat(longPath(s.absolute(name)))
	}
	if err != nil {
		return nil, err
	}
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return nil, &fs.PathError{Op: "lstat", Path: name, Err: errors.New("file attributes unavailable")}
	}
	if data.FileAttributes&(syscall.FILE_ATTRIBUTE_REPARSE_POINT|placeholderAttributes) != 0 && info.Mode()&(fs.ModeSymlink|fs.ModeIrregular) == 0 {
		return reparseInfo{info}, nil
	}
	if data.FileAttributes&fileAttributeEncrypted != 0 {
		return encryptedInfo{info}, nil
	}
	return info, nil
}

// ReadDir lists a directory after pinning it and its parents.
func (s *Scope) ReadDir(name string) ([]fs.DirEntry, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	if err := s.pin(name); err != nil {
		return nil, err
	}
	dir, err := s.root.Open(s.native(name))
	if err != nil {
		return nil, err
	}
	defer func() { _ = dir.Close() }()
	return dir.ReadDir(-1)
}

// OpenRegular opens name for reading without following it. The handle is
// refused unless it denotes a regular file that is neither a reparse point
// nor an offline or cloud placeholder.
func (s *Scope) OpenRegular(name string) (fs.File, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	if name == "." {
		return nil, &fs.PathError{Op: "open", Path: name, Err: errNotRegular}
	}
	if err := s.pin(parent(name)); err != nil {
		return nil, err
	}
	path := s.absolute(name)
	handle, err := openNoFollow(name, path, false)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), path), nil
}

// Identity returns the volume serial number and file index of name,
// read on a handle opened without following it and closed at once. A link,
// junction, reparse point or placeholder is refused.
func (s *Scope) Identity(name string) (backup.FileID, error) {
	info, err := s.Lstat(name)
	if err != nil {
		return backup.FileID{}, err
	}
	if info.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
		return backup.FileID{}, &NotFollowedError{Name: name, Reason: "link, junction, reparse point or placeholder"}
	}
	handle, err := openNoFollow(name, s.absolute(name), info.IsDir())
	if err != nil {
		return backup.FileID{}, err
	}
	defer func() { _ = syscall.CloseHandle(handle) }()
	var data syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(handle, &data); err != nil {
		return backup.FileID{}, &fs.PathError{Op: "identity", Path: name, Err: err}
	}
	return backup.FileID{Volume: data.VolumeSerialNumber, Index: uint64(data.FileIndexHigh)<<32 | uint64(data.FileIndexLow)}, nil
}

// SameFile compares volume serial numbers and file indexes.
func (s *Scope) SameFile(a, b fs.FileInfo) bool {
	return os.SameFile(unwrap(a), unwrap(b))
}

// Close releases every pinned directory.
func (s *Scope) Close() error {
	var errs []error
	for name, handle := range s.pins {
		if err := syscall.CloseHandle(handle); err != nil {
			errs = append(errs, fmt.Errorf("release %s: %w", name, err))
		}
	}
	s.pins = map[string]syscall.Handle{}
	errs = append(errs, s.root.Close())
	return errors.Join(errs...)
}

var errNotRegular = errors.New("not a regular file")

// parent returns the directory holding name, "." for the anchor.
func parent(name string) string {
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		return name[:i]
	}
	return "."
}

// openNoFollow opens path for reading with FILE_FLAG_OPEN_REPARSE_POINT and
// verifies the handle. Directories are opened without FILE_SHARE_DELETE so
// they cannot be renamed or deleted while held. name is used in errors.
func openNoFollow(name, path string, directory bool) (syscall.Handle, error) {
	pointer, err := syscall.UTF16PtrFromString(longPath(path))
	if err != nil {
		return syscall.InvalidHandle, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	// BACKUP_SEMANTICS lets a directory or a directory junction be opened
	// even where a file was expected, so the handle check below classifies
	// it instead of failing with a bare access-denied error.
	flags := uint32(syscall.FILE_FLAG_OPEN_REPARSE_POINT | syscall.FILE_FLAG_BACKUP_SEMANTICS | fileFlagOpenNoRecall)
	handle, err := syscall.CreateFile(pointer, syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, flags, 0)
	if err != nil {
		return syscall.InvalidHandle, &fs.PathError{Op: "open", Path: name, Err: describe(err)}
	}
	var data syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(handle, &data); err != nil {
		_ = syscall.CloseHandle(handle)
		return syscall.InvalidHandle, &fs.PathError{Op: "inspect handle", Path: name, Err: err}
	}
	isDirectory := data.FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY != 0
	switch {
	case data.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0:
		err = &NotFollowedError{Name: name, Reason: "reparse point (junction, symbolic link or cloud file)"}
	case data.FileAttributes&placeholderAttributes != 0:
		err = &NotFollowedError{Name: name, Reason: "offline or cloud placeholder"}
	case directory && !isDirectory:
		err = &fs.PathError{Op: "open", Path: name, Err: errNotDirectory}
	case !directory && isDirectory:
		err = &fs.PathError{Op: "open", Path: name, Err: errNotRegular}
	}
	if err != nil {
		_ = syscall.CloseHandle(handle)
		return syscall.InvalidHandle, err
	}
	return handle, nil
}

// describe maps volume errors to fs.ErrNotExist and names sharing locks.
func describe(err error) error {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case errorInvalidDrive, errorNotReady:
			return fmt.Errorf("volume not available: %w", fs.ErrNotExist)
		case errorSharingViolation:
			return fmt.Errorf("locked by another process: %w", err)
		}
	}
	return err
}

// longPath prefixes \\?\ so CreateFile reads the clean absolute path
// literally, beyond MAX_PATH and without Win32 normalisation.
func longPath(path string) string {
	if strings.HasPrefix(path, `\\`) {
		return path
	}
	return `\\?\` + path
}

// encryptedInfo marks an EFS-encrypted object, so that callers can refuse
// to copy it in clear without importing this package.
type encryptedInfo struct{ fs.FileInfo }

func (encryptedInfo) Encrypted() bool { return true }

type reparseInfo struct{ fs.FileInfo }

func (r reparseInfo) Mode() fs.FileMode { return r.FileInfo.Mode()&fs.ModePerm | fs.ModeIrregular }
func (r reparseInfo) IsDir() bool       { return false }

func unwrap(info fs.FileInfo) fs.FileInfo {
	switch wrapped := info.(type) {
	case reparseInfo:
		return wrapped.FileInfo
	case encryptedInfo:
		return wrapped.FileInfo
	}
	return info
}
