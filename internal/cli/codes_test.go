package cli_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// R5: every stable code and intervention cause that the sync feature
// declares is in the tables of ADR 0007, current-exists included, kept with
// its B2b meaning although this build no longer emits it.
func TestStableCodesAreDocumented(t *testing.T) {
	adr, err := os.ReadFile(filepath.Join("..", "..", "docs", "adr", "0007-sync-contract-receipt-v1.md"))
	if err != nil {
		t.Fatal(err)
	}
	sources, err := filepath.Glob(filepath.Join("..", "features", "sync", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	declared := regexp.MustCompile(`(?:backup\.Code|Cause)\s+=\s+"([a-z-]+)"`)
	seen := map[string]bool{}
	for _, name := range sources {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range declared.FindAllStringSubmatch(string(data), -1) {
			seen[m[1]] = true
		}
	}
	for _, code := range []string{"current-exists", "current-modified", "insufficient-space", "pending-operation"} {
		if !seen[code] {
			t.Fatalf("premise: %s is not found among the declared codes %v", code, seen)
		}
	}
	for code := range seen {
		if !strings.Contains(string(adr), "`"+code+"`") {
			t.Errorf("code %s is declared by the sync feature but not documented in ADR 0007", code)
		}
	}
}
