package sync

import (
	"fmt"
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

// target is the root an operation writes below, through its session, and
// how its diagnostics name it: a vault, with the directory of a game, for
// current and the snapshots; a restore area for a restore (ADR 0004,
// amendment B4). The engine reads back what it wrote below this root,
// never below the root a plan read from.
type target struct {
	// label and path name the root in diagnostics, path as the user gave
	// it; volume and segments split it.
	label    string
	path     string
	volume   string
	segments []string
	// dir is the directory of the operations below the root, games/<id> in
	// a vault, "." at the root of a restore area; lock is the lock file,
	// relative to dir.
	dir  string
	lock string
}

// vaultTarget is the target of the operations of game gameID in a vault.
func vaultTarget(path, volume string, segments []string, gameID string) target {
	return target{label: "vault", path: path, volume: volume, segments: segments, dir: gamesDir + "/" + gameID, lock: lockFile}
}

// at names rel below the directory of the operations, for the session.
func (t target) at(rel string) string {
	if t.dir == "." {
		return rel
	}
	return t.dir + "/" + rel
}

// errorf reports a root the run must refuse outright: exit code 1.
func (t target) errorf(format string, args ...any) error {
	return fmt.Errorf("%s %s: "+format, append([]any{t.label, t.path}, args...)...)
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
