package sync

import "testing"

// F3: the references of a recovery come in part from a map of journals, in
// no order. Their comparison ignores the order and nothing else: a
// reference added, removed, renamed or counted another number of times is a
// difference.
func TestSameReferencesIgnoresTheOrderOnly(t *testing.T) {
	install := reference{label: "installation", path: `D:\Games\Synthetic Game`}
	saves := reference{label: "source saves", path: `D:\Games\Synthetic Game\saves`}
	backedUp := reference{label: "backed-up source saves", path: `D:\Games\Synthetic Game\saves`}
	for _, tt := range []struct {
		name string
		a, b []reference
		same bool
	}{
		{"same order", []reference{install, saves, backedUp}, []reference{install, saves, backedUp}, true},
		{"another order", []reference{install, saves, backedUp}, []reference{backedUp, install, saves}, true},
		{"one added", []reference{install, saves}, []reference{install, saves, backedUp}, false},
		{"one removed", []reference{install, saves, backedUp}, []reference{install, backedUp}, false},
		{"another path", []reference{install, saves}, []reference{install, {label: "source saves", path: `D:\Other\saves`}}, false},
		{"another label", []reference{install, saves}, []reference{install, {label: "source other", path: saves.path}}, false},
		{"counted twice", []reference{install, saves, saves}, []reference{install, install, saves}, false},
		{"both empty", nil, []reference{}, true},
	} {
		if got := sameReferences(tt.a, tt.b); got != tt.same {
			t.Errorf("%s: sameReferences = %v, want %v", tt.name, got, tt.same)
		}
	}
}
