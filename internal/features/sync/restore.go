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

// codeUnreadable is the code inspect gives a file it could not read,
// documented with inspect: restore never emits it, but reads it as a
// snapshot that cannot be verified, never one proved altered (ADR 0009).
const codeUnreadable = backup.Code("unreadable")

// RestoreRequest selects the manifest, the vault, the snapshot and the
// destination of a restore (ADR 0009). A recovery leaves VaultPath and
// Snapshot empty: it never reads the vault.
type RestoreRequest struct {
	ManifestPath string
	VaultPath    string
	Snapshot     string
	Destination  string
}

// Destination states, as the plan names them.
const (
	DestinationNew   = "new restore area"
	DestinationEmpty = "empty directory, new restore area"
	DestinationArea  = "restore area"
)

// RestorePlan is the typed result of a read-only restore plan (ADR 0009).
// It is complete: a failed or interrupted plan returns an error.
type RestorePlan struct {
	ManifestPath string
	Game         game.Game
	Vault        string
	Snapshot     string
	// From is the operation of the generation the snapshot copied; Verified
	// is set once the snapshot proved intact, its content read in full, and
	// Condition classifies it otherwise.
	From      string
	Verified  bool
	Condition backup.SnapshotCondition
	// Destination is the area as the user named it, State how it was found,
	// Volume the volume that would hold the restore.
	Destination string
	State       string
	Volume      backup.Volume
	// Restores are the handed-over restores of the area, oldest first;
	// Incomplete counts its .incomplete copies, by name only.
	Restores   []HandedOverRestore
	Incomplete int
	// Files, Bytes and Needed describe the copy --apply would make.
	Files         int
	Bytes         int64
	Needed        uint64
	Pending       []Pending
	Interventions []Intervention
	Findings      []backup.Finding

	snapshot      *restoreSource
	area          *area
	vaultVolume   string
	vaultSegments []string
	destVolume    string
	destSegments  []string
}

// restoreSource is the snapshot a restore copies, as its authentication
// proved it: its journal, the SHA-256 of its receipt file, its published
// identity and its receipt.
type restoreSource struct {
	journal    backup.Journal
	receiptSum [32]byte
	identity   backup.FileID
	receipt    backup.SnapshotReceipt
}

// Blocking counts the blocking findings.
func (p RestorePlan) Blocking() int { return backup.CountBlocking(p.Findings) }

// Outcome summarises the plan in one stable phrase.
func (p RestorePlan) Outcome() string {
	switch {
	case len(p.Interventions) > 0:
		return "intervention required"
	case p.Blocking() > 0:
		return "blocked"
	}
	return "new restore"
}

// Verdict is the authentication of the snapshot, as the plan states it.
func (p RestorePlan) Verdict() string {
	if p.Verified {
		return "verified"
	}
	return p.Condition.String() + ", not restorable"
}

// State is the condition of a handed-over restore, as the plan states it
// (ADR 0009).
func (r HandedOverRestore) State() string {
	return [...]string{"present", "absent", "rename lost", "replaced", "not observable"}[r.Condition]
}

// RestorePlan shows what restore --apply would copy, without writing
// anything (ADR 0006 §11, ADR 0009). It reads, in the vault, the
// designated snapshot only; it reads the destination, and the identities of
// the installation and the sources, never their content.
func (s *Service) RestorePlan(ctx context.Context, request RestoreRequest) (RestorePlan, error) {
	return s.restorePlan(ctx, request, true)
}

// restorePlan is RestorePlan; a run that holds the lock of the area does
// not probe it.
func (s *Service) restorePlan(ctx context.Context, request RestoreRequest, probe bool) (RestorePlan, error) {
	manifest, err := s.manifests.Read(request.ManifestPath)
	if err != nil {
		return RestorePlan{}, err
	}
	if err := backup.ValidOperationID(request.Snapshot); err != nil {
		return RestorePlan{}, fmt.Errorf("snapshot: %w", err)
	}
	vaultVolume, vaultSegments, err := game.ParseWindowsAbsolute(request.VaultPath)
	if err != nil {
		return RestorePlan{}, fmt.Errorf("vault %s: %w", request.VaultPath, err)
	}
	destVolume, destSegments, err := game.ParseWindowsAbsolute(request.Destination)
	if err != nil {
		return RestorePlan{}, errDestination(request.Destination, "%v", err)
	}
	plan := RestorePlan{ManifestPath: request.ManifestPath, Game: manifest.Game, Vault: request.VaultPath,
		Snapshot: request.Snapshot, Destination: request.Destination,
		vaultVolume: vaultVolume, vaultSegments: vaultSegments, destVolume: destVolume, destSegments: destSegments}
	if err := s.authenticate(ctx, &plan, manifest.Game.ID); err != nil {
		return RestorePlan{}, err
	}
	if err := ctx.Err(); err != nil {
		return RestorePlan{}, fmt.Errorf("plan interrupted: %w", err)
	}
	if err := s.planDestination(&plan, manifest, probe); err != nil {
		return RestorePlan{}, err
	}
	if plan.snapshot != nil && len(plan.Interventions) == 0 {
		plan.size()
	}
	plan.handovers()
	plan.sortFindings()
	return plan, nil
}

// authenticate proves the designated snapshot by a targeted read of the
// vault (ADR 0006 §11, ADR 0009): its journal published, its receipt
// strict and of the published SHA-256, its copy of the published identity
// with no .incomplete beside it, its content read in full against the
// receipt and its identity read again after. Nothing else of the vault is
// read: neither games\<game-id> is listed, nor sync.lock observed. A degraded
// snapshot is a blocking finding; a state of the snapshot domain that is not
// terminal, or a read that fails, is a refusal, code 1.
func (s *Service) authenticate(ctx context.Context, plan *RestorePlan, gameID string) error {
	vaultPath, snap := plan.Vault, plan.Snapshot
	scope, err := s.files.OpenScope(plan.vaultVolume)
	if errors.Is(err, fs.ErrNotExist) {
		return errVault(vaultPath, "volume %s is not available", plan.vaultVolume)
	}
	if err != nil {
		return errVault(vaultPath, "open volume %s: %v", plan.vaultVolume, err)
	}
	defer func() { _ = scope.Close() }()
	v, err := resolveVault(scope, vaultPath, plan.vaultSegments)
	if err != nil {
		return err
	}
	if !v.exists {
		return errVault(vaultPath, "does not exist")
	}
	root := strings.Join(plan.vaultSegments, "/")
	data, err := readFile(scope, join(root, backup.MarkerName), markerReadLimit)
	if err != nil {
		return errVault(vaultPath, "not a Savewarden vault: %v", err)
	}
	if _, err := backup.DecodeMarker(data); err != nil {
		return errVault(vaultPath, "%v; a marker is never rewritten", err)
	}
	notVerifiable := func(err error) error {
		return fmt.Errorf("snapshot %s not verifiable: %w", snap, err)
	}
	game := join(root, gamesDir+"/"+gameID)
	domain := join(game, snapshotsDir)
	journalName := join(domain, journalDir+"/"+snap)
	for _, name := range []string{join(root, gamesDir), game, domain, join(domain, journalDir), journalName} {
		info, err := scope.Lstat(name)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return fmt.Errorf("snapshot %s not found in the vault %s", snap, vaultPath)
		case err != nil:
			return notVerifiable(err)
		case isLink(info.Mode()) || !info.IsDir():
			return errVault(vaultPath, "%s is a link, a file or a reparse point; not followed", name)
		}
	}
	j, _, err := readJournalWith(scope, journalName, snap, func(names []string, contents map[string][]byte) (backup.Journal, error) {
		return backup.ParseSnapshotJournal(gameID, snap, names, contents)
	})
	var ambiguity *backup.AmbiguityError
	switch {
	case errors.As(err, &ambiguity):
		return fmt.Errorf("snapshot %s is not restorable: its journal is ambiguous (%w); see snapshot --recover", snap, ambiguity)
	case err != nil:
		return notVerifiable(err)
	case !j.Terminal():
		last := string(j.Last())
		if last == "" {
			last = "none"
		}
		return fmt.Errorf("snapshot %s is not restorable: its operation is pending, last committed step %s; see snapshot and snapshot --recover", snap, last)
	case j.Last() != backup.StepPublished:
		return fmt.Errorf("snapshot %s was not published: its operation closed as %s", snap, j.Last())
	}
	identity, receiptSum, _ := j.Published()
	plan.From = j.SnapshotSource.Operation
	// The receipt: absent, invalid or another file degrades the snapshot.
	var receipt backup.SnapshotReceipt
	conforms := false
	receipts := join(domain, receiptsDir)
	info, err := scope.Lstat(receipts)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return notVerifiable(err)
	case isLink(info.Mode()):
		return errVault(vaultPath, "%s is a link, junction or reparse point; not followed", receipts)
	case info.IsDir():
		name := join(receipts, snap+".json")
		info, err := scope.Lstat(name)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return notVerifiable(err)
		case isLink(info.Mode()):
			return errVault(vaultPath, "%s is a link, junction or reparse point; not followed", name)
		case info.Mode().IsRegular():
			data, err := readFile(scope, name, recordReadLimit)
			var tooLarge *tooLargeError
			switch {
			case errors.As(err, &tooLarge):
			case err != nil:
				return notVerifiable(err)
			default:
				receipt, err = backup.CheckSnapshotReceipt(data, j)
				conforms = err == nil
			}
		}
	}
	published, err := restoreObserve(scope, join(domain, snap), identity)
	if err != nil {
		return notVerifiable(err)
	}
	incomplete, err := restoreObserve(scope, join(domain, snap+".incomplete"), identity)
	if err != nil {
		return notVerifiable(err)
	}
	condition := backup.PublishedSnapshotCondition(backup.SnapshotObservation{Published: published, Incomplete: incomplete}, conforms)
	if condition == backup.SnapshotIntact {
		segments := append(append([]string{}, plan.vaultSegments...), gamesDir, gameID, snapshotsDir, snap)
		dir, err := s.inventories.Directory(ctx, plan.vaultVolume, segments)
		switch {
		case ctx.Err() != nil:
			return fmt.Errorf("plan interrupted: %w", ctx.Err())
		case err != nil:
			return notVerifiable(err)
		}
		if hasFinding(dir.Findings, codeUnreadable) {
			return notVerifiable(errors.New("a file or directory of the snapshot cannot be read; close the programs that use the vault"))
		}
		differences := compareWithReceipt(dir, backup.Receipt{Sources: receipt.Sources})
		id, err := scope.Identity(join(domain, snap))
		switch {
		case err != nil && !errors.Is(err, fs.ErrNotExist):
			return notVerifiable(err)
		case len(differences) > 0 || err != nil || id != identity:
			condition = backup.SnapshotModified
		}
	}
	name := backup.SnapshotName(snap)
	plan.Condition = condition
	switch condition {
	case backup.SnapshotIntact:
		plan.Verified = true
		plan.snapshot = &restoreSource{journal: j, receiptSum: receiptSum, identity: identity, receipt: receipt}
	case backup.SnapshotMissing:
		plan.add(backup.Blocking, CodeSnapshotMissing, "", name, "the approved snapshot is absent; nothing can be restored from it")
	case backup.SnapshotRenameLost:
		plan.add(backup.Blocking, CodeSnapshotRenameLost, "", name,
			"the approved snapshot is absent and its copy is still incomplete, as after a power cut; nothing is restored from it")
	default:
		plan.add(backup.Blocking, CodeSnapshotModified, "", name,
			"the snapshot, its identity or its receipt differs from what its journal published; an object of an unexpected identity is never read, and no corruption is presumed: a vault copied to another volume changes every identity")
	}
	return nil
}

// restoreObserve describes a name of the snapshot domain without following
// it: Absent, New for a plain directory of the published identity, Other
// for anything else; an error means it cannot be read.
func restoreObserve(scope Scope, name string, identity backup.FileID) (backup.Presence, error) {
	info, err := scope.Lstat(name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return backup.Absent, nil
	case err != nil:
		return backup.Other, err
	case isLink(info.Mode()) || !info.IsDir():
		return backup.Other, nil
	}
	id, err := scope.Identity(name)
	switch {
	case err != nil:
		return backup.Other, err
	case id == identity:
		return backup.New, nil
	}
	return backup.Other, nil
}

func hasFinding(findings []backup.Finding, code backup.Code) bool {
	for _, finding := range findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

// planDestination reads the destination and keeps it apart from every
// reference (ADR 0006 §11): a new or empty directory, or a marked area of
// the game without an operation pending; never a vault, a source or the
// installation, whatever the drive letter.
func (s *Service) planDestination(plan *RestorePlan, manifest game.Manifest, probe bool) error {
	path := plan.Destination
	scope, err := s.files.OpenScope(plan.destVolume)
	if errors.Is(err, fs.ErrNotExist) {
		return errDestination(path, "volume %s is not available", plan.destVolume)
	}
	if err != nil {
		return errDestination(path, "open volume %s: %v", plan.destVolume, err)
	}
	defer func() { _ = scope.Close() }()
	name := strings.Join(plan.destSegments, "/")
	if len(plan.destSegments) == 0 {
		return errDestination(path, "is the root of a volume; choose a directory")
	}
	parent := strings.Join(plan.destSegments[:len(plan.destSegments)-1], "/")
	if parent == "" {
		parent = "."
	}
	if info, err := scope.Lstat(parent); err != nil || isLink(info.Mode()) || !info.IsDir() {
		return errDestination(path, "parent directory %s does not exist or is not a plain directory", windowsPath(plan.destVolume, plan.destSegments[:len(plan.destSegments)-1]))
	}
	a, err := readArea(scope, path, name, manifest.Game.ID)
	if err != nil {
		return err
	}
	plan.area = a
	switch {
	case !a.exists:
		plan.State = DestinationNew
	case a.root.State == backup.RestoreRootEmpty:
		plan.State = DestinationEmpty
	case a.root.State == backup.RestoreRootInterrupted:
		plan.State = DestinationArea
		a.intervene(CauseInitializationInterrupted, path, "holds only temporaries of the area marker; run restore --recover --apply to resume its initialization")
	default:
		plan.State = DestinationArea
	}
	running := len(a.pending) > 0 || len(a.interventions) > 0
	if probe && a.root.Lock && running && lockBusy(scope, join(name, backup.RestoreLockName)) {
		return errDestination(path, "another restore is running in this area; run it again once it ends")
	}
	plan.Restores, plan.Incomplete, plan.Pending = a.restores, a.incomplete(), a.pending
	plan.Interventions = append(plan.Interventions, a.interventions...)
	existing := plan.destSegments
	if !a.exists {
		existing = plan.destSegments[:len(plan.destSegments)-1]
	}
	if plan.Volume, err = s.volumes.Describe(windowsPath(plan.destVolume, existing)); err != nil {
		return errDestination(path, "volume cannot be described: %v", err)
	}
	if err := qualifyVolume(plan.Volume, func(format string, args ...any) error { return errDestination(path, format, args...) }); err != nil {
		return err
	}
	return s.distinctDestination(path, plan.destVolume, plan.destSegments, plan.references(manifest))
}

// references lists the paths the area must stay apart from: the vault, the
// installation, the sources of the manifest and, once the snapshot is
// proved, the paths its receipt backed up (ADR 0006 §11).
func (p *RestorePlan) references(manifest game.Manifest) []reference {
	refs := []reference{{label: "vault", path: p.Vault, volume: p.vaultVolume, segments: p.vaultSegments}}
	if volume, segments, err := game.ParseWindowsAbsolute(manifest.Installation.Path); err == nil {
		refs = append(refs, reference{label: "installation", path: manifest.Installation.Path, volume: volume, segments: segments})
	}
	for i, source := range manifest.Sources {
		if volume, segments, err := manifest.SourceSegments(i); err == nil {
			refs = append(refs, reference{label: "source " + source.ID, path: windowsPath(volume, segments), volume: volume, segments: segments})
		}
	}
	if p.snapshot != nil {
		for _, source := range p.snapshot.receipt.Sources {
			volume, segments, err := game.ParseWindowsAbsolute(source.Path)
			if err != nil {
				// A path that cannot be compared proves nothing: refused by
				// distinctDestination as a reference on no volume.
				volume, segments = source.Path, nil
			}
			refs = append(refs, reference{label: "backed-up source " + source.ID, path: source.Path, volume: volume, segments: segments})
		}
	}
	return refs
}

// size sizes the copy and checks the space of the destination volume and
// the length of the paths of the incomplete copy, the longest written.
func (p *RestorePlan) size() {
	var sizes []int64
	for _, source := range p.snapshot.receipt.Sources {
		p.Files += len(source.Entries)
		p.Bytes += source.Bytes()
		sizes = append(sizes, source.Bytes())
	}
	needed, err := spaceNeeded(sizes)
	switch {
	case err != nil:
		p.add(backup.Blocking, CodeInsufficientSpace, "", p.Destination, err.Error())
	case p.Volume.Available < needed:
		p.Needed = needed
		p.add(backup.Blocking, CodeInsufficientSpace, "", p.Destination,
			fmt.Sprintf("%d bytes needed, %d available to this user", needed, p.Volume.Available))
	default:
		p.Needed = needed
	}
	incomplete := windowsPath(p.destVolume, p.destSegments) + `\` + strings.Repeat("0", snapshotIDWidth) + ".incomplete"
	for _, source := range p.snapshot.receipt.Sources {
		longest, longestLength, count := "", 0, 0
		for _, entry := range source.Entries {
			full := incomplete + `\` + source.ID + `\` + strings.ReplaceAll(entry.Path, "/", `\`)
			if length := len(utf16.Encode([]rune(full))); length > maxPathUTF16 {
				count++
				if length > longestLength {
					longest, longestLength = entry.Path, length
				}
			}
		}
		if count > 0 {
			p.add(backup.Info, CodeLongPath, source.ID, longest,
				fmt.Sprintf("%d path(s) beyond %d characters in the incomplete copy of the restore", count, maxPathUTF16))
		}
	}
}

// handovers reports, as information, the handed-over restores of the
// area: those of the same snapshot, and those not found as published.
func (p *RestorePlan) handovers() {
	for _, r := range p.Restores {
		if r.Snapshot == p.Snapshot {
			p.add(backup.Info, CodeSnapshotAlreadyRestored, "", r.ID, "this snapshot was already restored in this area; a new restore is still possible")
		}
		switch r.Condition {
		case backup.HandoverMissing:
			p.add(backup.Info, CodeRestoreMissing, "", r.ID, "the handed-over restore is not found under its name; nothing is done for it")
		case backup.HandoverRenameLost:
			p.add(backup.Info, CodeRestoreRenameLost, "", r.ID,
				"the handed-over restore is still under .incomplete with its published identity, as after a power cut; it is never renamed")
		case backup.HandoverReplaced:
			p.add(backup.Info, CodeRestoreReplaced, "", r.ID, "another object has the name of the handed-over restore, or an .incomplete copy is beside it; nothing is done for it")
		}
	}
}

func (p *RestorePlan) add(severity backup.Severity, code backup.Code, source, path, detail string) {
	p.Findings = append(p.Findings, backup.Finding{Severity: severity, Code: code, Source: source, Path: path, Detail: detail})
}

// sortFindings orders findings by source, path and code.
func (p *RestorePlan) sortFindings() {
	sort.SliceStable(p.Findings, func(a, b int) bool {
		x, y := p.Findings[a], p.Findings[b]
		if x.Source != y.Source {
			return x.Source < y.Source
		}
		if x.Path != y.Path {
			return x.Path < y.Path
		}
		return x.Code < y.Code
	})
}
