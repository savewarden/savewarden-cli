package backup_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

// The example of docs/snapshot-receipt-v1.md is a valid snapshot receipt,
// the one the fixtures of this package build, that encodes back to the
// same bytes.
func TestDocumentedSnapshotReceiptExample(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "snapshot-receipt-v1.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut(string(content), "```json\n")
	example, _, closed := strings.Cut(rest, "```")
	if !ok || !closed {
		t.Fatal("docs/snapshot-receipt-v1.md has no JSON example")
	}
	r, err := backup.DecodeSnapshotReceipt([]byte(example))
	if err != nil {
		t.Fatalf("documented example is invalid: %v", err)
	}
	again, err := backup.EncodeSnapshotReceipt(r)
	if err != nil || string(again) != example {
		t.Fatalf("documented example is not the canonical encoding:\n%s", again)
	}
	fixture, err := backup.EncodeSnapshotReceipt(snapshotReceipt(t))
	if err != nil || string(fixture) != example {
		t.Fatalf("documented example differs from the fixture:\n%s", fixture)
	}
}
