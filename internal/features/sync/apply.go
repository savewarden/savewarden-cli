package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

// Codes of a run that published nothing, beyond the plan's findings
// (ADR 0007).
const (
	CodeCurrentExists        backup.Code = "current-exists"
	CodeChangedDuringSync    backup.Code = "changed-during-sync"
	CodeVerificationMismatch backup.Code = "verification-mismatch"
)

// Writer carries what --apply and --recover need besides the plan's
// read-only adapters. The plan never receives it (ADR 0007).
type Writer struct {
	Vaults Vaults
	Clock  Clock
	// Random supplies the random part of operation ids: crypto/rand.
	Random      io.Reader
	ToolVersion string
}

// Effect is what a run did to the vault, announced by the last line of
// stderr (ADR 0007).
type Effect int

// Effects of a run.
const (
	EffectNone         Effect = iota // not modified
	EffectMetadata                   // metadata only; nothing staged or published
	EffectStaged                     // current unchanged; operation kept in staging
	EffectPublished                  // published
	EffectRolledBack                 // rolled back; current unchanged
	EffectIntervention               // intervention required
)

// Result reports an --apply or --recover run, complete or not.
type Result struct {
	Plan      Plan
	Operation string
	Effect    Effect
	// Cause explains EffectIntervention.
	Cause string
	// Receipt is the Windows path of the receipt once published; Files and
	// Bytes count what was verified.
	Receipt string
	Files   int
	Bytes   int64
	// Recovery is what --recover found and did, nil for --apply.
	Recovery *Recovery
}

// State is the vault state of the run, as "savewarden: vault: <state>"
// announces it.
func (r Result) State() string {
	switch r.Effect {
	case EffectMetadata:
		return "metadata only; nothing staged or published"
	case EffectStaged:
		return "current unchanged; operation " + r.Operation + " kept in staging"
	case EffectPublished:
		return "published " + r.Operation
	case EffectRolledBack:
		return "rolled back " + r.Operation + "; current unchanged"
	case EffectIntervention:
		subject := r.Operation
		if subject == "" {
			subject = "the vault"
		}
		return "intervention required for " + subject + ": " + r.Cause
	}
	return "not modified"
}

// BlockedError ends a run that published nothing because of a blocking
// condition: exit code 2.
type BlockedError struct {
	Code   backup.Code // empty for a plan with blocking findings
	Detail string
}

func (e *BlockedError) Error() string {
	if e.Code == "" {
		return "plan blocked: " + e.Detail
	}
	return string(e.Code) + ": " + e.Detail
}

// InterventionError ends a run that found a state it must not guess, or
// that cannot finish without recovery: exit code 3.
type InterventionError struct {
	Operation string
	Cause     string
}

func (e *InterventionError) Error() string {
	subject := e.Operation
	if subject == "" {
		subject = "the vault"
	}
	return "intervention required for " + subject + ": " + e.Cause
}

// operation is one --apply run, from the vault session to its terminal
// record.
type operation struct {
	s       *Service
	w       Writer
	ctx     context.Context
	plan    Plan
	vault   Vault
	game    string // games/<id>
	op      string
	started time.Time
	effect  Effect
	// Journal: number and SHA-256 of the last committed record, and the
	// attempt numbers k already taken, per record number and for the
	// receipt and the marker (envelope E1).
	seq      int
	prev     string
	attempts map[int]map[int]bool
	receipt  map[int]bool
	marker   map[int]bool
	// critical is set from publish-intent to the terminal record:
	// cancellation is deferred until the switch is complete (ADR 0006 §7).
	critical bool
	// terminal is set once a terminal record is committed: nothing follows.
	terminal bool
	planned  *backup.Planned
	planSum  [32]byte
	staging  backup.FileID
	intentAt time.Time
}

// Apply plans, then copies, verifies and publishes a first generation
// (ADR 0006 §4). Its Result says what the vault now holds even when it
// fails. An existing current that differs from the plan is refused with
// current-exists: replacing it is the rotation of B2c.
func (s *Service) Apply(ctx context.Context, request Request, w Writer) (Result, error) {
	plan, err := s.Plan(ctx, request)
	if err != nil {
		return Result{}, err
	}
	result := Result{Plan: plan}
	switch {
	case len(plan.Interventions) > 0:
		first := plan.Interventions[0]
		result.Effect, result.Cause = EffectIntervention, string(first.Cause)+" "+first.Subject
		return result, &InterventionError{Cause: result.Cause}
	case plan.Blocking() > 0:
		return result, &BlockedError{Detail: fmt.Sprintf("%d blocking finding(s)", plan.Blocking())}
	case plan.upToDate():
		return result, nil
	case plan.Current != "":
		return result, &BlockedError{Code: CodeCurrentExists,
			Detail: "the vault already holds a generation of this game; replacing it is not available in this build"}
	}
	o, err := s.begin(ctx, plan, w)
	if err != nil {
		return result, err
	}
	err = o.run()
	if closeErr := o.vault.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	return o.result(), err
}

func (s *Service) begin(ctx context.Context, plan Plan, w Writer) (*operation, error) {
	now := w.Clock.Now().UTC()
	random := make([]byte, 4)
	if _, err := io.ReadFull(w.Random, random); err != nil {
		return nil, fmt.Errorf("operation id: %w", err)
	}
	session, err := w.Vaults.Open(plan.Vault)
	if err != nil {
		return nil, errVault(plan.Vault, "%v", err)
	}
	return &operation{
		s: s, w: w, ctx: ctx, plan: plan, vault: session,
		game:     gamesDir + "/" + plan.Game.ID,
		op:       now.Format("20060102T150405") + "-" + hex.EncodeToString(random),
		started:  now,
		attempts: map[int]map[int]bool{}, receipt: map[int]bool{}, marker: map[int]bool{},
	}, nil
}

// run performs the first publication; the caller closes the session.
func (o *operation) run() error {
	if err := o.initialize(); err != nil {
		return err
	}
	for _, phase := range []func() error{o.stage, o.recheck, o.verify, o.publish} {
		if err := phase(); err != nil {
			return o.fail(err)
		}
	}
	return nil
}

func (o *operation) result() Result {
	r := Result{Plan: o.plan, Operation: o.op, Effect: o.effect}
	if o.effect == EffectPublished {
		receipt := append(append([]string{}, o.plan.vaultSegments...), gamesDir, o.plan.Game.ID, receiptsDir, o.op+".json")
		r.Receipt = windowsPath(o.plan.vaultVolume, receipt)
		for _, source := range o.planned.Sources {
			r.Files += len(source.Entries)
			r.Bytes += source.Bytes()
		}
	}
	return r
}

// wrote marks a write of initialization metadata.
func (o *operation) wrote() {
	if o.effect == EffectNone {
		o.effect = EffectMetadata
	}
}

func (o *operation) now() time.Time { return o.w.Clock.Now().UTC() }

func (o *operation) journalDir() string { return o.game + "/" + journalDir + "/" + o.op }

func (o *operation) stagingDir() string { return o.game + "/" + backup.StagingName(o.op) }

// initialize follows steps I2 to I9 of ADR 0006 §4, made only of exclusive
// creations, up to the planned record.
func (o *operation) initialize() error {
	state := o.plan.VaultState
	// I2: the vault itself.
	if state == VaultAbsent {
		if err := o.vault.CreateDir("."); err != nil {
			if errors.Is(err, fs.ErrExist) {
				return errVault(o.plan.Vault, "vault-changed: the vault appeared since the plan; run sync again")
			}
			return errVault(o.plan.Vault, "%w", err)
		}
		o.wrote()
	}
	// I3 and I4: the marker, committed by rename.
	if state == VaultAbsent || state == VaultEmpty {
		marker, err := backup.EncodeMarker(backup.Marker{CreatedAt: o.now(), ToolVersion: o.w.ToolVersion})
		if err != nil {
			return err
		}
		if err := o.commitFile(backup.MarkerName, marker, o.marker, o.wrote); err != nil {
			return errVault(o.plan.Vault, "marker: %w", err)
		}
	}
	// I5: games and the game directory, each only if missing.
	for _, dir := range []string{gamesDir, o.game} {
		if err := o.ensureDir(dir); err != nil {
			return errVault(o.plan.Vault, "%w", err)
		}
	}
	// I6: the lock, never written or deleted.
	if err := o.vault.Lock(o.game + "/" + lockFile); err != nil {
		var busy interface{ Busy() bool }
		if errors.As(err, &busy) && busy.Busy() {
			return errVault(o.plan.Vault, "another sync is running on this game")
		}
		return errVault(o.plan.Vault, "lock: %w", err)
	}
	if o.plan.observed == nil || !o.plan.observed.lock {
		o.wrote()
	}
	// I7: under lock, the vault is read again and the space checked again.
	if err := o.underLock(); err != nil {
		return err
	}
	// I8: the operation directory, exclusive.
	if err := o.ensureDir(o.game + "/" + journalDir); err != nil {
		return errVault(o.plan.Vault, "%w", err)
	}
	if err := o.vault.CreateDir(o.journalDir()); err != nil {
		return errVault(o.plan.Vault, "operation directory: %w", err)
	}
	o.effect = EffectStaged
	// I9: the planned record, which starts the operation.
	planned, err := o.plannedContent()
	if err != nil {
		return o.fail(err)
	}
	if err := o.record(backup.StepPlanned, backup.StepValues{Plan: planned}); err != nil {
		return o.fail(err)
	}
	return nil
}

// underLock reads the vault again and requires the state of the plan, then
// checks the space once more (plan B2 §4.3, V2).
func (o *operation) underLock() error {
	v, err := o.s.observe(o.plan.Vault, o.plan.vaultVolume, o.plan.vaultSegments, o.plan.Game.ID)
	if err != nil {
		return err
	}
	before := o.plan.observed
	changed := v.state != VaultInitialized || len(v.interventions) > 0 || len(v.pending) > 0 ||
		v.current != before.current || v.currentLink != before.currentLink || len(v.receipts) != len(before.receipts)
	if changed {
		return errVault(o.plan.Vault, "vault-changed: the vault changed since the plan; run sync again")
	}
	volume, err := o.s.volumes.Describe(o.plan.Vault)
	if err != nil {
		return errVault(o.plan.Vault, "volume cannot be described: %w", err)
	}
	if volume.Available < o.plan.Needed {
		return &BlockedError{Code: CodeInsufficientSpace,
			Detail: fmt.Sprintf("%d bytes needed, %d available to this user under lock", o.plan.Needed, volume.Available)}
	}
	return nil
}

// plannedContent is the full plan the planned record fixes.
func (o *operation) plannedContent() (*backup.Planned, error) {
	p := &backup.Planned{
		ToolVersion: o.w.ToolVersion, GameID: o.plan.Game.ID, GameName: o.plan.Game.Name,
		StartedAt: o.started,
	}
	for _, source := range o.plan.Sources {
		rs := backup.ReceiptSource{ID: source.Source.ID, Kind: string(source.Source.Kind), Required: source.Source.Required, Path: source.Path}
		switch source.Status {
		case SourcePlanned:
			rs.State = backup.SourceCopied
			rs.Directories = append([]string{}, source.inventory.Directories...)
			rs.Entries = append([]backup.Entry{}, source.inventory.Entries...)
			p.Changes.Added += source.Changes.Added
			p.Changes.Modified += source.Changes.Modified
			p.Changes.Removed += source.Changes.Removed
			p.Changes.Unchanged += source.Changes.Unchanged
		case SourceMissing:
			rs.State, rs.Directories = backup.SourceMissing, []string{}
		default:
			return nil, fmt.Errorf("source %s is %s; nothing can be published", source.Source.ID, source.Status)
		}
		p.Sources = append(p.Sources, rs)
	}
	if _, sum, err := p.Encode(); err != nil {
		return nil, err
	} else {
		o.planned, o.planSum = p, sum
	}
	return p, nil
}

// ensureDir holds dir, creating it only if it is missing.
func (o *operation) ensureDir(dir string) error {
	err := o.vault.Hold(dir)
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := o.vault.CreateDir(dir); err != nil {
		return err
	}
	o.wrote()
	return nil
}

// record commits the next record of the journal (ADR 0006 §3).
func (o *operation) record(step backup.Step, values backup.StepValues) error {
	seq := o.seq + 1
	r, err := backup.NewRecord(seq, step, o.op, o.now(), o.prev, values)
	if err != nil {
		return err
	}
	data, err := backup.EncodeRecord(r)
	if err != nil {
		return err
	}
	if o.attempts[seq] == nil {
		o.attempts[seq] = map[int]bool{}
	}
	if err := o.commitFile(o.journalDir()+"/"+backup.RecordName(seq, step), data, o.attempts[seq], nil); err != nil {
		return fmt.Errorf("record %s: %w", backup.RecordName(seq, step), err)
	}
	if step == backup.StepPublishIntent {
		o.intentAt = r.At
	}
	sum := sha256.Sum256(data)
	o.seq, o.prev, o.terminal = seq, hex.EncodeToString(sum[:]), step.Terminal()
	return nil
}

// commitFile writes data to a new temporary and renames it to final: the
// smallest attempt number k not yet taken among used, which it takes even
// if the attempt fails. A temporary is never removed or reused.
func (o *operation) commitFile(final string, data []byte, used map[int]bool, created func()) error {
	k := 1
	for k <= backup.MaxTemporaries && used[k] {
		k++
	}
	if k > backup.MaxTemporaries {
		return backup.ErrEnvelopeExhausted
	}
	used[k] = true
	temporary := fmt.Sprintf("%s.tmp-%d", final, k)
	file, err := o.vault.NewFile(temporary, time.Time{})
	if err != nil {
		return err
	}
	if created != nil {
		created()
	}
	_, err = file.Write(data)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return o.move(temporary, final)
}

// Retries of ADR 0006 §7: sharing and lock violations only, with an
// exponential backoff, at most five seconds in all.
const (
	retryFirst  = 50 * time.Millisecond
	retryBudget = 5 * time.Second
)

// move renames with the bounded retries of ADR 0006 §7.
func (o *operation) move(from, to string) error {
	ctx := o.ctx
	if o.critical {
		ctx = context.WithoutCancel(ctx)
	}
	start, delay := o.w.Clock.Now(), retryFirst
	for {
		err := o.vault.Move(from, to)
		var busy interface{ Busy() bool }
		if err == nil || !errors.As(err, &busy) || !busy.Busy() {
			return err
		}
		if o.w.Clock.Now().Sub(start)+delay > retryBudget {
			return fmt.Errorf("still in use after %s: %w; close the programs that use the vault", retryBudget, err)
		}
		if err := o.w.Clock.Sleep(ctx, delay); err != nil {
			return err
		}
		delay *= 2
	}
}

// fail closes the operation with a failed record when the error leaves
// current unchanged, and returns err. Anything that may have touched
// current is left to recovery.
func (o *operation) fail(err error) error {
	var intervention *InterventionError
	if errors.As(err, &intervention) || o.seq == 0 || o.critical || o.terminal {
		return err
	}
	reason := strings.ReplaceAll(err.Error(), "\n", " ")
	if len(reason) > 1000 {
		reason = reason[:1000]
	}
	if recordErr := o.record(backup.StepFailed, backup.StepValues{Text: reason}); recordErr != nil {
		return errors.Join(err, fmt.Errorf("the failure could not be recorded, operation %s needs recovery: %w", o.op, recordErr))
	}
	return err
}

// observe reads the vault again, through a new read-only scope that is
// closed before returning.
func (s *Service) observe(vaultPath, volume string, segments []string, gameID string) (*vault, error) {
	scope, err := s.files.OpenScope(volume)
	if err != nil {
		return nil, errVault(vaultPath, "open volume %s: %v", volume, err)
	}
	defer func() { _ = scope.Close() }()
	v, err := resolveVault(scope, vaultPath, segments)
	if err != nil {
		return nil, err
	}
	if err := v.read(scope, vaultPath, segments, gameID); err != nil {
		return nil, err
	}
	return v, nil
}
