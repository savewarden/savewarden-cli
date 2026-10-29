package cli_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
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
	plan     sync.Plan
	result   sync.Result
	recovery sync.Recovery
	err      error
	request  sync.Request
	called   string
}

func (f *fakePlanner) Plan(_ context.Context, request sync.Request) (sync.Plan, error) {
	f.request, f.called = request, "plan"
	return f.plan, f.err
}

func (f *fakePlanner) Apply(_ context.Context, request sync.Request) (sync.Result, error) {
	f.request, f.called = request, "apply"
	return f.result, f.err
}

func (f *fakePlanner) RecoverPlan(_ context.Context, request sync.Request) (sync.Recovery, error) {
	f.request, f.called = request, "recover plan"
	return f.recovery, f.err
}

func (f *fakePlanner) Recover(_ context.Context, request sync.Request) (sync.Result, error) {
	f.request, f.called = request, "recover"
	return f.result, f.err
}

func runSync(t *testing.T, planner cli.Syncer, args ...string) (string, error) {
	t.Helper()
	stdout, _, err := runSyncAll(t, planner, &bytes.Buffer{}, args...)
	return stdout, err
}

func runSyncAll(t *testing.T, planner cli.Syncer, stdout io.Writer, args ...string) (string, string, error) {
	t.Helper()
	var stderr bytes.Buffer
	root := cli.NewRootCommand("dev", stdout, &stderr, nil, planner)
	root.SetArgs(append([]string{"sync"}, args...))
	err := root.ExecuteContext(context.Background())
	out := ""
	if b, ok := stdout.(*bytes.Buffer); ok {
		out = b.String()
	}
	return out, stderr.String(), err
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
		Volume:       backup.Volume{FileSystem: "NTFS", Drive: backup.DriveFixed, Available: 1 << 40},
		Needed:       6 + 64<<20,
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
volume: NTFS, fixed drive, 67108870 bytes needed
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
		{"removals are not available", []string{"--manifest", "m.yaml", "--vault", `D:\Vault`, "--allow-removals"}, "unknown flag: --allow-removals"},
		{"missing sources cannot be accepted", []string{"--manifest", "m.yaml", "--vault", `D:\Vault`, "--accept-missing-source", "saves"}, "unknown flag: --accept-missing-source"},
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

func TestRenderHeaderOnAnIntervention(t *testing.T) {
	plan := firstPlan()
	plan.VaultState, plan.Sources, plan.Findings, plan.CurrentUnchecked = sync.VaultInitialized, nil, nil, true
	plan.Interventions = []sync.Intervention{{Cause: sync.CauseJournalAmbiguous, Subject: "20261105T101500-1a2b3c4d", Detail: "chain broken"}}
	out := cli.RenderPlan(plan)
	for _, want := range []string{"current: not checked, intervention required\n", "pending: unknown, ambiguous journal\n"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s", want, out)
		}
	}
}

const applyOp = "20261105T101500-1a2b3c4d"

func published() sync.Result {
	return sync.Result{Plan: firstPlan(), Operation: applyOp, Effect: sync.EffectPublished, Files: 2, Bytes: 6,
		Receipt: `D:\Vault\games\synthetic-game\receipts\` + applyOp + `.json`}
}

func TestApplyReportsAndAnnouncesTheVault(t *testing.T) {
	planner := &fakePlanner{result: published()}
	stdout, stderr, err := runSyncAll(t, planner, &bytes.Buffer{}, "--manifest", "m.yaml", "--vault", `D:\Vault`, "--apply")
	if err != nil || planner.called != "apply" {
		t.Fatalf("err %v, called %q", err, planner.called)
	}
	for _, want := range []string{
		"mode: apply: copy, verify and publish\n",
		"\noperation: " + applyOp + "\nverification: 2 files, 6 bytes, SHA-256 verified\npublished: D:\\Vault\\games\\synthetic-game\\current\nreceipt: D:\\Vault\\games\\synthetic-game\\receipts\\" + applyOp + ".json\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout lacks %q:\n%s", want, stdout)
		}
	}
	if stderr != "savewarden: vault: published "+applyOp+"\n" {
		t.Fatalf("stderr %q", stderr)
	}
}

func TestApplyExitCodes(t *testing.T) {
	tests := []struct {
		name   string
		result sync.Result
		err    error
		code   int
		stderr string
	}{
		{"blocked", sync.Result{Plan: firstPlan()}, &sync.BlockedError{Code: sync.CodeCurrentExists, Detail: "already there"}, 2,
			"savewarden: current-exists: already there\nsavewarden: vault: not modified\n"},
		{"failed", sync.Result{Plan: firstPlan(), Operation: applyOp, Effect: sync.EffectStaged}, errors.New("disk full"), 1,
			"savewarden: disk full\nsavewarden: vault: current unchanged; operation " + applyOp + " kept in staging\n"},
		{"intervention", sync.Result{Plan: firstPlan(), Operation: applyOp, Effect: sync.EffectIntervention, Cause: "receipt missing"},
			&sync.InterventionError{Operation: applyOp, Cause: "receipt missing"}, 3,
			"savewarden: intervention required for " + applyOp + ": receipt missing\nsavewarden: vault: intervention required for " + applyOp + ": receipt missing\n"},
		{"no plan", sync.Result{}, errors.New("manifest invalid"), 1, "savewarden: manifest invalid\nsavewarden: vault: not modified\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, err := runSyncAll(t, &fakePlanner{result: tt.result, err: tt.err}, &bytes.Buffer{},
				"--manifest", "m.yaml", "--vault", `D:\Vault`, "--apply")
			var exit *cli.ExitError
			if !errors.As(err, &exit) || exit.Code != tt.code || !exit.Reported || stderr != tt.stderr {
				t.Fatalf("err %v, stderr %q", err, stderr)
			}
			if (tt.result.Plan.ManifestPath == "") != (stdout == "") {
				t.Fatalf("stdout %q", stdout)
			}
		})
	}
}

// ADR 0007: a publication that succeeded but could not be reported exits 1,
// and the state line still says it was published.
func TestPublishedButStdoutFailed(t *testing.T) {
	_, stderr, err := runSyncAll(t, &fakePlanner{result: published()}, failingWriter{}, "--manifest", "m.yaml", "--vault", `D:\Vault`, "--apply")
	var exit *cli.ExitError
	if !errors.As(err, &exit) || exit.Code != 1 || !strings.HasSuffix(stderr, "savewarden: vault: published "+applyOp+"\n") ||
		!strings.Contains(stderr, "stdout is closed") {
		t.Fatalf("err %v, stderr %q", err, stderr)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("stdout is closed") }

func TestRecoverModes(t *testing.T) {
	recovery := sync.Recovery{
		ManifestPath: "m.yaml", Game: game.Game{ID: "synthetic-game", Name: "Synthetic Game"}, Vault: `D:\Vault`,
		VaultState: sync.VaultInitialized, Volume: backup.Volume{FileSystem: "NTFS", Drive: backup.DriveFixed},
		Operation: applyOp, Last: backup.StepPublishIntent,
		Observed: &backup.Observation{Current: backup.New, Staging: backup.Absent}, Action: sync.RecoverVerifyPublish,
	}
	want := "manifest: m.yaml\ngame: synthetic-game (Synthetic Game)\nvault: D:\\Vault (initialized)\nvolume: NTFS, fixed drive\n" +
		"operation: " + applyOp + "\nlast committed step: publish-intent\nobserved: current new, staging absent\n" +
		"action: " + string(sync.RecoverVerifyPublish) + "\n"
	planner := &fakePlanner{recovery: recovery}
	stdout, stderr, err := runSyncAll(t, planner, &bytes.Buffer{}, "--manifest", "m.yaml", "--vault", `D:\Vault`, "--recover")
	if err != nil || planner.called != "recover plan" || stdout != want+"mode: recovery plan only, nothing written\n" ||
		stderr != "savewarden: vault: not modified\n" {
		t.Fatalf("recover plan: %v %q\n%s\n%q", err, planner.called, stdout, stderr)
	}
	planner = &fakePlanner{result: sync.Result{Operation: applyOp, Effect: sync.EffectPublished, Recovery: &recovery}}
	stdout, stderr, err = runSyncAll(t, planner, &bytes.Buffer{}, "--manifest", "m.yaml", "--vault", `D:\Vault`, "--recover", "--apply")
	if err != nil || planner.called != "recover" || stdout != want+"mode: recovery\n" || stderr != "savewarden: vault: published "+applyOp+"\n" {
		t.Fatalf("recover: %v %q\n%s\n%q", err, planner.called, stdout, stderr)
	}
	planner = &fakePlanner{recovery: recovery, err: &sync.InterventionError{Operation: applyOp, Cause: "observation does not match"}}
	_, stderr, err = runSyncAll(t, planner, &bytes.Buffer{}, "--manifest", "m.yaml", "--vault", `D:\Vault`, "--recover")
	var exit *cli.ExitError
	if !errors.As(err, &exit) || exit.Code != 3 || !strings.HasSuffix(stderr, "savewarden: vault: intervention required for "+applyOp+": observation does not match\n") {
		t.Fatalf("ambiguous: %v %q", err, stderr)
	}
}

func TestRemovalFlagsAreNotAvailable(t *testing.T) {
	for _, flag := range []string{"--allow-removals", "--accept-missing-source=saves"} {
		if _, err := runSync(t, &fakePlanner{}, "--manifest", "m.yaml", "--vault", `D:\Vault`, "--apply", flag); err == nil {
			t.Errorf("%s accepted", flag)
		}
	}
}
