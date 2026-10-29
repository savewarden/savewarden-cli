package backup

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Planned is the plan that the planned record of an operation fixes before
// any copy (ADR 0006 §4): the generation to publish, as its receipt will
// describe it. Recovery re-verifies current against it.
type Planned struct {
	ToolVersion string
	GameID      string
	GameName    string
	StartedAt   time.Time
	// Previous is the operation whose generation a rotation replaces,
	// empty for a first publication.
	Previous string
	Changes  ChangeCounts
	Sources  []ReceiptSource
}

type plannedOut struct {
	Changes     map[string]int    `json:"changes"`
	Game        map[string]string `json:"game"`
	Previous    *string           `json:"previous"`
	Sources     []sourceOut       `json:"sources"`
	StartedAt   string            `json:"started_at"`
	ToolVersion string            `json:"tool_version"`
}

type plannedJSON struct {
	Changes     *changesJSON   `json:"changes"`
	Game        *gameJSON      `json:"game"`
	Previous    nullable       `json:"previous"`
	Sources     *[]*sourceJSON `json:"sources"`
	StartedAt   *string        `json:"started_at"`
	ToolVersion *string        `json:"tool_version"`
}

// Encode returns the compact JSON form of p, the value of the "plan" field,
// and its SHA-256, the plan_sha256 of the operation and of its receipt.
func (p Planned) Encode() (json.RawMessage, [32]byte, error) {
	if err := p.validate(); err != nil {
		return nil, [32]byte{}, err
	}
	data, err := json.Marshal(plannedOut{
		Changes: map[string]int{"added": p.Changes.Added, "modified": p.Changes.Modified,
			"removed": p.Changes.Removed, "unchanged": p.Changes.Unchanged},
		Game:      map[string]string{"id": p.GameID, "name": p.GameName},
		Previous:  stringOrNil(p.Previous),
		Sources:   encodeSources(p.Sources),
		StartedAt: formatUTC(p.StartedAt), ToolVersion: p.ToolVersion,
	})
	if err != nil {
		return nil, [32]byte{}, err
	}
	return data, sha256.Sum256(data), nil
}

// DecodePlanned reads the plan of a planned record strictly, as a receipt.
func DecodePlanned(data []byte) (Planned, error) {
	var raw plannedJSON
	if err := decodeStrict(data, &raw); err != nil {
		return Planned{}, fmt.Errorf("plan: %w", err)
	}
	var p Planned
	var err error
	if p.ToolVersion, err = required("tool_version", raw.ToolVersion); err != nil {
		return Planned{}, fmt.Errorf("plan: %w", err)
	}
	game, err := required("game", raw.Game)
	if err != nil {
		return Planned{}, fmt.Errorf("plan: %w", err)
	}
	if p.GameID, err = required("game.id", game.ID); err != nil {
		return Planned{}, fmt.Errorf("plan: %w", err)
	}
	if p.GameName, err = required("game.name", game.Name); err != nil {
		return Planned{}, fmt.Errorf("plan: %w", err)
	}
	if p.StartedAt, err = requiredTime("started_at", raw.StartedAt); err != nil {
		return Planned{}, fmt.Errorf("plan: %w", err)
	}
	if p.Previous, _, err = raw.Previous.optionalString("previous"); err != nil {
		return Planned{}, fmt.Errorf("plan: %w", err)
	}
	changes, err := required("changes", raw.Changes)
	if err != nil {
		return Planned{}, fmt.Errorf("plan: %w", err)
	}
	for _, c := range []struct {
		field string
		in    *int
		out   *int
	}{
		{"changes.added", changes.Added, &p.Changes.Added},
		{"changes.modified", changes.Modified, &p.Changes.Modified},
		{"changes.removed", changes.Removed, &p.Changes.Removed},
		{"changes.unchanged", changes.Unchanged, &p.Changes.Unchanged},
	} {
		if *c.out, err = required(c.field, c.in); err != nil {
			return Planned{}, fmt.Errorf("plan: %w", err)
		}
	}
	sources, err := required("sources", raw.Sources)
	if err != nil {
		return Planned{}, fmt.Errorf("plan: %w", err)
	}
	for i, s := range sources {
		source, err := s.source(fmt.Sprintf("sources[%d]", i))
		if err != nil {
			return Planned{}, fmt.Errorf("plan: %w", err)
		}
		p.Sources = append(p.Sources, source)
	}
	return p, p.validate()
}

func (p Planned) validate() error {
	switch {
	case p.ToolVersion == "" || p.GameID == "" || p.GameName == "":
		return errors.New("plan: tool_version, game.id and game.name must not be empty")
	case p.Changes.Added < 0 || p.Changes.Modified < 0 || p.Changes.Removed < 0 || p.Changes.Unchanged < 0:
		return errors.New("plan: changes: counts must not be negative")
	case len(p.Sources) == 0:
		return errors.New("plan: at least one source is required")
	}
	if p.Previous != "" {
		if err := ValidOperationID(p.Previous); err != nil {
			return fmt.Errorf("plan: previous: %w", err)
		}
	}
	if err := validateSources(p.Sources); err != nil {
		return fmt.Errorf("plan: %w", err)
	}
	return nil
}

// ContentDigest is the SHA-256 of the canonical form of sources: what the
// staged and verified records attest, and what a copy must reproduce.
func ContentDigest(sources []ReceiptSource) ([32]byte, error) {
	data, err := json.Marshal(encodeSources(sources))
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(data), nil
}

// Receipt builds the receipt of operation op from its plan. A receipt
// written at publication has publishedAt; one reconstructed by recovery
// has recoveredAt and recoveredBy instead (ADR 0007).
func (p Planned) Receipt(op string, planSHA256 [32]byte, publishIntentAt time.Time, publishedAt, recoveredAt *time.Time, recoveredBy string) (Receipt, error) {
	r := Receipt{
		Operation: op, ToolVersion: p.ToolVersion, RecoveredBy: recoveredBy,
		GameID: p.GameID, GameName: p.GameName,
		StartedAt: p.StartedAt, PublishIntentAt: publishIntentAt,
		PublishedAt: publishedAt, RecoveredAt: recoveredAt,
		Previous: p.Previous, PlanSHA256: planSHA256, Changes: p.Changes,
		Sources: p.Sources,
	}
	return r, r.validate()
}
