//go:build windows

package vault_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// HW13 without elevation (decision 23; protocol accepted by the owner on
// 22 November 2026 under conditions, corrected after the owner's review of
// blob 24e14441). The elevated CI runner creates an ephemeral standard
// local account, member of Users only, and runs a SHA-256-verified copy of
// this test binary under it, in its own logon session, by Start-Process
// -Credential. The child proves its token, its TEMP and its environment,
// then tries the two routes of HW13 on a reserved free letter. The
// password lives in memory and in the credential objects only: it reaches
// PowerShell on its standard input, never in an argument, a variable of the
// environment, a file or a log. No scheduled task is used.

const (
	hw13ChildVariable   = "SAVEWARDEN_HW13_CHILD"
	hw13AccountVariable = "SAVEWARDEN_HW13_ACCOUNT"
	hw13DomainVariable  = "SAVEWARDEN_HW13_DOMAIN"
	hw13TempVariable    = "SAVEWARDEN_HW13_TEMP"
	hw13ChildTimeout    = 5 * time.Minute
	// hw13ScriptTimeout bounds each PowerShell run from Go, start of the
	// session included; the run that waits for the child gets a minute more.
	hw13ScriptTimeout = 2 * time.Minute
	hw13UsersSID      = "S-1-5-32-545"
)

// hw13Dangerous lists the privileges an administrator token carries and a
// standard user's does not: any of them in the child's token voids the
// proof. The ordinary privileges of a standard user are accepted.
var hw13Dangerous = map[string]bool{
	"SeAssignPrimaryTokenPrivilege": true, "SeBackupPrivilege": true, "SeCreateTokenPrivilege": true,
	"SeDebugPrivilege": true, "SeImpersonatePrivilege": true, "SeLoadDriverPrivilege": true,
	"SeManageVolumePrivilege": true, "SeRestorePrivilege": true, "SeSecurityPrivilege": true,
	"SeSystemEnvironmentPrivilege": true, "SeTakeOwnershipPrivilege": true, "SeTcbPrivilege": true,
	"SeCreateSymbolicLinkPrivilege": true, "SeRelabelPrivilege": true, "SeDelegateSessionUserImpersonatePrivilege": true,
}

// hw13RunnerPrefixes are the variables of the runner the child must not
// inherit.
var hw13RunnerPrefixes = []string{"GITHUB_", "RUNNER_", "ACTIONS_", "INPUT_"}

func hw13Random(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		t.Fatal(err)
	}
	return b
}

// hw13Quote quotes a value for a single-quoted PowerShell string.
func hw13Quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// hw13Redact hides the SID of the account in what is logged.
func hw13Redact(text, sid string) string {
	if sid == "" {
		return text
	}
	return strings.ReplaceAll(text, sid, "<SID>")
}

// hw13PowerShell runs script within timeout, the start of any session it
// opens included, giving it stdin; on expiry the process is killed and the
// run is an error.
func hw13PowerShell(timeout time.Duration, script, stdin string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "pwsh", "-NoProfile", "-NonInteractive", "-Command", script) //nolint:gosec // G702: scripts written by this test; the password goes on stdin only.
	cmd.Stdin = strings.NewReader(stdin)
	cmd.WaitDelay = 10 * time.Second
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		err = errors.Join(fmt.Errorf("PowerShell timed out after %s", timeout), err)
	}
	return string(output), err
}

// hw13Fields reads the KEY=VALUE lines of a script's output, exactly: a key
// given twice is an error.
func hw13Fields(output string) (map[string]string, error) {
	fields := map[string]string{}
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || key == "" || strings.ContainsAny(key, " \t") {
			continue
		}
		if _, seen := fields[key]; seen {
			return nil, fmt.Errorf("%s given twice", key)
		}
		fields[key] = value
	}
	return fields, nil
}

func hw13CopyVerified(t *testing.T, from, to string) {
	t.Helper()
	data, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, data, 0o600); err != nil { //nolint:gosec // G703: the copy of the test binary in the dedicated space this test created.
		t.Fatal(err)
	}
	copied, err := os.ReadFile(to) //nolint:gosec // G703: the same copy, read back for its SHA-256.
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(copied) != sha256.Sum256(data) {
		t.Fatal("the copy of the test binary differs from the original")
	}
	t.Logf("test binary copied, SHA-256 %x", sha256.Sum256(data))
}

// File system rights, as Get-Acl reports them: read and execute, and modify,
// each with SYNCHRONIZE, as icacls grants RX and M.
const (
	hw13ReadExecute uint64 = 0x1200A9
	hw13Modify      uint64 = 0x1301BF
)

// hw13CheckACL reads the DACL of path entry by entry with Get-Acl, by SID,
// type and rights, and requires: only Administrators, SYSTEM and the
// account; allow entries only; for the account, rights exactly equal to
// want once its entries are joined, nothing beyond; inheritance removed
// when protected is asked.
func hw13CheckACL(t *testing.T, path, sid string, want uint64, protected bool) {
	t.Helper()
	output, err := hw13PowerShell(hw13ScriptTimeout, `$ErrorActionPreference = 'Stop'
$acl = Get-Acl -LiteralPath `+hw13Quote(path)+`
"PROTECTED=" + $acl.AreAccessRulesProtected
$i = 0
foreach ($rule in $acl.Access) {
  $ruleSid = $rule.IdentityReference.Translate([System.Security.Principal.SecurityIdentifier]).Value
  "ACE$i=" + $ruleSid + '|' + $rule.AccessControlType + '|' + (([int64]$rule.FileSystemRights) -band 4294967295)
  $i++
}
"COUNT=" + $i`, "")
	fields, ferr := hw13Fields(output)
	if err != nil || ferr != nil {
		t.Fatalf("Get-Acl %s: %v %v\n%s", path, err, ferr, hw13Redact(output, sid))
	}
	count, err := strconv.Atoi(fields["COUNT"])
	if err != nil || count == 0 {
		t.Fatalf("Get-Acl %s: %q entries", path, fields["COUNT"])
	}
	if protected && !strings.EqualFold(fields["PROTECTED"], "True") {
		t.Fatalf("ACL of %s still inherits from its parent", path)
	}
	var account uint64
	for i := 0; i < count; i++ {
		parts := strings.Split(fields["ACE"+strconv.Itoa(i)], "|")
		if len(parts) != 3 {
			t.Fatalf("ACL of %s: entry %d unreadable", path, i)
		}
		rights, err := strconv.ParseUint(parts[2], 10, 32)
		if err != nil || parts[1] != "Allow" {
			t.Fatalf("ACL of %s: entry %d is %s %s", path, i, parts[1], parts[2])
		}
		switch parts[0] {
		case "S-1-5-32-544", "S-1-5-18":
		case sid:
			account |= rights
		default:
			t.Fatalf("ACL of %s grants %#x to an unexpected principal", path, rights)
		}
	}
	if account != want {
		t.Fatalf("ACL of %s gives the account %#x, want exactly %#x", path, account, want)
	}
}

// TestHW13NonElevatedAccount is the parent: CI only, on a GitHub-hosted
// ephemeral Windows runner only. A failed preflight or start, a timeout, a
// child that fails or is skipped, or an incomplete cleanup fails the
// qualification.
func TestHW13NonElevatedAccount(t *testing.T) {
	hwCIOnly(t, "HW13 without elevation")
	if os.Getenv("RUNNER_ENVIRONMENT") != "github-hosted" {
		t.Fatalf("HW13 without elevation runs on a GitHub-hosted ephemeral runner only, not %q", os.Getenv("RUNNER_ENVIRONMENT"))
	}
	if !hwElevated() {
		t.Fatal("the parent needs the elevated runner to create the ephemeral account")
	}
	if os.Getenv(hw13ChildVariable) != "" {
		t.Fatal("the parent must not run inside the child")
	}
	other := os.Getenv("RUNNER_TEMP")
	if other == "" {
		t.Fatal("RUNNER_TEMP is required")
	}

	// Preflight: Start-Process -Environment needs PowerShell 7.4 or later.
	output, err := hw13PowerShell(hw13ScriptTimeout, `"VERSION=" + $PSVersionTable.PSVersion.ToString()`, "")
	fields, ferr := hw13Fields(output)
	if err != nil || ferr != nil {
		t.Fatalf("PowerShell preflight: %v %v\n%s", err, ferr, output)
	}
	var major, minor int
	if _, err := fmt.Sscanf(fields["VERSION"], "%d.%d", &major, &minor); err != nil || major < 7 || (major == 7 && minor < 4) {
		t.Fatalf("PowerShell %q: 7.4 or later is required for Start-Process -Environment (T3)", fields["VERSION"])
	}
	t.Logf("PowerShell %s", fields["VERSION"])

	letters := hwLetterState(t)
	name := "swhw13" + hex.EncodeToString(hw13Random(t, 4))
	password := base64.RawURLEncoding.EncodeToString(hw13Random(t, 24)) + "aA1!"
	dir := filepath.Join(other, "savewarden-hw13-"+hex.EncodeToString(hw13Random(t, 4)))
	if err := os.Mkdir(dir, 0o700); err != nil { //nolint:gosec // G703: a name this test drew below RUNNER_TEMP, on the ephemeral runner.
		t.Fatal(err)
	}

	// The account: local, Users only. Its SID is kept for the cleanup,
	// never logged. Membership is checked, not assumed.
	output, err = hw13PowerShell(hw13ScriptTimeout, `$ErrorActionPreference = 'Stop'
$secure = ConvertTo-SecureString ([Console]::In.ReadLine()) -AsPlainText -Force
$user = New-LocalUser -Name `+hw13Quote(name)+` -Password $secure -AccountNeverExpires -Description 'Savewarden HW13 test, ephemeral'
$sid = $user.SID.Value
$users = Get-LocalGroup -SID '`+hw13UsersSID+`'
if (-not (Get-LocalGroupMember -Group $users -ErrorAction SilentlyContinue | Where-Object { $_.SID.Value -eq $sid })) {
  Add-LocalGroupMember -Group $users -Member $user
}
$groups = foreach ($group in Get-LocalGroup) {
  if (Get-LocalGroupMember -Group $group -ErrorAction SilentlyContinue | Where-Object { $_.SID.Value -eq $sid }) { $group.SID.Value }
}
"SID=" + $sid
"DOMAIN=" + $env:COMPUTERNAME
"GROUPS=" + (($groups | Sort-Object) -join ',')`, password+"\n")
	fields, ferr = hw13Fields(output)
	sid, domain := fields["SID"], fields["DOMAIN"]
	t.Cleanup(func() { hw13Cleanup(t, name, sid, dir, letters) })
	if err != nil || ferr != nil || sid == "" || domain == "" {
		t.Fatalf("create the ephemeral account: %v %v\n%s", err, ferr, hw13Redact(output, sid))
	}
	if fields["GROUPS"] != hw13UsersSID {
		t.Fatalf("the ephemeral account belongs to %q, want Users only", fields["GROUPS"])
	}
	account := domain + `\` + name
	t.Logf("ephemeral standard account %s created, member of Users only", account)

	// The dedicated space: inheritance removed; Administrators and SYSTEM
	// full control, the account read and execute only, the binary below.
	// The child's TEMP alone is writable by the account.
	if out, err := exec.Command("icacls", dir, "/inheritance:r", "/grant:r", //nolint:gosec // G702: icacls on the dedicated space, for the account this test created.
		"*S-1-5-32-544:(OI)(CI)F", "*S-1-5-18:(OI)(CI)F", "*"+sid+":(OI)(CI)RX").CombinedOutput(); err != nil {
		t.Fatalf("icacls: %v %s", err, hw13Redact(string(out), sid))
	}
	childTemp := filepath.Join(dir, "tmp")
	if err := os.Mkdir(childTemp, 0o700); err != nil { //nolint:gosec // G703: below the dedicated space this test created.
		t.Fatal(err)
	}
	if out, err := exec.Command("icacls", childTemp, "/grant", "*"+sid+":(OI)(CI)M").CombinedOutput(); err != nil { //nolint:gosec // G702: icacls on the child's TEMP, for the account this test created.
		t.Fatalf("icacls: %v %s", err, hw13Redact(string(out), sid))
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	copied := filepath.Join(dir, "vault.test.exe")
	hw13CopyVerified(t, executable, copied)
	hw13CheckACL(t, dir, sid, hw13ReadExecute, true)
	hw13CheckACL(t, copied, sid, hw13ReadExecute, false)
	hw13CheckACL(t, childTemp, sid, hw13Modify, false)

	// The child: the runner's environment reduced to the system variables,
	// then the guards, the expected account and the child's TEMP added.
	stdout, stderr := filepath.Join(dir, "child.out"), filepath.Join(dir, "child.err")
	script := `$ErrorActionPreference = 'Stop'
$secure = ConvertTo-SecureString ([Console]::In.ReadLine()) -AsPlainText -Force
$credential = [pscredential]::new(` + hw13Quote(account) + `, $secure)
# Load Start-Process before PSModulePath goes with the runner's variables.
$null = Get-Command Start-Process
$keep = 'SystemRoot', 'windir', 'SystemDrive', 'ComSpec', 'PATHEXT', 'Path', 'OS', 'PROCESSOR_ARCHITECTURE', 'NUMBER_OF_PROCESSORS', 'COMPUTERNAME'
foreach ($variable in Get-ChildItem Env:) {
  if ($keep -notcontains $variable.Name) { Remove-Item -LiteralPath ('Env:' + $variable.Name) }
}
$process = Start-Process -FilePath ` + hw13Quote(copied) + ` -ArgumentList '-test.run=^TestHW13NonElevatedChild$', '-test.v', '-test.count=1' ` +
		`-Credential $credential -LoadUserProfile -WorkingDirectory ` + hw13Quote(childTemp) +
		` -RedirectStandardOutput ` + hw13Quote(stdout) + ` -RedirectStandardError ` + hw13Quote(stderr) +
		` -Environment @{ ` + hw13ChildVariable + ` = '1'; ` + hw13AccountVariable + ` = ` + hw13Quote(name) +
		`; ` + hw13DomainVariable + ` = ` + hw13Quote(domain) + `; ` + hw13TempVariable + ` = ` + hw13Quote(childTemp) +
		`; TEMP = ` + hw13Quote(childTemp) + `; TMP = ` + hw13Quote(childTemp) + ` } -PassThru
$null = $process.Handle
if (-not $process.WaitForExit(` + strconv.FormatInt(hw13ChildTimeout.Milliseconds(), 10) + `)) {
  Stop-Process -Id $process.Id -Force
  "TIMEOUT=1"
  exit 3
}
"EXIT=" + $process.ExitCode`
	output, err = hw13PowerShell(hw13ChildTimeout+time.Minute, script, password+"\n")
	childOut, _ := os.ReadFile(stdout) //nolint:gosec // G703: the child output file this test chose in its dedicated space.
	childErr, _ := os.ReadFile(stderr) //nolint:gosec // G703: the child error file this test chose in its dedicated space.
	t.Logf("child standard output:\n%s", hw13Redact(string(childOut), sid))
	if len(childErr) > 0 {
		t.Logf("child standard error:\n%s", hw13Redact(string(childErr), sid))
	}
	fields, ferr = hw13Fields(output)
	if err != nil || ferr != nil || fields["TIMEOUT"] != "" || fields["EXIT"] != "0" {
		t.Fatalf("the child under the standard account did not succeed: %v %v\n%s", err, ferr, hw13Redact(output, sid))
	}
	lines := strings.Split(strings.ReplaceAll(string(childOut), "\r\n", "\n"), "\n")
	passed, failed, routes := false, false, 0
	for _, line := range lines {
		switch trimmed := strings.TrimSpace(line); {
		case strings.HasPrefix(trimmed, "--- PASS: TestHW13NonElevatedChild "):
			passed = true
		case strings.HasPrefix(trimmed, "--- FAIL:"), strings.HasPrefix(trimmed, "--- SKIP:"):
			failed = true
		case strings.HasSuffix(trimmed, "HW13 child route subst: true"), strings.HasSuffix(trimmed, "HW13 child route DefineDosDevice: true"):
			routes++
		}
	}
	if !passed || failed || routes == 0 {
		t.Fatalf("the child did not pass with a route: pass %t, fail or skip %t, routes %d; a skip is not a proof", passed, failed, routes)
	}
	t.Logf("HW13 without elevation: proven by the child under %s, %d route(s)", account, routes)
}

// hw13Cleanup stops the account's processes, deletes its profile and the
// account by their exact SID, removes the dedicated space, and fails the
// test if any of them is left. It is registered as soon as the account may
// exist, so it runs on every path, failures included.
func hw13Cleanup(t *testing.T, name, sid, dir string, letters string) {
	t.Helper()
	if sid == "" {
		// The account may exist without its SID read back: remove it by its
		// exact name, which the test drew at random.
		script := `$user = Get-LocalUser -Name ` + hw13Quote(name) + ` -ErrorAction SilentlyContinue
if ($user) { Remove-LocalUser -SID $user.SID.Value -ErrorAction Stop }
"ACCOUNT=" + $(if (Get-LocalUser -Name ` + hw13Quote(name) + ` -ErrorAction SilentlyContinue) { 'present' } else { 'absent' })`
		output, err := hw13PowerShell(hw13ScriptTimeout, script, "")
		fields, ferr := hw13Fields(output)
		if err != nil || ferr != nil || fields["ACCOUNT"] != "absent" {
			t.Errorf("delete the ephemeral account %s: %v %v\n%s", name, err, ferr, output)
		}
	} else {
		script := `$ErrorActionPreference = 'Stop'
$sid = ` + hw13Quote(sid) + `
# Owned lists the processes of the account. A process whose owner cannot be
# read counts as ended only if the same process (identifier and creation
# date) is gone; otherwise the read failed, and the proof of zero process is
# void. The kernel pseudo-processes, which have no token, are skipped.
function Owned {
  $owned = @(); $unreadable = @()
  foreach ($process in Get-CimInstance Win32_Process) {
    if ($process.ProcessId -in 0, 4 -or $process.Name -in 'Secure System', 'Registry', 'Memory Compression') { continue }
    $owner = $null
    try { $owner = Invoke-CimMethod -InputObject $process -MethodName GetOwnerSid -ErrorAction Stop } catch { $owner = $null }
    if ($owner -and $owner.ReturnValue -eq 0) {
      if ($owner.Sid -eq $sid) { $owned += $process }
      continue
    }
    $still = Get-CimInstance Win32_Process -Filter ('ProcessId=' + $process.ProcessId) | Where-Object { $_.CreationDate -eq $process.CreationDate }
    if ($still) { $unreadable += $process.ProcessId }
  }
  [pscustomobject]@{ Owned = $owned; Unreadable = $unreadable }
}
# Only the processes of the ephemeral SID are ever stopped.
foreach ($process in (Owned).Owned) { Stop-Process -Id $process.ProcessId -Force -ErrorAction SilentlyContinue }
Start-Sleep -Seconds 2
$userProfile = Get-CimInstance Win32_UserProfile | Where-Object SID -eq $sid
if ($userProfile) { Remove-CimInstance -InputObject $userProfile }
if (Get-LocalUser -SID $sid -ErrorAction SilentlyContinue) { Remove-LocalUser -SID $sid }
$last = Owned
"PROCESSES=" + @($last.Owned).Count
"UNREADABLE=" + @($last.Unreadable).Count
"PROFILE=" + $(if (Get-CimInstance Win32_UserProfile | Where-Object SID -eq $sid) { 'present' } else { 'absent' })
"ACCOUNT=" + $(if (Get-LocalUser -SID $sid -ErrorAction SilentlyContinue) { 'present' } else { 'absent' })`
		output, err := hw13PowerShell(hw13ScriptTimeout, script, "")
		fields, ferr := hw13Fields(output)
		if err != nil || ferr != nil || fields["PROCESSES"] != "0" || fields["UNREADABLE"] != "0" ||
			fields["PROFILE"] != "absent" || fields["ACCOUNT"] != "absent" {
			t.Errorf("delete the ephemeral account %s, its processes and its profile: %v %v\n%s", name, err, ferr, hw13Redact(output, sid))
		} else {
			t.Logf("ephemeral account %s, its processes and its profile deleted", name)
		}
	}
	if err := os.RemoveAll(dir); err != nil { //nolint:gosec // G703: the dedicated space this test created below RUNNER_TEMP.
		t.Errorf("remove the dedicated space: %v", err)
	}
	if got := hwLetterState(t); got != letters {
		t.Errorf("drive letters A to L of the runner changed:\nbefore %s\nafter  %s", letters, got)
	}
}

// hw13WhoAmI returns the CSV rows of whoami for one option.
func hw13WhoAmI(t *testing.T, option string) [][]string {
	t.Helper()
	output, err := exec.Command("whoami", option, "/fo", "csv", "/nh").Output()
	if err != nil {
		t.Fatalf("whoami %s: %v", option, err)
	}
	rows, err := csv.NewReader(bytes.NewReader(output)).ReadAll()
	if err != nil {
		t.Fatalf("whoami %s: %v", option, err)
	}
	return rows
}

// TestHW13NonElevatedChild runs only under the ephemeral account, started by
// TestHW13NonElevatedAccount: elsewhere it is skipped, which the parent
// refuses as a proof. A route that Windows refuses is recorded and the
// other still tried; a failed proof of the context or a cleanup left
// incomplete fails at once.
func TestHW13NonElevatedChild(t *testing.T) {
	if os.Getenv(hw13ChildVariable) != "1" {
		t.Skip("runs only under the ephemeral account of TestHW13NonElevatedAccount")
	}
	// 1. The effective token: the local account created, not elevated,
	// medium integrity, Administrators not active, no administrative
	// privilege.
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	account, domain, _, err := user.User.Sid.LookupAccount("")
	if err != nil {
		t.Fatal(err)
	}
	want, wantDomain := os.Getenv(hw13AccountVariable), os.Getenv(hw13DomainVariable)
	if want == "" || wantDomain == "" || !strings.EqualFold(account, want) || !strings.EqualFold(domain, wantDomain) {
		t.Fatalf("token of %s\\%s, want the local account %s\\%s", domain, account, wantDomain, want)
	}
	if token.IsElevated() {
		t.Fatal("the token is elevated")
	}
	administrators, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		t.Fatal(err)
	}
	if member, err := token.IsMember(administrators); err != nil || member {
		t.Fatalf("Administrators active in the token: %t, %v", member, err)
	}
	medium := false
	for _, row := range hw13WhoAmI(t, "/groups") {
		if len(row) > 2 && row[2] == "S-1-16-8192" {
			medium = true
		}
		if len(row) > 2 && (row[2] == "S-1-16-12288" || row[2] == "S-1-16-16384") {
			t.Fatalf("integrity level %s", row[2])
		}
	}
	if !medium {
		t.Fatal("the token has no medium integrity level")
	}
	var privileges []string
	for _, row := range hw13WhoAmI(t, "/priv") {
		if len(row) == 0 {
			continue
		}
		if hw13Dangerous[row[0]] {
			t.Fatalf("administrative privilege %s in the token", row[0])
		}
		privileges = append(privileges, row[0])
	}

	// 2. The TEMP and the environment: the folder given, writable by the
	// account; no variable of the runner.
	wantTemp := os.Getenv(hw13TempVariable)
	if wantTemp == "" || os.TempDir() != wantTemp || os.Getenv("TMP") != wantTemp {
		t.Fatalf("TEMP %q and TMP %q, want %q", os.TempDir(), os.Getenv("TMP"), wantTemp)
	}
	for _, variable := range os.Environ() {
		for _, prefix := range hw13RunnerPrefixes {
			if strings.HasPrefix(strings.ToUpper(variable), prefix) {
				name, _, _ := strings.Cut(variable, "=")
				t.Fatalf("the child inherited the runner's variable %s", name)
			}
		}
	}
	t.Logf("HW13 child context: local account of the test, not elevated, medium integrity, Administrators not active; privileges %s; TEMP the folder given; no runner variable",
		strings.Join(privileges, ", "))

	// 3. The two routes, each on its own, on a reserved free letter of this
	// logon session; the 26 letters of the session recorded before and
	// required identical after.
	allLetters := hwLetterStateOf(t, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	t.Cleanup(func() {
		if got := hwLetterStateOf(t, "ABCDEFGHIJKLMNOPQRSTUVWXYZ"); got != allLetters {
			t.Errorf("drive letters A to Z of the child's session changed:\nbefore %s\nafter  %s", allLetters, got)
		}
	})
	dir := hwTemp(t)
	volume := filepath.VolumeName(dir)
	guidRoot, err := hwVolumeGUID(volume + `\`)
	if err != nil {
		t.Fatal(err)
	}
	devices, err := hwQueryDosDevice(volume)
	if err != nil || len(devices) == 0 {
		t.Fatalf("QueryDosDevice %s: %v %v", volume, devices, err)
	}
	sub := filepath.Join(dir, "HW13", "sub")
	hwMkdir(t, sub)
	letter := hwReserveLetter(t)
	routes := []struct {
		name   string
		toRoot func() error
		toDir  func() error
	}{
		{"subst", func() error { return letter.substErr(volume + `\`) }, func() error { return letter.substErr(sub) }},
		{"DefineDosDevice", func() error { return letter.defineErr(devices[0]) }, func() error { return letter.defineErr(`\??\` + sub) }},
	}
	var outcomes []hw13Outcome
	for _, r := range routes {
		outcome := hw13Route(t, r.name, letter, r.toRoot, r.toDir, volume, guidRoot)
		t.Logf("HW13 child %s: %s", r.name, outcome.reason)
		t.Logf("HW13 child route %s: %t", r.name, outcome.passes())
		outcomes = append(outcomes, outcome)
	}
	switch verdict, why := hw13Verdict(outcomes); verdict {
	case hw13Violation:
		t.Fatalf("S-02 violated without elevation: %s; the qualification fails, whatever the other route", why)
	case hw13Fallen:
		t.Fatalf("HW13 falls without elevation: %s; stop and the owner's arbitration (T3)", why)
	}
}

// hw13Outcome is what one route showed.
type hw13Outcome struct {
	name string
	// available: both definitions were made and both roots observed.
	available bool
	// rootStrict: the three conditions at the volume root, on the volume
	// expected.
	rootStrict bool
	// dirObserved and dirStrict: the remap to a subdirectory was made and
	// observed, and whether it still met the three conditions.
	dirObserved, dirStrict bool
	reason                 string
}

// passes reports an admissible route: available, strictly equivalent to the
// volume root, refused once remapped to a subdirectory.
func (o hw13Outcome) passes() bool {
	return o.available && o.rootStrict && o.dirObserved && !o.dirStrict
}

type hw13Result int

const (
	hw13Proven hw13Result = iota
	hw13Fallen
	hw13Violation
)

// hw13Verdict decides HW13 without elevation. A subdirectory alias observed
// as strictly equivalent to a volume root, on any route, violates S-02 and
// fails the qualification, even if the other route passes. Otherwise one
// admissible route proves HW13; none makes it fall (T3). A route Windows
// refuses is only recorded.
func hw13Verdict(outcomes []hw13Outcome) (hw13Result, string) {
	for _, o := range outcomes {
		if o.dirObserved && o.dirStrict {
			return hw13Violation, "route " + o.name + " took an alias of a subdirectory for a volume root"
		}
	}
	for _, o := range outcomes {
		if o.passes() {
			return hw13Proven, "route " + o.name + " passes"
		}
	}
	return hw13Fallen, "no route passes"
}

// hw13Route tries one route: the letter to the volume root and the three
// conditions; then, whatever that gave, the letter remapped to a
// subdirectory, which must lose them, so that a violation of S-02 is never
// left untried. A definition Windows refuses, or a root that cannot be
// observed, makes the route unavailable; removing what was defined must
// succeed, or the test fails at once.
func hw13Route(t *testing.T, name string, letter *hwLetter, toRoot, toDir func() error, volume, guidRoot string) hw13Outcome {
	t.Helper()
	o := hw13Outcome{name: name}
	rootObserved := false
	var rootNote string
	if err := toRoot(); err != nil {
		rootNote = "unavailable to the volume root: " + err.Error()
	} else {
		atRoot, err := hwCheckRootErr(letter.name + `\`)
		letter.remove(t)
		if err != nil {
			rootNote = "volume root not observable: " + err.Error()
		} else {
			rootObserved = true
			o.rootStrict = atRoot.strict() && atRoot.volume == guidRoot
			rootNote = fmt.Sprintf("to %s\\: strict %t", volume, o.rootStrict)
		}
	}
	if err := toDir(); err != nil {
		o.reason = rootNote + "; unavailable to a subdirectory: " + err.Error()
		return o
	}
	atDir, err := hwCheckRootErr(letter.name + `\`)
	letter.remove(t)
	if err != nil {
		o.reason = rootNote + "; subdirectory not observable: " + err.Error()
		return o
	}
	o.dirObserved, o.dirStrict = true, atDir.strict()
	o.available = rootObserved
	o.reason = fmt.Sprintf("%s; remapped to a subdirectory: strict %t", rootNote, o.dirStrict)
	return o
}

// The verdict of HW13 without elevation, on every case: a route that takes
// a subdirectory for a volume root fails the qualification even when the
// other route passes.
func TestHW13VerdictTable(t *testing.T) {
	pass := hw13Outcome{name: "a", available: true, rootStrict: true, dirObserved: true}
	compromised := hw13Outcome{name: "b", available: true, rootStrict: true, dirObserved: true, dirStrict: true}
	unavailable := hw13Outcome{name: "c"}
	notRoot := hw13Outcome{name: "d", available: true, dirObserved: true}
	compromisedUnavailableRoot := hw13Outcome{name: "e", dirObserved: true, dirStrict: true}
	for _, tt := range []struct {
		name     string
		outcomes []hw13Outcome
		want     hw13Result
	}{
		{"one route passes", []hw13Outcome{pass, unavailable}, hw13Proven},
		{"both pass", []hw13Outcome{pass, pass}, hw13Proven},
		{"route compromised, other passes", []hw13Outcome{compromised, pass}, hw13Violation},
		{"route passes, other compromised", []hw13Outcome{pass, compromised}, hw13Violation},
		{"compromised with its root unobserved", []hw13Outcome{compromisedUnavailableRoot, pass}, hw13Violation},
		{"both unavailable", []hw13Outcome{unavailable, unavailable}, hw13Fallen},
		{"no strict root", []hw13Outcome{notRoot, unavailable}, hw13Fallen},
		{"no route", nil, hw13Fallen},
	} {
		if got, why := hw13Verdict(tt.outcomes); got != tt.want {
			t.Errorf("%s: %d (%s), want %d", tt.name, got, why, tt.want)
		}
	}
}
