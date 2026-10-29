package backup

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"time"
)

// Journal schema identifiers (ADR 0006 §3).
const (
	JournalSchema        = "savewarden.journal"
	JournalSchemaVersion = 1
	// MaxRecords bounds the committed records of one operation (envelope
	// E2): the automaton is acyclic and its longest path has 11 steps.
	MaxRecords = 11
)

// Step names one record of an operation journal.
type Step string

// Journal steps of ADR 0006 §4 and §5.
const (
	StepPlanned         Step = "planned"
	StepStagingIntent   Step = "staging-intent"
	StepStaged          Step = "staged"
	StepVerified        Step = "verified"
	StepPublishIntent   Step = "publish-intent"
	StepUnpublishIntent Step = "unpublish-intent"
	StepRetireIntent    Step = "retire-intent"
	StepPromoteIntent   Step = "promote-intent"
	StepUnpromoteIntent Step = "unpromote-intent"
	StepRestoreIntent   Step = "restore-intent"
	StepRollbackFailed  Step = "rollback-failed"
	StepPublished       Step = "published"
	StepFailed          Step = "failed"
	StepAbandoned       Step = "abandoned"
	StepRolledBack      Step = "rolled-back"
)

var knownSteps = map[Step]bool{
	StepPlanned: true, StepStagingIntent: true, StepStaged: true, StepVerified: true,
	StepPublishIntent: true, StepUnpublishIntent: true, StepRetireIntent: true,
	StepPromoteIntent: true, StepUnpromoteIntent: true, StepRestoreIntent: true,
	StepRollbackFailed: true, StepPublished: true, StepFailed: true, StepAbandoned: true,
	StepRolledBack: true,
}

// Terminal reports whether no record may follow s.
func (s Step) Terminal() bool {
	switch s {
	case StepPublished, StepFailed, StepAbandoned, StepRolledBack:
		return true
	}
	return false
}

// Record is one committed step of an operation.
type Record struct {
	Seq  int
	Step Step
	Op   string
	At   time.Time
	// Prev is the hexadecimal SHA-256 of the previous record file, empty
	// for the first record.
	Prev string
	// Fields carries the values proper to the step, such as identities or
	// the plan of 001-planned. They are validated by the code that writes
	// them; this package only checks that they are JSON.
	Fields map[string]json.RawMessage
}

// RecordName is the final file name of a record, such as 005-publish-intent.rec.
func RecordName(seq int, step Step) string { return fmt.Sprintf("%03d-%s.rec", seq, step) }

// EncodeRecord returns the two lines of a record file: canonical JSON with
// sorted keys, then the SHA-256 of that first line, LF included.
func EncodeRecord(r Record) ([]byte, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	fields := map[string]json.RawMessage{}
	for name, value := range r.Fields {
		var compact bytes.Buffer
		if err := json.Compact(&compact, value); err != nil {
			return nil, fmt.Errorf("record field %q: %w", name, err)
		}
		fields[name] = compact.Bytes()
	}
	var prev any
	if r.Prev != "" {
		prev = r.Prev
	}
	line, err := json.Marshal(map[string]any{
		"at": formatUTC(r.At), "fields": fields, "op": r.Op, "prev": prev,
		"schema": JournalSchema, "schema_version": JournalSchemaVersion,
		"seq": r.Seq, "step": r.Step,
	})
	if err != nil {
		return nil, err
	}
	line = append(line, '\n')
	sum := sha256.Sum256(line)
	return append(line, []byte("sha256 "+hex.EncodeToString(sum[:])+"\n")...), nil
}

func (r Record) check() error {
	switch {
	case r.Seq < 1 || r.Seq > MaxRecords:
		return fmt.Errorf("record seq %d outside 1..%d", r.Seq, MaxRecords)
	case !knownSteps[r.Step]:
		return fmt.Errorf("unknown step %q", r.Step)
	case (r.Seq == 1) != (r.Prev == ""):
		return errors.New("only the first record has no previous record")
	}
	if r.Prev != "" {
		if _, err := parseSHA256("prev", r.Prev); err != nil {
			return err
		}
	}
	return ValidOperationID(r.Op)
}

// DecodeRecord reads one committed record file and checks its own checksum.
func DecodeRecord(data []byte) (Record, error) {
	first := bytes.IndexByte(data, '\n')
	if first < 0 || !bytes.HasSuffix(data, []byte("\n")) {
		return Record{}, errors.New("truncated record: missing line ends")
	}
	line, trailer := data[:first+1], data[first+1:]
	sum := sha256.Sum256(line)
	if string(trailer) != "sha256 "+hex.EncodeToString(sum[:])+"\n" {
		return Record{}, errors.New("record checksum does not match its content")
	}
	var raw struct {
		At            *string                     `json:"at"`
		Fields        *map[string]json.RawMessage `json:"fields"`
		Op            *string                     `json:"op"`
		Prev          nullable                    `json:"prev"`
		Schema        *string                     `json:"schema"`
		SchemaVersion *int                        `json:"schema_version"`
		Seq           *int                        `json:"seq"`
		Step          *string                     `json:"step"`
	}
	if err := decodeStrict(line, &raw); err != nil {
		return Record{}, fmt.Errorf("record: %w", err)
	}
	schema, err := required("schema", raw.Schema)
	if err != nil {
		return Record{}, err
	}
	version, err := required("schema_version", raw.SchemaVersion)
	if err != nil {
		return Record{}, err
	}
	if schema != JournalSchema || version != JournalSchemaVersion {
		return Record{}, fmt.Errorf("unsupported journal schema %q version %d", schema, version)
	}
	var r Record
	if r.Seq, err = required("seq", raw.Seq); err != nil {
		return Record{}, err
	}
	step, err := required("step", raw.Step)
	if err != nil {
		return Record{}, err
	}
	r.Step = Step(step)
	if r.Op, err = required("op", raw.Op); err != nil {
		return Record{}, err
	}
	if r.At, err = requiredTime("at", raw.At); err != nil {
		return Record{}, err
	}
	if r.Prev, _, err = raw.Prev.optionalString("prev"); err != nil {
		return Record{}, err
	}
	fields, err := required("fields", raw.Fields)
	if err != nil {
		return Record{}, err
	}
	r.Fields = fields
	return r, r.check()
}

// AmbiguityError refuses a journal or an observation that the protocol does
// not explain. Recovery stops on it, without moving or writing anything.
type AmbiguityError struct {
	Op     string
	Reason string
	Err    error // optional cause, such as ErrEnvelopeExhausted
}

func (e *AmbiguityError) Error() string {
	if e.Err != nil {
		return "operation " + e.Op + ": " + e.Err.Error() + ": " + e.Reason
	}
	return "operation " + e.Op + ": " + e.Reason
}

// Unwrap returns the cause.
func (e *AmbiguityError) Unwrap() error { return e.Err }

// ErrEnvelopeExhausted is wrapped by an AmbiguityError when recovery would
// need a write beyond the envelope of ADR 0006 §9.
var ErrEnvelopeExhausted = errors.New("recovery envelope exhausted")

// Journal is the validated content of one operation directory.
type Journal struct {
	Op      string
	Records []Record
	// Prepared counts, for each step, the temporaries of the record that
	// would follow the last committed one: intentions written but never
	// committed.
	Prepared map[Step]int
	// Previous is the operation whose generation a rotation replaces, empty
	// for a first publication or before the planned record.
	Previous string
}

// Last returns the step of the last committed record, empty if none.
func (j Journal) Last() Step {
	if len(j.Records) == 0 {
		return ""
	}
	return j.Records[len(j.Records)-1].Step
}

// Terminal reports whether the operation is closed.
func (j Journal) Terminal() bool { return j.Last().Terminal() }

// Rotation reports whether the operation replaces an existing current: its
// planned record names the previous generation.
func (j Journal) Rotation() bool { return j.Previous != "" }

// plannedPrevious reads the "previous" field of a planned record: null for
// a first publication, or the operation id of the generation to replace.
// Rotation and first publication follow different automata, so the field is
// validated here rather than left to the writer.
func plannedPrevious(r Record) (string, error) {
	raw, ok := r.Fields["previous"]
	if !ok {
		return "", errors.New(`planned record lacks the field "previous"`)
	}
	if string(raw) == "null" {
		return "", nil
	}
	var previous string
	if err := json.Unmarshal(raw, &previous); err != nil {
		return "", errors.New(`planned record: "previous" must be null or an operation id`)
	}
	if err := ValidOperationID(previous); err != nil || previous == r.Op {
		return "", fmt.Errorf(`planned record: "previous" %q is not another operation id`, previous)
	}
	return previous, nil
}

var (
	finalName     = regexp.MustCompile(`^([0-9]{3})-([a-z-]+)\.rec$`)
	temporaryName = regexp.MustCompile(`^([0-9]{3})-([a-z-]+)\.rec\.tmp-([1-9])$`)
)

// ParseJournal validates the listing of journal\<op> and the content of its
// committed records. Every deviation from ADR 0006 §3 is an AmbiguityError.
func ParseJournal(op string, names []string, contents map[string][]byte) (Journal, error) {
	ambiguous := func(format string, args ...any) error {
		return &AmbiguityError{Op: op, Reason: fmt.Sprintf(format, args...)}
	}
	if err := ValidOperationID(op); err != nil {
		return Journal{}, ambiguous("journal directory: %v", err)
	}
	type temporary struct {
		seq  int
		step Step
	}
	var finals []string
	var temporaries []temporary
	for _, name := range names {
		if m := finalName.FindStringSubmatch(name); m != nil {
			finals = append(finals, name)
			continue
		}
		if m := temporaryName.FindStringSubmatch(name); m != nil {
			seq, _ := strconv.Atoi(m[1])
			temporaries = append(temporaries, temporary{seq, Step(m[2])})
			continue
		}
		return Journal{}, ambiguous("unexpected entry %q in the journal", name)
	}
	sort.Strings(finals)
	numbers := map[string]bool{}
	for _, name := range finals {
		if numbers[name[:3]] {
			return Journal{}, ambiguous("duplicate record number %s", name[:3])
		}
		numbers[name[:3]] = true
	}
	j := Journal{Op: op, Prepared: map[Step]int{}}
	var previous []byte
	for i, name := range finals {
		m := finalName.FindStringSubmatch(name)
		seq, _ := strconv.Atoi(m[1])
		if seq != i+1 {
			return Journal{}, ambiguous("record %s: missing or duplicate number, expected %03d", name, i+1)
		}
		data, ok := contents[name]
		if !ok {
			return Journal{}, ambiguous("record %s could not be read", name)
		}
		r, err := DecodeRecord(data)
		if err != nil {
			return Journal{}, ambiguous("record %s: %v", name, err)
		}
		if r.Seq != seq || string(r.Step) != m[2] || r.Op != op {
			return Journal{}, ambiguous("record %s: content does not match its name or operation", name)
		}
		if seq > 1 {
			sum := sha256.Sum256(previous)
			if r.Prev != hex.EncodeToString(sum[:]) {
				return Journal{}, ambiguous("record %s: chain broken, previous record does not match", name)
			}
		}
		if !allowed(j.steps(), r.Step, j.Rotation()) {
			return Journal{}, ambiguous("record %s: transition from %q to %q is not allowed", name, j.Last(), r.Step)
		}
		if r.Step == StepPlanned {
			if j.Previous, err = plannedPrevious(r); err != nil {
				return Journal{}, ambiguous("record %s: %v", name, err)
			}
		}
		j.Records = append(j.Records, r)
		previous = data
	}
	next := len(j.Records) + 1
	for _, t := range temporaries {
		switch {
		case j.Terminal():
			return Journal{}, ambiguous("temporary record after the terminal step %q", j.Last())
		case t.seq != next:
			return Journal{}, ambiguous("temporary record %03d-%s does not follow the last committed record", t.seq, t.step)
		case !allowed(j.steps(), t.step, j.Rotation()):
			return Journal{}, ambiguous("temporary record %03d-%s is not an allowed next step", t.seq, t.step)
		}
		j.Prepared[t.step]++
	}
	return j, nil
}

// Attempts counts the temporaries of the record that would follow the last
// committed one, whatever their step (envelope E1, counted per number).
func (j Journal) Attempts() int {
	total := 0
	for _, count := range j.Prepared {
		total += count
	}
	return total
}

func (j Journal) steps() []Step {
	steps := make([]Step, len(j.Records))
	for i, r := range j.Records {
		steps[i] = r.Step
	}
	return steps
}

// allowed applies the acyclic automaton of ADR 0006 §4 and §5. A first
// publication and a rotation never share a switch step.
func allowed(history []Step, next Step, rotation bool) bool {
	if len(history) >= MaxRecords {
		return false
	}
	var last Step
	if len(history) > 0 {
		last = history[len(history)-1]
	}
	switch last {
	case "":
		return next == StepPlanned || next == StepAbandoned
	case StepPlanned:
		return next == StepStagingIntent || next == StepFailed || next == StepAbandoned
	case StepStagingIntent:
		return next == StepStaged || next == StepFailed || next == StepAbandoned
	case StepStaged:
		return next == StepVerified || next == StepFailed || next == StepAbandoned
	case StepVerified:
		switch next {
		case StepPublishIntent:
			return !rotation
		case StepRetireIntent:
			return rotation
		}
		return next == StepFailed || next == StepAbandoned
	case StepPublishIntent:
		return next == StepPublished || next == StepUnpublishIntent || next == StepRolledBack
	case StepUnpublishIntent:
		return next == StepRolledBack || next == StepRollbackFailed
	case StepRetireIntent:
		return next == StepPromoteIntent || next == StepRestoreIntent || next == StepRolledBack
	case StepPromoteIntent:
		return next == StepPublished || next == StepUnpromoteIntent || next == StepRestoreIntent
	case StepUnpromoteIntent:
		return next == StepRestoreIntent || next == StepRollbackFailed
	case StepRestoreIntent:
		return next == StepRolledBack || next == StepRollbackFailed
	case StepRollbackFailed:
		// At most one rollback-failed per intent (envelope E3): after it,
		// only the successor of the failed intent may follow.
		switch history[len(history)-2] {
		case StepUnpublishIntent, StepRestoreIntent:
			return next == StepRolledBack
		case StepUnpromoteIntent:
			return next == StepRestoreIntent
		}
	}
	return false
}
