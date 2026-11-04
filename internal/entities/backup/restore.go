package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// RestoreStagingName is the copy of restore operation op until its
// publication, relative to the root of its restore area, as its
// staging-intent record names it (ADR 0006 §11).
func RestoreStagingName(op string) string { return op + incompleteSuffix }

// RestoreName is the published restore of operation op, relative to the
// root of its restore area.
func RestoreName(op string) string { return op }

// RestorePlan is the plan the planned record of a restore operation fixes
// before any copy (ADR 0006 §11): the snapshot it restores, as the receipt
// of that snapshot describes it, and the generation the snapshot copied.
type RestorePlan struct {
	ToolVersion string
	GameID      string
	GameName    string
	StartedAt   time.Time
	// Source is the generation of current the restored snapshot copied, as
	// its snapshot receipt names it.
	Source  SnapshotSource
	Sources []ReceiptSource
}

type restorePlanOut struct {
	Game        map[string]string `json:"game"`
	Source      map[string]string `json:"source"`
	Sources     []sourceOut       `json:"sources"`
	StartedAt   string            `json:"started_at"`
	ToolVersion string            `json:"tool_version"`
}

type restorePlanJSON struct {
	Game        *gameJSON           `json:"game"`
	Source      *snapshotSourceJSON `json:"source"`
	Sources     *[]*sourceJSON      `json:"sources"`
	StartedAt   *string             `json:"started_at"`
	ToolVersion *string             `json:"tool_version"`
}

// Encode returns the compact JSON form of p, the value of the "plan" field,
// and its SHA-256, the plan_sha256 of the operation and of its receipt.
func (p RestorePlan) Encode() (json.RawMessage, [32]byte, error) {
	if err := p.validate(); err != nil {
		return nil, [32]byte{}, err
	}
	data, err := json.Marshal(restorePlanOut{
		Game:        map[string]string{"id": p.GameID, "name": p.GameName},
		Source:      p.Source.encode(),
		Sources:     encodeSources(p.Sources),
		StartedAt:   formatUTC(p.StartedAt),
		ToolVersion: p.ToolVersion,
	})
	if err != nil {
		return nil, [32]byte{}, err
	}
	return data, sha256.Sum256(data), nil
}

// DecodeRestorePlan reads the plan of a restore planned record strictly.
func DecodeRestorePlan(data []byte) (RestorePlan, error) {
	var raw restorePlanJSON
	if err := decodeStrict(data, &raw); err != nil {
		return RestorePlan{}, fmt.Errorf("plan: %w", err)
	}
	var p RestorePlan
	var err error
	if p.ToolVersion, err = required("tool_version", raw.ToolVersion); err != nil {
		return RestorePlan{}, fmt.Errorf("plan: %w", err)
	}
	game, err := required("game", raw.Game)
	if err != nil {
		return RestorePlan{}, fmt.Errorf("plan: %w", err)
	}
	if p.GameID, err = required("game.id", game.ID); err != nil {
		return RestorePlan{}, fmt.Errorf("plan: %w", err)
	}
	if p.GameName, err = required("game.name", game.Name); err != nil {
		return RestorePlan{}, fmt.Errorf("plan: %w", err)
	}
	if p.StartedAt, err = requiredTime("started_at", raw.StartedAt); err != nil {
		return RestorePlan{}, fmt.Errorf("plan: %w", err)
	}
	source, err := required("source", raw.Source)
	if err != nil {
		return RestorePlan{}, fmt.Errorf("plan: %w", err)
	}
	if p.Source, err = source.decode("source"); err != nil {
		return RestorePlan{}, fmt.Errorf("plan: %w", err)
	}
	sources, err := required("sources", raw.Sources)
	if err != nil {
		return RestorePlan{}, fmt.Errorf("plan: %w", err)
	}
	for i, s := range sources {
		source, err := s.source(fmt.Sprintf("sources[%d]", i))
		if err != nil {
			return RestorePlan{}, fmt.Errorf("plan: %w", err)
		}
		p.Sources = append(p.Sources, source)
	}
	return p, p.validate()
}

func (p RestorePlan) validate() error {
	switch {
	case p.ToolVersion == "" || p.GameID == "" || p.GameName == "":
		return errors.New("plan: tool_version, game.id and game.name must not be empty")
	case len(p.Sources) == 0:
		return errors.New("plan: at least one source is required")
	}
	if err := ValidOperationID(p.Source.Operation); err != nil {
		return fmt.Errorf("plan: source.operation: %w", err)
	}
	if err := validateSources(p.Sources); err != nil {
		return fmt.Errorf("plan: %w", err)
	}
	return nil
}

// encode returns the JSON object of s: its operation and the SHA-256 of
// its receipt.
func (s SnapshotSource) encode() map[string]string {
	return map[string]string{"operation": s.Operation, "receipt_sha256": hex.EncodeToString(s.ReceiptSHA256[:])}
}

// decode reads an object of an operation and the SHA-256 of its receipt,
// field naming the object in errors.
func (raw snapshotSourceJSON) decode(field string) (SnapshotSource, error) {
	var s SnapshotSource
	var err error
	if s.Operation, err = required(field+".operation", raw.Operation); err != nil {
		return s, err
	}
	sum, err := required(field+".receipt_sha256", raw.ReceiptSHA256)
	if err != nil {
		return s, err
	}
	if s.ReceiptSHA256, err = parseSHA256(field+".receipt_sha256", sum); err != nil {
		return s, err
	}
	return s, nil
}

// restoreContract names the only fields of every step of a restore
// operation (ADR 0006 §11), all required. The rotation steps are absent,
// and unpublish-intent names the copy it moves back. The fields of the
// snapshot mode are absent too: a restore journal is never read as the
// journal of a snapshot, nor the reverse.
var restoreContract = map[Step]map[string]fieldKind{
	StepPlanned: {"snapshot": fieldRestoredSnapshot, "snapshot_receipt_sha256": fieldRestoredReceipt,
		"snapshot_identity": fieldSnapshotIdentity, "plan": fieldRestorePlan, "plan_sha256": fieldPlanSHA256},
	StepStagingIntent:   {"staging": fieldStagingName},
	StepStaged:          {"staging_identity": fieldStaging, "copied_sha256": fieldDigest},
	StepVerified:        {"verified_sha256": fieldDigest},
	StepPublishIntent:   {"staging_identity": fieldStaging},
	StepUnpublishIntent: {"staging_identity": fieldStaging},
	StepRollbackFailed:  {"error": fieldText},
	StepPublished:       {"receipt_sha256": fieldDigest},
	StepFailed:          {"reason": fieldText},
	StepAbandoned:       {"reason": fieldText},
	StepRolledBack:      {},
}

// NewRestoreRecord builds the record of one step of a restore operation
// from its values, which must follow the restore contract of the step.
func NewRestoreRecord(seq int, step Step, op string, at time.Time, prev string, v StepValues) (Record, error) {
	r, err := newRecord(restoreContract, RestoreStagingName(op), seq, step, op, at, prev, v)
	if err != nil {
		return Record{}, err
	}
	if _, err := r.RestoreValues(); err != nil {
		return Record{}, err
	}
	return r, nil
}

// RestoreValues decodes the fields of a record of a restore operation
// strictly against the restore contract of its step.
func (r Record) RestoreValues() (StepValues, error) {
	return r.values(restoreContract, RestoreStagingName(r.Op))
}

// ParseRestoreJournal validates the listing of journal\<op> of a restore
// area of game gameID and the content of its committed records, with the
// rules of ADR 0006 §3 and the automaton of a first publication (§11).
// Every deviation is an AmbiguityError.
func ParseRestoreJournal(gameID, op string, names []string, contents map[string][]byte) (Journal, error) {
	return parseJournal(op, names, contents, modeRestore, gameID)
}

// coherentRestore checks the values of the next record of a restore
// operation against the records before it.
func (j *Journal) coherentRestore(gameID string, r Record, v StepValues) error {
	switch step := r.Step; step {
	case StepPlanned:
		p := v.RestorePlan
		switch {
		case !p.StartedAt.Equal(r.At):
			return fmt.Errorf("plan started_at %s differs from the date of its record %s", formatUTC(p.StartedAt), formatUTC(r.At))
		case p.GameID != gameID:
			return fmt.Errorf("plan of game %q in a restore area of game %q", p.GameID, gameID)
		case p.Source.Operation == r.Op || p.Source.Operation == v.RestoredSnapshot.Operation:
			return errors.New("plan source.operation must differ from the operation and from the snapshot")
		}
		j.RestorePlan, j.PlanSHA256 = p, v.PlanSHA256
		j.RestoredSnapshot, j.SnapshotIdentity = v.RestoredSnapshot, v.SnapshotIdentity
	case StepStaged, StepVerified:
		want, err := ContentDigest(j.RestorePlan.Sources)
		if err != nil {
			return err
		}
		if v.Digest != want {
			return fmt.Errorf("%s digest does not match the content of the plan", step)
		}
		if step == StepStaged {
			j.Staging = v.Staging
		}
	case StepPublishIntent, StepUnpublishIntent:
		if v.Staging != j.Staging {
			return fmt.Errorf("staging identity %s differs from %s recorded by staged", v.Staging, j.Staging)
		}
	}
	return nil
}
