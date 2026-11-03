package sync

import (
	"fmt"
	"strings"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/entities/game"
)

// domain is where the operations of one kind live below the directory of
// their target, how their records are built and how they are named to the
// user: the operations of current (ADR 0006 §4 and §5), or of the
// snapshots (§10). The engine of publication and recovery reads its names
// here.
type domain struct {
	// command is the command that recovers the domain's operations.
	command string
	// journals holds the operation journals and stagings their stagings,
	// both relative to the directory of the target; staging names the
	// staging of op, and published what its publication renames it to.
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

// vaultTarget is the target of the operations of game gameID in the vault
// at path, its volume and segments taken from that one path.
func vaultTarget(path, gameID string) (target, error) {
	volume, segments, err := game.ParseWindowsAbsolute(path)
	if err != nil {
		return target{}, errVault(path, "%v", err)
	}
	return target{label: "vault", path: path, volume: volume, segments: segments, dir: gamesDir + "/" + gameID, lock: lockFile}, nil
}

// at names rel, a slash-separated name, below the directory of the
// operations for the session; "." is that directory itself. A name that
// io/fs refuses is returned as joined, for the session to refuse.
func (t target) at(rel string) string {
	switch {
	case rel == ".":
		return t.dir
	case t.dir == ".":
		return rel
	}
	return t.dir + "/" + rel
}

// chain lists the directories from below the root down to the directory
// of the operations, which a session holds before creating in them.
func (t target) chain() []string {
	if t.dir == "." {
		return nil
	}
	var chain []string
	parts := strings.Split(t.dir, "/")
	for k := range parts {
		chain = append(chain, strings.Join(parts[:k+1], "/"))
	}
	return chain
}

// windowsPath is the Windows path of rel below the directory of the
// operations, as reports name it.
func (t target) windowsPath(rel string) string {
	segments := append([]string{}, t.segments...)
	if name := t.at(rel); name != "." {
		segments = append(segments, strings.Split(name, "/")...)
	}
	return windowsPath(t.volume, segments)
}

// errorf reports a root the run must refuse outright: exit code 1.
func (t target) errorf(format string, args ...any) error {
	return fmt.Errorf("%s %s: "+format, append([]any{t.label, t.path}, args...)...)
}

// readRoot is the source of reading: the vault and game an operation reads,
// which is also the root of its target for every operation but a restore,
// whose target is a restore area (ADR 0004, amendment B4).
type readRoot struct {
	path     string
	volume   string
	segments []string
	gameID   string
}

// vaultReadRoot is the source of reading of game gameID in the vault at path.
func vaultReadRoot(path, gameID string) (readRoot, error) {
	volume, segments, err := game.ParseWindowsAbsolute(path)
	if err != nil {
		return readRoot{}, errVault(path, "%v", err)
	}
	return readRoot{path: path, volume: volume, segments: segments, gameID: gameID}, nil
}

// game returns the segments of rel below the directory of the game, "."
// being that directory, for an inventory of the vault.
func (r readRoot) game(rel string) []string {
	segments := append(append([]string{}, r.segments...), gamesDir, r.gameID)
	if rel == "." {
		return segments
	}
	return append(segments, strings.Split(rel, "/")...)
}

// scopeName is the name of rel below the directory of the game, for a
// read-only scope anchored on the volume root.
func (r readRoot) scopeName(rel string) string { return strings.Join(r.game(rel), "/") }

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
