package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/savewarden/savewarden-cli/internal/app"
)

// version is overridden at link time for an explicitly reviewed build.
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := app.Run(ctx, os.Args[1:], version, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
