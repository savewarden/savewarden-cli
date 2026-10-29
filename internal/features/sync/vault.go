package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/entities/game"
)

// Names of a game directory (ADR 0006, plan B2 §4.2).
const (
	gamesDir    = "games"
	lockFile    = "sync.lock"
	currentDir  = "current"
	retainedDir = "retained"
	stagingDir  = "staging"
	receiptsDir = "receipts"
	journalDir  = "journal"
)

// vault is what the plan read from the vault, without writing anything.
type vault struct {
	state         VaultState
	chain         []fs.FileInfo // descriptions from the volume root down to the vault, or its parent when absent
	exists        bool
	interventions []Intervention
	// Game directory content.
	games, game bool // games and games\<id> exist
	lock        bool // sync.lock exists
	current     bool // a plain current directory exists
	currentLink bool // current exists but is a link, a file or a reparse point
	receipts    []backup.Receipt
	invalid     []string // receipts that do not decode
	pending     []Pending
	// What recovery needs: the parsed journal of each operation, the
	// SHA-256 of its last record file, and the attempt numbers taken by
	// temporaries of the marker and of each receipt.
	journals     map[string]backup.Journal
	lastRecord   map[string]string
	rootTaken    map[int]bool
	receiptTaken map[string]map[int]bool
	receiptSum   map[string][32]byte // SHA-256 of each valid receipt file, by operation
}

func (v *vault) intervene(cause Cause, subject, detail string) {
	v.interventions = append(v.interventions, Intervention{Cause: cause, Subject: subject, Detail: detail})
}

// errVault reports a vault the plan must refuse outright: exit code 1.
func errVault(vaultPath, format string, args ...any) error {
	return fmt.Errorf("vault %s: "+format, append([]any{vaultPath}, args...)...)
}

// resolveVault checks every component of the vault path without following
// anything. The vault itself may be absent if its parent exists.
func resolveVault(scope Scope, vaultPath string, segments []string) (*vault, error) {
	v := &vault{exists: true}
	for k := 1; k <= len(segments); k++ {
		info, err := scope.Lstat(strings.Join(segments[:k], "/"))
		switch {
		case errors.Is(err, fs.ErrNotExist) && k == len(segments):
			v.exists, v.state = false, VaultAbsent
			return v, nil
		case errors.Is(err, fs.ErrNotExist):
			return nil, errVault(vaultPath, "parent directory %s does not exist", strings.Join(segments[:k], `\`))
		case err != nil:
			return nil, errVault(vaultPath, "%v", err)
		case isLink(info.Mode()):
			return nil, errVault(vaultPath, "%s is a link, junction or reparse point; not followed", strings.Join(segments[:k], `\`))
		case !info.IsDir():
			return nil, errVault(vaultPath, "%s is not a directory", strings.Join(segments[:k], `\`))
		}
		v.chain = append(v.chain, info)
	}
	return v, nil
}

// read classifies an existing vault and, when it is initialized, reads the
// marker, the game directory, its journals and receipts.
func (v *vault) read(scope Scope, vaultPath string, segments []string, gameID string) error {
	return v.readProbing(scope, vaultPath, segments, gameID, true)
}

// readProbing reads the vault; with probe, a pending operation whose lock
// is held by another process is reported as a running sync, code 1, rather
// than as an interrupted one. A run that holds the lock itself does not
// probe it.
func (v *vault) readProbing(scope Scope, vaultPath string, segments []string, gameID string, probe bool) error {
	if !v.exists {
		return nil
	}
	root := strings.Join(segments, "/")
	if root == "" {
		root = "."
	}
	entries, err := list(scope, root)
	if err != nil {
		return errVault(vaultPath, "%v", err)
	}
	for _, entry := range entries {
		if entry.Name == gamesDir && entry.Kind == backup.KindOther {
			return errVault(vaultPath, `%s\games is a link, junction or reparse point; not followed`, vaultPath)
		}
	}
	classified := backup.ClassifyVaultRoot(entries)
	v.rootTaken = classified.Taken
	switch classified.State {
	case backup.RootEmpty:
		v.state = VaultEmpty
		return nil
	case backup.RootForeign:
		return errVault(vaultPath, "not empty and not a Savewarden vault; choose a new or empty directory, an existing directory is never adopted")
	case backup.RootInterrupted:
		v.state = VaultInterrupted
		detail := "vault initialization was interrupted; only sync --recover --apply may resume it"
		if classified.Temporaries >= backup.MaxTemporaries {
			detail = backup.ErrEnvelopeExhausted.Error() + ": " + detail
		}
		v.intervene(CauseInitializationInterrupted, ".", detail)
		return nil
	}
	v.state = VaultInitialized
	data, err := readFile(scope, join(root, backup.MarkerName), markerReadLimit)
	if err != nil {
		return errVault(vaultPath, "%v", err)
	}
	if _, err := backup.DecodeMarker(data); err != nil {
		return errVault(vaultPath, "%v; a marker is never rewritten", err)
	}
	for _, name := range classified.Unexpected {
		v.intervene(CauseUnexpectedEntry, name, "has no place at the vault root")
	}
	if err := v.readGame(scope, vaultPath, root, gameID, hasEntry(entries, gamesDir)); err != nil {
		return err
	}
	if probe && v.lock && len(v.pending) > 0 {
		lock, err := scope.OpenRegular(join(root, gamesDir+"/"+gameID+"/"+lockFile))
		var busy interface{ Busy() bool }
		if errors.As(err, &busy) && busy.Busy() {
			return errVault(vaultPath, "another sync is running on this game; run it again once it ends")
		}
		if err == nil {
			_ = lock.Close()
		}
	}
	return nil
}

func (v *vault) readGame(scope Scope, vaultPath, root, gameID string, games bool) error {
	v.games = games
	if !games {
		return nil
	}
	gamesName := join(root, gamesDir)
	entries, err := list(scope, gamesName)
	if err != nil {
		return errVault(vaultPath, "%v", err)
	}
	found := false
	for _, entry := range entries {
		switch {
		case entry.Name == gameID && entry.Kind == backup.KindDirectory:
			found, v.game = true, true
		case entry.Name == gameID:
			return errVault(vaultPath, `games\%s is a link, a file or a reparse point; not followed`, gameID)
		case strings.EqualFold(entry.Name, gameID):
			// Windows would resolve games\<id> to this directory.
			v.intervene(CauseUnexpectedEntry, gamesDir+"/"+entry.Name, "differs from the game id only by case")
		case entry.Kind != backup.KindDirectory:
			v.intervene(CauseUnexpectedEntry, gamesDir+"/"+entry.Name, "has no place in games")
		}
	}
	if !found {
		return nil
	}
	gameName := join(gamesName, gameID)
	entries, err = list(scope, gameName)
	if err != nil {
		return errVault(vaultPath, "%v", err)
	}
	present := map[string]bool{}
	for _, entry := range entries {
		present[entry.Name] = true
		switch {
		case entry.Name == lockFile && entry.Kind == backup.KindRegular:
			v.lock = true
		case entry.Name == currentDir && entry.Kind == backup.KindDirectory:
			v.current = true
		case entry.Name == currentDir:
			v.currentLink = true
		case (entry.Name == retainedDir || entry.Name == stagingDir || entry.Name == receiptsDir || entry.Name == journalDir) &&
			entry.Kind == backup.KindDirectory:
		default:
			v.intervene(CauseUnexpectedEntry, gamesDir+"/"+gameID+"/"+entry.Name, "has no place in the game directory")
		}
	}
	prefix := gamesDir + "/" + gameID + "/"
	journals := map[string]bool{}
	if present[journalDir] {
		v.readJournals(scope, join(gameName, journalDir), prefix+journalDir, journals)
	}
	receipts := map[string]bool{}
	if present[receiptsDir] {
		if err := v.readReceipts(scope, vaultPath, join(gameName, receiptsDir), prefix+receiptsDir, gameID, receipts); err != nil {
			return err
		}
	}
	for dir, explained := range map[string]map[string]bool{stagingDir: journals, retainedDir: receipts} {
		if !present[dir] {
			continue
		}
		entries, err := list(scope, join(gameName, dir))
		if err != nil {
			return errVault(vaultPath, "%v", err)
		}
		for _, entry := range entries {
			if entry.Kind != backup.KindDirectory || !explained[entry.Name] {
				what := "journal"
				if dir == retainedDir {
					what = "receipt"
				}
				v.intervene(CauseUnexpectedEntry, prefix+dir+"/"+entry.Name, "is not explained by any "+what)
			}
		}
	}
	sort.Slice(v.interventions, func(a, b int) bool { return v.interventions[a].Subject < v.interventions[b].Subject })
	if len(v.pending) > 1 {
		v.intervene(CauseJournalAmbiguous, prefix+journalDir, fmt.Sprintf("%d operations are pending for this game; only one may be", len(v.pending)))
	}
	return nil
}

// readJournals validates every operation journal and records those that
// are not terminal. found receives the operations with a journal.
func (v *vault) readJournals(scope Scope, name, subject string, found map[string]bool) {
	entries, err := list(scope, name)
	if err != nil {
		// ADR 0006 §3: a journal that cannot be read is an ambiguity.
		v.intervene(CauseJournalAmbiguous, subject, "journal cannot be listed: "+err.Error())
		return
	}
	for _, entry := range entries {
		if entry.Kind != backup.KindDirectory || backup.ValidOperationID(entry.Name) != nil {
			v.intervene(CauseUnexpectedEntry, subject+"/"+entry.Name, "is not an operation journal")
			continue
		}
		found[entry.Name] = true
		journal, last, err := readJournal(scope, join(name, entry.Name), entry.Name)
		if err != nil {
			// Unreadable or inconsistent, the journal is ambiguous either way.
			v.intervene(CauseJournalAmbiguous, entry.Name, err.Error())
			continue
		}
		if v.journals == nil {
			v.journals, v.lastRecord = map[string]backup.Journal{}, map[string]string{}
		}
		v.journals[entry.Name], v.lastRecord[entry.Name] = journal, last
		if !journal.Terminal() {
			v.pending = append(v.pending, Pending{Operation: entry.Name, Last: journal.Last()})
			step := string(journal.Last())
			if step == "" {
				step = "none"
			}
			detail := fmt.Sprintf("last committed step %s; run sync --recover to see how it would be recovered", step)
			if journal.Last() == backup.StepPublishIntent && v.current {
				detail = "pending recovery: current published, receipt missing; run sync --recover"
			}
			v.intervene(CausePendingOperation, entry.Name, detail)
		}
	}
}

// readJournal parses one operation journal and returns the SHA-256 of its
// last record file, empty if none.
func readJournal(scope Scope, name, op string) (backup.Journal, string, error) {
	entries, err := list(scope, name)
	if err != nil {
		return backup.Journal{}, "", err
	}
	names := make([]string, 0, len(entries))
	contents := map[string][]byte{}
	for _, entry := range entries {
		if entry.Kind != backup.KindRegular {
			return backup.Journal{}, "", &backup.AmbiguityError{Op: op, Reason: fmt.Sprintf("entry %q is not a regular file", entry.Name)}
		}
		names = append(names, entry.Name)
		if strings.HasSuffix(entry.Name, ".rec") {
			data, err := readFile(scope, join(name, entry.Name), recordReadLimit)
			if err != nil {
				return backup.Journal{}, "", err
			}
			contents[entry.Name] = data
		}
	}
	journal, err := backup.ParseJournal(op, names, contents)
	if err != nil || len(journal.Records) == 0 {
		return journal, "", err
	}
	last := journal.Records[len(journal.Records)-1]
	sum := sha256.Sum256(contents[backup.RecordName(last.Seq, last.Step)])
	return journal, hex.EncodeToString(sum[:]), nil
}

var receiptName = regexp.MustCompile(`^([0-9]{8}T[0-9]{6}-[0-9a-f]{8})\.json(\.tmp-[1-9])?$`)

// readReceipts decodes every committed receipt; temporaries are ignored,
// never read as receipts. found receives the operations with a receipt.
func (v *vault) readReceipts(scope Scope, vaultPath, name, subject, gameID string, found map[string]bool) error {
	entries, err := list(scope, name)
	if err != nil {
		return errVault(vaultPath, "%v", err)
	}
	for _, entry := range entries {
		m := receiptName.FindStringSubmatch(entry.Name)
		switch {
		case m == nil || entry.Kind != backup.KindRegular:
			v.intervene(CauseUnexpectedEntry, subject+"/"+entry.Name, "is not a receipt")
			continue
		case m[2] != "":
			if v.receiptTaken == nil {
				v.receiptTaken = map[string]map[int]bool{}
			}
			if v.receiptTaken[m[1]] == nil {
				v.receiptTaken[m[1]] = map[int]bool{}
			}
			v.receiptTaken[m[1]][int(m[2][len(m[2])-1]-'0')] = true
			continue
		}
		data, err := readFile(scope, join(name, entry.Name), recordReadLimit)
		if err != nil {
			return errVault(vaultPath, "%v", err)
		}
		r, err := backup.DecodeReceipt(data)
		if err != nil || r.Operation != m[1] || r.GameID != gameID {
			v.invalid = append(v.invalid, entry.Name)
			continue
		}
		found[r.Operation] = true
		v.receipts = append(v.receipts, r)
		if v.receiptSum == nil {
			v.receiptSum = map[string][32]byte{}
		}
		v.receiptSum[r.Operation] = sha256.Sum256(data)
	}
	return nil
}

// head returns the receipt of the current generation: the only valid
// receipt that no other one names as previous.
func (v *vault) head() (backup.Receipt, error) {
	if len(v.invalid) > 0 {
		return backup.Receipt{}, fmt.Errorf("receipt %s is not valid", v.invalid[0])
	}
	replaced := map[string]bool{}
	for _, r := range v.receipts {
		replaced[r.Previous] = true
	}
	var heads []backup.Receipt
	for _, r := range v.receipts {
		if !replaced[r.Operation] {
			heads = append(heads, r)
		}
	}
	switch len(heads) {
	case 0:
		return backup.Receipt{}, errors.New("no receipt describes current")
	case 1:
		return heads[0], nil
	}
	return backup.Receipt{}, fmt.Errorf("%d receipts each claim to describe current", len(heads))
}

// list reads a directory and describes each entry without following it.
func list(scope Scope, name string) ([]backup.NamedEntry, error) {
	dirEntries, err := scope.ReadDir(name)
	if err != nil {
		return nil, err
	}
	entries := make([]backup.NamedEntry, 0, len(dirEntries))
	for _, entry := range dirEntries {
		info, err := scope.Lstat(join(name, entry.Name()))
		if err != nil {
			return nil, err
		}
		kind := backup.KindOther
		switch {
		case isLink(info.Mode()):
		case info.IsDir():
			kind = backup.KindDirectory
		case info.Mode().IsRegular():
			kind = backup.KindRegular
		}
		entries = append(entries, backup.NamedEntry{Name: entry.Name(), Kind: kind})
	}
	sort.Slice(entries, func(a, b int) bool { return entries[a].Name < entries[b].Name })
	return entries, nil
}

func hasEntry(entries []backup.NamedEntry, name string) bool {
	for _, entry := range entries {
		if entry.Name == name {
			return true
		}
	}
	return false
}

// readFile reads a regular file of at most limit bytes through the scope.
func readFile(scope Scope, name string, limit int64) ([]byte, error) {
	file, err := scope.OpenRegular(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("read %s: larger than %d bytes", name, limit)
	}
	return data, nil
}

// join builds a scope name below dir, "." being the anchor.
func join(dir, name string) string {
	if dir == "." {
		return name
	}
	return path.Join(dir, name)
}

func isLink(mode fs.FileMode) bool { return mode&(fs.ModeSymlink|fs.ModeIrregular) != 0 }

// chainOf describes, without following anything, the existing components
// from the volume root down to segments. complete reports whether the last
// segment itself exists as a directory.
func chainOf(scope Scope, segments []string) (chain []fs.FileInfo, complete bool) {
	for k := 1; k <= len(segments); k++ {
		info, err := scope.Lstat(strings.Join(segments[:k], "/"))
		if err != nil || isLink(info.Mode()) || !info.IsDir() {
			return chain, false
		}
		chain = append(chain, info)
	}
	return chain, true
}

// overlapping reports whether the directory ending a complete chain is one
// of the components of the other, in either direction.
func overlapping(scope Scope, a []fs.FileInfo, aComplete bool, b []fs.FileInfo, bComplete bool) bool {
	contains := func(chain []fs.FileInfo, complete bool, other []fs.FileInfo) bool {
		if !complete || len(chain) == 0 {
			return false
		}
		for _, info := range other {
			if scope.SameFile(chain[len(chain)-1], info) {
				return true
			}
		}
		return false
	}
	return contains(a, aComplete, b) || contains(b, bComplete, a)
}

// distinctByName refuses a vault that coincides with, contains or lies in
// the installation, compared segment by segment without regard to case.
func distinctByName(vaultPath, vaultVolume string, vaultSegments []string, installVolume string, installSegments []string) error {
	if !strings.EqualFold(vaultVolume, installVolume) {
		return nil
	}
	if game.SameSegments(vaultSegments, installSegments) || game.SameSegments(installSegments, vaultSegments) {
		return errVault(vaultPath, "overlaps the installation; the vault must be distinct from the installation and every source")
	}
	return nil
}
