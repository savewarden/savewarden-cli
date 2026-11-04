package backup_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

// The examples of docs/restore-receipt-v1.md are a valid restore receipt
// and a valid area marker, those the fixtures of this package build, that
// encode back to the same bytes.
func TestDocumentedRestoreExamples(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "restore-receipt-v1.md"))
	if err != nil {
		t.Fatal(err)
	}
	var examples []string
	rest := string(content)
	for {
		_, after, ok := strings.Cut(rest, "```json\n")
		if !ok {
			break
		}
		example, remainder, closed := strings.Cut(after, "```")
		if !closed {
			t.Fatal("docs/restore-receipt-v1.md has an unclosed JSON example")
		}
		examples, rest = append(examples, example), remainder
	}
	if len(examples) != 2 {
		t.Fatalf("docs/restore-receipt-v1.md has %d JSON examples, want the receipt and the marker", len(examples))
	}
	r, err := backup.DecodeRestoreReceipt([]byte(examples[0]))
	if err != nil {
		t.Fatalf("documented receipt is invalid: %v", err)
	}
	if again, err := backup.EncodeRestoreReceipt(r); err != nil || string(again) != examples[0] {
		t.Fatalf("documented receipt is not the canonical encoding:\n%s", again)
	}
	if fixture, err := backup.EncodeRestoreReceipt(restoreReceipt(t)); err != nil || string(fixture) != examples[0] {
		t.Fatalf("documented receipt differs from the fixture:\n%s", fixture)
	}
	m, err := backup.DecodeRestoreAreaMarker([]byte(examples[1]))
	if err != nil {
		t.Fatalf("documented marker is invalid: %v", err)
	}
	if again, err := backup.EncodeRestoreAreaMarker(m); err != nil || string(again) != examples[1] {
		t.Fatalf("documented marker is not the canonical encoding:\n%s", again)
	}
}
