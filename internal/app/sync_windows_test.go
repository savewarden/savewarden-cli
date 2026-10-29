//go:build windows

package app_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/app"
)

func runSync(manifest, vault string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := app.Run(context.Background(), []string{"sync", "--manifest", manifest, "--vault", vault}, "dev", &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestSyncPlanEndToEnd(t *testing.T) {
	manifest, install := installation(t, savesSource)
	vault := filepath.Join(filepath.Dir(install), "Coffre synthétique")
	code, out, errOut := runSync(manifest, vault)
	if code != 0 || errOut != "" {
		t.Fatalf("exit=%d stderr=%q\n%s", code, errOut, out)
	}
	for _, want := range []string{
		"vault: " + vault + " (will be created)\n",
		"destination: " + vault + `\games\synthetic-game\current` + "\n",
		"current: none\npending: none\nmode: plan only, nothing written\nplan: changes\n",
		"source saves (save, required): " + install + `\saves` + "\n  status: planned\n",
		"  files: 2  bytes: 6  added: 2  modified: 0  removed: 0  unchanged: 0\n",
		fmt.Sprintf("  + %x  3  ", sha256.Sum256([]byte("one"))),
		"  Profil été/2.save\n",
		"findings: 0 blocking, 1 info\n  info  same-volume  saves  ",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(vault); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the plan created the vault: %v", err)
	}
	if _, again, _ := runSync(manifest, vault); again != out {
		t.Fatalf("two plans differ:\n%s\n---\n%s", out, again)
	}
}

func TestSyncPlanExitCodes(t *testing.T) {
	manifest, install := installation(t, savesSource)
	base := filepath.Dir(install)
	foreign := filepath.Join(base, "Documents")
	interrupted := filepath.Join(base, "Coffre interrompu")
	for path, content := range map[string]string{
		filepath.Join(foreign, "photo.jpg"):                       "x",
		filepath.Join(interrupted, "savewarden-vault.json.tmp-1"): "{",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name  string
		vault string
		code  int
		err   string
		out   string
	}{
		{"foreign directory", foreign, 1, "not a Savewarden vault", ""},
		{"vault in the installation", filepath.Join(install, "Coffre"), 1, "overlaps the installation", ""},
		{"interrupted initialization", interrupted, 3, "savewarden: intervention required: initialization-interrupted .\n", "(initialization interrupted)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := runSync(manifest, tt.vault)
			if code != tt.code || !strings.Contains(errOut, tt.err) || strings.Count(errOut, "savewarden:") != 1 {
				t.Fatalf("exit=%d stderr=%q", code, errOut)
			}
			if (tt.out == "") != (out == "") || !strings.Contains(out, tt.out) {
				t.Fatalf("stdout:\n%s", out)
			}
		})
	}

	t.Run("junction in the source", func(t *testing.T) {
		elsewhere := filepath.Join(base, "elsewhere")
		if err := os.Mkdir(elsewhere, 0o700); err != nil {
			t.Fatal(err)
		}
		if output, err := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(install, "saves", "jn"), elsewhere).CombinedOutput(); err != nil {
			t.Fatalf("mklink: %v %s", err, output)
		}
		code, out, errOut := runSync(manifest, filepath.Join(base, "Coffre"))
		if code != 2 || errOut != "savewarden: plan blocked: 1 blocking finding(s)\n" ||
			!strings.Contains(out, "  blocking  link-not-followed  saves  jn  junction or reparse point; not followed\n") {
			t.Fatalf("exit=%d stderr=%q\n%s", code, errOut, out)
		}
	})
}
