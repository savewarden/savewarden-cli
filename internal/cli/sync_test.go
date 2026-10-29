package cli_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/cli"
	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/entities/game"
	"github.com/savewarden/savewarden-cli/internal/features/sync"
)

type fakePlanner struct {
	plan    sync.Plan
	err     error
	request sync.Request
}

func (f *fakePlanner) Plan(_ context.Context, request sync.Request) (sync.Plan, error) {
	f.request = request
	return f.plan, f.err
}

func runSync(t *testing.T, planner cli.Planner, args ...string) (string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	root := cli.NewRootCommand("dev", &stdout, &stderr, nil, planner)
	root.SetArgs(append([]string{"sync"}, args...))
	err := root.ExecuteContext(context.Background())
	return stdout.String(), err
}

func firstPlan() sync.Plan {
	entries := []backup.Entry{
		{Path: "1.save", Size: 3, ModTime: modTime, SHA256: sha256.Sum256([]byte("one"))},
		{Path: "Slot 1/2.save", Size: 3, ModTime: modTime, SHA256: sha256.Sum256([]byte("two"))},
	}
	return sync.Plan{
		ManifestPath: `D:\Manifests\synthetic-game.yaml`,
		Game:         game.Game{ID: "synthetic-game", Name: "Synthetic Game"},
		Vault:        `D:\Vault`,
		VaultState:   sync.VaultAbsent,
		Destination:  `D:\Vault\games\synthetic-game\current`,
		Sources: []sync.SourcePlan{{
			Source:  game.Source{ID: "saves", Kind: game.KindSave, Required: true},
			Path:    `D:\Games\Synthetic Game\saves`,
			Status:  sync.SourcePlanned,
			Changes: backup.Compare(nil, nil, entries, []string{"Slot 1"}),
			Files:   2,
			Bytes:   6,
		}},
		Findings: []backup.Finding{{Severity: backup.Info, Code: sync.CodeSameVolume, Source: "saves", Path: `D:\`,
			Detail: "vault and source share a volume; not an off-disk backup"}},
	}
}

func TestRenderFirstPlan(t *testing.T) {
	planner := &fakePlanner{plan: firstPlan()}
	out, err := runSync(t, planner, "--manifest", `D:\Manifests\synthetic-game.yaml`, "--vault", `D:\Vault`)
	if err != nil {
		t.Fatal(err)
	}
	if planner.request != (sync.Request{ManifestPath: `D:\Manifests\synthetic-game.yaml`, VaultPath: `D:\Vault`}) {
		t.Fatalf("request = %+v", planner.request)
	}
	want := `manifest: D:\Manifests\synthetic-game.yaml
game: synthetic-game (Synthetic Game)
vault: D:\Vault (will be created)
destination: D:\Vault\games\synthetic-game\current
volume: not checked by this build
current: none
pending: none
mode: plan only, nothing written
plan: changes

source saves (save, required): D:\Games\Synthetic Game\saves
  status: planned
  files: 2  bytes: 6  added: 2  modified: 0  removed: 0  unchanged: 0
  + 7692c3ad3540bb803c020b3aee66cd8887123234ea0c6e7143c0add73ff431ed  3  2026-10-09T16:32:08.1234567Z  1.save
  + 3fc4ccfe745870e2c0d99f71f30ff0656c8dedd41cc1d7d3d376b0dbe685e2f3  3  2026-10-09T16:32:08.1234567Z  Slot 1/2.save

findings: 0 blocking, 1 info
  info  same-volume  saves  D:\  vault and source share a volume; not an off-disk backup
`
	if out != want {
		t.Fatalf("output:\n%s\nwant:\n%s", out, want)
	}
}

func TestRenderUpToDateAndBlockedPlans(t *testing.T) {
	plan := firstPlan()
	plan.VaultState, plan.Current, plan.Findings = sync.VaultInitialized, "20261105T101500-1a2b3c4d", nil
	entries := []backup.Entry{{Path: "1.save", Size: 3, ModTime: modTime, SHA256: sha256.Sum256([]byte("one"))}}
	plan.Sources[0].Changes = backup.Compare(entries, nil, entries, nil)
	out, err := runSync(t, &fakePlanner{plan: plan}, "--manifest", "m.yaml", "--vault", `D:\Vault`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"vault: D:\\Vault (initialized)\n", "current: 20261105T101500-1a2b3c4d\n", "plan: up to date\n",
		"  = 7692c3ad", "\nfindings: none\n"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s", want, out)
		}
	}

	plan.Findings = []backup.Finding{{Severity: backup.Blocking, Code: sync.CodeSourceRemoved, Source: "profile",
		Detail: "in the receipt of the current generation but no longer in the manifest"}}
	out, err = runSync(t, &fakePlanner{plan: plan}, "--manifest", "m.yaml", "--vault", `D:\Vault`)
	var exit *cli.ExitError
	if !errors.As(err, &exit) || exit.Code != cli.ExitIncomplete || exit.Error() != "plan blocked: 1 blocking finding(s)" {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(out, "plan: blocked\n") || !strings.Contains(out, "  blocking  source-removed-from-manifest  profile  -  in the receipt") {
		t.Fatalf("output:\n%s", out)
	}
}

func TestRenderIntervention(t *testing.T) {
	plan := firstPlan()
	plan.VaultState, plan.Sources, plan.Findings = sync.VaultInitialized, nil, nil
	plan.Pending = []sync.Pending{{Operation: "20261105T101500-1a2b3c4d", Last: backup.StepPublishIntent}}
	plan.Interventions = []sync.Intervention{{Cause: sync.CausePendingOperation, Subject: "20261105T101500-1a2b3c4d",
		Detail: "last committed step publish-intent; recovery needed"}}
	out, err := runSync(t, &fakePlanner{plan: plan}, "--manifest", "m.yaml", "--vault", `D:\Vault`)
	var exit *cli.ExitError
	if !errors.As(err, &exit) || exit.Code != cli.ExitIntervention ||
		exit.Error() != "intervention required: pending-operation 20261105T101500-1a2b3c4d" {
		t.Fatalf("error = %v", err)
	}
	for _, want := range []string{
		"pending: 20261105T101500-1a2b3c4d (publish-intent)\n",
		"plan: intervention required\n",
		"\ninterventions: 1\n  pending-operation  20261105T101500-1a2b3c4d  last committed step publish-intent; recovery needed\n",
		"\nfindings: none\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\nsource ") {
		t.Fatalf("no source may be planned on a vault that needs an intervention:\n%s", out)
	}
}

func TestSyncInvocationErrors(t *testing.T) {
	failure := errors.New(`vault D:\Vault: not empty and not a Savewarden vault`)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no flags", nil, `required flag(s) "manifest", "vault" not set`},
		{"no vault", []string{"--manifest", "m.yaml"}, `required flag(s) "vault" not set`},
		{"apply does not exist yet", []string{"--manifest", "m.yaml", "--vault", `D:\Vault`, "--apply"}, "unknown flag: --apply"},
		{"recover does not exist yet", []string{"--manifest", "m.yaml", "--vault", `D:\Vault`, "--recover"}, "unknown flag: --recover"},
		{"positional argument", []string{"--manifest", "m.yaml", "--vault", `D:\Vault`, "extra"}, "unknown command"},
		{"service error", []string{"--manifest", "m.yaml", "--vault", `D:\Vault`}, failure.Error()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := runSync(t, &fakePlanner{err: failure}, tt.args...)
			var exit *cli.ExitError
			if err == nil || !strings.Contains(err.Error(), tt.want) || out != "" || errors.As(err, &exit) {
				t.Fatalf("error = %v, stdout = %q", err, out)
			}
		})
	}
}

func TestSyncFailedWriteAndEscaping(t *testing.T) {
	plan := firstPlan()
	plan.Sources[0].Changes.Files[0].Entry.Path = "evil\nfindings: none"
	full := cli.RenderPlan(plan)
	if !strings.Contains(full, `"evil\nfindings: none"`) || strings.Count(full, "\nfindings:") != 1 {
		t.Fatalf("a file name forged a line:\n%s", full)
	}
	stdout := &partialWriter{limit: 40}
	root := cli.NewRootCommand("dev", stdout, &bytes.Buffer{}, nil, &fakePlanner{plan: plan})
	root.SetArgs([]string{"sync", "--manifest", "m.yaml", "--vault", `D:\Vault`})
	err := root.ExecuteContext(context.Background())
	var exit *cli.ExitError
	if err == nil || err.Error() != "disk full" || errors.As(err, &exit) {
		t.Fatalf("error = %v", err)
	}
	if got := stdout.written.String(); got == "" || !strings.HasPrefix(full, got) || got == full {
		t.Fatalf("stdout = %q", got)
	}
}

// The plan shown in docs/commands/sync.md is the exact rendering.
func TestDocumentedPlanExample(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "docs", "commands", "sync.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "```text\n"+cli.RenderPlan(firstPlan())+"```") {
		t.Fatalf("docs/commands/sync.md does not show the rendering of the first plan:\n%s", cli.RenderPlan(firstPlan()))
	}
}
