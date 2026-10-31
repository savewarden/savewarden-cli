package sync

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/entities/game"
)

// Finding codes emitted by snapshot, beyond those it shares with sync
// (ADR 0008).
const (
	CodeNoCurrent          backup.Code = "no-current"
	CodeSnapshotMissing    backup.Code = "snapshot-missing"
	CodeSnapshotRenameLost backup.Code = "snapshot-rename-lost"
	CodeSnapshotModified   backup.Code = "snapshot-modified"
)

// snapshotIDWidth is the length of an operation id, the name of a snapshot.
const snapshotIDWidth = len("20261105T101500-1a2b3c4d")

// SnapshotRequest selects the manifest and the vault of a snapshot.
type SnapshotRequest struct {
	ManifestPath string
	VaultPath    string
}

// PublishedSnapshot is one approved snapshot, as the plan saw it.
type PublishedSnapshot struct {
	ID string
	// Source is the operation of the generation the snapshot copies.
	Source    string
	Files     int
	Bytes     int64
	Condition backup.SnapshotCondition
}

// Degraded reports an approved snapshot that is not intact: reported,
// never blocking, never repaired.
func (s PublishedSnapshot) Degraded() bool { return s.Condition != backup.SnapshotIntact }

// SnapshotPlan is the typed result of a read-only snapshot plan (ADR
// 0008). It is complete: a failed or interrupted plan returns an error.
type SnapshotPlan struct {
	ManifestPath string
	Game         game.Game
	Vault        string
	Volume       backup.Volume
	// Current is the operation of the recognised current generation, empty
	// when there is none; CurrentUnchecked, that an intervention stopped the
	// plan before it was checked.
	Current          string
	CurrentUnchecked bool
	// Recognized is set once current proved, content and identity, to be
	// the generation its receipt describes.
	Recognized bool
	VaultState VaultState
	// Snapshots are the approved snapshots, oldest first; Incomplete counts
	// the kept copies, by name only.
	Snapshots  []PublishedSnapshot
	Incomplete int
	// UpToDate names the intact snapshot that already describes current:
	// --apply would then write nothing.
	UpToDate string
	// Files, Bytes and Needed describe the copy --apply would make.
	Files         int
	Bytes         int64
	Needed        uint64
	Pending       []Pending // snapshot operations that are not terminal
	Interventions []Intervention
	Findings      []backup.Finding

	observed      *vault
	domain        *snapshotVault
	head          *backup.Receipt
	vaultVolume   string
	vaultSegments []string
}

// Blocking counts the blocking findings.
func (p SnapshotPlan) Blocking() int { return backup.CountBlocking(p.Findings) }

// Degraded counts the approved snapshots that are not intact.
func (p SnapshotPlan) Degraded() int {
	count := 0
	for _, s := range p.Snapshots {
		if s.Degraded() {
			count++
		}
	}
	return count
}

// Outcome summarises the plan in one stable phrase.
func (p SnapshotPlan) Outcome() string {
	switch {
	case len(p.Interventions) > 0:
		return "intervention required"
	case p.Blocking() > 0:
		return "blocked"
	case p.UpToDate != "":
		return "up to date"
	}
	return "new snapshot"
}

// SnapshotPlan shows what snapshot --apply would copy, without writing
// anything (ADR 0006 §10, ADR 0008). It reads the vault and the identities
// of the installation and the sources, never their content.
func (s *Service) SnapshotPlan(ctx context.Context, request SnapshotRequest) (SnapshotPlan, error) {
	return s.snapshotPlan(ctx, request, true)
}

// snapshotPlan is SnapshotPlan; a run that holds the lock does not probe it.
func (s *Service) snapshotPlan(ctx context.Context, request SnapshotRequest, probe bool) (SnapshotPlan, error) {
	manifest, err := s.manifests.Read(request.ManifestPath)
	if err != nil {
		return SnapshotPlan{}, err
	}
	vaultVolume, vaultSegments, err := game.ParseWindowsAbsolute(request.VaultPath)
	if err != nil {
		return SnapshotPlan{}, fmt.Errorf("vault %s: %w", request.VaultPath, err)
	}
	installVolume, installSegments, err := game.ParseWindowsAbsolute(manifest.Installation.Path)
	if err != nil {
		return SnapshotPlan{}, err
	}
	if err := distinctByName(request.VaultPath, vaultVolume, vaultSegments, installVolume, installSegments); err != nil {
		return SnapshotPlan{}, err
	}
	plan := SnapshotPlan{ManifestPath: request.ManifestPath, Game: manifest.Game, Vault: request.VaultPath,
		vaultVolume: vaultVolume, vaultSegments: vaultSegments}
	scope, err := s.files.OpenScope(vaultVolume)
	if errors.Is(err, fs.ErrNotExist) {
		return SnapshotPlan{}, errVault(request.VaultPath, "volume %s is not available", vaultVolume)
	}
	if err != nil {
		return SnapshotPlan{}, fmt.Errorf("vault %s: open volume %s: %w", request.VaultPath, vaultVolume, err)
	}
	defer func() { _ = scope.Close() }()
	v, err := resolveVault(scope, request.VaultPath, vaultSegments)
	if err != nil {
		return SnapshotPlan{}, err
	}
	if !v.exists {
		return SnapshotPlan{}, errVault(request.VaultPath, "does not exist; sync --apply creates a vault, snapshot never does")
	}
	if plan.Volume, err = s.describe(request.VaultPath, vaultVolume, vaultSegments, true); err != nil {
		return SnapshotPlan{}, err
	}
	if strings.EqualFold(vaultVolume, installVolume) {
		install, complete := chainOf(scope, installSegments)
		if overlapping(scope, v.chain, true, install, complete) {
			return SnapshotPlan{}, errVault(request.VaultPath, "is the installation, contains it or lies in it under another name; the vault must be distinct")
		}
	}
	if err := s.checkSources(manifest, installVolume, vaultVolume, scope); err != nil {
		return SnapshotPlan{}, err
	}
	if err := v.readProbing(scope, request.VaultPath, vaultSegments, manifest.Game.ID, probe); err != nil {
		return SnapshotPlan{}, err
	}
	if v.state == VaultEmpty {
		return SnapshotPlan{}, errVault(request.VaultPath, "is empty, not yet a vault; sync --apply initializes it, snapshot never does")
	}
	if err := ctx.Err(); err != nil {
		return SnapshotPlan{}, fmt.Errorf("plan interrupted: %w", err)
	}
	plan.observed, plan.VaultState = v, v.state
	root := strings.Join(vaultSegments, "/")
	gameName := join(root, gamesDir+"/"+manifest.Game.ID)
	plan.domain = &snapshotVault{}
	if v.game {
		if plan.domain, err = readSnapshots(scope, request.VaultPath, gameName, gamesDir+"/"+manifest.Game.ID+"/", manifest.Game.ID, v); err != nil {
			return SnapshotPlan{}, err
		}
	}
	plan.Snapshots, plan.Incomplete, plan.Pending = plan.domain.snapshots, plan.domain.incomplete, plan.domain.pending
	plan.Interventions = append(append([]Intervention{}, v.interventions...), plan.domain.interventions...)
	if len(plan.Interventions) > 0 {
		sort.SliceStable(plan.Interventions, func(a, b int) bool { return plan.Interventions[a].Subject < plan.Interventions[b].Subject })
		plan.CurrentUnchecked = v.current || v.currentLink
		return plan, nil
	}
	gameSegments := append(append([]string{}, vaultSegments...), gamesDir, manifest.Game.ID)
	// The current domain is judged exactly as sync judges it (ADR 0008).
	var current Plan
	head, err := s.checkCurrent(ctx, &current, v, vaultVolume, gameSegments)
	if err != nil {
		return SnapshotPlan{}, err
	}
	plan.Current, plan.Findings = current.Current, current.Findings
	switch {
	case len(plan.Findings) > 0:
		// current-modified: nothing is copied from a current that differs.
	case head == nil:
		plan.add(backup.Blocking, CodeNoCurrent, "", currentDir, "no generation is published for this game; sync --apply publishes one")
	default:
		// Identity read again after the content (ADR 0006 §5).
		if id, err := scope.Identity(join(gameName, currentDir)); err != nil || id != v.currentID {
			plan.Interventions = append(plan.Interventions, Intervention{Cause: CauseUnexpectedEntry, Subject: gamesDir + "/" + manifest.Game.ID + "/" + currentDir,
				Detail: "was replaced while it was read, or cannot be checked; nothing planned"})
			return plan, nil
		}
		plan.head, plan.Recognized = head, true
		if err := s.decide(ctx, &plan, scope, v, gameName, gameSegments); err != nil {
			return SnapshotPlan{}, err
		}
	}
	plan.degradations()
	plan.sortFindings()
	return plan, nil
}

// decide finds the intact snapshot that already describes current, read
// in full, and otherwise sizes the new copy (ADR 0008, S7).
func (s *Service) decide(ctx context.Context, plan *SnapshotPlan, scope Scope, v *vault, gameName string, gameSegments []string) error {
	sum := v.receiptSum[plan.head.Operation]
	for i := len(plan.Snapshots) - 1; i >= 0; i-- {
		snap := &plan.Snapshots[i]
		j := plan.domain.journals[snap.ID]
		if snap.Condition != backup.SnapshotIntact || j.SnapshotSource.Operation != plan.head.Operation || j.SnapshotSource.ReceiptSHA256 != sum {
			continue
		}
		name := backup.SnapshotName(snap.ID)
		dir, err := s.inventories.Directory(ctx, plan.vaultVolume, append(append([]string{}, gameSegments...), strings.Split(name, "/")...))
		if err != nil {
			return err
		}
		identity, _, _ := j.Published()
		differences := compareWithReceipt(dir, backup.Receipt{Sources: j.SnapshotPlan.Sources})
		if id, err := scope.Identity(join(gameName, name)); len(differences) > 0 || err != nil || id != identity {
			snap.Condition = backup.SnapshotModified
			continue
		}
		plan.UpToDate = snap.ID
		return nil
	}
	var sizes []int64
	for _, source := range plan.head.Sources {
		plan.Files += len(source.Entries)
		plan.Bytes += source.Bytes()
		sizes = append(sizes, source.Bytes())
	}
	needed, err := spaceNeeded(sizes)
	switch {
	case err != nil:
		plan.add(backup.Blocking, CodeInsufficientSpace, "", plan.Vault, err.Error())
	case plan.Volume.Available < needed:
		plan.Needed = needed
		plan.add(backup.Blocking, CodeInsufficientSpace, "", plan.Vault,
			fmt.Sprintf("%d bytes needed, %d available to this user", needed, plan.Volume.Available))
	default:
		plan.Needed = needed
	}
	plan.checkLength()
	return nil
}

// checkLength reports, once per source, the paths of the incomplete copy,
// the longest written, beyond MAX_PATH (ADR 0008).
func (p *SnapshotPlan) checkLength() {
	game := windowsPath(p.vaultVolume, append(append([]string{}, p.vaultSegments...), gamesDir, p.Game.ID))
	incomplete := game + `\` + snapshotsDir + `\` + strings.Repeat("0", snapshotIDWidth) + ".incomplete"
	for _, source := range p.head.Sources {
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
				fmt.Sprintf("%d path(s) beyond %d characters in the incomplete copy of the snapshot", count, maxPathUTF16))
		}
	}
}

// degradations reports every approved snapshot that is not intact (ADR
// 0008): information, never blocking.
func (p *SnapshotPlan) degradations() {
	for _, s := range p.Snapshots {
		name := backup.SnapshotName(s.ID)
		switch s.Condition {
		case backup.SnapshotMissing:
			p.add(backup.Info, CodeSnapshotMissing, "", name, "the approved snapshot is absent; it is never recreated")
		case backup.SnapshotRenameLost:
			p.add(backup.Info, CodeSnapshotRenameLost, "", name,
				"the approved snapshot is absent and its copy is still incomplete, as after a power cut; it is never renamed")
		case backup.SnapshotModified:
			p.add(backup.Info, CodeSnapshotModified, "", name, "the approved snapshot or its receipt differs from what its journal published; it is never repaired")
		}
	}
}

func (p *SnapshotPlan) add(severity backup.Severity, code backup.Code, source, path, detail string) {
	p.Findings = append(p.Findings, backup.Finding{Severity: severity, Code: code, Source: source, Path: path, Detail: detail})
}

// sortFindings orders findings by source, path and code.
func (p *SnapshotPlan) sortFindings() {
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

// snapshotVault is what a snapshot plan read of the snapshot domain
// (ADR 0006 §10). Nothing of it ever reaches the decisions of current.
type snapshotVault struct {
	journals     map[string]backup.Journal
	lastRecord   map[string]string
	receipts     map[string][]byte // final snapshot receipt files, by operation
	receiptTaken map[string]map[int]bool
	present      map[string]bool // snapshots\<op> and snapshots\<op>.incomplete found
	snapshots    []PublishedSnapshot
	incomplete   int
	pending      []Pending

	interventions []Intervention
}

func (d *snapshotVault) intervene(cause Cause, subject, detail string) {
	d.interventions = append(d.interventions, Intervention{Cause: cause, Subject: subject, Detail: detail})
}

var (
	snapshotName   = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}-[0-9a-f]{8}$`)
	incompleteName = regexp.MustCompile(`^([0-9]{8}T[0-9]{6}-[0-9a-f]{8})\.incomplete$`)
)

// readSnapshots reads games\<id>\snapshots strictly: its grammar, its
// journals and receipts, and what each name is explained by, then the
// condition of each approved snapshot. Its anomalies are its own: they
// block a new snapshot, never sync.
func readSnapshots(scope Scope, vaultPath, gameName, prefix, gameID string, v *vault) (*snapshotVault, error) {
	d := &snapshotVault{journals: map[string]backup.Journal{}, lastRecord: map[string]string{}, receipts: map[string][]byte{},
		receiptTaken: map[string]map[int]bool{}, present: map[string]bool{}}
	base, subject := join(gameName, snapshotsDir), prefix+snapshotsDir
	info, err := scope.Lstat(base)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && (isLink(info.Mode()) || !info.IsDir())) {
		// Absent, or refused by the reading of current already.
		return d, nil
	}
	if err != nil {
		return nil, errVault(vaultPath, "%v", err)
	}
	entries, err := list(scope, base)
	if err != nil {
		return nil, errVault(vaultPath, "%v", err)
	}
	for _, entry := range entries {
		directory := entry.Kind == backup.KindDirectory
		switch {
		case entry.Name == receiptsDir && directory:
			if err := d.readReceipts(scope, vaultPath, join(base, receiptsDir), subject+"/"+receiptsDir); err != nil {
				return nil, err
			}
		case entry.Name == journalDir && directory:
			d.readJournals(scope, join(base, journalDir), subject+"/"+journalDir, gameID)
		case directory && (snapshotName.MatchString(entry.Name) || incompleteName.MatchString(entry.Name)):
			d.present[entry.Name] = true
			if incompleteName.MatchString(entry.Name) {
				d.incomplete++
			}
		default:
			d.intervene(CauseUnexpectedEntry, subject+"/"+entry.Name, "has no place in the snapshot domain")
		}
	}
	// v is nil for a recovery, which never reads the current domain.
	for op := range d.journals {
		if v == nil {
			break
		}
		if _, used := v.journals[op]; used || v.receiptSum[op] != ([32]byte{}) {
			d.intervene(CauseJournalAmbiguous, subject+"/"+journalDir+"/"+op, "the operation id is also one of current; ids are never shared")
		}
	}
	d.explain(scope, base, subject)
	if len(d.pending) > 1 {
		d.intervene(CauseJournalAmbiguous, subject+"/"+journalDir, fmt.Sprintf("%d snapshot operations are pending for this game; only one may be", len(d.pending)))
	}
	sort.Slice(d.snapshots, func(a, b int) bool { return d.snapshots[a].ID < d.snapshots[b].ID })
	return d, nil
}

// readJournals parses every snapshot operation journal, in its own domain.
func (d *snapshotVault) readJournals(scope Scope, name, subject, gameID string) {
	entries, err := list(scope, name)
	if err != nil {
		d.intervene(CauseJournalAmbiguous, subject, "journal cannot be listed: "+err.Error())
		return
	}
	for _, entry := range entries {
		if entry.Kind != backup.KindDirectory || backup.ValidOperationID(entry.Name) != nil {
			d.intervene(CauseUnexpectedEntry, subject+"/"+entry.Name, "is not a snapshot operation journal")
			continue
		}
		op := entry.Name
		journal, last, err := readJournalWith(scope, join(name, op), op, func(names []string, contents map[string][]byte) (backup.Journal, error) {
			return backup.ParseSnapshotJournal(gameID, op, names, contents)
		})
		if err != nil {
			d.intervene(CauseJournalAmbiguous, subject+"/"+op, err.Error())
			continue
		}
		d.journals[op], d.lastRecord[op] = journal, last
		if !journal.Terminal() {
			step := string(journal.Last())
			if step == "" {
				step = "none"
			}
			d.pending = append(d.pending, Pending{Operation: op, Last: journal.Last()})
			d.intervene(CausePendingOperation, op, fmt.Sprintf("snapshot operation, last committed step %s; run snapshot --recover to see how it would be recovered", step))
		}
	}
}

// readReceipts keeps the bytes of every final snapshot receipt; temporaries
// are counted, never read.
func (d *snapshotVault) readReceipts(scope Scope, vaultPath, name, subject string) error {
	entries, err := list(scope, name)
	if err != nil {
		return errVault(vaultPath, "%v", err)
	}
	for _, entry := range entries {
		m := receiptName.FindStringSubmatch(entry.Name)
		switch {
		case m == nil || entry.Kind != backup.KindRegular:
			d.intervene(CauseUnexpectedEntry, subject+"/"+entry.Name, "is not a snapshot receipt")
		case m[2] != "":
			if d.receiptTaken[m[1]] == nil {
				d.receiptTaken[m[1]] = map[int]bool{}
			}
			d.receiptTaken[m[1]][int(m[2][len(m[2])-1]-'0')] = true
		default:
			data, err := readFile(scope, join(name, entry.Name), recordReadLimit)
			if err != nil {
				return errVault(vaultPath, "%v", err)
			}
			d.receipts[m[1]] = data
		}
	}
	return nil
}

// explain requires every snapshot, copy and receipt to be named by a
// journal of its domain, and classifies the approved snapshots (ADR 0006
// §10): by name always, by identity once staged fixed it.
func (d *snapshotVault) explain(scope Scope, base, subject string) {
	for name := range d.present {
		op := strings.TrimSuffix(name, ".incomplete")
		j, ok := d.journals[op]
		switch {
		case !ok:
			d.intervene(CauseUnexpectedEntry, subject+"/"+name, "is not explained by any snapshot journal")
		case !j.Terminal() || j.Last() == backup.StepPublished:
			// Judged by recovery, or by the condition of the snapshot below.
		case name == op:
			d.intervene(CauseUnexpectedEntry, subject+"/"+name, "exists although its operation closed as "+string(j.Last()))
		case j.Staging != (backup.FileID{}):
			if id, err := scope.Identity(join(base, name)); err != nil || id != j.Staging {
				d.intervene(CauseUnexpectedEntry, subject+"/"+name, "is not the copy its journal recorded")
			}
		}
	}
	for op := range d.receipts {
		j, ok := d.journals[op]
		if !ok || (j.Terminal() && j.Last() != backup.StepPublished) || (!j.Terminal() && j.Last() != backup.StepPublishIntent) {
			d.intervene(CauseJournalAmbiguous, subject+"/"+receiptsDir+"/"+op+".json", "no operation of the snapshot domain can have committed this receipt")
		}
	}
	for op, j := range d.journals {
		if j.Last() != backup.StepPublished {
			continue
		}
		identity, _, _ := j.Published()
		observed := backup.SnapshotObservation{
			Published:  snapshotPresence(scope, join(base, op), identity),
			Incomplete: snapshotPresence(scope, join(base, op+".incomplete"), identity),
		}
		conforms := false
		var receipt backup.SnapshotReceipt
		if data, ok := d.receipts[op]; ok {
			var err error
			receipt, err = backup.CheckSnapshotReceipt(data, j)
			conforms = err == nil
		}
		snap := PublishedSnapshot{ID: op, Source: j.SnapshotSource.Operation, Condition: backup.PublishedSnapshotCondition(observed, conforms)}
		sources := j.SnapshotPlan.Sources
		if conforms {
			sources = receipt.Sources
		}
		for _, source := range sources {
			snap.Files += len(source.Entries)
			snap.Bytes += source.Bytes()
		}
		d.snapshots = append(d.snapshots, snap)
	}
}

// snapshotPresence describes a name of the snapshot domain by identity,
// without following it.
func snapshotPresence(scope Scope, name string, published backup.FileID) backup.Presence {
	id, err := scope.Identity(name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return backup.Absent
	case err == nil && id == published:
		return backup.New
	}
	return backup.Other
}
