package sync_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/entities/game"
	"github.com/savewarden/savewarden-cli/internal/features/inspect"
	"github.com/savewarden/savewarden-cli/internal/features/sync"
)

const (
	installPath = `D:\Games\Synthetic Game`
	vaultPath   = `F:\Vault`
	op1         = "20261105T101500-1a2b3c4d"
	op2         = "20261106T101500-2b3c4d5e"
)

var at = time.Date(2026, 11, 5, 10, 15, 0, 0, time.UTC)

func source(id, path string, required bool) game.Source {
	return game.Source{ID: id, Kind: game.KindSave, Base: game.BaseGameInstall, Path: path, Required: required}
}

func manifest(sources ...game.Source) game.Manifest {
	if len(sources) == 0 {
		sources = []game.Source{source("saves", "saves", true)}
	}
	return game.Manifest{
		SchemaVersion: 1,
		Game:          game.Game{ID: "synthetic-game", Name: "Synthetic Game"},
		Installation:  game.Installation{Path: installPath},
		Sources:       sources,
	}
}

// world holds a synthetic installation on D:\ and an empty F:\ volume.
func world() *fakeFS {
	f := newFakeFS(`D:\`, `F:\`)
	f.put(installPath+`\saves\1.save`, f.file("one"))
	f.put(installPath+`\saves\Slot 1\2.save`, f.file("two"))
	return f
}

func service(f *fakeFS, m game.Manifest) *sync.Service {
	return sync.New(fakeManifests{m}, inspect.New(nil, inspectFS{f}), syncFiles{f})
}

func plan(t *testing.T, f *fakeFS, m game.Manifest, vault string) sync.Plan {
	t.Helper()
	p, err := service(f, m).Plan(context.Background(), sync.Request{ManifestPath: "m.yaml", VaultPath: vault})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func planError(f *fakeFS, m game.Manifest, vault string) error {
	_, err := service(f, m).Plan(context.Background(), sync.Request{ManifestPath: "m.yaml", VaultPath: vault})
	return err
}

func findings(p sync.Plan) []string {
	var out []string
	for _, finding := range p.Findings {
		out = append(out, fmt.Sprintf("%s %s %s %s", finding.Severity, finding.Code, finding.Source, finding.Path))
	}
	return out
}

func expectFindings(t *testing.T, p sync.Plan, want ...string) {
	t.Helper()
	if got := findings(p); !slices.Equal(got, want) {
		t.Fatalf("findings:\n got %q\nwant %q", got, want)
	}
}

func causes(p sync.Plan) []string {
	var out []string
	for _, i := range p.Interventions {
		out = append(out, string(i.Cause)+" "+i.Subject)
	}
	return out
}

// markVault initializes a vault as step I4 of ADR 0006 leaves it.
func markVault(t *testing.T, f *fakeFS, vault string) {
	t.Helper()
	data, err := backup.EncodeMarker(backup.Marker{CreatedAt: at, ToolVersion: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	f.mkdir(vault)
	f.put(vault+`\savewarden-vault.json`, f.file(string(data)))
}

// copyTree duplicates a fake tree with fresh identities.
func copyTree(f *fakeFS, n *node) *node {
	out := &node{mode: n.mode, data: append([]byte(nil), n.data...), modTime: n.modTime, crypted: n.crypted}
	f.nextID++
	out.id = f.nextID
	if n.children != nil {
		out.children = map[string]*node{}
		for name, child := range n.children {
			out.children[name] = copyTree(f, child)
		}
	}
	return out
}

// publish lays out what a completed publication of op leaves in the vault:
// current, copied from the sources, its receipt and a terminal journal.
func publish(t *testing.T, f *fakeFS, m game.Manifest, op, previous string) {
	t.Helper()
	game := vaultPath + `\games\` + m.Game.ID
	current := f.mkdir(game + `\current`)
	receipt := backup.Receipt{
		Operation: op, ToolVersion: "dev", GameID: m.Game.ID, GameName: m.Game.Name,
		StartedAt: at, PublishIntentAt: at.Add(time.Second), Previous: previous,
		PlanSHA256: sha256.Sum256([]byte(op)),
	}
	published := at.Add(2 * time.Second)
	receipt.PublishedAt = &published
	for i, s := range m.Sources {
		volume, segments, _ := m.SourceSegments(i)
		dir, err := inspect.New(nil, inspectFS{f}).Directory(context.Background(), volume, segments)
		if err != nil {
			t.Fatal(err)
		}
		rs := backup.ReceiptSource{ID: s.ID, Kind: string(s.Kind), Required: s.Required, State: backup.SourceMissing, Path: windows(volume, segments)}
		if dir.Status == backup.StatusInventoried {
			rs.State, rs.Directories, rs.Entries = backup.SourceCopied, dir.Directories, dir.Entries
			current.children[s.ID] = copyTree(f, f.get(windows(volume, segments)))
			receipt.Changes.Added += len(dir.Entries)
		}
		receipt.Sources = append(receipt.Sources, rs)
	}
	data, err := backup.EncodeReceipt(receipt)
	if err != nil {
		t.Fatal(err)
	}
	f.put(game+`\receipts\`+op+`.json`, f.file(string(data)))
	writeJournal(t, f, op, backup.StepPlanned, backup.StepStagingIntent, backup.StepStaged, backup.StepVerified,
		backup.StepPublishIntent, backup.StepPublished)
}

func windows(volume string, segments []string) string { return volume + strings.Join(segments, `\`) }

// writeJournal commits the given steps of op, as the protocol would.
func writeJournal(t *testing.T, f *fakeFS, op string, steps ...backup.Step) {
	t.Helper()
	dir := f.mkdir(vaultPath + `\games\synthetic-game\journal\` + op)
	prev := ""
	for i, step := range steps {
		fields := map[string]json.RawMessage{}
		if step == backup.StepPlanned {
			fields["previous"] = json.RawMessage("null")
		}
		data, err := backup.EncodeRecord(backup.Record{Seq: i + 1, Step: step, Op: op, At: at, Prev: prev, Fields: fields})
		if err != nil {
			t.Fatal(err)
		}
		dir.children[backup.RecordName(i+1, step)] = f.file(string(data))
		sum := sha256.Sum256(data)
		prev = hex.EncodeToString(sum[:])
	}
}

func TestFirstPlanIntoANewVault(t *testing.T) {
	f := world()
	f.mkdir(`F:\Backups`)
	p := plan(t, f, manifest(), `F:\Backups\Vault`)
	if p.VaultState != sync.VaultAbsent || p.Destination != `F:\Backups\Vault\games\synthetic-game\current` || p.Current != "" {
		t.Fatalf("plan = %+v", p)
	}
	if p.Outcome() != "changes" || len(p.Findings) != 0 || len(p.Sources) != 1 {
		t.Fatalf("outcome %q, findings %q", p.Outcome(), findings(p))
	}
	s := p.Sources[0]
	if s.Status != sync.SourcePlanned || s.Changes.Added != 2 || s.Files != 2 || s.Bytes != 6 ||
		!slices.Equal(s.Changes.AddedDirectories, []string{"Slot 1"}) || s.Path != installPath+`\saves` {
		t.Fatalf("source = %+v", s)
	}
	if again := plan(t, f, manifest(), `F:\Backups\Vault`); !reflect.DeepEqual(again, p) {
		t.Fatal("two plans of the same state differ")
	}
	empty := plan(t, f, manifest(), `F:\Backups`)
	if empty.VaultState != sync.VaultEmpty {
		t.Fatalf("empty directory: %s", empty.VaultState)
	}
}

func TestSameVolumeIsInformationOnly(t *testing.T) {
	f := world()
	p := plan(t, f, manifest(source("saves", "saves", true), source("extra", "extra", false)), `D:\Vault`)
	expectFindings(t, p,
		`info same-volume saves D:\`,
		`info same-volume extra D:\`,
		`info source-missing extra D:\Games\Synthetic Game\extra`,
	)
	if p.Blocking() != 0 || p.Outcome() != "changes" {
		t.Fatalf("outcome %q", p.Outcome())
	}
}

func TestRefusedVaults(t *testing.T) {
	tests := []struct {
		name  string
		setup func(f *fakeFS)
		vault string
		want  string
	}{
		{"relative path", nil, `Vault`, "absolute path"},
		{"UNC path", nil, `\\server\share`, "UNC"},
		{"missing parent", nil, `F:\Nope\Vault`, `parent directory Nope does not exist`},
		{"missing volume", nil, `G:\Vault`, `volume G:\ is not available`},
		{"vault is a file", func(f *fakeFS) { f.put(`F:\Vault`, f.file("x")) }, vaultPath, "is not a directory"},
		{"link on the way", func(f *fakeFS) { f.put(`F:\Linked`, f.link()) }, `F:\Linked\Vault`, "not followed"},
		{"vault is a link", func(f *fakeFS) { f.put(vaultPath, f.link()) }, vaultPath, "not followed"},
		{"foreign directory", func(f *fakeFS) { f.put(`F:\Vault\photo.jpg`, f.file("x")) }, vaultPath, "not a Savewarden vault"},
		{"old vault without marker", func(f *fakeFS) {
			f.put(`F:\Vault\games\synthetic-game\current\saves\1.save`, f.file("one"))
		}, vaultPath, "never adopted"},
		{"temporaries and another file", func(f *fakeFS) {
			f.put(`F:\Vault\savewarden-vault.json.tmp-1`, f.file("{"))
			f.put(`F:\Vault\notes.txt`, f.file("x"))
		}, vaultPath, "not a Savewarden vault"},
		{"invalid marker", func(f *fakeFS) { f.put(`F:\Vault\savewarden-vault.json`, f.file(`{"schema":"other"}`)) }, vaultPath, "never rewritten"},
		{"marker is a link", func(f *fakeFS) { f.put(`F:\Vault\savewarden-vault.json`, f.link()) }, vaultPath, "not a Savewarden vault"},
		{"games is a link", func(f *fakeFS) {
			f.put(`F:\Vault\savewarden-vault.json`, f.file(""))
			f.put(`F:\Vault\games`, f.link())
		}, vaultPath, "games is a link"},
		{"vault is the installation", nil, installPath, "overlaps the installation"},
		{"vault in the installation", nil, installPath + `\saves\Vault`, "overlaps the installation"},
		{"vault contains the installation", nil, `D:\Games`, "overlaps the installation"},
		{"vault in the installation by case", nil, `D:\GAMES\synthetic game\Vault`, "overlaps the installation"},
		{"vault is the volume root", nil, `D:\`, "overlaps the installation"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := world()
			if tt.setup != nil {
				tt.setup(f)
			}
			if err := planError(f, manifest(), tt.vault); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
	t.Run("game directory is a link", func(t *testing.T) {
		f := world()
		markVault(t, f, vaultPath)
		f.put(vaultPath+`\games\synthetic-game`, f.link())
		if err := planError(f, manifest(), vaultPath); err == nil || !strings.Contains(err.Error(), `games\synthetic-game is a link`) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("short name of the installation", func(t *testing.T) {
		// An 8.3 alias reaches the installation under another spelling.
		f := world()
		games := f.get(`D:\Games`)
		games.children["SYNTHE~1"] = games.children["Synthetic Game"]
		if err := planError(f, manifest(), `D:\Games\SYNTHE~1\Vault`); err == nil || !strings.Contains(err.Error(), "under another name") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestInterventions(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, f *fakeFS)
		want  []string
	}{
		{"interrupted initialization", func(t *testing.T, f *fakeFS) {
			f.put(`F:\Vault\savewarden-vault.json.tmp-1`, f.file("{"))
		}, []string{"initialization-interrupted ."}},
		{"stray at the root", func(t *testing.T, f *fakeFS) {
			markVault(t, f, vaultPath)
			f.put(`F:\Vault\notes.txt`, f.file("x"))
		}, []string{"unexpected-entry notes.txt"}},
		{"stray in the game directory", func(t *testing.T, f *fakeFS) {
			markVault(t, f, vaultPath)
			f.put(vaultPath+`\games\synthetic-game\backup.zip`, f.file("x"))
		}, []string{"unexpected-entry games/synthetic-game/backup.zip"}},
		{"pending operation", func(t *testing.T, f *fakeFS) {
			markVault(t, f, vaultPath)
			writeJournal(t, f, op1, backup.StepPlanned, backup.StepStagingIntent)
			f.mkdir(vaultPath + `\games\synthetic-game\staging\` + op1)
		}, []string{"pending-operation " + op1}},
		{"operation never started", func(t *testing.T, f *fakeFS) {
			markVault(t, f, vaultPath)
			f.mkdir(vaultPath + `\games\synthetic-game\journal\` + op1)
		}, []string{"pending-operation " + op1}},
		{"two pending operations", func(t *testing.T, f *fakeFS) {
			markVault(t, f, vaultPath)
			writeJournal(t, f, op1, backup.StepPlanned)
			writeJournal(t, f, op2, backup.StepPlanned)
		}, []string{"pending-operation " + op1, "pending-operation " + op2, "journal-ambiguous games/synthetic-game/journal"}},
		{"corrupt journal", func(t *testing.T, f *fakeFS) {
			markVault(t, f, vaultPath)
			writeJournal(t, f, op1, backup.StepPlanned, backup.StepAbandoned)
			record := f.get(vaultPath + `\games\synthetic-game\journal\` + op1 + `\002-abandoned.rec`)
			record.data = record.data[:len(record.data)-10]
		}, []string{"journal-ambiguous " + op1}},
		{"staging without journal", func(t *testing.T, f *fakeFS) {
			markVault(t, f, vaultPath)
			f.mkdir(vaultPath + `\games\synthetic-game\staging\` + op1)
		}, []string{"unexpected-entry games/synthetic-game/staging/" + op1}},
		{"retained without receipt", func(t *testing.T, f *fakeFS) {
			markVault(t, f, vaultPath)
			f.mkdir(vaultPath + `\games\synthetic-game\retained\` + op1)
		}, []string{"unexpected-entry games/synthetic-game/retained/" + op1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := world()
			tt.setup(t, f)
			p := plan(t, f, manifest(), vaultPath)
			if got := causes(p); !slices.Equal(got, tt.want) {
				t.Fatalf("interventions = %q, want %q", got, tt.want)
			}
			if p.Outcome() != "intervention required" || len(p.Sources) != 0 {
				t.Fatalf("outcome %q, sources %d", p.Outcome(), len(p.Sources))
			}
		})
	}
	t.Run("nine marker temporaries", func(t *testing.T) {
		f := world()
		for k := 1; k <= backup.MaxTemporaries; k++ {
			f.put(fmt.Sprintf(`F:\Vault\savewarden-vault.json.tmp-%d`, k), f.file("{"))
		}
		p := plan(t, f, manifest(), vaultPath)
		if p.VaultState != sync.VaultInterrupted || !strings.Contains(p.Interventions[0].Detail, "envelope exhausted") {
			t.Fatalf("plan = %+v", p)
		}
	})
}

func TestUpToDateAndChanges(t *testing.T) {
	f := world()
	markVault(t, f, vaultPath)
	publish(t, f, manifest(), op1, "")
	p := plan(t, f, manifest(), vaultPath)
	if p.Current != op1 || p.Outcome() != "up to date" || p.VaultState != sync.VaultInitialized || len(p.Findings) != 0 {
		t.Fatalf("plan = %+v, findings %q", p, findings(p))
	}

	f.get(installPath + `\saves\1.save`).data = []byte("ONE!")
	f.put(installPath+`\saves\3.save`, f.file("three"))
	p = plan(t, f, manifest(), vaultPath)
	var got []string
	for _, change := range p.Sources[0].Changes.Files {
		got = append(got, change.Kind.Marker()+change.Entry.Path)
	}
	if !slices.Equal(got, []string{"~1.save", "+3.save", "=Slot 1/2.save"}) || p.Outcome() != "changes" {
		t.Fatalf("changes = %q, outcome %q, findings %q", got, p.Outcome(), findings(p))
	}
}

func TestRemovalsAndMissingSources(t *testing.T) {
	two := manifest(source("saves", "saves", true), source("profile", "profile", false))
	tests := []struct {
		name    string
		publish game.Manifest
		mutate  func(f *fakeFS)
		plan    game.Manifest
		want    []string
	}{
		{"file removed", manifest(), func(f *fakeFS) {
			delete(f.get(installPath+`\saves`).children, "1.save")
		}, manifest(), []string{"blocking files-removed saves 1.save"}},
		{"directory emptied", manifest(), func(f *fakeFS) {
			delete(f.get(installPath+`\saves`).children, "Slot 1")
		}, manifest(), []string{"blocking files-removed saves Slot 1", "blocking files-removed saves Slot 1/2.save"}},
		{"required source missing", manifest(), func(f *fakeFS) {
			delete(f.get(installPath).children, "saves")
		}, manifest(), []string{`blocking source-missing saves D:\Games\Synthetic Game\saves`}},
		{"optional source vanished", two, func(f *fakeFS) {
			delete(f.get(installPath).children, "profile")
		}, two, []string{`blocking source-vanished profile D:\Games\Synthetic Game\profile`}},
		{"source removed from the manifest", two, func(*fakeFS) {}, manifest(),
			[]string{"blocking source-removed-from-manifest profile "}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := world()
			f.put(installPath+`\profile\p.ini`, f.file("p"))
			markVault(t, f, vaultPath)
			publish(t, f, tt.publish, op1, "")
			tt.mutate(f)
			p := plan(t, f, tt.plan, vaultPath)
			expectFindings(t, p, tt.want...)
			if p.Outcome() != "blocked" {
				t.Fatalf("outcome %q", p.Outcome())
			}
		})
	}
}

func TestCurrentIsCheckedAgainstItsReceipt(t *testing.T) {
	game := vaultPath + `\games\synthetic-game`
	tests := []struct {
		name   string
		mutate func(f *fakeFS)
		want   []string
	}{
		{"byte altered", func(f *fakeFS) { f.get(game + `\current\saves\1.save`).data = []byte("ONE") },
			[]string{"blocking current-modified saves 1.save"}},
		{"file added", func(f *fakeFS) { f.put(game+`\current\saves\extra.save`, f.file("x")) },
			[]string{"blocking current-modified saves extra.save"}},
		{"file deleted", func(f *fakeFS) { delete(f.get(game+`\current\saves`).children, "1.save") },
			[]string{"blocking current-modified saves 1.save"}},
		{"date touched", func(f *fakeFS) { f.get(game + `\current\saves\1.save`).modTime = epoch.Add(time.Hour) },
			[]string{"blocking current-modified saves 1.save"}},
		{"link inside", func(f *fakeFS) { f.put(game+`\current\saves\jn`, f.link()) },
			[]string{"blocking current-modified  saves/jn"}},
		{"current without receipt", func(f *fakeFS) { delete(f.get(game).children, "receipts") },
			[]string{"blocking current-modified  current"}},
		{"receipt without current", func(f *fakeFS) { delete(f.get(game).children, "current") },
			[]string{"blocking current-modified  current"}},
		{"current is a link", func(f *fakeFS) { f.put(game+`\current`, f.link()) },
			[]string{"blocking current-modified  current"}},
		{"corrupt receipt", func(f *fakeFS) {
			r := f.get(game + `\receipts\` + op1 + `.json`)
			r.data = r.data[:len(r.data)-5]
		}, []string{"blocking current-modified  current"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := world()
			markVault(t, f, vaultPath)
			publish(t, f, manifest(), op1, "")
			tt.mutate(f)
			p := plan(t, f, manifest(), vaultPath)
			expectFindings(t, p, tt.want...)
			if p.Outcome() != "blocked" {
				t.Fatalf("outcome %q", p.Outcome())
			}
		})
	}
	t.Run("two heads", func(t *testing.T) {
		f := world()
		markVault(t, f, vaultPath)
		publish(t, f, manifest(), op1, "")
		receipts := f.get(game + `\receipts`)
		other := strings.ReplaceAll(string(receipts.children[op1+".json"].data), op1, op2)
		receipts.children[op2+".json"] = f.file(other)
		writeJournal(t, f, op2, backup.StepAbandoned)
		p := plan(t, f, manifest(), vaultPath)
		if !strings.Contains(p.Findings[0].Detail, "2 receipts each claim") {
			t.Fatalf("findings %+v", p.Findings)
		}
	})
	t.Run("receipt temporaries are ignored", func(t *testing.T) {
		f := world()
		markVault(t, f, vaultPath)
		publish(t, f, manifest(), op1, "")
		f.put(game+`\receipts\`+op2+`.json.tmp-1`, f.file("{"))
		if p := plan(t, f, manifest(), vaultPath); p.Outcome() != "up to date" {
			t.Fatalf("outcome %q, findings %q", p.Outcome(), findings(p))
		}
	})
}

func TestSourceContentFindings(t *testing.T) {
	f := world()
	f.put(installPath+`\saves\jn`, f.link())
	f.put(installPath+`\saves\secret.save`, f.file("s")).crypted = true
	f.put(installPath+`\saves\trailing.`, f.file("t"))
	f.put(installPath+`\saves\CON`, f.dir())
	deep := installPath + `\saves`
	for i := range 12 {
		deep += fmt.Sprintf(`\very long directory name %02d`, i)
	}
	f.put(deep+`\deep.save`, f.file("d"))
	p := plan(t, f, manifest(), vaultPath)
	got := findings(p)
	for _, want := range []string{
		"blocking unportable-name saves CON",
		"blocking encrypted-source saves secret.save",
		"blocking unportable-name saves trailing.",
		"blocking link-not-followed saves jn",
	} {
		if !slices.Contains(got, want) {
			t.Fatalf("findings %q lack %q", got, want)
		}
	}
	var long bool
	for _, finding := range p.Findings {
		long = long || finding.Code == sync.CodeLongPath && strings.HasSuffix(finding.Path, "deep.save") && finding.Severity == backup.Info
	}
	if !long || p.Outcome() != "blocked" {
		t.Fatalf("long path %v, outcome %q, findings %q", long, p.Outcome(), got)
	}
}

func TestPlanErrors(t *testing.T) {
	f := world()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := service(f, manifest()).Plan(ctx, sync.Request{ManifestPath: "m.yaml", VaultPath: vaultPath})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	bad := manifest()
	bad.Game.ID = "Bad ID"
	if err := planError(f, bad, vaultPath); err == nil || !strings.Contains(err.Error(), "game.id") {
		t.Fatalf("invalid manifest: %v", err)
	}
	if f.opened != 0 {
		t.Fatalf("a refused plan read %d files", f.opened)
	}
}

// Sources that reach the same directory under another name, such as an 8.3
// short name, are refused as inspect refuses them.
func TestSourcesOverlappingByIdentity(t *testing.T) {
	f := world()
	f.put(installPath+`\Profile Data\Slot\p.save`, f.file("p"))
	install := f.get(installPath)
	install.children["PROFIL~1"] = install.children["Profile Data"]
	m := manifest(source("profile", "Profile Data", true), source("slot", `PROFIL~1\Slot`, true))
	if err := planError(f, m, vaultPath); err == nil || !strings.Contains(err.Error(), `sources "profile" and "slot" resolve to overlapping directories`) {
		t.Fatalf("error = %v", err)
	}
}

func TestVaultStatesThatFailClosed(t *testing.T) {
	game := vaultPath + `\games\synthetic-game`
	t.Run("game directory differing by case", func(t *testing.T) {
		f := world()
		markVault(t, f, vaultPath)
		f.mkdir(vaultPath + `\games\Synthetic-Game\current`)
		p := plan(t, f, manifest(), vaultPath)
		if got := causes(p); !slices.Equal(got, []string{"unexpected-entry games/Synthetic-Game"}) {
			t.Fatalf("interventions = %q", got)
		}
	})
	t.Run("unreadable journal", func(t *testing.T) {
		f := world()
		markVault(t, f, vaultPath)
		f.mkdir(game + `\journal`).listErr = errors.New("access denied")
		p := plan(t, f, manifest(), vaultPath)
		if got := causes(p); !slices.Equal(got, []string{"journal-ambiguous games/synthetic-game/journal"}) {
			t.Fatalf("interventions = %q", got)
		}
	})
	t.Run("unreadable operation journal", func(t *testing.T) {
		f := world()
		markVault(t, f, vaultPath)
		f.mkdir(game + `\journal\` + op1).listErr = errors.New("access denied")
		p := plan(t, f, manifest(), vaultPath)
		if got := causes(p); !slices.Equal(got, []string{"journal-ambiguous " + op1}) {
			t.Fatalf("interventions = %q", got)
		}
	})
	t.Run("receipt of another game", func(t *testing.T) {
		f := world()
		markVault(t, f, vaultPath)
		publish(t, f, manifest(), op1, "")
		r := f.get(game + `\receipts\` + op1 + `.json`)
		r.data = []byte(strings.Replace(string(r.data), `"id": "synthetic-game"`, `"id": "other-game"`, 1))
		expectFindings(t, plan(t, f, manifest(), vaultPath), "blocking current-modified  current")
	})
	t.Run("current not checked on an intervention", func(t *testing.T) {
		f := world()
		markVault(t, f, vaultPath)
		publish(t, f, manifest(), op1, "")
		writeJournal(t, f, op2, backup.StepPlanned)
		p := plan(t, f, manifest(), vaultPath)
		if !p.CurrentUnchecked || p.Current != "" || len(p.Pending) != 1 {
			t.Fatalf("plan = %+v", p)
		}
	})
}
