// Package cli owns Cobra commands and human-readable output, not domain logic.
package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// NewRootCommand constructs the CLI with explicit version, streams and
// services.
func NewRootCommand(version string, stdout, stderr io.Writer, inspector Inspector, planner Planner) *cobra.Command {
	root := &cobra.Command{
		Use:   "savewarden",
		Short: "Your progress. Always protected.",
		Long: "Savewarden protects game saves.\n\n" +
			"This build inspects declared save sources and plans their synchronisation\n" +
			"into a vault, read-only: nothing is copied, moved, written or executed.\n" +
			"Copying, snapshots and restoration come later.",
		Version:       version,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetVersionTemplate("savewarden {{.Version}}\n")
	root.CompletionOptions.DisableDefaultCmd = true
	root.DisableSuggestions = true
	root.AddCommand(newInspectCommand(inspector))
	root.AddCommand(newSyncCommand(planner))
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the build version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "savewarden %s\n", version)
			return err
		},
	})
	return root
}
