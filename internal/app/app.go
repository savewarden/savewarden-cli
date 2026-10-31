// Package app composes the CLI with its adapters and maps execution errors
// to process exit codes.
package app

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/savewarden/savewarden-cli/internal/cli"
	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/features/inspect"
	"github.com/savewarden/savewarden-cli/internal/features/sync"
	"github.com/savewarden/savewarden-cli/internal/platform/filesystem"
	"github.com/savewarden/savewarden-cli/internal/platform/manifests"
	"github.com/savewarden/savewarden-cli/internal/platform/vault"
)

// Run creates a fresh command tree for every invocation. Success is exit
// code 0; an invalid invocation or execution failure is code 1, with no data
// on stdout unless writing stdout itself failed, which may leave a truncated
// prefix; a cli.ExitError selects its own code after its report.
func Run(ctx context.Context, args []string, version string, stdout, stderr io.Writer) int {
	output := &checkedWriter{writer: stdout}
	inspector := inspect.New(manifests.Reader{}, scopes{})
	// The inspect service satisfies sync's Inventories port as is (ADR 0004).
	// Only --apply and --recover --apply receive the writer (ADR 0005, 0007).
	syncer := syncCommands{
		service: sync.New(manifests.Reader{}, inspector, syncScopes{}, volumes{}),
		writer:  sync.Writer{Vaults: vaults{}, Clock: systemClock{}, Random: rand.Reader, ToolVersion: version},
	}
	// The sync feature owns snapshots too (ADR 0004, B3 amendment).
	command := cli.NewRootCommand(version, output, stderr, inspector, syncer, syncer)
	command.SetArgs(args)
	err := command.ExecuteContext(ctx)
	var exit *cli.ExitError
	reported := errors.As(err, &exit) && exit.Reported
	if output.err != nil && !reported {
		// A report that could not be written in full exits 1, never 0 or 2,
		// even though part of it may already be on stdout.
		err = output.err
	}
	if err != nil {
		if !reported {
			// Reporting failure cannot change the already unsuccessful exit code.
			_, _ = fmt.Fprintf(stderr, "savewarden: %v\n", err)
		}
		if errors.As(err, &exit) {
			return exit.Code
		}
		return 1
	}
	return 0
}

// syncCommands gives the vault writer to Apply and Recover only: the plan
// and the recovery plan never receive it.
type syncCommands struct {
	service *sync.Service
	writer  sync.Writer
}

func (s syncCommands) Plan(ctx context.Context, request sync.Request) (sync.Plan, error) {
	return s.service.Plan(ctx, request)
}

func (s syncCommands) Apply(ctx context.Context, request sync.Request) (sync.Result, error) {
	return s.service.Apply(ctx, request, s.writer)
}

func (s syncCommands) RecoverPlan(ctx context.Context, request sync.Request) (sync.Recovery, error) {
	return s.service.RecoverPlan(ctx, request)
}

func (s syncCommands) Recover(ctx context.Context, request sync.Request) (sync.Result, error) {
	return s.service.Recover(ctx, request, s.writer)
}

func (s syncCommands) SnapshotPlan(ctx context.Context, request sync.SnapshotRequest) (sync.SnapshotPlan, error) {
	return s.service.SnapshotPlan(ctx, request)
}

func (s syncCommands) SnapshotApply(ctx context.Context, request sync.SnapshotRequest) (sync.SnapshotResult, error) {
	return s.service.SnapshotApply(ctx, request, s.writer)
}

// vaults bridges the create-only vault adapter to the sync port. It
// returns a nil interface on error, never a typed nil pointer.
type vaults struct{}

func (vaults) Open(path string) (sync.Vault, error) {
	session, err := vault.Open(path)
	if err != nil {
		return nil, err
	}
	return session, nil
}

// systemClock is the wall clock; Sleep stops early when ctx is done.
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func (systemClock) Sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
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

// syncScopes bridges the same read-only adapter to the sync port.
type syncScopes struct{}

func (syncScopes) OpenScope(root string) (sync.Scope, error) {
	scope, err := filesystem.OpenScope(root)
	if err != nil {
		return nil, err
	}
	return scope, nil
}

// volumes bridges the read-only volume queries of the vault adapter.
type volumes struct{}

func (volumes) Describe(path string) (backup.Volume, error) { return vault.Describe(path) }

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
