package backup_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

const op = "20261105T101500-1a2b3c4d"

var at = time.Date(2026, 11, 5, 10, 15, 0, 0, time.UTC)

// journalFiles commits steps in order, as the protocol would, and returns
// the directory listing and contents of journal\<op>.
func journalFiles(t *testing.T, rotation bool, steps ...backup.Step) ([]string, map[string][]byte) {
	t.Helper()
	var names []string
	contents := map[string][]byte{}
	prev := ""
	for i, step := range steps {
		fields := map[string]json.RawMessage{}
		if step == backup.StepPlanned {
			previous := json.RawMessage("null")
			if rotation {
				previous = json.RawMessage(`"20261020T080000-0badc0de"`)
			}
			fields["previous"] = previous
		}
		data, err := backup.EncodeRecord(backup.Record{Seq: i + 1, Step: step, Op: op, At: at.Add(time.Duration(i) * time.Second), Prev: prev, Fields: fields})
		if err != nil {
			t.Fatalf("encode %s: %v", step, err)
		}
		name := backup.RecordName(i+1, step)
		names = append(names, name)
		contents[name] = data
		sum := sha256.Sum256(data)
		prev = hex.EncodeToString(sum[:])
	}
	return names, contents
}

func journal(t *testing.T, rotation bool, steps ...backup.Step) backup.Journal {
	t.Helper()
	names, contents := journalFiles(t, rotation, steps...)
	j, err := backup.ParseJournal(op, names, contents)
	if err != nil {
		t.Fatalf("parse %v: %v", steps, err)
	}
	return j
}

var (
	pre       = []backup.Step{backup.StepPlanned, backup.StepStagingIntent, backup.StepStaged, backup.StepVerified}
	presences = []backup.Presence{backup.Absent, backup.Old, backup.New, backup.Unidentified, backup.Other}
	a, o, n   = backup.Absent, backup.Old, backup.New
	u         = backup.Unidentified
)

func obs(c, r, s backup.Presence) backup.Observation {
	return backup.Observation{Current: c, Retained: r, Staging: s}
}

func steps(prefix []backup.Step, more ...backup.Step) []backup.Step {
	return append(append([]backup.Step{}, prefix...), more...)
}

// The recovery tables of ADR 0006 §4 and §5, row by row. For each journal,
// every observation not listed must fail closed: the tables are closed.
func TestRecoveryTablesAreClosed(t *testing.T) {
	type row = map[backup.Observation]backup.Action
	cases := []struct {
		name     string
		rotation bool
		steps    []backup.Step
		rows     row
	}{
		{"not started", false, nil, row{obs(a, a, a): backup.ActionAbandon, obs(u, a, a): backup.ActionAbandon, obs(o, a, a): backup.ActionAbandon}},
		{"first planned", false, pre[:1], row{obs(a, a, a): backup.ActionAbandon}},
		{"first staging-intent", false, pre[:2], row{obs(a, a, a): backup.ActionAbandon, obs(a, a, u): backup.ActionAbandon}},
		{"first staged", false, pre[:3], row{obs(a, a, n): backup.ActionAbandon}},
		{"first verified", false, pre, row{obs(a, a, n): backup.ActionAbandon}},
		{"first publish-intent", false, steps(pre, backup.StepPublishIntent), row{
			obs(a, a, n): backup.ActionMarkRolledBack, obs(n, a, a): backup.ActionVerifyThenPublish}},
		{"first unpublish-intent", false, steps(pre, backup.StepPublishIntent, backup.StepUnpublishIntent), row{
			obs(n, a, a): backup.ActionFinishUnpublish, obs(a, a, n): backup.ActionMarkRolledBack}},
		{"first rollback-failed", false, steps(pre, backup.StepPublishIntent, backup.StepUnpublishIntent, backup.StepRollbackFailed), row{
			obs(n, a, a): backup.ActionFinishUnpublish, obs(a, a, n): backup.ActionMarkRolledBack}},
		{"rotation planned", true, pre[:1], row{obs(o, a, a): backup.ActionAbandon}},
		{"rotation staging-intent", true, pre[:2], row{obs(o, a, a): backup.ActionAbandon, obs(o, a, u): backup.ActionAbandon}},
		{"rotation verified", true, pre, row{obs(o, a, n): backup.ActionAbandon}},
		{"rotation retire-intent", true, steps(pre, backup.StepRetireIntent), row{
			obs(o, a, n): backup.ActionMarkRolledBack, obs(a, o, n): backup.ActionRestore}},
		{"rotation promote-intent", true, steps(pre, backup.StepRetireIntent, backup.StepPromoteIntent), row{
			obs(a, o, n): backup.ActionRestore, obs(n, o, a): backup.ActionVerifyThenPublish}},
		{"rotation unpromote-intent", true, steps(pre, backup.StepRetireIntent, backup.StepPromoteIntent, backup.StepUnpromoteIntent), row{
			obs(n, o, a): backup.ActionFinishUnpromote, obs(a, o, n): backup.ActionRestore}},
		{"rotation unpromote failed", true, steps(pre, backup.StepRetireIntent, backup.StepPromoteIntent, backup.StepUnpromoteIntent, backup.StepRollbackFailed), row{
			obs(n, o, a): backup.ActionFinishUnpromote, obs(a, o, n): backup.ActionRestore}},
		{"rotation restore-intent", true, steps(pre, backup.StepRetireIntent, backup.StepPromoteIntent, backup.StepRestoreIntent), row{
			obs(a, o, n): backup.ActionFinishRestore, obs(o, a, n): backup.ActionMarkRolledBack}},
		{"rotation restore failed", true, steps(pre, backup.StepRetireIntent, backup.StepRestoreIntent, backup.StepRollbackFailed), row{
			obs(a, o, n): backup.ActionFinishRestore, obs(o, a, n): backup.ActionMarkRolledBack}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			j := journal(t, tt.rotation, tt.steps...)
			for _, c := range presences {
				for _, r := range presences {
					for _, s := range presences {
						got, err := backup.Classify(j, obs(c, r, s))
						want, listed := tt.rows[obs(c, r, s)]
						switch {
						case listed && (err != nil || got != want):
							t.Errorf("%v: got %q, %v; want %q", obs(c, r, s), got, err, want)
						case !listed && err == nil:
							t.Errorf("%v: got %q; an unlisted observation must fail closed", obs(c, r, s), got)
						case !listed:
							var ambiguity *backup.AmbiguityError
							if !errors.As(err, &ambiguity) {
								t.Errorf("%v: error %v is not an AmbiguityError", obs(c, r, s), err)
							}
						}
					}
				}
			}
		})
	}
}

func TestTerminalOperationsNeedNoRecovery(t *testing.T) {
	for _, tail := range [][]backup.Step{
		{backup.StepAbandoned},
		steps(pre[:2], backup.StepFailed),
		steps(pre, backup.StepPublishIntent, backup.StepPublished),
		steps(pre, backup.StepPublishIntent, backup.StepRolledBack),
		steps(pre, backup.StepRetireIntent, backup.StepPromoteIntent, backup.StepPublished),
	} {
		j := journal(t, false, tail...)
		if action, err := backup.Classify(j, obs(backup.Other, backup.Other, backup.Other)); err != nil || action != backup.ActionNone {
			t.Errorf("%v: %q, %v", tail, action, err)
		}
	}
}

// Envelope E1: nine temporaries of the record that recovery must commit
// exhaust the envelope; eight still leave one attempt.
func TestRecoveryEnvelopeE1(t *testing.T) {
	for _, temporaries := range []int{8, 9} {
		names, contents := journalFiles(t, false, steps(pre, backup.StepPublishIntent)...)
		for k := 1; k <= temporaries; k++ {
			names = append(names, fmt.Sprintf("%s.tmp-%d", backup.RecordName(6, backup.StepPublished), k))
		}
		j, err := backup.ParseJournal(op, names, contents)
		if err != nil {
			t.Fatal(err)
		}
		action, err := backup.Classify(j, obs(n, a, a))
		if temporaries == 8 && (err != nil || action != backup.ActionVerifyThenPublish) {
			t.Fatalf("8 temporaries: %q, %v", action, err)
		}
		if temporaries == 9 && !errors.Is(err, backup.ErrEnvelopeExhausted) {
			t.Fatalf("9 temporaries: %q, %v", action, err)
		}
		// Temporaries of another step at the same number do not consume
		// the attempts of the record recovery needs.
		if temporaries == 9 {
			if action, err := backup.Classify(j, obs(a, a, n)); err != nil || action != backup.ActionMarkRolledBack {
				t.Fatalf("other step: %q, %v", action, err)
			}
		}
	}
}

// Envelope E2 and E3: the longest path has 11 records, and a second
// rollback-failed for the same intent is refused.
func TestRecoveryEnvelopeE2AndE3(t *testing.T) {
	longest := steps(pre, backup.StepRetireIntent, backup.StepPromoteIntent, backup.StepUnpromoteIntent,
		backup.StepRollbackFailed, backup.StepRestoreIntent, backup.StepRollbackFailed, backup.StepRolledBack)
	if len(longest) != backup.MaxRecords {
		t.Fatalf("longest path has %d records, want %d", len(longest), backup.MaxRecords)
	}
	journal(t, true, longest...)
	for _, invalid := range [][]backup.Step{
		steps(pre, backup.StepPublishIntent, backup.StepUnpublishIntent, backup.StepRollbackFailed, backup.StepRollbackFailed),
		steps(pre, backup.StepRetireIntent, backup.StepRestoreIntent, backup.StepRollbackFailed, backup.StepRestoreIntent),
	} {
		names, contents := journalFiles(t, true, invalid...)
		var ambiguity *backup.AmbiguityError
		if _, err := backup.ParseJournal(op, names, contents); !errors.As(err, &ambiguity) {
			t.Errorf("%v: %v", invalid, err)
		}
	}
}
