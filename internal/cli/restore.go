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

// Restorer plans and applies restores of an approved snapshot into an
// isolated restore area; the sync feature implements it (ADR 0004, B4
// amendment), and app gives it the area writer for RestoreApply only.
type Restorer interface {
	RestorePlan(ctx context.Context, request sync.RestoreRequest) (sync.RestorePlan, error)
	RestoreApply(ctx context.Context, request sync.RestoreRequest) (sync.RestoreResult, error)
	RestoreRecoverPlan(ctx context.Context, request sync.RestoreRequest) (sync.RestoreRecovery, error)
	RestoreRecover(ctx context.Context, request sync.RestoreRequest) (sync.RestoreResult, error)
}

func newRestoreCommand(restorer Restorer) *cobra.Command {
	var manifest, vault, snapshot, destination string
	var apply, recover bool
	command := &cobra.Command{
		Use: "restore --manifest <path> --vault <directory> --snapshot <id> --to <directory> [--apply]\n" +
			"  savewarden restore --manifest <path> --to <directory> --recover [--apply]",
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

Without --apply, restore plans only: nothing is copied, created, moved,
written or executed.

--apply marks a new area, takes its lock, copies the snapshot into
<to>\<operation>.incomplete, reads the copy back, and publishes it as
<to>\<operation> with its receipt. Each --apply publishes a new restore; a
published restore is handed over to you: restore never opens it for
writing, moves or deletes it again. Copy out what you need rather than
renaming anything in the area.

--recover shows how an interrupted restore of the area would be recovered,
without writing anything; --recover --apply performs that recovery. It
relies on the area and the manifest alone and never reads the vault:
--vault and --snapshot are refused with it.

Exit codes: 0 plan without blocking findings, restore published, or
recovery done or not needed; 1
invalid invocation, manifest, vault or destination, a snapshot that is not
found, not published, not verifiable or not restorable now, or a failure;
2 blocked, such as a degraded snapshot, insufficient space or a snapshot
changed during the copy, nothing published; 3 the restore area needs an
intervention first. With --apply or --recover, the last stderr line is
"savewarden: destination: <state>"; do not use <to>\<operation> before it
says "published restore <operation>". Trust the exit code, never the
presence of output.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			switch {
			case recover && (vault != "" || snapshot != ""):
				return errors.New("--recover never reads the vault: --vault and --snapshot are refused with it")
			case !recover && (vault == "" || snapshot == ""):
				return errors.New(`required flag(s) "snapshot" and "vault" not set; they are refused only with --recover`)
			}
			request := sync.RestoreRequest{ManifestPath: manifest, VaultPath: vault, Snapshot: snapshot, Destination: destination}
			switch {
			case recover && apply:
				result, err := restorer.RestoreRecover(cmd.Context(), request)
				report := ""
				if result.Recovery != nil && result.Recovery.ManifestPath != "" {
					report = RenderRestoreRecovery(*result.Recovery, "recovery")
				}
				return finishAs(cmd, report, "destination", result.State(), err)
			case recover:
				recovery, err := restorer.RestoreRecoverPlan(cmd.Context(), request)
				report := ""
				if recovery.ManifestPath != "" {
					report = RenderRestoreRecovery(recovery, "recovery plan only, nothing written")
				}
				state := sync.RestoreResult{}.State()
				var intervention *sync.InterventionError
				if errors.As(err, &intervention) {
					state = sync.RestoreResult{Operation: intervention.Operation, Effect: sync.EffectIntervention, Cause: intervention.Cause}.State()
				}
				return finishAs(cmd, report, "destination", state, err)
			}
			if apply {
				result, err := restorer.RestoreApply(cmd.Context(), request)
				report := ""
				if result.Plan.ManifestPath != "" {
					report = RenderRestoreApply(result)
				}
				return finishAs(cmd, report, "destination", result.State(), err)
			}
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
	command.Flags().StringVar(&vault, "vault", "", "vault directory, absolute Windows path (required, refused with --recover)")
	command.Flags().StringVar(&snapshot, "snapshot", "", "approved snapshot to restore, such as 20261106T101500-5e6f7a8b (required, refused with --recover)")
	command.Flags().StringVar(&destination, "to", "", "restore area, absolute Windows path: new, empty, or marked by restore (required)")
	command.Flags().BoolVar(&apply, "apply", false, "copy, verify and publish the restore; with --recover, perform the recovery")
	command.Flags().BoolVar(&recover, "recover", false, "show how an interrupted restore of the area would be recovered")
	for _, name := range []string{"manifest", "to"} {
		_ = command.MarkFlagRequired(name)
	}
	return command
}

// RenderRestoreRecovery formats what restore --recover found and would do,
// or did. The vault is never named: a recovery does not read it.
func RenderRestoreRecovery(r sync.RestoreRecovery, mode string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "manifest: %s\n", printable(r.ManifestPath))
	fmt.Fprintf(&b, "game: %s (%s)\n", r.Game.ID, printable(r.Game.Name))
	fmt.Fprintf(&b, "destination: %s (%s)", printable(r.Destination), r.State)
	if r.State == sync.DestinationArea {
		fmt.Fprintf(&b, ", %d published", len(r.Restores))
	}
	b.WriteString("\n")
	if r.Volume.FileSystem != "" {
		fmt.Fprintf(&b, "volume: %s, %s drive\n", printable(r.Volume.FileSystem), r.Volume.Drive)
	}
	operation, last := r.Operation, string(r.Last)
	if operation == "" {
		operation = "none"
	}
	if last == "" {
		last = "none"
	}
	fmt.Fprintf(&b, "restore operation: %s\n", operation)
	fmt.Fprintf(&b, "last committed step: %s\n", last)
	if r.Observed != nil {
		fmt.Fprintf(&b, "observed: restore %s, incomplete %s", r.Observed.Published, r.Observed.Incomplete)
		if r.Observed.Receipt {
			b.WriteString(", receipt committed")
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "action: %s\n", r.Action)
	if warning := r.Warning(); warning != "" {
		fmt.Fprintf(&b, "warning: %s\n", printable(warning))
	}
	fmt.Fprintf(&b, "mode: %s\n", mode)
	return b.String()
}

// RenderRestoreApply formats the plan of a restore --apply run, then what
// it did.
func RenderRestoreApply(result sync.RestoreResult) string {
	var b strings.Builder
	b.WriteString(renderRestorePlan(result.Plan, "apply: copy, verify and publish"))
	b.WriteString("\n")
	operation := result.Operation
	if operation == "" {
		operation = "none"
	}
	fmt.Fprintf(&b, "operation: %s\n", operation)
	if result.Effect == sync.EffectPublished {
		fmt.Fprintf(&b, "verification: %d files, %d bytes, SHA-256 verified\n", result.Files, result.Bytes)
		fmt.Fprintf(&b, "published: %s\n", printable(result.Restore))
		fmt.Fprintf(&b, "receipt: %s\n", printable(result.Receipt))
	}
	return b.String()
}

// RenderRestorePlan formats a restore plan as stable, line-oriented English
// text, with no execution date (ADR 0009): the header and the snapshot's
// verdict, the destination and its handed-over restores, the decision, the
// interventions, then the findings.
func RenderRestorePlan(plan sync.RestorePlan) string {
	return renderRestorePlan(plan, "plan only, nothing written")
}

func renderRestorePlan(plan sync.RestorePlan, mode string) string {
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
	fmt.Fprintf(&b, "mode: %s\n", mode)
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
