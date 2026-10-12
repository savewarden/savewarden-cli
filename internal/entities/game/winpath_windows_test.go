package game_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/entities/game"
)

// On a Windows host, the manifest grammar must never accept a path that the
// operating system itself would read differently. This cross-checks the
// host-independent grammar against path/filepath on Windows only.
func TestGrammarAgreesWithWindows(t *testing.T) {
	absolute := []string{
		`D:\`, `D:\Games`, `C:\Program Files (x86)\Ubisoft`, `D:\Jeux été\オデッセイ`,
		`D:Games`, `\Games`, `\\server\share`, `\\?\D:\Games`, `D:\Games\..\x`,
		`D:\Games\.\x`, `D:\Games\\x`, `D:\Games\x.`, `D:\Games\CON`, `D:\Games\x:y`,
	}
	for _, path := range absolute {
		_, segments, err := game.ParseWindowsAbsolute(path)
		if err != nil {
			continue
		}
		if !filepath.IsAbs(path) || filepath.VolumeName(path) != path[:2] {
			t.Errorf("%q accepted but Windows does not see an absolute drive path", path)
		}
		if cleaned := filepath.Clean(path); !strings.EqualFold(cleaned, path) {
			t.Errorf("%q accepted but Windows cleans it to %q", path, cleaned)
		}
		if len(segments) > 0 && filepath.Join(append([]string{path[:3]}, segments...)...) != path {
			t.Errorf("%q segments do not rebuild the same path", path)
		}
	}
	relative := []string{
		`saves`, `CPY_SAVES\Profile\UPLAY`, ` leading`, `100%\done`,
		`..`, `..\x`, `.`, `\saves`, `D:saves`, `saves\`, `saves.`, `NUL`, `nul.txt`, `x:y`, `COM1 .log`,
	}
	for _, path := range relative {
		segments, err := game.ParseWindowsRelative(path)
		if err != nil {
			continue
		}
		if !filepath.IsLocal(path) {
			t.Errorf("%q accepted but filepath.IsLocal rejects it on Windows", path)
		}
		if cleaned := filepath.Clean(path); cleaned != path {
			t.Errorf("%q accepted but Windows cleans it to %q", path, cleaned)
		}
		if filepath.Join(segments...) != path {
			t.Errorf("%q segments do not rebuild the same path", path)
		}
	}
}
