package cli_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/cli"
	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/entities/game"
	"github.com/savewarden/savewarden-cli/internal/features/sync"
)

func snapshotRecovery() sync.SnapshotRecovery {
	return sync.SnapshotRecovery{
		ManifestPath: `D:\Manifests\synthetic-game.yaml`,
		Game:         game.Game{ID: "synthetic-game", Name: "Synthetic Game"},
		Vault:        `F:\Vault`,
		Volume:       backup.Volume{FileSystem: "NTFS", Drive: backup.DriveFixed},
		Operation:    "20261106T101500-5e6f7a8b",
		Last:         backup.StepPublishIntent,
		Observed:     &backup.SnapshotObservation{Published: backup.New, Incomplete: backup.Absent},
		Action:       sync.RecoverSnapshotPublish,
	}
}

func TestSnapshotRecoverPlanReport(t *testing.T) {
	snapshotter := &fakeSnapshotter{recovery: snapshotRecovery()}
	stdout, stderr, err := runSnapshot(t, snapshotter, append(append([]string{}, snapshotArgs...), "--recover")...)
	if err != nil || snapshotter.called != "recover plan" {
		t.Fatalf("exit %v, called %q", err, snapshotter.called)
	}
	want := `manifest: D:\Manifests\synthetic-game.yaml
game: synthetic-game (Synthetic Game)
vault: F:\Vault
volume: NTFS, fixed drive
snapshot operation: 20261106T101500-5e6f7a8b
last committed step: publish-intent
observed: snapshot new, incomplete absent
action: ` + string(sync.RecoverSnapshotPublish) + `
mode: recovery plan only, nothing written
`
	if stdout != want {
		t.Fatalf("stdout:\n%s\nwant\n%s", stdout, want)
	}
	if stderr != "savewarden: vault: not modified\n" {
		t.Fatalf("stderr %q", stderr)
	}
}

func TestSnapshotRecoverReport(t *testing.T) {
	recovery := snapshotRecovery()
	recovery.Observed.Receipt, recovery.Action = true, sync.RecoverSnapshotConfirm
	result := sync.SnapshotResult{Operation: recovery.Operation, Effect: sync.EffectPublished, Recovery: &recovery}
	snapshotter := &fakeSnapshotter{result: result}
	stdout, stderr, err := runSnapshot(t, snapshotter, append(append([]string{}, snapshotArgs...), "--recover", "--apply")...)
	if err != nil || snapshotter.called != "recover" || !strings.Contains(stdout, "observed: snapshot new, incomplete absent, receipt committed\n") ||
		!strings.Contains(stdout, "mode: recovery\n") || stderr != "savewarden: vault: published snapshot 20261106T101500-5e6f7a8b\n" {
		t.Fatalf("exit %v, stdout:\n%s\nstderr %q", err, stdout, stderr)
	}
	intervention := &sync.InterventionError{Operation: recovery.Operation, Cause: "operation 20261106T101500-5e6f7a8b: ambiguous"}
	_, stderr, err = runSnapshot(t, &fakeSnapshotter{recovery: recovery, err: intervention}, append(append([]string{}, snapshotArgs...), "--recover")...)
	var exit *cli.ExitError
	if !errors.As(err, &exit) || exit.Code != cli.ExitIntervention ||
		!strings.HasSuffix(stderr, "savewarden: vault: intervention required for 20261106T101500-5e6f7a8b: operation 20261106T101500-5e6f7a8b: ambiguous\n") {
		t.Fatalf("intervention: %v, stderr %q", err, stderr)
	}
}
