//go:build !windows

package vault_test

import (
	"errors"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/platform/vault"
)

// POSIX rename(2) replaces its target: the adapter refuses to run.
func TestUnsupportedOutsideWindows(t *testing.T) {
	if _, err := vault.Open("/tmp/vault"); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("Open: %v", err)
	}
	if _, err := vault.Describe("/tmp"); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("Describe: %v", err)
	}
}
