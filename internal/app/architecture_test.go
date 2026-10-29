package app_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/savewarden/savewarden-cli/"

func inPackage(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

// externalModules lists, per layer, the only third-party modules it may import.
// Every other package is limited to the standard library.
var externalModules = map[string][]string{
	"internal/cli":                {"github.com/spf13/cobra"},
	"internal/platform/manifests": {"go.yaml.in/yaml/v3"},
}

func isStandardLibrary(path string) bool {
	return !strings.Contains(strings.Split(path, "/")[0], ".") && path != "C"
}

func allowedExternal(source, target string) bool {
	for layer, modules := range externalModules {
		if !inPackage(source, layer) {
			continue
		}
		for _, module := range modules {
			if inPackage(target, module) {
				return true
			}
		}
	}
	return false
}

func allowedImport(source, target string) bool {
	if !strings.HasPrefix(target, modulePath) {
		if !isStandardLibrary(target) {
			return allowedExternal(source, target)
		}
		// Raw operating-system calls stay inside platform adapters.
		if target == "syscall" {
			return strings.HasPrefix(source, "internal/platform/")
		}
		return true
	}
	target = strings.TrimPrefix(target, modulePath)
	if inPackage(source, "cmd/savewarden") {
		return target == "internal/app"
	}
	if inPackage(source, "internal/app") {
		return strings.HasPrefix(target, "internal/")
	}
	if inPackage(source, "internal/cli") {
		return target == "internal/cli" || strings.HasPrefix(target, "internal/cli/") ||
			strings.HasPrefix(target, "internal/features/")
	}
	if strings.HasPrefix(source, "internal/features/") {
		parts := strings.Split(source, "/")
		slice := strings.Join(parts[:3], "/")
		return target == slice || strings.HasPrefix(target, slice+"/") ||
			strings.HasPrefix(target, "internal/entities/")
	}
	if strings.HasPrefix(source, "internal/entities/") {
		return strings.HasPrefix(target, "internal/entities/")
	}
	if strings.HasPrefix(source, "internal/platform/") {
		return strings.HasPrefix(target, "internal/platform/") ||
			strings.HasPrefix(target, "internal/entities/")
	}
	return false
}

func TestArchitectureImports(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, directory := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, filepath.Dir(path))
			if err != nil {
				return err
			}
			for _, imported := range file.Imports {
				target, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					return err
				}
				if !allowedImport(filepath.ToSlash(relative), target) {
					t.Errorf("forbidden import: %s -> %s", path, target)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	content, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(content), "module "+strings.TrimSuffix(modulePath, "/")+"\n") {
		t.Fatal("update architecture modulePath when changing module identity")
	}
}

func TestArchitectureImportRules(t *testing.T) {
	tests := []struct {
		source string
		target string
		allow  bool
	}{
		{"cmd/savewarden", modulePath + "internal/app", true},
		{"cmd/savewarden", modulePath + "internal/cli", false},
		{"internal/app", modulePath + "internal/platform/filesystem", true},
		{"internal/cli", modulePath + "internal/features/inspect", true},
		{"internal/cli", modulePath + "internal/app", false},
		{"internal/features/inspect", modulePath + "internal/entities/game", true},
		{"internal/features/inspect", modulePath + "internal/features/sync", false},
		{"internal/features/inspect", modulePath + "internal/features/inspect/private", true},
		{"internal/features/inspect", modulePath + "internal/features/inspection", false},
		{"internal/features/inspect", modulePath + "internal/platform/filesystem", false},
		{"internal/features/inspect", modulePath + "internal/cli", false},
		{"internal/features/inspect", "github.com/spf13/cobra", false},
		{"internal/entities/game", "path/filepath", true},
		{"internal/entities/game", "gopkg.in/yaml.v3", false},
		{"internal/entities/game", modulePath + "internal/app", false},
		{"internal/platform/filesystem", modulePath + "internal/entities/game", true},
		{"internal/platform/filesystem", modulePath + "internal/features/sync", false},
		{"internal/platform/manifests", "go.yaml.in/yaml/v3", true},
		{"internal/platform/manifests", "gopkg.in/yaml.v3", false},
		{"internal/platform/filesystem", "go.yaml.in/yaml/v3", false},
		{"internal/platform/filesystem", "syscall", true},
		{"internal/cli", "github.com/spf13/cobra", true},
		{"internal/cli", "go.yaml.in/yaml/v3", false},
		{"internal/app", "github.com/spf13/cobra", false},
		{"cmd/savewarden", "github.com/spf13/cobra", false},
		{"internal/features/inspect", "syscall", false},
		{"internal/features/inspect", "crypto/sha256", true},
		{"internal/entities/game", "syscall", false},
		{"internal/cli", "C", false},
		{"internal/cliff", modulePath + "internal/features/sync", false},
		// ADR 0004: shared inventory types live in entities/backup.
		{"internal/features/inspect", modulePath + "internal/entities/backup", true},
		{"internal/entities/backup", "crypto/sha256", true},
		{"internal/entities/backup", "encoding/json", true},
		{"internal/entities/backup", modulePath + "internal/entities/game", true},
		{"internal/entities/backup", modulePath + "internal/features/inspect", false},
		{"internal/entities/backup", modulePath + "internal/platform/filesystem", false},
		{"internal/entities/backup", "syscall", false},
		{"internal/cli", modulePath + "internal/entities/backup", false},
		// ADR 0004: sync reaches inspect only through its own port.
		{"internal/features/sync", modulePath + "internal/entities/backup", true},
		{"internal/features/sync", modulePath + "internal/entities/game", true},
		{"internal/features/sync", modulePath + "internal/features/inspect", false},
		{"internal/features/inspect", modulePath + "internal/features/sync", false},
		{"internal/features/sync", modulePath + "internal/platform/filesystem", false},
		{"internal/features/sync", modulePath + "internal/platform/vault", false},
		{"internal/features/sync", modulePath + "internal/app", false},
		{"internal/features/sync", "syscall", false},
		{"internal/features/sync", "github.com/spf13/cobra", false},
		{"internal/cli", modulePath + "internal/features/sync", true},
	}
	for _, tt := range tests {
		t.Run(tt.source+"->"+tt.target, func(t *testing.T) {
			if got := allowedImport(tt.source, tt.target); got != tt.allow {
				t.Fatalf("allowedImport = %v, want %v", got, tt.allow)
			}
		})
	}
}
