package backup_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

var allSteps = []backup.Step{
	backup.StepPlanned, backup.StepStagingIntent, backup.StepStaged, backup.StepVerified, backup.StepPublishIntent,
	backup.StepUnpublishIntent, backup.StepRetireIntent, backup.StepPromoteIntent, backup.StepUnpromoteIntent,
	backup.StepRestoreIntent, backup.StepRollbackFailed, backup.StepPublished, backup.StepFailed,
	backup.StepAbandoned, backup.StepRolledBack,
}

// Each step's values survive the record format unchanged.
func TestStepValuesRoundTrip(t *testing.T) {
	for _, step := range allSteps {
		seq, prev := 2, strings.Repeat("0", 64)
		if step == backup.StepPlanned {
			seq, prev = 1, ""
		}
		want := valuesFor(t, step, false)
		r, err := backup.NewRecord(seq, step, op, at, prev, want)
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		decoded, err := backup.DecodeRecord(mustEncode(t, r))
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		got, err := decoded.Values()
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		if step == backup.StepPlanned {
			_, sum, err := want.Plan.Encode()
			if err != nil {
				t.Fatal(err)
			}
			want.PlanSHA256 = sum
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: values %+v, want %+v", step, got, want)
		}
	}
}

// The contract of each step is closed and typed: a missing, unknown or
// ill-typed field is refused, never ignored.
func TestStepFieldsAreStrict(t *testing.T) {
	edit := func(step backup.Step, change func(fields map[string]json.RawMessage)) error {
		seq, prev := 2, strings.Repeat("0", 64)
		if step == backup.StepPlanned {
			seq, prev = 1, ""
		}
		r, err := backup.NewRecord(seq, step, op, at, prev, valuesFor(t, step, false))
		if err != nil {
			t.Fatal(err)
		}
		change(r.Fields)
		_, err = r.Values()
		return err
	}
	set := func(name, value string) func(map[string]json.RawMessage) {
		return func(fields map[string]json.RawMessage) { fields[name] = json.RawMessage(value) }
	}
	tests := []struct {
		name   string
		step   backup.Step
		change func(map[string]json.RawMessage)
		want   string
	}{
		{"unknown field", backup.StepStaged, set("comment", `"x"`), `unknown field "comment"`},
		{"unknown field on rolled-back", backup.StepRolledBack, set("reason", `"x"`), `unknown field "reason"`},
		{"missing identity", backup.StepPublishIntent, func(f map[string]json.RawMessage) { delete(f, "staging_identity") }, `missing field "staging_identity"`},
		{"identity as a number", backup.StepStaged, set("staging_identity", `42`), "must be a string"},
		{"identity in capitals", backup.StepUnpublishIntent, set("current_identity", `"1A2B3C4D-0000000000000002"`), "not of the form"},
		{"digest in capitals", backup.StepVerified, set("verified_sha256", `"`+strings.Repeat("A", 64)+`"`), "verified_sha256"},
		{"short digest", backup.StepPublished, set("receipt_sha256", `"abc"`), "receipt_sha256"},
		{"another staging", backup.StepStagingIntent, set("staging", `"staging/elsewhere"`), `"staging" must be`},
		{"empty reason", backup.StepFailed, set("reason", `""`), "single line"},
		{"reason on two lines", backup.StepAbandoned, set("reason", `"a\nb"`), "single line"},
		{"reason too long", backup.StepRollbackFailed, set("error", `"`+strings.Repeat("x", 1025)+`"`), "single line"},
		{"plan digest that does not match", backup.StepPlanned, set("plan_sha256", `"`+strings.Repeat("0", 64)+`"`), "does not match the plan"},
		{"previous that contradicts the plan", backup.StepPlanned, set("previous", `"`+previousOp+`"`), "differs from the plan"},
		{"plan with an unknown key", backup.StepPlanned, func(f map[string]json.RawMessage) {
			f["plan"] = json.RawMessage(strings.Replace(string(f["plan"]), `{"changes"`, `{"extra":1,"changes"`, 1))
		}, "plan"},
		{"plan without sources", backup.StepPlanned, func(f map[string]json.RawMessage) {
			f["plan"] = json.RawMessage(`{"changes":{"added":0,"modified":0,"removed":0,"unchanged":0},"game":{"id":"g","name":"G"},"previous":null,"sources":[],"started_at":"2026-11-05T10:15:00Z","tool_version":"dev"}`)
		}, "at least one source"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := edit(tt.step, tt.change); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
	if _, err := backup.NewRecord(1, backup.StepPlanned, op, at, "", backup.StepValues{}); err == nil {
		t.Fatal("planned record without a plan")
	}
	if _, err := backup.NewRecord(2, backup.StepFailed, op, at, strings.Repeat("0", 64), backup.StepValues{}); err == nil {
		t.Fatal("failed record without a reason")
	}
}

// replace commits steps whose values for one step are changed, as a
// faulty writer would, and parses the journal.
func replace(t *testing.T, rotation bool, steps []backup.Step, step backup.Step, change func(*backup.StepValues)) error {
	t.Helper()
	var names []string
	contents := map[string][]byte{}
	prev := ""
	for i, s := range steps {
		values := valuesFor(t, s, rotation)
		if s == step {
			change(&values)
		}
		r, err := backup.NewRecord(i+1, s, op, at, prev, values)
		if err != nil {
			t.Fatal(err)
		}
		data := mustEncode(t, r)
		name := backup.RecordName(i+1, s)
		names = append(names, name)
		contents[name] = data
		sum := sha256.Sum256(data)
		prev = hex.EncodeToString(sum[:])
	}
	_, err := backup.ParseJournal(op, names, contents)
	return err
}

// Records must agree with each other: what an intent expects is what an
// earlier record observed.
func TestRecordsAreCoherent(t *testing.T) {
	other := backup.FileID{Volume: 0x1a2b3c4d, Index: 99}
	rotation := steps(pre, backup.StepRetireIntent, backup.StepPromoteIntent, backup.StepUnpromoteIntent, backup.StepRestoreIntent)
	tests := []struct {
		name     string
		rotation bool
		steps    []backup.Step
		step     backup.Step
		change   func(*backup.StepValues)
		want     string
	}{
		{"copy that is not the plan", false, pre, backup.StepStaged, func(v *backup.StepValues) { v.Digest[0] ^= 1 }, "staged digest"},
		{"verification that is not the plan", false, pre, backup.StepVerified, func(v *backup.StepValues) { v.Digest[0] ^= 1 }, "verified digest"},
		{"publish another directory", false, steps(pre, backup.StepPublishIntent), backup.StepPublishIntent,
			func(v *backup.StepValues) { v.Staging = other }, "differs from"},
		{"unpublish another directory", false, steps(pre, backup.StepPublishIntent, backup.StepUnpublishIntent), backup.StepUnpublishIntent,
			func(v *backup.StepValues) { v.Current = other }, "differs from the staging identity"},
		{"promote another directory", true, rotation, backup.StepPromoteIntent, func(v *backup.StepValues) { v.Staging = other }, "differs from"},
		{"unpromote another directory", true, rotation, backup.StepUnpromoteIntent, func(v *backup.StepValues) { v.Current = other }, "differs from the staging identity"},
		{"restore another generation", true, rotation, backup.StepRestoreIntent, func(v *backup.StepValues) { v.Retained = other }, "recorded by retire-intent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := replace(t, tt.rotation, tt.steps, tt.step, tt.change)
			var ambiguity *backup.AmbiguityError
			if !errors.As(err, &ambiguity) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want an ambiguity mentioning %q", err, tt.want)
			}
		})
	}
	j := journal(t, true, rotation...)
	if j.Plan == nil || j.Staging != stagingID || j.Old != oldID || len(j.Values) != len(rotation) || j.Previous != previousOp {
		t.Fatalf("journal %+v", j)
	}
}
