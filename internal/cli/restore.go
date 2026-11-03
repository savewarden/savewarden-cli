package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/savewarden/savewarden-cli/internal/features/sync"
)

// Restorer plans restores of an approved snapshot into an isolated restore
// area; the sync feature implements it (ADR 0004, B4 amendment).
type Restorer interface {
	RestorePlan(ctx context.Context, request sync.RestoreRequest) (sync.RestorePlan, error)
}

func newRestoreCommand(restorer Restorer) *cobra.Command {
	var manifest, vault, snapshot, destination string
	command := &cobra.Command{
		Use:   "restore --manifest <path> --vault <directory> --snapshot <id> --to <directory>",
		Short: "Plan the restore of an approved snapshot into an isolated directory",
		Long: `Plan the restore of an approved snapshot into an isolated directory.

A restore copies one approved snapshot of the vault into a restore area: a
new or empty directory that restore marks as its own, or an area it already
marked for the same game. It never lands in the vault, the installation or
a source, whatever the drive letter; nothing is ever replaced or deleted,
and the vault is never written, not even its lock.

The plan authenticates the snapshot by a targeted read of the vault: its
journal published, its receipt and its identity those it published, its
content read in full against its receipt. It reads nothing else of the
vault. It then reads the destination and lists the restores already handed
over there, by their names only.

This build plans only: nothing is copied, created, moved, written or
executed.

Exit codes: 0 plan without blocking findings; 1 invalid invocation,
manifest, vault or destination, or a snapshot that is not found, not
published, not verifiable or not restorable now; 2 blocked, such as a
degraded snapshot or insufficient space; 3 the restore area needs an
intervention first. Trust the exit code, never the presence of output.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			request := sync.RestoreRequest{ManifestPath: manifest, VaultPath: vault, Snapshot: snapshot, Destination: destination}
			plan, err := restorer.RestorePlan(cmd.Context(), request)
			if err != nil {
				return err
			}
			if _, err := io.WriteString(cmd.OutOrStdout(), RenderRestorePlan(plan)); err != nil {
				return err
			}
			if len(plan.Interventions) > 0 {
				first := plan.Interventions[0]
				return &ExitError{Code: ExitIntervention, Message: fmt.Sprintf("intervention required: %s %s", first.Cause, printable(first.Subject))}
			}
			if blocking := plan.Blocking(); blocking > 0 {
				return &ExitError{Code: ExitIncomplete, Message: fmt.Sprintf("plan blocked: %d blocking finding(s)", blocking)}
			}
			return nil
		},
	}
	command.Flags().StringVar(&manifest, "manifest", "", "path to the game's manifest.yaml (required)")
	command.Flags().StringVar(&vault, "vault", "", "vault directory, absolute Windows path (required)")
	command.Flags().StringVar(&snapshot, "snapshot", "", "approved snapshot to restore, such as 20261106T101500-5e6f7a8b (required)")
	command.Flags().StringVar(&destination, "to", "", "restore area, absolute Windows path: new, empty, or marked by restore (required)")
	for _, name := range []string{"manifest", "vault", "snapshot", "to"} {
		_ = command.MarkFlagRequired(name)
	}
	return command
}

// RenderRestorePlan formats a restore plan as stable, line-oriented English
// text, with no execution date (ADR 0009): the header and the snapshot's
// verdict, the destination and its handed-over restores, the decision, the
// interventions, then the findings.
func RenderRestorePlan(plan sync.RestorePlan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "manifest: %s\n", printable(plan.ManifestPath))
	fmt.Fprintf(&b, "game: %s (%s)\n", plan.Game.ID, printable(plan.Game.Name))
	fmt.Fprintf(&b, "vault: %s\n", printable(plan.Vault))
	from := plan.From
	if from == "" {
		from = "unknown"
	}
	fmt.Fprintf(&b, "snapshot: %s, from %s, %s\n", plan.Snapshot, from, plan.Verdict())
	fmt.Fprintf(&b, "destination: %s (%s)", printable(plan.Destination), plan.State)
	if plan.State == sync.DestinationArea {
		// Counted by name, never walked.
		fmt.Fprintf(&b, ", %d published, %d incomplete", len(plan.Restores), plan.Incomplete)
	}
	b.WriteString("\n")
	for _, r := range plan.Restores {
		fmt.Fprintf(&b, "  %s  of %s  %s\n", r.ID, r.Snapshot, r.State())
	}
	// The space available is not shown: two plans of an unchanged state
	// stay identical byte for byte.
	fmt.Fprintf(&b, "volume: %s, %s drive\n", printable(plan.Volume.FileSystem), plan.Volume.Drive)
	b.WriteString("mode: plan only, nothing written\n")
	fmt.Fprintf(&b, "plan: %s\n", plan.Outcome())
	if plan.Verified && len(plan.Interventions) == 0 && plan.Blocking() == 0 {
		fmt.Fprintf(&b, "restore: new, to %s\\<restore-id>\n", printable(plan.Destination))
		fmt.Fprintf(&b, "files: %d  bytes: %d  space needed: %d bytes\n", plan.Files, plan.Bytes, plan.Needed)
	}
	if len(plan.Interventions) > 0 {
		fmt.Fprintf(&b, "\ninterventions: %d\n", len(plan.Interventions))
		for _, intervention := range plan.Interventions {
			fmt.Fprintf(&b, "  %s  %s  %s\n", intervention.Cause, printable(intervention.Subject), printable(intervention.Detail))
		}
	}
	blocking := plan.Blocking()
	if len(plan.Findings) == 0 {
		b.WriteString("\nfindings: none\n")
		return b.String()
	}
	fmt.Fprintf(&b, "\nfindings: %d blocking, %d info\n", blocking, len(plan.Findings)-blocking)
	for _, finding := range plan.Findings {
		fmt.Fprintf(&b, "  %s  %s  %s  %s  %s\n", finding.Severity, finding.Code, orDash(finding.Source),
			orDash(printable(finding.Path)), printable(finding.Detail))
	}
	return b.String()
}
