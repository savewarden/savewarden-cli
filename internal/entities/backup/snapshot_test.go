package backup_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

const (
	snapOp   = "20261106T080000-5e6f7a8b"
	snapGame = "synthetic-game"
)

var (
	snapAt    = time.Date(2026, 11, 6, 8, 0, 0, 0, time.UTC)
	currentID = backup.FileID{Volume: 0x1a2b3c4d, Index: 4}
	snapPre   = []backup.Step{backup.StepPlanned, backup.StepStagingIntent, backup.StepStaged, backup.StepVerified}
)

// snapshotPlan copies the generation of testPlan, published by op.
func snapshotPlan() *backup.SnapshotPlan {
	p := testPlan(false)
	return &backup.SnapshotPlan{ToolVersion: "dev", GameID: p.GameID, GameName: p.GameName, StartedAt: snapAt, Sources: p.Sources}
}

// sourceReceipt is the receipt file of the generation the snapshot copies.
func sourceReceipt(t *testing.T) []byte {
	t.Helper()
	plan := testPlan(false)
	_, sum, err := plan.Encode()
	if err != nil {
		t.Fatal(err)
	}
	published := at.Add(4 * time.Second)
	r, err := plan.Receipt(op, sum, at.Add(3*time.Second), &published, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	data, err := backup.EncodeReceipt(r)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// snapshotValues gives each step of a snapshot the values the protocol
// would record; source is the SHA-256 of the receipt file it copies.
func snapshotValues(t *testing.T, step backup.Step, source [32]byte) backup.StepValues {
	t.Helper()
	plan := snapshotPlan()
	content, err := backup.ContentDigest(plan.Sources)
	if err != nil {
		t.Fatal(err)
	}
	switch step {
	case backup.StepPlanned:
		return backup.StepValues{SnapshotSource: backup.SnapshotSource{Operation: op, ReceiptSHA256: source},
			SourceIdentity: currentID, SnapshotPlan: plan}
	case backup.StepStaged:
		return backup.StepValues{Staging: stagingID, Digest: content}
	case backup.StepVerified:
		return backup.StepValues{Digest: content}
	case backup.StepPublishIntent, backup.StepUnpublishIntent:
		return backup.StepValues{Staging: stagingID}
	case backup.StepPublished:
		return backup.StepValues{Digest: sha256.Sum256([]byte("snapshot receipt"))}
	case backup.StepRollbackFailed, backup.StepFailed, backup.StepAbandoned:
		return backup.StepValues{Text: "synthetic " + string(step)}
	}
	return backup.StepValues{}
}

// snapshotFiles commits steps of the snapshot operation in order, with the
// values change may alter, and returns the listing and contents of
// snapshots\journal\<op>.
func snapshotFiles(t *testing.T, change func(backup.Step, *backup.StepValues), steps ...backup.Step) ([]string, map[string][]byte) {
	t.Helper()
	source := sha256.Sum256(sourceReceipt(t))
	var names []string
	contents := map[string][]byte{}
	prev := ""
	for i, step := range steps {
		values := snapshotValues(t, step, source)
		if change != nil {
			change(step, &values)
		}
		r, err := backup.NewSnapshotRecord(i+1, step, snapOp, snapAt.Add(time.Duration(i)*time.Second), prev, values)
		if err != nil {
			t.Fatalf("record %s: %v", step, err)
		}
		data := mustEncode(t, r)
		name := backup.RecordName(i+1, step)
		names = append(names, name)
		contents[name] = data
		sum := sha256.Sum256(data)
		prev = hex.EncodeToString(sum[:])
	}
	return names, contents
}

func snapshotJournal(t *testing.T, steps ...backup.Step) backup.Journal {
	t.Helper()
	names, contents := snapshotFiles(t, nil, steps...)
	j, err := backup.ParseSnapshotJournal(snapGame, snapOp, names, contents)
	if err != nil {
		t.Fatalf("parse %v: %v", steps, err)
	}
	return j
}

func isAmbiguity(err error) bool {
	var ambiguity *backup.AmbiguityError
	return errors.As(err, &ambiguity)
}

func snapshotReceipt(t *testing.T) backup.SnapshotReceipt {
	t.Helper()
	plan := snapshotPlan()
	_, sum, err := plan.Encode()
	if err != nil {
		t.Fatal(err)
	}
	published := snapAt.Add(5 * time.Second)
	source := backup.SnapshotSource{Operation: op, ReceiptSHA256: sha256.Sum256(sourceReceipt(t))}
	r, err := plan.Receipt(snapOp, sum, source, snapAt.Add(4*time.Second), &published, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSnapshotReceiptRoundTrip(t *testing.T) {
	r := snapshotReceipt(t)
	data, err := backup.EncodeSnapshotReceipt(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("{\n  \"schema\": \"savewarden.snapshot\",\n  \"schema_version\": 1,\n  \"snapshot\": \""+snapOp+"\",\n")) ||
		!bytes.Contains(data, []byte("\"source\": {\n    \"operation\": \""+op+"\",\n    \"receipt_sha256\": ")) ||
		!bytes.HasSuffix(data, []byte("}\n")) {
		t.Fatalf("layout:\n%s", data)
	}
	got, err := backup.DecodeSnapshotReceipt(data)
	if err != nil || !reflect.DeepEqual(got, r) {
		t.Fatalf("round trip: %v\n%+v\n%+v", err, got, r)
	}
	again, err := backup.EncodeSnapshotReceipt(got)
	if err != nil || !bytes.Equal(again, data) {
		t.Fatalf("encoding is not stable: %v", err)
	}
	recoveredAt := snapAt.Add(time.Hour)
	reconstructed, err := snapshotPlan().Receipt(snapOp, r.PlanSHA256, r.Source, r.PublishIntentAt, nil, &recoveredAt, "dev")
	if err != nil || !reconstructed.PublishIntentAt.Equal(r.PublishIntentAt) {
		t.Fatalf("reconstructed receipt: %v", err)
	}
}

func TestSnapshotReceiptIsStrict(t *testing.T) {
	data, err := backup.EncodeSnapshotReceipt(snapshotReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	mutate := func(change func(map[string]any)) []byte {
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}
		change(m)
		out, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	source := func(m map[string]any) map[string]any { return m["source"].(map[string]any) }
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"unknown field", mutate(func(m map[string]any) { m["previous"] = nil }), "unknown field"},
		{"receipt v1 schema", mutate(func(m map[string]any) { m["schema"] = "savewarden.receipt" }), "unsupported schema"},
		{"schema version 2", mutate(func(m map[string]any) { m["schema_version"] = 2 }), "unsupported schema"},
		{"missing source", mutate(func(m map[string]any) { delete(m, "source") }), "source: required"},
		{"missing source receipt", mutate(func(m map[string]any) { delete(source(m), "receipt_sha256") }), "source.receipt_sha256: required"},
		{"source is the snapshot", mutate(func(m map[string]any) { source(m)["operation"] = snapOp }), "must differ from snapshot"},
		{"uppercase digest", mutate(func(m map[string]any) {
			source(m)["receipt_sha256"] = strings.ToUpper(source(m)["receipt_sha256"].(string))
		}), "lowercase hexadecimal"},
		{"bad snapshot id", mutate(func(m map[string]any) { m["snapshot"] = "snapshot-1" }), "operation id"},
		{"no publication date", mutate(func(m map[string]any) { m["published_at"] = nil }), "published_at: required"},
		{"half reconstructed", mutate(func(m map[string]any) {
			m["recovered_at"] = formatTime(snapAt.Add(time.Hour))
		}), "both null or both set"},
		{"local date", mutate(func(m map[string]any) { m["started_at"] = "2026-11-06T09:00:00+01:00" }), "ending with Z"},
		{"no source", mutate(func(m map[string]any) { m["sources"] = []any{} }), "at least one source"},
		{"duplicate key", bytes.Replace(data, []byte(`"tool_version": "dev",`), []byte(`"tool_version": "dev", "tool_version": "dev",`), 1), "duplicate key"},
		{"trailing data", append(append([]byte{}, data...), []byte("{}")...), "after the JSON value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := backup.DecodeSnapshotReceipt(tt.data); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want %q", err, tt.want)
			}
		})
	}
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func TestSnapshotStepFieldsAreStrict(t *testing.T) {
	source := sha256.Sum256(sourceReceipt(t))
	build := func(step backup.Step) backup.Record {
		t.Helper()
		r, err := backup.NewSnapshotRecord(1, step, snapOp, snapAt, "", snapshotValues(t, step, source))
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		return r
	}
	for _, step := range []backup.Step{backup.StepRetireIntent, backup.StepPromoteIntent, backup.StepUnpromoteIntent, backup.StepRestoreIntent} {
		if _, err := backup.NewSnapshotRecord(5, step, snapOp, snapAt, "", backup.StepValues{Current: currentID, Retained: currentID, Staging: stagingID}); err == nil {
			t.Errorf("%s: a rotation step must not exist for a snapshot", step)
		}
	}
	tests := []struct {
		name   string
		step   backup.Step
		change func(map[string]json.RawMessage)
		want   string
	}{
		{"current identity in planned", backup.StepPlanned, func(f map[string]json.RawMessage) {
			f["current_identity"] = f["source_identity"]
		}, `unknown field "current_identity"`},
		{"previous in planned", backup.StepPlanned, func(f map[string]json.RawMessage) { f["previous"] = json.RawMessage("null") }, `unknown field "previous"`},
		{"missing source identity", backup.StepPlanned, func(f map[string]json.RawMessage) { delete(f, "source_identity") }, `missing field "source_identity"`},
		{"source is the snapshot", backup.StepPlanned, func(f map[string]json.RawMessage) {
			f["source_operation"] = json.RawMessage(`"` + snapOp + `"`)
		}, "another operation id"},
		{"plan digest", backup.StepPlanned, func(f map[string]json.RawMessage) {
			f["plan_sha256"] = json.RawMessage(`"` + strings.Repeat("0", 64) + `"`)
		}, "does not match the plan"},
		{"plan of a generation", backup.StepPlanned, func(f map[string]json.RawMessage) {
			plan, _, err := testPlan(false).Encode()
			if err != nil {
				t.Fatal(err)
			}
			f["plan"] = plan
		}, "unknown field"},
		{"staging of the current domain", backup.StepStagingIntent, func(f map[string]json.RawMessage) {
			f["staging"] = json.RawMessage(`"staging/` + snapOp + `"`)
		}, "snapshots/" + snapOp + ".incomplete"},
		{"unpublish names a current", backup.StepUnpublishIntent, func(f map[string]json.RawMessage) {
			f["current_identity"] = f["staging_identity"]
			delete(f, "staging_identity")
		}, `unknown field "current_identity"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := build(tt.step)
			tt.change(r.Fields)
			if _, err := r.SnapshotValues(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want %q", err, tt.want)
			}
		})
	}
	// Values never carry two plans, and plan_sha256 is that of the plan the
	// contract names.
	both := snapshotValues(t, backup.StepPlanned, source)
	both.Plan = testPlan(false)
	if _, err := backup.NewSnapshotRecord(1, backup.StepPlanned, snapOp, snapAt, "", both); err == nil || !strings.Contains(err.Error(), "two plans") {
		t.Errorf("snapshot record with two plans: %v", err)
	}
	both = valuesFor(t, backup.StepPlanned, false)
	both.SnapshotPlan = snapshotPlan()
	if _, err := backup.NewRecord(1, backup.StepPlanned, op, at, "", both); err == nil || !strings.Contains(err.Error(), "two plans") {
		t.Errorf("record of current with two plans: %v", err)
	}
	// The two domains never read each other's records.
	if _, err := build(backup.StepPlanned).Values(); err == nil {
		t.Error("a snapshot planned record read as a record of current")
	}
	current, err := backup.NewRecord(1, backup.StepPlanned, op, at, "", valuesFor(t, backup.StepPlanned, false))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := current.SnapshotValues(); err == nil {
		t.Error("a planned record of current read as a snapshot record")
	}
}

func TestSnapshotAutomaton(t *testing.T) {
	valid := [][]backup.Step{
		{backup.StepAbandoned},
		steps(snapPre[:2], backup.StepFailed),
		steps(snapPre, backup.StepFailed),
		steps(snapPre, backup.StepPublishIntent, backup.StepPublished),
		steps(snapPre, backup.StepPublishIntent, backup.StepRolledBack),
		steps(snapPre, backup.StepPublishIntent, backup.StepUnpublishIntent, backup.StepRolledBack),
		// The longest path: eight records, within E2.
		steps(snapPre, backup.StepPublishIntent, backup.StepUnpublishIntent, backup.StepRollbackFailed, backup.StepRolledBack),
	}
	for _, path := range valid {
		j := snapshotJournal(t, path...)
		if !j.Snapshot || j.Rotation() || j.Plan != nil || j.Last() != path[len(path)-1] {
			t.Errorf("%v: snapshot %v rotation %v plan %v last %q", path, j.Snapshot, j.Rotation(), j.Plan, j.Last())
		}
	}
	j := snapshotJournal(t, steps(snapPre, backup.StepPublishIntent, backup.StepPublished)...)
	identity, _, published := j.Published()
	if j.SnapshotPlan == nil || j.SnapshotSource.Operation != op || j.SourceIdentity != currentID || !published || identity != stagingID {
		t.Fatalf("planned values not kept: %+v", j)
	}
	forbidden := [][]backup.Step{
		{backup.StepPlanned, backup.StepPublished},
		steps(snapPre[:2], backup.StepPublishIntent),
		steps(snapPre, backup.StepPublishIntent, backup.StepPublished, backup.StepAbandoned),
		steps(snapPre, backup.StepUnpublishIntent),
	}
	for _, path := range forbidden {
		names, contents := snapshotFiles(t, nil, path...)
		if _, err := backup.ParseSnapshotJournal(snapGame, snapOp, names, contents); !isAmbiguity(err) {
			t.Errorf("%v: %v, want an ambiguity", path, err)
		}
	}
	// Each domain refuses the journals of the other.
	names, contents := snapshotFiles(t, nil, snapPre...)
	if _, err := backup.ParseJournal(snapOp, names, contents); !isAmbiguity(err) {
		t.Errorf("a snapshot journal read as a journal of current: %v", err)
	}
	names, contents = journalFiles(t, false, pre...)
	if _, err := backup.ParseSnapshotJournal(snapGame, op, names, contents); !isAmbiguity(err) {
		t.Errorf("a journal of current read as a snapshot journal: %v", err)
	}
}

func TestSnapshotPlanIsCoherent(t *testing.T) {
	other := backup.FileID{Volume: 0x1a2b3c4d, Index: 9}
	tests := []struct {
		name   string
		game   string
		change func(backup.Step, *backup.StepValues)
		steps  []backup.Step
		want   string
	}{
		{"game of another directory", "another-game", nil, snapPre[:1], "in the directory of game"},
		{"plan started elsewhere", snapGame, func(step backup.Step, v *backup.StepValues) {
			if step == backup.StepPlanned {
				v.SnapshotPlan.StartedAt = snapAt.Add(-time.Minute)
			}
		}, snapPre[:1], "differs from the date of its record"},
		{"copied digest", snapGame, func(step backup.Step, v *backup.StepValues) {
			if step == backup.StepStaged {
				v.Digest = sha256.Sum256([]byte("other"))
			}
		}, snapPre[:3], "staged digest"},
		{"verified digest", snapGame, func(step backup.Step, v *backup.StepValues) {
			if step == backup.StepVerified {
				v.Digest = sha256.Sum256([]byte("other"))
			}
		}, snapPre, "verified digest"},
		{"publish another staging", snapGame, func(step backup.Step, v *backup.StepValues) {
			if step == backup.StepPublishIntent {
				v.Staging = other
			}
		}, steps(snapPre, backup.StepPublishIntent), "recorded by staged"},
		{"unpublish another staging", snapGame, func(step backup.Step, v *backup.StepValues) {
			if step == backup.StepUnpublishIntent {
				v.Staging = other
			}
		}, steps(snapPre, backup.StepPublishIntent, backup.StepUnpublishIntent), "recorded by staged"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			names, contents := snapshotFiles(t, tt.change, tt.steps...)
			_, err := backup.ParseSnapshotJournal(tt.game, snapOp, names, contents)
			if !isAmbiguity(err) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want an ambiguity about %q", err, tt.want)
			}
		})
	}
}

func sobs(published, incomplete backup.Presence, receipt bool) backup.SnapshotObservation {
	return backup.SnapshotObservation{Published: published, Incomplete: incomplete, Receipt: receipt}
}

// The recovery table of ADR 0006 §10, row by row. For each journal, every
// observation not listed must fail closed: the table is closed.
func TestSnapshotRecoveryRows(t *testing.T) {
	type row = map[backup.SnapshotObservation]backup.Action
	publishing := steps(snapPre, backup.StepPublishIntent)
	unpublishing := steps(publishing, backup.StepUnpublishIntent)
	cases := []struct {
		name  string
		steps []backup.Step
		rows  row
	}{
		{"not started", nil, row{sobs(a, a, false): backup.ActionAbandon}},
		{"planned", snapPre[:1], row{sobs(a, a, false): backup.ActionAbandon}},
		{"staging-intent", snapPre[:2], row{sobs(a, a, false): backup.ActionAbandon, sobs(a, u, false): backup.ActionAbandon}},
		{"staged", snapPre[:3], row{sobs(a, n, false): backup.ActionAbandon}},
		{"verified", snapPre, row{sobs(a, n, false): backup.ActionAbandon}},
		{"publish-intent", publishing, row{
			sobs(a, n, false): backup.ActionMarkRolledBack,
			sobs(n, a, false): backup.ActionVerifyThenPublish,
			// A5, A6: the receipt passed the gate; it is confirmed, never undone.
			sobs(n, a, true): backup.ActionConfirmReceipt,
		}},
		{"unpublish-intent", unpublishing, row{
			sobs(n, a, false): backup.ActionFinishUnpublish, sobs(a, n, false): backup.ActionMarkRolledBack}},
		{"rollback-failed", steps(unpublishing, backup.StepRollbackFailed), row{
			sobs(n, a, false): backup.ActionFinishUnpublish, sobs(a, n, false): backup.ActionMarkRolledBack}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			j := snapshotJournal(t, tt.steps...)
			for _, p := range presences {
				for _, i := range presences {
					for _, receipt := range []bool{false, true} {
						observed := sobs(p, i, receipt)
						got, err := backup.ClassifySnapshot(j, observed)
						want, listed := tt.rows[observed]
						switch {
						case listed && (err != nil || got != want):
							t.Errorf("%v: got %q, %v; want %q", observed, got, err, want)
						case !listed && err == nil:
							t.Errorf("%v: got %q; an unlisted observation must fail closed", observed, got)
						case !listed && !isAmbiguity(err):
							t.Errorf("%v: error %v is not an AmbiguityError", observed, err)
						}
					}
				}
			}
		})
	}
}

func TestTerminalSnapshotOperations(t *testing.T) {
	published := snapshotJournal(t, steps(snapPre, backup.StepPublishIntent, backup.StepPublished)...)
	for _, observed := range []backup.SnapshotObservation{sobs(n, a, true), sobs(a, a, false), sobs(backup.Other, n, true)} {
		if action, err := backup.ClassifySnapshot(published, observed); err != nil || action != backup.ActionNone {
			t.Errorf("published %v: %q, %v", observed, action, err)
		}
	}
	for _, path := range [][]backup.Step{
		{backup.StepAbandoned},
		steps(snapPre, backup.StepFailed),
		steps(snapPre, backup.StepPublishIntent, backup.StepRolledBack),
	} {
		j := snapshotJournal(t, path...)
		if action, err := backup.ClassifySnapshot(j, sobs(a, n, false)); err != nil || action != backup.ActionNone {
			t.Errorf("%v: %q, %v", path, action, err)
		}
		// A receipt this operation cannot have validated is an orphan.
		if _, err := backup.ClassifySnapshot(j, sobs(a, n, true)); !isAmbiguity(err) {
			t.Errorf("%v with a receipt: %v, want an ambiguity", path, err)
		}
	}
	verified := snapshotJournal(t, snapPre...)
	if _, err := backup.ClassifySnapshot(verified, sobs(a, n, true)); !isAmbiguity(err) {
		t.Errorf("receipt before publish-intent: %v, want an ambiguity", err)
	}
	if _, err := backup.ClassifySnapshot(journal(t, false, pre...), sobs(a, n, false)); !isAmbiguity(err) {
		t.Errorf("journal of current classified as a snapshot: %v", err)
	}
}

func withSnapshotTemporaries(t *testing.T, path []backup.Step, prepared ...backup.Step) backup.Journal {
	t.Helper()
	names, contents := snapshotFiles(t, nil, path...)
	for k, step := range prepared {
		names = append(names, fmt.Sprintf("%s.tmp-%d", backup.RecordName(len(path)+1, step), k+1))
	}
	j, err := backup.ParseSnapshotJournal(snapGame, snapOp, names, contents)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

// A temporary was written before its action: the state it was prepared for
// must still be observed, and the envelope holds (ADR 0006 §3, §9).
func TestSnapshotPreparedTemporaries(t *testing.T) {
	publishing := steps(snapPre, backup.StepPublishIntent)
	tests := []struct {
		name     string
		path     []backup.Step
		prepared backup.Step
		observed backup.SnapshotObservation
		ok       bool
	}{
		{"publish prepared, not renamed", snapPre, backup.StepPublishIntent, sobs(a, n, false), true},
		// published is prepared only once the snapshot receipt is committed.
		{"published prepared without a receipt", publishing, backup.StepPublished, sobs(n, a, false), false},
		{"unpublish prepared after the rename", publishing, backup.StepUnpublishIntent, sobs(n, a, false), true},
		{"rolled-back prepared, not renamed", publishing, backup.StepRolledBack, sobs(a, n, false), true},
		{"rolled-back prepared, yet renamed", publishing, backup.StepRolledBack, sobs(n, a, false), false},
		{"published prepared, yet not renamed", publishing, backup.StepPublished, sobs(a, n, false), false},
		{"staged prepared, staging already identified", snapPre[:2], backup.StepStaged, sobs(a, a, false), false},
		// A5, A6: beside a committed receipt only published may be prepared.
		{"published prepared beside a receipt", publishing, backup.StepPublished, sobs(n, a, true), true},
		{"unpublish prepared beside a receipt", publishing, backup.StepUnpublishIntent, sobs(n, a, true), false},
		{"rolled-back prepared beside a receipt", publishing, backup.StepRolledBack, sobs(n, a, true), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := withSnapshotTemporaries(t, tt.path, tt.prepared)
			_, err := backup.ClassifySnapshot(j, tt.observed)
			if (err == nil) != tt.ok {
				t.Fatalf("got %v, want ok %v", err, tt.ok)
			}
		})
	}
	j := withSnapshotTemporaries(t, publishing, repeat(backup.StepPublished, backup.MaxTemporaries)...)
	if _, err := backup.ClassifySnapshot(j, sobs(n, a, true)); !errors.Is(err, backup.ErrEnvelopeExhausted) {
		t.Fatalf("E1: %v", err)
	}
}

func TestPublishedSnapshotCondition(t *testing.T) {
	other := backup.Other
	tests := []struct {
		observed backup.SnapshotObservation
		conforms bool
		want     backup.SnapshotCondition
	}{
		{sobs(n, a, true), true, backup.SnapshotIntact},
		{sobs(a, a, true), true, backup.SnapshotMissing},
		{sobs(a, other, true), true, backup.SnapshotMissing},
		{sobs(a, n, true), true, backup.SnapshotRenameLost},
		{sobs(other, a, true), true, backup.SnapshotModified},
		{sobs(n, other, true), true, backup.SnapshotModified},
		{sobs(n, n, true), true, backup.SnapshotModified},
		{sobs(n, a, false), false, backup.SnapshotModified},
		{sobs(n, a, true), false, backup.SnapshotModified},
	}
	for _, tt := range tests {
		if got := backup.PublishedSnapshotCondition(tt.observed, tt.conforms); got != tt.want {
			t.Errorf("%v, receipt conforms %v: %s, want %s", tt.observed, tt.conforms, got, tt.want)
		}
	}
}

// publishedSnapshot returns the journal of a published snapshot and the
// receipt file its published record names.
func publishedSnapshot(t *testing.T, change func(*backup.SnapshotReceipt)) (backup.Journal, []byte) {
	t.Helper()
	r := snapshotReceipt(t)
	if change != nil {
		change(&r)
	}
	data, err := backup.EncodeSnapshotReceipt(r)
	if err != nil {
		t.Fatal(err)
	}
	names, contents := snapshotFiles(t, func(step backup.Step, v *backup.StepValues) {
		if step == backup.StepPublished {
			v.Digest = sha256.Sum256(data)
		}
	}, steps(snapPre, backup.StepPublishIntent, backup.StepPublished)...)
	j, err := backup.ParseSnapshotJournal(snapGame, snapOp, names, contents)
	if err != nil {
		t.Fatal(err)
	}
	return j, data
}

func TestCheckSnapshotReceipt(t *testing.T) {
	j, data := publishedSnapshot(t, nil)
	if _, err := backup.CheckSnapshotReceipt(data, j); err != nil {
		t.Fatalf("conforming receipt: %v", err)
	}
	// A5, A6: the same receipt beside publish-intent.
	if _, err := backup.CheckSnapshotReceipt(data, snapshotJournal(t, steps(snapPre, backup.StepPublishIntent)...)); err != nil {
		t.Fatalf("receipt before published: %v", err)
	}
	if _, err := backup.CheckSnapshotReceipt(data, snapshotJournal(t, snapPre...)); err == nil {
		t.Error("a receipt without publish-intent must be refused")
	}
	tests := []struct {
		name   string
		change func(*backup.SnapshotReceipt)
		want   string
	}{
		{"another snapshot", func(r *backup.SnapshotReceipt) { r.Snapshot = "20261106T080000-00000000" }, "in the operation"},
		{"another tool", func(r *backup.SnapshotReceipt) { r.ToolVersion = "v9" }, "differ from the plan"},
		{"another intent date", func(r *backup.SnapshotReceipt) { r.PublishIntentAt = r.PublishIntentAt.Add(time.Second) }, "publish_intent_at"},
		{"another source", func(r *backup.SnapshotReceipt) { r.Source.Operation = previousOp }, "source differs"},
		{"another plan", func(r *backup.SnapshotReceipt) { r.PlanSHA256 = sha256.Sum256([]byte("other")) }, "plan_sha256"},
		{"another content", func(r *backup.SnapshotReceipt) {
			sources := append([]backup.ReceiptSource{}, r.Sources...)
			sources[0].Entries = []backup.Entry{{Path: "1.save", Size: 3, ModTime: at, SHA256: sha256.Sum256([]byte("two"))}}
			r.Sources = sources
		}, "sources differ"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := snapshotReceipt(t)
			tt.change(&r)
			other, err := backup.EncodeSnapshotReceipt(r)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := backup.CheckSnapshotReceipt(other, j); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want %q", err, tt.want)
			}
		})
	}
	// Reformatted, the same receipt is no longer the file published names.
	var indented bytes.Buffer
	if err := json.Indent(&indented, data, "", "\t"); err != nil {
		t.Fatal(err)
	}
	if _, err := backup.CheckSnapshotReceipt(indented.Bytes(), j); err == nil || !strings.Contains(err.Error(), "receipt_sha256 of published") {
		t.Fatalf("bytes of another file: %v", err)
	}
}

func TestCheckSourceReceipt(t *testing.T) {
	data := sourceReceipt(t)
	recording := func(file []byte) backup.Journal {
		t.Helper()
		names, contents := snapshotFiles(t, func(step backup.Step, v *backup.StepValues) {
			if step == backup.StepPlanned {
				v.SnapshotSource.ReceiptSHA256 = sha256.Sum256(file)
			}
		}, snapPre...)
		j, err := backup.ParseSnapshotJournal(snapGame, snapOp, names, contents)
		if err != nil {
			t.Fatal(err)
		}
		return j
	}
	j := recording(data)
	if err := backup.CheckSourceReceipt(data, snapGame, j); err != nil {
		t.Fatalf("conforming source receipt: %v", err)
	}
	receiptOf := func(change func(*backup.Receipt)) []byte {
		t.Helper()
		r, err := backup.DecodeReceipt(data)
		if err != nil {
			t.Fatal(err)
		}
		change(&r)
		out, err := backup.EncodeReceipt(r)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	anotherOp := receiptOf(func(r *backup.Receipt) { r.Operation = previousOp })
	anotherGame := receiptOf(func(r *backup.Receipt) { r.GameID = "another-game" })
	anotherContent := receiptOf(func(r *backup.Receipt) {
		r.Sources = append([]backup.ReceiptSource{}, r.Sources...)
		r.Sources[0].Entries = []backup.Entry{{Path: "1.save", Size: 3, ModTime: at, SHA256: sha256.Sum256([]byte("two"))}}
	})
	invalid := []byte("{}\n")
	tests := []struct {
		name string
		data []byte
		j    backup.Journal
		game string
		want string
	}{
		{"altered bytes", append(append([]byte{}, data...), ' '), j, snapGame, "differs from the one planned recorded"},
		{"another operation", anotherOp, recording(anotherOp), snapGame, "names the operation"},
		{"another game", anotherGame, recording(anotherGame), snapGame, "is of game"},
		{"directory of another game", data, j, "another-game", "is of game"},
		{"another content", anotherContent, recording(anotherContent), snapGame, "another content"},
		{"not a receipt", invalid, recording(invalid), snapGame, "required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := backup.CheckSourceReceipt(tt.data, tt.game, tt.j); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want %q", err, tt.want)
			}
		})
	}
}
