package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/savewarden/savewarden-cli/internal/features/sync"
)

// Snapshotter plans and applies snapshots of the current generation of a
// vault; the sync feature implements it (ADR 0004, B3 amendment), and app
// gives it the vault writer for SnapshotApply only.
type Snapshotter interface {
	SnapshotPlan(ctx context.Context, request sync.SnapshotRequest) (sync.SnapshotPlan, error)
	SnapshotApply(ctx context.Context, request sync.SnapshotRequest) (sync.SnapshotResult, error)
	SnapshotRecoverPlan(ctx context.Context, request sync.SnapshotRequest) (sync.SnapshotRecovery, error)
	SnapshotRecover(ctx context.Context, request sync.SnapshotRequest) (sync.SnapshotResult, error)
}

func newSnapshotCommand(snapshotter Snapshotter) *cobra.Command {
	var manifest, vault string
	var apply, recover bool
	command := &cobra.Command{
		Use:   "snapshot --manifest <path> --vault <directory> [--apply] [--recover]",
		Short: "Plan an immutable snapshot of the current generation of a vault",
		Long: `Plan an immutable snapshot of the current generation of a vault.

A snapshot copies the generation the vault already holds, checked against its
receipt and its identity: the state saved in the vault, not necessarily the
state of the game now; run sync --apply first to save that. It reads neither
the sources nor the installation, whose identities only keep the vault
distinct from them. The plan lists the approved snapshots, those that are
degraded, and the copies kept incomplete, without walking them.

Without flags, snapshot plans only: nothing is copied, created, moved,
written or executed.

--apply copies current into snapshots\<operation>.incomplete, reads the copy
back, checks its provenance against the receipt of the generation, and
publishes it as snapshots\<operation> with its receipt. Once published, a
snapshot is never opened for writing, moved, replaced or deleted. Nothing
is written outside snapshots\, but the empty lock file, created as sync
creates it.

--recover shows how an interrupted snapshot operation would be recovered,
without writing anything; --recover --apply performs that recovery. It
relies on the snapshot domain alone: neither current nor an operation of
sync can block it, and it never reads current.

Exit codes: 0 plan without blocking findings, up to date or published; 1
invalid invocation, manifest or vault, or failure, the last stderr line
telling what the vault holds; 2 blocked, nothing published by this run; 3
the vault needs an intervention first. With --apply, the last stderr line
is "savewarden: vault: <state>". A degraded snapshot never blocks: it is
reported on stdout, and stderr warns of it. Trust the exit code, never the
presence of output.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			request := sync.SnapshotRequest{ManifestPath: manifest, VaultPath: vault}
			switch {
			case recover && apply:
				result, err := snapshotter.SnapshotRecover(cmd.Context(), request)
				report := ""
				if result.Recovery != nil && result.Recovery.ManifestPath != "" {
					report = RenderSnapshotRecovery(*result.Recovery, "recovery")
					warnDegraded(cmd.ErrOrStderr(), result.Recovery.Degraded())
				}
				return finish(cmd, report, result.State(), err)
			case recover:
				recovery, err := snapshotter.SnapshotRecoverPlan(cmd.Context(), request)
				report := ""
				if recovery.ManifestPath != "" {
					report = RenderSnapshotRecovery(recovery, "recovery plan only, nothing written")
					warnDegraded(cmd.ErrOrStderr(), recovery.Degraded())
				}
				state := sync.SnapshotResult{}.State()
				var intervention *sync.InterventionError
				if errors.As(err, &intervention) {
					state = sync.SnapshotResult{Operation: intervention.Operation, Effect: sync.EffectIntervention, Cause: intervention.Cause}.State()
				}
				return finish(cmd, report, state, err)
			}
			if apply {
				result, err := snapshotter.SnapshotApply(cmd.Context(), request)
				report := ""
				if result.Plan.ManifestPath != "" {
					report = RenderSnapshotApply(result)
					warnDegraded(cmd.ErrOrStderr(), result.Plan.Degraded())
				}
				return finish(cmd, report, result.State(), err)
			}
			plan, err := snapshotter.SnapshotPlan(cmd.Context(), request)
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
	command.Flags().BoolVar(&apply, "apply", false, "copy, verify and publish the snapshot; with --recover, perform the recovery")
	command.Flags().BoolVar(&recover, "recover", false, "show how an interrupted snapshot operation would be recovered")
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

// RenderSnapshotApply formats the plan of a snapshot --apply run, then
// what it did.
func RenderSnapshotApply(result sync.SnapshotResult) string {
	var b strings.Builder
	b.WriteString(renderSnapshotPlan(result.Plan, "apply: copy, verify and publish"))
	b.WriteString("\n")
	operation := result.Operation
	if operation == "" {
		operation = "none"
	}
	fmt.Fprintf(&b, "operation: %s\n", operation)
	if result.Effect == sync.EffectPublished {
		fmt.Fprintf(&b, "verification: %d files, %d bytes, SHA-256 verified\n", result.Files, result.Bytes)
		fmt.Fprintf(&b, "published: %s\n", printable(result.Snapshot))
		fmt.Fprintf(&b, "receipt: %s\n", printable(result.Receipt))
	}
	return b.String()
}

// RenderSnapshotRecovery formats what snapshot --recover found and would
// do, or did.
func RenderSnapshotRecovery(r sync.SnapshotRecovery, mode string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "manifest: %s\n", printable(r.ManifestPath))
	fmt.Fprintf(&b, "game: %s (%s)\n", r.Game.ID, printable(r.Game.Name))
	fmt.Fprintf(&b, "vault: %s\n", printable(r.Vault))
	fmt.Fprintf(&b, "volume: %s, %s drive\n", printable(r.Volume.FileSystem), r.Volume.Drive)
	operation, last := r.Operation, string(r.Last)
	if operation == "" {
		operation = "none"
	}
	if last == "" {
		last = "none"
	}
	fmt.Fprintf(&b, "snapshot operation: %s\n", operation)
	fmt.Fprintf(&b, "last committed step: %s\n", last)
	if r.Observed != nil {
		fmt.Fprintf(&b, "observed: snapshot %s, incomplete %s", r.Observed.Published, r.Observed.Incomplete)
		if r.Observed.Receipt {
			b.WriteString(", receipt committed")
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "action: %s\n", r.Action)
	fmt.Fprintf(&b, "mode: %s\n", mode)
	return b.String()
}

// RenderSnapshotPlan formats a snapshot plan as stable, line-oriented
// English text, with no execution date (ADR 0008).
func RenderSnapshotPlan(plan sync.SnapshotPlan) string {
	return renderSnapshotPlan(plan, "plan only, nothing written")
}

func renderSnapshotPlan(plan sync.SnapshotPlan, mode string) string {
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
	fmt.Fprintf(&b, "mode: %s\n", mode)
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
