// Command scripts checks the bootstrap's small workflow supply-chain surface.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	usesLine    = regexp.MustCompile(`^\s*(?:-\s*)?uses:\s*(.+?)\s*$`)
	usesToken   = regexp.MustCompile(`(?:["']?uses["']?)\s*:`)
	actionPin   = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_./-]+@[a-f0-9]{40}$`)
	goRun       = regexp.MustCompile(`\bgo run\s+(\S+)`)
	toolVersion = regexp.MustCompile(`^[A-Za-z0-9./_-]+@v[0-9]+\.[0-9]+\.[0-9]+$`)
	goVersion   = regexp.MustCompile(`(?m)^go [0-9]+\.[0-9]+\.[0-9]+$`)
)

func checkWorkflow(content string) error {
	for i, line := range strings.Split(content, "\n") {
		code, _, _ := strings.Cut(line, " #")
		match := usesLine.FindStringSubmatch(code)
		if usesToken.MatchString(code) && match == nil {
			return fmt.Errorf("line %d: use explicit block-style Action pins", i+1)
		}
		if match != nil {
			pin := strings.Trim(strings.TrimSpace(match[1]), "\"'")
			if !actionPin.MatchString(pin) {
				return fmt.Errorf("line %d: Action must use a full SHA: %s", i+1, pin)
			}
		}
		if strings.Contains(code, "-latest") || strings.Contains(code, "@latest") {
			return fmt.Errorf("line %d: floating OS/tool version", i+1)
		}
		for _, match := range goRun.FindAllStringSubmatch(code, -1) {
			if !strings.HasPrefix(match[1], "./") && !toolVersion.MatchString(match[1]) {
				return fmt.Errorf("line %d: Go tool requires an exact version", i+1)
			}
		}
		// B0 installs tools from checksum-authenticated Go modules only.
		if strings.Contains(code, "curl ") || strings.Contains(code, "wget ") ||
			strings.Contains(code, "go install ") || strings.Contains(code, "docker://") {
			return fmt.Errorf("line %d: unsupported tool installer; add reviewed checksum policy first", i+1)
		}
	}
	return nil
}

func run() error {
	paths, err := filepath.Glob(".github/workflows/*.y*ml")
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("no workflows found; run from repository root")
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := checkWorkflow(string(content)); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	content, err := os.ReadFile("go.mod")
	if err != nil {
		return err
	}
	if !goVersion.Match(content) {
		return fmt.Errorf("go.mod must pin an exact Go patch version")
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Workflow Action, Go tool and OS pins verified")
}
