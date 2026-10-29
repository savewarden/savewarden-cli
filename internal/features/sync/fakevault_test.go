package sync_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/features/sync"
)

// errCrash stands for the death of the process: once returned, every later
// call of the session fails too.
var errCrash = errors.New("simulated crash")

type busyError struct{}

func (busyError) Error() string { return "in use by another process" }
func (busyError) Busy() bool    { return true }

// fakeVaults writes into a fakeFS with the semantics of platform/vault:
// exclusive creations, held parents, renames that never replace.
type fakeVaults struct {
	f     *fakeFS
	locks map[string]bool
	// calls lists every mutating call in order: CreateDir, NewFile, Write,
	// Close, Move, Lock.
	calls []string
	// crashBefore or crashAfter, when positive, kill the process at that
	// mutating call, before or after performing it.
	crashBefore, crashAfter int
	dead                    bool
	// busy counts, per source name, the busy errors Move returns first.
	busy map[string]int
	// failWrite, if set, fails a write or a close.
	failWrite func(call string) error
}

func newFakeVaults(f *fakeFS) *fakeVaults {
	return &fakeVaults{f: f, locks: map[string]bool{}, busy: map[string]int{}}
}

func (v *fakeVaults) Open(path string) (sync.Vault, error) {
	if v.dead {
		return nil, errCrash
	}
	parts := strings.Split(path[3:], `\`)
	n := v.f.volumes[path[:3]]
	for i, part := range parts {
		child, ok := n.children[part]
		switch {
		case !ok && i == len(parts)-1:
			return &fakeVault{v: v, base: path, held: map[string]bool{}}, nil
		case !ok:
			return nil, fs.ErrNotExist
		case isLink(child.mode):
			return nil, &notFollowedError{part}
		case !child.mode.IsDir():
			return nil, errors.New(part + ": not a directory")
		}
		n = child
	}
	return &fakeVault{v: v, base: path, held: map[string]bool{".": true}}, nil
}

// call counts one mutating call and applies the crash plan; perform runs it.
func (v *fakeVaults) call(name string, perform func() error) error {
	if v.dead {
		return errCrash
	}
	v.calls = append(v.calls, name)
	k := len(v.calls)
	if k == v.crashBefore {
		v.dead = true
		return errCrash
	}
	if err := perform(); err != nil {
		return err
	}
	if k == v.crashAfter {
		v.dead = true
		return errCrash
	}
	return nil
}

type fakeVault struct {
	v    *fakeVaults
	base string
	held map[string]bool
	lock string
}

func (s *fakeVault) path(name string) string {
	if name == "." {
		return s.base
	}
	return s.base + `\` + strings.ReplaceAll(name, "/", `\`)
}

func parentName(name string) string {
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		return name[:i]
	}
	return "."
}

func (s *fakeVault) requireParent(name string) error {
	if s.v.dead {
		return errCrash
	}
	if name == "." || s.held[parentName(name)] {
		return nil
	}
	return fmt.Errorf("%s: parent not held", name)
}

func (s *fakeVault) CreateDir(name string) error {
	if err := s.requireParent(name); err != nil {
		return err
	}
	return s.v.call("CreateDir "+name, func() error {
		if s.v.f.get(s.path(name)) != nil {
			return fs.ErrExist
		}
		s.v.f.put(s.path(name), s.v.f.dir())
		s.held[name] = true
		return nil
	})
}

func (s *fakeVault) Hold(name string) error {
	if err := s.requireParent(name); err != nil {
		return err
	}
	n := s.v.f.get(s.path(name))
	switch {
	case n == nil:
		return fs.ErrNotExist
	case isLink(n.mode):
		return &notFollowedError{name}
	case !n.mode.IsDir():
		return errors.New(name + ": not a directory")
	}
	s.held[name] = true
	return nil
}

func (s *fakeVault) Release(dir string) error {
	for name := range s.held {
		if dir == "." || name == dir || strings.HasPrefix(name, dir+"/") {
			delete(s.held, name)
		}
	}
	return nil
}

func (s *fakeVault) NewFile(name string, modTime time.Time) (io.WriteCloser, error) {
	if err := s.requireParent(name); err != nil {
		return nil, err
	}
	var created *node
	err := s.v.call("NewFile "+name, func() error {
		if s.v.f.get(s.path(name)) != nil {
			return fs.ErrExist
		}
		created = s.v.f.put(s.path(name), s.v.f.file(""))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &fakeWriter{s: s, name: name, n: created, modTime: modTime}, nil
}

type fakeWriter struct {
	s       *fakeVault
	name    string
	n       *node
	modTime time.Time
	closed  bool
}

func (w *fakeWriter) Write(p []byte) (int, error) {
	err := w.s.v.call("Write "+w.name, func() error {
		if w.s.v.failWrite != nil {
			if err := w.s.v.failWrite("Write " + w.name); err != nil {
				return err
			}
		}
		w.n.data = append(w.n.data, p...)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *fakeWriter) Close() error {
	if w.closed {
		return fs.ErrClosed
	}
	w.closed = true
	return w.s.v.call("Close "+w.name, func() error {
		if w.s.v.failWrite != nil {
			if err := w.s.v.failWrite("Close " + w.name); err != nil {
				return err
			}
		}
		if !w.modTime.IsZero() {
			w.n.modTime = w.modTime
		}
		return nil
	})
}

func (s *fakeVault) Move(from, to string) error {
	for _, name := range []string{from, to} {
		if err := s.requireParent(name); err != nil {
			return err
		}
	}
	for name := range s.held {
		if name == from || strings.HasPrefix(name, from+"/") {
			return fmt.Errorf("%s is held; release it first", from)
		}
	}
	return s.v.call("Move "+from+" "+to, func() error {
		if s.v.busy[from] > 0 {
			s.v.busy[from]--
			return busyError{}
		}
		fromParent, fromName := s.v.f.parentOf(s.path(from), false)
		if fromParent == nil || fromParent.children[fromName] == nil {
			return fs.ErrNotExist
		}
		if s.v.f.get(s.path(to)) != nil {
			return fs.ErrExist
		}
		n := fromParent.children[fromName]
		delete(fromParent.children, fromName)
		s.v.f.put(s.path(to), n)
		return nil
	})
}

func (s *fakeVault) Identity(name string) (backup.FileID, error) {
	if err := s.requireParent(name); err != nil {
		return backup.FileID{}, err
	}
	n := s.v.f.get(s.path(name))
	switch {
	case n == nil:
		return backup.FileID{}, fs.ErrNotExist
	case isLink(n.mode):
		return backup.FileID{}, &notFollowedError{name}
	}
	return backup.FileID{Volume: 0x1a2b3c4d, Index: uint64(n.id)}, nil //nolint:gosec // G115: node ids count up from 1.
}

func (s *fakeVault) Lock(name string) error {
	if err := s.requireParent(name); err != nil {
		return err
	}
	return s.v.call("Lock "+name, func() error {
		path := s.path(name)
		if s.v.locks[path] {
			return busyError{}
		}
		switch n := s.v.f.get(path); {
		case n == nil:
			s.v.f.put(path, s.v.f.file(""))
		case !n.mode.IsRegular():
			return errors.New(name + ": not a regular file")
		}
		s.v.locks[path], s.lock = true, path
		return nil
	})
}

func (s *fakeVault) Close() error {
	if s.lock != "" {
		delete(s.v.locks, s.lock)
		s.lock = ""
	}
	s.held = map[string]bool{}
	return nil
}

// fakeClock advances only when slept on.
type fakeClock struct {
	now   time.Time
	slept []time.Duration
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.slept = append(c.slept, d)
	c.now = c.now.Add(d)
	return nil
}

func writer(v *fakeVaults, clock *fakeClock) sync.Writer {
	return sync.Writer{Vaults: v, Clock: clock, Random: bytes.NewReader([]byte{0x1a, 0x2b, 0x3c, 0x4d}), ToolVersion: "dev"}
}
