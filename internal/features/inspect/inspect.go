// Package inspect inventories the sources declared by a manifest, read
// only. It never follows links or reparse points, never writes and never
// executes anything; those guarantees rely on the Scope contract and are
// tested against fakes on every OS and against real files on Windows.
//
// The inventory is best effort, not an atomic snapshot: size, modification
// time and identity are checked around each read, so most concurrent
// changes are reported, but a change preserving all three can go unseen.
package inspect

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/entities/game"
)

// Request selects the manifest to inspect.
type Request struct {
	ManifestPath string
}

// Service runs inspections through injected adapters.
type Service struct {
	manifests ManifestReader
	files     FileSystem
}

// New returns a Service using the given adapters.
func New(manifests ManifestReader, files FileSystem) *Service {
	return &Service{manifests: manifests, files: files}
}

// Inspect reads the manifest and inventories every source. An error means
// no report: invalid manifest, unusable volume, overlapping sources or
// cancellation. Problems with individual files become findings instead.
func (s *Service) Inspect(ctx context.Context, request Request) (Report, error) {
	manifest, err := s.manifests.Read(request.ManifestPath)
	if err != nil {
		return Report{}, err
	}
	report := Report{
		ManifestPath: request.ManifestPath,
		Game:         manifest.Game,
		Installation: manifest.Installation.Path,
	}
	volume, _, err := manifest.SourceSegments(0)
	if err != nil {
		return Report{}, err
	}
	scope, err := s.files.OpenScope(volume)
	volumeMissing := errors.Is(err, fs.ErrNotExist)
	if err != nil && !volumeMissing {
		return Report{}, fmt.Errorf("open volume %s: %w", volume, err)
	}
	run := &inspection{ctx: ctx, scope: scope}
	if scope != nil {
		defer func() { _ = scope.Close() }()
	}

	chains := make([][]fs.FileInfo, len(manifest.Sources))
	for i, source := range manifest.Sources {
		_, segments, err := manifest.SourceSegments(i)
		if err != nil {
			return Report{}, err
		}
		result := SourceReport{Source: source, Path: windowsPath(volume, segments), Status: StatusInventoried}
		if volumeMissing {
			result.Status = StatusMissing
			run.add(missingSeverity(source), CodeSourceMissing, source.ID, volume, "volume "+volume+" is not available")
		} else if chain, stopped := run.resolve(volume, segments); stopped != nil {
			result.Status = stopped.status
			severity := backup.Blocking
			if stopped.status == StatusMissing {
				severity = missingSeverity(source)
			}
			run.add(severity, stopped.code, source.ID, stopped.path, stopped.detail)
		} else {
			chains[i] = chain
		}
		report.Sources = append(report.Sources, result)
	}
	if err := checkOverlaps(scope, manifest.Sources, chains); err != nil {
		return Report{}, err
	}
	for i := range report.Sources {
		if report.Sources[i].Status != StatusInventoried {
			continue
		}
		_, segments, _ := manifest.SourceSegments(i)
		tree := &tree{}
		if err := run.walk(tree, report.Sources[i].Source.ID, strings.Join(segments, "/"), ""); err != nil {
			return Report{}, err
		}
		tree.sort()
		report.Sources[i].Entries, report.Sources[i].Bytes = tree.entries, tree.bytes
	}
	order := map[string]int{}
	for i, source := range manifest.Sources {
		order[source.ID] = i
	}
	report.Findings = run.findings
	sort.SliceStable(report.Findings, func(a, b int) bool {
		x, y := report.Findings[a], report.Findings[b]
		if order[x.Source] != order[y.Source] {
			return order[x.Source] < order[y.Source]
		}
		if x.Path != y.Path {
			return x.Path < y.Path
		}
		return x.Code < y.Code
	})
	return report, nil
}

type inspection struct {
	ctx      context.Context
	scope    Scope
	findings []backup.Finding
}

func (r *inspection) add(severity backup.Severity, code backup.Code, source, path, detail string) {
	r.findings = append(r.findings, backup.Finding{Severity: severity, Code: code, Source: source, Path: path, Detail: detail})
}

func missingSeverity(source game.Source) backup.Severity {
	if source.Required {
		return backup.Blocking
	}
	return backup.Info
}

// stop explains why resolve did not reach the directory.
type stop struct {
	status SourceStatus // StatusMissing or StatusRefused
	code   backup.Code
	path   string // Windows path of the component in cause
	detail string
}

// resolve checks every component from the volume root down to the
// directory without following anything, and returns their descriptions.
func (r *inspection) resolve(volume string, segments []string) ([]fs.FileInfo, *stop) {
	chain := make([]fs.FileInfo, 0, len(segments))
	for k := 1; k <= len(segments); k++ {
		display := windowsPath(volume, segments[:k])
		info, err := r.scope.Lstat(strings.Join(segments[:k], "/"))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return nil, &stop{StatusMissing, CodeSourceMissing, display, "does not exist"}
		case err != nil:
			return nil, &stop{StatusRefused, codeFor(err), display, err.Error()}
		case isLink(info.Mode()):
			return nil, &stop{StatusRefused, CodeLinkNotFollowed, display, describeLink(info.Mode()) + " on the way to the source; not followed"}
		case !info.IsDir():
			return nil, &stop{StatusRefused, CodeSourceNotDirectory, display, "is not a directory"}
		}
		chain = append(chain, info)
	}
	return chain, nil
}

// checkOverlaps compares resolved directories by identity, which also
// catches 8.3 short names that the manifest grammar cannot see.
func checkOverlaps(scope Scope, sources []game.Source, chains [][]fs.FileInfo) error {
	for i := range sources {
		for j := range sources {
			if i == j || len(chains[i]) == 0 || len(chains[j]) == 0 {
				continue
			}
			target := chains[i][len(chains[i])-1]
			for _, ancestor := range chains[j] {
				if scope.SameFile(target, ancestor) {
					return fmt.Errorf("sources %q and %q resolve to overlapping directories", sources[i].ID, sources[j].ID)
				}
			}
		}
	}
	return nil
}

func windowsPath(volume string, segments []string) string {
	return volume + strings.Join(segments, `\`)
}

func isLink(mode fs.FileMode) bool { return mode&(fs.ModeSymlink|fs.ModeIrregular) != 0 }

func describeLink(mode fs.FileMode) string {
	if mode&fs.ModeSymlink != 0 {
		return "symbolic link"
	}
	return "junction or reparse point"
}

// notFollowed matches adapter errors refusing a link, without importing them.
func notFollowed(err error) bool {
	var refusal interface{ NotFollowed() bool }
	return errors.As(err, &refusal) && refusal.NotFollowed()
}

func codeFor(err error) backup.Code {
	if notFollowed(err) {
		return CodeLinkNotFollowed
	}
	return CodeUnreadable
}
