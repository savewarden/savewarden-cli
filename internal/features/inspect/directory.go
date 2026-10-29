package inspect

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

// Directory inventories one directory tree with the same no-follow walk as
// Inspect: every component from the volume root down to the directory is
// checked, nothing is followed, every regular file is hashed and checked
// around its read. volume is a root such as D:\ and segments lead to the
// directory; no segment inventories the volume root itself.
//
// A missing volume or component yields StatusMissing, a link or file on the
// way StatusRefused with one blocking finding. Only an unusable scope and
// cancellation return an error, never a partial inventory. Findings carry
// no source id. The sync feature consumes this method through a port of
// its own (ADR 0004).
func (s *Service) Directory(ctx context.Context, volume string, segments []string) (backup.Directory, error) {
	if err := ctx.Err(); err != nil {
		return backup.Directory{}, interrupted(err)
	}
	scope, err := s.files.OpenScope(volume)
	if errors.Is(err, fs.ErrNotExist) {
		return backup.Directory{Status: backup.StatusMissing, Missing: volume}, nil
	}
	if err != nil {
		return backup.Directory{}, fmt.Errorf("open volume %s: %w", volume, err)
	}
	defer func() { _ = scope.Close() }()
	run := &inspection{ctx: ctx, scope: scope}
	if _, stopped := run.resolve(volume, segments); stopped != nil {
		if stopped.status == StatusMissing {
			return backup.Directory{Status: backup.StatusMissing, Missing: stopped.path}, nil
		}
		finding := backup.Finding{Severity: backup.Blocking, Code: stopped.code, Path: stopped.path, Detail: stopped.detail}
		return backup.Directory{Status: backup.StatusRefused, Findings: []backup.Finding{finding}}, nil
	}
	name := strings.Join(segments, "/")
	if name == "" {
		name = "."
	}
	tree := &tree{}
	if err := run.walk(tree, "", name, ""); err != nil {
		return backup.Directory{}, err
	}
	tree.sort()
	sort.SliceStable(run.findings, func(a, b int) bool {
		x, y := run.findings[a], run.findings[b]
		if x.Path != y.Path {
			return x.Path < y.Path
		}
		return x.Code < y.Code
	})
	return backup.Directory{
		Status:      backup.StatusInventoried,
		Entries:     tree.entries,
		Directories: tree.directories,
		Encrypted:   tree.encrypted,
		Bytes:       tree.bytes,
		Findings:    run.findings,
	}, nil
}
