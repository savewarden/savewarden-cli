package app_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The read-only guard is a static tripwire, not a proof: it rejects known
// mutating, executing and dynamic-loading APIs in production code. Dynamic
// non-mutation tests complement it for the behaviour actually shipped.

var forbiddenProductionImports = map[string]string{
	"os/exec":  "process execution",
	"net":      "network access",
	"net/http": "network access",
	"plugin":   "dynamic code loading",
}

var forbiddenProductionSelectors = map[string]string{
	// File-system mutation through os, os.Root, io/fs or syscall.
	"Create": "file creation", "CreateTemp": "file creation", "MkdirTemp": "directory creation",
	"OpenFile": "open mode may create or write", "WriteFile": "file write", "CopyFS": "file write",
	"Mkdir": "directory creation", "MkdirAll": "directory creation",
	"Remove": "deletion", "RemoveAll": "deletion", "Rename": "rename",
	"Link": "link creation", "Symlink": "link creation",
	"Chmod": "metadata change", "Chown": "metadata change", "Lchown": "metadata change",
	"Chtimes": "metadata change", "Truncate": "file write",
	"MoveFile": "rename", "DeleteFile": "deletion", "RemoveDirectory": "deletion",
	"CreateDirectory": "directory creation", "SetFileAttributes": "metadata change",
	"SetFileTime": "metadata change", "CreateSymbolicLink": "link creation",
	"CreateHardLink": "link creation", "DeviceIoControl": "reparse point change",
	"SetEndOfFile": "file write",
	// Write-capable open modes and dispositions.
	"O_CREATE": "write-capable open", "O_CREAT": "write-capable open",
	"O_WRONLY": "write-capable open", "O_RDWR": "write-capable open",
	"O_TRUNC": "write-capable open", "O_APPEND": "write-capable open",
	"O_EXCL": "write-capable open", "GENERIC_WRITE": "write-capable open",
	"GENERIC_ALL": "write-capable open", "FILE_APPEND_DATA": "write-capable open",
	"FILE_WRITE_ATTRIBUTES": "write-capable open", "CREATE_NEW": "file creation",
	"CREATE_ALWAYS": "file creation", "OPEN_ALWAYS": "file creation",
	"TRUNCATE_EXISTING": "file write",
	// Process execution and dynamic loading of arbitrary system calls.
	"Exec": "process execution", "ForkExec": "process execution",
	"StartProcess": "process execution", "CreateProcess": "process execution",
	"CreateProcessAsUser": "process execution", "ShellExecute": "process execution",
	"LoadDLL": "dynamic code loading", "MustLoadDLL": "dynamic code loading",
	"NewLazyDLL": "dynamic code loading", "LoadLibrary": "dynamic code loading",
	"GetProcAddress": "dynamic code loading",
	"Syscall":        "raw system call", "Syscall6": "raw system call",
	"Syscall9": "raw system call", "Syscall12": "raw system call",
	"SyscallN": "raw system call",
}

func readOnlyViolations(path string, src any) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	var violations []string
	for _, imported := range file.Imports {
		target, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			return nil, err
		}
		if reason, ok := forbiddenProductionImports[target]; ok {
			violations = append(violations, "import "+target+": "+reason)
		}
		if imported.Name != nil && imported.Name.Name == "." {
			violations = append(violations, "dot import "+target+": hides guarded selectors")
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		if selector, ok := node.(*ast.SelectorExpr); ok {
			if reason, found := forbiddenProductionSelectors[selector.Sel.Name]; found {
				violations = append(violations, "selector ."+selector.Sel.Name+": "+reason)
			}
		}
		return true
	})
	sort.Strings(violations)
	return violations, nil
}

func TestProductionCodeIsReadOnly(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, directory := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			violations, err := readOnlyViolations(path, nil)
			if err != nil {
				return err
			}
			for _, violation := range violations {
				t.Errorf("%s: %s", path, violation)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadOnlyGuardRules(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"write file", `package p; import "os"; func f() { _ = os.WriteFile("x", nil, 0) }`, "selector .WriteFile"},
		{"root remove", `package p; import "os"; func f(r *os.Root) { _ = r.Remove("x") }`, "selector .Remove"},
		{"open file", `package p; import "os"; func f() { _, _ = os.OpenFile("x", 0, 0) }`, "selector .OpenFile"},
		{"create", `package p; import "os"; func f() { _, _ = os.Create("x") }`, "selector .Create"},
		{"write flag", `package p; import "syscall"; var _ = syscall.O_RDWR`, "selector .O_RDWR"},
		{"generic write", `package p; import "syscall"; var _ = syscall.GENERIC_WRITE`, "selector .GENERIC_WRITE"},
		{"create disposition", `package p; import "syscall"; var _ = syscall.CREATE_ALWAYS`, "selector .CREATE_ALWAYS"},
		{"exec import", `package p; import "os/exec"; var _ = exec.Command`, "import os/exec"},
		{"start process", `package p; import "os"; func f() { _, _ = os.StartProcess("x", nil, nil) }`, "selector .StartProcess"},
		{"lazy dll", `package p; import "syscall"; var _ = syscall.NewLazyDLL("x")`, "selector .NewLazyDLL"},
		{"network", `package p; import "net/http"; var _ = http.Get`, "import net/http"},
		{"dot import", `package p; import . "os"; var _ = Getpid`, "dot import os"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations, err := readOnlyViolations("guard.go", tt.src)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.Join(violations, "\n"), tt.want) {
				t.Fatalf("violations = %q, want %q", violations, tt.want)
			}
		})
	}
	allowed := `package p
import ("crypto/sha256"; "io"; "os"; "strings"; "syscall")
func f(b *strings.Builder, r *os.Root) {
	file, _ := os.Open("x")
	_, _ = io.Copy(sha256.New(), file)
	_, _ = r.Lstat("x")
	b.WriteString("text")
	_ = syscall.FILE_SHARE_WRITE | syscall.GENERIC_READ | syscall.O_RDONLY
}`
	violations, err := readOnlyViolations("allowed.go", allowed)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("read-only code flagged: %q", violations)
	}
}
