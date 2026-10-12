//go:build windows

package app_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/app"
)

// installation writes a synthetic game tree and its manifest, and returns
// the manifest path and the installation directory.
func installation(t *testing.T, sources string) (string, string) {
	t.Helper()
	dir := filepath.Clean(t.TempDir())
	install := filepath.Join(dir, "Jeu synthétique")
	for name, content := range map[string]string{`saves\1.save`: "one", `saves\Profil été\2.save`: "two"} {
		path := filepath.Join(install, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	manifest := filepath.Join(dir, "manifest.yaml")
	content := fmt.Sprintf("schema_version: 1\ngame:\n  id: synthetic-game\n  name: Synthetic Game\n"+
		"installation:\n  path: '%s'\nsources:\n%s", install, sources)
	if err := os.WriteFile(manifest, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return manifest, install
}

const savesSource = "  - id: saves\n    kind: save\n    base: game-install\n    path: saves\n    required: true\n"

func runInspect(manifest string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := app.Run(context.Background(), []string{"inspect", "--manifest", manifest}, "dev", &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestInspectEndToEnd(t *testing.T) {
	manifest, install := installation(t, savesSource)
	code, out, errOut := runInspect(manifest)
	if code != 0 || errOut != "" {
		t.Fatalf("exit=%d stderr=%q\n%s", code, errOut, out)
	}
	for _, want := range []string{
		"installation: " + install + "\n",
		"source saves (save, required): " + install + `\saves` + "\n  status: inventoried\n  files: 2  bytes: 6\n",
		fmt.Sprintf("  %x  3  ", sha256.Sum256([]byte("one"))),
		"  1.save\n",
		"  Profil été/2.save\n",
		"\nfindings: none\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s", want, out)
		}
	}
	if _, again, _ := runInspect(manifest); again != out {
		t.Fatalf("two runs differ:\n%s\n---\n%s", out, again)
	}
}

func TestInspectReportsJunctionWithExitTwo(t *testing.T) {
	manifest, install := installation(t, savesSource)
	elsewhere := filepath.Join(filepath.Dir(install), "elsewhere")
	if err := os.Mkdir(elsewhere, 0o700); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(install, "saves", "jn"), elsewhere).CombinedOutput(); err != nil {
		t.Fatalf("mklink: %v %s", err, output)
	}
	code, out, errOut := runInspect(manifest)
	if code != 2 || errOut != "savewarden: inspection incomplete: 1 blocking finding(s)\n" {
		t.Fatalf("exit=%d stderr=%q", code, errOut)
	}
	if !strings.Contains(out, "files: 2  bytes: 6") || !strings.Contains(out, "  blocking  link-not-followed  saves  jn  junction or reparse point; not followed\n") {
		t.Fatalf("output:\n%s", out)
	}
	var stderr bytes.Buffer
	if code := app.Run(context.Background(), []string{"inspect", "--manifest", manifest}, "dev", failingWriter{}, &stderr); code != 1 {
		t.Fatalf("an unwritten report must exit 1, got %d: %q", code, stderr.String())
	}
}

func TestInspectOptionalSourceMissingIsInformation(t *testing.T) {
	manifest, _ := installation(t, savesSource+"  - id: extra\n    kind: other\n    base: game-install\n    path: extra\n    required: false\n")
	code, out, errOut := runInspect(manifest)
	if code != 0 || errOut != "" || !strings.Contains(out, "findings: 0 blocking, 1 info\n  info  source-missing  extra  ") {
		t.Fatalf("exit=%d stderr=%q\n%s", code, errOut, out)
	}
}
