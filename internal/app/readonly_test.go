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
//
// ADR 0005 relaxes it for one package only, writePackage, which may create
// files and directories and rename them without replacement. The forbidden
// list applies to every package, writePackage included.

const writePackage = "internal/platform/vault"

var forbiddenProductionImports = map[string]string{
	"os/exec":  "process execution",
	"net":      "network access",
	"net/http": "network access",
	"plugin":   "dynamic code loading",
}

var forbiddenProductionSelectors = map[string]string{
	// File-system mutation through os, os.Root, io/fs, syscall or x/sys.
	"Create": "file creation", "CreateTemp": "file creation", "MkdirTemp": "directory creation",
	"OpenFile": "open mode may create or write", "WriteFile": "file write", "CopyFS": "file write",
	"Mkdir": "directory creation", "MkdirAll": "directory creation",
	"Remove": "deletion", "RemoveAll": "deletion", "Rename": "rename",
	"Link": "link creation", "Symlink": "link creation",
	"Chmod": "metadata change", "Chown": "metadata change", "Lchown": "metadata change",
	"Fchmod": "metadata change", "Fchown": "metadata change",
	"Chtimes": "metadata change", "Utimes": "metadata change", "UtimesNano": "metadata change",
	"Futimes": "metadata change", "Truncate": "file write", "Ftruncate": "file write",
	"MoveFile": "rename that may copy and delete", "MoveFileWithProgress": "rename that may copy and delete",
	"MoveFileTransacted": "transactional rename", "ReplaceFile": "replacement",
	"DeleteFile": "deletion", "RemoveDirectory": "deletion",
	"SetFileAttributes": "metadata change", "SetFileInformationByHandle": "rename or deletion by handle",
	"NtCreateFile": "raw file creation", "NtSetInformationFile": "rename or deletion by handle",
	"NtSetEaFile": "metadata change", "SetFileValidData": "file write",
	"SetSecurityInfo": "security change", "SetNamedSecurityInfo": "security change",
	"CreateSymbolicLink": "link creation", "CreateHardLink": "link creation",
	"DeleteVolumeMountPoint": "mount point change", "SetVolumeMountPoint": "mount point change",
	"DeviceIoControl": "reparse point change", "SetEndOfFile": "file write",
	// Write-capable open modes, access rights and dispositions.
	"O_CREATE": "write-capable open", "O_CREAT": "write-capable open",
	"O_WRONLY": "write-capable open", "O_RDWR": "write-capable open",
	"O_TRUNC": "write-capable open", "O_APPEND": "write-capable open",
	"O_EXCL": "write-capable open", "GENERIC_ALL": "write-capable open",
	"FILE_APPEND_DATA": "write-capable open", "FILE_WRITE_DATA": "write-capable open",
	"FILE_WRITE_EA": "write-capable open", "DELETE": "delete access",
	"WRITE_DAC": "security change", "WRITE_OWNER": "security change",
	"CREATE_ALWAYS": "file creation", "OPEN_ALWAYS": "file creation",
	"TRUNCATE_EXISTING": "file write", "FILE_FLAG_DELETE_ON_CLOSE": "deletion on close",
	// Process execution and dynamic loading of arbitrary system calls.
	"Exec": "process execution", "ForkExec": "process execution",
	"StartProcess": "process execution", "CreateProcess": "process execution",
	"CreateProcessAsUser": "process execution", "ShellExecute": "process execution",
	"LoadDLL": "dynamic code loading", "MustLoadDLL": "dynamic code loading",
	"NewLazyDLL": "dynamic code loading", "NewLazySystemDLL": "dynamic code loading",
	"LoadLibrary": "dynamic code loading", "LoadLibraryEx": "dynamic code loading",
	"GetProcAddress": "dynamic code loading",
	"Syscall":        "raw system call", "Syscall6": "raw system call",
	"Syscall9": "raw system call", "Syscall12": "raw system call",
	"SyscallN": "raw system call",
}

// Every MOVEFILE_* flag is refused everywhere: replacement, copy across
// volumes, deferred move, hard link, write-through. MoveFileEx takes 0.
const forbiddenSelectorPrefix = "MOVEFILE_"

// vaultOnlySelectors are permitted in writePackage alone (ADR 0005), each
// for one need of the create-only protocol of ADR 0006.
var vaultOnlySelectors = map[string]string{
	// Opens a file for writing, always together with CREATE_NEW.
	"GENERIC_WRITE": "write-capable open",
	// Sets the date of a file or directory the vault has just created.
	"FILE_WRITE_ATTRIBUTES": "write-capable open",
	// The only creating disposition: fails when the name exists.
	"CREATE_NEW": "file creation",
	// Creates one directory; fails when it exists.
	"CreateDirectory": "directory creation",
	// Makes a temporary durable before it is committed by rename.
	"FlushFileBuffers": "flush",
	// Copies the source modification date onto the vault's own file.
	"SetFileTime": "metadata change",
	// The only rename, with the literal flags 0: no replacement of an
	// existing target, no copy across volumes.
	"MoveFileEx": "rename",
}

// readOnlyViolations reports the guarded APIs of one production file of the
// package directory pkg, slash-separated and relative to the module root.
func readOnlyViolations(path, pkg string, src any) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	writes := pkg == writePackage
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
	// A MoveFileEx selector is accepted only as the function of a direct
	// call whose flags are the literal 0: no variable, constant or
	// expression, and no function value that could be called otherwise.
	zeroFlags := map[*ast.SelectorExpr]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) != 3 || call.Ellipsis != token.NoPos {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		literal, zero := call.Args[2].(*ast.BasicLit)
		if ok && selector.Sel.Name == "MoveFileEx" && zero && literal.Kind == token.INT && literal.Value == "0" {
			zeroFlags[selector] = true
		}
		return true
	})
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		name := selector.Sel.Name
		reason, vaultOnly := vaultOnlySelectors[name]
		switch {
		case strings.HasPrefix(name, forbiddenSelectorPrefix):
			violations = append(violations, "selector ."+name+": rename flag other than 0")
		case vaultOnly && !writes:
			violations = append(violations, "selector ."+name+": "+reason+" outside "+writePackage+" (ADR 0005)")
		case name == "MoveFileEx" && !zeroFlags[selector]:
			violations = append(violations, "selector .MoveFileEx: flags must be the literal 0 in a direct call")
		case !vaultOnly:
			if reason, found := forbiddenProductionSelectors[name]; found {
				violations = append(violations, "selector ."+name+": "+reason)
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
			pkg, err := filepath.Rel(root, filepath.Dir(path))
			if err != nil {
				return err
			}
			violations, err := readOnlyViolations(path, filepath.ToSlash(pkg), nil)
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

func violationsOf(t *testing.T, pkg, src string) string {
	t.Helper()
	violations, err := readOnlyViolations("guard.go", pkg, src)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(violations, "\n")
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
			packages := []string{"internal/features/sync", writePackage}
			if _, vaultOnly := vaultOnlySelectors[strings.TrimPrefix(tt.want, "selector .")]; vaultOnly {
				packages = packages[:1]
			}
			for _, pkg := range packages {
				if got := violationsOf(t, pkg, tt.src); !strings.Contains(got, tt.want) {
					t.Fatalf("%s: violations = %q, want %q", pkg, got, tt.want)
				}
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
	for _, pkg := range []string{"internal/features/sync", writePackage} {
		if got := violationsOf(t, pkg, allowed); got != "" {
			t.Fatalf("%s: read-only code flagged: %q", pkg, got)
		}
	}
}

// ADR 0005: the write APIs are permitted in internal/platform/vault alone,
// not in a neighbouring name, a sub-package or any other package.
func TestWriteAPIsStayInTheVaultPackage(t *testing.T) {
	use := func(name string) string {
		if name == "MoveFileEx" {
			return `package p; import "golang.org/x/sys/windows"; func f(a, b *uint16) error { return windows.MoveFileEx(a, b, 0) }`
		}
		return `package p; import "golang.org/x/sys/windows"; var _ = windows.` + name
	}
	for name := range vaultOnlySelectors {
		if got := violationsOf(t, writePackage, use(name)); got != "" {
			t.Errorf("%s refused in %s: %q", name, writePackage, got)
		}
		for _, pkg := range []string{
			"internal/platform/vaultx", "internal/platform/vault/inner", "internal/platform/filesystem",
			"internal/features/sync", "internal/app", "cmd/savewarden",
		} {
			if got := violationsOf(t, pkg, use(name)); !strings.Contains(got, "selector ."+name+": ") || !strings.Contains(got, "ADR 0005") {
				t.Errorf("%s accepted in %s: %q", name, pkg, got)
			}
		}
	}
	realistic := `package vault
import "golang.org/x/sys/windows"
func write(name, final *uint16, data []byte) error {
	h, err := windows.CreateFile(name, windows.GENERIC_WRITE, 0, nil, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	mtime := windows.NsecToFiletime(0)
	_ = windows.SetFileTime(h, nil, nil, &mtime)
	_ = windows.FlushFileBuffers(h)
	_ = windows.CreateDirectory(name, nil)
	return windows.MoveFileEx(name, final, 0)
}`
	if got := violationsOf(t, writePackage, realistic); got != "" {
		t.Fatalf("create-only code flagged: %q", got)
	}
}

// ADR 0005: the reinforced forbidden APIs stay forbidden in the vault too.
func TestVaultPackageKeepsTheForbiddenAPIs(t *testing.T) {
	for name, reason := range forbiddenProductionSelectors {
		src := `package p; import "golang.org/x/sys/windows"; var _ = windows.` + name
		if got := violationsOf(t, writePackage, src); !strings.Contains(got, "selector ."+name+": "+reason) {
			t.Errorf("%s accepted in %s: %q", name, writePackage, got)
		}
	}
	for _, flag := range []string{
		"MOVEFILE_REPLACE_EXISTING", "MOVEFILE_COPY_ALLOWED", "MOVEFILE_DELAY_UNTIL_REBOOT",
		"MOVEFILE_CREATE_HARDLINK", "MOVEFILE_WRITE_THROUGH", "MOVEFILE_FAIL_IF_NOT_TRACKABLE",
	} {
		src := `package p; import "golang.org/x/sys/windows"; var _ = windows.` + flag
		if got := violationsOf(t, writePackage, src); !strings.Contains(got, "selector ."+flag+": rename flag other than 0") {
			t.Errorf("%s accepted in %s: %q", flag, writePackage, got)
		}
	}
	for _, imported := range []string{"os/exec", "net", "net/http", "plugin"} {
		src := `package p; import _ "` + imported + `"`
		if got := violationsOf(t, writePackage, src); !strings.Contains(got, "import "+imported) {
			t.Errorf("import %s accepted in %s: %q", imported, writePackage, got)
		}
	}
}

// ADR 0005: MoveFileEx is accepted only as a direct call with the literal
// flags 0.
func TestMoveFileExTakesTheLiteralZero(t *testing.T) {
	refused := map[string]string{
		"flag one":            `windows.MoveFileEx(a, b, 1)`,
		"hexadecimal zero":    `windows.MoveFileEx(a, b, 0x0)`,
		"parenthesized zero":  `windows.MoveFileEx(a, b, (0))`,
		"expression":          `windows.MoveFileEx(a, b, 0|1)`,
		"variable":            `windows.MoveFileEx(a, b, flags)`,
		"named constant":      `windows.MoveFileEx(a, b, none)`,
		"replace flag":        `windows.MoveFileEx(a, b, windows.MOVEFILE_REPLACE_EXISTING)`,
		"spread arguments":    `windows.MoveFileEx(args...)`,
		"missing flags":       `windows.MoveFileEx(a, b)`,
		"function value":      `move := windows.MoveFileEx; _ = move(a, b, 0)`,
		"deferred with flags": `defer windows.MoveFileEx(a, b, 2)`,
	}
	for name, body := range refused {
		src := "package p\nimport \"golang.org/x/sys/windows\"\nconst none = 0\nfunc f(a, b *uint16, flags uint32, args []any) {\n" + body + "\n}"
		if got := violationsOf(t, writePackage, src); !strings.Contains(got, "selector .MoveFileEx: flags must be the literal 0") {
			t.Errorf("%s accepted: %q", name, got)
		}
	}
	for name, body := range map[string]string{
		"direct call":   `_ = windows.MoveFileEx(a, b, 0)`,
		"deferred call": `defer windows.MoveFileEx(a, b, 0)`,
	} {
		src := "package p\nimport \"golang.org/x/sys/windows\"\nfunc f(a, b *uint16) {\n" + body + "\n}"
		if got := violationsOf(t, writePackage, src); got != "" {
			t.Errorf("%s refused: %q", name, got)
		}
	}
}
