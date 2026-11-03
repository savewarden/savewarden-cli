package sync

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"

	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/game"
)

func TestTargetNames(t *testing.T) {
	vault, err := vaultTarget(`D:\Vault`, "synthetic-game")
	if err != nil {
		t.Fatal(err)
	}
	volume, segments, err := game.ParseWindowsAbsolute(`D:\Vault`)
	if err != nil {
		t.Fatal(err)
	}
	if vault.volume != volume || !reflect.DeepEqual(vault.segments, segments) {
		t.Fatalf("root %q %v, want %q %v from the one path", vault.volume, vault.segments, volume, segments)
	}
	area := target{label: "destination", path: `G:\Area`, volume: "G:", segments: []string{"Area"}, dir: ".", lock: "restore.lock"}
	for _, tt := range []struct {
		t    target
		rel  string
		want string
	}{
		{vault, "journal/op", "games/synthetic-game/journal/op"},
		{vault, ".", "games/synthetic-game"},
		{vault, vault.lock, "games/synthetic-game/sync.lock"},
		{area, "journal/op", "journal/op"},
		{area, ".", "."},
		{area, area.lock, "restore.lock"},
	} {
		// Every name the engine builds is one the session accepts: no "./".
		if got := tt.t.at(tt.rel); got != tt.want || strings.HasPrefix(got, "./") {
			t.Errorf("%s at(%q) = %q, want %q", tt.t.label, tt.rel, got, tt.want)
		}
	}
	if got := vault.chain(); !reflect.DeepEqual(got, []string{"games", "games/synthetic-game"}) {
		t.Errorf("vault chain %v", got)
	}
	if got := area.chain(); got != nil {
		t.Errorf("area chain %v, want none: the root is held by the session", got)
	}
	if got, want := vault.windowsPath("receipts/op.json"), windowsPath(volume, append(append([]string{}, segments...), "games", "synthetic-game", "receipts", "op.json")); got != want {
		t.Errorf("vault path %q, want %q", got, want)
	}
	if got := area.errorf("%s", "refused").Error(); got != `destination G:\Area: refused` {
		t.Errorf("area error %q", got)
	}
	if got := vault.errorf("%s", "refused").Error(); got != errVault(`D:\Vault`, "%s", "refused").Error() {
		t.Errorf("vault error %q differs from errVault", got)
	}
	reads, err := vaultReadRoot(`D:\Vault`, "synthetic-game")
	if err != nil {
		t.Fatal(err)
	}
	if got := reads.game("."); !reflect.DeepEqual(got, append(append([]string{}, segments...), "games", "synthetic-game")) {
		t.Errorf("game dir %v", got)
	}
	if got := reads.scopeName("snapshots/receipts"); got != strings.Join(append(append([]string{}, segments...), "games", "synthetic-game", "snapshots", "receipts"), "/") {
		t.Errorf("scope name %q", got)
	}
	if _, err := vaultTarget(`relative\Vault`, "synthetic-game"); err == nil {
		t.Error("a relative vault path must be refused")
	}
}

// The engine never reads the plan of sync: only the own phases of sync do,
// in apply.go and copy.go. The helpers that a restore shares, and every
// file of the snapshot and restore operations, are checked on their syntax
// tree (ADR 0004, amendment B4).
func TestEngineNeverReadsThePlanOfSync(t *testing.T) {
	engine := map[string]bool{
		"open": true, "openVault": true, "record": true, "commitFile": true, "move": true, "fail": true,
		"ensureDir": true, "unfinished": true, "journalDir": true, "stagingDir": true, "at": true,
		"readBack": true, "readBackIn": true, "switchTo": true, "intervene": true, "wrote": true, "now": true,
		"copyContent": true, "copyFile": true, "verifyIncomplete": true,
	}
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	found := map[string]bool{}
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		whole := name != "apply.go" && name != "copy.go"
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if !whole && !engine[fn.Name.Name] {
				continue
			}
			found[fn.Name.Name] = true
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "plan" {
					if id, ok := sel.X.(*ast.Ident); ok && id.Name == "o" {
						t.Errorf("%s: %s reads o.plan", fset.Position(sel.Pos()), fn.Name.Name)
					}
				}
				return true
			})
		}
	}
	for name := range engine {
		if !found[name] {
			t.Errorf("engine helper %s not found: update the list", name)
		}
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("no entropy") }

type countingVaults struct {
	opens int
	err   error
}

func (v *countingVaults) Open(string) (Vault, error) {
	v.opens++
	return nil, v.err
}

type fixedClock struct{}

func (fixedClock) Now() time.Time                             { return time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC) }
func (fixedClock) Sleep(context.Context, time.Duration) error { return nil }

func TestOpenFailures(t *testing.T) {
	s := &Service{}
	vault, err := vaultTarget(`D:\Vault`, "synthetic-game")
	if err != nil {
		t.Fatal(err)
	}
	// No operation id, no session.
	vaults := &countingVaults{}
	_, err = s.open(context.Background(), vault, currentDomain, Writer{Vaults: vaults, Clock: fixedClock{}, Random: failingReader{}})
	if err == nil || !strings.HasPrefix(err.Error(), "operation id: ") || vaults.opens != 0 {
		t.Errorf("failing random: %v, %d opens", err, vaults.opens)
	}
	// A session that cannot open is reported against the target, as before.
	vaults = &countingVaults{err: errors.New("boom")}
	_, err = s.open(context.Background(), vault, currentDomain, Writer{Vaults: vaults, Clock: fixedClock{}, Random: strings.NewReader("abcd")})
	if err == nil || err.Error() != `vault D:\Vault: boom` || vaults.opens != 1 {
		t.Errorf("failing open: %v, %d opens", err, vaults.opens)
	}
}
