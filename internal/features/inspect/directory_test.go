package inspect_test

import (
	"context"
	"errors"
	"io/fs"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/features/inspect"
)

func directory(t *testing.T, files *fakeFS, volume string, segments ...string) backup.Directory {
	t.Helper()
	dir, err := inspect.New(nil, files).Directory(context.Background(), volume, segments)
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDirectoryInventoriesAnyTree(t *testing.T) {
	files := newFakeFS()
	files.put("Vault/games/g/current/saves/b.save", files.file("bravo"))
	files.put("Vault/games/g/current/saves/Slot 1/a.save", files.file("alpha"))
	files.put("Vault/games/g/current/saves/empty", files.dir())
	files.put("Vault/games/g/current/config/crypt.ini", files.file("secret")).crypted = true
	dir := directory(t, files, `D:\`, "Vault", "games", "g", "current")
	if dir.Status != backup.StatusInventoried || len(dir.Findings) != 0 || dir.Bytes != 16 {
		t.Fatalf("directory = %+v", dir)
	}
	if got := paths(dir.Entries); !slices.Equal(got, []string{"config/crypt.ini", "saves/Slot 1/a.save", "saves/b.save"}) {
		t.Fatalf("entries = %q", got)
	}
	if !slices.Equal(dir.Directories, []string{"config", "saves", "saves/Slot 1", "saves/empty"}) {
		t.Fatalf("directories = %q", dir.Directories)
	}
	if !slices.Equal(dir.Encrypted, []string{"config/crypt.ini"}) {
		t.Fatalf("encrypted = %q", dir.Encrypted)
	}
	if again := directory(t, files, `D:\`, "Vault", "games", "g", "current"); !reflect.DeepEqual(again, dir) {
		t.Fatal("two inventories of the same tree differ")
	}
}

// Directory and Inspect share one walk: the same source gives the same
// files, sizes, dates and hashes.
func TestDirectoryMatchesInspect(t *testing.T) {
	files := newFakeFS()
	files.put(install+"/saves/1.save", files.file("one"))
	files.put(install+"/saves/Profil été/2.save", files.file("two"))
	report := run(t, files, manifest())
	dir := directory(t, files, `D:\`, "Games", "Synthetic Game", "saves")
	if !reflect.DeepEqual(dir.Entries, report.Sources[0].Entries) || dir.Bytes != report.Sources[0].Bytes {
		t.Fatalf("directory %+v\ninspect %+v", dir.Entries, report.Sources[0].Entries)
	}
}

func TestDirectoryStates(t *testing.T) {
	files := newFakeFS()
	files.put("Vault/games/g/current/a.save", files.file("a"))
	files.put("Vault/games/g/current/jn", files.special(fs.ModeIrregular))
	files.put("Vault/file", files.file("x"))
	files.put("Linked/target/a.save", files.file("a"))
	parent, name := files.parentOf("Linked/target")
	parent.children[name].mode = fs.ModeIrregular | fs.ModeDir

	if dir := directory(t, files, `D:\`, "Vault", "games", "other", "current"); dir.Status != backup.StatusMissing || dir.Missing != `D:\Vault\games\other` {
		t.Fatalf("missing component: %+v", dir)
	}
	if dir := directory(t, files, `E:\`, "Vault"); dir.Status != backup.StatusMissing || dir.Missing != `E:\` {
		t.Fatalf("missing volume: %+v", dir)
	}
	refused := directory(t, files, `D:\`, "Linked", "target")
	if refused.Status != backup.StatusRefused || len(refused.Findings) != 1 || refused.Findings[0].Code != inspect.CodeLinkNotFollowed ||
		refused.Findings[0].Source != "" || refused.Findings[0].Path != `D:\Linked\target` || len(refused.Entries) != 0 {
		t.Fatalf("link on the way: %+v", refused)
	}
	if file := directory(t, files, `D:\`, "Vault", "file"); file.Status != backup.StatusRefused || file.Findings[0].Code != inspect.CodeSourceNotDirectory {
		t.Fatalf("file on the way: %+v", file)
	}
	inner := directory(t, files, `D:\`, "Vault", "games", "g", "current")
	if inner.Status != backup.StatusInventoried || len(inner.Findings) != 1 || inner.Findings[0].Path != "jn" || inner.Findings[0].Source != "" {
		t.Fatalf("link inside: %+v", inner)
	}
}

func TestDirectoryOfTheVolumeRoot(t *testing.T) {
	files := newFakeFS()
	files.put("top.save", files.file("t"))
	files.put("sub/low.save", files.file("l"))
	dir := directory(t, files, `D:\`)
	if dir.Status != backup.StatusInventoried || !slices.Equal(paths(dir.Entries), []string{"sub/low.save", "top.save"}) || !slices.Equal(dir.Directories, []string{"sub"}) {
		t.Fatalf("root = %+v", dir)
	}
}

func TestDirectoryErrors(t *testing.T) {
	files := newFakeFS()
	files.put("Vault/a.save", files.file(strings.Repeat("a", 3<<20)))
	ctx, cancel := context.WithCancel(context.Background())
	files.volumes[`D:\`].children["Vault"].children["a.save"].onRead = cancel
	dir, err := inspect.New(nil, files).Directory(ctx, `D:\`, []string{"Vault"})
	if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(dir, backup.Directory{}) {
		t.Fatalf("cancellation: %+v, %v", dir, err)
	}
	unsupported := newFakeFS()
	unsupported.scopeErr = errors.New("Windows paths are not supported on this system")
	if _, err := inspect.New(nil, unsupported).Directory(context.Background(), `D:\`, []string{"Vault"}); err == nil || !strings.Contains(err.Error(), `open volume D:\`) {
		t.Fatalf("scope error: %v", err)
	}
}
