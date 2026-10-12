package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBinarySmoke(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(t.TempDir(), "jeu synthétique avec espaces")
	if err := os.Mkdir(work, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(work, "synthetic.txt")
	if err := os.WriteFile(sentinel, []byte("synthetic fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	name := "savewarden"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(work, name)
	build := exec.Command("go", "build", "-trimpath", "-o", binary, "./cmd/savewarden")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	for _, args := range [][]string{{"--help"}, {"version"}, {"--version"}, {"sync"}} {
		cmd := exec.Command(binary, args...)
		cmd.Dir = work
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		if args[0] == "sync" {
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "unknown command") {
				t.Fatalf("%v: err=%v stdout=%q stderr=%q", args, err, stdout.String(), stderr.String())
			}
		} else {
			if err != nil || stderr.Len() != 0 {
				t.Fatalf("%v: err=%v stderr=%q", args, err, stderr.String())
			}
			if args[0] == "--help" {
				if !strings.Contains(stdout.String(), "Bootstrap B0") || !strings.Contains(stdout.String(), "version") {
					t.Fatalf("help: %q", stdout.String())
				}
			} else if stdout.String() != "savewarden dev\n" {
				t.Fatalf("version: %q", stdout.String())
			}
		}
	}
	content, err := os.ReadFile(sentinel)
	if err != nil || string(content) != "synthetic fixture\n" {
		t.Fatalf("fixture changed: %q, %v", content, err)
	}
	entries, err := os.ReadDir(work)
	if err != nil || len(entries) != 2 {
		t.Fatalf("unexpected working directory writes: %v, %v", entries, err)
	}
}

func TestReadmeCommandExamples(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, example := range []string{
		"go run ./cmd/savewarden --help",
		"go run ./cmd/savewarden version",
		"go build -trimpath -o bin/savewarden ./cmd/savewarden",
		"savewarden dev\n",
	} {
		if !strings.Contains(string(content), example) {
			t.Errorf("missing tested README example %q", example)
		}
	}
}
