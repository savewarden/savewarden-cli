package sync

import (
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

// domain is where the operations of one kind live in the game directory,
// how their records are built and how they are named to the user: the
// operations of current (ADR 0006 §4 and §5), or of the snapshots (§10).
// The engine of publication and recovery reads its names here.
type domain struct {
	// command is the command that recovers the domain's operations.
	command string
	// journals holds the operation journals and stagings their stagings,
	// both relative to the game directory; staging names the staging of op,
	// and published what its publication renames it to.
	journals, stagings string
	staging, published func(op string) string
	newRecord          func(seq int, step backup.Step, op string, at time.Time, prev string, v backup.StepValues) (backup.Record, error)
	// changed is the code of a planned file found changed during its copy;
	// busy, the diagnostic of the lock held by another operation.
	changed backup.Code
	busy    string
}

// currentDomain holds current, its retained generations and their
// operations.
var currentDomain = domain{
	command:   "sync",
	journals:  journalDir,
	stagings:  stagingDir,
	staging:   backup.StagingName,
	published: func(string) string { return currentDir },
	newRecord: backup.NewRecord,
	changed:   CodeChangedDuringSync,
	busy:      "another sync is running on this game",
}
