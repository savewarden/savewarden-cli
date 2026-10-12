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
)

const readBuffer = 1 << 20

// walk inventories one directory of a source. name is relative to the scope
// anchor, rel to the source. It returns an error only on cancellation.
func (r *inspection) walk(source *SourceReport, name, rel string) error {
	if err := r.ctx.Err(); err != nil {
		return interrupted(err)
	}
	entries, err := r.scope.ReadDir(name)
	if err != nil {
		r.add(Blocking, codeFor(err), source.Source.ID, display(rel), "cannot list directory: "+err.Error())
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	r.caseCollisions(source.Source.ID, rel, names)
	for _, entryName := range names {
		if err := r.ctx.Err(); err != nil {
			return interrupted(err)
		}
		childName, childRel := name+"/"+entryName, join(rel, entryName)
		info, err := r.scope.Lstat(childName)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			r.add(Blocking, CodeChangedDuringRead, source.Source.ID, childRel, "disappeared during inspection")
		case err != nil:
			r.add(Blocking, codeFor(err), source.Source.ID, childRel, err.Error())
		case isLink(info.Mode()):
			r.add(Blocking, CodeLinkNotFollowed, source.Source.ID, childRel, describeLink(info.Mode())+"; not followed")
		case info.IsDir():
			if err := r.walk(source, childName, childRel); err != nil {
				return err
			}
		case info.Mode().IsRegular():
			if err := r.hash(source, childName, childRel, info); err != nil {
				return err
			}
		default:
			r.add(Blocking, CodeSpecialFile, source.Source.ID, childRel, "special file of type "+info.Mode().Type().String()+"; not read")
		}
	}
	return nil
}

// hash reads one regular file in full and records it only if its identity,
// size and modification time held before, during and after the read.
func (r *inspection) hash(source *SourceReport, name, rel string, before fs.FileInfo) error {
	id := source.Source.ID
	file, err := r.scope.OpenRegular(name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		r.add(Blocking, CodeChangedDuringRead, id, rel, "disappeared before reading")
		return nil
	case err != nil:
		r.add(Blocking, codeFor(err), id, rel, err.Error())
		return nil
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil {
		r.add(Blocking, CodeUnreadable, id, rel, err.Error())
		return nil
	}
	if !r.scope.SameFile(before, opened) {
		r.add(Blocking, CodeReplacedBeforeRead, id, rel, "replaced by another file before reading")
		return nil
	}
	if changed(before, opened) {
		r.add(Blocking, CodeChangedDuringRead, id, rel, "modified before reading")
		return nil
	}
	digest := sha256.New()
	read, err := io.CopyBuffer(digest, contextReader{r.ctx, file}, make([]byte, readBuffer))
	if err != nil {
		if ctxErr := r.ctx.Err(); ctxErr != nil {
			return interrupted(ctxErr)
		}
		r.add(Blocking, CodeUnreadable, id, rel, err.Error())
		return nil
	}
	after, err := file.Stat()
	if err != nil {
		r.add(Blocking, CodeUnreadable, id, rel, err.Error())
		return nil
	}
	if changed(before, after) || read != before.Size() {
		r.add(Blocking, CodeChangedDuringRead, id, rel, "modified while reading")
		return nil
	}
	entry := Entry{Path: rel, Size: read, ModTime: before.ModTime().UTC()}
	copy(entry.SHA256[:], digest.Sum(nil))
	source.Entries = append(source.Entries, entry)
	source.Bytes += read
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
			r.add(Blocking, CodeCaseCollision, id, join(rel, name), fmt.Sprintf("differs from %q only by case", previous))
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
