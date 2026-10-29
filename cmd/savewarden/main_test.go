package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// buildBinary compiles the real CLI outside the directories under test.
func buildBinary(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	name := "savewarden"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(filepath.Clean(t.TempDir()), "bin", name)
	build := exec.Command("go", "build", "-trimpath", "-o", binary, "./cmd/savewarden")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	return binary
}

func TestBinarySmoke(t *testing.T) {
	binary := buildBinary(t)
	work := filepath.Join(t.TempDir(), "jeu synthétique avec espaces")
	if err := os.Mkdir(work, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(work, "synthetic.txt")
	if err := os.WriteFile(sentinel, []byte("synthetic fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--help"}, {"version"}, {"--version"}, {"sync"}, {"inspect"}} {
		cmd := exec.Command(binary, args...)
		cmd.Dir = work
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		if args[0] == "sync" || args[0] == "inspect" {
			var exit *exec.ExitError
			want := map[string]string{"sync": `required flag(s) "manifest", "vault" not set`, "inspect": `required flag(s) "manifest" not set`}[args[0]]
			if !errors.As(err, &exit) || exit.ExitCode() != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), want) {
				t.Fatalf("%v: err=%v stdout=%q stderr=%q", args, err, stdout.String(), stderr.String())
			}
		} else {
			if err != nil || stderr.Len() != 0 {
				t.Fatalf("%v: err=%v stderr=%q", args, err, stderr.String())
			}
			if args[0] == "--help" {
				if !strings.Contains(stdout.String(), "plans their synchronisation") || !strings.Contains(stdout.String(), "inspect") || !strings.Contains(stdout.String(), "sync") {
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
	if err != nil || len(entries) != 1 {
		t.Fatalf("unexpected working directory writes: %v, %v", entries, err)
	}
}

// fingerprint records every path, mode, size, time and content below root,
// reading each object itself rather than its parent's directory listing.
func fingerprint(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		line := fmt.Sprintf("%s|%v|%d|%d", path, info.Mode(), info.Size(), info.ModTime().UnixNano())
		if info.Mode().IsRegular() {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			line += fmt.Sprintf("|%x", sha256.Sum256(content))
		}
		lines = append(lines, line)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// denyWrites removes the rights to create, modify and delete children for
// everyone on dir and its content, where the OS allows it, and restores
// them at cleanup. Denying DELETE itself would also block reads on Windows,
// so deletions are caught by the fingerprint comparison instead.
func denyWrites(t *testing.T, dir string) bool {
	t.Helper()
	if runtime.GOOS != "windows" {
		if err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			mode := os.FileMode(0o444)
			if entry.IsDir() {
				mode = 0o555
			}
			return os.Chmod(path, mode)
		}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, _ error) error {
				if entry != nil && entry.IsDir() {
					return os.Chmod(path, 0o700) //nolint:gosec // G302: directories need the search bit to be removed at cleanup.
				}
				return nil
			})
		})
		return os.Geteuid() != 0
	}
	output, err := exec.Command("icacls", dir, "/deny", "*S-1-1-0:(OI)(CI)(WD,AD,WEA,WA,DC)").CombinedOutput()
	if err != nil {
		if os.Getenv("GITHUB_ACTIONS") == "true" {
			t.Fatalf("write denial is required on CI: %v %s", err, output)
		}
		t.Logf("write denial unavailable on %s: %v %s", dir, err, output)
		return false
	}
	t.Cleanup(func() {
		if output, err := exec.Command("icacls", dir, "/remove:d", "*S-1-1-0").CombinedOutput(); err != nil {
			t.Errorf("restore rights on %s: %v %s", dir, err, output)
		}
	})
	if err := os.WriteFile(filepath.Join(dir, "probe"), nil, 0o600); err == nil {
		t.Fatalf("write denial not effective on %s", dir)
	}
	return true
}

// assertReadOnlyRuns runs the real binary twice against a synthetic
// installation with an isolated environment and, where possible, with
// write access denied on the whole tree. Nothing may change, no file may
// appear and both runs must print the same output. args receives the
// temporary base directory and the manifest path.
func assertReadOnlyRuns(t *testing.T, args func(base, manifest string) []string, check func(t *testing.T, stdout, stderr string, err error)) {
	t.Helper()
	binary := buildBinary(t)
	base := filepath.Clean(t.TempDir())
	install := filepath.Join(base, "Jeu synthétique")
	for name, content := range map[string]string{"1.save": "one", filepath.Join("Profil été", "2.save"): "two"} {
		path := filepath.Join(install, "saves", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	installPath := install
	if runtime.GOOS != "windows" {
		installPath = `D:\Games\Jeu synthétique` // valid grammar, unavailable volume here
	}
	manifest := filepath.Join(base, "manifest.yaml")
	content := "schema_version: 1\ngame:\n  id: synthetic-game\n  name: Synthetic Game\ninstallation:\n  path: '" + installPath +
		"'\nsources:\n  - id: saves\n    kind: save\n    base: game-install\n    path: saves\n    required: true\n"
	if err := os.WriteFile(manifest, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(base, "dossier courant")
	isolated := map[string]string{}
	for _, name := range []string{"TEMP", "TMP", "TMPDIR", "APPDATA", "LOCALAPPDATA", "USERPROFILE", "HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME"} {
		isolated[name] = filepath.Join(base, "env", name)
	}
	for _, dir := range append([]string{work}, values(isolated)...) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	denied := denyWrites(t, base)
	if !denied && os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Fatal("write access must be denied during the run on CI")
	}
	t.Logf("write access denied during the run: %v", denied)
	before := fingerprint(t, base)

	var outputs []string
	for range 2 {
		cmd := exec.Command(binary, args(base, manifest)...)
		cmd.Dir = work
		cmd.Env = os.Environ()
		for name, dir := range isolated {
			cmd.Env = append(cmd.Env, name+"="+dir)
		}
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		check(t, stdout.String(), stderr.String(), err)
		outputs = append(outputs, stdout.String())
	}
	if outputs[0] != outputs[1] {
		t.Fatalf("two runs differ:\n%s\n---\n%s", outputs[0], outputs[1])
	}
	if after := fingerprint(t, base); after != before {
		t.Fatalf("the run changed the tree:\n%s\n---\n%s", before, after)
	}
}

// expectWindowsOnly checks the refusal of Windows volumes on other systems.
func expectWindowsOnly(t *testing.T, stdout, stderr string, err error) {
	t.Helper()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 || stdout != "" || !strings.Contains(stderr, "Windows installations only") {
		t.Fatalf("err=%v stdout=%q stderr=%q", err, stdout, stderr)
	}
}

func TestInspectDoesNotWrite(t *testing.T) {
	assertReadOnlyRuns(t, func(_, manifest string) []string { return []string{"inspect", "--manifest", manifest} },
		func(t *testing.T, stdout, stderr string, err error) {
			if runtime.GOOS != "windows" {
				expectWindowsOnly(t, stdout, stderr, err)
				return
			}
			if err != nil || stderr != "" || !strings.Contains(stdout, "files: 2  bytes: 6\n") || !strings.Contains(stdout, "findings: none\n") {
				t.Fatalf("err=%v stderr=%q stdout:\n%s", err, stderr, stdout)
			}
		})
}

// The plan mode of sync writes nothing: not the vault, not its parent, not
// the sources, not the working directory.
func TestSyncPlanDoesNotWrite(t *testing.T) {
	assertReadOnlyRuns(t, func(base, manifest string) []string {
		vault := `D:\Coffre synthétique`
		if runtime.GOOS == "windows" {
			vault = filepath.Join(base, "Coffre synthétique")
		}
		return []string{"sync", "--manifest", manifest, "--vault", vault}
	}, func(t *testing.T, stdout, stderr string, err error) {
		if runtime.GOOS != "windows" {
			expectWindowsOnly(t, stdout, stderr, err)
			return
		}
		if err != nil || stderr != "" {
			t.Fatalf("err=%v stderr=%q stdout:\n%s", err, stderr, stdout)
		}
		for _, want := range []string{"(will be created)\n", "mode: plan only, nothing written\nplan: changes\n", "added: 2", "same-volume"} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("stdout lacks %q:\n%s", want, stdout)
			}
		}
	})
}

func values(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
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
		`.\bin\savewarden.exe inspect --manifest '`,
		"docs/commands/inspect.md",
		`.\bin\savewarden.exe sync --manifest '`,
		"docs/commands/sync.md",
	} {
		if !strings.Contains(string(content), example) {
			t.Errorf("missing tested README example %q", example)
		}
	}
}
