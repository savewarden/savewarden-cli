package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Restore receipt schema identifiers (ADR 0009).
const (
	RestoreReceiptSchema        = "savewarden.restore"
	RestoreReceiptSchemaVersion = 1
)

// RestoreReceipt describes one published restore (ADR 0009): the snapshot
// it restores, the generation that snapshot copied and, as the snapshot
// receipt did, every file of it. Timestamps are UTC; a receipt rebuilt by
// recovery has no PublishedAt and names its RecoveredBy and RecoveredAt.
type RestoreReceipt struct {
	Restore         string
	ToolVersion     string
	RecoveredBy     string // empty unless rebuilt by recovery
	GameID          string
	GameName        string
	StartedAt       time.Time
	PublishIntentAt time.Time
	PublishedAt     *time.Time // nil for a rebuilt receipt
	RecoveredAt     *time.Time // nil unless rebuilt
	Snapshot        SnapshotSource
	Source          SnapshotSource
	PlanSHA256      [32]byte
	Sources         []ReceiptSource
}

type restoreReceiptJSON struct {
	Schema          *string             `json:"schema"`
	SchemaVersion   *int                `json:"schema_version"`
	Restore         *string             `json:"restore"`
	ToolVersion     *string             `json:"tool_version"`
	RecoveredBy     nullable            `json:"recovered_by"`
	Game            *gameJSON           `json:"game"`
	StartedAt       *string             `json:"started_at"`
	PublishIntentAt *string             `json:"publish_intent_at"`
	PublishedAt     nullable            `json:"published_at"`
	RecoveredAt     nullable            `json:"recovered_at"`
	Snapshot        *restoredJSON       `json:"snapshot"`
	Source          *snapshotSourceJSON `json:"source"`
	PlanSHA256      *string             `json:"plan_sha256"`
	Sources         *[]*sourceJSON      `json:"sources"`
}

type restoredJSON struct {
	ID            *string `json:"id"`
	ReceiptSHA256 *string `json:"receipt_sha256"`
}

// Receipt builds the receipt of restore operation restore from its plan and
// the snapshot it restores. A receipt written at publication has
// publishedAt; one rebuilt by recovery has recoveredAt and recoveredBy
// instead.
func (p RestorePlan) Receipt(restore string, planSHA256 [32]byte, snapshot SnapshotSource, publishIntentAt time.Time, publishedAt, recoveredAt *time.Time, recoveredBy string) (RestoreReceipt, error) {
	r := RestoreReceipt{
		Restore: restore, ToolVersion: p.ToolVersion, RecoveredBy: recoveredBy,
		GameID: p.GameID, GameName: p.GameName,
		StartedAt: p.StartedAt, PublishIntentAt: publishIntentAt,
		PublishedAt: publishedAt, RecoveredAt: recoveredAt,
		Snapshot: snapshot, Source: p.Source, PlanSHA256: planSHA256, Sources: p.Sources,
	}
	return r, r.validate()
}

// EncodeRestoreReceipt validates r and returns its JSON form, ending with LF.
func EncodeRestoreReceipt(r RestoreReceipt) ([]byte, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}
	out := struct {
		Schema          string            `json:"schema"`
		SchemaVersion   int               `json:"schema_version"`
		Restore         string            `json:"restore"`
		ToolVersion     string            `json:"tool_version"`
		RecoveredBy     *string           `json:"recovered_by"`
		Game            map[string]any    `json:"game"`
		StartedAt       string            `json:"started_at"`
		PublishIntentAt string            `json:"publish_intent_at"`
		PublishedAt     *string           `json:"published_at"`
		RecoveredAt     *string           `json:"recovered_at"`
		Snapshot        map[string]string `json:"snapshot"`
		Source          map[string]string `json:"source"`
		PlanSHA256      string            `json:"plan_sha256"`
		Sources         []sourceOut       `json:"sources"`
	}{
		Schema: RestoreReceiptSchema, SchemaVersion: RestoreReceiptSchemaVersion,
		Restore: r.Restore, ToolVersion: r.ToolVersion, RecoveredBy: stringOrNil(r.RecoveredBy),
		Game:      map[string]any{"id": r.GameID, "name": r.GameName},
		StartedAt: formatUTC(r.StartedAt), PublishIntentAt: formatUTC(r.PublishIntentAt),
		PublishedAt: timeOrNil(r.PublishedAt), RecoveredAt: timeOrNil(r.RecoveredAt),
		Snapshot: map[string]string{"id": r.Snapshot.Operation,
			"receipt_sha256": hex.EncodeToString(r.Snapshot.ReceiptSHA256[:])},
		Source:     r.Source.encode(),
		PlanSHA256: hex.EncodeToString(r.PlanSHA256[:]),
		Sources:    encodeSources(r.Sources),
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// DecodeRestoreReceipt reads a restore receipt strictly: unknown, missing
// or duplicate fields, trailing data and every rule of ADR 0009 are refused.
func DecodeRestoreReceipt(data []byte) (RestoreReceipt, error) {
	var raw restoreReceiptJSON
	if err := decodeStrict(data, &raw); err != nil {
		return RestoreReceipt{}, fmt.Errorf("restore receipt: %w", err)
	}
	r, err := raw.receipt()
	if err != nil {
		return RestoreReceipt{}, fmt.Errorf("restore receipt: %w", err)
	}
	if err := r.validate(); err != nil {
		return RestoreReceipt{}, err
	}
	return r, nil
}

func (raw restoreReceiptJSON) receipt() (RestoreReceipt, error) {
	var r RestoreReceipt
	schema, err := required("schema", raw.Schema)
	if err != nil {
		return r, err
	}
	version, err := required("schema_version", raw.SchemaVersion)
	if err != nil {
		return r, err
	}
	if schema != RestoreReceiptSchema || version != RestoreReceiptSchemaVersion {
		return r, fmt.Errorf("unsupported schema %q version %d; this build reads %s version %d", schema, version, RestoreReceiptSchema, RestoreReceiptSchemaVersion)
	}
	if r.Restore, err = required("restore", raw.Restore); err != nil {
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
	snapshot, err := required("snapshot", raw.Snapshot)
	if err != nil {
		return r, err
	}
	restored := snapshotSourceJSON{Operation: snapshot.ID, ReceiptSHA256: snapshot.ReceiptSHA256}
	if r.Snapshot, err = restored.decode("snapshot"); err != nil {
		return r, err
	}
	source, err := required("source", raw.Source)
	if err != nil {
		return r, err
	}
	if r.Source, err = source.decode("source"); err != nil {
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

// validate applies the semantic rules of ADR 0009.
func (r RestoreReceipt) validate() error {
	fail := func(format string, args ...any) error { return fmt.Errorf("restore receipt: "+format, args...) }
	for _, id := range [][2]string{{"restore", r.Restore}, {"snapshot.id", r.Snapshot.Operation}, {"source.operation", r.Source.Operation}} {
		if err := ValidOperationID(id[1]); err != nil {
			return fail("%s: %v", id[0], err)
		}
	}
	if r.Restore == r.Snapshot.Operation || r.Restore == r.Source.Operation || r.Snapshot.Operation == r.Source.Operation {
		return fail("restore, snapshot.id and source.operation must differ")
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

// CheckRestoreReceipt reads the receipt of a restore strictly and checks it
// against the journal of its operation (ADR 0009): every field it repeats
// from planned, the date of publish-intent and, once the operation is
// published, the SHA-256 of its bytes. An error means it is not the receipt
// this operation wrote. The vault is never read to judge it.
func CheckRestoreReceipt(data []byte, j Journal) (RestoreReceipt, error) {
	if !j.Restore || j.RestorePlan == nil {
		return RestoreReceipt{}, fmt.Errorf("operation %s has no restore plan", j.Op)
	}
	r, err := DecodeRestoreReceipt(data)
	if err != nil {
		return RestoreReceipt{}, err
	}
	p := j.RestorePlan
	intent, ok := j.intentAt(StepPublishIntent)
	switch {
	case !ok:
		return RestoreReceipt{}, fmt.Errorf("operation %s never committed publish-intent", j.Op)
	case r.Restore != j.Op:
		return RestoreReceipt{}, fmt.Errorf("restore receipt of %s in the operation %s", r.Restore, j.Op)
	case r.ToolVersion != p.ToolVersion || r.GameID != p.GameID || r.GameName != p.GameName || !r.StartedAt.Equal(p.StartedAt):
		return RestoreReceipt{}, errors.New("restore receipt: tool, game or start differ from the plan")
	case !r.PublishIntentAt.Equal(intent):
		return RestoreReceipt{}, errors.New("restore receipt: publish_intent_at differs from the publish-intent record")
	case r.Snapshot != j.RestoredSnapshot:
		return RestoreReceipt{}, errors.New("restore receipt: snapshot differs from the plan")
	case r.Source != p.Source:
		return RestoreReceipt{}, errors.New("restore receipt: source differs from the plan")
	case r.PlanSHA256 != j.PlanSHA256:
		return RestoreReceipt{}, errors.New("restore receipt: plan_sha256 differs from the plan")
	}
	if !sameContent(r.Sources, p.Sources) {
		return RestoreReceipt{}, errors.New("restore receipt: sources differ from the plan")
	}
	if _, sum, published := j.Published(); published && sha256.Sum256(data) != sum {
		return RestoreReceipt{}, errors.New("restore receipt: its bytes differ from the receipt_sha256 of published")
	}
	return r, nil
}
