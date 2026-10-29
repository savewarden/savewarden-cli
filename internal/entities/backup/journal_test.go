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

func TestRecordRoundTrip(t *testing.T) {
	names, contents := journalFiles(t, false, backup.StepPlanned, backup.StepStagingIntent)
	data := contents[names[1]]
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[1], "sha256 ") {
		t.Fatalf("record layout:\n%s", data)
	}
	if !strings.HasPrefix(lines[0], `{"at":"2026-11-05T10:15:01Z","fields":{"staging":"staging/`+op+`"},"op":"`+op+`","prev":"`) {
		t.Fatalf("keys must be sorted: %s", lines[0])
	}
	r, err := backup.DecodeRecord(data)
	if err != nil || r.Seq != 2 || r.Step != backup.StepStagingIntent || r.Op != op || r.Prev == "" {
		t.Fatalf("decoded %+v, %v", r, err)
	}
	first, err := backup.DecodeRecord(contents[names[0]])
	if err != nil || first.Prev != "" || string(first.Fields["previous"]) != "null" {
		t.Fatalf("first record %+v, %v", first, err)
	}
}

func TestJournalAmbiguities(t *testing.T) {
	base := func() ([]string, map[string][]byte) {
		return journalFiles(t, false, backup.StepPlanned, backup.StepStagingIntent, backup.StepStaged)
	}
	tests := []struct {
		name   string
		mutate func(names []string, contents map[string][]byte) []string
		want   string
	}{
		{"truncated final", func(names []string, c map[string][]byte) []string {
			c[names[1]] = c[names[1]][:len(c[names[1]])-20]
			return names
		}, "record 002-staging-intent.rec"},
		{"checksum mismatch", func(names []string, c map[string][]byte) []string {
			c[names[2]] = bytes.Replace(c[names[2]], []byte(`"seq":3`), []byte(`"seq":4`), 1)
			return names
		}, "checksum"},
		{"missing number", func(names []string, c map[string][]byte) []string { return []string{names[0], names[2]} }, "expected 002"},
		{"duplicate number", func(names []string, c map[string][]byte) []string {
			other := backup.RecordName(2, backup.StepFailed)
			c[other] = c[names[1]]
			return append(names, other)
		}, "duplicate record number 002"},
		{"broken chain", func(names []string, c map[string][]byte) []string {
			// A valid planned record, but not the one record 002 chains to.
			values := valuesFor(t, backup.StepPlanned, false)
			values.Plan.GameName = "Another Game"
			r, err := backup.NewRecord(1, backup.StepPlanned, op, at, "", values)
			if err != nil {
				t.Fatal(err)
			}
			c[names[0]] = mustEncode(t, r)
			return names
		}, "chain broken"},
		{"forbidden transition", func(names []string, c map[string][]byte) []string {
			names2, c2 := journalFiles(t, false, backup.StepPlanned, backup.StepPublished)
			for k, v := range c2 {
				c[k] = v
			}
			return names2
		}, "transition"},
		{"temporary of another number", func(names []string, c map[string][]byte) []string {
			return append(names, backup.RecordName(5, backup.StepPublishIntent)+".tmp-1")
		}, "does not follow"},
		{"temporary of a forbidden step", func(names []string, c map[string][]byte) []string {
			return append(names, backup.RecordName(4, backup.StepPublished)+".tmp-1")
		}, "not an allowed next step"},
		{"tenth temporary name", func(names []string, c map[string][]byte) []string {
			return append(names, backup.RecordName(4, backup.StepVerified)+".tmp-10")
		}, "unexpected entry"},
		{"foreign file", func(names []string, c map[string][]byte) []string { return append(names, "notes.txt") }, "unexpected entry"},
		{"record of another operation", func(names []string, c map[string][]byte) []string {
			c[names[0]] = mustEncode(t, backup.Record{Seq: 1, Step: backup.StepPlanned, Op: "20261105T101500-ffffffff", At: at, Fields: map[string]json.RawMessage{}})
			return names
		}, "does not match its name or operation"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			names, contents := base()
			names = tt.mutate(names, contents)
			_, err := backup.ParseJournal(op, names, contents)
			var ambiguity *backup.AmbiguityError
			if !errors.As(err, &ambiguity) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want an ambiguity mentioning %q", err, tt.want)
			}
		})
	}

	t.Run("temporary after terminal", func(t *testing.T) {
		names, contents := journalFiles(t, false, backup.StepAbandoned)
		names = append(names, backup.RecordName(2, backup.StepPlanned)+".tmp-1")
		if _, err := backup.ParseJournal(op, names, contents); err == nil || !strings.Contains(err.Error(), "terminal") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("prepared intention is counted", func(t *testing.T) {
		names, contents := base()
		names = append(names, backup.RecordName(4, backup.StepVerified)+".tmp-1", backup.RecordName(4, backup.StepVerified)+".tmp-2")
		j, err := backup.ParseJournal(op, names, contents)
		if err != nil || j.Prepared[backup.StepVerified] != 2 || j.Last() != backup.StepStaged || j.Terminal() {
			t.Fatalf("journal %+v, %v", j, err)
		}
	})
}

func mustEncode(t *testing.T, r backup.Record) []byte {
	t.Helper()
	data, err := backup.EncodeRecord(r)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestEncodeRecordRefusesInvalidRecords(t *testing.T) {
	for _, r := range []backup.Record{
		{Seq: 0, Step: backup.StepPlanned, Op: op},
		{Seq: 12, Step: backup.StepPlanned, Op: op, Prev: strings.Repeat("0", 64)},
		{Seq: 1, Step: "teleport", Op: op},
		{Seq: 2, Step: backup.StepStaged, Op: op},
		{Seq: 1, Step: backup.StepPlanned, Op: "today"},
		{Seq: 1, Step: backup.StepPlanned, Op: op, Fields: map[string]json.RawMessage{"x": json.RawMessage("{")}},
	} {
		if _, err := backup.EncodeRecord(r); err == nil {
			t.Errorf("%+v: accepted", r)
		}
	}
}

// A first publication and a rotation never share a switch step, and the
// planned record says which one the operation is.
func TestFirstPublicationAndRotationDoNotMix(t *testing.T) {
	for _, tt := range []struct {
		rotation bool
		steps    []backup.Step
	}{
		{false, steps(pre, backup.StepRetireIntent)},
		{true, steps(pre, backup.StepPublishIntent)},
	} {
		names, contents := journalFiles(t, tt.rotation, tt.steps...)
		if _, err := backup.ParseJournal(op, names, contents); err == nil || !strings.Contains(err.Error(), "not allowed") {
			t.Errorf("rotation %v, %v: %v", tt.rotation, tt.steps, err)
		}
	}
	for _, previous := range []string{``, `42`, `"yesterday"`, `"` + op + `"`, `"` + previousOp + `"`} {
		r, err := backup.NewRecord(1, backup.StepPlanned, op, at, "", valuesFor(t, backup.StepPlanned, false))
		if err != nil {
			t.Fatal(err)
		}
		delete(r.Fields, "previous")
		if previous != "" {
			r.Fields["previous"] = json.RawMessage(previous)
		}
		data := mustEncode(t, r)
		name := backup.RecordName(1, backup.StepPlanned)
		var ambiguity *backup.AmbiguityError
		if _, err := backup.ParseJournal(op, []string{name}, map[string][]byte{name: data}); !errors.As(err, &ambiguity) || !strings.Contains(err.Error(), "previous") {
			t.Errorf("previous %s: %v", previous, err)
		}
	}
}

// An empty "prev" is not null: the first record must say null explicitly.
func TestRecordEmptyPrevIsRefused(t *testing.T) {
	line := []byte(`{"at":"2026-11-05T10:15:00Z","fields":{"previous":null},"op":"` + op + `","prev":"","schema":"savewarden.journal","schema_version":1,"seq":1,"step":"planned"}` + "\n")
	sum := sha256.Sum256(line)
	data := append(line, []byte("sha256 "+hex.EncodeToString(sum[:])+"\n")...)
	if _, err := backup.DecodeRecord(data); err == nil || !strings.Contains(err.Error(), "non-empty string") {
		t.Fatalf("error = %v", err)
	}
}

// Envelope E2: no operation has a twelfth record, even a well-formed one.
func TestRecordBeyondTheEnvelope(t *testing.T) {
	longest := steps(pre, backup.StepRetireIntent, backup.StepPromoteIntent, backup.StepUnpromoteIntent,
		backup.StepRollbackFailed, backup.StepRestoreIntent, backup.StepRollbackFailed, backup.StepRolledBack)
	names, contents := journalFiles(t, true, longest...)
	sum := sha256.Sum256(contents[names[len(names)-1]])
	line := []byte(`{"at":"2026-11-05T10:15:00Z","fields":{},"op":"` + op + `","prev":"` + hex.EncodeToString(sum[:]) +
		`","schema":"savewarden.journal","schema_version":1,"seq":12,"step":"abandoned"}` + "\n")
	own := sha256.Sum256(line)
	name := "012-abandoned.rec"
	contents[name] = append(line, []byte("sha256 "+hex.EncodeToString(own[:])+"\n")...)
	var ambiguity *backup.AmbiguityError
	if _, err := backup.ParseJournal(op, append(names, name), contents); !errors.As(err, &ambiguity) || !strings.Contains(err.Error(), "outside 1..11") {
		t.Fatalf("error = %v", err)
	}
}

func tmp(seq int, step backup.Step, k int) string {
	return fmt.Sprintf("%s.tmp-%d", backup.RecordName(seq, step), k)
}

// A temporary left by an interrupted attempt stays beside the record that a
// later attempt committed, possibly under another step: Savewarden never
// deletes it. Such states come from the protocol and must stay readable.
func TestAbandonedTemporariesAreAccepted(t *testing.T) {
	tests := []struct {
		name     string
		rotation bool
		steps    []backup.Step
		extra    []string
		prepared map[backup.Step]int
	}{
		{"operation abandoned before its first record (I8)", false,
			[]backup.Step{backup.StepAbandoned},
			[]string{tmp(1, backup.StepPlanned, 1)}, nil},
		{"intent only prepared, then abandoned (A1)", false,
			steps(pre, backup.StepAbandoned),
			[]string{tmp(5, backup.StepPublishIntent, 1), tmp(5, backup.StepPublishIntent, 2)}, nil},
		{"published record committed on a second attempt (A6)", false,
			steps(pre, backup.StepPublishIntent, backup.StepPublished),
			[]string{tmp(6, backup.StepPublished, 1)}, nil},
		{"eight abandoned attempts beside a record", false,
			steps(pre[:2]),
			[]string{tmp(2, backup.StepStagingIntent, 1), tmp(2, backup.StepStagingIntent, 2), tmp(2, backup.StepFailed, 3),
				tmp(2, backup.StepAbandoned, 4), tmp(2, backup.StepStagingIntent, 5), tmp(2, backup.StepStagingIntent, 6),
				tmp(2, backup.StepStagingIntent, 7), tmp(2, backup.StepStagingIntent, 8)}, nil},
		{"abandoned attempts on several numbers, then a prepared intent", false,
			pre,
			[]string{tmp(1, backup.StepPlanned, 1), tmp(3, backup.StepStaged, 1), tmp(3, backup.StepFailed, 2),
				tmp(5, backup.StepPublishIntent, 1)},
			map[backup.Step]int{backup.StepPublishIntent: 1}},
		{"rotation: an abandoned retire intent", true,
			steps(pre, backup.StepRetireIntent),
			[]string{tmp(5, backup.StepRetireIntent, 1)}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			names, contents := journalFiles(t, tt.rotation, tt.steps...)
			j, err := backup.ParseJournal(op, append(names, tt.extra...), contents)
			if err != nil {
				t.Fatal(err)
			}
			want := tt.prepared
			if want == nil {
				want = map[backup.Step]int{}
			}
			attempts := 0
			for _, count := range want {
				attempts += count
			}
			if len(j.Records) != len(tt.steps) || !reflect.DeepEqual(j.Prepared, want) || j.Attempts() != attempts ||
				len(j.Taken) != attempts {
				t.Fatalf("records %d, prepared %v, attempts %d, taken %v", len(j.Records), j.Prepared, j.Attempts(), j.Taken)
			}
		})
	}
}

// Only the states the protocol can produce are accepted: anything else
// about temporaries stays an ambiguity.
func TestIncoherentTemporariesAreAmbiguous(t *testing.T) {
	nine := func(seq int, step backup.Step) []string {
		var out []string
		for k := 1; k <= backup.MaxTemporaries; k++ {
			out = append(out, tmp(seq, step, k))
		}
		return out
	}
	tests := []struct {
		name     string
		rotation bool
		steps    []backup.Step
		extra    []string
		want     string
	}{
		{"step not allowed at its number", false, pre,
			[]string{tmp(2, backup.StepPublished, 1)}, "not an allowed step at that number"},
		{"first-publication step in a rotation", true, steps(pre, backup.StepRetireIntent),
			[]string{tmp(5, backup.StepPublishIntent, 1)}, "not an allowed step at that number"},
		{"rotation step in a first publication", false, steps(pre, backup.StepPublishIntent),
			[]string{tmp(5, backup.StepRetireIntent, 1)}, "not an allowed step at that number"},
		{"nine temporaries beside a committed record", false, pre,
			nine(3, backup.StepStaged), "beside nine temporaries"},
		{"attempt number shared by two steps of a committed record", false, pre,
			[]string{tmp(2, backup.StepStagingIntent, 1), tmp(2, backup.StepFailed, 1)}, "share the attempt number 1"},
		{"attempt number shared by two prepared steps", false, pre,
			[]string{tmp(5, backup.StepPublishIntent, 2), tmp(5, backup.StepFailed, 2)}, "share the attempt number 2"},
		{"temporary numbered 000", false, pre,
			[]string{tmp(0, backup.StepPlanned, 1)}, "does not follow"},
		{"temporary beyond the next record", false, pre,
			[]string{tmp(6, backup.StepPublished, 1)}, "does not follow"},
		{"next record prepared after a terminal step", false, steps(pre, backup.StepAbandoned),
			[]string{tmp(5, backup.StepPublishIntent, 1), tmp(6, backup.StepPublished, 1)}, "terminal"},
		{"unknown step", false, pre,
			[]string{tmp(2, "teleport", 1)}, "not an allowed step at that number"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			names, contents := journalFiles(t, tt.rotation, tt.steps...)
			_, err := backup.ParseJournal(op, append(names, tt.extra...), contents)
			var ambiguity *backup.AmbiguityError
			if !errors.As(err, &ambiguity) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want an ambiguity mentioning %q", err, tt.want)
			}
		})
	}
}

// Plan B2 §4.7: the plan starts at the date of its own record.
func TestPlanStartsAtItsRecord(t *testing.T) {
	data := record(t, 1, backup.StepPlanned, false, "", at.Add(time.Second))
	name := backup.RecordName(1, backup.StepPlanned)
	var ambiguity *backup.AmbiguityError
	if _, err := backup.ParseJournal(op, []string{name}, map[string][]byte{name: data}); !errors.As(err, &ambiguity) ||
		!strings.Contains(err.Error(), "started_at") {
		t.Fatalf("error = %v", err)
	}
}
