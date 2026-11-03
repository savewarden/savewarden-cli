package backup

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/game"
)

// Restore area identifiers (ADR 0006 §11, ADR 0009).
const (
	RestoreAreaMarkerName    = "savewarden-restore.json"
	RestoreAreaSchema        = "savewarden.restore-area"
	RestoreAreaSchemaVersion = 1
	RestoreLockName          = "restore.lock"
	RestoreReceiptsDir       = "receipts"
	RestoreJournalDir        = "journal"
)

// RestoreAreaMarker is the content of savewarden-restore.json: an area
// serves one game only.
type RestoreAreaMarker struct {
	CreatedAt   time.Time
	ToolVersion string
	GameID      string
}

// EncodeRestoreAreaMarker returns the JSON form of m, ending with LF.
func EncodeRestoreAreaMarker(m RestoreAreaMarker) ([]byte, error) {
	if err := m.validate(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(struct {
		CreatedAt     string `json:"created_at"`
		Game          string `json:"game"`
		Schema        string `json:"schema"`
		SchemaVersion int    `json:"schema_version"`
		ToolVersion   string `json:"tool_version"`
	}{formatUTC(m.CreatedAt), m.GameID, RestoreAreaSchema, RestoreAreaSchemaVersion, m.ToolVersion})
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// DecodeRestoreAreaMarker reads a restore area marker strictly. A vault
// marker is not an area marker, nor the reverse.
func DecodeRestoreAreaMarker(data []byte) (RestoreAreaMarker, error) {
	var raw struct {
		CreatedAt     *string `json:"created_at"`
		Game          *string `json:"game"`
		Schema        *string `json:"schema"`
		SchemaVersion *int    `json:"schema_version"`
		ToolVersion   *string `json:"tool_version"`
	}
	if err := decodeStrict(data, &raw); err != nil {
		return RestoreAreaMarker{}, fmt.Errorf("restore area marker: %w", err)
	}
	schema, err := required("schema", raw.Schema)
	if err != nil {
		return RestoreAreaMarker{}, fmt.Errorf("restore area marker: %w", err)
	}
	version, err := required("schema_version", raw.SchemaVersion)
	if err != nil {
		return RestoreAreaMarker{}, fmt.Errorf("restore area marker: %w", err)
	}
	if schema != RestoreAreaSchema || version != RestoreAreaSchemaVersion {
		return RestoreAreaMarker{}, fmt.Errorf("restore area marker: unsupported schema %q version %d; this build reads %s version %d", schema, version, RestoreAreaSchema, RestoreAreaSchemaVersion)
	}
	var m RestoreAreaMarker
	if m.CreatedAt, err = requiredTime("created_at", raw.CreatedAt); err != nil {
		return RestoreAreaMarker{}, fmt.Errorf("restore area marker: %w", err)
	}
	if m.GameID, err = required("game", raw.Game); err != nil {
		return RestoreAreaMarker{}, fmt.Errorf("restore area marker: %w", err)
	}
	if m.ToolVersion, err = required("tool_version", raw.ToolVersion); err != nil {
		return RestoreAreaMarker{}, fmt.Errorf("restore area marker: %w", err)
	}
	return m, m.validate()
}

func (m RestoreAreaMarker) validate() error {
	if m.ToolVersion == "" {
		return fmt.Errorf("restore area marker: tool_version must not be empty")
	}
	if err := game.CheckID(m.GameID); err != nil {
		return fmt.Errorf("restore area marker: game: %w", err)
	}
	return nil
}

// RestoreRootState classifies the content of a restore destination
// (ADR 0006 §11).
type RestoreRootState int

// Restore destination states.
const (
	RestoreRootEmpty       RestoreRootState = iota // nothing to recover; a new area starts at Z3
	RestoreRootInterrupted                         // only marker temporaries: resumed by --recover --apply only
	RestoreRootMarked                              // a marker file is present; its content is checked separately
	RestoreRootForeign                             // not a restore area: refused, never adopted
)

// RestoreObject is an entry at the root of a restore area that carries the
// name of an operation, published copy or .incomplete, of any kind.
type RestoreObject struct {
	Name string
	Kind EntryKind
}

// RestoreRoot is the classification of a restore destination listing.
type RestoreRoot struct {
	State RestoreRootState
	// Temporaries counts the marker temporaries; at MaxTemporaries the
	// recovery envelope is exhausted. Taken holds their attempt numbers.
	Temporaries int
	Taken       map[int]bool
	// Lock, Receipts and Journal report the structure of a marked area.
	Lock, Receipts, Journal bool
	// Published and Incomplete hold, by operation id in its canonical form,
	// the entry named after the operation and its .incomplete copy.
	Published, Incomplete map[string]RestoreObject
	// Unexpected lists, for a marked area, the entries that have no place
	// at its root, sorted: they are ambiguities.
	Unexpected []string
}

var (
	areaMarkerTemporary = regexp.MustCompile(`^` + regexp.QuoteMeta(RestoreAreaMarkerName) + `\.tmp-([1-9])$`)
	areaOperation       = regexp.MustCompile(`(?i)^[0-9]{8}T[0-9]{6}-[0-9a-f]{8}$`)
)

const (
	operationIDLength = len("20261105T101500-1a2b3c4d")
	incompleteSuffix  = ".incomplete"
)

// CanonicalOperationID returns the operation id a name denotes on Windows,
// which compares names without regard to case, and whether it denotes one.
func CanonicalOperationID(name string) (string, bool) {
	if len(name) != operationIDLength || !areaOperation.MatchString(name) {
		return "", false
	}
	id := name[:8] + "T" + name[9:15] + "-" + strings.ToLower(name[16:])
	return id, ValidOperationID(id) == nil
}

// ClassifyRestoreRoot applies the initialization and grammar rules of ADR
// 0006 §11 to the entries of an existing restore destination. Operation
// names are recognised without regard to case; which of them a journal
// explains is the caller's to decide.
func ClassifyRestoreRoot(entries []NamedEntry) RestoreRoot {
	if len(entries) == 0 {
		return RestoreRoot{State: RestoreRootEmpty}
	}
	root := RestoreRoot{Published: map[string]RestoreObject{}, Incomplete: map[string]RestoreObject{}}
	marked, other := false, false
	for _, entry := range entries {
		switch {
		case entry.Name == RestoreAreaMarkerName:
			if entry.Kind != KindRegular {
				return RestoreRoot{State: RestoreRootForeign}
			}
			marked = true
		case areaMarkerTemporary.MatchString(entry.Name) && entry.Kind == KindRegular:
			root.Temporaries++
			if root.Taken == nil {
				root.Taken = map[int]bool{}
			}
			root.Taken[int(entry.Name[len(entry.Name)-1]-'0')] = true
		default:
			other = true
			root.classify(entry)
		}
	}
	if !marked {
		// Marker temporaries alone are an interrupted initialization;
		// anything else makes the directory foreign.
		if other {
			return RestoreRoot{State: RestoreRootForeign}
		}
		root.State = RestoreRootInterrupted
		return root
	}
	sort.Strings(root.Unexpected)
	root.State = RestoreRootMarked
	return root
}

func (root *RestoreRoot) classify(entry NamedEntry) {
	switch {
	case entry.Name == RestoreLockName && entry.Kind == KindRegular:
		root.Lock = true
		return
	case entry.Name == RestoreReceiptsDir && entry.Kind == KindDirectory:
		root.Receipts = true
		return
	case entry.Name == RestoreJournalDir && entry.Kind == KindDirectory:
		root.Journal = true
		return
	}
	if len(entry.Name) == operationIDLength+len(incompleteSuffix) && strings.EqualFold(entry.Name[operationIDLength:], incompleteSuffix) {
		if id, ok := CanonicalOperationID(entry.Name[:operationIDLength]); ok {
			root.add(root.Incomplete, id, entry)
			return
		}
	}
	if id, ok := CanonicalOperationID(entry.Name); ok {
		root.add(root.Published, id, entry)
		return
	}
	root.Unexpected = append(root.Unexpected, entry.Name)
}

// add records entry under id; two entries of one id, which a case-sensitive
// directory could hold, are both unexpected.
func (root *RestoreRoot) add(set map[string]RestoreObject, id string, entry NamedEntry) {
	if previous, taken := set[id]; taken {
		root.Unexpected = append(root.Unexpected, previous.Name, entry.Name)
		delete(set, id)
		return
	}
	set[id] = RestoreObject(entry)
}
