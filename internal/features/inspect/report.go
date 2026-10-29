package inspect

import (
	"github.com/savewarden/savewarden-cli/internal/entities/backup"
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
	Findings     []backup.Finding
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
	Entries []backup.Entry // sorted by Path, byte order
	Bytes   int64
}

// Finding codes emitted by inspect. Codes are stable for scripts.
const (
	CodeSourceMissing      backup.Code = "source-missing"
	CodeSourceNotDirectory backup.Code = "source-not-directory"
	CodeLinkNotFollowed    backup.Code = "link-not-followed"
	CodeSpecialFile        backup.Code = "special-file"
	CodeUnreadable         backup.Code = "unreadable"
	CodeChangedDuringRead  backup.Code = "changed-during-read"
	CodeReplacedBeforeRead backup.Code = "replaced-before-read"
	CodeCaseCollision      backup.Code = "case-collision"
)

// Blocking counts findings that make the inventory untrustworthy.
func (r Report) Blocking() int { return backup.CountBlocking(r.Findings) }
