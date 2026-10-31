package backup

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// SnapshotStagingName is the copy of snapshot operation op until its
// publication, relative to the game directory, as its staging-intent
// record names it (ADR 0006 §10).
func SnapshotStagingName(op string) string { return "snapshots/" + op + ".incomplete" }

// SnapshotName is the published snapshot of operation op, relative to the
// game directory.
func SnapshotName(op string) string { return "snapshots/" + op }

// SnapshotSource is the generation a snapshot copies: the operation that
// published it, and the SHA-256 of that operation's receipt file.
type SnapshotSource struct {
	Operation     string
	ReceiptSHA256 [32]byte
}

// SnapshotPlan is the plan the planned record of a snapshot operation fixes
// before any copy (ADR 0006 §10): the generation of current it copies, as
// the receipt of that generation describes it. It has no previous and no
// changes: a snapshot replaces nothing.
type SnapshotPlan struct {
	ToolVersion string
	GameID      string
	GameName    string
	StartedAt   time.Time
	Sources     []ReceiptSource
}

type snapshotPlanOut struct {
	Game        map[string]string `json:"game"`
	Sources     []sourceOut       `json:"sources"`
	StartedAt   string            `json:"started_at"`
	ToolVersion string            `json:"tool_version"`
}

type snapshotPlanJSON struct {
	Game        *gameJSON      `json:"game"`
	Sources     *[]*sourceJSON `json:"sources"`
	StartedAt   *string        `json:"started_at"`
	ToolVersion *string        `json:"tool_version"`
}

// Encode returns the compact JSON form of p, the value of the "plan" field,
// and its SHA-256, the plan_sha256 of the operation and of its receipt.
func (p SnapshotPlan) Encode() (json.RawMessage, [32]byte, error) {
	if err := p.validate(); err != nil {
		return nil, [32]byte{}, err
	}
	data, err := json.Marshal(snapshotPlanOut{
		Game:      map[string]string{"id": p.GameID, "name": p.GameName},
		Sources:   encodeSources(p.Sources),
		StartedAt: formatUTC(p.StartedAt), ToolVersion: p.ToolVersion,
	})
	if err != nil {
		return nil, [32]byte{}, err
	}
	return data, sha256.Sum256(data), nil
}

// DecodeSnapshotPlan reads the plan of a snapshot planned record strictly.
func DecodeSnapshotPlan(data []byte) (SnapshotPlan, error) {
	var raw snapshotPlanJSON
	if err := decodeStrict(data, &raw); err != nil {
		return SnapshotPlan{}, fmt.Errorf("plan: %w", err)
	}
	var p SnapshotPlan
	var err error
	if p.ToolVersion, err = required("tool_version", raw.ToolVersion); err != nil {
		return SnapshotPlan{}, fmt.Errorf("plan: %w", err)
	}
	game, err := required("game", raw.Game)
	if err != nil {
		return SnapshotPlan{}, fmt.Errorf("plan: %w", err)
	}
	if p.GameID, err = required("game.id", game.ID); err != nil {
		return SnapshotPlan{}, fmt.Errorf("plan: %w", err)
	}
	if p.GameName, err = required("game.name", game.Name); err != nil {
		return SnapshotPlan{}, fmt.Errorf("plan: %w", err)
	}
	if p.StartedAt, err = requiredTime("started_at", raw.StartedAt); err != nil {
		return SnapshotPlan{}, fmt.Errorf("plan: %w", err)
	}
	sources, err := required("sources", raw.Sources)
	if err != nil {
		return SnapshotPlan{}, fmt.Errorf("plan: %w", err)
	}
	for i, s := range sources {
		source, err := s.source(fmt.Sprintf("sources[%d]", i))
		if err != nil {
			return SnapshotPlan{}, fmt.Errorf("plan: %w", err)
		}
		p.Sources = append(p.Sources, source)
	}
	return p, p.validate()
}

func (p SnapshotPlan) validate() error {
	switch {
	case p.ToolVersion == "" || p.GameID == "" || p.GameName == "":
		return errors.New("plan: tool_version, game.id and game.name must not be empty")
	case len(p.Sources) == 0:
		return errors.New("plan: at least one source is required")
	}
	if err := validateSources(p.Sources); err != nil {
		return fmt.Errorf("plan: %w", err)
	}
	return nil
}

// snapshotContract names the only fields of every step of a snapshot
// operation (ADR 0006 §10), all required. The rotation steps are absent,
// and unpublish-intent names the staging it moves back, never a current.
var snapshotContract = map[Step]map[string]fieldKind{
	StepPlanned: {"source_operation": fieldSourceOperation, "source_receipt_sha256": fieldSourceReceipt,
		"source_identity": fieldSourceIdentity, "plan": fieldSnapshotPlan, "plan_sha256": fieldPlanSHA256},
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

// NewSnapshotRecord builds the record of one step of a snapshot operation
// from its values, which must follow the snapshot contract of the step.
func NewSnapshotRecord(seq int, step Step, op string, at time.Time, prev string, v StepValues) (Record, error) {
	r, err := newRecord(snapshotContract, SnapshotStagingName(op), seq, step, op, at, prev, v)
	if err != nil {
		return Record{}, err
	}
	if _, err := r.SnapshotValues(); err != nil {
		return Record{}, err
	}
	return r, nil
}

// SnapshotValues decodes the fields of a record of a snapshot operation
// strictly against the snapshot contract of its step.
func (r Record) SnapshotValues() (StepValues, error) {
	return r.values(snapshotContract, SnapshotStagingName(r.Op))
}

// ParseSnapshotJournal validates the listing of snapshots\journal\<op> of
// game gameID and the content of its committed records, with the rules of
// ADR 0006 §3 and the automaton of a first publication (§10). Every
// deviation is an AmbiguityError.
func ParseSnapshotJournal(gameID, op string, names []string, contents map[string][]byte) (Journal, error) {
	return parseJournal(op, names, contents, true, gameID)
}

// coherentSnapshot checks the values of the next record of a snapshot
// operation against the records before it.
func (j *Journal) coherentSnapshot(gameID string, r Record, v StepValues) error {
	switch step := r.Step; step {
	case StepPlanned:
		if !v.SnapshotPlan.StartedAt.Equal(r.At) {
			return fmt.Errorf("plan started_at %s differs from the date of its record %s", formatUTC(v.SnapshotPlan.StartedAt), formatUTC(r.At))
		}
		if v.SnapshotPlan.GameID != gameID {
			return fmt.Errorf("plan of game %q in the directory of game %q", v.SnapshotPlan.GameID, gameID)
		}
		j.SnapshotPlan, j.PlanSHA256 = v.SnapshotPlan, v.PlanSHA256
		j.SnapshotSource, j.SourceIdentity = v.SnapshotSource, v.SourceIdentity
	case StepStaged, StepVerified:
		want, err := ContentDigest(j.SnapshotPlan.Sources)
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

// intentAt returns the date of the first committed record of step.
func (j Journal) intentAt(step Step) (time.Time, bool) {
	for _, r := range j.Records {
		if r.Step == step {
			return r.At, true
		}
	}
	return time.Time{}, false
}
