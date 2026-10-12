// Package app composes the CLI with its adapters and maps execution errors
// to process exit codes.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/savewarden/savewarden-cli/internal/cli"
	"github.com/savewarden/savewarden-cli/internal/features/inspect"
	"github.com/savewarden/savewarden-cli/internal/platform/filesystem"
	"github.com/savewarden/savewarden-cli/internal/platform/manifests"
)

// Run creates a fresh command tree for every invocation. Success is exit
// code 0; an invalid invocation or execution failure is code 1, with no data
// on stdout unless writing stdout itself failed, which may leave a truncated
// prefix; a cli.ExitError selects its own code after its report.
func Run(ctx context.Context, args []string, version string, stdout, stderr io.Writer) int {
	output := &checkedWriter{writer: stdout}
	inspector := inspect.New(manifests.Reader{}, scopes{})
	command := cli.NewRootCommand(version, output, stderr, inspector)
	command.SetArgs(args)
	err := command.ExecuteContext(ctx)
	if output.err != nil {
		// A report that could not be written in full exits 1, never 0 or 2,
		// even though part of it may already be on stdout.
		err = output.err
	}
	if err != nil {
		// Reporting failure cannot change the already unsuccessful exit code.
		_, _ = fmt.Fprintf(stderr, "savewarden: %v\n", err)
		var exit *cli.ExitError
		if errors.As(err, &exit) {
			return exit.Code
		}
		return 1
	}
	return 0
}

// scopes bridges the file-system adapter to the inspect port. It returns a
// nil interface on error, never a typed nil pointer.
type scopes struct{}

func (scopes) OpenScope(root string) (inspect.Scope, error) {
	scope, err := filesystem.OpenScope(root)
	if err != nil {
		return nil, err
	}
	return scope, nil
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
