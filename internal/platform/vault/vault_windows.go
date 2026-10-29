//go:build windows

package vault

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/entities/game"
)

const placeholderAttributes = windows.FILE_ATTRIBUTE_OFFLINE | windows.FILE_ATTRIBUTE_RECALL_ON_OPEN |
	windows.FILE_ATTRIBUTE_RECALL_ON_DATA_ACCESS

// noFollow opens the object a name denotes, never what it points to, and
// never recalls a cloud file.
const noFollow = windows.FILE_FLAG_OPEN_REPARSE_POINT | windows.FILE_FLAG_BACKUP_SEMANTICS | windows.FILE_FLAG_OPEN_NO_RECALL

// Session creates below one vault directory. It is not safe for
// concurrent use.
type Session struct {
	base string
	// ancestors pins the volume root and every directory down to the
	// vault's parent; pins holds the vault, under ".", and the directories
	// created or held below it.
	ancestors []windows.Handle
	pins      map[string]windows.Handle
	lock      windows.Handle
}

// Open starts a session on the vault directory path, an absolute clean
// path below a drive letter such as F:\Vault. Every existing directory from
// the volume root down to the vault, or to its parent when the vault does
// not exist yet, is opened without following it and pinned. A missing
// parent, a link, a junction, a placeholder or a file on the way is refused.
func Open(path string) (*Session, error) {
	volume := filepath.VolumeName(path)
	if len(volume) != 2 || volume[1] != ':' || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, fmt.Errorf("vault path %q must be an absolute, clean path below a drive letter", path)
	}
	var segments []string
	if rest := path[len(volume)+1:]; rest != "" {
		segments = strings.Split(rest, `\`)
	}
	for _, segment := range segments {
		if err := game.CheckWindowsName(segment); err != nil {
			return nil, fmt.Errorf("vault path %q: %w", path, err)
		}
	}
	s := &Session{base: path, pins: map[string]windows.Handle{}, lock: windows.InvalidHandle}
	current := volume + `\`
	for k := 0; k <= len(segments); k++ {
		if k > 0 {
			current = filepath.Join(current, segments[k-1])
		}
		handle, err := openDirectory(current, current)
		switch {
		case err == nil && k == len(segments):
			s.pins["."] = handle
		case err == nil:
			s.ancestors = append(s.ancestors, handle)
		case errors.Is(err, fs.ErrNotExist) && k == len(segments) && k > 0:
			return s, nil
		default:
			_ = s.Close()
			return nil, err
		}
	}
	return s, nil
}

func (s *Session) absolute(name string) string {
	if name == "." {
		return s.base
	}
	return filepath.Join(s.base, filepath.FromSlash(name))
}

// requireParent refuses a name whose parent directory is not pinned: every
// creation, rename and identity check happens in a held directory. The
// vault's own parent is pinned by Open.
func (s *Session) requireParent(name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	dir := parent(name)
	if _, held := s.pins[dir]; held || dir == "" {
		return nil
	}
	return &fs.PathError{Op: "vault", Path: name, Err: fmt.Errorf("parent directory %q is not held by the session", dir)}
}

// CreateDir creates the directory name, "." being the vault itself, and
// holds it. It fails with fs.ErrExist if anything has that name.
func (s *Session) CreateDir(name string) error {
	if err := s.requireParent(name); err != nil {
		return err
	}
	if _, held := s.pins[name]; held {
		return &fs.PathError{Op: "create directory", Path: name, Err: fs.ErrExist}
	}
	path := s.absolute(name)
	pointer, err := windows.UTF16PtrFromString(longPath(path))
	if err != nil {
		return &fs.PathError{Op: "create directory", Path: name, Err: err}
	}
	if err := windows.CreateDirectory(pointer, nil); err != nil {
		return &fs.PathError{Op: "create directory", Path: name, Err: describe("create directory", name, err)}
	}
	// The new directory is checked on its handle like any other: a
	// directory swapped in between is refused, and left in place.
	handle, err := openDirectory(name, path)
	if err != nil {
		return err
	}
	s.pins[name] = handle
	return nil
}

// Hold opens the existing directory name without following it and keeps
// it pinned until Release or Close.
func (s *Session) Hold(name string) error {
	if err := s.requireParent(name); err != nil {
		return err
	}
	if _, held := s.pins[name]; held {
		return nil
	}
	handle, err := openDirectory(name, s.absolute(name))
	if err != nil {
		return err
	}
	s.pins[name] = handle
	return nil
}

// Release closes the pins of dir and of every directory held below it, so
// that dir can be renamed.
func (s *Session) Release(dir string) error {
	var errs []error
	for name, handle := range s.pins {
		if below(name, dir) {
			errs = append(errs, closeHandle(name, handle))
			delete(s.pins, name)
		}
	}
	return errors.Join(errs...)
}

// NewFile creates the regular file name with CREATE_NEW: it fails with
// fs.ErrExist if anything has that name. Nothing else may open the file
// while it is written. Close flushes it, then gives it modTime unless that
// is zero; the caller treats any error of Write or Close as a failed file,
// which stays in place.
func (s *Session) NewFile(name string, modTime time.Time) (io.WriteCloser, error) {
	if err := s.requireParent(name); err != nil {
		return nil, err
	}
	if name == "." {
		return nil, &fs.PathError{Op: "create file", Path: name, Err: fs.ErrInvalid}
	}
	path := s.absolute(name)
	pointer, err := windows.UTF16PtrFromString(longPath(path))
	if err != nil {
		return nil, &fs.PathError{Op: "create file", Path: name, Err: err}
	}
	handle, err := windows.CreateFile(pointer, windows.GENERIC_WRITE|windows.FILE_READ_ATTRIBUTES, 0, nil,
		windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, &fs.PathError{Op: "create file", Path: name, Err: describe("create file", name, err)}
	}
	if err := checkHandle(name, handle, false); err != nil {
		_ = windows.CloseHandle(handle)
		return nil, err
	}
	return &file{name: name, handle: handle, file: os.NewFile(uintptr(handle), path), modTime: modTime}, nil
}

type file struct {
	name    string
	handle  windows.Handle
	file    *os.File
	modTime time.Time
	closed  bool
}

func (f *file) Write(p []byte) (int, error) {
	if f.closed {
		return 0, &fs.PathError{Op: "write", Path: f.name, Err: fs.ErrClosed}
	}
	return f.file.Write(p)
}

// Close dates, flushes and closes the file. The date is set before the
// flush so that it is made durable with the content.
func (f *file) Close() error {
	if f.closed {
		return &fs.PathError{Op: "close", Path: f.name, Err: fs.ErrClosed}
	}
	f.closed = true
	var errs []error
	if !f.modTime.IsZero() {
		modified := windows.NsecToFiletime(f.modTime.UnixNano())
		if err := windows.SetFileTime(f.handle, nil, nil, &modified); err != nil {
			errs = append(errs, &fs.PathError{Op: "set date", Path: f.name, Err: err})
		}
	}
	if err := windows.FlushFileBuffers(f.handle); err != nil {
		errs = append(errs, &fs.PathError{Op: "flush", Path: f.name, Err: err})
	}
	if err := f.file.Close(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// Move renames from to to with MoveFileEx and flags 0: it fails if to
// exists and never copies across volumes. Both parents must be held, and
// neither from nor anything below it: a pin would block the rename.
// Sharing and lock violations are reported as *BusyError and never retried
// here.
func (s *Session) Move(from, to string) error {
	for _, name := range []string{from, to} {
		if err := s.requireParent(name); err != nil {
			return err
		}
		if name == "." {
			return &fs.PathError{Op: "move", Path: name, Err: fs.ErrInvalid}
		}
	}
	for name := range s.pins {
		if below(name, from) {
			return &fs.PathError{Op: "move", Path: from, Err: fmt.Errorf("%q is held by the session; release it first", name)}
		}
	}
	source, err := windows.UTF16PtrFromString(longPath(s.absolute(from)))
	if err != nil {
		return &fs.PathError{Op: "move", Path: from, Err: err}
	}
	target, err := windows.UTF16PtrFromString(longPath(s.absolute(to)))
	if err != nil {
		return &fs.PathError{Op: "move", Path: to, Err: err}
	}
	if err := windows.MoveFileEx(source, target, 0); err != nil {
		return &fs.PathError{Op: "move", Path: from + " to " + to, Err: describe("move", from, err)}
	}
	return nil
}

// Identity returns the volume serial number and file index of name, opened
// without following it and closed at once. The handle shares reading,
// writing and deletion, so it never blocks a rename. Links, junctions,
// reparse points and placeholders are refused; a missing name gives
// fs.ErrNotExist.
func (s *Session) Identity(name string) (backup.FileID, error) {
	if err := s.requireParent(name); err != nil {
		return backup.FileID{}, err
	}
	pointer, err := windows.UTF16PtrFromString(longPath(s.absolute(name)))
	if err != nil {
		return backup.FileID{}, &fs.PathError{Op: "identity", Path: name, Err: err}
	}
	handle, err := windows.CreateFile(pointer, windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, noFollow, 0)
	if err != nil {
		return backup.FileID{}, &fs.PathError{Op: "identity", Path: name, Err: describe("identity", name, err)}
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	var data windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &data); err != nil {
		return backup.FileID{}, &fs.PathError{Op: "identity", Path: name, Err: err}
	}
	if err := refuseLinks(name, data.FileAttributes); err != nil {
		return backup.FileID{}, err
	}
	return backup.FileID{Volume: data.VolumeSerialNumber, Index: uint64(data.FileIndexHigh)<<32 | uint64(data.FileIndexLow)}, nil
}

// Lock creates the regular file name, or opens it if it exists, without
// any sharing, and keeps it open until Close. A second session fails with
// a *BusyError; the system releases the handle if the process dies. The
// lock file is never written, truncated or deleted.
func (s *Session) Lock(name string) error {
	if s.lock != windows.InvalidHandle {
		return errors.New("the session already holds a lock")
	}
	if err := s.requireParent(name); err != nil {
		return err
	}
	pointer, err := windows.UTF16PtrFromString(longPath(s.absolute(name)))
	if err != nil {
		return &fs.PathError{Op: "lock", Path: name, Err: err}
	}
	handle, err := windows.CreateFile(pointer, windows.GENERIC_READ, 0, nil, windows.CREATE_NEW,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if errors.Is(err, fs.ErrExist) {
		handle, err = windows.CreateFile(pointer, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, noFollow, 0)
	}
	if err != nil {
		return &fs.PathError{Op: "lock", Path: name, Err: describe("lock", name, err)}
	}
	if err := checkHandle(name, handle, false); err != nil {
		_ = windows.CloseHandle(handle)
		return err
	}
	s.lock = handle
	return nil
}

// Close releases the lock and every pin.
func (s *Session) Close() error {
	var errs []error
	if s.lock != windows.InvalidHandle {
		errs = append(errs, closeHandle("lock", s.lock))
		s.lock = windows.InvalidHandle
	}
	errs = append(errs, s.Release("."))
	for _, handle := range s.ancestors {
		errs = append(errs, closeHandle(s.base, handle))
	}
	s.ancestors = nil
	return errors.Join(errs...)
}

// Describe reports the file system and drive type of the volume holding
// path, and the bytes the calling user may still write in path, which must
// be an existing directory. It reads and writes nothing else.
func Describe(path string) (backup.Volume, error) {
	volume := filepath.VolumeName(path)
	if len(volume) != 2 || volume[1] != ':' || !filepath.IsAbs(path) {
		return backup.Volume{}, fmt.Errorf("volume of %q: an absolute path below a drive letter is required", path)
	}
	root, err := windows.UTF16PtrFromString(volume + `\`)
	if err != nil {
		return backup.Volume{}, err
	}
	const nameSize = windows.MAX_PATH + 1
	var name [nameSize]uint16
	if err := windows.GetVolumeInformation(root, nil, 0, nil, nil, nil, &name[0], nameSize); err != nil {
		return backup.Volume{}, fmt.Errorf("volume %s: %w", volume, describe("volume", volume, err))
	}
	directory, err := windows.UTF16PtrFromString(longPath(path))
	if err != nil {
		return backup.Volume{}, err
	}
	var available, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(directory, &available, &total, &free); err != nil {
		return backup.Volume{}, fmt.Errorf("free space of %s: %w", path, describe("volume", path, err))
	}
	return backup.Volume{FileSystem: windows.UTF16ToString(name[:]), Drive: drive(windows.GetDriveType(root)), Available: available}, nil
}

func drive(kind uint32) backup.Drive {
	switch kind {
	case windows.DRIVE_REMOVABLE:
		return backup.DriveRemovable
	case windows.DRIVE_FIXED:
		return backup.DriveFixed
	case windows.DRIVE_REMOTE:
		return backup.DriveRemote
	case windows.DRIVE_CDROM:
		return backup.DriveOptical
	case windows.DRIVE_RAMDISK:
		return backup.DriveRAM
	}
	return backup.DriveUnknown
}

// openDirectory opens path as a pin: for reading, without following it,
// sharing reading and writing but not deletion, so that the directory
// cannot be renamed or deleted while held. name is used in errors.
func openDirectory(name, path string) (windows.Handle, error) {
	pointer, err := windows.UTF16PtrFromString(longPath(path))
	if err != nil {
		return windows.InvalidHandle, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	handle, err := windows.CreateFile(pointer, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_EXISTING, noFollow, 0)
	if err != nil {
		return windows.InvalidHandle, &fs.PathError{Op: "open", Path: name, Err: describe("open", name, err)}
	}
	if err := checkHandle(name, handle, true); err != nil {
		_ = windows.CloseHandle(handle)
		return windows.InvalidHandle, err
	}
	return handle, nil
}

// checkHandle refuses a handle that denotes a reparse point, a placeholder
// or an object of the other kind.
func checkHandle(name string, handle windows.Handle, directory bool) error {
	var data windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &data); err != nil {
		return &fs.PathError{Op: "inspect handle", Path: name, Err: err}
	}
	if err := refuseLinks(name, data.FileAttributes); err != nil {
		return err
	}
	isDirectory := data.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0
	switch {
	case directory && !isDirectory:
		return &fs.PathError{Op: "open", Path: name, Err: errors.New("not a directory")}
	case !directory && isDirectory:
		return &fs.PathError{Op: "open", Path: name, Err: errors.New("not a regular file")}
	}
	return nil
}

func refuseLinks(name string, attributes uint32) error {
	switch {
	case attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0:
		return &NotFollowedError{Name: name, Reason: "reparse point (junction, symbolic link or cloud file)"}
	case attributes&placeholderAttributes != 0:
		return &NotFollowedError{Name: name, Reason: "offline or cloud placeholder"}
	}
	return nil
}

// describe marks sharing and lock violations as busy, and maps unavailable
// volumes to fs.ErrNotExist. Other errors keep their system error, which
// matches fs.ErrExist, fs.ErrNotExist or fs.ErrPermission where it applies.
func describe(op, name string, err error) error {
	var errno windows.Errno
	if errors.As(err, &errno) {
		switch errno {
		case windows.ERROR_SHARING_VIOLATION, windows.ERROR_LOCK_VIOLATION:
			return &BusyError{Op: op, Name: name, Err: err}
		case windows.ERROR_INVALID_DRIVE, windows.ERROR_NOT_READY:
			return fmt.Errorf("volume not available: %w", fs.ErrNotExist)
		}
	}
	return err
}

func closeHandle(name string, handle windows.Handle) error {
	if err := windows.CloseHandle(handle); err != nil {
		return fmt.Errorf("release %s: %w", name, err)
	}
	return nil
}

// longPath prefixes \\?\ so that the clean absolute path is used
// literally, beyond MAX_PATH and without Win32 normalisation.
func longPath(path string) string { return `\\?\` + path }
