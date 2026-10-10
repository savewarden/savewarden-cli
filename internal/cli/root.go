// Package cli owns Cobra commands and human-readable output, not domain logic.
package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// NewRootCommand constructs the bootstrap CLI with explicit version and streams.
func NewRootCommand(version string, stdout, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:           "savewarden",
		Short:         "Your progress. Always protected.",
		Long:          "Savewarden protects game saves.\n\nBootstrap B0: help and version only. No save files are accessed.",
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
