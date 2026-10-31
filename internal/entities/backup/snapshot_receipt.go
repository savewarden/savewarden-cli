package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Snapshot receipt schema identifiers (ADR 0008).
const (
	SnapshotReceiptSchema        = "savewarden.snapshot"
	SnapshotReceiptSchemaVersion = 1
)

// SnapshotReceipt describes one published snapshot (ADR 0008): the
// generation it copies and, as its receipt did, every file of it.
// Timestamps are UTC; a receipt reconstructed by recovery has no
// PublishedAt and names its RecoveredBy and RecoveredAt.
type SnapshotReceipt struct {
	Snapshot        string
	ToolVersion     string
	RecoveredBy     string // empty unless reconstructed by recovery
	GameID          string
	GameName        string
	StartedAt       time.Time
	PublishIntentAt time.Time
	PublishedAt     *time.Time // nil for a reconstructed receipt
	RecoveredAt     *time.Time // nil unless reconstructed
	Source          SnapshotSource
	PlanSHA256      [32]byte
	Sources         []ReceiptSource
}

type snapshotReceiptJSON struct {
	Schema          *string             `json:"schema"`
	SchemaVersion   *int                `json:"schema_version"`
	Snapshot        *string             `json:"snapshot"`
	ToolVersion     *string             `json:"tool_version"`
	RecoveredBy     nullable            `json:"recovered_by"`
	Game            *gameJSON           `json:"game"`
	StartedAt       *string             `json:"started_at"`
	PublishIntentAt *string             `json:"publish_intent_at"`
	PublishedAt     nullable            `json:"published_at"`
	RecoveredAt     nullable            `json:"recovered_at"`
	Source          *snapshotSourceJSON `json:"source"`
	PlanSHA256      *string             `json:"plan_sha256"`
	Sources         *[]*sourceJSON      `json:"sources"`
}

type snapshotSourceJSON struct {
	Operation     *string `json:"operation"`
	ReceiptSHA256 *string `json:"receipt_sha256"`
}

// Receipt builds the receipt of snapshot operation snapshot from its plan
// and the generation it copies. A receipt written at publication has
// publishedAt; one reconstructed by recovery has recoveredAt and
// recoveredBy instead.
func (p SnapshotPlan) Receipt(snapshot string, planSHA256 [32]byte, source SnapshotSource, publishIntentAt time.Time, publishedAt, recoveredAt *time.Time, recoveredBy string) (SnapshotReceipt, error) {
	r := SnapshotReceipt{
		Snapshot: snapshot, ToolVersion: p.ToolVersion, RecoveredBy: recoveredBy,
		GameID: p.GameID, GameName: p.GameName,
		StartedAt: p.StartedAt, PublishIntentAt: publishIntentAt,
		PublishedAt: publishedAt, RecoveredAt: recoveredAt,
		Source: source, PlanSHA256: planSHA256, Sources: p.Sources,
	}
	return r, r.validate()
}

// EncodeSnapshotReceipt validates r and returns its JSON form, ending with LF.
func EncodeSnapshotReceipt(r SnapshotReceipt) ([]byte, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}
	out := struct {
		Schema          string            `json:"schema"`
		SchemaVersion   int               `json:"schema_version"`
		Snapshot        string            `json:"snapshot"`
		ToolVersion     string            `json:"tool_version"`
		RecoveredBy     *string           `json:"recovered_by"`
		Game            map[string]any    `json:"game"`
		StartedAt       string            `json:"started_at"`
		PublishIntentAt string            `json:"publish_intent_at"`
		PublishedAt     *string           `json:"published_at"`
		RecoveredAt     *string           `json:"recovered_at"`
		Source          map[string]string `json:"source"`
		PlanSHA256      string            `json:"plan_sha256"`
		Sources         []sourceOut       `json:"sources"`
	}{
		Schema: SnapshotReceiptSchema, SchemaVersion: SnapshotReceiptSchemaVersion,
		Snapshot: r.Snapshot, ToolVersion: r.ToolVersion, RecoveredBy: stringOrNil(r.RecoveredBy),
		Game:      map[string]any{"id": r.GameID, "name": r.GameName},
		StartedAt: formatUTC(r.StartedAt), PublishIntentAt: formatUTC(r.PublishIntentAt),
		PublishedAt: timeOrNil(r.PublishedAt), RecoveredAt: timeOrNil(r.RecoveredAt),
		Source: map[string]string{"operation": r.Source.Operation,
			"receipt_sha256": hex.EncodeToString(r.Source.ReceiptSHA256[:])},
		PlanSHA256: hex.EncodeToString(r.PlanSHA256[:]),
		Sources:    encodeSources(r.Sources),
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// DecodeSnapshotReceipt reads a snapshot receipt strictly: unknown, missing
// or duplicate fields, trailing data and every rule of ADR 0008 are refused.
func DecodeSnapshotReceipt(data []byte) (SnapshotReceipt, error) {
	var raw snapshotReceiptJSON
	if err := decodeStrict(data, &raw); err != nil {
		return SnapshotReceipt{}, fmt.Errorf("snapshot receipt: %w", err)
	}
	r, err := raw.receipt()
	if err != nil {
		return SnapshotReceipt{}, fmt.Errorf("snapshot receipt: %w", err)
	}
	if err := r.validate(); err != nil {
		return SnapshotReceipt{}, err
	}
	return r, nil
}

func (raw snapshotReceiptJSON) receipt() (SnapshotReceipt, error) {
	var r SnapshotReceipt
	schema, err := required("schema", raw.Schema)
	if err != nil {
		return r, err
	}
	version, err := required("schema_version", raw.SchemaVersion)
	if err != nil {
		return r, err
	}
	if schema != SnapshotReceiptSchema || version != SnapshotReceiptSchemaVersion {
		return r, fmt.Errorf("unsupported schema %q version %d; this build reads %s version %d", schema, version, SnapshotReceiptSchema, SnapshotReceiptSchemaVersion)
	}
	if r.Snapshot, err = required("snapshot", raw.Snapshot); err != nil {
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
	source, err := required("source", raw.Source)
	if err != nil {
		return r, err
	}
	if r.Source.Operation, err = required("source.operation", source.Operation); err != nil {
		return r, err
	}
	receiptSum, err := required("source.receipt_sha256", source.ReceiptSHA256)
	if err != nil {
		return r, err
	}
	if r.Source.ReceiptSHA256, err = parseSHA256("source.receipt_sha256", receiptSum); err != nil {
		return r, err
	}
	planSum, err := required("plan_sha256", raw.PlanSHA256)
	if err != nil {
		return r, err
	}
	if r.PlanSHA256, err = parseSHA256("plan_sha256", planSum); err != nil {
		return r, err
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

// validate applies the semantic rules of ADR 0008.
func (r SnapshotReceipt) validate() error {
	fail := func(format string, args ...any) error { return fmt.Errorf("snapshot receipt: "+format, args...) }
	if err := ValidOperationID(r.Snapshot); err != nil {
		return fail("snapshot: %v", err)
	}
	if err := ValidOperationID(r.Source.Operation); err != nil {
		return fail("source.operation: %v", err)
	}
	if r.Source.Operation == r.Snapshot {
		return fail("source.operation: must differ from snapshot")
	}
	if r.ToolVersion == "" || r.GameID == "" || r.GameName == "" {
		return fail("tool_version, game.id and game.name must not be empty")
	}
	if err := checkPublicationTimes(r.StartedAt, r.PublishIntentAt, r.PublishedAt, r.RecoveredAt, r.RecoveredBy); err != nil {
		return fail("%v", err)
	}
	if len(r.Sources) == 0 {
		return fail("at least one source is required")
	}
	if err := validateSources(r.Sources); err != nil {
		return fail("%v", err)
	}
	return nil
}

// CheckSnapshotReceipt reads the receipt of a snapshot strictly and checks
// it against the journal of its operation (ADR 0008): every field it
// repeats from planned, the date of publish-intent and, once the operation
// is published, the SHA-256 of its bytes. An error means it is not the
// receipt this operation wrote.
func CheckSnapshotReceipt(data []byte, j Journal) (SnapshotReceipt, error) {
	if !j.Snapshot || j.SnapshotPlan == nil {
		return SnapshotReceipt{}, fmt.Errorf("operation %s has no snapshot plan", j.Op)
	}
	r, err := DecodeSnapshotReceipt(data)
	if err != nil {
		return SnapshotReceipt{}, err
	}
	p := j.SnapshotPlan
	intent, ok := j.intentAt(StepPublishIntent)
	switch {
	case !ok:
		return SnapshotReceipt{}, fmt.Errorf("operation %s never committed publish-intent", j.Op)
	case r.Snapshot != j.Op:
		return SnapshotReceipt{}, fmt.Errorf("snapshot receipt of %s in the operation %s", r.Snapshot, j.Op)
	case r.ToolVersion != p.ToolVersion || r.GameID != p.GameID || r.GameName != p.GameName || !r.StartedAt.Equal(p.StartedAt):
		return SnapshotReceipt{}, errors.New("snapshot receipt: tool, game or start differ from the plan")
	case !r.PublishIntentAt.Equal(intent):
		return SnapshotReceipt{}, errors.New("snapshot receipt: publish_intent_at differs from the publish-intent record")
	case r.Source != j.SnapshotSource:
		return SnapshotReceipt{}, errors.New("snapshot receipt: source differs from the plan")
	case r.PlanSHA256 != j.PlanSHA256:
		return SnapshotReceipt{}, errors.New("snapshot receipt: plan_sha256 differs from the plan")
	}
	if !sameContent(r.Sources, p.Sources) {
		return SnapshotReceipt{}, errors.New("snapshot receipt: sources differ from the plan")
	}
	if _, sum, published := j.Published(); published && sha256.Sum256(data) != sum {
		return SnapshotReceipt{}, errors.New("snapshot receipt: its bytes differ from the receipt_sha256 of published")
	}
	return r, nil
}

// CheckSourceReceipt is the pure part of the provenance gate of ADR 0006
// §10: the receipt file data of the generation snapshot operation j copies
// must have the SHA-256 planned recorded, be a valid receipt of that
// operation and of game gameID, and describe the content of the plan. It
// need not be the head receipt any more. An error is contradictory
// evidence; reading the file is the caller's.
func CheckSourceReceipt(data []byte, gameID string, j Journal) error {
	if !j.Snapshot || j.SnapshotPlan == nil {
		return fmt.Errorf("operation %s has no snapshot plan", j.Op)
	}
	if sha256.Sum256(data) != j.SnapshotSource.ReceiptSHA256 {
		return fmt.Errorf("the receipt of %s differs from the one planned recorded", j.SnapshotSource.Operation)
	}
	r, err := DecodeReceipt(data)
	switch {
	case err != nil:
		return err
	case r.Operation != j.SnapshotSource.Operation:
		return fmt.Errorf("the receipt of %s names the operation %s", j.SnapshotSource.Operation, r.Operation)
	case r.GameID != gameID || r.GameID != j.SnapshotPlan.GameID:
		return fmt.Errorf("the receipt of %s is of game %q", j.SnapshotSource.Operation, r.GameID)
	case !sameContent(r.Sources, j.SnapshotPlan.Sources):
		return fmt.Errorf("the receipt of %s describes another content than the plan", j.SnapshotSource.Operation)
	}
	return nil
}

// sameContent compares two lists of sources by their content digest.
func sameContent(a, b []ReceiptSource) bool {
	da, errA := ContentDigest(a)
	db, errB := ContentDigest(b)
	return errA == nil && errB == nil && da == db
}
