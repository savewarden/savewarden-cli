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
	"os/exec":    "process execution",
	"net":        "network access",
	"crypto/tls": "network access",
	"plugin":     "dynamic code loading",
	// Both reach APIs without naming them: go:linkname needs unsafe, and
	// reflect calls methods such as os.Root.Remove by name.
	"unsafe":  "unchecked memory access and go:linkname",
	"reflect": "calls by name that bypass this guard",
}

// Every package below net is network access too.
const forbiddenImportPrefix = "net/"

var forbiddenProductionSelectors = map[string]string{
	// File-system mutation through os, os.Root, io/fs, syscall or x/sys,
	// on Windows and on Unix.
	"Create": "file creation", "CreateTemp": "file creation", "MkdirTemp": "directory creation",
	"OpenFile": "open mode may create or write", "WriteFile": "file write", "CopyFS": "file write",
	"Mkdir": "directory creation", "MkdirAll": "directory creation", "Mkdirat": "directory creation",
	"Mknod": "file creation", "Mknodat": "file creation", "Mkfifo": "file creation",
	"Remove": "deletion", "RemoveAll": "deletion", "Unlink": "deletion", "Unlinkat": "deletion",
	"Rmdir": "deletion", "Undelete": "deletion change",
	"Rename": "rename", "Renameat": "rename", "Renameat2": "rename", "Exchangedata": "exchange",
	"Link": "link creation", "Linkat": "link creation", "Symlink": "link creation", "Symlinkat": "link creation",
	"Chmod": "metadata change", "Chown": "metadata change", "Lchown": "metadata change",
	"Fchmod": "metadata change", "Fchown": "metadata change", "Fchmodat": "metadata change", "Fchownat": "metadata change",
	"Chtimes": "metadata change", "Utimes": "metadata change", "UtimesNano": "metadata change",
	"UtimesNanoAt": "metadata change", "Futimes": "metadata change", "Futimesat": "metadata change",
	"Setxattr": "metadata change", "Lsetxattr": "metadata change", "Fsetxattr": "metadata change",
	"Removexattr": "metadata change", "Lremovexattr": "metadata change", "Fremovexattr": "metadata change",
	"Truncate": "file write", "Ftruncate": "file write",
	"MoveFile": "rename that may copy and delete", "MoveFileWithProgress": "rename that may copy and delete",
	"MoveFileTransacted": "transactional rename", "ReplaceFile": "replacement",
	"DeleteFile": "deletion", "RemoveDirectory": "deletion",
	"SetFileAttributes": "metadata change", "SetFileInformationByHandle": "rename or deletion by handle",
	"NtCreateFile": "raw file creation", "NtSetInformationFile": "rename or deletion by handle",
	"NtSetEaFile": "metadata change", "SetFileValidData": "file write",
	"SetSecurityInfo": "security change", "SetNamedSecurityInfo": "security change",
	"SetKernelObjectSecurity": "security change",
	"CreateSymbolicLink":      "link creation", "CreateHardLink": "link creation",
	"DeleteVolumeMountPoint": "mount point change", "SetVolumeMountPoint": "mount point change",
	"DefineDosDevice": "device name change", "SetVolumeLabel": "volume change",
	"Mount": "mount change", "Unmount": "mount change",
	"DeviceIoControl": "reparse point change", "SetEndOfFile": "file write",
	// Write-capable open modes, access rights and dispositions.
	"O_CREATE": "write-capable open", "O_CREAT": "write-capable open",
	"O_WRONLY": "write-capable open", "O_RDWR": "write-capable open",
	"O_TRUNC": "write-capable open", "O_APPEND": "write-capable open",
	"O_EXCL": "write-capable open", "GENERIC_ALL": "write-capable open",
	"FILE_GENERIC_WRITE": "write-capable open", "MAXIMUM_ALLOWED": "write-capable open",
	"STANDARD_RIGHTS_REQUIRED": "delete and security access", "STANDARD_RIGHTS_ALL": "delete and security access",
	"SPECIFIC_RIGHTS_ALL": "write-capable open", "ACCESS_SYSTEM_SECURITY": "security access",
	"FILE_APPEND_DATA": "write-capable open", "FILE_WRITE_DATA": "write-capable open",
	"FILE_WRITE_EA": "write-capable open", "DELETE": "delete access",
	"WRITE_DAC": "security change", "WRITE_OWNER": "security change",
	"CREATE_ALWAYS": "file creation", "OPEN_ALWAYS": "file creation",
	"TRUNCATE_EXISTING": "file write", "FILE_FLAG_DELETE_ON_CLOSE": "deletion on close",
	// Process execution, services, other processes, network.
	"Exec": "process execution", "ForkExec": "process execution",
	"StartProcess": "process execution", "CreateProcess": "process execution",
	"CreateProcessAsUser": "process execution", "ShellExecute": "process execution",
	"CreateService": "process execution", "StartService": "process execution",
	"ChangeServiceConfig": "process execution", "ChangeServiceConfig2": "process execution",
	"WriteProcessMemory": "process memory write",
	"Socket":             "network access", "Connect": "network access", "Bind": "network access",
	"Listen": "network access", "Sendto": "network access", "Sendmsg": "network access",
	"WSASocket": "network access", "WSAConnect": "network access", "WSASend": "network access",
	"WSASendTo": "network access", "WSASendMsg": "network access",
	// Dynamic loading of arbitrary system calls, and calls by name.
	"LoadDLL": "dynamic code loading", "MustLoadDLL": "dynamic code loading",
	"NewLazyDLL": "dynamic code loading", "NewLazySystemDLL": "dynamic code loading",
	"LazyDLL": "dynamic code loading", "LazyProc": "dynamic code loading",
	"DLL": "dynamic code loading", "Proc": "dynamic code loading", "NewProc": "dynamic code loading",
	"FindProc": "dynamic code loading", "MustFindProc": "dynamic code loading",
	"FindProcByOrdinal": "dynamic code loading", "MustFindProcByOrdinal": "dynamic code loading",
	"LoadLibrary": "dynamic code loading", "LoadLibraryEx": "dynamic code loading",
	"GetProcAddress": "dynamic code loading", "GetProcAddressByOrdinal": "dynamic code loading",
	"MethodByName": "call by name",
	"Syscall":      "raw system call", "Syscall6": "raw system call",
	"Syscall9": "raw system call", "Syscall12": "raw system call",
	"Syscall15": "raw system call", "Syscall18": "raw system call",
	"SyscallN": "raw system call", "RawSyscall": "raw system call",
	"RawSyscall6": "raw system call", "RawSyscallNoError": "raw system call",
}

// Every MOVEFILE_* flag is refused everywhere: replacement, copy across
// volumes, deferred move, hard link, write-through. MoveFileEx takes 0.
const forbiddenSelectorPrefix = "MOVEFILE_"

// vaultOnlySelectors are permitted in writePackage alone (ADR 0005), each
// for one need of the create-only protocol of ADR 0006.
var vaultOnlySelectors = map[string]string{
	// Opens a file for writing, with CREATE_NEW only (see openRules).
	"GENERIC_WRITE": "write-capable open",
	// Dates a file the vault creates, with CREATE_NEW only.
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

// The arguments of an open are checked by name, without literals or
// variables: the access rights and the disposition of CreateFile, and the
// flags of a three-argument Open or of Openat. Write access is permitted in
// writePackage alone, and only together with CREATE_NEW.
var (
	readAccess       = names("GENERIC_READ", "FILE_READ_ATTRIBUTES", "FILE_READ_DATA", "FILE_LIST_DIRECTORY", "READ_CONTROL", "SYNCHRONIZE")
	vaultWriteAccess = names("GENERIC_WRITE", "FILE_WRITE_ATTRIBUTES")
	readOpenFlags    = names("O_RDONLY", "O_NOFOLLOW", "O_CLOEXEC", "O_NONBLOCK", "O_DIRECTORY", "O_NOCTTY", "O_LARGEFILE")
)

func names(list ...string) map[string]bool {
	set := map[string]bool{}
	for _, name := range list {
		set[name] = true
	}
	return set
}

// selectorNames returns the selector names of an expression made only of
// selectors joined by |, such as windows.GENERIC_READ|windows.SYNCHRONIZE.
func selectorNames(expr ast.Expr) ([]string, bool) {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		return []string{e.Sel.Name}, true
	case *ast.ParenExpr:
		return selectorNames(e.X)
	case *ast.BinaryExpr:
		if e.Op != token.OR {
			return nil, false
		}
		left, ok := selectorNames(e.X)
		if !ok {
			return nil, false
		}
		right, ok := selectorNames(e.Y)
		return append(left, right...), ok
	}
	return nil, false
}

// openRules checks one call of CreateFile, Open or Openat and reports
// whether its selector was examined.
func openRules(call *ast.CallExpr, selector *ast.SelectorExpr, writes bool) (bool, []string) {
	allIn := func(arg ast.Expr, allowed ...map[string]bool) ([]string, bool) {
		found, ok := selectorNames(arg)
		for _, name := range found {
			permitted := false
			for _, set := range allowed {
				permitted = permitted || set[name]
			}
			ok = ok && permitted
		}
		return found, ok
	}
	spread := call.Ellipsis != token.NoPos
	switch name := selector.Sel.Name; {
	case name == "CreateFile":
		if len(call.Args) != 7 || spread {
			return true, []string{"selector .CreateFile: access and disposition must be explicit arguments"}
		}
		writeSet := map[string]bool{}
		if writes {
			writeSet = vaultWriteAccess
		}
		access, accessOK := allIn(call.Args[1], readAccess, writeSet)
		disposition, dispositionOK := selectorNames(call.Args[4])
		dispositionOK = dispositionOK && len(disposition) == 1 &&
			(disposition[0] == "OPEN_EXISTING" || writes && disposition[0] == "CREATE_NEW")
		var violations []string
		if !accessOK {
			violations = append(violations, "selector .CreateFile: access must be named read rights, or write rights in "+writePackage)
		}
		if !dispositionOK {
			violations = append(violations, "selector .CreateFile: disposition must be OPEN_EXISTING, or CREATE_NEW in "+writePackage)
		}
		for _, right := range access {
			if vaultWriteAccess[right] && (len(disposition) != 1 || disposition[0] != "CREATE_NEW") {
				violations = append(violations, "selector .CreateFile: write access "+right+" without CREATE_NEW")
			}
		}
		return true, violations
	case name == "Open" && len(call.Args) == 3, name == "Openat":
		flags := 1
		if name == "Openat" {
			flags = 2
		}
		if len(call.Args) <= flags || spread {
			return true, []string{"selector ." + name + ": flags must be an explicit argument"}
		}
		if _, ok := allIn(call.Args[flags], readOpenFlags); !ok {
			return true, []string{"selector ." + name + ": flags must be named read-only flags"}
		}
		return true, nil
	}
	return false, nil
}

// readOnlyViolations reports the guarded APIs of one production file of the
// package directory pkg, slash-separated and relative to the module root.
func readOnlyViolations(path, pkg string, src any) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, src, parser.SkipObjectResolution|parser.ParseComments)
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
		} else if strings.HasPrefix(target, forbiddenImportPrefix) {
			violations = append(violations, "import "+target+": network access")
		}
		if imported.Name != nil && imported.Name.Name == "." {
			violations = append(violations, "dot import "+target+": hides guarded selectors")
		}
	}
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if strings.HasPrefix(comment.Text, "//go:linkname") {
				violations = append(violations, "directive go:linkname: calls a function without naming it")
			}
		}
	}
	// A MoveFileEx selector is accepted only as the function of a direct
	// call whose flags are the literal 0: no variable, constant or
	// expression, and no function value that could be called otherwise.
	// CreateFile and Openat are accepted only in direct calls whose
	// arguments openRules can check.
	zeroFlags := map[*ast.SelectorExpr]bool{}
	checkedOpens := map[*ast.SelectorExpr]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if checked, found := openRules(call, selector, writes); checked {
			checkedOpens[selector] = true
			violations = append(violations, found...)
		}
		if selector.Sel.Name == "MoveFileEx" && len(call.Args) == 3 && call.Ellipsis == token.NoPos {
			if literal, ok := call.Args[2].(*ast.BasicLit); ok && literal.Kind == token.INT && literal.Value == "0" {
				zeroFlags[selector] = true
			}
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
		case (name == "CreateFile" || name == "Openat") && !checkedOpens[selector]:
			violations = append(violations, "selector ."+name+": must be called directly, with checkable arguments")
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

// compiledSource reports files the Go tool would build or link besides Go
// sources: assembly, cgo sources and object files bypass this guard.
func compiledSource(path string) bool {
	switch filepath.Ext(path) {
	case ".s", ".S", ".sx", ".syso", ".c", ".h", ".cc", ".cpp", ".cxx", ".hh", ".hpp", ".hxx",
		".m", ".f", ".F", ".for", ".f90", ".swig", ".swigcxx":
		return true
	}
	return false
}

func TestProductionCodeIsReadOnly(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, directory := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			if compiledSource(path) {
				t.Errorf("%s: non-Go source in production code", path)
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
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
		{"unlink", `package p; import "syscall"; func f() { _ = syscall.Unlink("x") }`, "selector .Unlink"},
		{"rmdir", `package p; import "syscall"; func f() { _ = syscall.Rmdir("x") }`, "selector .Rmdir"},
		{"write flag", `package p; import "syscall"; var _ = syscall.O_RDWR`, "selector .O_RDWR"},
		{"generic write", `package p; import "syscall"; var _ = syscall.GENERIC_WRITE`, "selector .GENERIC_WRITE"},
		{"generic write mask", `package p; import "golang.org/x/sys/windows"; var _ = windows.FILE_GENERIC_WRITE`, "selector .FILE_GENERIC_WRITE"},
		{"standard rights", `package p; import "syscall"; var _ = syscall.STANDARD_RIGHTS_ALL`, "selector .STANDARD_RIGHTS_ALL"},
		{"create disposition", `package p; import "syscall"; var _ = syscall.CREATE_ALWAYS`, "selector .CREATE_ALWAYS"},
		{"exec import", `package p; import "os/exec"; var _ = exec.Command`, "import os/exec"},
		{"start process", `package p; import "os"; func f() { _, _ = os.StartProcess("x", nil, nil) }`, "selector .StartProcess"},
		{"service", `package p; import "golang.org/x/sys/windows"; var _ = windows.CreateService`, "selector .CreateService"},
		{"lazy dll", `package p; import "syscall"; var _ = syscall.NewLazyDLL("x")`, "selector .NewLazyDLL"},
		{"lazy dll literal", `package p; import "syscall"; var _ = (&syscall.LazyDLL{Name: "kernel32.dll"}).NewProc("DeleteFileW")`, "selector .LazyDLL"},
		{"new proc", `package p; func f(d interface{ NewProc(string) any }) { _ = d.NewProc("x") }`, "selector .NewProc"},
		{"proc by ordinal", `package p; import "golang.org/x/sys/windows"; func f(d *windows.DLL) { _, _ = d.FindProcByOrdinal(1) }`, "selector .FindProcByOrdinal"},
		{"raw syscall", `package p; import "syscall"; var _ = syscall.Syscall15`, "selector .Syscall15"},
		{"socket", `package p; import "syscall"; var _ = syscall.Socket`, "selector .Socket"},
		{"network", `package p; import "net/http"; var _ = http.Get`, "import net/http"},
		{"network below net", `package p; import "net/smtp"; var _ = smtp.SendMail`, "import net/smtp"},
		{"tls", `package p; import "crypto/tls"; var _ = tls.Dial`, "import crypto/tls"},
		{"unsafe", `package p; import _ "unsafe"`, "import unsafe"},
		{"reflect", `package p; import "reflect"; var _ = reflect.ValueOf`, "import reflect"},
		{"call by name", `package p; func f(v interface{ MethodByName(string) any }) { _ = v.MethodByName("Remove") }`, "selector .MethodByName"},
		{"linkname", "package p\nimport _ \"unsafe\"\n//go:linkname move golang.org/x/sys/windows.MoveFileEx\nfunc move(a, b *uint16, f uint32) error", "directive go:linkname"},
		{"dot import", `package p; import . "os"; var _ = Getpid`, "dot import os"},
		{"numeric access", `package p; import "syscall"; func f(p *uint16) { _, _ = syscall.CreateFile(p, 0x40000000, 0, nil, 3, 0, 0) }`, "access must be named"},
		{"numeric disposition", `package p; import "syscall"; func f(p *uint16) { _, _ = syscall.CreateFile(p, syscall.GENERIC_READ, 0, nil, 2, 0, 0) }`, "disposition must be"},
		{"access in a variable", `package p; import "syscall"; func f(p *uint16, a uint32) { _, _ = syscall.CreateFile(p, a, 0, nil, syscall.OPEN_EXISTING, 0, 0) }`, "access must be named"},
		{"create file value", `package p; import "syscall"; var open = syscall.CreateFile`, "selector .CreateFile: must be called directly"},
		{"numeric open flags", `package p; import "syscall"; func f() { _, _ = syscall.Open("x", 0x241, 0) }`, "selector .Open: flags must be named"},
		{"openat", `package p; import "golang.org/x/sys/unix"; func f() { _, _ = unix.Openat(3, "x", 1, 0) }`, "selector .Openat: flags must be named"},
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
func f(b *strings.Builder, r *os.Root, p *uint16) {
	file, _ := os.Open("x")
	_, _ = io.Copy(sha256.New(), file)
	_, _ = r.Lstat("x")
	b.WriteString("text")
	_ = syscall.FILE_SHARE_WRITE | syscall.GENERIC_READ | syscall.O_RDONLY
	_, _ = syscall.CreateFile(p, syscall.GENERIC_READ, syscall.FILE_SHARE_READ, nil, syscall.OPEN_EXISTING, 0, 0)
	_, _ = syscall.Open("x", syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
}`
	for _, pkg := range []string{"internal/features/sync", writePackage} {
		if got := violationsOf(t, pkg, allowed); got != "" {
			t.Fatalf("%s: read-only code flagged: %q", pkg, got)
		}
	}
}

// adr0005Permitted is the list of ADR 0005, kept apart from the guard so
// that a change to the guard alone fails here.
var adr0005Permitted = []string{
	"GENERIC_WRITE", "FILE_WRITE_ATTRIBUTES", "CREATE_NEW", "CreateDirectory", "FlushFileBuffers", "SetFileTime", "MoveFileEx",
}

func TestVaultPermitsExactlyTheAPIsOfADR0005(t *testing.T) {
	var got []string
	for name := range vaultOnlySelectors {
		got = append(got, name)
	}
	want := append([]string(nil), adr0005Permitted...)
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("permitted in %s: %v, ADR 0005 says %v", writePackage, got, want)
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
	for _, name := range adr0005Permitted {
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
	h, err := windows.CreateFile(name, windows.GENERIC_WRITE|windows.FILE_READ_ATTRIBUTES, 0, nil, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
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

// adr0005Forbidden repeats, apart from the guard, the reinforced list of
// plan B2 §6.2 and ADR 0005, with the names the review of the relaxation
// added: each must stay refused in the vault too.
var adr0005Forbidden = []string{
	"Remove", "RemoveAll", "DeleteFile", "RemoveDirectory", "Rename", "MoveFile", "MoveFileWithProgress",
	"MoveFileTransacted", "ReplaceFile", "Truncate", "SetEndOfFile", "O_TRUNC", "O_APPEND", "CREATE_ALWAYS",
	"OPEN_ALWAYS", "TRUNCATE_EXISTING", "FILE_FLAG_DELETE_ON_CLOSE", "SetFileInformationByHandle", "Link", "Symlink",
	"CreateSymbolicLink", "CreateHardLink", "DeviceIoControl", "SetFileAttributes", "Chmod", "Chown",
	"StartProcess", "CreateProcess", "NewLazyDLL", "LoadDLL", "Syscall", "SyscallN",
	"Unlink", "Rmdir", "Unlinkat", "Renameat", "Mkdirat", "Mknod", "Fchmodat", "Fchownat", "Setxattr",
	"Removexattr", "Mount", "Unmount", "FILE_GENERIC_WRITE", "MAXIMUM_ALLOWED", "STANDARD_RIGHTS_REQUIRED",
	"STANDARD_RIGHTS_ALL", "SPECIFIC_RIGHTS_ALL", "ACCESS_SYSTEM_SECURITY", "DELETE", "WRITE_DAC", "WRITE_OWNER",
	"LazyDLL", "LazyProc", "DLL", "Proc", "NewProc", "FindProc", "FindProcByOrdinal", "GetProcAddressByOrdinal",
	"Syscall15", "Syscall18", "RawSyscall", "RawSyscall6", "CreateService", "StartService", "ChangeServiceConfig",
	"Socket", "Connect", "Sendto", "WSASend", "DefineDosDevice", "SetVolumeLabel", "SetKernelObjectSecurity",
	"WriteProcessMemory", "MethodByName", "NtCreateFile", "NtSetInformationFile",
}

// ADR 0005: the forbidden APIs stay forbidden in the vault too.
func TestVaultPackageKeepsTheForbiddenAPIs(t *testing.T) {
	refused := func(name string) {
		t.Helper()
		src := `package p; import "golang.org/x/sys/windows"; var _ = windows.` + name
		if got := violationsOf(t, writePackage, src); !strings.Contains(got, "selector ."+name+": ") {
			t.Errorf("%s accepted in %s: %q", name, writePackage, got)
		}
	}
	for _, name := range adr0005Forbidden {
		refused(name)
	}
	for name := range forbiddenProductionSelectors {
		refused(name)
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
	for _, imported := range []string{"os/exec", "net", "net/http", "net/rpc", "crypto/tls", "plugin", "unsafe", "reflect"} {
		src := `package p; import _ "` + imported + `"`
		if got := violationsOf(t, writePackage, src); !strings.Contains(got, "import "+imported) {
			t.Errorf("import %s accepted in %s: %q", imported, writePackage, got)
		}
	}
}

// ADR 0005: write access is tied to CREATE_NEW, in the vault alone.
func TestFileOpensAreChecked(t *testing.T) {
	call := func(access, disposition string) string {
		return `package p; import "golang.org/x/sys/windows"; func f(p *uint16) { _, _ = windows.CreateFile(p, ` +
			access + `, 0, nil, ` + disposition + `, windows.FILE_ATTRIBUTE_NORMAL, 0) }`
	}
	refused := map[string]string{
		"overwrite an existing file": call("windows.GENERIC_WRITE", "windows.OPEN_EXISTING"),
		"re-date an existing file":   call("windows.FILE_WRITE_ATTRIBUTES", "windows.OPEN_EXISTING"),
		"write access with read":     call("windows.GENERIC_READ|windows.GENERIC_WRITE", "windows.OPEN_EXISTING"),
		"numeric write access":       call("0x40000000", "windows.CREATE_NEW"),
		"numeric disposition":        call("windows.GENERIC_WRITE", "1"),
		"computed disposition":       call("windows.GENERIC_WRITE", "windows.CREATE_NEW|windows.OPEN_EXISTING"),
		"spread arguments":           `package p; import "golang.org/x/sys/windows"; func f(a []any) { _, _ = windows.CreateFile(a...) }`,
	}
	for name, src := range refused {
		if got := violationsOf(t, writePackage, src); !strings.Contains(got, "selector .CreateFile: ") {
			t.Errorf("%s accepted in %s: %q", name, writePackage, got)
		}
	}
	accepted := map[string]string{
		"create a new file":  call("windows.GENERIC_WRITE|windows.FILE_READ_ATTRIBUTES", "windows.CREATE_NEW"),
		"create a lock":      call("windows.GENERIC_READ", "windows.CREATE_NEW"),
		"open for reading":   call("windows.GENERIC_READ", "windows.OPEN_EXISTING"),
		"read an identity":   call("windows.FILE_READ_ATTRIBUTES", "windows.OPEN_EXISTING"),
		"parenthesized read": call("(windows.GENERIC_READ)", "(windows.OPEN_EXISTING)"),
	}
	for name, src := range accepted {
		if got := violationsOf(t, writePackage, src); got != "" {
			t.Errorf("%s refused in %s: %q", name, writePackage, got)
		}
	}
	if got := violationsOf(t, "internal/platform/filesystem", call("windows.GENERIC_READ", "windows.CREATE_NEW")); !strings.Contains(got, "disposition must be") {
		t.Errorf("CREATE_NEW accepted outside the vault: %q", got)
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

func TestCompiledSourcesAreRefused(t *testing.T) {
	for _, path := range []string{"x.s", "x.S", "x.syso", "x.c", "x.h", "x.cpp", "x.m", "x.swig"} {
		if !compiledSource(path) {
			t.Errorf("%s accepted", path)
		}
	}
	for _, path := range []string{"x.go", "x_test.go", "testdata/valid.yaml", "README.md"} {
		if compiledSource(path) {
			t.Errorf("%s refused", path)
		}
	}
}
