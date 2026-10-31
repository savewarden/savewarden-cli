package app_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/app"
)

func TestCLIContract(t *testing.T) {
	tests := []struct {
		name string
		args []string
		code int
		out  string
		err  string
		// state: the run ends with the vault state line, after its error.
		state bool
	}{
		{name: "default help", out: "inspects declared save sources, plans their synchronisation"},
		{name: "help lists inspect", args: []string{"--help"}, out: "inspect     Inventory the save sources of a manifest, read-only"},
		{name: "help lists sync", args: []string{"--help"}, out: "sync        Plan, apply or recover the copy of a manifest's sources into a vault"},
		{name: "sync help", args: []string{"sync", "--help"}, out: "Exit codes: 0 success: plan without blocking findings, published"},
		{name: "sync without flags", args: []string{"sync"}, code: 1, err: `required flag(s) "manifest", "vault" not set`},
		{name: "sync apply missing manifest", args: []string{"sync", "--manifest", "missing-manifest.yaml", "--vault", `D:\Vault`, "--apply"},
			code: 1, err: "missing-manifest.yaml", state: true},
		{name: "sync recover missing manifest", args: []string{"sync", "--manifest", "missing-manifest.yaml", "--vault", `D:\Vault`, "--recover"},
			code: 1, err: "missing-manifest.yaml", state: true},
		{name: "sync recover refuses policy flags", args: []string{"sync", "--manifest", "m.yaml", "--vault", `D:\Vault`, "--recover", "--allow-removals"},
			code: 1, err: "--recover takes neither --allow-removals nor --accept-missing-source"},
		{name: "sync missing manifest", args: []string{"sync", "--manifest", "missing-manifest.yaml", "--vault", `D:\Vault`}, code: 1, err: "missing-manifest.yaml"},
		{name: "help lists snapshot", args: []string{"--help"}, out: "snapshot    Plan an immutable snapshot of the current generation of a vault"},
		{name: "snapshot help", args: []string{"snapshot", "--help"}, out: "the state saved in the vault, not necessarily the"},
		{name: "snapshot without flags", args: []string{"snapshot"}, code: 1, err: `required flag(s) "manifest", "vault" not set`},
		{name: "snapshot missing manifest", args: []string{"snapshot", "--manifest", "missing-manifest.yaml", "--vault", `D:\Vault`}, code: 1, err: "missing-manifest.yaml"},
		{name: "snapshot apply is not in this build", args: []string{"snapshot", "--manifest", "m.yaml", "--vault", `D:\Vault`, "--apply"}, code: 1, err: "unknown flag"},
		{name: "inspect help", args: []string{"inspect", "--help"}, out: "Exit codes: 0 complete inventory"},
		{name: "inspect without manifest", args: []string{"inspect"}, code: 1, err: `required flag(s) "manifest" not set`},
		{name: "inspect missing manifest", args: []string{"inspect", "--manifest", "missing-manifest.yaml"}, code: 1, err: "missing-manifest.yaml"},
		{name: "long help", args: []string{"--help"}, out: "Usage:"},
		{name: "short help", args: []string{"-h"}, out: "Usage:"},
		{name: "help command", args: []string{"help"}, out: "Usage:"},
		{name: "version help", args: []string{"version", "--help"}, out: "Print the build version"},
		{name: "version", args: []string{"version"}, out: "savewarden dev\n"},
		{name: "version flag", args: []string{"--version"}, out: "savewarden dev\n"},
		{name: "short version flag", args: []string{"-v"}, out: "savewarden dev\n"},
		{name: "unknown command", args: []string{"upload"}, code: 1, err: "unknown command"},
		{name: "unknown flag", args: []string{"--manifest", "anything.yaml"}, code: 1, err: "unknown flag"},
		{name: "version arguments", args: []string{"version", "extra"}, code: 1, err: "unknown command"},
		{name: "version flag error", args: []string{"version", "--json"}, code: 1, err: "unknown flag"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := app.Run(context.Background(), tt.args, "dev", &stdout, &stderr)
			if code != tt.code {
				t.Fatalf("exit = %d, want %d; stderr = %q", code, tt.code, stderr.String())
			}
			if tt.err != "" {
				if stdout.Len() != 0 || !strings.Contains(stderr.String(), tt.err) {
					t.Fatalf("stdout = %q; stderr = %q", stdout.String(), stderr.String())
				}
				lines := 1
				if tt.state {
					lines = 2
					if !strings.HasSuffix(stderr.String(), "\nsavewarden: vault: not modified\n") {
						t.Fatalf("no vault state line last: %q", stderr.String())
					}
				}
				if strings.Count(stderr.String(), "savewarden:") != lines || strings.Contains(stderr.String(), "Usage:") {
					t.Fatalf("error printed more than once or with usage: %q", stderr.String())
				}
			} else if stderr.Len() != 0 || !strings.Contains(stdout.String(), tt.out) {
				t.Fatalf("stdout = %q; stderr = %q", stdout.String(), stderr.String())
			}
			if tt.out == "savewarden dev\n" && stdout.String() != tt.out {
				t.Fatalf("version must be exact: %q", stdout.String())
			}
		})
	}
}

func TestFreshCommandsAndInjectedVersion(t *testing.T) {
	for range 2 {
		var stdout, stderr bytes.Buffer
		if code := app.Run(context.Background(), []string{"version"}, "v0.1.0-a1", &stdout, &stderr); code != 0 {
			t.Fatalf("exit = %d; stderr = %q", code, stderr.String())
		}
		if stdout.String() != "savewarden v0.1.0-a1\n" {
			t.Fatalf("version = %q", stdout.String())
		}
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }

func TestOutputFailure(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}, {"--help"}, {}} {
		var stderr bytes.Buffer
		if code := app.Run(context.Background(), args, "dev", failingWriter{}, &stderr); code != 1 {
			t.Fatalf("%v: exit = %d, want 1", args, code)
		}
		if !strings.Contains(stderr.String(), "output unavailable") {
			t.Fatalf("%v: stderr = %q", args, stderr.String())
		}
	}
}

type shortWriter struct{}

func (shortWriter) Write(data []byte) (int, error) { return len(data) - 1, nil }

func TestShortOutput(t *testing.T) {
	var stderr bytes.Buffer
	if code := app.Run(context.Background(), []string{"version"}, "dev", shortWriter{}, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "short write") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestHelpIsDeterministic(t *testing.T) {
	var first bytes.Buffer
	if code := app.Run(context.Background(), []string{"--help"}, "dev", &first, &bytes.Buffer{}); code != 0 {
		t.Fatalf("help exit = %d", code)
	}
	for _, args := range [][]string{nil, {"--help"}, {"-h"}, {"help"}} {
		var stdout, stderr bytes.Buffer
		if code := app.Run(context.Background(), args, "dev", &stdout, &stderr); code != 0 || stdout.String() != first.String() {
			t.Fatalf("%v: help differs: exit=%d stdout=%q stderr=%q", args, code, stdout.String(), stderr.String())
		}
	}
}

func TestInspectRejectsInvalidManifestWithoutReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.yaml")
	content := "schema_version: 1\ngame:\n  id: Synthetic\n  name: x\ninstallation:\n  path: 'D:\\Games'\n" +
		"sources:\n  - id: saves\n    kind: save\n    base: game-install\n    path: saves\n    required: true\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := app.Run(context.Background(), []string{"inspect", "--manifest", path}, "dev", &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "manifest.yaml:3:7: game.id:") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if strings.Count(stderr.String(), "savewarden:") != 1 {
		t.Fatalf("diagnostic printed more than once: %q", stderr.String())
	}
}
