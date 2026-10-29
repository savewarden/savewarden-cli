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
	for _, tt := range []struct {
		rotation bool
		steps    []backup.Step
	}{
		{false, []backup.Step{backup.StepAbandoned}},
		{false, steps(pre[:2], backup.StepFailed)},
		{false, steps(pre, backup.StepPublishIntent, backup.StepPublished)},
		{false, steps(pre, backup.StepPublishIntent, backup.StepRolledBack)},
		{true, steps(pre, backup.StepRetireIntent, backup.StepPromoteIntent, backup.StepPublished)},
	} {
		j := journal(t, tt.rotation, tt.steps...)
		if action, err := backup.Classify(j, obs(backup.Other, backup.Other, backup.Other)); err != nil || action != backup.ActionNone {
			t.Errorf("%v: %q, %v", tt.steps, action, err)
		}
	}
}

// withTemporaries adds temporaries of the record following steps.
func withTemporaries(t *testing.T, rotation bool, steps []backup.Step, prepared ...backup.Step) backup.Journal {
	t.Helper()
	names, contents := journalFiles(t, rotation, steps...)
	attempt := map[backup.Step]int{}
	for _, step := range prepared {
		attempt[step]++
		names = append(names, fmt.Sprintf("%s.tmp-%d", backup.RecordName(len(steps)+1, step), attempt[step]))
	}
	j, err := backup.ParseJournal(op, names, contents)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func repeat(step backup.Step, n int) []backup.Step {
	out := make([]backup.Step, n)
	for i := range out {
		out[i] = step
	}
	return out
}

// A temporary was written before its action: the state it was prepared for
// must still be observed, otherwise recovery fails closed (ADR 0006 §3).
func TestPreparedTemporariesMustMatchTheObservation(t *testing.T) {
	publishing := steps(pre, backup.StepPublishIntent)
	retiring := steps(pre, backup.StepRetireIntent)
	tests := []struct {
		name     string
		rotation bool
		steps    []backup.Step
		prepared backup.Step
		obs      backup.Observation
		want     backup.Action // empty: ambiguity
	}{
		{"published prepared, current published", false, publishing, backup.StepPublished, obs(n, a, a), backup.ActionVerifyThenPublish},
		{"published prepared, staging still there", false, publishing, backup.StepPublished, obs(a, a, n), ""},
		{"unpublish prepared, rename never happened", false, publishing, backup.StepUnpublishIntent, obs(a, a, n), ""},
		{"rolled-back prepared, rename never happened", false, publishing, backup.StepRolledBack, obs(a, a, n), backup.ActionMarkRolledBack},
		{"restore prepared, retire done", true, retiring, backup.StepRestoreIntent, obs(a, o, n), backup.ActionRestore},
		{"restore prepared, retire never happened", true, retiring, backup.StepRestoreIntent, obs(o, a, n), ""},
		{"staged prepared during the copy", false, pre[:2], backup.StepStaged, obs(a, a, u), backup.ActionAbandon},
		{"staging-intent prepared, staging exists", false, pre[:1], backup.StepStagingIntent, obs(a, a, u), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := backup.Classify(withTemporaries(t, tt.rotation, tt.steps, tt.prepared), tt.obs)
			if tt.want == "" {
				var ambiguity *backup.AmbiguityError
				if !errors.As(err, &ambiguity) {
					t.Fatalf("got %q, %v; want an ambiguity", got, err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

// Envelope E1: nine temporaries of the next record, whatever their step,
// exhaust the envelope; eight still leave one attempt.
func TestRecoveryEnvelopeE1(t *testing.T) {
	publishing := steps(pre, backup.StepPublishIntent)
	if action, err := backup.Classify(withTemporaries(t, false, publishing, repeat(backup.StepPublished, 8)...), obs(n, a, a)); err != nil || action != backup.ActionVerifyThenPublish {
		t.Fatalf("8 temporaries: %q, %v", action, err)
	}
	for name, prepared := range map[string][]backup.Step{
		"nine of one step":  repeat(backup.StepPublished, 9),
		"nine of two steps": append(repeat(backup.StepPublished, 5), repeat(backup.StepUnpublishIntent, 4)...),
	} {
		if action, err := backup.Classify(withTemporaries(t, false, publishing, prepared...), obs(n, a, a)); !errors.Is(err, backup.ErrEnvelopeExhausted) {
			t.Fatalf("%s: %q, %v", name, action, err)
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
