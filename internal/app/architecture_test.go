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

func allowedImport(source, target string) bool {
	if !strings.HasPrefix(target, modulePath) {
		if inPackage(source, "internal/cli") || strings.HasPrefix(source, "internal/platform/") {
			return true
		}
		// Domain and composition packages depend only on the standard library.
		return !strings.Contains(strings.Split(target, "/")[0], ".") && target != "C"
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
		{"internal/cliff", modulePath + "internal/features/sync", false},
	}
	for _, tt := range tests {
		t.Run(tt.source+"->"+tt.target, func(t *testing.T) {
			if got := allowedImport(tt.source, tt.target); got != tt.allow {
				t.Fatalf("allowedImport = %v, want %v", got, tt.allow)
			}
		})
	}
}
