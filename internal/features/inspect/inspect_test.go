package inspect_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/game"
	"github.com/savewarden/savewarden-cli/internal/features/inspect"
)

const install = "Games/Synthetic Game"

func manifest(sources ...game.Source) game.Manifest {
	if len(sources) == 0 {
		sources = []game.Source{{ID: "saves", Kind: game.KindSave, Base: game.BaseGameInstall, Path: "saves", Required: true}}
	}
	return game.Manifest{
		SchemaVersion: 1,
		Game:          game.Game{ID: "synthetic-game", Name: "Synthetic Game"},
		Installation:  game.Installation{Path: `D:\Games\Synthetic Game`},
		Sources:       sources,
	}
}

func source(id, path string, required bool) game.Source {
	return game.Source{ID: id, Kind: game.KindSave, Base: game.BaseGameInstall, Path: path, Required: required}
}

func run(t *testing.T, files *fakeFS, m game.Manifest) inspect.Report {
	t.Helper()
	report, err := inspect.New(fakeManifests{manifest: m}, files).Inspect(context.Background(), inspect.Request{ManifestPath: "m.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func paths(entries []inspect.Entry) []string {
	var out []string
	for _, entry := range entries {
		out = append(out, entry.Path)
	}
	return out
}

func codes(report inspect.Report) []string {
	var out []string
	for _, finding := range report.Findings {
		out = append(out, fmt.Sprintf("%s %s %s %s", finding.Severity, finding.Code, finding.Source, finding.Path))
	}
	return out
}

func expectFindings(t *testing.T, report inspect.Report, want ...string) {
	t.Helper()
	if got := codes(report); !slices.Equal(got, want) {
		t.Fatalf("findings:\n got %q\nwant %q", got, want)
	}
}

func TestInventoryIsSortedHashedAndComplete(t *testing.T) {
	files := newFakeFS()
	files.put(install+"/saves/b.save", files.file("bravo"))
	files.put(install+"/saves/a.save", files.file("alpha"))
	files.put(install+"/saves/a-b.save", files.file("alpha-bravo"))
	files.put(install+"/saves/dir/c.save", files.file("charlie"))
	files.put(install+"/saves/empty.save", files.file(""))
	files.put(install+"/saves/Jeu été/オ.save", files.file("unicode"))
	files.put(install+"/saves/empty dir", files.dir())

	report := run(t, files, manifest())
	if report.Blocking() != 0 || len(report.Findings) != 0 {
		t.Fatalf("findings: %q", codes(report))
	}
	got := report.Sources[0]
	want := []string{"a-b.save", "a.save", "b.save", "dir/c.save", "empty.save", "Jeu été/オ.save"}
	slices.Sort(want)
	if !slices.Equal(paths(got.Entries), want) {
		t.Fatalf("paths = %q, want %q", paths(got.Entries), want)
	}
	if got.Status != inspect.StatusInventoried || got.Path != `D:\Games\Synthetic Game\saves` || got.Bytes != 35 {
		t.Fatalf("source = %+v", got)
	}
	for _, entry := range got.Entries {
		if entry.ModTime.Location() != time.UTC || !entry.ModTime.Equal(epoch) {
			t.Fatalf("%s mod time %v", entry.Path, entry.ModTime)
		}
		if entry.Path == "empty.save" && hex.EncodeToString(entry.SHA256[:]) != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
			t.Fatalf("empty file hash %x", entry.SHA256)
		}
		if entry.Path == "a.save" && entry.SHA256 != sha256.Sum256([]byte("alpha")) {
			t.Fatalf("hash of a.save")
		}
	}
	if again := run(t, files, manifest()); !reflect.DeepEqual(again, report) {
		t.Fatal("two inspections of the same tree differ")
	}
}

func TestLinksAndSpecialFilesAreNeverOpened(t *testing.T) {
	files := newFakeFS()
	files.put(install+"/saves/ok.save", files.file("ok"))
	files.put(install+"/saves/link-file", files.special(fs.ModeSymlink))
	files.put(install+"/saves/link-dir", files.special(fs.ModeSymlink|fs.ModeDir))
	files.put(install+"/saves/junction", files.special(fs.ModeIrregular))
	files.put(install+"/saves/device", files.special(fs.ModeDevice|fs.ModeCharDevice))
	files.put(install+"/saves/pipe", files.special(fs.ModeNamedPipe))

	report := run(t, files, manifest())
	expectFindings(t, report,
		"blocking special-file saves device",
		"blocking link-not-followed saves junction",
		"blocking link-not-followed saves link-dir",
		"blocking link-not-followed saves link-file",
		"blocking special-file saves pipe",
	)
	if !slices.Equal(files.opened, []string{install + "/saves/ok.save"}) {
		t.Fatalf("opened = %q", files.opened)
	}
	for _, listed := range files.listed {
		if strings.Contains(listed, "link") || strings.Contains(listed, "junction") {
			t.Fatalf("listed through a link: %q", listed)
		}
	}
}

func TestLinkOnTheWayRefusesTheSource(t *testing.T) {
	for _, component := range []string{"Games", install, install + "/saves"} {
		t.Run(component, func(t *testing.T) {
			files := newFakeFS()
			files.put(install+"/saves/ok.save", files.file("ok"))
			parent, name := files.parentOf(component)
			parent.children[name].mode = fs.ModeIrregular | fs.ModeDir
			report := run(t, files, manifest())
			want := `D:\` + strings.ReplaceAll(component, "/", `\`)
			expectFindings(t, report, "blocking link-not-followed saves "+want)
			if report.Sources[0].Status != inspect.StatusRefused || len(files.listed) != 0 || len(files.opened) != 0 {
				t.Fatalf("status %s, listed %q, opened %q", report.Sources[0].Status, files.listed, files.opened)
			}
		})
	}
}

func TestUnreadableEntriesDoNotStopTheWalk(t *testing.T) {
	files := newFakeFS()
	files.put(install+"/saves/a.save", files.file("a"))
	files.put(install+"/saves/locked.save", files.file("x")).openErr = errors.New("sharing violation")
	files.put(install+"/saves/denied.save", files.file("x")).lstatErr = fs.ErrPermission
	files.put(install+"/saves/private", files.dir()).listErr = fs.ErrPermission
	files.put(install+"/saves/z.save", files.file("z"))

	report := run(t, files, manifest())
	expectFindings(t, report,
		"blocking unreadable saves denied.save",
		"blocking unreadable saves locked.save",
		"blocking unreadable saves private",
	)
	if !slices.Equal(paths(report.Sources[0].Entries), []string{"a.save", "z.save"}) {
		t.Fatalf("entries = %q", paths(report.Sources[0].Entries))
	}
}

func TestConcurrentChangesAreReported(t *testing.T) {
	files := newFakeFS()
	files.put(install+"/saves/stable.save", files.file("stable"))
	growing := files.put(install+"/saves/growing.save", files.file("abc"))
	growing.onRead = func() { growing.data = []byte("abcdef"); growing.modTime = epoch.Add(time.Second) }
	touched := files.put(install+"/saves/touched.save", files.file("abc"))
	touched.onRead = func() { touched.modTime = epoch.Add(time.Second) }
	before := files.put(install+"/saves/before.save", files.file("abc"))
	before.onOpen = func() { before.data = []byte("abcd") }
	files.put(install+"/saves/swapped.save", files.file("abc")).onOpen = func() {
		files.put(install+"/saves/swapped.save", files.file("abc"))
	}
	files.put(install+"/saves/relinked.save", files.file("abc")).onOpen = func() {
		files.put(install+"/saves/relinked.save", files.special(fs.ModeSymlink))
	}
	files.put(install+"/saves/vanished.save", files.file("abc")).onOpen = func() {
		files.remove(install + "/saves/vanished.save")
	}

	report := run(t, files, manifest())
	expectFindings(t, report,
		"blocking changed-during-read saves before.save",
		"blocking changed-during-read saves growing.save",
		"blocking link-not-followed saves relinked.save",
		"blocking replaced-before-read saves swapped.save",
		"blocking changed-during-read saves touched.save",
		"blocking changed-during-read saves vanished.save",
	)
	if !slices.Equal(paths(report.Sources[0].Entries), []string{"stable.save"}) {
		t.Fatalf("entries = %q", paths(report.Sources[0].Entries))
	}
}

func TestSourceStates(t *testing.T) {
	files := newFakeFS()
	files.put(install+"/saves/a.save", files.file("a"))
	files.put(install+"/config.ini", files.file("ini"))
	m := manifest(
		source("saves", "saves", true),
		source("required-missing", `missing\required`, true),
		source("optional-missing", "optional", false),
		source("not-a-directory", "config.ini", true),
	)
	report := run(t, files, m)
	expectFindings(t, report,
		`blocking source-missing required-missing D:\Games\Synthetic Game\missing`,
		`info source-missing optional-missing D:\Games\Synthetic Game\optional`,
		`blocking source-not-directory not-a-directory D:\Games\Synthetic Game\config.ini`,
	)
	if report.Blocking() != 2 {
		t.Fatalf("blocking = %d", report.Blocking())
	}
	statuses := []inspect.SourceStatus{inspect.StatusInventoried, inspect.StatusMissing, inspect.StatusMissing, inspect.StatusRefused}
	for i, want := range statuses {
		if report.Sources[i].Status != want {
			t.Fatalf("source %d status %s, want %s", i, report.Sources[i].Status, want)
		}
	}

	optionalOnly := run(t, files, manifest(source("saves", "saves", true), source("optional-missing", "optional", false)))
	if optionalOnly.Blocking() != 0 || len(optionalOnly.Findings) != 1 {
		t.Fatalf("optional absence must be information only: %q", codes(optionalOnly))
	}
}

func TestMissingVolume(t *testing.T) {
	files := newFakeFS()
	delete(files.volumes, `D:\`)
	report := run(t, files, manifest(source("saves", "saves", true), source("extra", "extra", false)))
	expectFindings(t, report, `blocking source-missing saves D:\`, `info source-missing extra D:\`)
}

func TestFatalErrors(t *testing.T) {
	files := newFakeFS()
	files.put(install+"/saves/a.save", files.file("a"))
	service := func(m fakeManifests, f *fakeFS) error {
		_, err := inspect.New(m, f).Inspect(context.Background(), inspect.Request{ManifestPath: "m.yaml"})
		return err
	}
	manifestErr := errors.New("m.yaml:1:1: schema_version: required field is missing")
	if err := service(fakeManifests{err: manifestErr}, files); !errors.Is(err, manifestErr) {
		t.Fatalf("manifest error: %v", err)
	}
	unsupported := newFakeFS()
	unsupported.scopeErr = errors.New("Windows paths are not supported on this system")
	if err := service(fakeManifests{manifest: manifest()}, unsupported); err == nil || !strings.Contains(err.Error(), `open volume D:\`) {
		t.Fatalf("scope error: %v", err)
	}
	// An 8.3 short name reaches the same directory under another spelling.
	short := newFakeFS()
	short.put(install+"/saves/a.save", short.file("a"))
	parent, _ := short.parentOf(install + "/saves")
	parent.children["SAVES~1"] = parent.children["saves"]
	err := service(fakeManifests{manifest: manifest(source("saves", "saves", true), source("short", "SAVES~1", true))}, short)
	if err == nil || !strings.Contains(err.Error(), "overlapping directories") {
		t.Fatalf("overlap by identity: %v", err)
	}
}

func TestCaseCollision(t *testing.T) {
	files := newFakeFS()
	files.put(install+"/saves/Slot.save", files.file("1"))
	files.put(install+"/saves/slot.save", files.file("2"))
	report := run(t, files, manifest())
	expectFindings(t, report, "blocking case-collision saves slot.save")
	if len(report.Sources[0].Entries) != 2 {
		t.Fatalf("both files must still be inventoried")
	}
}

func TestLargeAndDeepTrees(t *testing.T) {
	files := newFakeFS()
	for i := range 10000 {
		files.put(fmt.Sprintf("%s/saves/bulk/%05d.save", install, i), files.file("x"))
	}
	deep := install + "/saves"
	for i := range 60 {
		deep += fmt.Sprintf("/level-%02d", i)
	}
	files.put(deep+"/bottom.save", files.file("deep"))
	report := run(t, files, manifest())
	if len(report.Findings) != 0 || len(report.Sources[0].Entries) != 10001 || report.Sources[0].Bytes != 10004 {
		t.Fatalf("findings %q, entries %d, bytes %d", codes(report), len(report.Sources[0].Entries), report.Sources[0].Bytes)
	}
}

func TestCancellationNeverReturnsAReport(t *testing.T) {
	files := newFakeFS()
	files.put(install+"/saves/a.save", files.file("a"))
	ctx, cancel := context.WithCancel(context.Background())
	files.put(install+"/saves/b.save", files.file(strings.Repeat("b", 3<<20))).onRead = cancel
	files.put(install+"/saves/c.save", files.file("c"))
	report, err := inspect.New(fakeManifests{manifest: manifest()}, files).Inspect(ctx, inspect.Request{ManifestPath: "m.yaml"})
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "interrupted") {
		t.Fatalf("error = %v", err)
	}
	if !reflect.DeepEqual(report, inspect.Report{}) {
		t.Fatalf("a partial report was returned: %+v", report)
	}
	canceled, stop := context.WithCancel(context.Background())
	stop()
	if _, err := inspect.New(fakeManifests{manifest: manifest()}, files).Inspect(canceled, inspect.Request{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled context: %v", err)
	}
}

func TestFindingsFollowSourceOrder(t *testing.T) {
	files := newFakeFS()
	files.put(install+"/zz/link", files.special(fs.ModeSymlink))
	files.put(install+"/aa/link", files.special(fs.ModeSymlink))
	report := run(t, files, manifest(source("second", "zz", true), source("first", "aa", true)))
	expectFindings(t, report, "blocking link-not-followed second link", "blocking link-not-followed first link")
}
