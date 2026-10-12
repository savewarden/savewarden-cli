package manifests_test

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/entities/game"
	"github.com/savewarden/savewarden-cli/internal/platform/manifests"
)

func validText(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", "valid.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

var wantValid = game.Manifest{
	SchemaVersion: 1,
	Game:          game.Game{ID: "synthetic-game", Name: "Synthetic Game"},
	Installation:  game.Installation{Path: `D:\Games\Synthetic Game`},
	Sources: []game.Source{
		{ID: "saves", Kind: game.KindSave, Base: game.BaseGameInstall, Path: "saves", Required: true},
	},
}

func TestDecodeValid(t *testing.T) {
	text := validText(t)
	variants := map[string][]byte{
		"plain":         []byte(text),
		"bom and crlf":  append([]byte{0xEF, 0xBB, 0xBF}, strings.ReplaceAll(text, "\n", "\r\n")...),
		"flow sequence": []byte(strings.Split(text, "sources:")[0] + "sources: [{id: saves, kind: save, base: game-install, path: saves, required: true}]\n"),
		"comments":      []byte("# leading comment\n" + text + "# trailing comment\n"),
	}
	for name, data := range variants {
		t.Run(name, func(t *testing.T) {
			got, err := manifests.Decode("m.yaml", data)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, wantValid) {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestDecodeRejects(t *testing.T) {
	text := validText(t)
	replace := func(old, new string) string {
		if !strings.Contains(text, old) {
			t.Fatalf("fixture lacks %q", old)
		}
		return strings.Replace(text, old, new, 1)
	}
	tests := []struct {
		name string
		data string
		want string
	}{
		{"empty", "", "empty manifest"},
		{"comments only", "# nothing\n", "empty manifest"},
		{"two documents", text + "---\nschema_version: 1\n", "several YAML documents"},
		{"top-level list", "- schema_version\n", "expected a mapping"},
		{"unknown top field", text + "extra: 1\n", "extra: unknown field"},
		{"unknown game field", replace("  id: synthetic-game\n", "  id: synthetic-game\n  publisher: x\n"), "game.publisher: unknown field"},
		{"unknown source field", text + "    strategy: copy\n", "sources[0].strategy: unknown field"},
		{"duplicate top key", text + "schema_version: 1\n", "schema_version: duplicate key, first defined at line 2"},
		{"duplicate game key", replace("  id: synthetic-game\n", "  id: synthetic-game\n  id: other-game\n"), "game.id: duplicate key"},
		{"duplicate source key", text + "    path: other\n", "sources[0].path: duplicate key"},
		{"missing schema", replace("schema_version: 1\n", ""), "schema_version: required field is missing"},
		{"missing required flag", replace("    required: true\n", ""), "sources[0].required: required field is missing"},
		{"missing base", replace("    base: game-install\n", ""), "sources[0].base: required field is missing"},
		{"schema quoted", replace("schema_version: 1", `schema_version: "1"`), "expected a decimal integer"},
		{"schema float", replace("schema_version: 1", "schema_version: 1.0"), "expected a decimal integer"},
		{"schema hex", replace("schema_version: 1", "schema_version: 0x1"), "expected a decimal integer"},
		{"schema two", replace("schema_version: 1", "schema_version: 2"), "unsupported schema version 2"},
		{"bool yes", replace("required: true", "required: yes"), "expected true or false"},
		{"bool on", replace("required: true", "required: on"), "expected true or false"},
		{"bool title case", replace("required: true", "required: True"), "expected true or false"},
		{"bool number", replace("required: true", "required: 1"), "expected true or false"},
		{"bool quoted", replace("required: true", `required: "true"`), "expected true or false"},
		{"bool yaml 1.1 directive", "%YAML 1.1\n---\n" + replace("required: true", "required: yes"), "expected true or false"},
		{"name number", replace(`name: "Synthetic Game"`, "name: 1943"), "game.name: expected a string"},
		{"id bool", replace("id: synthetic-game", "id: true"), "game.id: expected a string"},
		{"path null", replace("    path: saves\n", "    path: ~\n"), "sources[0].path: expected a string"},
		{"game scalar", replace("game:\n  id: synthetic-game\n  name: \"Synthetic Game\"\n", "game: synthetic\n"), "game: expected a mapping"},
		{"sources mapping", strings.Split(text, "sources:")[0] + "sources: {}\n", "sources: expected a list"},
		{"sources empty", strings.Split(text, "sources:")[0] + "sources: []\n", "sources: at least one source"},
		{"non-string key", text + "1: x\n", "keys must be plain strings"},
		{"anchor", replace(`name: "Synthetic Game"`, `name: &n "Synthetic Game"`), "anchors are not allowed"},
		{"anchor and alias", strings.Replace(replace(`name: "Synthetic Game"`, `name: &n "Synthetic Game"`), "    path: saves\n", "    path: *n\n", 1), "anchors are not allowed"},
		{"undefined alias", replace("    path: saves\n", "    path: *n\n"), "invalid YAML"},
		{"merge key", replace("  id: synthetic-game\n", "  <<: {id: synthetic-game}\n"), "merge keys (<<) are not allowed"},
		{"binary tag", replace(`name: "Synthetic Game"`, "name: !!binary U3ludGhldGlj"), "explicit tags"},
		{"custom tag", replace("id: synthetic-game", "id: !custom synthetic-game"), "explicit tags"},
		{"str tag", replace("schema_version: 1", `schema_version: !!int "1"`), "explicit tags"},
		{"tab indentation", replace("  id: synthetic-game", "\tid: synthetic-game"), "invalid YAML"},
		{"long name", replace(`"Synthetic Game"`, `"`+strings.Repeat("x", 300)+`"`), "game.name: longer than 200"},
		{"invalid installation", replace(`'D:\Games\Synthetic Game'`, `'\\server\share'`), "installation.path: UNC and device paths"},
		{"traversal", replace("    path: saves\n", "    path: '..\\outside'\n"), `sources[0].path: "." and ".." segments`},
		{"utf8 nul", replace("Synthetic Game", "Synthetic\x00Game"), "NUL byte"},
		{"invalid utf8", replace("Synthetic Game", "Synthetic \xff"), "not valid UTF-8"},
		{"utf16 le bom", "\xff\xfes\x00", "UTF-16 is not supported"},
		{"utf16 be bom", "\xfe\xff\x00s", "UTF-16 is not supported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := manifests.Decode("m.yaml", []byte(tt.data))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if !strings.HasPrefix(err.Error(), "m.yaml") {
				t.Fatalf("error must name the manifest: %v", err)
			}
		})
	}
}

func TestDecodeErrorPosition(t *testing.T) {
	text := strings.Replace(validText(t), "    path: saves\n", "    path: 'saves\\..\\x'\n", 1)
	_, err := manifests.Decode("m.yaml", []byte(text))
	var manifestErr *manifests.Error
	if !errors.As(err, &manifestErr) {
		t.Fatalf("error = %v", err)
	}
	if manifestErr.Line != 15 || manifestErr.Column != 11 || manifestErr.Field != "sources[0].path" {
		t.Fatalf("position = %d:%d field %q", manifestErr.Line, manifestErr.Column, manifestErr.Field)
	}
	if !strings.HasPrefix(err.Error(), "m.yaml:15:11: sources[0].path: ") {
		t.Fatalf("message = %q", err)
	}
}

func TestDecodeSizeLimit(t *testing.T) {
	text := validText(t)
	padding := manifests.MaxSize - len(text) - 2
	atLimit := text + "#" + strings.Repeat("x", padding) + "\n"
	if len(atLimit) != manifests.MaxSize {
		t.Fatalf("fixture size %d", len(atLimit))
	}
	if _, err := manifests.Decode("m.yaml", []byte(atLimit)); err != nil {
		t.Fatalf("manifest at the limit rejected: %v", err)
	}
	if _, err := manifests.Decode("m.yaml", []byte(atLimit+"\n")); err == nil || !strings.Contains(err.Error(), "larger than 64 KiB") {
		t.Fatalf("oversized manifest: %v", err)
	}
}

func TestReader(t *testing.T) {
	got, err := manifests.Reader{}.Read(filepath.Join("testdata", "valid.yaml"))
	if err != nil || !reflect.DeepEqual(got, wantValid) {
		t.Fatalf("got %+v, %v", got, err)
	}
	dir := filepath.Join(t.TempDir(), "dossier synthétique")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := (manifests.Reader{}).Read(filepath.Join(dir, "missing.yaml")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing file: %v", err)
	}
	if _, err := (manifests.Reader{}).Read(dir); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("directory: %v", err)
	}
	large := filepath.Join(dir, "large.yaml")
	if err := os.WriteFile(large, bytes.Repeat([]byte("#\n"), manifests.MaxSize), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (manifests.Reader{}).Read(large); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("large file: %v", err)
	}
}

// The reference example of docs/manifest-v1.md must stay valid.
func TestDocumentedExample(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "manifest-v1.md"))
	if err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile("(?s)```yaml\n(.*?)```").FindAllStringSubmatch(string(content), -1)
	if len(blocks) == 0 {
		t.Fatal("no yaml example in docs/manifest-v1.md")
	}
	if _, err := manifests.Decode("docs/manifest-v1.md", []byte(blocks[0][1])); err != nil {
		t.Fatalf("documented example: %v", err)
	}
}
