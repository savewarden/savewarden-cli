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

type fakeSnapshotter struct {
	plan     sync.SnapshotPlan
	result   sync.SnapshotResult
	recovery sync.SnapshotRecovery
	err      error
	request  sync.SnapshotRequest
	called   string
}

func (f *fakeSnapshotter) SnapshotRecoverPlan(_ context.Context, request sync.SnapshotRequest) (sync.SnapshotRecovery, error) {
	f.request, f.called = request, "recover plan"
	return f.recovery, f.err
}

func (f *fakeSnapshotter) SnapshotRecover(_ context.Context, request sync.SnapshotRequest) (sync.SnapshotResult, error) {
	f.request, f.called = request, "recover"
	return f.result, f.err
}

func (f *fakeSnapshotter) SnapshotApply(_ context.Context, request sync.SnapshotRequest) (sync.SnapshotResult, error) {
	f.request, f.called = request, "apply"
	return f.result, f.err
}

func (f *fakeSnapshotter) SnapshotPlan(_ context.Context, request sync.SnapshotRequest) (sync.SnapshotPlan, error) {
	f.request, f.called = request, "plan"
	return f.plan, f.err
}

func runSnapshot(t *testing.T, snapshotter cli.Snapshotter, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	root := cli.NewRootCommand("dev", &stdout, &stderr, nil, nil, snapshotter)
	root.SetArgs(append([]string{"snapshot"}, args...))
	err := root.ExecuteContext(context.Background())
	return stdout.String(), stderr.String(), err
}

func newSnapshotPlan() sync.SnapshotPlan {
	return sync.SnapshotPlan{
		ManifestPath: `D:\Manifests\synthetic-game.yaml`,
		Game:         game.Game{ID: "synthetic-game", Name: "Synthetic Game"},
		Vault:        `F:\Vault`,
		VaultState:   sync.VaultInitialized,
		Volume:       backup.Volume{FileSystem: "NTFS", Drive: backup.DriveFixed, Available: 1 << 40},
		Current:      "20261105T101500-1a2b3c4d",
		Recognized:   true,
		Snapshots: []sync.PublishedSnapshot{
			{ID: "20261101T090000-0a1b2c3d", Source: "20261030T120000-11223344", Files: 2, Bytes: 6},
			{ID: "20261102T090000-4e5f6a7b", Source: "20261101T120000-55667788", Files: 2, Bytes: 6, Condition: backup.SnapshotMissing},
		},
		Files: 2, Bytes: 6, Needed: 6 + 64<<20,
		Findings: []backup.Finding{{Severity: backup.Info, Code: sync.CodeSnapshotMissing, Path: "snapshots/20261102T090000-4e5f6a7b",
			Detail: "the approved snapshot is absent; it is never recreated"}},
	}
}

var snapshotArgs = []string{"--manifest", `D:\Manifests\synthetic-game.yaml`, "--vault", `F:\Vault`}

func TestSnapshotPlanRendering(t *testing.T) {
	snapshotter := &fakeSnapshotter{plan: newSnapshotPlan()}
	stdout, stderr, err := runSnapshot(t, snapshotter, snapshotArgs...)
	if err != nil {
		t.Fatalf("exit: %v", err)
	}
	want := `manifest: D:\Manifests\synthetic-game.yaml
game: synthetic-game (Synthetic Game)
vault: F:\Vault (initialized)
volume: NTFS, fixed drive, 67108870 bytes needed
current: 20261105T101500-1a2b3c4d, recognized
pending: none
mode: plan only, nothing written
plan: new snapshot

snapshots: 2 published, 1 degraded, 0 incomplete
  20261101T090000-0a1b2c3d  from 20261030T120000-11223344  2 files  6 bytes
  20261102T090000-4e5f6a7b  from 20261101T120000-55667788  2 files  6 bytes  missing
snapshot: new, from 20261105T101500-1a2b3c4d
files: 2  bytes: 6

findings: 0 blocking, 1 info
  info  snapshot-missing  -  snapshots/20261102T090000-4e5f6a7b  the approved snapshot is absent; it is never recreated
`
	if stdout != want {
		t.Fatalf("stdout:\n%s\nwant\n%s", stdout, want)
	}
	if stderr != "savewarden: warning: 1 degraded snapshot(s)\n" {
		t.Fatalf("stderr %q", stderr)
	}
	if snapshotter.request != (sync.SnapshotRequest{ManifestPath: `D:\Manifests\synthetic-game.yaml`, VaultPath: `F:\Vault`}) {
		t.Fatalf("request %+v", snapshotter.request)
	}
}

func TestSnapshotUpToDateAndIncomplete(t *testing.T) {
	plan := newSnapshotPlan()
	plan.Snapshots, plan.Findings, plan.Files, plan.Bytes, plan.Needed = plan.Snapshots[:1], nil, 0, 0, 0
	plan.UpToDate, plan.Incomplete = plan.Snapshots[0].ID, 2
	stdout, stderr, err := runSnapshot(t, &fakeSnapshotter{plan: plan}, snapshotArgs...)
	if err != nil || stderr != "" {
		t.Fatalf("exit %v, stderr %q", err, stderr)
	}
	for _, line := range []string{"plan: up to date\n", "snapshots: 1 published, 0 degraded, 2 incomplete, size not measured\n",
		"snapshot: up to date (20261101T090000-0a1b2c3d)\n", "findings: none\n"} {
		if !strings.Contains(stdout, line) {
			t.Fatalf("missing %q in\n%s", line, stdout)
		}
	}
	if strings.Contains(stdout, "files: ") {
		t.Fatalf("an up-to-date plan copies nothing:\n%s", stdout)
	}
}

func TestSnapshotExitCodes(t *testing.T) {
	blocked := newSnapshotPlan()
	blocked.Recognized, blocked.Snapshots = false, nil
	blocked.Findings = []backup.Finding{{Severity: backup.Blocking, Code: sync.CodeNoCurrent, Path: "current", Detail: "no generation"}}
	intervention := newSnapshotPlan()
	intervention.Recognized, intervention.Current, intervention.CurrentUnchecked = false, "", true
	intervention.Interventions = []sync.Intervention{{Cause: sync.CausePendingOperation, Subject: "20261106T080000-5e6f7a8b", Detail: "snapshot operation"}}
	tests := []struct {
		name    string
		plan    sync.SnapshotPlan
		err     error
		code    int
		message string
		stdout  string
	}{
		{"blocked", blocked, nil, cli.ExitIncomplete, "plan blocked: 1 blocking finding(s)", "plan: blocked\n"},
		{"intervention", intervention, nil, cli.ExitIntervention, "intervention required: pending-operation 20261106T080000-5e6f7a8b",
			"current: not checked, intervention required\n"},
		{"failure", sync.SnapshotPlan{}, errors.New("vault F:\\Vault: does not exist"), 0, "does not exist", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, _, err := runSnapshot(t, &fakeSnapshotter{plan: tt.plan, err: tt.err}, snapshotArgs...)
			var exit *cli.ExitError
			switch {
			case tt.code == 0:
				if err == nil || errors.As(err, &exit) || !strings.Contains(err.Error(), tt.message) || stdout != "" {
					t.Fatalf("err %v, stdout %q", err, stdout)
				}
			case !errors.As(err, &exit) || exit.Code != tt.code || exit.Message != tt.message || !strings.Contains(stdout, tt.stdout):
				t.Fatalf("err %v, stdout:\n%s", err, stdout)
			}
		})
	}
}

func TestSnapshotApplyReport(t *testing.T) {
	result := sync.SnapshotResult{Plan: newSnapshotPlan(), Operation: "20261106T101500-5e6f7a8b", Effect: sync.EffectPublished,
		Snapshot: `F:\Vault\games\synthetic-game\snapshots\20261106T101500-5e6f7a8b`,
		Receipt:  `F:\Vault\games\synthetic-game\snapshots\receipts\20261106T101500-5e6f7a8b.json`,
		Files:    2, Bytes: 6}
	snapshotter := &fakeSnapshotter{result: result}
	stdout, stderr, err := runSnapshot(t, snapshotter, append(append([]string{}, snapshotArgs...), "--apply")...)
	if err != nil || snapshotter.called != "apply" {
		t.Fatalf("exit %v, called %q", err, snapshotter.called)
	}
	for _, line := range []string{"mode: apply: copy, verify and publish\n", "operation: 20261106T101500-5e6f7a8b\n",
		"verification: 2 files, 6 bytes, SHA-256 verified\n", "published: " + result.Snapshot + "\n", "receipt: " + result.Receipt + "\n"} {
		if !strings.Contains(stdout, line) {
			t.Fatalf("missing %q in\n%s", line, stdout)
		}
	}
	if stderr != "savewarden: warning: 1 degraded snapshot(s)\nsavewarden: vault: published snapshot 20261106T101500-5e6f7a8b\n" {
		t.Fatalf("stderr %q", stderr)
	}
	for _, tt := range []struct {
		result sync.SnapshotResult
		err    error
		code   int
		state  string
	}{
		{sync.SnapshotResult{Plan: newSnapshotPlan(), Operation: "20261106T101500-5e6f7a8b", Effect: sync.EffectStaged},
			&sync.BlockedError{Code: sync.CodeCurrentModified, Detail: "changed"}, cli.ExitIncomplete, "snapshot 20261106T101500-5e6f7a8b kept incomplete"},
		{sync.SnapshotResult{Plan: newSnapshotPlan(), Operation: "20261106T101500-5e6f7a8b", Effect: sync.EffectRolledBack},
			&sync.InterventionError{Operation: "20261106T101500-5e6f7a8b", Cause: "source-unverified: gone"}, cli.ExitIntervention,
			"rolled back snapshot 20261106T101500-5e6f7a8b"},
		{sync.SnapshotResult{Plan: newSnapshotPlan()}, nil, 0, "not modified"},
	} {
		_, stderr, err := runSnapshot(t, &fakeSnapshotter{result: tt.result, err: tt.err}, append(append([]string{}, snapshotArgs...), "--apply")...)
		var exit *cli.ExitError
		if (tt.code == 0) != (err == nil) || (tt.code != 0 && (!errors.As(err, &exit) || exit.Code != tt.code)) ||
			!strings.HasSuffix(stderr, "savewarden: vault: "+tt.state+"\n") {
			t.Errorf("%s: exit %v, stderr %q", tt.state, err, stderr)
		}
	}
}

func TestSnapshotFlags(t *testing.T) {
	for _, args := range [][]string{nil, {"--manifest", "m.yaml"}, {"--vault", `F:\Vault`, "--apply"}} {
		snapshotter := &fakeSnapshotter{plan: newSnapshotPlan()}
		if _, _, err := runSnapshot(t, snapshotter, args...); err == nil || snapshotter.request != (sync.SnapshotRequest{}) {
			t.Errorf("%v: err %v, request %+v", args, err, snapshotter.request)
		}
	}
}
