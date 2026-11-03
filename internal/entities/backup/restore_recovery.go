package backup

// RestoreObservation is the state of the two names a restore operation
// moves (ADR 0006 §11): Published, <op> at the root of its area, and
// Incomplete, <op>.incomplete, with the presences of a snapshot
// observation; and whether a final receipt of the restore exists.
type RestoreObservation = SnapshotObservation

// ClassifyRestore decides the recovery of a restore operation from its
// journal and the observed names, per the table of ADR 0006 §11, which is
// that of the snapshots without a provenance gate. A committed restore
// receipt admits only publish-intent with the copy renamed, and published.
// An operation terminal in published needs no recovery, whatever is
// observed: its copy is handed over.
func ClassifyRestore(j Journal, o RestoreObservation) (Action, error) {
	if !j.Restore {
		return "", &AmbiguityError{Op: j.Op, Reason: "not the journal of a restore operation"}
	}
	return classifyCopy(j, o, "restore")
}

// HandoverCondition is how a handed-over restore looks: the copy of an
// operation whose journal is valid and terminal in published (ADR 0006 §11,
// ADR 0009). It is information only, never blocking, never repaired.
type HandoverCondition int

// Handover conditions.
const (
	// HandoverPresent: the copy is under its name with the published
	// identity, and nothing is under the .incomplete name.
	HandoverPresent HandoverCondition = iota
	// HandoverMissing: the copy is absent.
	HandoverMissing
	// HandoverRenameLost: the copy is absent and still under .incomplete
	// with the published identity; it is never renamed.
	HandoverRenameLost
	// HandoverReplaced: another object has the copy's name, or an object
	// has the .incomplete name beside the copy.
	HandoverReplaced
	// HandoverNotObservable: a name could not be described; nothing is
	// concluded.
	HandoverNotObservable
)

func (c HandoverCondition) String() string {
	return [...]string{"present", "missing", "rename-lost", "replaced", "not-observable"}[c]
}

// HandedOverCondition classifies the names of a handed-over restore. Each
// presence is Absent, New for a plain directory of the published identity,
// or Other for anything else, a link or a file included; known is false
// for a name whose type or identity could not be read.
func HandedOverCondition(published, incomplete Presence, publishedKnown, incompleteKnown bool) HandoverCondition {
	switch {
	case !publishedKnown:
		return HandoverNotObservable
	case published != Absent && published != New:
		return HandoverReplaced
	case !incompleteKnown:
		return HandoverNotObservable
	case published == New && incomplete == Absent:
		return HandoverPresent
	case published == New:
		return HandoverReplaced
	case incomplete == New:
		return HandoverRenameLost
	}
	return HandoverMissing
}
