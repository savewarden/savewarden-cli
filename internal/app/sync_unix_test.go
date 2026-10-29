//go:build unix

package app_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/app"
)

// sync supports Windows vaults only; elsewhere a valid manifest and vault
// must fail explicitly instead of resolving D:\ against the working directory.
func TestSyncRefusesWindowsVolumesOnUnix(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yaml")
	content := "schema_version: 1\ngame:\n  id: synthetic-game\n  name: Synthetic\ninstallation:\n  path: 'D:\\Games'\n" +
		"sources:\n  - id: saves\n    kind: save\n    base: game-install\n    path: saves\n    required: true\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := app.Run(context.Background(), []string{"sync", "--manifest", path, "--vault", `D:\Vault`}, "dev", &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "Windows installations only") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
		t.Fatalf("the refused plan wrote something: %v %v", entries, err)
	}
}
