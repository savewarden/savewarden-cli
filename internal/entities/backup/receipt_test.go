package backup_test

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

func receipt() backup.Receipt {
	published := at.Add(4 * time.Second)
	return backup.Receipt{
		Operation: op, ToolVersion: "dev", GameID: "synthetic-game", GameName: "Synthetic Game",
		StartedAt: at, PublishIntentAt: at.Add(3 * time.Second), PublishedAt: &published,
		PlanSHA256: sha256.Sum256([]byte("plan")),
		Changes:    backup.ChangeCounts{Added: 1},
		Sources: []backup.ReceiptSource{{
			ID: "saves", Kind: "save", Required: true, State: backup.SourceCopied,
			Path: `D:\Games\Synthetic Game\saves`, Directories: []string{"Slot 1"},
			Entries: []backup.Entry{{Path: "Slot 1/1.save", Size: 3, ModTime: time.Date(2026, 10, 9, 16, 32, 8, 123456700, time.UTC), SHA256: sha256.Sum256([]byte("one"))}},
		}},
	}
}

func TestReceiptRoundTrip(t *testing.T) {
	data, err := backup.EncodeReceipt(receipt())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"schema": "savewarden.receipt"`, `"recovered_by": null`, `"published_at": "2026-11-05T10:15:04Z"`,
		`"previous": null`, `"mod_time": "2026-10-09T16:32:08.1234567Z"`, `"files": 1`, `"bytes": 3`,
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("receipt lacks %s:\n%s", want, data)
		}
	}
	back, err := backup.DecodeReceipt(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, receipt()) {
		t.Fatalf("round trip:\n got %+v\nwant %+v", back, receipt())
	}
	if again, _ := backup.EncodeReceipt(back); string(again) != string(data) {
		t.Fatal("encoding is not deterministic")
	}
}

func TestReconstructedReceiptProvenance(t *testing.T) {
	r := receipt()
	recovered := at.Add(time.Hour)
	r.PublishedAt, r.RecoveredAt, r.RecoveredBy = nil, &recovered, "dev"
	data, err := backup.EncodeReceipt(r)
	if err != nil {
		t.Fatal(err)
	}
	back, err := backup.DecodeReceipt(data)
	if err != nil || !back.Recovered() || back.PublishedAt != nil || !strings.Contains(string(data), `"published_at": null`) {
		t.Fatalf("reconstructed receipt: %+v, %v\n%s", back, err, data)
	}
	r.PublishedAt = &recovered
	if _, err := backup.EncodeReceipt(r); err == nil || !strings.Contains(err.Error(), "must be null in a reconstructed receipt") {
		t.Fatalf("a reconstructed receipt must not claim a publication time: %v", err)
	}
}

func TestReceiptStrictness(t *testing.T) {
	valid, err := backup.EncodeReceipt(receipt())
	if err != nil {
		t.Fatal(err)
	}
	text := string(valid)
	tests := []struct {
		name, from, to, want string
	}{
		{"unknown field", `"schema": "savewarden.receipt",`, `"schema": "savewarden.receipt", "extra": 1,`, "unknown field"},
		{"duplicate field", `"tool_version": "dev",`, `"tool_version": "dev", "tool_version": "evil",`, "duplicate key"},
		{"missing field", `"recovered_by": null,`, ``, "recovered_by: required field is missing"},
		{"schema version", `"schema_version": 1`, `"schema_version": 2`, "unsupported schema"},
		{"local time", `"started_at": "2026-11-05T10:15:00Z"`, `"started_at": "2026-11-05T11:15:00+01:00"`, "in UTC"},
		{"bad operation", `"operation": "` + op + `"`, `"operation": "yesterday"`, "operation"},
		{"previous equals operation", `"previous": null`, `"previous": "` + op + `"`, "must differ"},
		{"intent before start", `"publish_intent_at": "2026-11-05T10:15:03Z"`, `"publish_intent_at": "2026-11-05T10:14:00Z"`, "precedes started_at"},
		{"uppercase hash", `"plan_sha256": "`, `"plan_sha256": "A`, "lowercase hexadecimal"},
		{"files mismatch", `"files": 1`, `"files": 2`, "does not match"},
		{"bytes mismatch", `"bytes": 3`, `"bytes": 4`, "does not match"},
		{"unknown status", `"status": "copied"`, `"status": "moved"`, "copied or missing"},
		{"escaping path", `"directories": [`, `"directories": ["../up",`, "relative"},
		{"half recovered", `"recovered_at": null`, `"recovered_at": "2026-11-05T11:00:00Z"`, "both null or both set"},
		{"null required", `"tool_version": "dev"`, `"tool_version": null`, "tool_version: required field is missing"},
		{"case variant duplicate", `"tool_version": "dev",`, `"tool_version": "dev", "TOOL_VERSION": "evil",`, "lowercase ASCII"},
		{"case variant key", `"operation":`, `"Operation":`, "lowercase ASCII"},
		{"folded variant key", `"sha256": "7`, `"017Fha256": "7`, "lowercase ASCII"},
		{"empty previous", `"previous": null`, `"previous": ""`, "non-empty string"},
		{"empty recovered_by", `"recovered_by": null`, `"recovered_by": ""`, "non-empty string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(text, tt.from) {
				t.Fatalf("fixture lacks %q", tt.from)
			}
			data := strings.Replace(text, tt.from, tt.to, 1)
			if _, err := backup.DecodeReceipt([]byte(data)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}

	if _, err := backup.DecodeReceipt([]byte(text + "{}")); err == nil || !strings.Contains(err.Error(), "after the JSON value") {
		t.Fatalf("trailing data: %v", err)
	}

	unsorted := receipt()
	unsorted.Sources[0].Entries = append(unsorted.Sources[0].Entries, backup.Entry{Path: "A.save"})
	if _, err := backup.EncodeReceipt(unsorted); err == nil || !strings.Contains(err.Error(), "sorted") {
		t.Fatalf("unsorted entries: %v", err)
	}
	accepted := receipt()
	accepted.Sources[0].Accepted = true
	if _, err := backup.EncodeReceipt(accepted); err == nil {
		t.Fatal("a copied source cannot be accepted as missing")
	}
	missing := receipt()
	missing.Sources[0].State = backup.SourceMissing
	if _, err := backup.EncodeReceipt(missing); err == nil {
		t.Fatal("a missing source cannot hold entries")
	}
}

// The example of docs/receipt-v1.md is a valid receipt that encodes back to
// the same bytes.
func TestDocumentedReceiptExample(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "receipt-v1.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut(string(content), "```json\n")
	example, _, closed := strings.Cut(rest, "```")
	if !ok || !closed {
		t.Fatal("docs/receipt-v1.md has no JSON example")
	}
	r, err := backup.DecodeReceipt([]byte(example))
	if err != nil {
		t.Fatalf("documented example is invalid: %v", err)
	}
	again, err := backup.EncodeReceipt(r)
	if err != nil || string(again) != example {
		t.Fatalf("documented example is not the canonical encoding:\n%s", again)
	}
}
