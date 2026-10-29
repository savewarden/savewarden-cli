package backup_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

func TestMarkerRoundTripAndStrictness(t *testing.T) {
	data, err := backup.EncodeMarker(backup.Marker{CreatedAt: at, ToolVersion: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schema":"savewarden.vault","schema_version":1,"created_at":"2026-11-05T10:15:00Z","tool_version":"dev"}` + "\n"
	if string(data) != want {
		t.Fatalf("marker = %s", data)
	}
	if m, err := backup.DecodeMarker(data); err != nil || m.ToolVersion != "dev" || !m.CreatedAt.Equal(at) {
		t.Fatalf("decoded %+v, %v", m, err)
	}
	for _, bad := range []string{
		``, `{}`, `{"schema":"savewarden.vault","schema_version":2,"created_at":"2026-11-05T10:15:00Z","tool_version":"dev"}`,
		`{"schema":"savewarden.vault","schema_version":1,"created_at":"2026-11-05T10:15:00Z","tool_version":""}`,
		`{"schema":"savewarden.vault","schema_version":1,"created_at":"2026-11-05T10:15:00Z","tool_version":"dev","extra":true}`,
		`{"schema":"savewarden.vault","schema_version":1,"schema_version":1,"created_at":"2026-11-05T10:15:00Z","tool_version":"dev"}`,
		strings.TrimSuffix(want, "}\n"),
	} {
		if _, err := backup.DecodeMarker([]byte(bad)); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

// The initialization table of ADR 0006 §4, applied to a vault listing.
func TestClassifyVaultRoot(t *testing.T) {
	file := func(name string) backup.NamedEntry { return backup.NamedEntry{Name: name, Kind: backup.KindRegular} }
	dir := func(name string) backup.NamedEntry { return backup.NamedEntry{Name: name, Kind: backup.KindDirectory} }
	other := func(name string) backup.NamedEntry { return backup.NamedEntry{Name: name, Kind: backup.KindOther} }
	tests := []struct {
		name    string
		entries []backup.NamedEntry
		want    backup.VaultRoot
	}{
		{"empty", nil, backup.VaultRoot{State: backup.RootEmpty}},
		{"interrupted", []backup.NamedEntry{file("savewarden-vault.json.tmp-1"), file("savewarden-vault.json.tmp-2")},
			backup.VaultRoot{State: backup.RootInterrupted, Temporaries: 2, Taken: map[int]bool{1: true, 2: true}}},
		{"marked", []backup.NamedEntry{file("savewarden-vault.json"), file("savewarden-vault.json.tmp-1"), dir("games")},
			backup.VaultRoot{State: backup.RootMarked, Temporaries: 1, Taken: map[int]bool{1: true}}},
		{"marked with strays", []backup.NamedEntry{file("savewarden-vault.json"), file("notes.txt"), dir("Games")},
			backup.VaultRoot{State: backup.RootMarked, Unexpected: []string{"Games", "notes.txt"}}},
		{"temporaries and another file", []backup.NamedEntry{file("savewarden-vault.json.tmp-1"), file("photo.jpg")},
			backup.VaultRoot{State: backup.RootForeign}},
		{"games without marker", []backup.NamedEntry{dir("games")}, backup.VaultRoot{State: backup.RootForeign}},
		{"marker is a directory", []backup.NamedEntry{dir("savewarden-vault.json")}, backup.VaultRoot{State: backup.RootForeign}},
		{"marker is a link", []backup.NamedEntry{other("savewarden-vault.json")}, backup.VaultRoot{State: backup.RootForeign}},
		{"temporary as directory", []backup.NamedEntry{dir("savewarden-vault.json.tmp-1")}, backup.VaultRoot{State: backup.RootForeign}},
		{"tenth temporary", []backup.NamedEntry{file("savewarden-vault.json.tmp-10")}, backup.VaultRoot{State: backup.RootForeign}},
		{"foreign data", []backup.NamedEntry{file("1.save"), dir("current")}, backup.VaultRoot{State: backup.RootForeign}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := backup.ClassifyVaultRoot(tt.entries); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
