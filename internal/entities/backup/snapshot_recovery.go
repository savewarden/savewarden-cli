package backup

import "fmt"

// SnapshotObservation is the state of the two names a snapshot operation
// moves (ADR 0006 §10): Published, snapshots\<op>, and Incomplete,
// snapshots\<op>.incomplete, each Absent, New (the identity staged
// recorded), Unidentified (a plain directory whose identity was not yet
// recorded) or Other; and whether a final receipt of the snapshot exists.
type SnapshotObservation struct {
	Published, Incomplete Presence
	Receipt               bool
}

func (o SnapshotObservation) String() string {
	s := fmt.Sprintf("(snapshot %s, incomplete %s)", o.Published, o.Incomplete)
	if o.Receipt {
		s += " with a committed snapshot receipt"
	}
	return s
}

// ActionConfirmReceipt: the snapshot receipt is committed (A5, A6). Read it
// strictly against the plan and verify the snapshot fully; equal:
// published, without the provenance gate the receipt already passed;
// otherwise stop, nothing written or moved.
const ActionConfirmReceipt Action = "confirm-receipt"

// ClassifySnapshot decides the recovery of a snapshot operation from its
// journal and the observed names, per the table of ADR 0006 §10. A
// committed snapshot receipt admits only publish-intent with the snapshot
// renamed, and published; any combination absent from the table is an
// AmbiguityError. An operation terminal in published needs no recovery,
// whatever is observed: its snapshot may only be degraded.
func ClassifySnapshot(j Journal, o SnapshotObservation) (Action, error) {
	if !j.Snapshot {
		return "", &AmbiguityError{Op: j.Op, Reason: "not the journal of a snapshot operation"}
	}
	last := j.Last()
	if o.Receipt && last != StepPublishIntent && last != StepPublished {
		return "", &AmbiguityError{Op: j.Op, Reason: fmt.Sprintf("a snapshot receipt exists after the step %q", displayStep(last))}
	}
	if j.Terminal() {
		return ActionNone, nil
	}
	action, ok := classifySnapshot(j, o)
	if !ok {
		return "", &AmbiguityError{Op: j.Op, Reason: fmt.Sprintf("observation %s does not match the last step %q", o, displayStep(last))}
	}
	for step, count := range j.Prepared {
		if count > 0 && !snapshotPreparedFor(step, j, o) {
			return "", &AmbiguityError{Op: j.Op, Reason: fmt.Sprintf("a temporary record %q was prepared for another state than %s", step, o)}
		}
	}
	if j.Attempts() >= MaxTemporaries || len(j.Records) >= MaxRecords {
		return "", &AmbiguityError{Op: j.Op, Reason: fmt.Sprintf("record %03d cannot be committed again", len(j.Records)+1), Err: ErrEnvelopeExhausted}
	}
	return action, nil
}

func classifySnapshot(j Journal, o SnapshotObservation) (Action, bool) {
	p, i := o.Published, o.Incomplete
	is := func(published, incomplete Presence) bool { return p == published && i == incomplete }
	last := j.Last()
	if last == StepRollbackFailed && len(j.Records) >= 2 {
		// The failed intent's rows apply again.
		last = j.Records[len(j.Records)-2].Step
	}
	switch last {
	case "":
		// Operation not started: it fixed neither plan nor identity.
		return ActionAbandon, is(Absent, Absent)
	case StepPlanned:
		return ActionAbandon, is(Absent, Absent)
	case StepStagingIntent:
		return ActionAbandon, is(Absent, Absent) || is(Absent, Unidentified)
	case StepStaged, StepVerified:
		return ActionAbandon, is(Absent, New)
	case StepPublishIntent:
		switch {
		case o.Receipt:
			return ActionConfirmReceipt, is(New, Absent)
		case is(Absent, New):
			return ActionMarkRolledBack, true
		case is(New, Absent):
			return ActionVerifyThenPublish, true
		}
	case StepUnpublishIntent:
		switch {
		case is(New, Absent):
			return ActionFinishUnpublish, true
		case is(Absent, New):
			return ActionMarkRolledBack, true
		}
	}
	return "", false
}

// snapshotPreparedFor reports whether a temporary of step, following the
// last committed record of j, could have been written in the observed
// state: the state before its own action.
func snapshotPreparedFor(step Step, j Journal, o SnapshotObservation) bool {
	is := func(published, incomplete Presence) bool {
		return o.Published == published && o.Incomplete == incomplete
	}
	if o.Receipt {
		// A5, A6: the receipt was written after the gate; only published
		// may follow it, never a rollback.
		return step == StepPublished && is(New, Absent)
	}
	switch step {
	case StepPlanned, StepStagingIntent:
		return is(Absent, Absent)
	case StepStaged:
		return is(Absent, Unidentified)
	case StepVerified, StepPublishIntent:
		return is(Absent, New)
	case StepFailed, StepAbandoned:
		return is(Absent, Absent) || is(Absent, Unidentified) || is(Absent, New)
	case StepPublished, StepUnpublishIntent:
		return is(New, Absent)
	case StepRolledBack:
		return is(Absent, New)
	case StepRollbackFailed:
		return j.Last() == StepUnpublishIntent && is(New, Absent)
	}
	return false
}

// SnapshotCondition is how an approved snapshot looks, for an operation
// whose journal is valid and terminal in published (ADR 0006 §10, ADR 0008).
type SnapshotCondition int

// Snapshot conditions. Every condition but SnapshotIntact degrades the
// snapshot: reported, never blocking, never repaired.
const (
	SnapshotIntact SnapshotCondition = iota
	// SnapshotMissing: the snapshot is absent.
	SnapshotMissing
	// SnapshotRenameLost: the snapshot is absent and its copy is still
	// under .incomplete with the published identity, as a power cut that
	// lost the rename would leave it; the copy is never renamed.
	SnapshotRenameLost
	// SnapshotModified: the snapshot has another identity or an
	// .incomplete beside it, or its receipt is missing, invalid or differs
	// from the plan, or its content differs from its receipt.
	SnapshotModified
)

func (c SnapshotCondition) String() string {
	return [...]string{"intact", "missing", "rename-lost", "modified"}[c]
}

// PublishedSnapshotCondition classifies the observation of a snapshot
// whose operation is terminal in published: receiptConforms reports that
// its receipt was read and checked against the journal; the content of
// the snapshot, when read, is the caller's to compare.
func PublishedSnapshotCondition(o SnapshotObservation, receiptConforms bool) SnapshotCondition {
	switch {
	case o.Published == Absent && o.Incomplete == New:
		return SnapshotRenameLost
	case o.Published == Absent:
		return SnapshotMissing
	case o.Published != New || o.Incomplete != Absent || !receiptConforms:
		return SnapshotModified
	}
	return SnapshotIntact
}
