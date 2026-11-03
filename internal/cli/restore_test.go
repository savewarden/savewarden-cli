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
	plan    sync.RestorePlan
	err     error
	request sync.RestoreRequest
	called  int
}

func (f *fakeRestorer) RestorePlan(_ context.Context, request sync.RestoreRequest) (sync.RestorePlan, error) {
	f.request = request
	f.called++
	return f.plan, f.err
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

// Every flag is required; this build has neither --apply nor --recover.
func TestRestoreFlags(t *testing.T) {
	for _, args := range [][]string{nil, restoreArgs[:6], append(append([]string{}, restoreArgs...), "--apply"),
		append(append([]string{}, restoreArgs...), "--recover"), append(append([]string{}, restoreArgs...), "extra")} {
		restorer := &fakeRestorer{plan: newRestorePlan()}
		if _, _, err := runRestore(t, restorer, args...); err == nil || restorer.called != 0 {
			t.Errorf("%v: err %v, called %d", args, err, restorer.called)
		}
	}
}
