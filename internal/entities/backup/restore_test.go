package backup_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

const restOp = "20261107T090000-9a8b7c6d"

var (
	restAt     = time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC)
	snapshotID = backup.FileID{Volume: 0x1a2b3c4d, Index: 7}
)

// restorePlan restores the snapshot of snapshotReceipt, which copied the
// generation published by op.
func restorePlan(t *testing.T) *backup.RestorePlan {
	t.Helper()
	p := testPlan(false)
	return &backup.RestorePlan{ToolVersion: "dev", GameID: p.GameID, GameName: p.GameName, StartedAt: restAt,
		Source: backup.SnapshotSource{Operation: op, ReceiptSHA256: sha256.Sum256(sourceReceipt(t))}, Sources: p.Sources}
}

// restoredSnapshot is the snapshot the operation restores: its id and the
// SHA-256 of its receipt file.
func restoredSnapshot(t *testing.T) backup.SnapshotSource {
	t.Helper()
	data, err := backup.EncodeSnapshotReceipt(snapshotReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	return backup.SnapshotSource{Operation: snapOp, ReceiptSHA256: sha256.Sum256(data)}
}

// restoreValues gives each step of a restore the values the protocol would
// record.
func restoreValues(t *testing.T, step backup.Step) backup.StepValues {
	t.Helper()
	plan := restorePlan(t)
	content, err := backup.ContentDigest(plan.Sources)
	if err != nil {
		t.Fatal(err)
	}
	switch step {
	case backup.StepPlanned:
		return backup.StepValues{RestoredSnapshot: restoredSnapshot(t), SnapshotIdentity: snapshotID, RestorePlan: plan}
	case backup.StepStaged:
		return backup.StepValues{Staging: stagingID, Digest: content}
	case backup.StepVerified:
		return backup.StepValues{Digest: content}
	case backup.StepPublishIntent, backup.StepUnpublishIntent:
		return backup.StepValues{Staging: stagingID}
	case backup.StepPublished:
		return backup.StepValues{Digest: sha256.Sum256([]byte("restore receipt"))}
	case backup.StepRollbackFailed, backup.StepFailed, backup.StepAbandoned:
		return backup.StepValues{Text: "synthetic " + string(step)}
	}
	return backup.StepValues{}
}

func restoreFiles(t *testing.T, change func(backup.Step, *backup.StepValues), steps ...backup.Step) ([]string, map[string][]byte) {
	t.Helper()
	var names []string
	contents := map[string][]byte{}
	prev := ""
	for i, step := range steps {
		values := restoreValues(t, step)
		if change != nil {
			change(step, &values)
		}
		r, err := backup.NewRestoreRecord(i+1, step, restOp, restAt.Add(time.Duration(i)*time.Second), prev, values)
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

func restoreJournal(t *testing.T, steps ...backup.Step) backup.Journal {
	t.Helper()
	names, contents := restoreFiles(t, nil, steps...)
	j, err := backup.ParseRestoreJournal(snapGame, restOp, names, contents)
	if err != nil {
		t.Fatalf("parse %v: %v", steps, err)
	}
	return j
}

func restoreReceipt(t *testing.T) backup.RestoreReceipt {
	t.Helper()
	plan := restorePlan(t)
	_, sum, err := plan.Encode()
	if err != nil {
		t.Fatal(err)
	}
	published := restAt.Add(5 * time.Second)
	r, err := plan.Receipt(restOp, sum, restoredSnapshot(t), restAt.Add(4*time.Second), &published, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRestorePlanRoundTrip(t *testing.T) {
	plan := restorePlan(t)
	data, sum, err := plan.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if sum != sha256.Sum256(data) {
		t.Fatal("plan_sha256 is not the SHA-256 of the plan")
	}
	got, err := backup.DecodeRestorePlan(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(&got, plan) {
		t.Fatalf("round trip differs:\n got %+v\nwant %+v", got, *plan)
	}
	for _, key := range []string{`"game"`, `"source"`, `"sources"`, `"started_at"`, `"tool_version"`} {
		if !bytes.Contains(data, []byte(key)) {
			t.Errorf("plan lacks %s: %s", key, data)
		}
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
	for name, tt := range map[string]struct {
		data []byte
		want string
	}{
		"unknown field":   {mutate(func(m map[string]any) { m["changes"] = nil }), "unknown field"},
		"missing source":  {mutate(func(m map[string]any) { delete(m, "source") }), "source: required"},
		"bad source":      {mutate(func(m map[string]any) { m["source"].(map[string]any)["operation"] = "op" }), "operation id"},
		"no file sources": {mutate(func(m map[string]any) { m["sources"] = []any{} }), "at least one source"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := backup.DecodeRestorePlan(tt.data); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want %q", err, tt.want)
			}
		})
	}
}

func TestRestoreReceiptRoundTrip(t *testing.T) {
	r := restoreReceipt(t)
	data, err := backup.EncodeRestoreReceipt(r)
	if err != nil {
		t.Fatal(err)
	}
	got, err := backup.DecodeRestoreReceipt(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, r) {
		t.Fatalf("round trip differs:\n got %+v\nwant %+v", got, r)
	}
	again, err := backup.EncodeRestoreReceipt(got)
	if err != nil || !bytes.Equal(again, data) {
		t.Fatalf("encoding is not canonical: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("{\n  \"schema\": \"savewarden.restore\",\n  \"schema_version\": 1,\n  \"restore\": \""+restOp+"\"")) {
		t.Errorf("unexpected head:\n%s", data)
	}
	// A receipt rebuilt by recovery has no publication date.
	rebuilt := r
	recovered := restAt.Add(time.Hour)
	rebuilt.PublishedAt, rebuilt.RecoveredAt, rebuilt.RecoveredBy = nil, &recovered, "dev"
	data, err = backup.EncodeRestoreReceipt(rebuilt)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := backup.DecodeRestoreReceipt(data); err != nil || !reflect.DeepEqual(got, rebuilt) {
		t.Fatalf("rebuilt receipt: %v", err)
	}
}

func TestRestoreReceiptIsStrict(t *testing.T) {
	data, err := backup.EncodeRestoreReceipt(restoreReceipt(t))
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
	snapshot := func(m map[string]any) map[string]any { return m["snapshot"].(map[string]any) }
	source := func(m map[string]any) map[string]any { return m["source"].(map[string]any) }
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"unknown field", mutate(func(m map[string]any) { m["previous"] = nil }), "unknown field"},
		{"snapshot receipt schema", mutate(func(m map[string]any) { m["schema"] = "savewarden.snapshot" }), "unsupported schema"},
		{"schema version 2", mutate(func(m map[string]any) { m["schema_version"] = 2 }), "unsupported schema"},
		{"missing snapshot", mutate(func(m map[string]any) { delete(m, "snapshot") }), "snapshot: required"},
		{"missing snapshot receipt", mutate(func(m map[string]any) { delete(snapshot(m), "receipt_sha256") }), "snapshot.receipt_sha256: required"},
		{"unknown snapshot field", mutate(func(m map[string]any) { snapshot(m)["operation"] = snapOp }), "unknown field"},
		{"missing source", mutate(func(m map[string]any) { delete(m, "source") }), "source: required"},
		{"restore is the snapshot", mutate(func(m map[string]any) { m["restore"] = snapOp }), "must differ"},
		{"snapshot is the source", mutate(func(m map[string]any) { snapshot(m)["id"] = op }), "must differ"},
		{"restore is the source", mutate(func(m map[string]any) { source(m)["operation"] = restOp }), "must differ"},
		{"bad restore id", mutate(func(m map[string]any) { m["restore"] = "restore-1" }), "operation id"},
		{"uppercase digest", mutate(func(m map[string]any) {
			snapshot(m)["receipt_sha256"] = strings.ToUpper(snapshot(m)["receipt_sha256"].(string))
		}), "lowercase hexadecimal"},
		{"no publication date", mutate(func(m map[string]any) { m["published_at"] = nil }), "published_at: required"},
		{"half rebuilt", mutate(func(m map[string]any) { m["recovered_at"] = formatTime(restAt.Add(time.Hour)) }), "both null or both set"},
		{"no source", mutate(func(m map[string]any) { m["sources"] = []any{} }), "at least one source"},
		{"duplicate key", bytes.Replace(data, []byte(`"tool_version": "dev",`), []byte(`"tool_version": "dev", "tool_version": "dev",`), 1), "duplicate key"},
		{"trailing data", append(append([]byte{}, data...), []byte("{}")...), "after the JSON value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := backup.DecodeRestoreReceipt(tt.data); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want %q", err, tt.want)
			}
		})
	}
	// A snapshot receipt is not a restore receipt, nor the reverse.
	snap, err := backup.EncodeSnapshotReceipt(snapshotReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backup.DecodeRestoreReceipt(snap); err == nil {
		t.Error("a snapshot receipt read as a restore receipt")
	}
	if _, err := backup.DecodeSnapshotReceipt(data); err == nil {
		t.Error("a restore receipt read as a snapshot receipt")
	}
}

func publishedRestore(t *testing.T) (backup.Journal, []byte) {
	t.Helper()
	data, err := backup.EncodeRestoreReceipt(restoreReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	names, contents := restoreFiles(t, func(step backup.Step, v *backup.StepValues) {
		if step == backup.StepPublished {
			v.Digest = sha256.Sum256(data)
		}
	}, steps(snapPre, backup.StepPublishIntent, backup.StepPublished)...)
	j, err := backup.ParseRestoreJournal(snapGame, restOp, names, contents)
	if err != nil {
		t.Fatal(err)
	}
	return j, data
}

func TestCheckRestoreReceipt(t *testing.T) {
	j, data := publishedRestore(t)
	if _, err := backup.CheckRestoreReceipt(data, j); err != nil {
		t.Fatalf("conforming receipt: %v", err)
	}
	// A5, A6: the same receipt beside publish-intent.
	if _, err := backup.CheckRestoreReceipt(data, restoreJournal(t, steps(snapPre, backup.StepPublishIntent)...)); err != nil {
		t.Fatalf("receipt before published: %v", err)
	}
	if _, err := backup.CheckRestoreReceipt(data, restoreJournal(t, snapPre...)); err == nil {
		t.Error("a receipt without publish-intent must be refused")
	}
	if _, err := backup.CheckRestoreReceipt(data, snapshotJournal(t, snapPre...)); err == nil {
		t.Error("a restore receipt checked against a snapshot journal")
	}
	tests := []struct {
		name   string
		change func(*backup.RestoreReceipt)
		want   string
	}{
		{"another restore", func(r *backup.RestoreReceipt) { r.Restore = "20261107T090000-00000000" }, "in the operation"},
		{"another tool", func(r *backup.RestoreReceipt) { r.ToolVersion = "v9" }, "differ from the plan"},
		{"another intent date", func(r *backup.RestoreReceipt) { r.PublishIntentAt = r.PublishIntentAt.Add(time.Second) }, "publish_intent_at"},
		{"another snapshot", func(r *backup.RestoreReceipt) { r.Snapshot.ReceiptSHA256 = sha256.Sum256([]byte("other")) }, "snapshot differs"},
		{"another source", func(r *backup.RestoreReceipt) { r.Source.Operation = previousOp }, "source differs"},
		{"another plan", func(r *backup.RestoreReceipt) { r.PlanSHA256 = sha256.Sum256([]byte("other")) }, "plan_sha256"},
		{"another content", func(r *backup.RestoreReceipt) {
			sources := append([]backup.ReceiptSource{}, r.Sources...)
			sources[0].Entries = []backup.Entry{{Path: "1.save", Size: 3, ModTime: at, SHA256: sha256.Sum256([]byte("two"))}}
			r.Sources = sources
		}, "sources differ"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := restoreReceipt(t)
			tt.change(&r)
			other, err := backup.EncodeRestoreReceipt(r)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := backup.CheckRestoreReceipt(other, j); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want %q", err, tt.want)
			}
		})
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, data, "", "\t"); err != nil {
		t.Fatal(err)
	}
	if _, err := backup.CheckRestoreReceipt(indented.Bytes(), j); err == nil || !strings.Contains(err.Error(), "receipt_sha256 of published") {
		t.Fatalf("bytes of another file: %v", err)
	}
}

func TestRestoreAreaMarkerIsStrict(t *testing.T) {
	m := backup.RestoreAreaMarker{CreatedAt: restAt, ToolVersion: "dev", GameID: snapGame}
	data, err := backup.EncodeRestoreAreaMarker(m)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"created_at":"2026-11-07T09:00:00Z","game":"synthetic-game","schema":"savewarden.restore-area","schema_version":1,"tool_version":"dev"}` + "\n"
	if string(data) != want {
		t.Fatalf("marker:\n got %s\nwant %s", data, want)
	}
	if got, err := backup.DecodeRestoreAreaMarker(data); err != nil || !reflect.DeepEqual(got, m) {
		t.Fatalf("round trip: %+v, %v", got, err)
	}
	for name, tt := range map[string]struct {
		data string
		want string
	}{
		"vault marker":   {`{"schema":"savewarden.vault","schema_version":1,"created_at":"2026-11-07T09:00:00Z","tool_version":"dev"}`, "unsupported schema"},
		"another schema": {strings.Replace(want, "restore-area", "vault", 1), "unsupported schema"},
		"version 2":      {strings.Replace(want, `"schema_version":1`, `"schema_version":2`, 1), "unsupported schema"},
		"missing game":   {strings.Replace(want, `"game":"synthetic-game",`, "", 1), "game: required"},
		"invalid game":   {strings.Replace(want, "synthetic-game", "Synthetic Game", 1), "lowercase letters"},
		"empty tool":     {strings.Replace(want, `"tool_version":"dev"`, `"tool_version":""`, 1), "tool_version"},
		"unknown field":  {strings.Replace(want, `"game"`, `"games":1,"game"`, 1), "unknown field"},
		"duplicate key":  {strings.Replace(want, `"game"`, `"game":"x","game"`, 1), "duplicate key"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := backup.DecodeRestoreAreaMarker([]byte(tt.data)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want %q", err, tt.want)
			}
		})
	}
	vault, err := backup.EncodeMarker(backup.Marker{CreatedAt: restAt, ToolVersion: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backup.DecodeMarker(data); err == nil {
		t.Error("an area marker read as a vault marker")
	}
	if _, err := backup.DecodeRestoreAreaMarker(vault); err == nil {
		t.Error("a vault marker read as an area marker")
	}
}

func TestRestoreStepFieldsAreStrict(t *testing.T) {
	build := func(step backup.Step) backup.Record {
		t.Helper()
		r, err := backup.NewRestoreRecord(1, step, restOp, restAt, "", restoreValues(t, step))
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		return r
	}
	for _, step := range []backup.Step{backup.StepRetireIntent, backup.StepPromoteIntent, backup.StepUnpromoteIntent, backup.StepRestoreIntent} {
		if _, err := backup.NewRestoreRecord(5, step, restOp, restAt, "", backup.StepValues{Current: currentID, Retained: currentID, Staging: stagingID}); err == nil {
			t.Errorf("%s: a rotation step must not exist for a restore", step)
		}
	}
	tests := []struct {
		name   string
		step   backup.Step
		change func(map[string]json.RawMessage)
		want   string
	}{
		{"source identity of a snapshot", backup.StepPlanned, func(f map[string]json.RawMessage) {
			f["source_identity"] = f["snapshot_identity"]
		}, `unknown field "source_identity"`},
		{"previous in planned", backup.StepPlanned, func(f map[string]json.RawMessage) { f["previous"] = json.RawMessage("null") }, `unknown field "previous"`},
		{"missing snapshot identity", backup.StepPlanned, func(f map[string]json.RawMessage) { delete(f, "snapshot_identity") }, `missing field "snapshot_identity"`},
		{"snapshot is the restore", backup.StepPlanned, func(f map[string]json.RawMessage) {
			f["snapshot"] = json.RawMessage(`"` + restOp + `"`)
		}, "another operation id"},
		{"plan digest", backup.StepPlanned, func(f map[string]json.RawMessage) {
			f["plan_sha256"] = json.RawMessage(`"` + strings.Repeat("0", 64) + `"`)
		}, "does not match the plan"},
		{"plan of a snapshot", backup.StepPlanned, func(f map[string]json.RawMessage) {
			plan, _, err := snapshotPlan().Encode()
			if err != nil {
				t.Fatal(err)
			}
			f["plan"] = plan
		}, "source: required"},
		{"staging of a snapshot", backup.StepStagingIntent, func(f map[string]json.RawMessage) {
			f["staging"] = json.RawMessage(`"snapshots/` + restOp + `.incomplete"`)
		}, restOp + ".incomplete"},
		{"unpublish names a current", backup.StepUnpublishIntent, func(f map[string]json.RawMessage) {
			f["current_identity"] = f["staging_identity"]
			delete(f, "staging_identity")
		}, `unknown field "current_identity"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := build(tt.step)
			tt.change(r.Fields)
			if _, err := r.RestoreValues(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want %q", err, tt.want)
			}
		})
	}
	both := restoreValues(t, backup.StepPlanned)
	both.SnapshotPlan = snapshotPlan()
	if _, err := backup.NewRestoreRecord(1, backup.StepPlanned, restOp, restAt, "", both); err == nil || !strings.Contains(err.Error(), "two plans") {
		t.Errorf("restore record with two plans: %v", err)
	}
	current := valuesFor(t, backup.StepPlanned, false)
	current.RestorePlan = restorePlan(t)
	if _, err := backup.NewRecord(1, backup.StepPlanned, op, at, "", current); err == nil || !strings.Contains(err.Error(), "two plans") {
		t.Errorf("record of current with a restore plan: %v", err)
	}
	snapshot := snapshotValues(t, backup.StepPlanned, sha256.Sum256(sourceReceipt(t)))
	snapshot.RestorePlan = restorePlan(t)
	if _, err := backup.NewSnapshotRecord(1, backup.StepPlanned, snapOp, snapAt, "", snapshot); err == nil || !strings.Contains(err.Error(), "two plans") {
		t.Errorf("snapshot record with a restore plan: %v", err)
	}
	// The three modes never read each other's records.
	planned := build(backup.StepPlanned)
	if _, err := planned.Values(); err == nil {
		t.Error("a restore planned record read as a record of current")
	}
	if _, err := planned.SnapshotValues(); err == nil {
		t.Error("a restore planned record read as a snapshot record")
	}
	snap, err := backup.NewSnapshotRecord(1, backup.StepPlanned, snapOp, snapAt, "", snapshotValues(t, backup.StepPlanned, sha256.Sum256(sourceReceipt(t))))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := snap.RestoreValues(); err == nil {
		t.Error("a snapshot planned record read as a restore record")
	}
}

func TestRestoreAutomaton(t *testing.T) {
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
		j := restoreJournal(t, path...)
		if !j.Restore || j.Snapshot || j.Rotation() || j.Plan != nil || j.SnapshotPlan != nil || j.Last() != path[len(path)-1] {
			t.Errorf("%v: restore %v snapshot %v rotation %v last %q", path, j.Restore, j.Snapshot, j.Rotation(), j.Last())
		}
	}
	j := restoreJournal(t, steps(snapPre, backup.StepPublishIntent, backup.StepPublished)...)
	identity, _, published := j.Published()
	if j.RestorePlan == nil || j.RestoredSnapshot != restoredSnapshot(t) || j.SnapshotIdentity != snapshotID || !published || identity != stagingID {
		t.Fatalf("planned values not kept: %+v", j)
	}
	forbidden := [][]backup.Step{
		{backup.StepPlanned, backup.StepPublished},
		steps(snapPre[:2], backup.StepPublishIntent),
		steps(snapPre, backup.StepPublishIntent, backup.StepPublished, backup.StepAbandoned),
		steps(snapPre, backup.StepUnpublishIntent),
	}
	for _, path := range forbidden {
		names, contents := restoreFiles(t, nil, path...)
		if _, err := backup.ParseRestoreJournal(snapGame, restOp, names, contents); !isAmbiguity(err) {
			t.Errorf("%v: %v, want an ambiguity", path, err)
		}
	}
	// Each mode refuses the journals of the others.
	names, contents := restoreFiles(t, nil, snapPre...)
	if _, err := backup.ParseJournal(restOp, names, contents); !isAmbiguity(err) {
		t.Errorf("a restore journal read as a journal of current: %v", err)
	}
	if _, err := backup.ParseSnapshotJournal(snapGame, restOp, names, contents); !isAmbiguity(err) {
		t.Errorf("a restore journal read as a snapshot journal: %v", err)
	}
	names, contents = snapshotFiles(t, nil, snapPre...)
	if _, err := backup.ParseRestoreJournal(snapGame, snapOp, names, contents); !isAmbiguity(err) {
		t.Errorf("a snapshot journal read as a restore journal: %v", err)
	}
	names, contents = journalFiles(t, false, pre...)
	if _, err := backup.ParseRestoreJournal(snapGame, op, names, contents); !isAmbiguity(err) {
		t.Errorf("a journal of current read as a restore journal: %v", err)
	}
}

func TestRestorePlanIsCoherent(t *testing.T) {
	other := backup.FileID{Volume: 0x1a2b3c4d, Index: 9}
	tests := []struct {
		name   string
		game   string
		change func(backup.Step, *backup.StepValues)
		steps  []backup.Step
		want   string
	}{
		{"area of another game", "another-game", nil, snapPre[:1], "restore area of game"},
		{"plan started elsewhere", snapGame, func(step backup.Step, v *backup.StepValues) {
			if step == backup.StepPlanned {
				v.RestorePlan.StartedAt = restAt.Add(-time.Minute)
			}
		}, snapPre[:1], "differs from the date of its record"},
		{"source is the snapshot", snapGame, func(step backup.Step, v *backup.StepValues) {
			if step == backup.StepPlanned {
				v.RestorePlan.Source.Operation = snapOp
			}
		}, snapPre[:1], "must differ"},
		{"source is the restore", snapGame, func(step backup.Step, v *backup.StepValues) {
			if step == backup.StepPlanned {
				v.RestorePlan.Source.Operation = restOp
			}
		}, snapPre[:1], "must differ"},
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
		{"publish another copy", snapGame, func(step backup.Step, v *backup.StepValues) {
			if step == backup.StepPublishIntent {
				v.Staging = other
			}
		}, steps(snapPre, backup.StepPublishIntent), "recorded by staged"},
		{"unpublish another copy", snapGame, func(step backup.Step, v *backup.StepValues) {
			if step == backup.StepUnpublishIntent {
				v.Staging = other
			}
		}, steps(snapPre, backup.StepPublishIntent, backup.StepUnpublishIntent), "recorded by staged"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			names, contents := restoreFiles(t, tt.change, tt.steps...)
			_, err := backup.ParseRestoreJournal(tt.game, restOp, names, contents)
			if !isAmbiguity(err) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want an ambiguity about %q", err, tt.want)
			}
		})
	}
}

// The recovery table of ADR 0006 §11, row by row. For each journal, every
// observation not listed must fail closed: the table is closed.
func TestRestoreRecoveryRows(t *testing.T) {
	type row = map[backup.RestoreObservation]backup.Action
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
			// A5, A6: the receipt is committed; it is confirmed, never undone.
			sobs(n, a, true): backup.ActionConfirmReceipt,
		}},
		{"unpublish-intent", unpublishing, row{
			sobs(n, a, false): backup.ActionFinishUnpublish, sobs(a, n, false): backup.ActionMarkRolledBack}},
		{"rollback-failed", steps(unpublishing, backup.StepRollbackFailed), row{
			sobs(n, a, false): backup.ActionFinishUnpublish, sobs(a, n, false): backup.ActionMarkRolledBack}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			j := restoreJournal(t, tt.steps...)
			for _, p := range presences {
				for _, i := range presences {
					for _, receipt := range []bool{false, true} {
						observed := sobs(p, i, receipt)
						got, err := backup.ClassifyRestore(j, observed)
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
	// Terminal operations need no recovery; a receipt beside a closed
	// operation other than published is an ambiguity.
	published := restoreJournal(t, steps(publishing, backup.StepPublished)...)
	for _, observed := range []backup.RestoreObservation{sobs(n, a, true), sobs(a, a, true), sobs(backup.Other, n, true)} {
		if action, err := backup.ClassifyRestore(published, observed); err != nil || action != backup.ActionNone {
			t.Errorf("published %v: %q, %v", observed, action, err)
		}
	}
	for _, path := range [][]backup.Step{{backup.StepAbandoned}, steps(snapPre, backup.StepFailed), steps(publishing, backup.StepRolledBack)} {
		j := restoreJournal(t, path...)
		if action, err := backup.ClassifyRestore(j, sobs(a, a, false)); err != nil || action != backup.ActionNone {
			t.Errorf("%v: %q, %v", path, action, err)
		}
		if _, err := backup.ClassifyRestore(j, sobs(a, a, true)); !isAmbiguity(err) {
			t.Errorf("%v with a receipt: %v, want an ambiguity", path, err)
		}
	}
	if _, err := backup.ClassifyRestore(snapshotJournal(t, snapPre...), sobs(a, n, false)); !isAmbiguity(err) {
		t.Errorf("a snapshot journal classified as a restore: %v", err)
	}
	if _, err := backup.ClassifySnapshot(restoreJournal(t, snapPre...), sobs(a, n, false)); !isAmbiguity(err) {
		t.Errorf("a restore journal classified as a snapshot: %v", err)
	}
}

func TestClassifyRestoreRoot(t *testing.T) {
	entry := func(name string, kind backup.EntryKind) backup.NamedEntry {
		return backup.NamedEntry{Name: name, Kind: kind}
	}
	file, dir, other := backup.KindRegular, backup.KindDirectory, backup.KindOther
	marker := entry(backup.RestoreAreaMarkerName, file)
	if got := backup.ClassifyRestoreRoot(nil); got.State != backup.RestoreRootEmpty {
		t.Errorf("empty: %v", got.State)
	}
	interrupted := backup.ClassifyRestoreRoot([]backup.NamedEntry{entry(backup.RestoreAreaMarkerName+".tmp-1", file), entry(backup.RestoreAreaMarkerName+".tmp-3", file)})
	if interrupted.State != backup.RestoreRootInterrupted || interrupted.Temporaries != 2 || !interrupted.Taken[1] || !interrupted.Taken[3] {
		t.Errorf("interrupted: %+v", interrupted)
	}
	for name, entries := range map[string][]backup.NamedEntry{
		"user file":                 {entry("notes.txt", file)},
		"temporaries and a file":    {entry(backup.RestoreAreaMarkerName+".tmp-1", file), entry("notes.txt", file)},
		"lock without marker":       {entry(backup.RestoreLockName, file)},
		"journal without marker":    {entry(backup.RestoreJournalDir, dir)},
		"marker that is not a file": {entry(backup.RestoreAreaMarkerName, dir)},
		"marker that is a link":     {entry(backup.RestoreAreaMarkerName, other)},
		"vault marker":              {entry(backup.MarkerName, file)},
	} {
		if got := backup.ClassifyRestoreRoot(entries); got.State != backup.RestoreRootForeign {
			t.Errorf("%s: %v, want foreign", name, got.State)
		}
	}
	upper := "20261107T090000-9A8B7C6D"
	marked := backup.ClassifyRestoreRoot([]backup.NamedEntry{
		marker, entry(backup.RestoreAreaMarkerName+".tmp-2", file), entry(backup.RestoreLockName, file),
		entry(backup.RestoreReceiptsDir, dir), entry(backup.RestoreJournalDir, dir),
		entry(upper, dir), entry(restOp+".INCOMPLETE", other), entry(snapOp, file),
		entry("notes.txt", file), entry("20261107T250000-9a8b7c6d", dir),
	})
	if marked.State != backup.RestoreRootMarked || marked.Temporaries != 1 || !marked.Lock || !marked.Receipts || !marked.Journal {
		t.Fatalf("marked: %+v", marked)
	}
	if got := marked.Published[restOp]; got.Name != upper || got.Kind != dir {
		t.Errorf("published copy %+v: a case variant names the same operation", got)
	}
	if got := marked.Incomplete[restOp]; got.Kind != other {
		t.Errorf("incomplete copy %+v: any kind is kept for the caller", got)
	}
	if got := marked.Published[snapOp]; got.Kind != file {
		t.Errorf("an operation name of any kind is kept: %+v", got)
	}
	if want := []string{"20261107T250000-9a8b7c6d", "notes.txt"}; !reflect.DeepEqual(marked.Unexpected, want) {
		t.Errorf("unexpected %v, want %v", marked.Unexpected, want)
	}
	// Structure entries of the wrong kind have no place in the area.
	wrong := backup.ClassifyRestoreRoot([]backup.NamedEntry{marker, entry(backup.RestoreLockName, dir), entry(backup.RestoreJournalDir, file), entry(backup.RestoreReceiptsDir, other)})
	if wrong.Lock || wrong.Journal || wrong.Receipts || len(wrong.Unexpected) != 3 {
		t.Errorf("wrong kinds: %+v", wrong)
	}
	// Two names of one operation, as a case-sensitive directory could hold.
	twice := backup.ClassifyRestoreRoot([]backup.NamedEntry{marker, entry(restOp, dir), entry(upper, dir)})
	if _, kept := twice.Published[restOp]; kept || len(twice.Unexpected) != 2 {
		t.Errorf("twice: %+v", twice)
	}
}

func TestCanonicalOperationID(t *testing.T) {
	for name, want := range map[string]string{
		restOp:                     restOp,
		"20261107t090000-9A8B7C6D": restOp,
	} {
		if got, ok := backup.CanonicalOperationID(name); !ok || got != want {
			t.Errorf("%s: %q, %v", name, got, ok)
		}
	}
	for _, name := range []string{"", restOp + ".incomplete", "20261307T090000-9a8b7c6d", "20261107T090000-9a8b7c6", "x0261107T090000-9a8b7c6d"} {
		if got, ok := backup.CanonicalOperationID(name); ok {
			t.Errorf("%q denotes %q", name, got)
		}
	}
}

func TestHandedOverCondition(t *testing.T) {
	other := backup.Other
	tests := []struct {
		published, incomplete    backup.Presence
		publishedKnown, incKnown bool
		want                     backup.HandoverCondition
	}{
		{n, a, true, true, backup.HandoverPresent},
		{a, a, true, true, backup.HandoverMissing},
		{a, other, true, true, backup.HandoverMissing},
		{a, u, true, true, backup.HandoverMissing},
		{a, n, true, true, backup.HandoverRenameLost},
		{other, a, true, true, backup.HandoverReplaced},
		{other, n, true, false, backup.HandoverReplaced},
		{u, a, true, true, backup.HandoverReplaced},
		{n, n, true, true, backup.HandoverReplaced},
		{n, other, true, true, backup.HandoverReplaced},
		{a, a, false, true, backup.HandoverNotObservable},
		{a, a, true, false, backup.HandoverNotObservable},
		{n, a, true, false, backup.HandoverNotObservable},
	}
	for _, tt := range tests {
		if got := backup.HandedOverCondition(tt.published, tt.incomplete, tt.publishedKnown, tt.incKnown); got != tt.want {
			t.Errorf("(%v %v, known %v %v): %v, want %v", tt.published, tt.incomplete, tt.publishedKnown, tt.incKnown, got, tt.want)
		}
	}
}
