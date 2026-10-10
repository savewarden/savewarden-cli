// Package app composes the CLI and maps execution errors to process exit codes.
package app

import (
	"context"
	"fmt"
	"io"

	"github.com/savewarden/savewarden-cli/internal/cli"
)

// Run creates a fresh command tree for every invocation. It never accesses saves.
// Success is exit code 0; an invalid invocation or execution failure is code 1.
func Run(ctx context.Context, args []string, version string, stdout, stderr io.Writer) int {
	output := &checkedWriter{writer: stdout}
	command := cli.NewRootCommand(version, output, stderr)
	command.SetArgs(args)
	err := command.ExecuteContext(ctx)
	if err == nil {
		err = output.err
	}
	if err != nil {
		// Reporting failure cannot change the already unsuccessful exit code.
		_, _ = fmt.Fprintf(stderr, "savewarden: %v\n", err)
		return 1
	}
	return 0
}

// Cobra's built-in help/version printers do not return write failures.
// Retain them at the application boundary to keep the exit contract reliable.
type checkedWriter struct {
	writer io.Writer
	err    error
}

func (w *checkedWriter) Write(data []byte) (int, error) {
	n, err := w.writer.Write(data)
	if n < len(data) && err == nil {
		err = io.ErrShortWrite
	}
	if w.err == nil {
		w.err = err
	}
	return n, err
}
