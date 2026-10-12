package inspect

import (
	"crypto/sha256"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/game"
)

// Report is the typed result of one inspection, independent of rendering.
// It is complete: an interrupted or failed inspection returns an error
// instead of a partial report.
type Report struct {
	ManifestPath string
	Game         game.Game
	Installation string
	Sources      []SourceReport
	Findings     []Finding
}

// SourceStatus summarises what happened to one source.
type SourceStatus string

// Source statuses.
const (
	StatusInventoried SourceStatus = "inventoried" // walked; findings may still apply
	StatusMissing     SourceStatus = "missing"
	StatusRefused     SourceStatus = "refused" // not walked: link, non-directory or unreadable parent
)

// SourceReport is the inventory of one declared source.
type SourceReport struct {
	Source  game.Source
	Path    string // Windows path of the source directory
	Status  SourceStatus
	Entries []Entry // sorted by Path, byte order
	Bytes   int64
}

// Entry describes one regular file read in full.
type Entry struct {
	Path    string // slash-separated, relative to the source
	Size    int64
	ModTime time.Time // UTC
	SHA256  [sha256.Size]byte
}

// Severity says whether a finding makes the inspection incomplete.
type Severity int

// Severities.
const (
	Info Severity = iota
	Blocking
)

func (s Severity) String() string {
	if s == Blocking {
		return "blocking"
	}
	return "info"
}

// Code identifies a kind of finding. Codes are stable for scripts.
type Code string

// Finding codes.
const (
	CodeSourceMissing      Code = "source-missing"
	CodeSourceNotDirectory Code = "source-not-directory"
	CodeLinkNotFollowed    Code = "link-not-followed"
	CodeSpecialFile        Code = "special-file"
	CodeUnreadable         Code = "unreadable"
	CodeChangedDuringRead  Code = "changed-during-read"
	CodeReplacedBeforeRead Code = "replaced-before-read"
	CodeCaseCollision      Code = "case-collision"
)

// Finding is an anomaly met during inspection.
type Finding struct {
	Severity Severity
	Code     Code
	Source   string // source id
	// Path is relative to the source with slashes, or a Windows path when
	// the problem lies on the way to the source.
	Path   string
	Detail string
}

// Blocking counts findings that make the inventory untrustworthy.
func (r Report) Blocking() int {
	count := 0
	for _, finding := range r.Findings {
		if finding.Severity == Blocking {
			count++
		}
	}
	return count
}
