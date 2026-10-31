// Package sync plans and performs every publication into the vault of a
// game: the copy of a manifest's sources into current, and the snapshots
// of current. A plan reads the manifest, the vault and, for current, the
// sources, and writes nothing; --apply and --recover --apply write only
// into the vault, through the Vaults port, by exclusive creations and
// renames that replace nothing. Publication and recovery follow ADR 0006;
// the contracts of the commands are ADR 0007 and ADR 0008.
//
// The plan never follows links: sources are inventoried by the no-follow
// walk of inspect, received through the Inventories port, and the vault is
// read through a no-follow Scope.
package sync

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/entities/game"
)

// Request selects the manifest and the vault of a plan, and the explicit
// policies of plan B2 §4.4 (D5, V1).
type Request struct {
	ManifestPath string
	VaultPath    string
	// AllowRemovals accepts files removed from a source that is present,
	// and nothing else.
	AllowRemovals bool
	// AcceptMissing names, one by one, optional sources physically absent
	// whose files the receipt of current holds. Naming any other source is
	// an invalid invocation.
	AcceptMissing []string
}

// InvocationError refuses flags that do not apply to the plan: exit code
// 1, nothing written.
type InvocationError struct{ Detail string }

func (e *InvocationError) Error() string { return "invalid invocation: " + e.Detail }

// Service plans synchronisations through injected adapters, all of them
// read-only. Writing needs a Writer, given to --apply and --recover only.
type Service struct {
	manifests   ManifestReader
	inventories Inventories
	files       Files
	volumes     Volumes
}

// New returns a Service using the given adapters.
func New(manifests ManifestReader, inventories Inventories, files Files, volumes Volumes) *Service {
	return &Service{manifests: manifests, inventories: inventories, files: files, volumes: volumes}
}

// Plan computes what a synchronisation would copy, without writing
// anything. An error means no plan: invalid manifest or vault, vault
// overlapping the installation, unusable volume, read error or
// cancellation. Problems with sources or with the current generation
// become findings; states of the vault that need a human become
// interventions, and then no source is planned.
func (s *Service) Plan(ctx context.Context, request Request) (Plan, error) {
	manifest, err := s.manifests.Read(request.ManifestPath)
	if err != nil {
		return Plan{}, err
	}
	vaultVolume, vaultSegments, err := game.ParseWindowsAbsolute(request.VaultPath)
	if err != nil {
		return Plan{}, fmt.Errorf("vault %s: %w", request.VaultPath, err)
	}
	installVolume, installSegments, err := game.ParseWindowsAbsolute(manifest.Installation.Path)
	if err != nil {
		return Plan{}, err
	}
	if err := distinctByName(request.VaultPath, vaultVolume, vaultSegments, installVolume, installSegments); err != nil {
		return Plan{}, err
	}
	gameSegments := append(append([]string{}, vaultSegments...), gamesDir, manifest.Game.ID)
	plan := Plan{
		ManifestPath:  request.ManifestPath,
		Game:          manifest.Game,
		Vault:         request.VaultPath,
		Destination:   windowsPath(vaultVolume, append(append([]string{}, gameSegments...), currentDir)),
		AllowRemovals: request.AllowRemovals,
		manifest:      manifest, vaultVolume: vaultVolume, vaultSegments: vaultSegments,
	}
	accepted := map[string]bool{}
	for _, id := range request.AcceptMissing {
		if accepted[id] {
			return Plan{}, &InvocationError{fmt.Sprintf("--accept-missing-source %s is given twice", id)}
		}
		accepted[id] = true
	}

	scope, err := s.files.OpenScope(vaultVolume)
	if errors.Is(err, fs.ErrNotExist) {
		return Plan{}, errVault(request.VaultPath, "volume %s is not available", vaultVolume)
	}
	if err != nil {
		return Plan{}, fmt.Errorf("vault %s: open volume %s: %w", request.VaultPath, vaultVolume, err)
	}
	defer func() { _ = scope.Close() }()
	v, err := resolveVault(scope, request.VaultPath, vaultSegments)
	if err != nil {
		return Plan{}, err
	}
	if plan.Volume, err = s.describe(request.VaultPath, vaultVolume, vaultSegments, v.exists); err != nil {
		return Plan{}, err
	}
	if strings.EqualFold(vaultVolume, installVolume) {
		install, complete := chainOf(scope, installSegments)
		if overlapping(scope, v.chain, v.exists, install, complete) {
			return Plan{}, errVault(request.VaultPath, "is the installation, contains it or lies in it under another name; the vault must be distinct")
		}
	}
	if err := s.checkSources(manifest, installVolume, vaultVolume, scope); err != nil {
		return Plan{}, err
	}
	if err := v.read(scope, request.VaultPath, vaultSegments, manifest.Game.ID); err != nil {
		return Plan{}, err
	}
	if err := ctx.Err(); err != nil {
		return Plan{}, fmt.Errorf("plan interrupted: %w", err)
	}
	plan.VaultState, plan.Pending, plan.observed = v.state, v.pending, v
	if len(v.interventions) > 0 {
		plan.Interventions, plan.CurrentUnchecked = v.interventions, v.current || v.currentLink
		sort.SliceStable(plan.Interventions, func(a, b int) bool { return plan.Interventions[a].Subject < plan.Interventions[b].Subject })
		return plan, nil
	}

	previous, err := s.checkCurrent(ctx, &plan, v, vaultVolume, gameSegments)
	if err != nil {
		return Plan{}, err
	}
	for i, source := range manifest.Sources {
		volume, segments, err := manifest.SourceSegments(i)
		if err != nil {
			return Plan{}, err
		}
		dir, err := s.inventories.Directory(ctx, volume, segments)
		if err != nil {
			return Plan{}, err
		}
		plan.planSource(source, windowsPath(volume, segments), dir, previous, accepted[source.ID])
		if strings.EqualFold(volume, vaultVolume) {
			plan.add(backup.Info, CodeSameVolume, source.ID, vaultVolume, "vault and source share a volume; not an off-disk backup")
		}
	}
	if err := plan.checkAccepted(previous, request.AcceptMissing); err != nil {
		return Plan{}, err
	}
	plan.checkSpace()
	if previous != nil {
		for _, old := range previous.Sources {
			if !declared(manifest, old.ID) {
				plan.add(backup.Blocking, CodeSourceRemoved, old.ID, "",
					"in the receipt of the current generation but no longer in the manifest; removing a source needs a separate decision")
			}
		}
	}
	plan.sortFindings(manifest)
	return plan, nil
}

// describe qualifies the vault's volume, read through the vault itself or,
// while it does not exist, through its parent.
func (s *Service) describe(vaultPath, volume string, segments []string, exists bool) (backup.Volume, error) {
	existing := segments
	if !exists {
		existing = segments[:len(segments)-1]
	}
	described, err := s.volumes.Describe(windowsPath(volume, existing))
	if err != nil {
		return backup.Volume{}, errVault(vaultPath, "volume cannot be described: %v", err)
	}
	return described, qualify(vaultPath, described)
}

// checkSpace compares the space a publication needs with the space the
// user may still write (plan B2 §4.2, V2). It is checked again under lock.
func (p *Plan) checkSpace() {
	var sizes []int64
	for _, source := range p.Sources {
		sizes = append(sizes, source.Bytes)
	}
	needed, err := spaceNeeded(sizes)
	switch {
	case err != nil:
		p.add(backup.Blocking, CodeInsufficientSpace, "", p.Vault, err.Error())
	case p.Volume.Available < needed:
		p.Needed = needed
		p.add(backup.Blocking, CodeInsufficientSpace, "", p.Vault,
			fmt.Sprintf("%d bytes needed, %d available to this user", needed, p.Volume.Available))
	default:
		p.Needed = needed
	}
}

// checkSources refuses sources that resolve to overlapping directories by
// identity, such as an 8.3 short name, as inspect does; the manifest grammar
// already refuses overlaps by name.
func (s *Service) checkSources(manifest game.Manifest, installVolume, vaultVolume string, vaultScope Scope) error {
	scope := vaultScope
	if !strings.EqualFold(installVolume, vaultVolume) {
		opened, err := s.files.OpenScope(installVolume)
		if errors.Is(err, fs.ErrNotExist) {
			return nil // every source is missing; the plan reports it
		}
		if err != nil {
			return fmt.Errorf("open volume %s: %w", installVolume, err)
		}
		defer func() { _ = opened.Close() }()
		scope = opened
	}
	chains := make([][]fs.FileInfo, len(manifest.Sources))
	complete := make([]bool, len(manifest.Sources))
	for i := range manifest.Sources {
		_, segments, err := manifest.SourceSegments(i)
		if err != nil {
			return err
		}
		chains[i], complete[i] = chainOf(scope, segments)
	}
	for i := range manifest.Sources {
		for j := i + 1; j < len(manifest.Sources); j++ {
			if overlapping(scope, chains[i], complete[i], chains[j], complete[j]) {
				return fmt.Errorf("sources %q and %q resolve to overlapping directories", manifest.Sources[i].ID, manifest.Sources[j].ID)
			}
		}
	}
	return nil
}

// checkCurrent compares the current generation with its receipt, exactly
// (plan B2 §4.5). It returns the receipt to plan against, or nil when the
// vault holds no generation for this game.
func (s *Service) checkCurrent(ctx context.Context, plan *Plan, v *vault, volume string, gameSegments []string) (*backup.Receipt, error) {
	if !v.current && !v.currentLink && len(v.receipts) == 0 && len(v.invalid) == 0 {
		return nil, nil
	}
	head, headErr := v.head()
	switch {
	case v.currentLink:
		plan.add(backup.Blocking, CodeCurrentModified, "", currentDir, "current is a link, a file or a reparse point; not followed")
	case !v.current:
		plan.add(backup.Blocking, CodeCurrentModified, "", currentDir, "a receipt exists but current does not")
	case headErr != nil:
		plan.add(backup.Blocking, CodeCurrentModified, "", currentDir, headErr.Error())
	}
	if headErr != nil {
		return nil, nil
	}
	if !v.current {
		return &head, nil
	}
	plan.Current = head.Operation
	dir, err := s.inventories.Directory(ctx, volume, append(append([]string{}, gameSegments...), currentDir))
	if err != nil {
		return nil, err
	}
	for _, difference := range compareWithReceipt(dir, head) {
		plan.add(backup.Blocking, CodeCurrentModified, difference.source, difference.path, difference.detail)
	}
	return &head, nil
}

type difference struct{ source, path, detail string }

// compareWithReceipt lists every way the inventory of current differs from
// the receipt that describes it.
func compareWithReceipt(dir backup.Directory, receipt backup.Receipt) []difference {
	var out []difference
	if dir.Status != backup.StatusInventoried {
		return []difference{{"", currentDir, "current cannot be inventoried: " + string(dir.Status)}}
	}
	for _, finding := range dir.Findings {
		out = append(out, difference{"", finding.Path, "current: " + finding.Detail})
	}
	files := map[string]backup.Entry{}
	for _, entry := range dir.Entries {
		files[entry.Path] = entry
	}
	directories := map[string]bool{}
	for _, d := range dir.Directories {
		directories[d] = true
	}
	expected := map[string]bool{}
	for _, source := range receipt.Sources {
		if source.State != backup.SourceCopied {
			continue
		}
		expected[source.ID] = true
		if !directories[source.ID] {
			out = append(out, difference{source.ID, "", "directory of the source is missing from current"})
		}
		for _, d := range source.Directories {
			expected[source.ID+"/"+d] = true
			if !directories[source.ID+"/"+d] {
				out = append(out, difference{source.ID, d, "directory missing from current"})
			}
		}
		for _, want := range source.Entries {
			full := source.ID + "/" + want.Path
			expected[full] = true
			got, ok := files[full]
			switch {
			case !ok:
				out = append(out, difference{source.ID, want.Path, "missing from current"})
			case got.Size != want.Size || !got.ModTime.Equal(want.ModTime) || got.SHA256 != want.SHA256:
				out = append(out, difference{source.ID, want.Path, "differs from its receipt"})
			}
		}
	}
	for _, entry := range dir.Entries {
		if !expected[entry.Path] {
			out = append(out, difference{sourceOf(entry.Path), relativeTo(entry.Path), "not in the receipt of current"})
		}
	}
	for _, d := range dir.Directories {
		if !expected[d] {
			out = append(out, difference{sourceOf(d), relativeTo(d), "directory not in the receipt of current"})
		}
	}
	return out
}

func sourceOf(p string) string {
	source, _, _ := strings.Cut(p, "/")
	return source
}

func relativeTo(p string) string {
	_, rest, _ := strings.Cut(p, "/")
	return rest
}

// planSource applies plan B2 §4.4 to one source. accepted says that
// --accept-missing-source names it; its validity is checked afterwards.
func (p *Plan) planSource(source game.Source, path string, dir backup.Directory, previous *backup.Receipt, accepted bool) {
	result := SourcePlan{Source: source, Path: path, inventory: dir}
	var old backup.ReceiptSource
	hadOld := false
	if previous != nil {
		old, hadOld = previous.Source(source.ID)
	}
	switch dir.Status {
	case backup.StatusMissing:
		result.Status = SourceMissing
		switch {
		case source.Required:
			p.add(backup.Blocking, CodeSourceMissing, source.ID, dir.Missing, "does not exist")
		case hadOld && old.State == backup.SourceCopied && accepted:
			result.Accepted = true
			p.add(backup.Info, CodeSourceVanished, source.ID, dir.Missing,
				"optional source absent, accepted by --accept-missing-source; its files stay in retained")
		case hadOld && old.State == backup.SourceCopied:
			p.add(backup.Blocking, CodeSourceVanished, source.ID, dir.Missing,
				"optional source absent but present in the receipt of the current generation; an absence is never a deletion")
		default:
			p.add(backup.Info, CodeSourceMissing, source.ID, dir.Missing, "does not exist")
		}
	case backup.StatusRefused:
		result.Status = SourceRefused
		p.inherit(source.ID, dir.Findings)
	default:
		result.Status = SourcePlanned
		p.inherit(source.ID, dir.Findings)
		for _, name := range dir.Encrypted {
			p.add(backup.Blocking, CodeEncryptedSource, source.ID, name, "encrypted by EFS; never copied in clear")
		}
		for _, name := range append(append([]string{}, dir.Directories...), entryPaths(dir.Entries)...) {
			if err := portable(name); err != nil {
				p.add(backup.Blocking, CodeUnportableName, source.ID, name, err.Error())
			}
		}
		result.Changes = backup.Compare(old.Entries, old.Directories, dir.Entries, dir.Directories)
		result.Files, result.Bytes = len(dir.Entries), dir.Bytes
		severity, accepted := backup.Blocking, ""
		if p.AllowRemovals {
			severity, accepted = backup.Info, "; accepted by --allow-removals, kept in retained"
		}
		for _, change := range result.Changes.Files {
			if change.Kind == backup.Removed {
				p.add(severity, CodeFilesRemoved, source.ID, change.Entry.Path, "in the current generation but no longer in the source"+accepted)
			}
		}
		for _, d := range result.Changes.RemovedDirectories {
			p.add(severity, CodeFilesRemoved, source.ID, d, "directory in the current generation but no longer in the source"+accepted)
		}
		p.checkLength(source.ID, dir.Entries)
	}
	p.Sources = append(p.Sources, result)
}

// checkAccepted refuses every --accept-missing-source that does not name
// an optional source of the manifest, physically absent, whose files the
// receipt of current holds (plan B2 §4.4, V1): an invalid invocation, code
// 1, nothing written.
func (p *Plan) checkAccepted(previous *backup.Receipt, ids []string) error {
	for _, id := range ids {
		var planned *SourcePlan
		for i := range p.Sources {
			if p.Sources[i].Source.ID == id {
				planned = &p.Sources[i]
			}
		}
		var old backup.ReceiptSource
		known := false
		if previous != nil {
			old, known = previous.Source(id)
		}
		var why string
		switch {
		case planned == nil && known:
			why = "was removed from the manifest; removing a source needs a separate decision, never this flag"
		case planned == nil:
			why = "is not a source of the manifest"
		case planned.Source.Required:
			why = "is required; a required source that is absent always blocks"
		case planned.Status != SourceMissing:
			why = "is present; files removed from a present source need --allow-removals"
		case !known || old.State != backup.SourceCopied:
			why = "is not in the receipt of the current generation; nothing of it would be lost"
		default:
			continue
		}
		return &InvocationError{fmt.Sprintf("--accept-missing-source %s: the source %s", id, why)}
	}
	return nil
}

func (p *Plan) inherit(source string, findings []backup.Finding) {
	for _, finding := range findings {
		finding.Source = source
		p.Findings = append(p.Findings, finding)
	}
}

// checkLength reports, once per source, paths that will exceed MAX_PATH in
// the current generation.
func (p *Plan) checkLength(source string, entries []backup.Entry) {
	longest, longestLength, count := "", 0, 0
	for _, entry := range entries {
		full := p.Destination + `\` + source + `\` + strings.ReplaceAll(entry.Path, "/", `\`)
		if length := len(utf16.Encode([]rune(full))); length > maxPathUTF16 {
			count++
			if length > longestLength {
				longest, longestLength = entry.Path, length
			}
		}
	}
	if count > 0 {
		p.add(backup.Info, CodeLongPath, source, longest,
			fmt.Sprintf("%d path(s) beyond %d characters in the vault; Explorer and some games may fail on a manual copy", count, maxPathUTF16))
	}
}

func portable(name string) error {
	for _, segment := range strings.Split(name, "/") {
		if err := game.CheckWindowsName(segment); err != nil {
			return err
		}
	}
	return nil
}

func entryPaths(entries []backup.Entry) []string {
	out := make([]string, len(entries))
	for i, entry := range entries {
		out[i] = entry.Path
	}
	return out
}

func (p *Plan) add(severity backup.Severity, code backup.Code, source, path, detail string) {
	p.Findings = append(p.Findings, backup.Finding{Severity: severity, Code: code, Source: source, Path: path, Detail: detail})
}

// sortFindings orders findings like inspect: vault findings first, then by
// source in manifest order, path and code.
func (p *Plan) sortFindings(manifest game.Manifest) {
	order := map[string]int{"": -1}
	for i, source := range manifest.Sources {
		order[source.ID] = i
	}
	rank := func(source string) int {
		if r, ok := order[source]; ok {
			return r
		}
		return len(manifest.Sources)
	}
	sort.SliceStable(p.Findings, func(a, b int) bool {
		x, y := p.Findings[a], p.Findings[b]
		if rank(x.Source) != rank(y.Source) {
			return rank(x.Source) < rank(y.Source)
		}
		if x.Source != y.Source {
			return x.Source < y.Source
		}
		if x.Path != y.Path {
			return x.Path < y.Path
		}
		return x.Code < y.Code
	})
}

func declared(manifest game.Manifest, id string) bool {
	for _, source := range manifest.Sources {
		if source.ID == id {
			return true
		}
	}
	return false
}

func windowsPath(volume string, segments []string) string {
	return volume + strings.Join(segments, `\`)
}
