package inspect

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

const readBuffer = 1 << 20

// tree accumulates the inventory of one directory.
type tree struct {
	entries     []backup.Entry
	directories []string
	encrypted   []string
	bytes       int64
}

func (t *tree) sort() {
	sort.Slice(t.entries, func(a, b int) bool { return t.entries[a].Path < t.entries[b].Path })
	sort.Strings(t.directories)
	sort.Strings(t.encrypted)
}

// walk inventories one directory into t. name is relative to the scope
// anchor ("." for the anchor itself), rel to the inventoried directory;
// label is the source id given to findings. It returns an error only on
// cancellation.
func (r *inspection) walk(t *tree, label, name, rel string) error {
	if err := r.ctx.Err(); err != nil {
		return interrupted(err)
	}
	entries, err := r.scope.ReadDir(name)
	if err != nil {
		r.add(backup.Blocking, codeFor(err), label, display(rel), "cannot list directory: "+err.Error())
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	r.caseCollisions(label, rel, names)
	for _, entryName := range names {
		if err := r.ctx.Err(); err != nil {
			return interrupted(err)
		}
		childName, childRel := entryName, join(rel, entryName)
		if name != "." {
			childName = name + "/" + entryName
		}
		info, err := r.scope.Lstat(childName)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			r.add(backup.Blocking, CodeChangedDuringRead, label, childRel, "disappeared during inspection")
		case err != nil:
			r.add(backup.Blocking, codeFor(err), label, childRel, err.Error())
		case isLink(info.Mode()):
			r.add(backup.Blocking, CodeLinkNotFollowed, label, childRel, describeLink(info.Mode())+"; not followed")
		case info.IsDir():
			t.directories = append(t.directories, childRel)
			if err := r.walk(t, label, childName, childRel); err != nil {
				return err
			}
		case info.Mode().IsRegular():
			if encrypted(info) {
				t.encrypted = append(t.encrypted, childRel)
			}
			if err := r.hash(t, label, childName, childRel, info); err != nil {
				return err
			}
		default:
			r.add(backup.Blocking, CodeSpecialFile, label, childRel, "special file of type "+info.Mode().Type().String()+"; not read")
		}
	}
	return nil
}

// encrypted matches adapter descriptions of EFS-encrypted files, without
// importing them.
func encrypted(info fs.FileInfo) bool {
	e, ok := info.(interface{ Encrypted() bool })
	return ok && e.Encrypted()
}

// hash reads one regular file in full and records it only if its identity,
// size and modification time held before, during and after the read.
func (r *inspection) hash(t *tree, label, name, rel string, before fs.FileInfo) error {
	file, err := r.scope.OpenRegular(name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		r.add(backup.Blocking, CodeChangedDuringRead, label, rel, "disappeared before reading")
		return nil
	case err != nil:
		r.add(backup.Blocking, codeFor(err), label, rel, err.Error())
		return nil
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil {
		r.add(backup.Blocking, CodeUnreadable, label, rel, err.Error())
		return nil
	}
	if !r.scope.SameFile(before, opened) {
		r.add(backup.Blocking, CodeReplacedBeforeRead, label, rel, "replaced by another file before reading")
		return nil
	}
	if changed(before, opened) {
		r.add(backup.Blocking, CodeChangedDuringRead, label, rel, "modified before reading")
		return nil
	}
	digest := sha256.New()
	read, err := io.CopyBuffer(digest, contextReader{r.ctx, file}, make([]byte, readBuffer))
	if err != nil {
		if ctxErr := r.ctx.Err(); ctxErr != nil {
			return interrupted(ctxErr)
		}
		r.add(backup.Blocking, CodeUnreadable, label, rel, err.Error())
		return nil
	}
	after, err := file.Stat()
	if err != nil {
		r.add(backup.Blocking, CodeUnreadable, label, rel, err.Error())
		return nil
	}
	if changed(before, after) || read != before.Size() {
		r.add(backup.Blocking, CodeChangedDuringRead, label, rel, "modified while reading")
		return nil
	}
	entry := backup.Entry{Path: rel, Size: read, ModTime: before.ModTime().UTC()}
	copy(entry.SHA256[:], digest.Sum(nil))
	t.entries = append(t.entries, entry)
	t.bytes += read
	return nil
}

func changed(before, now fs.FileInfo) bool {
	return now.Size() != before.Size() || !now.ModTime().Equal(before.ModTime())
}

// caseCollisions reports names that a case-insensitive volume would merge.
func (r *inspection) caseCollisions(id, rel string, names []string) {
	first := map[string]string{}
	for _, name := range names {
		key := strings.ToUpper(name)
		if previous, ok := first[key]; ok {
			r.add(backup.Blocking, CodeCaseCollision, id, join(rel, name), fmt.Sprintf("differs from %q only by case", previous))
			continue
		}
		first[key] = name
	}
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (c contextReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.reader.Read(p)
}

func interrupted(err error) error { return fmt.Errorf("inspection interrupted: %w", err) }

func join(rel, name string) string {
	if rel == "" {
		return name
	}
	return rel + "/" + name
}

// display names the source root as "." in findings about it.
func display(rel string) string {
	if rel == "" {
		return "."
	}
	return rel
}
