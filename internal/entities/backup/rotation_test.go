package backup_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

// A1: the planned record of a rotation fixes the identity of the current it
// replaces; the journal keeps it as Old from planned on.
func TestRotationPlannedCarriesCurrentIdentity(t *testing.T) {
	names, contents := journalFiles(t, true, backup.StepPlanned)
	r, err := backup.DecodeRecord(contents[names[0]])
	if err != nil {
		t.Fatal(err)
	}
	if got := string(r.Fields["current_identity"]); got != `"`+oldID.String()+`"` {
		t.Fatalf("current_identity %s", got)
	}
	for _, steps := range [][]backup.Step{pre[:1], pre, steps(pre, backup.StepRetireIntent)} {
		if j := journal(t, true, steps...); j.Old != oldID || !j.Rotation() {
			t.Fatalf("%v: old %s, rotation %v", steps, j.Old, j.Rotation())
		}
	}
}

// plannedWith commits a planned record whose fields are then changed, as a
// faulty writer would, and parses the journal.
func plannedWith(t *testing.T, rotation bool, change func(map[string]json.RawMessage)) error {
	t.Helper()
	r, err := backup.NewRecord(1, backup.StepPlanned, op, at, "", valuesFor(t, backup.StepPlanned, rotation))
	if err != nil {
		t.Fatal(err)
	}
	change(r.Fields)
	name := backup.RecordName(1, backup.StepPlanned)
	_, err = backup.ParseJournal(op, []string{name}, map[string][]byte{name: mustEncode(t, r)})
	return err
}

func TestRotationPlannedRequiresCurrentIdentity(t *testing.T) {
	err := plannedWith(t, true, func(f map[string]json.RawMessage) { delete(f, "current_identity") })
	var ambiguity *backup.AmbiguityError
	if !errors.As(err, &ambiguity) || !strings.Contains(err.Error(), `missing field "current_identity"`) {
		t.Fatalf("error = %v", err)
	}
	if _, err := backup.NewRecord(1, backup.StepPlanned, op, at, "", backup.StepValues{Previous: previousOp, Plan: testPlan(true)}); err == nil ||
		!strings.Contains(err.Error(), "current_identity") {
		t.Fatalf("a rotation planned without identity was written: %v", err)
	}
}

func TestFirstPublicationRefusesCurrentIdentity(t *testing.T) {
	err := plannedWith(t, false, func(f map[string]json.RawMessage) {
		f["current_identity"] = json.RawMessage(`"` + oldID.String() + `"`)
	})
	var ambiguity *backup.AmbiguityError
	if !errors.As(err, &ambiguity) || !strings.Contains(err.Error(), "forbidden for a first publication") {
		t.Fatalf("error = %v", err)
	}
	// The writer leaves it out: a first publication has nothing to replace.
	values := valuesFor(t, backup.StepPlanned, false)
	values.Current = oldID
	r, err := backup.NewRecord(1, backup.StepPlanned, op, at, "", values)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Fields["current_identity"]; ok {
		t.Fatal("first publication written with current_identity")
	}
}

// retire-intent and restore-intent repeat the identity of planned, never
// one observed later.
func TestRetireIntentKeepsThePlannedIdentity(t *testing.T) {
	other := backup.FileID{Volume: 0x1a2b3c4d, Index: 99}
	err := replace(t, true, steps(pre, backup.StepRetireIntent), backup.StepRetireIntent, func(v *backup.StepValues) { v.Current = other })
	var ambiguity *backup.AmbiguityError
	if !errors.As(err, &ambiguity) || !strings.Contains(err.Error(), "recorded by planned") {
		t.Fatalf("error = %v", err)
	}
}

func TestRestoreIntentKeepsThePlannedIdentity(t *testing.T) {
	other := backup.FileID{Volume: 0x1a2b3c4d, Index: 99}
	for _, path := range [][]backup.Step{
		steps(pre, backup.StepRetireIntent, backup.StepRestoreIntent),
		steps(pre, backup.StepRetireIntent, backup.StepPromoteIntent, backup.StepUnpromoteIntent, backup.StepRestoreIntent),
	} {
		err := replace(t, true, path, backup.StepRestoreIntent, func(v *backup.StepValues) { v.Retained = other })
		var ambiguity *backup.AmbiguityError
		if !errors.As(err, &ambiguity) || !strings.Contains(err.Error(), "recorded by planned") {
			t.Fatalf("%v: error = %v", path, err)
		}
	}
}

// b2bRecord encodes a record exactly as B2b did, its fields given as
// values: canonical JSON with sorted keys, then the SHA-256 trailer.
func b2bRecord(t *testing.T, seq int, step backup.Step, prev string, fields map[string]any) []byte {
	t.Helper()
	var prevValue any
	if prev != "" {
		prevValue = prev
	}
	line, err := json.Marshal(map[string]any{
		"at": "2026-11-05T10:15:00Z", "fields": fields, "op": op, "prev": prevValue,
		"schema": "savewarden.journal", "schema_version": 1, "seq": seq, "step": step,
	})
	if err != nil {
		t.Fatal(err)
	}
	line = append(line, '\n')
	sum := sha256.Sum256(line)
	return append(line, []byte("sha256 "+hex.EncodeToString(sum[:])+"\n")...)
}

// A first publication journal as B2b wrote it, without current_identity,
// still reads the same: no rotation, no old identity, its published
// directory and receipt digest.
func TestB2bJournalsStillParse(t *testing.T) {
	plan, planSum, err := testPlan(false).Encode()
	if err != nil {
		t.Fatal(err)
	}
	content, err := backup.ContentDigest(testPlan(false).Sources)
	if err != nil {
		t.Fatal(err)
	}
	receipt := sha256.Sum256([]byte("receipt"))
	records := []struct {
		step   backup.Step
		fields map[string]any
	}{
		{backup.StepPlanned, map[string]any{"plan": json.RawMessage(plan), "plan_sha256": hex.EncodeToString(planSum[:]), "previous": nil}},
		{backup.StepStagingIntent, map[string]any{"staging": "staging/" + op}},
		{backup.StepStaged, map[string]any{"copied_sha256": hex.EncodeToString(content[:]), "staging_identity": stagingID.String()}},
		{backup.StepVerified, map[string]any{"verified_sha256": hex.EncodeToString(content[:])}},
		{backup.StepPublishIntent, map[string]any{"staging_identity": stagingID.String()}},
		{backup.StepPublished, map[string]any{"receipt_sha256": hex.EncodeToString(receipt[:])}},
	}
	var names []string
	contents := map[string][]byte{}
	prev := ""
	for i, r := range records {
		data := b2bRecord(t, i+1, r.step, prev, r.fields)
		name := backup.RecordName(i+1, r.step)
		names, contents[name] = append(names, name), data
		sum := sha256.Sum256(data)
		prev = hex.EncodeToString(sum[:])
		// The record this build writes for the same values is byte for byte B2b's.
		values := valuesFor(t, r.step, false)
		values.Digest = map[backup.Step][32]byte{backup.StepStaged: content, backup.StepVerified: content, backup.StepPublished: receipt}[r.step]
		written, err := backup.NewRecord(i+1, r.step, op, at, recordPrev(t, contents, names[:i]), values)
		if err != nil {
			t.Fatal(err)
		}
		if got := mustEncode(t, written); string(got) != string(data) {
			t.Fatalf("%s written differently:\n%s\n%s", r.step, got, data)
		}
	}
	j, err := backup.ParseJournal(op, names, contents)
	if err != nil {
		t.Fatal(err)
	}
	identity, digest, ok := j.Published()
	if j.Rotation() || j.Old != (backup.FileID{}) || !ok || identity != stagingID || digest != receipt {
		t.Fatalf("journal %+v, published %s %x %v", j, identity, digest, ok)
	}
}

// recordPrev is the SHA-256 of the last of the named record files.
func recordPrev(t *testing.T, contents map[string][]byte, names []string) string {
	t.Helper()
	if len(names) == 0 {
		return ""
	}
	sum := sha256.Sum256(contents[names[len(names)-1]])
	return hex.EncodeToString(sum[:])
}

// The identity an operation published is that of its staging, as staged
// recorded it and publish-intent or promote-intent repeated it.
func TestPublishedIdentityOfAnOperation(t *testing.T) {
	receipt := sha256.Sum256([]byte("receipt"))
	for _, tt := range []struct {
		rotation bool
		steps    []backup.Step
		ok       bool
	}{
		{false, steps(pre, backup.StepPublishIntent, backup.StepPublished), true},
		{true, steps(pre, backup.StepRetireIntent, backup.StepPromoteIntent, backup.StepPublished), true},
		{false, steps(pre, backup.StepPublishIntent), false},
		{false, steps(pre, backup.StepPublishIntent, backup.StepRolledBack), false},
		{true, steps(pre, backup.StepRetireIntent, backup.StepRestoreIntent, backup.StepRolledBack), false},
	} {
		identity, digest, ok := journal(t, tt.rotation, tt.steps...).Published()
		if ok != tt.ok || (ok && (identity != stagingID || digest != receipt)) {
			t.Errorf("%v: %s %x %v", tt.steps, identity, digest, ok)
		}
	}
	other := backup.FileID{Volume: 0x1a2b3c4d, Index: 99}
	for _, step := range []backup.Step{backup.StepPublishIntent, backup.StepPromoteIntent} {
		path := steps(pre, step, backup.StepPublished)
		if step == backup.StepPromoteIntent {
			path = steps(pre, backup.StepRetireIntent, step, backup.StepPublished)
		}
		err := replace(t, step == backup.StepPromoteIntent, path, step, func(v *backup.StepValues) { v.Staging = other })
		var ambiguity *backup.AmbiguityError
		if !errors.As(err, &ambiguity) {
			t.Errorf("%s of another directory: %v", step, err)
		}
	}
}

// R2: current is the previous generation only if the whole proof chain
// holds, the observed identity included.
func TestPreviousGenerationNeedsItsWholeProofChain(t *testing.T) {
	head := backup.Receipt{Operation: op}
	headSum := sha256.Sum256([]byte("receipt"))
	published := journal(t, false, steps(pre, backup.StepPublishIntent, backup.StepPublished)...)
	if err := backup.RecognizePrevious(head, headSum, published, stagingID); err != nil {
		t.Fatalf("recognised generation refused: %v", err)
	}
	rotated := journal(t, true, steps(pre, backup.StepRetireIntent, backup.StepPromoteIntent, backup.StepPublished)...)
	if err := backup.RecognizePrevious(head, headSum, rotated, stagingID); err != nil {
		t.Fatalf("generation of a rotation refused: %v", err)
	}
	for _, tt := range []struct {
		name    string
		head    backup.Receipt
		sum     [32]byte
		journal backup.Journal
		id      backup.FileID
		want    string
	}{
		{"journal of another operation", backup.Receipt{Operation: previousOp}, headSum, published, stagingID, "not that of the head receipt"},
		{"journal not terminal", head, headSum, journal(t, false, steps(pre, backup.StepPublishIntent)...), stagingID, "not terminal in published"},
		{"journal rolled back", head, headSum, journal(t, false, steps(pre, backup.StepPublishIntent, backup.StepRolledBack)...), stagingID, "not terminal in published"},
		{"receipt file that differs", head, sha256.Sum256([]byte("another receipt")), published, stagingID, "does not match its receipt file"},
		{"another directory, same content", head, headSum, published, oldID, "not the directory"},
		{"no identity", head, headSum, published, backup.FileID{}, "not the directory"},
	} {
		if err := backup.RecognizePrevious(tt.head, tt.sum, tt.journal, tt.id); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: error = %v, want %q", tt.name, err, tt.want)
		}
	}
}

// A plain directory whose identity no record explains is never old: an
// operation without any record cannot be closed beside it, generation
// expected or not, and a rotation cannot take it for its old current.
func TestUnidentifiedCurrentIsNeverOld(t *testing.T) {
	var ambiguity *backup.AmbiguityError
	for _, observed := range []backup.Observation{obs(u, a, a), expecting(u, a, a), obs(backup.Other, a, a), expecting(backup.Other, a, a)} {
		if action, err := backup.Classify(journal(t, false), observed); !errors.As(err, &ambiguity) {
			t.Errorf("not started, %v: %q, %v", observed, action, err)
		}
	}
	for _, path := range [][]backup.Step{pre[:1], pre, steps(pre, backup.StepRetireIntent)} {
		if action, err := backup.Classify(journal(t, true, path...), obs(u, a, n)); !errors.As(err, &ambiguity) {
			t.Errorf("rotation %v, unidentified current: %q, %v", path, action, err)
		}
	}
	// M3, the two contradictions: a generation expected but no current, and
	// a current where none was ever published.
	if _, err := backup.Classify(journal(t, false), expecting(a, a, a)); !errors.As(err, &ambiguity) {
		t.Errorf("missing current beside a published generation: %v", err)
	}
	if _, err := backup.Classify(journal(t, false), obs(o, a, a)); !errors.As(err, &ambiguity) {
		t.Errorf("current without any published generation: %v", err)
	}
}
