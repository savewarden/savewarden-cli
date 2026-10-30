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
	// the plan of 001-planned, under the "fields" key. Their contract is
	// fixed per step and checked by Values.
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
	// committed. Temporaries of a committed number are abandoned attempts:
	// accepted, never counted, never read.
	Prepared map[Step]int
	// Previous is the operation whose generation a rotation replaces, empty
	// for a first publication or before the planned record.
	Previous string
	// Values holds the decoded fields of each record, in order.
	Values []StepValues
	// Plan and PlanSHA256 come from the planned record; Plan is nil before it.
	Plan       *Planned
	PlanSHA256 [32]byte
	// Staging is the identity of the staging directory recorded by staged;
	// Old, that of the current a rotation replaces, fixed under lock by the
	// current_identity of planned and never replaced (ADR 0006 §5, A1).
	Staging, Old FileID
	// Taken holds the attempt numbers k already used by temporaries of the
	// next record: the writer takes the smallest k not in it.
	Taken map[int]bool
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
		seq, k int
		step   Step
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
			k, _ := strconv.Atoi(m[3])
			temporaries = append(temporaries, temporary{seq: seq, k: k, step: Step(m[2])})
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
	j := Journal{Op: op, Prepared: map[Step]int{}, Taken: map[int]bool{}}
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
		values, err := r.Values()
		if err != nil {
			return Journal{}, ambiguous("record %s: %v", name, err)
		}
		if err := j.coherent(r, values); err != nil {
			return Journal{}, ambiguous("record %s: %v", name, err)
		}
		j.Records = append(j.Records, r)
		j.Values = append(j.Values, values)
		previous = data
	}
	// The nine names k = 1..9 of a record number are shared by all its
	// steps (envelope E1): the writer takes the smallest k that no
	// temporary of that number carries, and a committed record was renamed
	// from one of them, so at most eight remain beside it.
	next := len(j.Records) + 1
	used := map[[2]int]bool{}
	abandoned := map[int]int{}
	for _, t := range temporaries {
		if used[[2]int{t.seq, t.k}] {
			return Journal{}, ambiguous("temporaries of record %03d share the attempt number %d", t.seq, t.k)
		}
		used[[2]int{t.seq, t.k}] = true
		switch {
		case t.seq == next && j.Terminal():
			return Journal{}, ambiguous("temporary record after the terminal step %q", j.Last())
		case t.seq == next:
			if !allowed(j.steps(), t.step, j.Rotation()) {
				return Journal{}, ambiguous("temporary record %03d-%s is not an allowed next step", t.seq, t.step)
			}
			j.Prepared[t.step]++
			j.Taken[t.k] = true
		case t.seq >= 1 && t.seq < next:
			// An attempt abandoned before the record of its number was
			// committed, possibly under another step: it must have been an
			// allowed step at that number.
			if !allowed(j.steps()[:t.seq-1], t.step, j.Rotation()) {
				return Journal{}, ambiguous("abandoned temporary %03d-%s was not an allowed step at that number", t.seq, t.step)
			}
			if abandoned[t.seq]++; abandoned[t.seq] >= MaxTemporaries {
				return Journal{}, ambiguous("record %03d is committed beside nine temporaries; at most eight may remain", t.seq)
			}
		default:
			return Journal{}, ambiguous("temporary record %03d-%s does not follow the last committed record", t.seq, t.step)
		}
	}
	return j, nil
}

// coherent checks the values of the next record against the records
// before it, and keeps what later records refer to.
func (j *Journal) coherent(r Record, v StepValues) error {
	switch step := r.Step; step {
	case StepPlanned:
		// The plan starts at the date of its record (plan B2 §4.7).
		if !v.Plan.StartedAt.Equal(r.At) {
			return fmt.Errorf("plan started_at %s differs from the date of its record %s", formatUTC(v.Plan.StartedAt), formatUTC(r.At))
		}
		j.Previous, j.Plan, j.PlanSHA256, j.Old = v.Previous, v.Plan, v.PlanSHA256, v.Current
	case StepStaged, StepVerified:
		want, err := ContentDigest(j.Plan.Sources)
		if err != nil {
			return err
		}
		if v.Digest != want {
			return fmt.Errorf("%s digest does not match the content of the plan", step)
		}
		if step == StepStaged {
			j.Staging = v.Staging
		}
	case StepPublishIntent, StepPromoteIntent:
		if v.Staging != j.Staging {
			return fmt.Errorf("staging identity %s differs from %s recorded by staged", v.Staging, j.Staging)
		}
	case StepUnpublishIntent, StepUnpromoteIntent:
		// What was published or promoted is the staging directory.
		if v.Current != j.Staging {
			return fmt.Errorf("current identity %s differs from the staging identity %s", v.Current, j.Staging)
		}
	case StepRetireIntent:
		// Never an identity observed later: the one planned fixed.
		if v.Current != j.Old {
			return fmt.Errorf("current identity %s differs from %s recorded by planned", v.Current, j.Old)
		}
	case StepRestoreIntent:
		if v.Retained != j.Old {
			return fmt.Errorf("retained identity %s differs from %s recorded by planned", v.Retained, j.Old)
		}
	}
	return nil
}

// Published returns, for an operation terminal in published, the identity
// of the directory it published, staging_identity of staged as publish-intent
// or promote-intent repeated it, and the receipt_sha256 of its published
// record (ADR 0006 §4). ok is false for any other operation.
func (j Journal) Published() (identity FileID, receipt [32]byte, ok bool) {
	if j.Last() != StepPublished {
		return FileID{}, [32]byte{}, false
	}
	return j.Staging, j.Values[len(j.Values)-1].Digest, true
}

// Attempts counts the temporaries of the record that would follow the last
// committed one, whatever their step (envelope E1, counted per number).
// Abandoned temporaries of committed numbers do not count: their number is
// never written again.
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
