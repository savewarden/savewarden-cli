//go:build windows

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var operationID = regexp.MustCompile(`[0-9]{8}T[0-9]{6}-[0-9a-f]{8}`)

// Plan B4 §9, non-regression: B4 changes neither sync nor snapshot. On the
// 23 steps of the B3 differential scenario, their outputs and those of
// inspect equal byte for byte the outputs recorded from a binary built at
// 0bb8693, whose code is that of the B3 closure f3074d0; operation ids, the
// base directory and its volume are masked.
func TestSyncAndSnapshotOutputsUnchanged(t *testing.T) {
	binary := buildBinary(t)
	base := filepath.Clean(t.TempDir())
	install := filepath.Join(base, "Synthetic Game")
	saves := filepath.Join(install, "saves")
	if err := os.MkdirAll(filepath.Join(saves, "Slot 1"), 0o700); err != nil {
		t.Fatal(err)
	}
	fixed := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	write := func(name, content string, at time.Time) {
		path := filepath.Join(saves, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
	write("1.save", "one", fixed)
	write(filepath.Join("Slot 1", "2.save"), "two", fixed)
	manifest := filepath.Join(base, "manifest.yaml")
	writeManifest(t, manifest, install)
	vault, inside := filepath.Join(base, "Vault"), filepath.Join(install, "Vault")
	sync := func(args ...string) []string {
		return append([]string{"sync", "--manifest", manifest, "--vault", vault}, args...)
	}
	snapshot := func(args ...string) []string {
		return append([]string{"snapshot", "--manifest", manifest, "--vault", vault}, args...)
	}
	steps := []struct {
		name string
		args []string
	}{
		{"sync plan, no vault", sync()},
		{"snapshot plan, no vault", snapshot()},
		{"sync apply, first publication", sync("--apply")},
		{"sync plan, up to date", sync()},
		{"sync apply, up to date", sync("--apply")},
		{"sync recover, nothing", sync("--recover")},
		{"snapshot plan", snapshot()},
		{"snapshot apply", snapshot("--apply")},
		{"snapshot plan, up to date", snapshot()},
		{"snapshot apply, up to date", snapshot("--apply")},
		{"snapshot recover", snapshot("--recover")},
		{"snapshot recover apply", snapshot("--recover", "--apply")},
		{"MODIFY", nil},
		{"sync plan, modified", sync()},
		{"sync apply, rotation", sync("--apply")},
		{"sync plan after rotation", sync()},
		{"sync recover apply, nothing", sync("--recover", "--apply")},
		{"snapshot plan after rotation", snapshot()},
		{"snapshot apply after rotation", snapshot("--apply")},
		{"snapshot plan, two snapshots", snapshot()},
		{"sync vault inside installation", []string{"sync", "--manifest", manifest, "--vault", inside}},
		{"snapshot vault inside installation", []string{"snapshot", "--manifest", manifest, "--vault", inside}},
		{"inspect", []string{"inspect", "--manifest", manifest}},
	}
	var out strings.Builder
	for _, step := range steps {
		if step.args == nil {
			write("1.save", "ONE", fixed.Add(5*time.Minute))
			out.WriteString("== modify 1.save\n")
			continue
		}
		cmd := exec.Command(binary, step.args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		code := exitCode(t, cmd.Run())
		fmt.Fprintf(&out, "== %s: exit %d\n-- stdout\n%s-- stderr\n%s", step.name, code, stdout.String(), stderr.String())
	}
	got := strings.ReplaceAll(out.String(), base, "<BASE>")
	got = strings.ReplaceAll(got, filepath.VolumeName(base)+`\`, `<VOLUME>\`)
	got = operationID.ReplaceAllString(got, "<OP>")
	want, err := os.ReadFile(filepath.Join("testdata", "b3-outputs.golden"))
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		gotLines, wantLines := strings.Split(got, "\n"), strings.Split(string(want), "\n")
		for i := range max(len(gotLines), len(wantLines)) {
			var g, w string
			if i < len(gotLines) {
				g = gotLines[i]
			}
			if i < len(wantLines) {
				w = wantLines[i]
			}
			if g != w {
				t.Fatalf("outputs differ from 0bb8693 at line %d:\n got %q\nwant %q", i+1, g, w)
			}
		}
	}
}
