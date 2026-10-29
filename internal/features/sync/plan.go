package sync

import (
	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/entities/game"
)

// Finding codes emitted by sync, stable for scripts (ADR 0007). Findings
// of the no-follow walk keep the codes of inspect.
const (
	CodeSourceMissing   backup.Code = "source-missing"
	CodeSourceVanished  backup.Code = "source-vanished"
	CodeSourceRemoved   backup.Code = "source-removed-from-manifest"
	CodeFilesRemoved    backup.Code = "files-removed"
	CodeUnportableName  backup.Code = "unportable-name"
	CodeEncryptedSource backup.Code = "encrypted-source"
	CodeCurrentModified backup.Code = "current-modified"
	CodeLongPath        backup.Code = "long-path"
	CodeSameVolume      backup.Code = "same-volume"
)

const (
	// maxPathUTF16 is MAX_PATH less its terminating NUL.
	maxPathUTF16 = 259
	// Read limits for vault metadata: records, receipts, marker.
	recordReadLimit = 64 << 20
	markerReadLimit = 64 << 10
)

// Cause names why a vault needs intervention before any plan (code 3).
type Cause string

// Intervention causes (ADR 0007).
const (
	CausePendingOperation          Cause = "pending-operation"
	CauseInitializationInterrupted Cause = "initialization-interrupted"
	CauseUnexpectedEntry           Cause = "unexpected-entry"
	CauseJournalAmbiguous          Cause = "journal-ambiguous"
)

// Intervention is a state of the vault that the plan refuses to guess.
type Intervention struct {
	Cause   Cause
	Subject string // operation id or path relative to the vault, slash-separated
	Detail  string
}

// VaultState says what the plan found at the vault path.
type VaultState string

// Vault states.
const (
	VaultAbsent      VaultState = "will be created"
	VaultEmpty       VaultState = "empty, will be initialized"
	VaultInitialized VaultState = "initialized"
	VaultInterrupted VaultState = "initialization interrupted"
)

// Pending is an operation whose journal is not terminal.
type Pending struct {
	Operation string
	Last      backup.Step // empty when no record was committed
}

// SourceStatus summarises the plan of one source.
type SourceStatus string

// Source statuses of a plan.
const (
	SourcePlanned SourceStatus = "planned"
	SourceMissing SourceStatus = "missing"
	SourceRefused SourceStatus = "refused"
)

// SourcePlan is the plan of one declared source.
type SourcePlan struct {
	Source  game.Source
	Path    string // Windows path of the source directory
	Status  SourceStatus
	Changes backup.Changes // against the current generation; empty unless planned
	Files   int
	Bytes   int64
}

// Plan is the typed result of a read-only sync plan, independent of
// rendering. It is complete: a failed or interrupted plan returns an error.
type Plan struct {
	ManifestPath string
	Game         game.Game
	Vault        string
	VaultState   VaultState
	Destination  string // Windows path of the current generation
	// Current is the operation that produced the current generation, empty
	// when the vault holds none for this game.
	Current string
	Pending []Pending
	// Sources are planned only when no intervention is required.
	Sources       []SourcePlan
	Interventions []Intervention
	Findings      []backup.Finding
}

// Blocking counts the blocking findings.
func (p Plan) Blocking() int { return backup.CountBlocking(p.Findings) }

// Outcome summarises the plan in one stable phrase.
func (p Plan) Outcome() string {
	switch {
	case len(p.Interventions) > 0:
		return "intervention required"
	case p.Blocking() > 0:
		return "blocked"
	case p.upToDate():
		return "up to date"
	}
	return "changes"
}

// upToDate reports whether a publication would copy exactly the current
// generation again.
func (p Plan) upToDate() bool {
	if p.Current == "" {
		return false
	}
	for _, source := range p.Sources {
		switch source.Status {
		case SourcePlanned:
			if !source.Changes.None() {
				return false
			}
		case SourceMissing:
		default:
			return false
		}
	}
	return true
}
