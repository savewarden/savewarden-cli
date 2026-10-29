package backup_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

const op = "20261105T101500-1a2b3c4d"

var at = time.Date(2026, 11, 5, 10, 15, 0, 0, time.UTC)

const previousOp = "20261020T080000-0badc0de"

var (
	stagingID = backup.FileID{Volume: 0x1a2b3c4d, Index: 2}
	oldID     = backup.FileID{Volume: 0x1a2b3c4d, Index: 3}
)

// testPlan is the plan of the synthetic operation; a rotation names the
// generation it replaces.
func testPlan(rotation bool) *backup.Planned {
	p := &backup.Planned{
		ToolVersion: "dev", GameID: "synthetic-game", GameName: "Synthetic Game", StartedAt: at,
		Changes: backup.ChangeCounts{Added: 1},
		Sources: []backup.ReceiptSource{{
			ID: "saves", Kind: "save", Required: true, State: backup.SourceCopied, Path: `D:\Games\Synthetic Game\saves`,
			Directories: []string{}, Entries: []backup.Entry{{Path: "1.save", Size: 3, ModTime: at, SHA256: sha256.Sum256([]byte("one"))}},
		}},
	}
	if rotation {
		p.Previous = previousOp
	}
	return p
}

// valuesFor gives each step the values the protocol would record.
func valuesFor(t *testing.T, step backup.Step, rotation bool) backup.StepValues {
	t.Helper()
	plan := testPlan(rotation)
	content, err := backup.ContentDigest(plan.Sources)
	if err != nil {
		t.Fatal(err)
	}
	switch step {
	case backup.StepPlanned:
		values := backup.StepValues{Previous: plan.Previous, Plan: plan}
		if rotation {
			values.Current = oldID
		}
		return values
	case backup.StepStaged:
		return backup.StepValues{Staging: stagingID, Digest: content}
	case backup.StepVerified:
		return backup.StepValues{Digest: content}
	case backup.StepPublishIntent, backup.StepPromoteIntent:
		return backup.StepValues{Staging: stagingID}
	case backup.StepUnpublishIntent, backup.StepUnpromoteIntent:
		return backup.StepValues{Current: stagingID}
	case backup.StepRetireIntent:
		return backup.StepValues{Current: oldID}
	case backup.StepRestoreIntent:
		return backup.StepValues{Retained: oldID}
	case backup.StepPublished:
		return backup.StepValues{Digest: sha256.Sum256([]byte("receipt"))}
	case backup.StepRollbackFailed, backup.StepFailed, backup.StepAbandoned:
		return backup.StepValues{Text: "synthetic " + string(step)}
	}
	return backup.StepValues{}
}

func record(t *testing.T, seq int, step backup.Step, rotation bool, prev string, at time.Time) []byte {
	t.Helper()
	r, err := backup.NewRecord(seq, step, op, at, prev, valuesFor(t, step, rotation))
	if err != nil {
		t.Fatalf("record %s: %v", step, err)
	}
	return mustEncode(t, r)
}

// journalFiles commits steps in order, as the protocol would, and returns
// the directory listing and contents of journal\<op>.
func journalFiles(t *testing.T, rotation bool, steps ...backup.Step) ([]string, map[string][]byte) {
	t.Helper()
	var names []string
	contents := map[string][]byte{}
	prev := ""
	for i, step := range steps {
		data := record(t, i+1, step, rotation, prev, at.Add(time.Duration(i)*time.Second))
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

// expecting is obs for a game with a published generation expected.
func expecting(c, r, s backup.Presence) backup.Observation {
	o := obs(c, r, s)
	o.Expected = true
	return o
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
		expected bool // a published generation is expected (rule M3)
		steps    []backup.Step
		rows     row
	}{
		// Rule M3: without a generation expected, current must be absent;
		// with one, it must be the recognised previous generation.
		{"not started", false, false, nil, row{obs(a, a, a): backup.ActionAbandon}},
		{"not started beside a generation", false, true, nil, row{expecting(o, a, a): backup.ActionAbandon}},
		{"rotation planned, generation expected", true, true, pre[:1], row{expecting(o, a, a): backup.ActionAbandon}},
		{"first planned", false, false, pre[:1], row{obs(a, a, a): backup.ActionAbandon}},
		{"first staging-intent", false, false, pre[:2], row{obs(a, a, a): backup.ActionAbandon, obs(a, a, u): backup.ActionAbandon}},
		{"first staged", false, false, pre[:3], row{obs(a, a, n): backup.ActionAbandon}},
		{"first verified", false, false, pre, row{obs(a, a, n): backup.ActionAbandon}},
		{"first publish-intent", false, false, steps(pre, backup.StepPublishIntent), row{
			obs(a, a, n): backup.ActionMarkRolledBack, obs(n, a, a): backup.ActionVerifyThenPublish}},
		{"first unpublish-intent", false, false, steps(pre, backup.StepPublishIntent, backup.StepUnpublishIntent), row{
			obs(n, a, a): backup.ActionFinishUnpublish, obs(a, a, n): backup.ActionMarkRolledBack}},
		{"first rollback-failed", false, false, steps(pre, backup.StepPublishIntent, backup.StepUnpublishIntent, backup.StepRollbackFailed), row{
			obs(n, a, a): backup.ActionFinishUnpublish, obs(a, a, n): backup.ActionMarkRolledBack}},
		{"rotation planned", true, false, pre[:1], row{obs(o, a, a): backup.ActionAbandon}},
		{"rotation staging-intent", true, false, pre[:2], row{obs(o, a, a): backup.ActionAbandon, obs(o, a, u): backup.ActionAbandon}},
		{"rotation verified", true, false, pre, row{obs(o, a, n): backup.ActionAbandon}},
		{"rotation retire-intent", true, false, steps(pre, backup.StepRetireIntent), row{
			obs(o, a, n): backup.ActionMarkRolledBack, obs(a, o, n): backup.ActionRestore}},
		{"rotation promote-intent", true, false, steps(pre, backup.StepRetireIntent, backup.StepPromoteIntent), row{
			obs(a, o, n): backup.ActionRestore, obs(n, o, a): backup.ActionVerifyThenPublish}},
		{"rotation unpromote-intent", true, false, steps(pre, backup.StepRetireIntent, backup.StepPromoteIntent, backup.StepUnpromoteIntent), row{
			obs(n, o, a): backup.ActionFinishUnpromote, obs(a, o, n): backup.ActionRestore}},
		{"rotation unpromote failed", true, false, steps(pre, backup.StepRetireIntent, backup.StepPromoteIntent, backup.StepUnpromoteIntent, backup.StepRollbackFailed), row{
			obs(n, o, a): backup.ActionFinishUnpromote, obs(a, o, n): backup.ActionRestore}},
		{"rotation restore-intent", true, false, steps(pre, backup.StepRetireIntent, backup.StepPromoteIntent, backup.StepRestoreIntent), row{
			obs(a, o, n): backup.ActionFinishRestore, obs(o, a, n): backup.ActionMarkRolledBack}},
		{"rotation restore failed", true, false, steps(pre, backup.StepRetireIntent, backup.StepRestoreIntent, backup.StepRollbackFailed), row{
			obs(a, o, n): backup.ActionFinishRestore, obs(o, a, n): backup.ActionMarkRolledBack}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			j := journal(t, tt.rotation, tt.steps...)
			for _, c := range presences {
				for _, r := range presences {
					for _, s := range presences {
						observed := obs(c, r, s)
						observed.Expected = tt.expected
						got, err := backup.Classify(j, observed)
						want, listed := tt.rows[observed]
						switch {
						case listed && (err != nil || got != want):
							t.Errorf("%v: got %q, %v; want %q", observed, got, err, want)
						case !listed && err == nil:
							t.Errorf("%v: got %q; an unlisted observation must fail closed", observed, got)
						case !listed:
							var ambiguity *backup.AmbiguityError
							if !errors.As(err, &ambiguity) {
								t.Errorf("%v: error %v is not an AmbiguityError", observed, err)
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

// withTemporaries adds temporaries of the record following steps. As the
// writer does, each takes the next attempt number of that record, whatever
// its step.
func withTemporaries(t *testing.T, rotation bool, steps []backup.Step, prepared ...backup.Step) backup.Journal {
	t.Helper()
	names, contents := journalFiles(t, rotation, steps...)
	for k, step := range prepared {
		names = append(names, fmt.Sprintf("%s.tmp-%d", backup.RecordName(len(steps)+1, step), k+1))
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
		{"rollback-failed prepared after restore-intent", true, steps(retiring, backup.StepRestoreIntent), backup.StepRollbackFailed, obs(a, o, n), backup.ActionFinishRestore},
		{"rollback-failed prepared, M3 done", true, steps(retiring, backup.StepRestoreIntent), backup.StepRollbackFailed, obs(o, a, n), ""},
		{"rollback-failed prepared after unpromote-intent", true, steps(retiring, backup.StepPromoteIntent, backup.StepUnpromoteIntent), backup.StepRollbackFailed, obs(n, o, a), backup.ActionFinishUnpromote},
		{"rollback-failed prepared, M4 done", true, steps(retiring, backup.StepPromoteIntent, backup.StepUnpromoteIntent), backup.StepRollbackFailed, obs(a, o, n), ""},
		{"rollback-failed prepared after unpublish-intent", false, steps(publishing, backup.StepUnpublishIntent), backup.StepRollbackFailed, obs(n, a, a), backup.ActionFinishUnpublish},
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

// Abandoned temporaries of a committed number never consume the envelope of
// the next one, and do not reopen a closed operation.
func TestAbandonedTemporariesDoNotConsumeTheEnvelope(t *testing.T) {
	publishing := steps(pre, backup.StepPublishIntent)
	names, contents := journalFiles(t, false, publishing...)
	for k := 1; k < backup.MaxTemporaries; k++ {
		names = append(names, fmt.Sprintf("%s.tmp-%d", backup.RecordName(5, backup.StepPublishIntent), k))
	}
	for k := 1; k < backup.MaxTemporaries; k++ {
		names = append(names, fmt.Sprintf("%s.tmp-%d", backup.RecordName(6, backup.StepPublished), k))
	}
	j, err := backup.ParseJournal(op, names, contents)
	if err != nil {
		t.Fatal(err)
	}
	if action, err := backup.Classify(j, obs(n, a, a)); err != nil || action != backup.ActionVerifyThenPublish {
		t.Fatalf("eight prepared beside eight abandoned: %q, %v", action, err)
	}

	names, contents = journalFiles(t, false, steps(publishing, backup.StepPublished)...)
	names = append(names, backup.RecordName(5, backup.StepPublishIntent)+".tmp-1", backup.RecordName(6, backup.StepPublished)+".tmp-1")
	if j, err = backup.ParseJournal(op, names, contents); err != nil {
		t.Fatal(err)
	}
	if action, err := backup.Classify(j, obs(n, a, a)); err != nil || action != backup.ActionNone {
		t.Fatalf("published with abandoned temporaries: %q, %v", action, err)
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
