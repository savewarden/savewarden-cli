package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/savewarden/savewarden-cli/internal/features/sync"
)

// Snapshotter plans snapshots of the current generation of a vault; the
// sync feature implements it (ADR 0004, B3 amendment).
type Snapshotter interface {
	SnapshotPlan(ctx context.Context, request sync.SnapshotRequest) (sync.SnapshotPlan, error)
}

func newSnapshotCommand(snapshotter Snapshotter) *cobra.Command {
	var manifest, vault string
	command := &cobra.Command{
		Use:   "snapshot --manifest <path> --vault <directory>",
		Short: "Plan an immutable snapshot of the current generation of a vault",
		Long: `Plan an immutable snapshot of the current generation of a vault.

A snapshot copies the generation the vault already holds, checked against its
receipt and its identity: the state saved in the vault, not necessarily the
state of the game now; run sync --apply first to save that. It reads neither
the sources nor the installation, whose identities only keep the vault
distinct from them. The plan lists the approved snapshots, those that are
degraded, and the copies kept incomplete, without walking them.

This build plans only: nothing is copied, created, moved, written or executed.

Exit codes: 0 plan without blocking findings, or up to date; 1 invalid
invocation, manifest or vault, or failure; 2 blocked; 3 the vault needs an
intervention first. A degraded snapshot never blocks: it is reported on
stdout, and stderr warns of it. Trust the exit code, never the presence of
output.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			plan, err := snapshotter.SnapshotPlan(cmd.Context(), sync.SnapshotRequest{ManifestPath: manifest, VaultPath: vault})
			if err != nil {
				return err
			}
			if _, err := io.WriteString(cmd.OutOrStdout(), RenderSnapshotPlan(plan)); err != nil {
				return err
			}
			warnDegraded(cmd.ErrOrStderr(), plan.Degraded())
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
	_ = command.MarkFlagRequired("manifest")
	_ = command.MarkFlagRequired("vault")
	return command
}

// warnDegraded announces, on every run, the approved snapshots that are not
// intact: never blocking, never silent (ADR 0008).
func warnDegraded(stderr io.Writer, degraded int) {
	if degraded > 0 {
		_, _ = fmt.Fprintf(stderr, "savewarden: warning: %d degraded snapshot(s)\n", degraded)
	}
}

// RenderSnapshotPlan formats a snapshot plan as stable, line-oriented
// English text, with no execution date (ADR 0008).
func RenderSnapshotPlan(plan sync.SnapshotPlan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "manifest: %s\n", printable(plan.ManifestPath))
	fmt.Fprintf(&b, "game: %s (%s)\n", plan.Game.ID, printable(plan.Game.Name))
	fmt.Fprintf(&b, "vault: %s (%s)\n", printable(plan.Vault), plan.VaultState)
	// The space available is not shown: two plans of an unchanged state
	// stay identical byte for byte.
	fmt.Fprintf(&b, "volume: %s, %s drive", printable(plan.Volume.FileSystem), plan.Volume.Drive)
	if plan.Needed > 0 {
		fmt.Fprintf(&b, ", %d bytes needed", plan.Needed)
	}
	b.WriteString("\n")
	current := plan.Current
	switch {
	case current != "" && plan.Recognized:
		current += ", recognized"
	case current != "":
	case plan.CurrentUnchecked:
		current = "not checked, intervention required"
	default:
		current = "none"
	}
	fmt.Fprintf(&b, "current: %s\n", current)
	if len(plan.Pending) == 0 {
		b.WriteString("pending: none\n")
	}
	for _, pending := range plan.Pending {
		last := string(pending.Last)
		if last == "" {
			last = "nothing committed"
		}
		fmt.Fprintf(&b, "pending: %s (%s)\n", pending.Operation, last)
	}
	b.WriteString("mode: plan only, nothing written\n")
	fmt.Fprintf(&b, "plan: %s\n", plan.Outcome())
	fmt.Fprintf(&b, "\nsnapshots: %d published, %d degraded, %d incomplete", len(plan.Snapshots), plan.Degraded(), plan.Incomplete)
	if plan.Incomplete > 0 {
		// Incomplete copies are counted by name, never walked.
		b.WriteString(", size not measured")
	}
	b.WriteString("\n")
	for _, s := range plan.Snapshots {
		fmt.Fprintf(&b, "  %s  from %s  %d files  %d bytes", s.ID, s.Source, s.Files, s.Bytes)
		if s.Degraded() {
			fmt.Fprintf(&b, "  %s", s.Condition)
		}
		b.WriteString("\n")
	}
	switch {
	case plan.UpToDate != "":
		fmt.Fprintf(&b, "snapshot: up to date (%s)\n", plan.UpToDate)
	case plan.Recognized && plan.Blocking() == 0:
		fmt.Fprintf(&b, "snapshot: new, from %s\n", plan.Current)
		fmt.Fprintf(&b, "files: %d  bytes: %d\n", plan.Files, plan.Bytes)
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
