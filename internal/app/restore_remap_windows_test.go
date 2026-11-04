//go:build windows

package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/features/inspect"
	"github.com/savewarden/savewarden-cli/internal/features/sync"
	"github.com/savewarden/savewarden-cli/internal/platform/manifests"
)

// remapVaults gives the drive letter of a subst alias to another directory
// just before it opens a real session, as a hostile or careless program
// could between the plan of a restore and its first write.
type remapVaults struct {
	t      *testing.T
	drive  string
	target string
}

func (r remapVaults) Open(path string) (sync.Vault, error) {
	r.t.Helper()
	if output, err := exec.Command("subst", r.drive, "/D").CombinedOutput(); err != nil {
		r.t.Fatalf("remove the alias %s: %v %s", r.drive, err, output)
	}
	if output, err := exec.Command("subst", r.drive, r.target).CombinedOutput(); err != nil {
		r.t.Fatalf("remap the alias %s: %v %s", r.drive, err, output)
	}
	return vaults{}.Open(path)
}

// freeAlias gives a free drive letter to dir with subst, removed at
// cleanup. On CI, an alias that cannot be given is a failure.
func freeAlias(t *testing.T, dir string) string {
	t.Helper()
	for letter := 'Z'; letter >= 'M'; letter-- {
		drive := string(letter) + ":"
		if _, err := os.Stat(drive + `\`); err == nil {
			continue
		}
		if exec.Command("subst", drive, dir).Run() != nil {
			continue
		}
		t.Cleanup(func() { _ = exec.Command("subst", drive, "/D").Run() })
		return drive
	}
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Fatal("no drive letter could be given by subst on CI")
	}
	t.Skip("no drive letter could be given by subst")
	return ""
}

// tree lists every path, size and content below root.
func tree(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		line := path
		if entry.Type().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			line += fmt.Sprintf("|%x", sha256.Sum256(data))
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

// Security review P1-02, on real Windows with the real adapters: the drive
// letter of the destination is given to a source directory between the
// plan and the opening of the session. The separation, checked again once
// the session is open and before its first write, refuses the restore and
// its recovery, code 1: the source stays identical. The limit that remains,
// a letter remapped while the session is open, is written in the safety
// model.
func TestRestoreRefusesALetterRemappedBeforeTheSession(t *testing.T) {
	base := filepath.Clean(t.TempDir())
	install := filepath.Join(base, "Jeu synthétique")
	saves := filepath.Join(install, "saves")
	neutral := filepath.Join(base, "Neutre")
	for _, dir := range []string{saves, neutral} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(saves, "1.save"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(base, "manifest.yaml")
	content := "schema_version: 1\ngame:\n  id: synthetic-game\n  name: Synthetic Game\ninstallation:\n  path: '" + install +
		"'\nsources:\n  - id: saves\n    kind: save\n    base: game-install\n    path: saves\n    required: true\n"
	if err := os.WriteFile(manifest, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	vault := filepath.Join(base, "Coffre")
	run := func(args ...string) string {
		var stdout, stderr bytes.Buffer
		if code := Run(context.Background(), args, "dev", &stdout, &stderr); code != 0 {
			t.Fatalf("%q: exit %d, stderr %q", args, code, stderr.String())
		}
		return stderr.String()
	}
	run("sync", "--manifest", manifest, "--vault", vault, "--apply")
	published := regexp.MustCompile(`published snapshot (\S+)\n`).FindStringSubmatch(run("snapshot", "--manifest", manifest, "--vault", vault, "--apply"))
	if published == nil {
		t.Fatal("no snapshot published")
	}
	drive := freeAlias(t, neutral)
	inspector := inspect.New(manifests.Reader{}, scopes{})
	service := sync.New(manifests.Reader{}, inspector, syncScopes{}, volumes{})
	writer := sync.Writer{Vaults: remapVaults{t: t, drive: drive, target: saves}, Clock: systemClock{}, Random: rand.Reader, ToolVersion: "dev"}
	restore := func(dest string, recover bool) error {
		// Back to the neutral directory before each plan.
		_ = exec.Command("subst", drive, "/D").Run()
		if output, err := exec.Command("subst", drive, neutral).CombinedOutput(); err != nil {
			t.Fatalf("alias: %v %s", err, output)
		}
		request := sync.RestoreRequest{ManifestPath: manifest, VaultPath: vault, Snapshot: published[1], Destination: dest}
		if recover {
			request.VaultPath, request.Snapshot = "", ""
			_, err := service.RestoreRecover(context.Background(), request, writer)
			return err
		}
		_, err := service.RestoreApply(context.Background(), request, writer)
		return err
	}
	for _, tt := range []struct {
		name    string
		prepare func(dir string)
		recover bool
	}{
		{"new area, Z2", func(string) {}, false},
		{"empty directory, Z3", func(dir string) { _ = os.Mkdir(filepath.Join(dir, "Restauration"), 0o700) }, false},
		{"resumed marker", func(dir string) {
			_ = os.Mkdir(filepath.Join(dir, "Restauration"), 0o700)
			_ = os.WriteFile(filepath.Join(dir, "Restauration", "savewarden-restore.json.tmp-1"), []byte("{"), 0o600)
		}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, dir := range []string{neutral, saves} {
				_ = os.RemoveAll(filepath.Join(dir, "Restauration"))
				tt.prepare(dir)
			}
			source := tree(t, saves)
			err := restore(drive+`\Restauration`, tt.recover)
			if err == nil || !strings.Contains(err.Error(), "overlaps the source saves") {
				t.Fatalf("restore after the remapping: %v", err)
			}
			if tree(t, saves) != source {
				t.Fatal("the session wrote in the source")
			}
		})
	}
}
