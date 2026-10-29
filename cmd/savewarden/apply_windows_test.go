//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

type synthetic struct {
	binary, base, install, manifest, vault string
}

// newSynthetic lays out a synthetic installation with files files and a
// manifest, and names a vault that does not exist yet.
func newSynthetic(t *testing.T, binary string, files int) synthetic {
	t.Helper()
	base := filepath.Clean(t.TempDir())
	s := synthetic{binary: binary, base: base, install: filepath.Join(base, "Jeu synthétique"),
		manifest: filepath.Join(base, "manifest.yaml"), vault: filepath.Join(base, "Coffre synthétique")}
	contents := map[string]string{"1.save": "one", filepath.Join("Profil été", "2.save"): "two"}
	for i := range files {
		contents[filepath.Join("bulk", fmt.Sprintf("%04d.save", i))] = strings.Repeat("x", 4096)
	}
	for name, content := range contents {
		path := filepath.Join(s.install, "saves", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeManifest(t, s.manifest, s.install)
	return s
}

func writeManifest(t *testing.T, path, install string) {
	t.Helper()
	content := "schema_version: 1\ngame:\n  id: synthetic-game\n  name: Synthetic Game\ninstallation:\n  path: '" + install +
		"'\nsources:\n  - id: saves\n    kind: save\n    base: game-install\n    path: saves\n    required: true\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (s synthetic) run(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(s.binary, append([]string{"sync", "--manifest", s.manifest, "--vault", s.vault}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return stdout.String(), stderr.String(), code
}

var publishedLine = regexp.MustCompile(`^savewarden: vault: published ([0-9]{8}T[0-9]{6}-[0-9a-f]{8})\n$`)

// Plan B2 §9.2, binary: --apply with write access denied on the source and
// the installation publishes, leaves the source identical, and a second
// --apply finds the vault up to date; inspect reads current with the
// SHA-256 of the receipt.
func TestSyncApplyEndToEnd(t *testing.T) {
	s := newSynthetic(t, buildBinary(t), 0)
	denied := denyWrites(t, s.install)
	t.Logf("write access denied on the installation: %v", denied)
	source := fingerprint(t, s.install)

	stdout, stderr, code := s.run(t, "--apply")
	match := publishedLine.FindStringSubmatch(stderr)
	if code != 0 || match == nil || !strings.Contains(stdout, "verification: 2 files, 6 bytes, SHA-256 verified\n") {
		t.Fatalf("exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	op := match[1]
	if fingerprint(t, s.install) != source {
		t.Fatal("the source changed")
	}
	stdout, stderr, code = s.run(t, "--apply")
	if code != 0 || stderr != "savewarden: vault: not modified\n" || !strings.Contains(stdout, "plan: up to date\n") {
		t.Fatalf("second apply: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}

	game := filepath.Join(s.vault, "games", "synthetic-game")
	data, err := os.ReadFile(filepath.Join(game, "receipts", op+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Sources []struct {
			Entries []struct {
				Path   string `json:"path"`
				SHA256 string `json:"sha256"`
			} `json:"entries"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(data, &receipt); err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(s.base, "current.yaml")
	writeManifest(t, current, filepath.Join(game, "current"))
	inspected, err := exec.Command(s.binary, "inspect", "--manifest", current).Output()
	if err != nil {
		t.Fatalf("inspect current: %v\n%s", err, inspected)
	}
	for _, entry := range receipt.Sources[0].Entries {
		if !strings.Contains(string(inspected), entry.SHA256) {
			t.Fatalf("inspect of current lacks %s %s:\n%s", entry.SHA256, entry.Path, inspected)
		}
	}
}

// receiptEntries reads the SHA-256 of every file a receipt describes.
func receiptEntries(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Sources []struct {
			Entries []struct {
				SHA256 string `json:"sha256"`
			} `json:"entries"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(data, &receipt); err != nil {
		t.Fatal(err)
	}
	var sums []string
	for _, entry := range receipt.Sources[0].Entries {
		sums = append(sums, entry.SHA256)
	}
	return sums
}

// Plan B2 §9.3, binary: once the source changed, --apply rotates current
// into retained on a real NTFS volume; inspect reads the retained
// generation with the SHA-256 of its receipt; a third --apply is up to
// date; a current modified by hand blocks in code 2, nothing moved.
func TestRotationEndToEnd(t *testing.T) {
	s := newSynthetic(t, buildBinary(t), 0)
	_, stderr, code := s.run(t, "--apply")
	first := publishedLine.FindStringSubmatch(stderr)
	if code != 0 || first == nil {
		t.Fatalf("first apply: %d %q", code, stderr)
	}
	if err := os.WriteFile(filepath.Join(s.install, "saves", "1.save"), []byte("ONE"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := s.run(t, "--apply")
	second := publishedLine.FindStringSubmatch(stderr)
	if code != 0 || second == nil || second[1] == first[1] {
		t.Fatalf("rotation: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	game := filepath.Join(s.vault, "games", "synthetic-game")
	if data, err := os.ReadFile(filepath.Join(game, "current", "saves", "1.save")); err != nil || string(data) != "ONE" {
		t.Fatalf("current: %q, %v", data, err)
	}
	retained := filepath.Join(game, "retained", first[1])
	manifest := filepath.Join(s.base, "retained.yaml")
	writeManifest(t, manifest, retained)
	inspected, err := exec.Command(s.binary, "inspect", "--manifest", manifest).Output()
	if err != nil {
		t.Fatalf("inspect retained: %v\n%s", err, inspected)
	}
	for _, sum := range receiptEntries(t, filepath.Join(game, "receipts", first[1]+".json")) {
		if !strings.Contains(string(inspected), sum) {
			t.Fatalf("inspect of the retained generation lacks %s:\n%s", sum, inspected)
		}
	}
	if _, stderr, code := s.run(t, "--apply"); code != 0 || stderr != "savewarden: vault: not modified\n" {
		t.Fatalf("third apply: exit %d, stderr %q", code, stderr)
	}
	if err := os.WriteFile(filepath.Join(game, "current", "saves", "1.save"), []byte("one by hand"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := fingerprint(t, s.vault)
	if _, stderr, code := s.run(t, "--apply"); code != 2 || !strings.HasSuffix(stderr, "savewarden: vault: not modified\n") {
		t.Fatalf("modified current: exit %d, stderr %q", code, stderr)
	}
	if fingerprint(t, s.vault) != before {
		t.Fatal("the blocked apply changed the vault")
	}
}

// A process killed during the staging of a large synthetic source leaves
// an operation that --recover shows and --recover --apply closes; a new
// --apply then publishes.
func TestSyncKilledDuringStagingThenRecovered(t *testing.T) {
	s := newSynthetic(t, buildBinary(t), 2000)
	cmd := exec.Command(s.binary, "sync", "--manifest", s.manifest, "--vault", s.vault, "--apply")
	var childErr bytes.Buffer
	cmd.Stderr = &childErr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	staging := filepath.Join(s.vault, "games", "synthetic-game", "staging")
	deadline := time.After(60 * time.Second)
	killed := false
	for !killed {
		select {
		case err := <-done:
			t.Skipf("the copy ended before it could be interrupted: %v %s", err, childErr.String())
		case <-deadline:
			_ = cmd.Process.Kill()
			t.Fatal("the copy did not start within a minute")
		default:
		}
		if entries, _ := filepath.Glob(filepath.Join(staging, "*", "saves", "bulk", "*.save")); len(entries) > 10 {
			killed = cmd.Process.Kill() == nil
		}
		time.Sleep(time.Millisecond)
	}
	<-done
	stdout, stderr, code := s.run(t, "--recover")
	if code != 0 || !strings.Contains(stdout, "action: record abandoned") || stderr != "savewarden: vault: not modified\n" {
		t.Fatalf("recover plan: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	stdout, stderr, code = s.run(t, "--recover", "--apply")
	if code != 0 || !strings.Contains(stderr, "kept in staging") {
		t.Fatalf("recover: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	if _, stderr, code := s.run(t, "--apply"); code != 0 || publishedLine.FindStringSubmatch(stderr) == nil {
		t.Fatalf("apply after recovery: exit %d, stderr %q", code, stderr)
	}
}

// The policy flags on a real vault: a removed file blocks in code 2 until
// --allow-removals, which keeps it in retained; --recover refuses them in
// code 1 without writing.
func TestRotationFlagsEndToEnd(t *testing.T) {
	s := newSynthetic(t, buildBinary(t), 0)
	_, stderr, code := s.run(t, "--apply")
	first := publishedLine.FindStringSubmatch(stderr)
	if code != 0 || first == nil {
		t.Fatalf("first apply: %d %q", code, stderr)
	}
	if err := os.Remove(filepath.Join(s.install, "saves", "1.save")); err != nil {
		t.Fatal(err)
	}
	before := fingerprint(t, s.vault)
	if stdout, stderr, code := s.run(t, "--apply"); code != 2 || !strings.Contains(stdout, "blocking  files-removed  saves  1.save") {
		t.Fatalf("removal without the flag: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	if _, stderr, code := s.run(t, "--recover", "--allow-removals"); code != 1 || !strings.Contains(stderr, "--recover takes neither") {
		t.Fatalf("--recover with a policy flag: exit %d, stderr %q", code, stderr)
	}
	if _, stderr, code := s.run(t, "--apply", "--accept-missing-source", "saves"); code != 1 || !strings.Contains(stderr, "is required") {
		t.Fatalf("invalid --accept-missing-source: exit %d, stderr %q", code, stderr)
	}
	if fingerprint(t, s.vault) != before {
		t.Fatal("a refused run changed the vault")
	}
	stdout, stderr, code := s.run(t, "--apply", "--allow-removals")
	if code != 0 || publishedLine.FindStringSubmatch(stderr) == nil || !strings.Contains(stdout, "info  files-removed  saves  1.save") {
		t.Fatalf("removal with the flag: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	game := filepath.Join(s.vault, "games", "synthetic-game")
	if _, err := os.Stat(filepath.Join(game, "current", "saves", "1.save")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the removed file is still current: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(game, "retained", first[1], "saves", "1.save")); err != nil || string(data) != "one" {
		t.Fatalf("the removed file is not retained: %q, %v", data, err)
	}
}
