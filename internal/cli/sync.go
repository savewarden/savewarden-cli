package cli

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/savewarden/savewarden-cli/internal/features/sync"
)

// ExitIntervention is the exit code of a plan that needs a human first:
// pending operation, interrupted initialization or ambiguous vault.
const ExitIntervention = 3

// Planner computes a sync plan; the sync feature implements it.
type Planner interface {
	Plan(ctx context.Context, request sync.Request) (sync.Plan, error)
}

func newSyncCommand(planner Planner) *cobra.Command {
	var manifest, vault string
	command := &cobra.Command{
		Use:   "sync --manifest <path> --vault <directory>",
		Short: "Plan the copy of a manifest's sources into a vault, read-only",
		Long: `Plan the copy of the sources declared by a manifest into a vault, read-only.

This build only plans: nothing is copied, created, moved, written or
executed, neither in the sources nor in the vault. The plan lists every file
as added (+), modified (~), removed (-) or unchanged (=) against the current
generation of the vault, then the interventions and findings. Copying with
--apply arrives in a later build.

Exit codes: 0 plan without blocking findings; 1 invalid invocation, manifest
or vault, or fatal error, with no plan on stdout, or a plan that could not be
written in full, in which case stdout may hold a truncated prefix; 2 complete
plan with blocking findings; 3 the vault needs an intervention first. Trust
the exit code, never the presence of output.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			plan, err := planner.Plan(cmd.Context(), sync.Request{ManifestPath: manifest, VaultPath: vault})
			if err != nil {
				return err
			}
			// As for inspect, a failed write may leave a prefix on stdout; the
			// write error, exit code 1, marks it as incomplete.
			if _, err := io.WriteString(cmd.OutOrStdout(), RenderPlan(plan)); err != nil {
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
	_ = command.MarkFlagRequired("manifest")
	_ = command.MarkFlagRequired("vault")
	return command
}

// RenderPlan formats a plan as stable, line-oriented English text, with no
// execution date (ADR 0007).
func RenderPlan(plan sync.Plan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "manifest: %s\n", printable(plan.ManifestPath))
	fmt.Fprintf(&b, "game: %s (%s)\n", plan.Game.ID, printable(plan.Game.Name))
	fmt.Fprintf(&b, "vault: %s (%s)\n", printable(plan.Vault), plan.VaultState)
	fmt.Fprintf(&b, "destination: %s\n", printable(plan.Destination))
	b.WriteString("volume: not checked by this build\n")
	current := plan.Current
	switch {
	case current != "":
	case plan.CurrentUnchecked:
		current = "not checked, intervention required"
	default:
		current = "none"
	}
	fmt.Fprintf(&b, "current: %s\n", current)
	if len(plan.Pending) == 0 {
		pending := "none"
		for _, intervention := range plan.Interventions {
			if intervention.Cause == sync.CauseJournalAmbiguous {
				pending = "unknown, ambiguous journal"
			}
		}
		fmt.Fprintf(&b, "pending: %s\n", pending)
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
	for _, source := range plan.Sources {
		requirement := "optional"
		if source.Source.Required {
			requirement = "required"
		}
		fmt.Fprintf(&b, "\nsource %s (%s, %s): %s\n", source.Source.ID, source.Source.Kind, requirement, printable(source.Path))
		fmt.Fprintf(&b, "  status: %s\n", source.Status)
		if source.Status != sync.SourcePlanned {
			continue
		}
		c := source.Changes
		fmt.Fprintf(&b, "  files: %d  bytes: %d  added: %d  modified: %d  removed: %d  unchanged: %d\n",
			source.Files, source.Bytes, c.Added, c.Modified, c.Removed, c.Unchanged)
		for _, change := range c.Files {
			fmt.Fprintf(&b, "  %s %s  %d  %s  %s\n", change.Kind.Marker(), hex.EncodeToString(change.Entry.SHA256[:]),
				change.Entry.Size, change.Entry.ModTime.UTC().Format(time.RFC3339Nano), printable(change.Entry.Path))
		}
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

// orDash stands for an empty column, such as the source of a finding about
// the vault itself.
func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
