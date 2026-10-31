package backup

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// StepValues are the values proper to one step, kept under the "fields"
// key of its record (ADR 0006 §3). Each step uses the values its contract
// names and leaves the others zero.
type StepValues struct {
	// Previous and Plan: planned. PlanSHA256 is computed, never chosen.
	Previous   string
	Plan       *Planned
	PlanSHA256 [32]byte
	// Identities of the staging directory, of current, of the retained
	// generation, as each intent expects them. For planned, Current is the
	// current a rotation replaces, zero for a first publication.
	Staging, Current, Retained FileID
	// Digest: copied_sha256 of staged, verified_sha256 of verified,
	// receipt_sha256 of published.
	Digest [32]byte
	// Text: reason of failed and abandoned, error of rollback-failed.
	Text string
	// Planned of a snapshot operation (ADR 0006 §10): the generation it
	// copies and the SHA-256 of its receipt file, the identity of current
	// under lock, and the plan. PlanSHA256 is computed from SnapshotPlan.
	SnapshotSource SnapshotSource
	SourceIdentity FileID
	SnapshotPlan   *SnapshotPlan
}

type fieldKind int

const (
	fieldPrevious fieldKind = iota
	fieldPlan
	fieldPlanSHA256
	fieldStagingName
	fieldStaging
	fieldCurrent
	fieldRetained
	fieldDigest
	fieldText
	fieldSourceOperation
	fieldSourceReceipt
	fieldSourceIdentity
	fieldSnapshotPlan
)

// stepContract names, for every step, its only fields; all are required,
// except the current_identity of planned, required for a rotation only.
var stepContract = map[Step]map[string]fieldKind{
	StepPlanned:         {"previous": fieldPrevious, "plan": fieldPlan, "plan_sha256": fieldPlanSHA256, plannedCurrent: fieldCurrent},
	StepStagingIntent:   {"staging": fieldStagingName},
	StepStaged:          {"staging_identity": fieldStaging, "copied_sha256": fieldDigest},
	StepVerified:        {"verified_sha256": fieldDigest},
	StepPublishIntent:   {"staging_identity": fieldStaging},
	StepUnpublishIntent: {"current_identity": fieldCurrent},
	StepRetireIntent:    {"current_identity": fieldCurrent},
	StepPromoteIntent:   {"staging_identity": fieldStaging},
	StepUnpromoteIntent: {"current_identity": fieldCurrent},
	StepRestoreIntent:   {"retained_identity": fieldRetained},
	StepRollbackFailed:  {"error": fieldText},
	StepPublished:       {"receipt_sha256": fieldDigest},
	StepFailed:          {"reason": fieldText},
	StepAbandoned:       {"reason": fieldText},
	StepRolledBack:      {},
}

// maxText bounds the free text of a record.
const maxText = 1024

// plannedCurrent is the identity of the current a rotation replaces, taken
// under lock before the planned record (ADR 0006 §5, A1): required when
// previous is not null, forbidden otherwise. B2b never wrote it.
const plannedCurrent = "current_identity"

// StagingName is the staging directory of op, relative to the game
// directory, as the staging-intent record names it.
func StagingName(op string) string { return "staging/" + op }

// NewRecord builds the record of one step of the current domain from its
// values, which must follow the step's contract.
func NewRecord(seq int, step Step, op string, at time.Time, prev string, v StepValues) (Record, error) {
	if step == StepPlanned && v.Previous != "" && v.Current == (FileID{}) {
		return Record{}, fmt.Errorf("planned record of a rotation: %s of the replaced current required", plannedCurrent)
	}
	r, err := newRecord(stepContract, StagingName(op), seq, step, op, at, prev, v)
	if err != nil {
		return Record{}, err
	}
	if _, err := r.Values(); err != nil {
		return Record{}, err
	}
	return r, nil
}

func newRecord(contracts map[Step]map[string]fieldKind, staging string, seq int, step Step, op string, at time.Time, prev string, v StepValues) (Record, error) {
	contract, ok := contracts[step]
	if !ok {
		return Record{}, fmt.Errorf("unknown step %q", step)
	}
	fields := map[string]json.RawMessage{}
	for name, kind := range contract {
		if step == StepPlanned && name == plannedCurrent && v.Previous == "" {
			continue
		}
		var value any
		switch kind {
		case fieldPrevious:
			if v.Previous != "" {
				value = v.Previous
			}
		case fieldPlan:
			if v.Plan == nil {
				return Record{}, errors.New("planned record: plan required")
			}
			plan, _, err := v.Plan.Encode()
			if err != nil {
				return Record{}, err
			}
			fields[name] = plan
			continue
		case fieldPlanSHA256:
			var sum [32]byte
			var err error
			switch {
			case v.Plan != nil:
				_, sum, err = v.Plan.Encode()
			case v.SnapshotPlan != nil:
				_, sum, err = v.SnapshotPlan.Encode()
			default:
				err = errors.New("planned record: plan required")
			}
			if err != nil {
				return Record{}, err
			}
			value = hex.EncodeToString(sum[:])
		case fieldSnapshotPlan:
			if v.SnapshotPlan == nil {
				return Record{}, errors.New("planned record: plan required")
			}
			plan, _, err := v.SnapshotPlan.Encode()
			if err != nil {
				return Record{}, err
			}
			fields[name] = plan
			continue
		case fieldSourceOperation:
			value = v.SnapshotSource.Operation
		case fieldSourceReceipt:
			value = hex.EncodeToString(v.SnapshotSource.ReceiptSHA256[:])
		case fieldSourceIdentity:
			value = v.SourceIdentity.String()
		case fieldStagingName:
			value = staging
		case fieldStaging:
			value = v.Staging.String()
		case fieldCurrent:
			value = v.Current.String()
		case fieldRetained:
			value = v.Retained.String()
		case fieldDigest:
			value = hex.EncodeToString(v.Digest[:])
		case fieldText:
			value = v.Text
		}
		data, err := json.Marshal(value)
		if err != nil {
			return Record{}, err
		}
		fields[name] = data
	}
	return Record{Seq: seq, Step: step, Op: op, At: at, Prev: prev, Fields: fields}, nil
}

// Values decodes the fields of a record of the current domain strictly
// against the contract of its step: every field required, no other
// accepted, each of its exact type.
func (r Record) Values() (StepValues, error) {
	return r.values(stepContract, StagingName(r.Op))
}

func (r Record) values(contracts map[Step]map[string]fieldKind, staging string) (StepValues, error) {
	contract, ok := contracts[r.Step]
	if !ok {
		return StepValues{}, fmt.Errorf("unknown step %q", r.Step)
	}
	fail := func(format string, args ...any) error {
		return fmt.Errorf("%s fields: %s", r.Step, fmt.Sprintf(format, args...))
	}
	for name := range r.Fields {
		if _, known := contract[name]; !known {
			return StepValues{}, fail("unknown field %q", name)
		}
	}
	names := make([]string, 0, len(contract))
	for name := range contract {
		names = append(names, name)
	}
	sort.Strings(names)
	var v StepValues
	var planSum [32]byte
	for _, name := range names {
		raw, ok := r.Fields[name]
		if !ok && r.Step == StepPlanned && name == plannedCurrent {
			continue // checked against previous below
		}
		if !ok {
			return StepValues{}, fail("missing field %q", name)
		}
		switch contract[name] {
		case fieldPrevious:
			if string(raw) == "null" {
				continue
			}
			previous, err := decodeString(raw)
			if err != nil || ValidOperationID(previous) != nil || previous == r.Op {
				return StepValues{}, fail(`"previous" must be null or another operation id`)
			}
			v.Previous = previous
		case fieldPlan:
			plan, err := DecodePlanned(raw)
			if err != nil {
				return StepValues{}, fail("%v", err)
			}
			v.Plan = &plan
			planSum = sha256.Sum256(raw)
		case fieldPlanSHA256:
			sum, err := decodeSHA256(raw)
			if err != nil {
				return StepValues{}, fail("%q: %v", name, err)
			}
			v.PlanSHA256 = sum
		case fieldStagingName:
			if name, err := decodeString(raw); err != nil || name != staging {
				return StepValues{}, fail(`"staging" must be %q`, staging)
			}
		case fieldSnapshotPlan:
			plan, err := DecodeSnapshotPlan(raw)
			if err != nil {
				return StepValues{}, fail("%v", err)
			}
			v.SnapshotPlan = &plan
			planSum = sha256.Sum256(raw)
		case fieldSourceOperation:
			source, err := decodeString(raw)
			if err != nil || ValidOperationID(source) != nil || source == r.Op {
				return StepValues{}, fail(`%q must be another operation id`, name)
			}
			v.SnapshotSource.Operation = source
		case fieldSourceReceipt:
			sum, err := decodeSHA256(raw)
			if err != nil {
				return StepValues{}, fail("%q: %v", name, err)
			}
			v.SnapshotSource.ReceiptSHA256 = sum
		case fieldStaging, fieldCurrent, fieldRetained, fieldSourceIdentity:
			text, err := decodeString(raw)
			if err != nil {
				return StepValues{}, fail("%q: %v", name, err)
			}
			id, err := ParseFileID(text)
			if err != nil {
				return StepValues{}, fail("%q: %v", name, err)
			}
			switch contract[name] {
			case fieldStaging:
				v.Staging = id
			case fieldCurrent:
				v.Current = id
			case fieldSourceIdentity:
				v.SourceIdentity = id
			default:
				v.Retained = id
			}
		case fieldDigest:
			sum, err := decodeSHA256(raw)
			if err != nil {
				return StepValues{}, fail("%q: %v", name, err)
			}
			v.Digest = sum
		case fieldText:
			text, err := decodeString(raw)
			if err != nil || text == "" || len(text) > maxText || strings.ContainsAny(text, "\r\n") {
				return StepValues{}, fail("%q must be a single line of 1 to %d bytes", name, maxText)
			}
			v.Text = text
		}
	}
	if v.Plan != nil {
		if v.PlanSHA256 != planSum {
			return StepValues{}, fail(`"plan_sha256" does not match the plan`)
		}
		if v.Plan.Previous != v.Previous {
			return StepValues{}, fail(`"previous" differs from the plan`)
		}
		_, recorded := r.Fields[plannedCurrent]
		switch {
		case v.Previous != "" && !recorded:
			return StepValues{}, fail(`missing field %q: a rotation records the current it replaces`, plannedCurrent)
		case v.Previous == "" && recorded:
			return StepValues{}, fail(`%q is forbidden for a first publication`, plannedCurrent)
		}
	}
	if v.SnapshotPlan != nil && v.PlanSHA256 != planSum {
		return StepValues{}, fail(`"plan_sha256" does not match the plan`)
	}
	return v, nil
}

func decodeString(raw json.RawMessage) (string, error) {
	if !bytes.HasPrefix(bytes.TrimSpace(raw), []byte(`"`)) {
		return "", errors.New("must be a string")
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", err
	}
	return s, nil
}

func decodeSHA256(raw json.RawMessage) ([32]byte, error) {
	s, err := decodeString(raw)
	if err != nil {
		return [32]byte{}, err
	}
	return parseSHA256("sha256", s)
}
