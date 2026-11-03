package cli_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/cli"
	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/entities/game"
	"github.com/savewarden/savewarden-cli/internal/features/sync"
)

type fakeRestorer struct {
	plan     sync.RestorePlan
	result   sync.RestoreResult
	recovery sync.RestoreRecovery
	err      error
	request  sync.RestoreRequest
	called   int
	applied  bool
	mode     string
}

func (f *fakeRestorer) RestorePlan(_ context.Context, request sync.RestoreRequest) (sync.RestorePlan, error) {
	f.request = request
	f.called++
	return f.plan, f.err
}

func (f *fakeRestorer) RestoreApply(_ context.Context, request sync.RestoreRequest) (sync.RestoreResult, error) {
	f.request, f.applied = request, true
	f.called++
	return f.result, f.err
}

func (f *fakeRestorer) RestoreRecoverPlan(_ context.Context, request sync.RestoreRequest) (sync.RestoreRecovery, error) {
	f.request, f.mode = request, "recover plan"
	f.called++
	return f.recovery, f.err
}

func (f *fakeRestorer) RestoreRecover(_ context.Context, request sync.RestoreRequest) (sync.RestoreResult, error) {
	f.request, f.mode = request, "recover"
	f.called++
	return f.result, f.err
}

func runRestore(t *testing.T, restorer cli.Restorer, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	root := cli.NewRootCommand("dev", &stdout, &stderr, nil, nil, nil, restorer)
	root.SetArgs(append([]string{"restore"}, args...))
	err := root.ExecuteContext(context.Background())
	return stdout.String(), stderr.String(), err
}

func newRestorePlan() sync.RestorePlan {
	return sync.RestorePlan{
		ManifestPath: `D:\Manifests\synthetic-game.yaml`,
		Game:         game.Game{ID: "synthetic-game", Name: "Synthetic Game"},
		Vault:        `F:\Vault`,
		Snapshot:     "20261102T090000-4e5f6a7b",
		From:         "20261101T120000-55667788",
		Verified:     true,
		Destination:  `G:\Restores\synthetic`,
		State:        sync.DestinationArea,
		Volume:       backup.Volume{FileSystem: "NTFS", Drive: backup.DriveRemovable, Available: 1 << 40},
		Restores: []sync.HandedOverRestore{
			{ID: "20261103T100000-9a8b7c6d", Snapshot: "20261102T090000-4e5f6a7b"},
			{ID: "20261103T110000-0a1b2c3d", Snapshot: "20261101T090000-0a1b2c3d", Condition: backup.HandoverRenameLost},
		},
		Incomplete: 1,
		Files:      2, Bytes: 6, Needed: 6 + 64<<20,
		Findings: []backup.Finding{
			{Severity: backup.Info, Code: sync.CodeSnapshotAlreadyRestored, Path: "20261103T100000-9a8b7c6d",
				Detail: "this snapshot was already restored in this area; a new restore is still possible"},
			{Severity: backup.Info, Code: sync.CodeRestoreRenameLost, Path: "20261103T110000-0a1b2c3d", Detail: "still under .incomplete"},
		},
	}
}

var restoreArgs = []string{"--manifest", `D:\Manifests\synthetic-game.yaml`, "--vault", `F:\Vault`,
	"--snapshot", "20261102T090000-4e5f6a7b", "--to", `G:\Restores\synthetic`}

func TestRestorePlanRendering(t *testing.T) {
	restorer := &fakeRestorer{plan: newRestorePlan()}
	stdout, stderr, err := runRestore(t, restorer, restoreArgs...)
	if err != nil || stderr != "" {
		t.Fatalf("exit: %v, stderr %q", err, stderr)
	}
	want := `manifest: D:\Manifests\synthetic-game.yaml
game: synthetic-game (Synthetic Game)
vault: F:\Vault
snapshot: 20261102T090000-4e5f6a7b, from 20261101T120000-55667788, verified
destination: G:\Restores\synthetic (restore area), 2 published, 1 incomplete
  20261103T100000-9a8b7c6d  of 20261102T090000-4e5f6a7b  present
  20261103T110000-0a1b2c3d  of 20261101T090000-0a1b2c3d  rename lost
volume: NTFS, removable drive
mode: plan only, nothing written
plan: new restore
restore: new, to G:\Restores\synthetic\<restore-id>
files: 2  bytes: 6  space needed: 67108870 bytes

findings: 0 blocking, 2 info
  info  snapshot-already-restored  -  20261103T100000-9a8b7c6d  this snapshot was already restored in this area; a new restore is still possible
  info  restore-rename-lost  -  20261103T110000-0a1b2c3d  still under .incomplete
`
	if stdout != want {
		t.Fatalf("stdout:\n%s\nwant\n%s", stdout, want)
	}
	if restorer.request != (sync.RestoreRequest{ManifestPath: `D:\Manifests\synthetic-game.yaml`, VaultPath: `F:\Vault`,
		Snapshot: "20261102T090000-4e5f6a7b", Destination: `G:\Restores\synthetic`}) {
		t.Fatalf("request %+v", restorer.request)
	}
}

func TestRestorePlanOfANewArea(t *testing.T) {
	plan := newRestorePlan()
	plan.State, plan.Restores, plan.Incomplete, plan.Findings = sync.DestinationNew, nil, 0, nil
	stdout, _, err := runRestore(t, &fakeRestorer{plan: plan}, restoreArgs...)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"destination: G:\\Restores\\synthetic (new restore area)\nvolume: ", "findings: none\n"} {
		if !strings.Contains(stdout, line) {
			t.Fatalf("missing %q in\n%s", line, stdout)
		}
	}
	// The words of ADR 0009, not the stable codes of the findings.
	want := []string{"present", "absent", "rename lost", "replaced", "not observable"}
	for i, condition := range []backup.HandoverCondition{backup.HandoverPresent, backup.HandoverMissing,
		backup.HandoverRenameLost, backup.HandoverReplaced, backup.HandoverNotObservable} {
		if text := (sync.HandedOverRestore{Condition: condition}).State(); text != want[i] {
			t.Fatalf("%s: handed-over state %q", condition, text)
		}
	}
}

func TestRestoreExitCodes(t *testing.T) {
	degraded := newRestorePlan()
	degraded.Verified, degraded.Condition, degraded.Files, degraded.Bytes, degraded.Needed = false, backup.SnapshotModified, 0, 0, 0
	degraded.Findings = []backup.Finding{{Severity: backup.Blocking, Code: sync.CodeSnapshotModified, Path: "snapshots/20261102T090000-4e5f6a7b", Detail: "differs"}}
	intervention := newRestorePlan()
	intervention.Findings = nil
	intervention.Interventions = []sync.Intervention{{Cause: sync.CausePendingOperation, Subject: "20261103T120000-5e6f7a8b", Detail: "restore operation"}}
	tests := []struct {
		name    string
		plan    sync.RestorePlan
		err     error
		code    int
		message string
		stdout  string
	}{
		{"degraded", degraded, nil, cli.ExitIncomplete, "plan blocked: 1 blocking finding(s)",
			"snapshot: 20261102T090000-4e5f6a7b, from 20261101T120000-55667788, modified, not restorable\n"},
		{"intervention", intervention, nil, cli.ExitIntervention, "intervention required: pending-operation 20261103T120000-5e6f7a8b",
			"plan: intervention required\n\ninterventions: 1\n  pending-operation  20261103T120000-5e6f7a8b  restore operation\n"},
		{"failure", sync.RestorePlan{}, errors.New("snapshot 20261102T090000-4e5f6a7b not found in the vault F:\\Vault"), 0, "not found", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, _, err := runRestore(t, &fakeRestorer{plan: tt.plan, err: tt.err}, restoreArgs...)
			var exit *cli.ExitError
			switch {
			case tt.code == 0:
				if err == nil || errors.As(err, &exit) || !strings.Contains(err.Error(), tt.message) || stdout != "" {
					t.Fatalf("err %v, stdout %q", err, stdout)
				}
			case !errors.As(err, &exit) || exit.Code != tt.code || exit.Message != tt.message || !strings.Contains(stdout, tt.stdout):
				t.Fatalf("err %v, stdout:\n%s", err, stdout)
			case strings.Contains(stdout, "restore: new"):
				t.Fatalf("a blocked plan announces no restore:\n%s", stdout)
			}
		})
	}
}

func TestRestoreApplyReport(t *testing.T) {
	op := "20261107T101500-9a8b7c6d"
	result := sync.RestoreResult{Plan: newRestorePlan(), Operation: op, Effect: sync.EffectPublished,
		Restore: `G:\Restores\synthetic\` + op, Receipt: `G:\Restores\synthetic\receipts\` + op + ".json", Files: 2, Bytes: 6}
	restorer := &fakeRestorer{result: result}
	stdout, stderr, err := runRestore(t, restorer, append(append([]string{}, restoreArgs...), "--apply")...)
	if err != nil || !restorer.applied {
		t.Fatalf("exit %v, applied %v", err, restorer.applied)
	}
	for _, line := range []string{"mode: apply: copy, verify and publish\n", "operation: " + op + "\n",
		"verification: 2 files, 6 bytes, SHA-256 verified\n", "published: " + result.Restore + "\n", "receipt: " + result.Receipt + "\n"} {
		if !strings.Contains(stdout, line) {
			t.Fatalf("missing %q in\n%s", line, stdout)
		}
	}
	if stderr != "savewarden: destination: published restore "+op+"\n" {
		t.Fatalf("stderr %q", stderr)
	}
	for _, tt := range []struct {
		result sync.RestoreResult
		err    error
		code   int
		state  string
	}{
		{sync.RestoreResult{Plan: newRestorePlan(), Operation: op, Effect: sync.EffectStaged},
			&sync.BlockedError{Code: sync.CodeChangedDuringRestore, Detail: "changed"}, cli.ExitIncomplete, "restore " + op + " kept incomplete"},
		{sync.RestoreResult{Plan: newRestorePlan(), Operation: op, Effect: sync.EffectRolledBack},
			errors.New("publication failed"), 1, "rolled back restore " + op},
		{sync.RestoreResult{Plan: newRestorePlan(), Effect: sync.EffectMetadata},
			errors.New("destination G:\\Restores\\synthetic: destination-changed"), 1, "metadata only; nothing staged or published"},
		{sync.RestoreResult{Plan: newRestorePlan(), Effect: sync.EffectIntervention, Cause: "pending-operation " + op},
			&sync.InterventionError{Subject: "the restore area", Cause: "pending-operation " + op}, cli.ExitIntervention,
			"intervention required for the restore area: pending-operation " + op},
		{sync.RestoreResult{}, errors.New("snapshot not found"), 1, "not modified"},
	} {
		stdout, stderr, err := runRestore(t, &fakeRestorer{result: tt.result, err: tt.err}, append(append([]string{}, restoreArgs...), "--apply")...)
		var exit *cli.ExitError
		if (tt.code == 0) != (err == nil) || (tt.code != 0 && (!errors.As(err, &exit) || exit.Code != tt.code)) ||
			!strings.HasSuffix(stderr, "savewarden: destination: "+tt.state+"\n") || strings.Contains(stderr, "vault:") {
			t.Errorf("%s: exit %v, stderr %q", tt.state, err, stderr)
		}
		if tt.result.Plan.ManifestPath == "" && stdout != "" {
			t.Errorf("%s: a run refused before its plan reports nothing: %q", tt.state, stdout)
		}
	}
}

var recoverArgs = []string{"--manifest", `D:\Manifests\synthetic-game.yaml`, "--to", `G:\Restores\synthetic`, "--recover"}

func TestRestoreRecoveryReport(t *testing.T) {
	op := "20261107T101500-9a8b7c6d"
	recovery := sync.RestoreRecovery{ManifestPath: `D:\Manifests\synthetic-game.yaml`, Game: game.Game{ID: "synthetic-game", Name: "Synthetic Game"},
		Destination: `G:\Restores\synthetic`, State: sync.DestinationArea, Volume: backup.Volume{FileSystem: "NTFS", Drive: backup.DriveFixed},
		Operation: op, Last: backup.StepPublishIntent, Observed: &backup.RestoreObservation{Published: backup.New, Incomplete: backup.Absent},
		Action: sync.RecoverRestorePublish}
	restorer := &fakeRestorer{recovery: recovery}
	stdout, stderr, err := runRestore(t, restorer, recoverArgs...)
	if err != nil || restorer.mode != "recover plan" {
		t.Fatalf("exit %v, mode %q", err, restorer.mode)
	}
	want := `manifest: D:\Manifests\synthetic-game.yaml
game: synthetic-game (Synthetic Game)
destination: G:\Restores\synthetic (restore area), 0 published
volume: NTFS, fixed drive
restore operation: 20261107T101500-9a8b7c6d
last committed step: publish-intent
observed: restore new, incomplete absent
action: ` + string(sync.RecoverRestorePublish) + `
warning: operation 20261107T101500-9a8b7c6d pending recovery: copy published, receipt missing; do not use G:\Restores\synthetic\20261107T101500-9a8b7c6d yet
mode: recovery plan only, nothing written
`
	if stdout != want || stderr != "savewarden: destination: not modified\n" {
		t.Fatalf("stdout:\n%s\nwant\n%s\nstderr %q", stdout, want, stderr)
	}
	if restorer.request != (sync.RestoreRequest{ManifestPath: `D:\Manifests\synthetic-game.yaml`, Destination: `G:\Restores\synthetic`}) {
		t.Fatalf("request %+v", restorer.request)
	}
	if strings.Contains(stdout, "vault") {
		t.Fatalf("a recovery never names the vault:\n%s", stdout)
	}
	// Performed, and an intervention.
	result := sync.RestoreResult{Operation: op, Effect: sync.EffectPublished, Recovery: &recovery}
	restorer = &fakeRestorer{result: result}
	stdout, stderr, err = runRestore(t, restorer, append(append([]string{}, recoverArgs...), "--apply")...)
	if err != nil || restorer.mode != "recover" || !strings.Contains(stdout, "mode: recovery\n") ||
		stderr != "savewarden: destination: published restore "+op+"\n" {
		t.Fatalf("recover --apply: %v, %q\n%s", err, stderr, stdout)
	}
	_, stderr, err = runRestore(t, &fakeRestorer{err: &sync.InterventionError{Subject: "the restore area", Cause: "unexpected-entry notes.txt"}}, recoverArgs...)
	var exit *cli.ExitError
	if !errors.As(err, &exit) || exit.Code != cli.ExitIntervention ||
		!strings.HasSuffix(stderr, "savewarden: destination: intervention required for the restore area: unexpected-entry notes.txt\n") {
		t.Fatalf("intervention: %v, %q", err, stderr)
	}
}

// --manifest and --to are always required; --vault and --snapshot are
// required, except with --recover, which refuses them.
func TestRestoreFlags(t *testing.T) {
	for _, args := range [][]string{nil, restoreArgs[:6], append(append([]string{}, restoreArgs[2:]...), "--apply"),
		append(append([]string{}, restoreArgs...), "--recover"), append(append([]string{}, recoverArgs...), "--vault", `F:\Vault`),
		append(append([]string{}, recoverArgs...), "--snapshot", "20261102T090000-4e5f6a7b"), recoverArgs[2:], recoverArgs[:2],
		append(append([]string{}, restoreArgs...), "extra")} {
		restorer := &fakeRestorer{plan: newRestorePlan()}
		if _, _, err := runRestore(t, restorer, args...); err == nil || restorer.called != 0 {
			t.Errorf("%v: err %v, called %d", args, err, restorer.called)
		}
	}
}
