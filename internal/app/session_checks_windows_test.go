//go:build windows

package app

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

var testDate = time.Date(2026, 11, 5, 10, 15, 0, 0, time.UTC)

// The checks under the session of plan B5 §4.3 (T4), on real Windows with
// the real adapters. Until S-02, the plan still compares the vault with the
// installation by letter: a vault named through a subst alias passes it,
// and only the session, on the chain it holds, refuses it.

// t4Installation writes a synthetic installation below base, with its
// manifest, and returns the manifest path.
func t4Installation(t *testing.T, base, install string, required bool) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(base, "manifests"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(base, "manifests", "manifest.yaml")
	req := "true"
	if !required {
		req = "false"
	}
	content := "schema_version: 1\ngame:\n  id: synthetic-game\n  name: Synthetic Game\ninstallation:\n  path: '" + install +
		"'\nsources:\n  - id: saves\n    kind: save\n    base: game-install\n    path: saves\n    required: " + req + "\n"
	if err := os.WriteFile(manifest, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func t4Run(args ...string) (int, string) {
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), args, "dev", &stdout, &stderr)
	return code, stderr.String()
}

// t4Saves creates the installation with its source.
func t4Saves(t *testing.T, install string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(install, "saves"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(install, "saves", "1.save"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A vault, and a restore area, absent beside the installation are
// accepted: the parent only anchors the absent target.
func TestAbsentTargetBesideAReferenceIsAcceptedOnWindows(t *testing.T) {
	base := filepath.Clean(t.TempDir())
	install := filepath.Join(base, "Jeu synthétique")
	t4Saves(t, install)
	manifest := t4Installation(t, base, install, true)
	vault := filepath.Join(base, "Coffre")
	if code, errOut := t4Run("sync", "--manifest", manifest, "--vault", vault, "--apply"); code != 0 {
		t.Fatalf("sync: exit %d, %q", code, errOut)
	}
	code, errOut := t4Run("snapshot", "--manifest", manifest, "--vault", vault, "--apply")
	if code != 0 {
		t.Fatalf("snapshot: exit %d, %q", code, errOut)
	}
	published := regexp.MustCompile(`published snapshot (\S+)\n`).FindStringSubmatch(errOut)
	if published == nil {
		t.Fatalf("no snapshot published: %q", errOut)
	}
	area := filepath.Join(base, "Restauration")
	if code, errOut := t4Run("restore", "--manifest", manifest, "--vault", vault, "--snapshot", published[1], "--to", area, "--apply"); code != 0 {
		t.Fatalf("restore: exit %d, %q", code, errOut)
	}
}

// A vault absent below the installation, named through a subst alias of
// its parent, passes the plan and is refused under the session, code 1:
// nothing is created.
func TestAbsentTargetUnderAReferenceIsRefusedOnWindows(t *testing.T) {
	base := filepath.Clean(t.TempDir())
	install := filepath.Join(base, "Jeu synthétique")
	t4Saves(t, install)
	manifest := t4Installation(t, base, install, true)
	drive := freeAlias(t, base)
	for _, vault := range []string{drive + `\Jeu synthétique\Coffre`, drive + `\Jeu synthétique\saves\Coffre`} {
		source := tree(t, install)
		if code, errOut := t4Run("sync", "--manifest", manifest, "--vault", vault); code != 0 {
			t.Fatalf("the plan refused already, the session guard is not the one tested: exit %d, %q", code, errOut)
		}
		code, errOut := t4Run("sync", "--manifest", manifest, "--vault", vault, "--apply")
		if code != 1 || !strings.Contains(errOut, "overlaps the installation "+install) {
			t.Fatalf("%s: exit %d, %q", vault, code, errOut)
		}
		if _, err := os.Lstat(vault); !errors.Is(err, fs.ErrNotExist) || tree(t, install) != source {
			t.Fatalf("%s: something was written: %v", vault, err)
		}
	}
}

// An existing vault that contains the installation, absent, named through
// a subst alias, is refused under the session, code 1, nothing written.
func TestExistingTargetContainingAReferenceIsRefusedOnWindows(t *testing.T) {
	base := filepath.Clean(t.TempDir())
	box := filepath.Join(base, "Box")
	if err := os.Mkdir(box, 0o700); err != nil {
		t.Fatal(err)
	}
	marker, err := backup.EncodeMarker(backup.Marker{CreatedAt: testDate, ToolVersion: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(box, backup.MarkerName), marker, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := t4Installation(t, base, filepath.Join(box, "Jeu absent"), false)
	drive := freeAlias(t, base)
	vault := drive + `\Box`
	before := tree(t, box)
	if code, errOut := t4Run("sync", "--manifest", manifest, "--vault", vault); code != 0 {
		t.Fatalf("the plan refused already: exit %d, %q", code, errOut)
	}
	code, errOut := t4Run("sync", "--manifest", manifest, "--vault", vault, "--apply")
	if code != 1 || !strings.Contains(errOut, "overlaps the installation "+filepath.Join(box, "Jeu absent")) {
		t.Fatalf("exit %d, %q", code, errOut)
	}
	if tree(t, box) != before {
		t.Fatal("something was written in the vault")
	}
}
