package backup

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"time"
)

// Vault marker identifiers (ADR 0007).
const (
	MarkerName          = "savewarden-vault.json"
	MarkerSchema        = "savewarden.vault"
	MarkerSchemaVersion = 1
	// MaxTemporaries bounds the temporary files of one committed file:
	// record, receipt or marker (envelope E1 of ADR 0006).
	MaxTemporaries = 9
)

// Marker is the content of savewarden-vault.json.
type Marker struct {
	CreatedAt   time.Time
	ToolVersion string
}

// EncodeMarker returns the JSON form of m, ending with LF.
func EncodeMarker(m Marker) ([]byte, error) {
	if m.ToolVersion == "" {
		return nil, fmt.Errorf("vault marker: tool_version must not be empty")
	}
	data, err := json.Marshal(struct {
		Schema        string `json:"schema"`
		SchemaVersion int    `json:"schema_version"`
		CreatedAt     string `json:"created_at"`
		ToolVersion   string `json:"tool_version"`
	}{MarkerSchema, MarkerSchemaVersion, formatUTC(m.CreatedAt), m.ToolVersion})
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// DecodeMarker reads a vault marker strictly.
func DecodeMarker(data []byte) (Marker, error) {
	var raw struct {
		Schema        *string `json:"schema"`
		SchemaVersion *int    `json:"schema_version"`
		CreatedAt     *string `json:"created_at"`
		ToolVersion   *string `json:"tool_version"`
	}
	if err := decodeStrict(data, &raw); err != nil {
		return Marker{}, fmt.Errorf("vault marker: %w", err)
	}
	schema, err := required("schema", raw.Schema)
	if err != nil {
		return Marker{}, fmt.Errorf("vault marker: %w", err)
	}
	version, err := required("schema_version", raw.SchemaVersion)
	if err != nil {
		return Marker{}, fmt.Errorf("vault marker: %w", err)
	}
	if schema != MarkerSchema || version != MarkerSchemaVersion {
		return Marker{}, fmt.Errorf("vault marker: unsupported schema %q version %d; this build reads %s version %d", schema, version, MarkerSchema, MarkerSchemaVersion)
	}
	var m Marker
	if m.CreatedAt, err = requiredTime("created_at", raw.CreatedAt); err != nil {
		return Marker{}, fmt.Errorf("vault marker: %w", err)
	}
	if m.ToolVersion, err = required("tool_version", raw.ToolVersion); err != nil || m.ToolVersion == "" {
		return Marker{}, fmt.Errorf("vault marker: tool_version must be a non-empty string")
	}
	return m, nil
}

// EntryKind is the type of a directory entry, observed without following it.
type EntryKind int

// Entry kinds. Links, junctions, reparse points and placeholders are Other.
const (
	KindRegular EntryKind = iota
	KindDirectory
	KindOther
)

// NamedEntry is one entry of a directory listing.
type NamedEntry struct {
	Name string
	Kind EntryKind
}

// RootState classifies the content of a vault directory (ADR 0006 §4).
type RootState int

// Vault root states.
const (
	RootEmpty       RootState = iota // accepted: initialization starts at I3
	RootInterrupted                  // only marker temporaries: resumed by --recover --apply only
	RootMarked                       // a marker file is present; its content is checked separately
	RootForeign                      // not a Savewarden vault: refused, never adopted
)

// VaultRoot is the classification of a vault directory listing.
type VaultRoot struct {
	State RootState
	// Temporaries counts the marker temporaries; at MaxTemporaries the
	// recovery envelope is exhausted. Taken holds their attempt numbers.
	Temporaries int
	Taken       map[int]bool
	// Unexpected lists, for a marked vault, the entries that have no place
	// at its root, sorted.
	Unexpected []string
}

var markerTemporary = regexp.MustCompile(`^` + regexp.QuoteMeta(MarkerName) + `\.tmp-([1-9])$`)

// ClassifyVaultRoot applies the initialization table of ADR 0006 to the
// entries of an existing vault directory.
func ClassifyVaultRoot(entries []NamedEntry) VaultRoot {
	if len(entries) == 0 {
		return VaultRoot{State: RootEmpty}
	}
	var root VaultRoot
	marked, games := false, false
	for _, entry := range entries {
		switch {
		case entry.Name == MarkerName:
			if entry.Kind != KindRegular {
				return VaultRoot{State: RootForeign}
			}
			marked = true
		case markerTemporary.MatchString(entry.Name) && entry.Kind == KindRegular:
			root.Temporaries++
			if root.Taken == nil {
				root.Taken = map[int]bool{}
			}
			root.Taken[int(entry.Name[len(entry.Name)-1]-'0')] = true
		case entry.Name == "games" && entry.Kind == KindDirectory:
			games = true
		default:
			root.Unexpected = append(root.Unexpected, entry.Name)
		}
	}
	if !marked {
		// Marker temporaries alone are an interrupted initialization;
		// anything else makes the directory foreign.
		if games || len(root.Unexpected) > 0 {
			return VaultRoot{State: RootForeign}
		}
		root.State = RootInterrupted
		return root
	}
	sort.Strings(root.Unexpected)
	root.State = RootMarked
	return root
}
