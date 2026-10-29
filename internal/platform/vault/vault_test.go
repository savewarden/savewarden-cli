package vault

import "testing"

func TestNameGrammar(t *testing.T) {
	for _, name := range []string{".", "games", "games/synthetic-game/journal/20261105T101500-1a2b3c4d/001-planned.rec.tmp-1",
		"savewarden-vault.json.tmp-9", "Slot 1/2.save", "é/日本"} {
		if err := checkName(name); err != nil {
			t.Errorf("%q refused: %v", name, err)
		}
	}
	for _, name := range []string{"", "..", "/a", "a/", "a//b", "a/./b", "a/../b", `a\b`, "a:b", "CON", "lpt1.txt",
		"trailing.", "trailing ", "a/b.", "x\x01"} {
		if err := checkName(name); err == nil {
			t.Errorf("%q accepted", name)
		}
	}
}

func TestParentAndBelow(t *testing.T) {
	for name, want := range map[string]string{".": "", "games": ".", "games/g": "games", "games/g/current": "games/g"} {
		if got := parent(name); got != want {
			t.Errorf("parent(%q) = %q, want %q", name, got, want)
		}
	}
	for _, tt := range []struct {
		name, dir string
		want      bool
	}{
		{"games/g", ".", true}, {"games/g", "games/g", true}, {"games/g/current", "games/g", true},
		{"games/gx", "games/g", false}, {"games", "games/g", false}, {".", "games", false},
	} {
		if got := below(tt.name, tt.dir); got != tt.want {
			t.Errorf("below(%q, %q) = %v", tt.name, tt.dir, got)
		}
	}
}
