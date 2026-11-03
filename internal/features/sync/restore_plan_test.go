package sync_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/entities/game"
	"github.com/savewarden/savewarden-cli/internal/features/sync"
)

const (
	restoreDir = `G:\Restores\synthetic`
	restored1  = "20261107T090000-9a8b7c6d"
	restored2  = "20261107T100000-0b1c2d3e"
	snapDir    = gameDir + `\snapshots\` + snapApplied
)

var restoreAt = snapAt.Add(24 * time.Hour)

// restorable leaves in the vault on F:\ an intact approved snapshot,
// snapApplied, taken by a real snapshot of a real generation, and an empty
// G:\ volume with a Restores directory.
func restorable(t *testing.T) *fakeFS {
	t.Helper()
	f := generation(t)
	if result, err := snapshotApply(f, newFakeVaults(f)); err != nil || result.Effect != sync.EffectPublished {
		t.Fatalf("snapshot: %+v, %v", result, err)
	}
	withRestoreVolume(f)
	return f
}

func withRestoreVolume(f *fakeFS) {
	f.volumes[`G:\`] = f.dir()
	f.mkdir(`G:\Restores`)
}

func restoreRequest(destination string) sync.RestoreRequest {
	return sync.RestoreRequest{ManifestPath: "m.yaml", VaultPath: vaultPath, Snapshot: snapApplied, Destination: destination}
}

func restorePlanWith(f *fakeFS, m game.Manifest, request sync.RestoreRequest) (sync.RestorePlan, error) {
	return service(f, m).RestorePlan(context.Background(), request)
}

func restorePlanOf(t *testing.T, f *fakeFS) sync.RestorePlan {
	t.Helper()
	p, err := restorePlanWith(f, manifest(), restoreRequest(restoreDir))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func restoreError(f *fakeFS, destination string) error {
	_, err := restorePlanWith(f, manifest(), restoreRequest(destination))
	return err
}

func restoreFindings(p sync.RestorePlan) []string {
	var out []string
	for _, finding := range p.Findings {
		out = append(out, finding.Severity.String()+" "+string(finding.Code)+" "+finding.Path)
	}
	return out
}

func restoreCauses(p sync.RestorePlan) []string {
	var out []string
	for _, i := range p.Interventions {
		out = append(out, string(i.Cause)+" "+i.Subject)
	}
	return out
}

// everything lists every node of every volume, so that a test can prove
// nothing was written anywhere.
func everything(f *fakeFS) string {
	var roots []string
	for root := range f.volumes {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	var parts []string
	for _, root := range roots {
		parts = append(parts, snapshot(f, root))
	}
	return strings.Join(parts, "\n")
}

func expectRestoreRefused(t *testing.T, err error, mentions ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a refusal, code 1")
	}
	var blocked *sync.BlockedError
	var intervention *sync.InterventionError
	if errors.As(err, &blocked) || errors.As(err, &intervention) {
		t.Fatalf("a refusal is code 1, got %v", err)
	}
	for _, mention := range mentions {
		if !strings.Contains(err.Error(), mention) {
			t.Fatalf("%q does not mention %q", err, mention)
		}
	}
}

// markArea initializes a restore area of gameID as step Z4 of ADR 0006 §11
// leaves it, with its structure.
func markArea(t *testing.T, f *fakeFS, area, gameID string) {
	t.Helper()
	data, err := backup.EncodeRestoreAreaMarker(backup.RestoreAreaMarker{CreatedAt: restoreAt, ToolVersion: "dev", GameID: gameID})
	if err != nil {
		t.Fatal(err)
	}
	f.mkdir(area)
	if f.get(area+`\`+backup.RestoreAreaMarkerName) == nil {
		f.put(area+`\`+backup.RestoreAreaMarkerName, f.file(string(data)))
	}
	f.mkdir(area + `\receipts`)
	f.mkdir(area + `\journal`)
}

// restoreInputs are the values a restore of snapApplied records in its
// planned step, and the receipt bytes of that snapshot.
type restoreInputs struct {
	plan     *backup.RestorePlan
	planSum  [32]byte
	snapshot backup.SnapshotSource
	identity backup.FileID
}

func inputsOf(t *testing.T, f *fakeFS, start time.Time) restoreInputs {
	t.Helper()
	data := f.get(gameDir + `\snapshots\receipts\` + snapApplied + ".json").data
	sr, err := backup.DecodeSnapshotReceipt(data)
	if err != nil {
		t.Fatal(err)
	}
	plan := &backup.RestorePlan{ToolVersion: "dev", GameID: sr.GameID, GameName: sr.GameName, StartedAt: start, Source: sr.Source, Sources: sr.Sources}
	_, planSum, err := plan.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return restoreInputs{plan: plan, planSum: planSum, snapshot: backup.SnapshotSource{Operation: snapApplied, ReceiptSHA256: sha(data)},
		identity: identityOf(f.get(snapDir))}
}

// writeAreaRecords commits the records of restore operation op in area,
// chained and one second apart from start.
func writeAreaRecords(t *testing.T, f *fakeFS, area, op string, start time.Time, steps []backup.Step, values func(backup.Step) backup.StepValues) {
	t.Helper()
	dir := f.mkdir(area + `\journal\` + op)
	prev := ""
	for i, step := range steps {
		r, err := backup.NewRestoreRecord(i+1, step, op, start.Add(time.Duration(i)*time.Second), prev, values(step))
		if err != nil {
			t.Fatalf("record %s: %v", step, err)
		}
		data, err := backup.EncodeRecord(r)
		if err != nil {
			t.Fatal(err)
		}
		dir.children[backup.RecordName(i+1, step)] = f.file(string(data))
		sum := sha256.Sum256(data)
		prev = hex.EncodeToString(sum[:])
	}
}

// publishRestore lays out what a published restore op of snapApplied leaves
// in area: the area itself if needed, a copy of the snapshot with its own
// identity, its receipt and a journal terminal in published. It returns the
// copy.
func publishRestore(t *testing.T, f *fakeFS, area, op string, start time.Time) *node {
	t.Helper()
	markArea(t, f, area, "synthetic-game")
	in := inputsOf(t, f, start)
	copied := copyTree(f, f.get(snapDir))
	f.get(area).children[op] = copied
	content, err := backup.ContentDigest(in.plan.Sources)
	if err != nil {
		t.Fatal(err)
	}
	published := start.Add(5 * time.Second)
	receipt, err := in.plan.Receipt(op, in.planSum, in.snapshot, start.Add(4*time.Second), &published, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	data, err := backup.EncodeRestoreReceipt(receipt)
	if err != nil {
		t.Fatal(err)
	}
	f.put(area+`\receipts\`+op+".json", f.file(string(data)))
	steps := []backup.Step{backup.StepPlanned, backup.StepStagingIntent, backup.StepStaged, backup.StepVerified, backup.StepPublishIntent, backup.StepPublished}
	writeAreaRecords(t, f, area, op, start, steps, func(step backup.Step) backup.StepValues {
		switch step {
		case backup.StepPlanned:
			return backup.StepValues{RestoredSnapshot: in.snapshot, SnapshotIdentity: in.identity, RestorePlan: in.plan}
		case backup.StepStaged:
			return backup.StepValues{Staging: identityOf(copied), Digest: content}
		case backup.StepVerified:
			return backup.StepValues{Digest: content}
		case backup.StepPublishIntent:
			return backup.StepValues{Staging: identityOf(copied)}
		case backup.StepPublished:
			return backup.StepValues{Digest: sha(data)}
		}
		return backup.StepValues{}
	})
	return copied
}

// startRestore leaves restore op of snapApplied pending in area, after the
// given steps.
func startRestore(t *testing.T, f *fakeFS, area, op string, start time.Time, steps ...backup.Step) {
	t.Helper()
	markArea(t, f, area, "synthetic-game")
	in := inputsOf(t, f, start)
	writeAreaRecords(t, f, area, op, start, steps, func(step backup.Step) backup.StepValues {
		switch step {
		case backup.StepPlanned:
			return backup.StepValues{RestoredSnapshot: in.snapshot, SnapshotIdentity: in.identity, RestorePlan: in.plan}
		case backup.StepFailed, backup.StepAbandoned:
			return backup.StepValues{Text: "synthetic"}
		}
		return backup.StepValues{}
	})
}

// ADR 0009: the plan of a new area authenticates the snapshot in full and
// writes nothing, anywhere; nor does the plan of an empty directory.
func TestRestorePlanWritesNothing(t *testing.T) {
	f := restorable(t)
	before := everything(f)
	p := restorePlanOf(t, f)
	if !p.Verified || p.Verdict() != "verified" || p.From != op1 || p.State != sync.DestinationNew || p.Outcome() != "new restore" {
		t.Fatalf("plan: %+v", p)
	}
	if p.Files != 2 || p.Bytes != 6 || p.Needed != 6+64<<20 || len(p.Findings) != 0 || len(p.Interventions) != 0 || len(p.Restores) != 0 {
		t.Fatalf("plan: %d files, %d bytes, %d needed, %q, %q", p.Files, p.Bytes, p.Needed, restoreFindings(p), restoreCauses(p))
	}
	if p.Volume.FileSystem != "NTFS" || p.Game.ID != "synthetic-game" || p.Vault != vaultPath || p.Destination != restoreDir {
		t.Fatalf("plan header: %+v", p)
	}
	if everything(f) != before {
		t.Fatal("the plan wrote something")
	}
	f.mkdir(restoreDir)
	before = everything(f)
	if p := restorePlanOf(t, f); p.State != sync.DestinationEmpty || p.Outcome() != "new restore" {
		t.Fatalf("empty directory: %s %s", p.State, p.Outcome())
	}
	if everything(f) != before {
		t.Fatal("the plan wrote something")
	}
	again := restorePlanOf(t, f)
	if again.Outcome() != p.Outcome() || again.Needed != p.Needed {
		t.Fatal("two plans of an unchanged state differ")
	}
}

// ADR 0009, table 1 and 2: a snapshot that is unknown, not published, still
// pending, or whose journal is ambiguous is not restorable: code 1, never a
// blocking finding, whether sync.lock is held or not; sync.lock is never
// observed.
func TestRestoreRequiresAPublishedSnapshot(t *testing.T) {
	f := restorable(t)
	_, err := restorePlanWith(f, manifest(), sync.RestoreRequest{ManifestPath: "m.yaml", VaultPath: vaultPath,
		Snapshot: "20261101T000000-00000000", Destination: restoreDir})
	expectRestoreRefused(t, err, "snapshot 20261101T000000-00000000 not found in the vault "+vaultPath)
	_, err = restorePlanWith(f, manifest(), sync.RestoreRequest{ManifestPath: "m.yaml", VaultPath: vaultPath, Snapshot: "latest", Destination: restoreDir})
	expectRestoreRefused(t, err, "snapshot:")

	for _, held := range []bool{false, true} {
		pending := interruptedAt(t, snapCommit(2, "staging-intent"), true)
		withRestoreVolume(pending)
		if held {
			pending.put(gameDir+`\sync.lock`, pending.file("")).busy = true
		}
		pending.reads = nil
		before := everything(pending)
		expectRestoreRefused(t, restoreError(pending, restoreDir), "snapshot "+snapApplied+" is not restorable", "pending", "last committed step staging-intent",
			"snapshot --recover")
		for _, read := range pending.reads {
			if strings.Contains(read, "sync.lock") {
				t.Fatalf("sync.lock observed: %s", read)
			}
		}
		if everything(pending) != before {
			t.Fatal("the plan wrote something")
		}
		if held {
			pending.get(gameDir + `\sync.lock`).busy = false
		}
		if _, err := snapshotRecoverWith(pending, newFakeVaults(pending)); err != nil {
			t.Fatal(err)
		}
		expectRestoreRefused(t, restoreError(pending, restoreDir), "snapshot "+snapApplied+" was not published", "abandoned")
	}

	ambiguous := restorable(t)
	ambiguous.put(gameDir+`\snapshots\journal\`+snapApplied+`\notes.txt`, ambiguous.file("x"))
	expectRestoreRefused(t, restoreError(ambiguous, restoreDir), "is not restorable: its journal is ambiguous", "snapshot --recover")
}

// A receipt rebuilt by snapshot recovery, published_at null, is valid.
func TestRestoreAcceptsARebuiltSnapshotReceipt(t *testing.T) {
	f := interruptedAt(t, snapMove, true)
	if _, err := snapshotRecoverWith(f, newFakeVaults(f)); err != nil {
		t.Fatal(err)
	}
	r, err := backup.DecodeSnapshotReceipt(f.get(gameDir + `\snapshots\receipts\` + snapApplied + ".json").data)
	if err != nil || r.PublishedAt != nil || r.RecoveredAt == nil {
		t.Fatalf("receipt not rebuilt: %+v, %v", r, err)
	}
	withRestoreVolume(f)
	if p := restorePlanOf(t, f); !p.Verified || p.Outcome() != "new restore" {
		t.Fatalf("plan: %s %q", p.Verdict(), restoreFindings(p))
	}
}

// ADR 0009, tables 3 to 5: a degraded snapshot is a blocking finding,
// code 2, never read when its identity differs; nothing is written.
func TestRestoreRefusesADegradedSnapshot(t *testing.T) {
	snaps := gameDir + `\snapshots`
	receipt := snaps + `\receipts\` + snapApplied + ".json"
	tests := []struct {
		name   string
		change func(f *fakeFS)
		code   backup.Code
	}{
		{"missing", func(f *fakeFS) { delete(f.get(snaps).children, snapApplied) }, sync.CodeSnapshotMissing},
		{"rename lost", func(f *fakeFS) {
			f.get(snaps).children[snapApplied+".incomplete"] = f.get(snapDir)
			delete(f.get(snaps).children, snapApplied)
		}, sync.CodeSnapshotRenameLost},
		{"other identity", func(f *fakeFS) { f.get(snaps).children[snapApplied] = copyTree(f, f.get(snapDir)) }, sync.CodeSnapshotModified},
		{"incomplete beside", func(f *fakeFS) { f.mkdir(snapDir + ".incomplete") }, sync.CodeSnapshotModified},
		{"a file", func(f *fakeFS) { f.get(snaps).children[snapApplied] = f.file("x") }, sync.CodeSnapshotModified},
		{"receipt absent", func(f *fakeFS) { delete(f.get(snaps+`\receipts`).children, snapApplied+".json") }, sync.CodeSnapshotModified},
		{"receipts absent", func(f *fakeFS) { delete(f.get(snaps).children, "receipts") }, sync.CodeSnapshotModified},
		{"receipt invalid", func(f *fakeFS) { f.get(receipt).data = []byte("{}") }, sync.CodeSnapshotModified},
		{"receipt differs", func(f *fakeFS) { f.get(receipt).data = append(f.get(receipt).data, ' ') }, sync.CodeSnapshotModified},
		{"receipt a directory", func(f *fakeFS) { f.get(snaps + `\receipts`).children[snapApplied+".json"] = f.dir() }, sync.CodeSnapshotModified},
		{"content differs", func(f *fakeFS) { f.get(snapDir + `\saves\1.save`).data = []byte("ONE") }, sync.CodeSnapshotModified},
		{"file added", func(f *fakeFS) { f.put(snapDir+`\saves\extra.save`, f.file("x")) }, sync.CodeSnapshotModified},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := restorable(t)
			tt.change(f)
			f.opened = 0
			before := everything(f)
			p := restorePlanOf(t, f)
			if p.Verified || p.Outcome() != "blocked" || p.Blocking() != 1 || !slices.Equal(restoreFindings(p), []string{"blocking " + string(tt.code) + " snapshots/" + snapApplied}) {
				t.Fatalf("plan: %s, %q", p.Verdict(), restoreFindings(p))
			}
			if p.Files != 0 || p.Needed != 0 || !strings.HasSuffix(p.Verdict(), ", not restorable") {
				t.Fatalf("a degraded snapshot is not sized: %d files, %d needed, %q", p.Files, p.Needed, p.Verdict())
			}
			if everything(f) != before {
				t.Fatal("the plan wrote something")
			}
		})
	}
	// An object of another identity is never read.
	f := restorable(t)
	f.get(gameDir + `\snapshots`).children[snapApplied] = copyTree(f, f.get(snapDir))
	f.reads = nil
	restorePlanOf(t, f)
	for _, read := range f.reads {
		if strings.HasPrefix(read, "OpenRegular "+snapDir+`\`) || strings.HasPrefix(read, "ReadDir "+snapDir) {
			t.Fatalf("a snapshot of another identity was read: %s", read)
		}
	}
	// A link in the chain of the snapshot domain is a refusal.
	f = restorable(t)
	f.get(gameDir + `\snapshots\receipts`).mode = fs.ModeIrregular | fs.ModeDir
	expectRestoreRefused(t, restoreError(f, restoreDir), "is a link, junction or reparse point; not followed")
	f = restorable(t)
	f.get(gameDir).mode = fs.ModeIrregular | fs.ModeDir
	expectRestoreRefused(t, restoreError(f, restoreDir), "not followed")
}

// ADR 0009: a snapshot that cannot be read is not verifiable, code 1,
// without any degradation code.
func TestUnreadableSnapshotIsNotDegraded(t *testing.T) {
	tests := map[string]func(f *fakeFS){
		"file held":    func(f *fakeFS) { f.get(snapDir + `\saves\1.save`).busy = true },
		"identity":     func(f *fakeFS) { f.get(snapDir).busy = true },
		"receipt held": func(f *fakeFS) { f.get(gameDir + `\snapshots\receipts\` + snapApplied + ".json").busy = true },
		"journal unlisted": func(f *fakeFS) {
			f.get(gameDir + `\snapshots\journal\` + snapApplied).listErr = errors.New("access denied")
		},
		"chain": func(f *fakeFS) { f.get(gameDir + `\snapshots`).statErr = errors.New("access denied") },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			f := restorable(t)
			change(f)
			expectRestoreRefused(t, restoreError(f, restoreDir), "snapshot "+snapApplied+" not verifiable")
		})
	}
}

// ADR 0009, table 2: the vault is read for the designated snapshot only:
// nothing of current, retained, staging, the receipts or the journal of
// the game, nor of another snapshot; games\<game-id> is never listed, and
// sync.lock never observed. The installation and the sources are read for
// their identities only.
func TestRestoreReadsOnlyTheSelectedSnapshot(t *testing.T) {
	f := restorable(t)
	publishSnapshot(t, f, snapOp2, snapAt.Add(-time.Hour))
	f.put(gameDir+`\sync.lock`, f.file(""))
	f.reads = nil
	if p := restorePlanOf(t, f); !p.Verified {
		t.Fatalf("plan: %s", p.Verdict())
	}
	ancestors := []string{`F:\`, vaultPath, vaultPath + `\games`, gameDir, gameDir + `\snapshots`, gameDir + `\snapshots\journal`, gameDir + `\snapshots\receipts`}
	journal := gameDir + `\snapshots\journal\` + snapApplied
	for _, read := range f.reads {
		call, path, _ := strings.Cut(read, " ")
		switch {
		case strings.HasPrefix(path, `G:\`):
		case strings.HasPrefix(path, `D:\`):
			if call != "Lstat" && call != "Identity" {
				t.Errorf("the installation or a source was read: %s", read)
			}
		case slices.Contains(ancestors, path):
			if call == "ReadDir" || call == "OpenRegular" {
				t.Errorf("an ancestor of the snapshot was listed: %s", read)
			}
		case path == vaultPath+`\savewarden-vault.json`, path == journal, strings.HasPrefix(path, journal+`\`),
			path == gameDir+`\snapshots\receipts\`+snapApplied+".json", path == snapDir, strings.HasPrefix(path, snapDir+`\`):
		case path == snapDir+".incomplete":
			if call != "Lstat" {
				t.Errorf("the incomplete name was more than observed: %s", read)
			}
		default:
			t.Errorf("read outside the snapshot: %s", read)
		}
	}
}

// Anomalies of the vault outside the designated snapshot change nothing
// for its restore.
func TestRestoreIgnoresOtherVaultAnomalies(t *testing.T) {
	tests := map[string]func(f *fakeFS){
		"current absent":    func(f *fakeFS) { delete(f.get(gameDir).children, "current") },
		"current modified":  func(f *fakeFS) { f.get(gameDir + `\current\saves\1.save`).data = []byte("ONE") },
		"current journal":   func(f *fakeFS) { f.put(gameDir+`\journal\`+op1+`\notes.txt`, f.file("x")) },
		"unexpected entry":  func(f *fakeFS) { f.put(gameDir+`\notes.txt`, f.file("x")) },
		"pending current":   func(f *fakeFS) { writeJournal(t, f, op2, backup.StepPlanned) },
		"other snapshot":    func(f *fakeFS) { f.mkdir(gameDir + `\snapshots\` + snapOp2) },
		"other receipt":     func(f *fakeFS) { f.put(gameDir+`\snapshots\receipts\`+snapOp2+".json", f.file("{}")) },
		"snapshot pending":  func(f *fakeFS) { f.mkdir(gameDir + `\snapshots\journal\` + snapOp2) },
		"sync.lock held":    func(f *fakeFS) { f.put(gameDir+`\sync.lock`, f.file("")).busy = true },
		"vault root extra":  func(f *fakeFS) { f.put(vaultPath+`\notes.txt`, f.file("x")) },
		"staging leftovers": func(f *fakeFS) { f.mkdir(gameDir + `\staging\` + op2) },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			f := restorable(t)
			change(f)
			p := restorePlanOf(t, f)
			if !p.Verified || p.Outcome() != "new restore" || len(p.Findings) != 0 || len(p.Interventions) != 0 {
				t.Fatalf("plan: %s %s %q %q", p.Verdict(), p.Outcome(), restoreFindings(p), restoreCauses(p))
			}
		})
	}
}

// ADR 0009: the destination is absent with a plain parent, an empty
// directory, or an area of the same game; anything else is refused, code 1,
// and nothing is written.
func TestRestoreDestinationIsNewEmptyOrArea(t *testing.T) {
	tests := []struct {
		name        string
		destination string
		change      func(f *fakeFS)
		mention     string
	}{
		{"not empty", restoreDir, func(f *fakeFS) { f.put(restoreDir+`\notes.txt`, f.file("x")) }, "is not empty and not a restore area"},
		{"lock without marker", restoreDir, func(f *fakeFS) { f.put(restoreDir+`\restore.lock`, f.file("")) }, "is not empty and not a restore area"},
		{"file", restoreDir, func(f *fakeFS) { f.put(restoreDir, f.file("x")) }, "is a link, a file or a reparse point"},
		{"link", restoreDir, func(f *fakeFS) { f.put(restoreDir, f.link()) }, "is a link, a file or a reparse point"},
		{"the vault", vaultPath, func(*fakeFS) {}, "is not empty and not a restore area"},
		{"vault marker", restoreDir, func(f *fakeFS) { markVault(t, f, restoreDir) }, "is not empty and not a restore area"},
		{"other game", restoreDir, func(f *fakeFS) { markArea(t, f, restoreDir, "other-game") }, `is a restore area of game "other-game"`},
		{"marker invalid", restoreDir, func(f *fakeFS) {
			markArea(t, f, restoreDir, "synthetic-game")
			f.get(restoreDir + `\savewarden-restore.json`).data = []byte(`{"schema":"savewarden.vault","schema_version":1}`)
		}, "a marker is never rewritten"},
		{"marker a directory", restoreDir, func(f *fakeFS) { f.mkdir(restoreDir + `\savewarden-restore.json`) }, "is not empty and not a restore area"},
		{"parent missing", `G:\Missing\synthetic`, func(*fakeFS) {}, `parent directory G:\Missing does not exist`},
		{"parent a link", `G:\Linked\synthetic`, func(f *fakeFS) { f.put(`G:\Linked`, f.link()) }, `parent directory G:\Linked`},
		{"no volume", `K:\Restores\synthetic`, func(*fakeFS) {}, `volume K:\ is not available`},
		{"relative", `Restores\synthetic`, func(*fakeFS) {}, `destination Restores\synthetic:`},
		{"volume root", `G:\`, func(*fakeFS) {}, "is the root of a volume"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := restorable(t)
			tt.change(f)
			before := everything(f)
			expectRestoreRefused(t, restoreError(f, tt.destination), "destination ", tt.mention)
			if everything(f) != before {
				t.Fatal("the plan wrote something")
			}
		})
	}
}

// ADR 0006 §11: a restore never lands in or around the vault, the
// installation or a source, by name or by identity, nor under a vault or a
// restore area.
func TestRestoreDistinction(t *testing.T) {
	tests := []struct {
		name        string
		destination string
		change      func(f *fakeFS)
		mention     string
	}{
		{"in the vault", vaultPath + `\restores`, func(*fakeFS) {}, `lies in a Savewarden vault or restore area, F:\Vault`},
		{"in the game of the vault", gameDir + `\restore`, func(*fakeFS) {}, "lies in a Savewarden vault"},
		{"in a source", installPath + `\saves\restore`, func(*fakeFS) {}, "overlaps the installation " + installPath},
		{"in an area", restoreDir + `\inner`, func(f *fakeFS) { markArea(t, f, restoreDir, "synthetic-game") }, `lies in a Savewarden vault or restore area, G:\Restores\synthetic`},
		{"under a vault marker", `G:\Restores\inner`, func(f *fakeFS) { markVault(t, f, `G:\Restores`) }, `restore area, G:\Restores`},
		{"under a marked root", `G:\Restores\inner`, func(f *fakeFS) { f.volumes[`G:\`].children[backup.MarkerName] = f.file("{}") }, `restore area, G:\;`},
		{"by a short name", `D:\Games\SYNTHE~1\saves\restore`, func(f *fakeFS) {
			f.get(`D:\Games`).children["SYNTHE~1"] = f.get(installPath)
		}, "overlaps the installation"},
		{"beside the vault", `F:\Outer`, func(f *fakeFS) { f.mkdir(`F:\Outer`) }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := restorable(t)
			tt.change(f)
			before := everything(f)
			err := restoreError(f, tt.destination)
			if tt.mention == "" {
				if err != nil {
					t.Fatalf("an empty directory beside the vault is distinct: %v", err)
				}
				return
			}
			expectRestoreRefused(t, err, tt.mention)
			if everything(f) != before {
				t.Fatal("the plan wrote something")
			}
		})
	}
	// The destination contains a reference, by identity: an empty
	// directory that is the directory of an absent source.
	f := restorable(t)
	f.mkdir(installPath + `\profile`)
	f.mkdir(`G:\Alias`).children["profile"] = f.get(installPath + `\profile`)
	m := manifest(source("saves", "saves", true), source("profile", `profile\sub`, false))
	_, err := restorePlanWith(f, m, restoreRequest(`G:\Alias\profile`))
	expectRestoreRefused(t, err, "overlaps the source profile "+installPath+`\profile\sub`)
}

// ADR 0006 §11, observability rule: two letters naming the same nodes,
// like a subst alias, never let a restore land in a source or the vault.
func TestRestoreDistinctionAcrossDriveLetters(t *testing.T) {
	f := restorable(t)
	f.volumes[`H:\`] = f.get(installPath)
	expectRestoreRefused(t, restoreError(f, `H:\saves\restore`), "overlaps the installation "+installPath)
	expectRestoreRefused(t, restoreError(f, `H:\restore`), "overlaps the installation "+installPath)
	f.volumes[`V:\`] = f.get(vaultPath)
	expectRestoreRefused(t, restoreError(f, `V:\games\restore`), `lies in a Savewarden vault or restore area, V:\`)
	// The reverse: the vault named through the alias of its volume.
	f.volumes[`W:\`] = f.volumes[`F:\`]
	_, err := restorePlanWith(f, manifest(), sync.RestoreRequest{ManifestPath: "m.yaml", VaultPath: `W:\Vault`, Snapshot: snapApplied, Destination: vaultPath + `\inner`})
	expectRestoreRefused(t, err, `lies in a Savewarden vault or restore area, F:\Vault`)
}

// ADR 0006 §11, observability rule: a reference on a letter without a
// volume, or whose chain cannot be observed, never proves separation; a
// reference that resolves to nothing now is decided by the chain that
// exists and by its names below the root, on any letter.
func TestRestoreDistinctionWithUnavailableReferences(t *testing.T) {
	moved := func(path string) game.Manifest {
		m := manifest()
		m.Installation.Path = path
		return m
	}
	tests := []struct {
		name        string
		m           game.Manifest
		destination string
		change      func(f *fakeFS)
		mention     string
	}{
		{"letter without volume", moved(`E:\Games\Synthetic Game`), restoreDir, func(*fakeFS) {},
			`separation from the installation E:\Games\Synthetic Game cannot be proven: its drive has no volume now`},
		{"link on the chain", manifest(), restoreDir, func(f *fakeFS) { f.get(`D:\Games`).mode = fs.ModeIrregular | fs.ModeDir },
			"separation from the installation " + installPath + " cannot be proven"},
		{"identity unreadable", manifest(), restoreDir, func(f *fakeFS) { f.get(installPath).busy = true },
			"separation from the installation " + installPath + " cannot be proven"},
		{"chain unreadable", manifest(), restoreDir, func(f *fakeFS) { f.get(`D:\Games`).statErr = errors.New("access denied") },
			"cannot be proven: access denied"},
		{"root unreadable", manifest(), restoreDir, func(f *fakeFS) { f.volumes[`D:\`].busy = true }, "cannot be proven"},
		{"destination unreadable", manifest(), restoreDir, func(f *fakeFS) { f.get(`G:\Restores`).busy = true }, "cannot be checked"},
		{"uninstalled, letter taken by another disk", moved(`D:\Old\Synthetic Game`), `G:\Old\Synthetic Game\restore`,
			func(f *fakeFS) { f.mkdir(`G:\Old\Synthetic Game`) }, "overlaps the installation"},
		{"uninstalled, named below", moved(`D:\Old\Synthetic Game`), `D:\Old\Synthetic Game\saves`,
			func(f *fakeFS) { f.mkdir(`D:\Old\Synthetic Game`) }, "overlaps the installation"},
		{"uninstalled, elsewhere", moved(`D:\Old\Synthetic Game`), restoreDir, func(*fakeFS) {}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := restorable(t)
			tt.change(f)
			before := everything(f)
			_, err := restorePlanWith(f, tt.m, restoreRequest(tt.destination))
			if everything(f) != before {
				t.Fatal("the plan wrote something")
			}
			if tt.mention == "" {
				if err != nil {
					t.Fatalf("a destination apart from a vanished installation is distinct: %v", err)
				}
				return
			}
			expectRestoreRefused(t, err, tt.mention)
		})
	}
}

// ADR 0006 §11: the paths the snapshot backed up are references too, even
// when the manifest now names other sources.
func TestReferencePathsIncludeTheSnapshotSources(t *testing.T) {
	f := restorable(t)
	f.mkdir(`D:\Elsewhere\saves`)
	m := manifest()
	m.Installation.Path = `D:\Elsewhere`
	_, err := restorePlanWith(f, m, restoreRequest(installPath+`\saves\restore`))
	expectRestoreRefused(t, err, "overlaps the backed-up source saves "+installPath+`\saves`)
	if _, err := restorePlanWith(f, m, restoreRequest(restoreDir)); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreVolumeIsQualified(t *testing.T) {
	f := restorable(t)
	f.volumeOf[`G:\`] = backup.Volume{FileSystem: "FAT32", Drive: backup.DriveRemovable, Available: 1 << 40}
	expectRestoreRefused(t, restoreError(f, restoreDir), "destination "+restoreDir+`: volume file system is "FAT32"`)
	f.volumeOf[`G:\`] = backup.Volume{FileSystem: "NTFS", Drive: backup.DriveRemote, Available: 1 << 40}
	expectRestoreRefused(t, restoreError(f, restoreDir), "only local fixed or removable drives are qualified")
	f.volumeOf[`G:\`] = backup.Volume{FileSystem: "NTFS", Drive: backup.DriveRemovable, Available: 1 << 40}
	if p := restorePlanOf(t, f); p.Outcome() != "new restore" || p.Volume.Drive != backup.DriveRemovable {
		t.Fatalf("removable drive: %s", p.Outcome())
	}
}

func TestRestoreSpaceIsChecked(t *testing.T) {
	f := restorable(t)
	f.volumeOf[`G:\`] = backup.Volume{FileSystem: "NTFS", Drive: backup.DriveFixed, Available: 6 + 64<<20 - 1}
	p := restorePlanOf(t, f)
	if p.Outcome() != "blocked" || !slices.Equal(restoreFindings(p), []string{"blocking insufficient-space " + restoreDir}) {
		t.Fatalf("plan: %s %q", p.Outcome(), restoreFindings(p))
	}
	f.volumeOf[`G:\`] = backup.Volume{FileSystem: "NTFS", Drive: backup.DriveFixed, Available: 6 + 64<<20}
	if p := restorePlanOf(t, f); p.Outcome() != "new restore" {
		t.Fatalf("exact space: %q", restoreFindings(p))
	}
}

// The longest path written is that of the incomplete copy.
func TestRestoreLongPathIsReported(t *testing.T) {
	f := restorable(t)
	// <dest>\<op>.incomplete\saves\Slot 1\2.save is 260 characters long.
	long := `G:\Restores\` + strings.Repeat("a", 260-len(`G:\Restores\`)-len(`\20261107T090000-9a8b7c6d.incomplete\saves\Slot 1\2.save`))
	p, err := restorePlanWith(f, manifest(), restoreRequest(long))
	if err != nil {
		t.Fatal(err)
	}
	if p.Outcome() != "new restore" || !slices.Equal(restoreFindings(p), []string{"info long-path Slot 1/2.save"}) || p.Findings[0].Source != "saves" {
		t.Fatalf("plan: %q", restoreFindings(p))
	}
	if p, _ := restorePlanWith(f, manifest(), restoreRequest(long[:len(long)-1])); len(p.Findings) != 0 {
		t.Fatalf("259 characters: %q", restoreFindings(p))
	}
}

// ADR 0009: the plan of an existing area lists its handed-over restores by
// their names, never their content, and reports a restore of the same
// snapshot as information.
func TestRestorePlanInAnExistingArea(t *testing.T) {
	f := restorable(t)
	copied := publishRestore(t, f, restoreDir, restored1, restoreAt)
	f.put(restoreDir+`\restore.lock`, f.file(""))
	f.reads = nil
	before := everything(f)
	p := restorePlanOf(t, f)
	if p.State != sync.DestinationArea || p.Outcome() != "new restore" || len(p.Restores) != 1 || p.Incomplete != 0 {
		t.Fatalf("plan: %s %s %+v", p.State, p.Outcome(), p.Restores)
	}
	if r := p.Restores[0]; r.ID != restored1 || r.Snapshot != snapApplied || r.State() != "present" {
		t.Fatalf("handed over: %+v", r)
	}
	if !slices.Equal(restoreFindings(p), []string{"info snapshot-already-restored " + restored1}) {
		t.Fatalf("findings %q", restoreFindings(p))
	}
	for _, read := range f.reads {
		if call, path, _ := strings.Cut(read, " "); strings.HasPrefix(path, restoreDir+`\`+restored1+`\`) ||
			(path == restoreDir+`\`+restored1 && call != "Lstat" && call != "Identity") {
			t.Fatalf("a handed-over restore was read: %s", read)
		}
	}
	if everything(f) != before || copied.children == nil {
		t.Fatal("the plan wrote something")
	}
}

// ADR 0009, handed-over restores: their names are classified, never
// blocking; an operation name no journal explains is an intervention.
func TestHandedOverRestoreObservations(t *testing.T) {
	published := restoreDir + `\` + restored1
	tests := []struct {
		name     string
		change   func(f *fakeFS, copied *node)
		state    string
		findings []string
		causes   []string
	}{
		{"missing", func(f *fakeFS, _ *node) { delete(f.get(restoreDir).children, restored1) }, "absent",
			[]string{"info restore-missing " + restored1}, nil},
		{"rename lost", func(f *fakeFS, copied *node) {
			delete(f.get(restoreDir).children, restored1)
			f.get(restoreDir).children[restored1+".incomplete"] = copied
		}, "rename lost", []string{"info restore-rename-lost " + restored1}, nil},
		{"replaced", func(f *fakeFS, copied *node) { f.get(restoreDir).children[restored1] = copyTree(f, copied) }, "replaced",
			[]string{"info restore-replaced " + restored1}, nil},
		{"a file", func(f *fakeFS, _ *node) { f.put(published, f.file("x")) }, "replaced", []string{"info restore-replaced " + restored1}, nil},
		{"a link", func(f *fakeFS, _ *node) { f.put(published, f.link()) }, "replaced", []string{"info restore-replaced " + restored1}, nil},
		{"incomplete beside", func(f *fakeFS, _ *node) { f.put(published+".incomplete", f.file("x")) }, "replaced",
			[]string{"info restore-replaced " + restored1}, nil},
		{"identity unreadable", func(_ *fakeFS, copied *node) { copied.busy = true }, "not observable", nil, nil},
		{"type unreadable", func(_ *fakeFS, copied *node) { copied.statErr = errors.New("access denied") }, "not observable", nil, nil},
		{"incomplete unreadable", func(f *fakeFS, _ *node) {
			delete(f.get(restoreDir).children, restored1)
			f.put(published+".incomplete", f.dir()).busy = true
		}, "not observable", nil, nil},
		{"renamed", func(f *fakeFS, copied *node) {
			delete(f.get(restoreDir).children, restored1)
			f.get(restoreDir).children[restored2] = copied
		}, "absent", []string{"info restore-missing " + restored1}, []string{"unexpected-entry " + restored2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := restorable(t)
			copied := publishRestore(t, f, restoreDir, restored1, restoreAt)
			tt.change(f, copied)
			p := restorePlanOf(t, f)
			findings := append([]string{"info snapshot-already-restored " + restored1}, tt.findings...)
			sort.Strings(findings)
			if len(p.Restores) != 1 || p.Restores[0].State() != tt.state || !slices.Equal(restoreFindings(p), findings) ||
				!slices.Equal(restoreCauses(p), tt.causes) || p.Blocking() != 0 {
				t.Fatalf("plan: %+v, %q, %q", p.Restores, restoreFindings(p), restoreCauses(p))
			}
		})
	}
}

// ADR 0006 §11: an area whose operations or receipts no journal explains,
// or with an operation pending, needs an intervention, code 3; a restore
// running in the area is never taken for an interrupted one.
func TestRestoreAreaInterventions(t *testing.T) {
	tests := []struct {
		name   string
		change func(t *testing.T, f *fakeFS)
		causes []string
	}{
		{"initialization interrupted", func(_ *testing.T, f *fakeFS) {
			f.put(restoreDir+`\savewarden-restore.json.tmp-1`, f.file("{"))
		}, []string{"initialization-interrupted " + restoreDir}},
		{"unexpected entry", func(t *testing.T, f *fakeFS) {
			markArea(t, f, restoreDir, "synthetic-game")
			f.put(restoreDir+`\notes.txt`, f.file("x"))
		}, []string{"unexpected-entry notes.txt"}},
		{"pending", func(t *testing.T, f *fakeFS) {
			startRestore(t, f, restoreDir, restored2, restoreAt, backup.StepPlanned)
		}, []string{"pending-operation " + restored2}},
		{"unexplained copy", func(t *testing.T, f *fakeFS) {
			markArea(t, f, restoreDir, "synthetic-game")
			f.mkdir(restoreDir + `\` + restored2 + ".incomplete")
		}, []string{"unexpected-entry " + restored2 + ".incomplete"}},
		{"orphan receipt", func(t *testing.T, f *fakeFS) {
			markArea(t, f, restoreDir, "synthetic-game")
			f.put(restoreDir+`\receipts\`+restored2+".json", f.file("{}"))
		}, []string{"journal-ambiguous receipts/" + restored2 + ".json"}},
		{"receipt of a handed-over restore missing", func(t *testing.T, f *fakeFS) {
			publishRestore(t, f, restoreDir, restored1, restoreAt)
			delete(f.get(restoreDir+`\receipts`).children, restored1+".json")
		}, []string{"journal-ambiguous receipts/" + restored1 + ".json"}},
		{"receipt of a handed-over restore altered", func(t *testing.T, f *fakeFS) {
			publishRestore(t, f, restoreDir, restored1, restoreAt)
			receipt := f.get(restoreDir + `\receipts\` + restored1 + ".json")
			receipt.data = append(receipt.data, ' ')
		}, []string{"journal-ambiguous receipts/" + restored1 + ".json"}},
		{"receipts missing", func(t *testing.T, f *fakeFS) {
			publishRestore(t, f, restoreDir, restored1, restoreAt)
			delete(f.get(restoreDir).children, "receipts")
		}, []string{"journal-ambiguous receipts", "journal-ambiguous receipts/" + restored1 + ".json"}},
		{"copy of a failed restore", func(t *testing.T, f *fakeFS) {
			startRestore(t, f, restoreDir, restored2, restoreAt, backup.StepPlanned, backup.StepStagingIntent, backup.StepFailed)
			f.mkdir(restoreDir + `\` + restored2)
		}, []string{"unexpected-entry " + restored2}},
		{"journal ambiguous", func(t *testing.T, f *fakeFS) {
			// Its copy and its receipt are then explained by no journal.
			publishRestore(t, f, restoreDir, restored1, restoreAt)
			f.put(restoreDir+`\journal\`+restored1+`\notes.txt`, f.file("x"))
		}, []string{"unexpected-entry " + restored1, "journal-ambiguous journal/" + restored1, "journal-ambiguous receipts/" + restored1 + ".json"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := restorable(t)
			f.mkdir(restoreDir)
			tt.change(t, f)
			before := everything(f)
			p := restorePlanOf(t, f)
			if p.Outcome() != "intervention required" || !slices.Equal(restoreCauses(p), tt.causes) {
				t.Fatalf("plan: %s, %q", p.Outcome(), restoreCauses(p))
			}
			if everything(f) != before {
				t.Fatal("the plan wrote something")
			}
		})
	}
	// A pending operation whose lock is held belongs to a running restore.
	f := restorable(t)
	startRestore(t, f, restoreDir, restored2, restoreAt, backup.StepPlanned)
	f.put(restoreDir+`\restore.lock`, f.file("")).busy = true
	expectRestoreRefused(t, restoreError(f, restoreDir), "another restore is running in this area")
	f.get(restoreDir + `\restore.lock`).busy = false
	if p := restorePlanOf(t, f); !slices.Equal(restoreCauses(p), []string{"pending-operation " + restored2}) {
		t.Fatalf("lock free: %q", restoreCauses(p))
	}
}
