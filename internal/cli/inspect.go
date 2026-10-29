package cli

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/savewarden/savewarden-cli/internal/features/inspect"
)

// ExitIncomplete is the exit code of a complete report with blocking findings.
const ExitIncomplete = 2

// Inspector runs an inspection; the inspect feature implements it.
type Inspector interface {
	Inspect(ctx context.Context, request inspect.Request) (inspect.Report, error)
}

// ExitError asks for a specific process exit code after its report was
// written. Its message is the one-line summary printed on stderr, unless
// Reported says the command already wrote its diagnostic there.
type ExitError struct {
	Code     int
	Message  string
	Reported bool
}

func (e *ExitError) Error() string { return e.Message }

func newInspectCommand(inspector Inspector) *cobra.Command {
	var manifest string
	command := &cobra.Command{
		Use:   "inspect --manifest <path>",
		Short: "Inventory the save sources of a manifest, read-only",
		Long: `Inventory the save sources declared by a manifest, read-only.

Every regular file is listed with its SHA-256, size and modification time.
Links, junctions, reparse points and placeholders are reported, never
followed. Nothing is written, moved or executed.

Exit codes: 0 complete inventory without blocking findings; 1 invalid
invocation, invalid manifest or fatal error, with no report on stdout, or a
report that could not be written in full, in which case stdout may hold a
truncated prefix; 2 complete report with blocking findings. Trust the exit
code, never the presence of output.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := inspector.Inspect(cmd.Context(), inspect.Request{ManifestPath: manifest})
			if err != nil {
				return err
			}
			// A failed write may leave a prefix on stdout that cannot be taken
			// back; the write error, exit code 1, marks it as incomplete.
			if _, err := io.WriteString(cmd.OutOrStdout(), Render(report)); err != nil {
				return err
			}
			if blocking := report.Blocking(); blocking > 0 {
				return &ExitError{Code: ExitIncomplete, Message: fmt.Sprintf("inspection incomplete: %d blocking finding(s)", blocking)}
			}
			return nil
		},
	}
	command.Flags().StringVar(&manifest, "manifest", "", "path to the game's manifest.yaml (required)")
	_ = command.MarkFlagRequired("manifest")
	return command
}

// Render formats a report as stable, line-oriented English text.
func Render(report inspect.Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "manifest: %s\n", printable(report.ManifestPath))
	fmt.Fprintf(&b, "game: %s (%s)\n", report.Game.ID, printable(report.Game.Name))
	fmt.Fprintf(&b, "installation: %s\n", printable(report.Installation))
	for _, source := range report.Sources {
		requirement := "optional"
		if source.Source.Required {
			requirement = "required"
		}
		fmt.Fprintf(&b, "\nsource %s (%s, %s): %s\n", source.Source.ID, source.Source.Kind, requirement, printable(source.Path))
		fmt.Fprintf(&b, "  status: %s\n", source.Status)
		if source.Status != inspect.StatusInventoried {
			continue
		}
		fmt.Fprintf(&b, "  files: %d  bytes: %d\n", len(source.Entries), source.Bytes)
		for _, entry := range source.Entries {
			fmt.Fprintf(&b, "  %s  %d  %s  %s\n", hex.EncodeToString(entry.SHA256[:]), entry.Size,
				entry.ModTime.UTC().Format(time.RFC3339Nano), printable(entry.Path))
		}
	}
	blocking := report.Blocking()
	if len(report.Findings) == 0 {
		b.WriteString("\nfindings: none\n")
		return b.String()
	}
	fmt.Fprintf(&b, "\nfindings: %d blocking, %d info\n", blocking, len(report.Findings)-blocking)
	for _, finding := range report.Findings {
		fmt.Fprintf(&b, "  %s  %s  %s  %s  %s\n", finding.Severity, finding.Code, finding.Source,
			printable(finding.Path), printable(finding.Detail))
	}
	return b.String()
}

// printable quotes text that could hide or forge output: invalid UTF-8,
// non-printable characters, a leading quote or surrounding spaces.
func printable(text string) string {
	if utf8.ValidString(text) && !strings.HasPrefix(text, `"`) && strings.TrimSpace(text) == text &&
		strings.IndexFunc(text, func(r rune) bool { return !unicode.IsPrint(r) }) < 0 {
		return text
	}
	return strconv.Quote(text)
}
