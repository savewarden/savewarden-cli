package sync

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

// Finding codes emitted by restore, beyond those it shares with sync and
// snapshot (ADR 0009).
const (
	CodeChangedDuringRestore    backup.Code = "changed-during-restore"
	CodeSnapshotAlreadyRestored backup.Code = "snapshot-already-restored"
	CodeRestoreMissing          backup.Code = "restore-missing"
	CodeRestoreRenameLost       backup.Code = "restore-rename-lost"
	CodeRestoreReplaced         backup.Code = "restore-replaced"
)

// HandedOverRestore is one published restore of an area, as a read saw its
// names: never its content (ADR 0006 §11, ADR 0009).
type HandedOverRestore struct {
	ID        string
	Snapshot  string
	Condition backup.HandoverCondition
}

// area is what a read of a restore destination found, without writing
// anything (ADR 0006 §11): whether it exists, how its root classifies and,
// for a marked area, its operations, receipts and handed-over restores.
// Its anomalies are its own: the vault is never concerned.
type area struct {
	exists     bool
	root       backup.RestoreRoot
	marker     backup.RestoreAreaMarker
	journals   map[string]backup.Journal
	lastRecord map[string]string
	receipts   map[string][]byte
	// receiptTaken holds, per operation, the attempt numbers of its
	// receipt temporaries; unobservable, the operation names of the root
	// whose type could not be read.
	receiptTaken  map[string]map[int]bool
	unobservable  map[string]bool
	restores      []HandedOverRestore
	pending       []Pending
	interventions []Intervention
}

func (a *area) intervene(cause Cause, subject, detail string) {
	a.interventions = append(a.interventions, Intervention{Cause: cause, Subject: subject, Detail: detail})
}

// errDestination reports a destination the run must refuse outright: exit
// code 1.
func errDestination(path, format string, args ...any) error {
	return fmt.Errorf("destination %s: "+format, append([]any{path}, args...)...)
}

// readArea reads the destination name, a scope name below the volume root,
// of game gameID. A missing destination is a new one; a link, a file, or a
// directory that is not empty and not marked is refused, never adopted.
func readArea(scope Scope, path, name, gameID string) (*area, error) {
	a := &area{journals: map[string]backup.Journal{}, lastRecord: map[string]string{}, receipts: map[string][]byte{},
		receiptTaken: map[string]map[int]bool{}, unobservable: map[string]bool{}}
	info, err := scope.Lstat(name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return a, nil
	case err != nil:
		return nil, errDestination(path, "%v", err)
	case isLink(info.Mode()) || !info.IsDir():
		return nil, errDestination(path, "is a link, a file or a reparse point; not a restore area")
	}
	a.exists = true
	entries, err := a.listRoot(scope, name)
	if err != nil {
		return nil, errDestination(path, "%v", err)
	}
	a.root = backup.ClassifyRestoreRoot(entries)
	switch a.root.State {
	case backup.RestoreRootEmpty, backup.RestoreRootInterrupted:
		return a, nil
	case backup.RestoreRootForeign:
		return nil, errDestination(path, "is not empty and not a restore area; choose a new or empty directory, an existing directory is never adopted")
	}
	data, err := readFile(scope, join(name, backup.RestoreAreaMarkerName), markerReadLimit)
	if err != nil {
		return nil, errDestination(path, "marker: %v", err)
	}
	if a.marker, err = backup.DecodeRestoreAreaMarker(data); err != nil {
		return nil, errDestination(path, "%v; a marker is never rewritten", err)
	}
	if a.marker.GameID != gameID {
		return nil, errDestination(path, "is a restore area of game %q, not of %q; choose another destination", a.marker.GameID, gameID)
	}
	for _, entry := range a.root.Unexpected {
		a.intervene(CauseUnexpectedEntry, entry, "has no place in a restore area")
	}
	if a.root.Receipts {
		if err := a.readReceipts(scope, join(name, backup.RestoreReceiptsDir)); err != nil {
			return nil, errDestination(path, "%v", err)
		}
	}
	if a.root.Journal {
		a.readJournals(scope, join(name, backup.RestoreJournalDir), gameID)
	}
	a.explain(scope, name)
	if len(a.pending) > 1 {
		a.intervene(CauseJournalAmbiguous, backup.RestoreJournalDir, fmt.Sprintf("%d restore operations are pending in this area; only one may be", len(a.pending)))
	}
	sort.Slice(a.restores, func(x, y int) bool { return a.restores[x].ID < a.restores[y].ID })
	sort.SliceStable(a.interventions, func(x, y int) bool { return a.interventions[x].Subject < a.interventions[y].Subject })
	return a, nil
}

// listRoot lists the root of the destination without following anything.
// An entry named after an operation whose type cannot be read is kept as
// unobservable, for the handover to classify; any other is an error.
func (a *area) listRoot(scope Scope, name string) ([]backup.NamedEntry, error) {
	dirEntries, err := scope.ReadDir(name)
	if err != nil {
		return nil, err
	}
	entries := make([]backup.NamedEntry, 0, len(dirEntries))
	for _, entry := range dirEntries {
		info, err := scope.Lstat(join(name, entry.Name()))
		if err != nil {
			if _, ok := operationOf(entry.Name()); !ok {
				return nil, err
			}
			a.unobservable[entry.Name()] = true
			entries = append(entries, backup.NamedEntry{Name: entry.Name(), Kind: backup.KindOther})
			continue
		}
		kind := backup.KindOther
		switch {
		case isLink(info.Mode()):
		case info.IsDir():
			kind = backup.KindDirectory
		case info.Mode().IsRegular():
			kind = backup.KindRegular
		}
		entries = append(entries, backup.NamedEntry{Name: entry.Name(), Kind: kind})
	}
	sort.Slice(entries, func(x, y int) bool { return entries[x].Name < entries[y].Name })
	return entries, nil
}

// operationOf returns the operation a name of the area root denotes, the
// copy <op> or <op>.incomplete, without regard to case.
func operationOf(name string) (string, bool) {
	if op, ok := backup.CanonicalOperationID(name); ok {
		return op, true
	}
	suffix := len(".incomplete")
	if len(name) > suffix && strings.EqualFold(name[len(name)-suffix:], ".incomplete") {
		return backup.CanonicalOperationID(name[:len(name)-suffix])
	}
	return "", false
}

// readReceipts keeps the bytes of every final restore receipt; temporaries
// are counted, never read.
func (a *area) readReceipts(scope Scope, name string) error {
	entries, err := list(scope, name)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		m := receiptName.FindStringSubmatch(entry.Name)
		switch {
		case m == nil || entry.Kind != backup.KindRegular:
			a.intervene(CauseUnexpectedEntry, backup.RestoreReceiptsDir+"/"+entry.Name, "is not a restore receipt")
		case m[2] != "":
			if a.receiptTaken[m[1]] == nil {
				a.receiptTaken[m[1]] = map[int]bool{}
			}
			a.receiptTaken[m[1]][int(m[2][len(m[2])-1]-'0')] = true
		default:
			data, err := readFile(scope, join(name, entry.Name), recordReadLimit)
			if err != nil {
				return err
			}
			a.receipts[m[1]] = data
		}
	}
	return nil
}

// readJournals parses every restore operation journal of the area.
func (a *area) readJournals(scope Scope, name, gameID string) {
	entries, err := list(scope, name)
	if err != nil {
		a.intervene(CauseJournalAmbiguous, backup.RestoreJournalDir, "journal cannot be listed: "+err.Error())
		return
	}
	for _, entry := range entries {
		subject := backup.RestoreJournalDir + "/" + entry.Name
		if entry.Kind != backup.KindDirectory || backup.ValidOperationID(entry.Name) != nil {
			a.intervene(CauseUnexpectedEntry, subject, "is not a restore operation journal")
			continue
		}
		op := entry.Name
		journal, last, err := readJournalWith(scope, join(name, op), op, func(names []string, contents map[string][]byte) (backup.Journal, error) {
			return backup.ParseRestoreJournal(gameID, op, names, contents)
		})
		if err != nil {
			a.intervene(CauseJournalAmbiguous, subject, err.Error())
			continue
		}
		a.journals[op], a.lastRecord[op] = journal, last
		if !journal.Terminal() {
			step := string(journal.Last())
			if step == "" {
				step = "none"
			}
			a.pending = append(a.pending, Pending{Operation: op, Last: journal.Last()})
			a.intervene(CausePendingOperation, op, fmt.Sprintf("restore operation, last committed step %s; run restore --recover to see how it would be recovered", step))
		}
	}
}

// explain requires every copy and receipt of the area to be named by a
// journal of the area, and classifies the handed-over restores (ADR 0006
// §11): structure and objects of an operation that is not handed over
// stay strict; the names of a handed-over restore are never an ambiguity.
func (a *area) explain(scope Scope, name string) {
	if !a.root.Receipts && (len(a.journals) > 0 || len(a.root.Published) > 0 || len(a.root.Incomplete) > 0) {
		a.intervene(CauseJournalAmbiguous, backup.RestoreReceiptsDir, "is missing beside restore operations; only an empty structure is completed")
	}
	for _, names := range []struct {
		set       map[string]backup.RestoreObject
		published bool
	}{{a.root.Published, true}, {a.root.Incomplete, false}} {
		for op, object := range names.set {
			j, ok := a.journals[op]
			switch {
			case !ok:
				a.intervene(CauseUnexpectedEntry, object.Name, "is not explained by any restore journal")
			case j.Last() == backup.StepPublished:
				// Handed over: classified below, whatever it is.
			case names.published && j.Terminal():
				a.intervene(CauseUnexpectedEntry, object.Name, "exists although its operation closed as "+string(j.Last()))
			case object.Kind != backup.KindDirectory:
				a.intervene(CauseUnexpectedEntry, object.Name, "is not a plain directory")
			case !j.Terminal():
				// Judged by recovery.
			case j.Staging != (backup.FileID{}):
				if id, err := scope.Identity(join(name, object.Name)); err != nil || id != j.Staging {
					a.intervene(CauseUnexpectedEntry, object.Name, "is not the copy its journal recorded")
				}
			}
		}
	}
	for op := range a.receipts {
		j, ok := a.journals[op]
		if !ok || (j.Terminal() && j.Last() != backup.StepPublished) || (!j.Terminal() && j.Last() != backup.StepPublishIntent) {
			a.intervene(CauseJournalAmbiguous, backup.RestoreReceiptsDir+"/"+op+".json", "no restore operation of this area can have committed this receipt")
		}
	}
	for op, j := range a.journals {
		if j.Last() != backup.StepPublished {
			continue
		}
		// The receipt is not handed over: it stays strict (ADR 0009).
		data, ok := a.receipts[op]
		if !ok {
			a.intervene(CauseJournalAmbiguous, backup.RestoreReceiptsDir+"/"+op+".json", "the receipt of a handed-over restore is missing")
		} else if _, err := backup.CheckRestoreReceipt(data, j); err != nil {
			a.intervene(CauseJournalAmbiguous, backup.RestoreReceiptsDir+"/"+op+".json", err.Error())
		}
		identity, _, _ := j.Published()
		published, publishedKnown := a.handedPresence(scope, name, a.root.Published, op, identity)
		incomplete, incompleteKnown := a.handedPresence(scope, name, a.root.Incomplete, op, identity)
		a.restores = append(a.restores, HandedOverRestore{ID: op, Snapshot: j.RestoredSnapshot.Operation,
			Condition: backup.HandedOverCondition(published, incomplete, publishedKnown, incompleteKnown)})
	}
}

// handedPresence describes one name of a handed-over restore: Absent, New
// for a plain directory of the published identity, Other for anything else;
// known is false when its type or identity cannot be read. Nothing is
// opened for its content, listed, pinned or followed: the identity is read
// on a handle closed at once.
func (a *area) handedPresence(scope Scope, name string, set map[string]backup.RestoreObject, op string, identity backup.FileID) (backup.Presence, bool) {
	object, ok := set[op]
	switch {
	case !ok:
		return backup.Absent, true
	case a.unobservable[object.Name]:
		return backup.Other, false
	case object.Kind != backup.KindDirectory:
		return backup.Other, true
	}
	id, err := scope.Identity(join(name, object.Name))
	switch {
	case err != nil:
		return backup.Other, false
	case id == identity:
		return backup.New, true
	}
	return backup.Other, true
}

// incomplete counts the .incomplete copies of the area, by name only.
func (a *area) incomplete() int { return len(a.root.Incomplete) }
