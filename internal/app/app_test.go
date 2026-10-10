package app_test

import (
	"bytes"
	"context"
	"errors"
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
	}{
		{name: "default help", out: "Bootstrap B0"},
		{name: "long help", args: []string{"--help"}, out: "Usage:"},
		{name: "short help", args: []string{"-h"}, out: "Usage:"},
		{name: "help command", args: []string{"help"}, out: "Usage:"},
		{name: "version help", args: []string{"version", "--help"}, out: "Print the build version"},
		{name: "version", args: []string{"version"}, out: "savewarden dev\n"},
		{name: "version flag", args: []string{"--version"}, out: "savewarden dev\n"},
		{name: "short version flag", args: []string{"-v"}, out: "savewarden dev\n"},
		{name: "unknown command", args: []string{"sync"}, code: 1, err: "unknown command"},
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
				if strings.Count(stderr.String(), "savewarden:") != 1 || strings.Contains(stderr.String(), "Usage:") {
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
