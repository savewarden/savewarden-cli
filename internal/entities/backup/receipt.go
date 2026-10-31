package backup

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Receipt schema identifiers (ADR 0007).
const (
	ReceiptSchema        = "savewarden.receipt"
	ReceiptSchemaVersion = 1
)

// SourceState says whether a source was copied into a generation.
type SourceState string

// Source states of a receipt.
const (
	SourceCopied  SourceState = "copied"
	SourceMissing SourceState = "missing"
)

// Receipt describes one published generation. Timestamps are UTC. A
// receipt reconstructed by recovery has no PublishedAt and names its
// RecoveredBy and RecoveredAt; see ADR 0007 for the provenance of fields.
type Receipt struct {
	Operation       string
	ToolVersion     string
	RecoveredBy     string // empty unless reconstructed by recovery
	GameID          string
	GameName        string
	StartedAt       time.Time
	PublishIntentAt time.Time
	PublishedAt     *time.Time // nil for a reconstructed receipt
	RecoveredAt     *time.Time // nil unless reconstructed
	Previous        string     // operation of the replaced generation, empty for the first
	PlanSHA256      [32]byte
	Changes         ChangeCounts
	Sources         []ReceiptSource
}

// ChangeCounts summarises a generation against the previous one.
type ChangeCounts struct {
	Added, Modified, Removed, Unchanged int
}

// ReceiptSource is one source of a generation.
type ReceiptSource struct {
	ID          string
	Kind        string
	Required    bool
	State       SourceState
	Accepted    bool   // a missing source explicitly accepted by the user
	Path        string // Windows path of the source when copied
	Directories []string
	Entries     []Entry
}

// Bytes sums the sizes of the source's entries.
func (s ReceiptSource) Bytes() int64 {
	var total int64
	for _, entry := range s.Entries {
		total += entry.Size
	}
	return total
}

// Source returns the source with the given id.
func (r Receipt) Source(id string) (ReceiptSource, bool) {
	for _, source := range r.Sources {
		if source.ID == id {
			return source, true
		}
	}
	return ReceiptSource{}, false
}

// Recovered reports whether recovery reconstructed the receipt.
func (r Receipt) Recovered() bool { return r.RecoveredAt != nil }

type receiptJSON struct {
	Schema          *string        `json:"schema"`
	SchemaVersion   *int           `json:"schema_version"`
	Operation       *string        `json:"operation"`
	ToolVersion     *string        `json:"tool_version"`
	RecoveredBy     nullable       `json:"recovered_by"`
	Game            *gameJSON      `json:"game"`
	StartedAt       *string        `json:"started_at"`
	PublishIntentAt *string        `json:"publish_intent_at"`
	PublishedAt     nullable       `json:"published_at"`
	RecoveredAt     nullable       `json:"recovered_at"`
	Previous        nullable       `json:"previous"`
	PlanSHA256      *string        `json:"plan_sha256"`
	Changes         *changesJSON   `json:"changes"`
	Sources         *[]*sourceJSON `json:"sources"`
}

type gameJSON struct {
	ID   *string `json:"id"`
	Name *string `json:"name"`
}

type changesJSON struct {
	Added     *int `json:"added"`
	Modified  *int `json:"modified"`
	Removed   *int `json:"removed"`
	Unchanged *int `json:"unchanged"`
}

type sourceJSON struct {
	ID          *string       `json:"id"`
	Kind        *string       `json:"kind"`
	Required    *bool         `json:"required"`
	Status      *string       `json:"status"`
	Accepted    *bool         `json:"accepted"`
	Path        *string       `json:"path"`
	Files       *int          `json:"files"`
	Bytes       *int64        `json:"bytes"`
	Directories *[]string     `json:"directories"`
	Entries     *[]*entryJSON `json:"entries"`
}

type entryJSON struct {
	Path    *string `json:"path"`
	Size    *int64  `json:"size"`
	ModTime *string `json:"mod_time"`
	SHA256  *string `json:"sha256"`
}

// EncodeReceipt validates r and returns its JSON form, ending with LF.
func EncodeReceipt(r Receipt) ([]byte, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}
	out := struct {
		Schema          string         `json:"schema"`
		SchemaVersion   int            `json:"schema_version"`
		Operation       string         `json:"operation"`
		ToolVersion     string         `json:"tool_version"`
		RecoveredBy     *string        `json:"recovered_by"`
		Game            map[string]any `json:"game"`
		StartedAt       string         `json:"started_at"`
		PublishIntentAt string         `json:"publish_intent_at"`
		PublishedAt     *string        `json:"published_at"`
		RecoveredAt     *string        `json:"recovered_at"`
		Previous        *string        `json:"previous"`
		PlanSHA256      string         `json:"plan_sha256"`
		Changes         map[string]int `json:"changes"`
		Sources         []sourceOut    `json:"sources"`
	}{
		Schema: ReceiptSchema, SchemaVersion: ReceiptSchemaVersion,
		Operation: r.Operation, ToolVersion: r.ToolVersion, RecoveredBy: stringOrNil(r.RecoveredBy),
		Game:      map[string]any{"id": r.GameID, "name": r.GameName},
		StartedAt: formatUTC(r.StartedAt), PublishIntentAt: formatUTC(r.PublishIntentAt),
		PublishedAt: timeOrNil(r.PublishedAt), RecoveredAt: timeOrNil(r.RecoveredAt),
		Previous: stringOrNil(r.Previous), PlanSHA256: hex.EncodeToString(r.PlanSHA256[:]),
		Changes: map[string]int{"added": r.Changes.Added, "modified": r.Changes.Modified,
			"removed": r.Changes.Removed, "unchanged": r.Changes.Unchanged},
		Sources: encodeSources(r.Sources),
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

type entryOut struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
	SHA256  string `json:"sha256"`
}

type sourceOut struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	Required    bool       `json:"required"`
	Status      string     `json:"status"`
	Accepted    bool       `json:"accepted"`
	Path        string     `json:"path"`
	Files       int        `json:"files"`
	Bytes       int64      `json:"bytes"`
	Directories []string   `json:"directories"`
	Entries     []entryOut `json:"entries"`
}

// encodeSources gives the JSON form of sources shared by receipts and the
// planned record.
func encodeSources(in []ReceiptSource) []sourceOut {
	sources := make([]sourceOut, 0, len(in))
	for _, s := range in {
		entries := make([]entryOut, 0, len(s.Entries))
		for _, e := range s.Entries {
			entries = append(entries, entryOut{e.Path, e.Size, formatUTC(e.ModTime), hex.EncodeToString(e.SHA256[:])})
		}
		directories := append([]string{}, s.Directories...)
		sources = append(sources, sourceOut{s.ID, s.Kind, s.Required, string(s.State), s.Accepted, s.Path,
			len(s.Entries), s.Bytes(), directories, entries})
	}
	return sources
}

func stringOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func timeOrNil(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := formatUTC(*t)
	return &s
}

// DecodeReceipt reads a receipt strictly: unknown, missing or duplicate
// fields, trailing data and every rule of ADR 0007 are refused.
func DecodeReceipt(data []byte) (Receipt, error) {
	var raw receiptJSON
	if err := decodeStrict(data, &raw); err != nil {
		return Receipt{}, fmt.Errorf("receipt: %w", err)
	}
	r, err := raw.receipt()
	if err != nil {
		return Receipt{}, fmt.Errorf("receipt: %w", err)
	}
	if err := r.validate(); err != nil {
		return Receipt{}, err
	}
	return r, nil
}

func (raw receiptJSON) receipt() (Receipt, error) {
	var r Receipt
	var err error
	schema, err := required("schema", raw.Schema)
	if err != nil {
		return r, err
	}
	version, err := required("schema_version", raw.SchemaVersion)
	if err != nil {
		return r, err
	}
	if schema != ReceiptSchema || version != ReceiptSchemaVersion {
		return r, fmt.Errorf("unsupported schema %q version %d; this build reads %s version %d", schema, version, ReceiptSchema, ReceiptSchemaVersion)
	}
	if r.Operation, err = required("operation", raw.Operation); err != nil {
		return r, err
	}
	if r.ToolVersion, err = required("tool_version", raw.ToolVersion); err != nil {
		return r, err
	}
	if r.RecoveredBy, _, err = raw.RecoveredBy.optionalString("recovered_by"); err != nil {
		return r, err
	}
	game, err := required("game", raw.Game)
	if err != nil {
		return r, err
	}
	if r.GameID, err = required("game.id", game.ID); err != nil {
		return r, err
	}
	if r.GameName, err = required("game.name", game.Name); err != nil {
		return r, err
	}
	if r.StartedAt, err = requiredTime("started_at", raw.StartedAt); err != nil {
		return r, err
	}
	if r.PublishIntentAt, err = requiredTime("publish_intent_at", raw.PublishIntentAt); err != nil {
		return r, err
	}
	if r.PublishedAt, err = optionalTime("published_at", raw.PublishedAt); err != nil {
		return r, err
	}
	if r.RecoveredAt, err = optionalTime("recovered_at", raw.RecoveredAt); err != nil {
		return r, err
	}
	if r.Previous, _, err = raw.Previous.optionalString("previous"); err != nil {
		return r, err
	}
	sum, err := required("plan_sha256", raw.PlanSHA256)
	if err != nil {
		return r, err
	}
	if r.PlanSHA256, err = parseSHA256("plan_sha256", sum); err != nil {
		return r, err
	}
	changes, err := required("changes", raw.Changes)
	if err != nil {
		return r, err
	}
	for _, c := range []struct {
		field string
		in    *int
		out   *int
	}{
		{"changes.added", changes.Added, &r.Changes.Added},
		{"changes.modified", changes.Modified, &r.Changes.Modified},
		{"changes.removed", changes.Removed, &r.Changes.Removed},
		{"changes.unchanged", changes.Unchanged, &r.Changes.Unchanged},
	} {
		if *c.out, err = required(c.field, c.in); err != nil {
			return r, err
		}
	}
	sources, err := required("sources", raw.Sources)
	if err != nil {
		return r, err
	}
	for i, s := range sources {
		source, err := s.source(fmt.Sprintf("sources[%d]", i))
		if err != nil {
			return r, err
		}
		r.Sources = append(r.Sources, source)
	}
	return r, nil
}

func (s *sourceJSON) source(field string) (ReceiptSource, error) {
	var out ReceiptSource
	if s == nil {
		return out, fmt.Errorf("%s: must be an object", field)
	}
	var err error
	if out.ID, err = required(field+".id", s.ID); err != nil {
		return out, err
	}
	if out.Kind, err = required(field+".kind", s.Kind); err != nil {
		return out, err
	}
	if out.Required, err = required(field+".required", s.Required); err != nil {
		return out, err
	}
	status, err := required(field+".status", s.Status)
	if err != nil {
		return out, err
	}
	out.State = SourceState(status)
	if out.Accepted, err = required(field+".accepted", s.Accepted); err != nil {
		return out, err
	}
	if out.Path, err = required(field+".path", s.Path); err != nil {
		return out, err
	}
	files, err := required(field+".files", s.Files)
	if err != nil {
		return out, err
	}
	bytes, err := required(field+".bytes", s.Bytes)
	if err != nil {
		return out, err
	}
	if out.Directories, err = required(field+".directories", s.Directories); err != nil {
		return out, err
	}
	entries, err := required(field+".entries", s.Entries)
	if err != nil {
		return out, err
	}
	for j, e := range entries {
		entryField := fmt.Sprintf("%s.entries[%d]", field, j)
		if e == nil {
			return out, fmt.Errorf("%s: must be an object", entryField)
		}
		var entry Entry
		if entry.Path, err = required(entryField+".path", e.Path); err != nil {
			return out, err
		}
		if entry.Size, err = required(entryField+".size", e.Size); err != nil {
			return out, err
		}
		if entry.ModTime, err = requiredTime(entryField+".mod_time", e.ModTime); err != nil {
			return out, err
		}
		sum, err := required(entryField+".sha256", e.SHA256)
		if err != nil {
			return out, err
		}
		if entry.SHA256, err = parseSHA256(entryField+".sha256", sum); err != nil {
			return out, err
		}
		out.Entries = append(out.Entries, entry)
	}
	if files != len(out.Entries) {
		return out, fmt.Errorf("%s.files: %d does not match %d entries", field, files, len(out.Entries))
	}
	if bytes != out.Bytes() {
		return out, fmt.Errorf("%s.bytes: %d does not match the sum of entry sizes %d", field, bytes, out.Bytes())
	}
	return out, nil
}

func requiredTime(field string, value *string) (time.Time, error) {
	s, err := required(field, value)
	if err != nil {
		return time.Time{}, err
	}
	return parseUTC(field, s)
}

func optionalTime(field string, value nullable) (*time.Time, error) {
	s, ok, err := value.optionalString(field)
	if err != nil || !ok {
		return nil, err
	}
	t, err := parseUTC(field, s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// validate applies the semantic rules of ADR 0007.
func (r Receipt) validate() error {
	fail := func(format string, args ...any) error { return fmt.Errorf("receipt: "+format, args...) }
	if err := ValidOperationID(r.Operation); err != nil {
		return fail("operation: %v", err)
	}
	if r.Previous != "" {
		if err := ValidOperationID(r.Previous); err != nil {
			return fail("previous: %v", err)
		}
		if r.Previous == r.Operation {
			return fail("previous: must differ from operation")
		}
	}
	if r.ToolVersion == "" || r.GameID == "" || r.GameName == "" {
		return fail("tool_version, game.id and game.name must not be empty")
	}
	if err := checkPublicationTimes(r.StartedAt, r.PublishIntentAt, r.PublishedAt, r.RecoveredAt, r.RecoveredBy); err != nil {
		return fail("%v", err)
	}
	if r.Changes.Added < 0 || r.Changes.Modified < 0 || r.Changes.Removed < 0 || r.Changes.Unchanged < 0 {
		return fail("changes: counts must not be negative")
	}
	if err := validateSources(r.Sources); err != nil {
		return fail("%v", err)
	}
	return nil
}

// checkPublicationTimes applies the rules of dates and provenance shared by
// every receipt (ADR 0007): a receipt written at publication knows
// published_at; one reconstructed by recovery names recovered_at and
// recovered_by instead.
func checkPublicationTimes(startedAt, publishIntentAt time.Time, publishedAt, recoveredAt *time.Time, recoveredBy string) error {
	if publishIntentAt.Before(startedAt) {
		return errors.New("publish_intent_at precedes started_at")
	}
	switch {
	case recoveredAt == nil && recoveredBy == "":
		if publishedAt == nil {
			return errors.New("published_at: required unless the receipt was reconstructed by recovery")
		}
		if publishedAt.Before(publishIntentAt) {
			return errors.New("published_at precedes publish_intent_at")
		}
	case recoveredAt != nil && recoveredBy != "":
		if publishedAt != nil {
			return errors.New("published_at: must be null in a reconstructed receipt, the moment of the rename is unknown")
		}
		if recoveredAt.Before(publishIntentAt) {
			return errors.New("recovered_at precedes publish_intent_at")
		}
	default:
		return errors.New("recovered_at and recovered_by must be both null or both set")
	}
	return nil
}

// validateSources applies the rules shared by receipts and plans: unique
// ids, known states, sorted unique relative paths.
func validateSources(sources []ReceiptSource) error {
	fail := fmt.Errorf
	ids := map[string]bool{}
	for i, s := range sources {
		field := fmt.Sprintf("sources[%d]", i)
		if s.ID == "" || ids[s.ID] {
			return fail("%s.id: must be unique and not empty", field)
		}
		ids[s.ID] = true
		if s.Kind == "" {
			return fail("%s.kind: must not be empty", field)
		}
		switch s.State {
		case SourceCopied:
			if s.Accepted {
				return fail("%s.accepted: only a missing source can be accepted", field)
			}
		case SourceMissing:
			if len(s.Entries) != 0 || len(s.Directories) != 0 {
				return fail("%s: a missing source has no entries and no directories", field)
			}
		default:
			return fail("%s.status: %q must be copied or missing", field, s.State)
		}
		if err := sortedUnique(s.Directories); err != nil {
			return fail("%s.directories: %v", field, err)
		}
		paths := make([]string, len(s.Entries))
		for j, e := range s.Entries {
			if e.Size < 0 {
				return fail("%s.entries[%d].size: must not be negative", field, j)
			}
			paths[j] = e.Path
		}
		if err := sortedUnique(paths); err != nil {
			return fail("%s.entries: %v", field, err)
		}
	}
	return nil
}

// sortedUnique accepts valid relative paths, sorted in byte order, without
// duplicates.
func sortedUnique(paths []string) error {
	for i, path := range paths {
		if !validRelative(path) {
			return fmt.Errorf("%q is not a relative slash-separated path", path)
		}
		if i > 0 && paths[i-1] >= path {
			return errors.New("paths must be sorted in byte order without duplicates")
		}
	}
	return nil
}
