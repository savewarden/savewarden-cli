package backup_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	if !strings.HasPrefix(lines[0], `{"at":"2026-11-05T10:15:01Z","fields":{},"op":"`+op+`","prev":"`) {
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
			c[names[0]] = mustEncode(t, backup.Record{Seq: 1, Step: backup.StepPlanned, Op: op, At: at.Add(time.Hour), Fields: map[string]json.RawMessage{"previous": json.RawMessage("null")}})
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
	for _, previous := range []string{``, `42`, `"yesterday"`, `"` + op + `"`} {
		fields := map[string]json.RawMessage{}
		if previous != "" {
			fields["previous"] = json.RawMessage(previous)
		}
		data := mustEncode(t, backup.Record{Seq: 1, Step: backup.StepPlanned, Op: op, At: at, Fields: fields})
		name := backup.RecordName(1, backup.StepPlanned)
		var ambiguity *backup.AmbiguityError
		if _, err := backup.ParseJournal(op, []string{name}, map[string][]byte{name: data}); !errors.As(err, &ambiguity) {
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
