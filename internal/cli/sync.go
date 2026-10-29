package cli

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/savewarden/savewarden-cli/internal/features/sync"
)

// ExitIntervention is the exit code of a run that needs a human first:
// pending operation, interrupted initialization, ambiguous vault, failed
// rollback or exhausted recovery envelope.
const ExitIntervention = 3

// Syncer plans, applies and recovers synchronisations; the sync feature
// implements it, and app gives it the vault writer for Apply and Recover
// only.
type Syncer interface {
	Plan(ctx context.Context, request sync.Request) (sync.Plan, error)
	Apply(ctx context.Context, request sync.Request) (sync.Result, error)
	RecoverPlan(ctx context.Context, request sync.Request) (sync.Recovery, error)
	Recover(ctx context.Context, request sync.Request) (sync.Result, error)
}

func newSyncCommand(syncer Syncer) *cobra.Command {
	var manifest, vault string
	var apply, recover bool
	command := &cobra.Command{
		Use:   "sync --manifest <path> --vault <directory> [--apply] [--recover]",
		Short: "Plan, apply or recover the copy of a manifest's sources into a vault",
		Long: `Plan, apply or recover the copy of the sources declared by a manifest into
a vault.

Without flags, sync plans only: nothing is copied, created, moved, written or
executed, neither in the sources nor in the vault. The plan lists every file
as added (+), modified (~), removed (-) or unchanged (=) against the current
generation of the vault, then the interventions and findings.

--apply copies the sources into a staging directory of the vault, verifies
it, and publishes it as the first generation of the game. This build
refuses to replace an existing generation. Sources are only read; nothing is
ever deleted or overwritten.

--recover shows how an interrupted operation would be recovered, without
writing anything; --recover --apply performs that recovery.

Exit codes: 0 success: plan without blocking findings, published, up to date
or recovered; 1 invalid invocation, manifest or vault, or failure, the last
stderr line telling what the vault holds; 2 blocked, nothing published by
this run; 3 the vault needs an intervention first. With --apply or
--recover, the last stderr line is "savewarden: vault: <state>". Trust the
exit code, never the presence of output.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			request := sync.Request{ManifestPath: manifest, VaultPath: vault}
			switch {
			case recover && apply:
				result, err := syncer.Recover(cmd.Context(), request)
				report := ""
				if result.Recovery != nil && result.Recovery.ManifestPath != "" {
					report = RenderRecovery(*result.Recovery, "recovery")
				}
				return finish(cmd, report, result.State(), err)
			case recover:
				recovery, err := syncer.RecoverPlan(cmd.Context(), request)
				report := ""
				if recovery.ManifestPath != "" {
					report = RenderRecovery(recovery, "recovery plan only, nothing written")
				}
				state := sync.Result{}.State()
				var intervention *sync.InterventionError
				if errors.As(err, &intervention) {
					state = sync.Result{Operation: intervention.Operation, Effect: sync.EffectIntervention, Cause: intervention.Cause}.State()
				}
				return finish(cmd, report, state, err)
			case apply:
				result, err := syncer.Apply(cmd.Context(), request)
				report := ""
				if result.Plan.ManifestPath != "" {
					report = RenderApply(result)
				}
				return finish(cmd, report, result.State(), err)
			}
			plan, err := syncer.Plan(cmd.Context(), request)
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
	command.Flags().BoolVar(&apply, "apply", false, "copy, verify and publish; with --recover, perform the recovery")
	command.Flags().BoolVar(&recover, "recover", false, "show how an interrupted operation would be recovered")
	_ = command.MarkFlagRequired("manifest")
	_ = command.MarkFlagRequired("vault")
	return command
}

// finish writes the report of an --apply or --recover run, then on stderr
// its diagnostic and, last, the vault state line (ADR 0007). The exit code
// describes the run; the state line, what the vault now holds.
func finish(cmd *cobra.Command, report, state string, err error) error {
	code := 0
	var blocked *sync.BlockedError
	var intervention *sync.InterventionError
	switch {
	case err == nil:
	case errors.As(err, &intervention):
		code = ExitIntervention
	case errors.As(err, &blocked):
		code = ExitIncomplete
	default:
		code = 1
	}
	if _, writeErr := io.WriteString(cmd.OutOrStdout(), report); writeErr != nil {
		// The vault keeps what the run did: only the report is incomplete.
		code, err = 1, errors.Join(err, writeErr)
	}
	stderr := cmd.ErrOrStderr()
	if err != nil {
		// Reporting failure cannot change the already unsuccessful exit code.
		_, _ = fmt.Fprintf(stderr, "savewarden: %s\n", printable(strings.ReplaceAll(err.Error(), "\n", "; ")))
	}
	_, _ = fmt.Fprintf(stderr, "savewarden: vault: %s\n", printable(state))
	if code == 0 {
		return nil
	}
	return &ExitError{Code: code, Message: "", Reported: true}
}

// RenderApply formats the plan of an --apply run, then what it did.
func RenderApply(result sync.Result) string {
	var b strings.Builder
	b.WriteString(renderPlan(result.Plan, "apply: copy, verify and publish"))
	b.WriteString("\n")
	operation := result.Operation
	if operation == "" {
		operation = "none"
	}
	fmt.Fprintf(&b, "operation: %s\n", operation)
	if result.Effect == sync.EffectPublished {
		fmt.Fprintf(&b, "verification: %d files, %d bytes, SHA-256 verified\n", result.Files, result.Bytes)
		fmt.Fprintf(&b, "published: %s\n", printable(result.Plan.Destination))
		fmt.Fprintf(&b, "receipt: %s\n", printable(result.Receipt))
	}
	return b.String()
}

// RenderRecovery formats what --recover found and would do, or did.
func RenderRecovery(r sync.Recovery, mode string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "manifest: %s\n", printable(r.ManifestPath))
	fmt.Fprintf(&b, "game: %s (%s)\n", r.Game.ID, printable(r.Game.Name))
	fmt.Fprintf(&b, "vault: %s (%s)\n", printable(r.Vault), r.VaultState)
	fmt.Fprintf(&b, "volume: %s, %s drive\n", printable(r.Volume.FileSystem), r.Volume.Drive)
	operation, last := r.Operation, string(r.Last)
	if operation == "" {
		operation = "none"
	}
	if last == "" {
		last = "none"
	}
	fmt.Fprintf(&b, "operation: %s\n", operation)
	fmt.Fprintf(&b, "last committed step: %s\n", last)
	if r.Observed != nil {
		fmt.Fprintf(&b, "observed: current %s, staging %s\n", r.Observed.Current, r.Observed.Staging)
	}
	fmt.Fprintf(&b, "action: %s\n", r.Action)
	fmt.Fprintf(&b, "mode: %s\n", mode)
	return b.String()
}

// RenderPlan formats a plan as stable, line-oriented English text, with no
// execution date (ADR 0007).
func RenderPlan(plan sync.Plan) string { return renderPlan(plan, "plan only, nothing written") }

func renderPlan(plan sync.Plan, mode string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "manifest: %s\n", printable(plan.ManifestPath))
	fmt.Fprintf(&b, "game: %s (%s)\n", plan.Game.ID, printable(plan.Game.Name))
	fmt.Fprintf(&b, "vault: %s (%s)\n", printable(plan.Vault), plan.VaultState)
	fmt.Fprintf(&b, "destination: %s\n", printable(plan.Destination))
	// The space available is not shown: it varies between two plans of an
	// unchanged state, which must stay identical byte for byte.
	fmt.Fprintf(&b, "volume: %s, %s drive", printable(plan.Volume.FileSystem), plan.Volume.Drive)
	if plan.Needed > 0 {
		fmt.Fprintf(&b, ", %d bytes needed", plan.Needed)
	}
	b.WriteString("\n")
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
	fmt.Fprintf(&b, "mode: %s\n", mode)
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
