package backup

import "fmt"

// Presence is what an observation finds under one name of a game
// directory: current, retained\<prev-op> or staging\<op>.
type Presence int

// Presences. Old is the identity of current recorded by retire-intent, New
// the identity of the staging recorded by staged. Unidentified is a plain
// directory whose identity was not yet recorded; Other is anything else: a
// different identity, a link, a file.
const (
	Absent Presence = iota
	Old
	New
	Unidentified
	Other
)

func (p Presence) String() string {
	return [...]string{"absent", "old", "new", "unidentified", "other"}[p]
}

// Observation is the state of the three names that publication moves.
type Observation struct {
	Current, Retained, Staging Presence
}

func (o Observation) String() string {
	return fmt.Sprintf("(current %s, retained %s, staging %s)", o.Current, o.Retained, o.Staging)
}

// Action is what --recover --apply does for one interrupted operation, per
// the recovery tables of ADR 0006 §4 and §5.
type Action string

// Recovery actions.
const (
	// ActionNone: the operation is terminal.
	ActionNone Action = "none"
	// ActionAbandon: commit abandoned; the staging stays in place.
	ActionAbandon Action = "abandon"
	// ActionMarkRolledBack: the intended move did not happen; commit
	// rolled-back, nothing to move.
	ActionMarkRolledBack Action = "mark-rolled-back"
	// ActionVerifyThenPublish: the staging was renamed to current. Verify
	// current fully against the plan; equal: receipt, then published;
	// different: unpublish-intent (first publication) or unpromote-intent
	// (rotation), then move it back.
	ActionVerifyThenPublish Action = "verify-then-publish"
	// ActionFinishUnpublish: unpublish-intent is committed and current is
	// still in place: move current back to staging, then rolled-back.
	ActionFinishUnpublish Action = "finish-unpublish"
	// ActionRestore: commit restore-intent, move retained back to current
	// (M3), then rolled-back.
	ActionRestore Action = "restore"
	// ActionFinishUnpromote: unpromote-intent is committed: move current
	// back to staging (M4), then restore-intent, M3 and rolled-back.
	ActionFinishUnpromote Action = "finish-unpromote"
	// ActionFinishRestore: restore-intent is committed: move retained back
	// to current (M3), then rolled-back.
	ActionFinishRestore Action = "finish-restore"
)

// nextWrite is the step each action commits first, for envelope E1.
var nextWrite = map[Action]Step{
	ActionAbandon:           StepAbandoned,
	ActionMarkRolledBack:    StepRolledBack,
	ActionVerifyThenPublish: StepPublished,
	ActionFinishUnpublish:   StepRolledBack,
	ActionRestore:           StepRestoreIntent,
	ActionFinishUnpromote:   StepRestoreIntent,
	ActionFinishRestore:     StepRolledBack,
}

// Classify decides the recovery of a non-terminal operation from its
// journal and the observed identities. Any combination absent from the
// tables of ADR 0006 is an AmbiguityError: nothing is guessed.
func Classify(j Journal, o Observation) (Action, error) {
	if j.Terminal() {
		return ActionNone, nil
	}
	action, ok := classify(j, o)
	if !ok {
		return "", &AmbiguityError{Op: j.Op, Reason: fmt.Sprintf("observation %s does not match the last step %q", o, displayStep(j.Last()))}
	}
	if j.Prepared[nextWrite[action]] >= MaxTemporaries || len(j.Records) >= MaxRecords {
		return "", &AmbiguityError{Op: j.Op, Reason: fmt.Sprintf("%s cannot be committed again", nextWrite[action]), Err: ErrEnvelopeExhausted}
	}
	return action, nil
}

func displayStep(s Step) Step {
	if s == "" {
		return "none"
	}
	return s
}

func classify(j Journal, o Observation) (Action, bool) {
	last := j.Last()
	if last == "" {
		// Operation not started (step I8): nothing but its own directory
		// exists. Whether a current exists is irrelevant, a staging is not.
		ok := o.Staging == Absent && o.Retained == Absent &&
			(o.Current == Absent || o.Current == Old || o.Current == Unidentified)
		return ActionAbandon, ok
	}
	if last == StepRollbackFailed {
		// The failed intent's rows apply again (ADR 0006).
		last = j.Records[len(j.Records)-2].Step
	}
	if !j.Rotation() {
		return classifyFirst(last, o)
	}
	return classifyRotation(last, o)
}

// classifyFirst applies the recovery table of a first publication: no
// current existed and no generation is retained.
func classifyFirst(last Step, o Observation) (Action, bool) {
	if o.Retained != Absent {
		return "", false
	}
	c, s := o.Current, o.Staging
	switch last {
	case StepPlanned:
		return ActionAbandon, c == Absent && s == Absent
	case StepStagingIntent:
		return ActionAbandon, c == Absent && (s == Absent || s == Unidentified)
	case StepStaged, StepVerified:
		return ActionAbandon, c == Absent && s == New
	case StepPublishIntent:
		switch {
		case c == Absent && s == New:
			return ActionMarkRolledBack, true
		case c == New && s == Absent:
			return ActionVerifyThenPublish, true
		}
	case StepUnpublishIntent:
		switch {
		case c == New && s == Absent:
			return ActionFinishUnpublish, true
		case c == Absent && s == New:
			return ActionMarkRolledBack, true
		}
	}
	return "", false
}

// classifyRotation applies the closed recovery table of a rotation.
func classifyRotation(last Step, o Observation) (Action, bool) {
	is := func(c, r, s Presence) bool { return o.Current == c && o.Retained == r && o.Staging == s }
	switch last {
	case StepPlanned:
		return ActionAbandon, is(Old, Absent, Absent)
	case StepStagingIntent:
		return ActionAbandon, is(Old, Absent, Absent) || is(Old, Absent, Unidentified)
	case StepStaged, StepVerified:
		return ActionAbandon, is(Old, Absent, New)
	case StepRetireIntent:
		switch {
		case is(Old, Absent, New):
			return ActionMarkRolledBack, true
		case is(Absent, Old, New):
			return ActionRestore, true
		}
	case StepPromoteIntent:
		switch {
		case is(Absent, Old, New):
			return ActionRestore, true
		case is(New, Old, Absent):
			return ActionVerifyThenPublish, true
		}
	case StepUnpromoteIntent:
		switch {
		case is(New, Old, Absent):
			return ActionFinishUnpromote, true
		case is(Absent, Old, New):
			return ActionRestore, true
		}
	case StepRestoreIntent:
		switch {
		case is(Absent, Old, New):
			return ActionFinishRestore, true
		case is(Old, Absent, New):
			return ActionMarkRolledBack, true
		}
	}
	return "", false
}
