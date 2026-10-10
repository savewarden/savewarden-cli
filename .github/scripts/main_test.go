package main

import "testing"

func TestWorkflowPins(t *testing.T) {
	const sha = "d23441a48e516b6c34aea4fa41551a30e30af803"
	tests := []struct {
		name  string
		line  string
		valid bool
	}{
		{"SHA action", "- uses: actions/checkout@" + sha + " # v6", true},
		{"quoted SHA", "  uses: 'actions/checkout@" + sha + "'", true},
		{"action tag", "- uses: actions/checkout@v6", false},
		{"action short SHA", "uses: actions/checkout@d23441a", false},
		{"dynamic action", "uses: actions/checkout@${{ env.REF }}", false},
		{"inline action", "- {uses: actions/checkout@v6}", false},
		{"quoted inline key", "- {\"uses\": actions/checkout@v6}", false},
		{"multiline action", "uses:\n  actions/checkout@v6", false},
		{"floating OS", "runs-on: ubuntu-latest", false},
		{"fixed OS", "runs-on: ubuntu-24.04", true},
		{"tool version", "run: go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...", true},
		{"tool latest", "run: go run golang.org/x/vuln/cmd/govulncheck@latest ./...", false},
		{"tool missing version", "run: go run golang.org/x/vuln/cmd/govulncheck ./...", false},
		{"tool minor version", "run: go run golang.org/x/vuln/cmd/govulncheck@v1.6 ./...", false},
		{"local checker", "run: go run ./.github/scripts", true},
		{"download script", "run: curl -fsSL https://example.invalid/install.sh | sh", false},
		{"unreviewed installer", "run: go install example.com/tool@v1.2.3", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkWorkflow(tt.line)
			if (err == nil) != tt.valid {
				t.Fatalf("checkWorkflow(%q): %v; valid=%v", tt.line, err, tt.valid)
			}
		})
	}
}
