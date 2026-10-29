package game_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/entities/game"
)

func validManifest() game.Manifest {
	return game.Manifest{
		SchemaVersion: 1,
		Game:          game.Game{ID: "assassins-creed-odyssey", Name: "Assassin's Creed Odyssey"},
		Installation:  game.Installation{Path: `D:\Games\Assassins Creed - Odyssey`},
		Sources: []game.Source{
			{ID: "saves", Kind: game.KindSave, Base: game.BaseGameInstall, Path: "saves", Required: true},
		},
	}
}

func TestValidManifest(t *testing.T) {
	if err := validManifest().Validate(); err != nil {
		t.Fatal(err)
	}
	m := validManifest()
	m.Installation.Path = `e:\`
	m.Game.Name = "Jeu synthétique ✓ 1943"
	m.Sources = append(m.Sources,
		game.Source{ID: "profile-2", Kind: game.KindProfile, Base: game.BaseGameInstall, Path: `CPY_SAVES\Profil été\UPLAY`},
		game.Source{ID: "cfg", Kind: game.KindConfig, Base: game.BaseGameInstall, Path: `config.d\ini`},
		game.Source{ID: "keys", Kind: game.KindControls, Base: game.BaseGameInstall, Path: ` leading space`},
		game.Source{ID: "misc", Kind: game.KindOther, Base: game.BaseGameInstall, Path: `100%\done`},
	)
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestManifestRules(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*game.Manifest)
		field  string
		want   string
	}{
		{"schema zero", func(m *game.Manifest) { m.SchemaVersion = 0 }, "schema_version", "unsupported schema version 0"},
		{"schema two", func(m *game.Manifest) { m.SchemaVersion = 2 }, "schema_version", "unsupported schema version 2"},
		{"id empty", func(m *game.Manifest) { m.Game.ID = "" }, "game.id", "must not be empty"},
		{"id uppercase", func(m *game.Manifest) { m.Game.ID = "Odyssey" }, "game.id", "lowercase"},
		{"id space", func(m *game.Manifest) { m.Game.ID = "ac odyssey" }, "game.id", "lowercase"},
		{"id double hyphen", func(m *game.Manifest) { m.Game.ID = "ac--odyssey" }, "game.id", "single hyphens"},
		{"id dots", func(m *game.Manifest) { m.Game.ID = ".." }, "game.id", "lowercase"},
		{"id too long", func(m *game.Manifest) { m.Game.ID = strings.Repeat("a", 65) }, "game.id", "longer than 64"},
		{"name blank", func(m *game.Manifest) { m.Game.Name = "  " }, "game.name", "must not be empty"},
		{"name control", func(m *game.Manifest) { m.Game.Name = "a\nb" }, "game.name", "control characters"},
		{"name too long", func(m *game.Manifest) { m.Game.Name = strings.Repeat("é", 201) }, "game.name", "longer than 200"},
		{"name invalid utf8", func(m *game.Manifest) { m.Game.Name = "\xff" }, "game.name", "UTF-8"},
		{"no sources", func(m *game.Manifest) { m.Sources = nil }, "sources", "at least one"},
		{"source id", func(m *game.Manifest) { m.Sources[0].ID = "Saves" }, "sources[0].id", "lowercase"},
		{"duplicate id", func(m *game.Manifest) {
			m.Sources = append(m.Sources, game.Source{ID: "saves", Kind: game.KindSave, Base: game.BaseGameInstall, Path: "other"})
		}, "sources[1].id", "duplicate source id"},
		{"kind", func(m *game.Manifest) { m.Sources[0].Kind = "cache" }, "sources[0].kind", "unknown kind"},
		{"base", func(m *game.Manifest) { m.Sources[0].Base = "saved-games" }, "sources[0].base", "game-install only"},
		{"same path other case", func(m *game.Manifest) {
			m.Sources = append(m.Sources, game.Source{ID: "copy", Kind: game.KindSave, Base: game.BaseGameInstall, Path: "SAVES"})
		}, "sources[1].path", `overlaps source "saves"`},
		{"nested path", func(m *game.Manifest) {
			m.Sources = append(m.Sources, game.Source{ID: "deep", Kind: game.KindSave, Base: game.BaseGameInstall, Path: `Saves\CPY`})
		}, "sources[1].path", "overlaps"},
		{"parent path", func(m *game.Manifest) {
			m.Sources[0].Path = `saves\profile`
			m.Sources = append(m.Sources, game.Source{ID: "all", Kind: game.KindSave, Base: game.BaseGameInstall, Path: "saves"})
		}, "sources[1].path", "overlaps"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := validManifest()
			tt.mutate(&m)
			assertFieldError(t, m.Validate(), tt.field, tt.want)
		})
	}
}

func TestInstallationPathGrammar(t *testing.T) {
	rejected := map[string]string{
		`Games\Odyssey`:                  "drive letter",
		`\Games\Odyssey`:                 "drive letter",
		`D:Games`:                        "drive-relative",
		`D:`:                             "drive-relative",
		`\\server\share\Games`:           "UNC and device",
		`\\?\D:\Games`:                   "UNC and device",
		`\\.\D:\Games`:                   "UNC and device",
		`\??\D:\Games`:                   "UNC and device",
		`D:/Games/Odyssey`:               "backslashes",
		`D:\Games\..\Odyssey`:            `".."`,
		`D:\Games\.\Odyssey`:             `"."`,
		`D:\Games\\Odyssey`:              "empty segment",
		`D:\Games\Odyssey\`:              "empty segment",
		`%USERPROFILE%\Games`:            "environment variables",
		`D:\%GAMES%\Odyssey`:             "environment variables",
		`D:\Games\Odyssey:stream`:        "alternate data streams",
		`D:\Games\CON`:                   "reserved Windows device name",
		`D:\Games\nul.txt`:               "reserved Windows device name",
		`D:\Games\COM1 .log`:             "reserved Windows device name",
		`D:\Games\LPT¹`:                  "reserved Windows device name",
		`D:\Games\Odyssey.`:              "dot or a space",
		`D:\Games\Odyssey `:              "dot or a space",
		`D:\Ga<mes`:                      "not allowed",
		`D:\Ga|mes`:                      "not allowed",
		"D:\\Ga\tmes":                    "control characters",
		`D:\` + strings.Repeat("x", 256): "255 UTF-16",
		"D:\\\xff":                       "UTF-8",
	}
	for path, want := range rejected {
		t.Run(path, func(t *testing.T) {
			m := validManifest()
			m.Installation.Path = path
			assertFieldError(t, m.Validate(), "installation.path", want)
		})
	}
	accepted := []string{`D:\`, `z:\Games`, `C:\Program Files (x86)\Ubisoft\Odyssey`, `D:\Jeux été\オデッセイ`, `D:\` + strings.Repeat("x", 255), `D:\PROGRA~1\Game`, `D:\COM10\console`}
	for _, path := range accepted {
		t.Run("accept "+path, func(t *testing.T) {
			m := validManifest()
			m.Installation.Path = path
			if err := m.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSourcePathGrammar(t *testing.T) {
	rejected := map[string]string{
		``:                "must not be empty",
		`\saves`:          "leading backslash",
		`D:\saves`:        "drive letters",
		`D:saves`:         "drive letters",
		`..`:              `".."`,
		`..\outside`:      `".."`,
		`saves\..\..\x`:   `".."`,
		`.`:               `"."`,
		`saves\`:          "empty segment",
		`saves/profile`:   "backslashes",
		`saves:hidden`:    "alternate data streams",
		`saves\AUX`:       "reserved Windows device name",
		`saves\conout$`:   "reserved Windows device name",
		`saves.`:          "dot or a space",
		`%APPDATA%\saves`: "environment variables",
		`sa"ves`:          "not allowed",
		`saves?`:          "not allowed",
	}
	for path, want := range rejected {
		t.Run(path, func(t *testing.T) {
			m := validManifest()
			m.Sources[0].Path = path
			assertFieldError(t, m.Validate(), "sources[0].path", want)
		})
	}
}

func TestSourceSegments(t *testing.T) {
	m := validManifest()
	m.Sources[0].Path = `CPY_SAVES\Profile\UPLAY`
	volume, segments, err := m.SourceSegments(0)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Games", "Assassins Creed - Odyssey", "CPY_SAVES", "Profile", "UPLAY"}
	if volume != `D:\` || strings.Join(segments, "|") != strings.Join(want, "|") {
		t.Fatalf("volume %q segments %q", volume, segments)
	}
}

func assertFieldError(t *testing.T, err error, field, want string) {
	t.Helper()
	var fieldErr *game.FieldError
	if !errors.As(err, &fieldErr) {
		t.Fatalf("error = %v, want *FieldError", err)
	}
	if fieldErr.Field != field || !strings.Contains(fieldErr.Message, want) {
		t.Fatalf("error = %q, want field %q containing %q", err, field, want)
	}
}

func TestCheckWindowsName(t *testing.T) {
	for _, name := range []string{"1.save", "Profil été", "100%done%.save", "a b", ".hidden", "CONsole.txt"} {
		if err := game.CheckWindowsName(name); err != nil {
			t.Errorf("%q refused: %v", name, err)
		}
	}
	for _, name := range []string{"trailing.", "trailing ", "CON", "nul.txt", "COM1", "a:b", "a?b", "tab\tname", strings.Repeat("x", 256), ""} {
		if err := game.CheckWindowsName(name); err == nil {
			t.Errorf("%q accepted", name)
		}
	}
}
