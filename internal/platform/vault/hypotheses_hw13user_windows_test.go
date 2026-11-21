//go:build windows

package vault_test

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// HW13 without elevation (decision 23; protocol accepted by the owner on
// 22 November 2026 under conditions). The elevated CI runner creates an
// ephemeral standard local account, member of Users only, and runs a
// verified copy of this test binary under it, in its own logon session, by
// Start-Process -Credential. The child proves its token is not elevated,
// then tries the two routes of HW13 on a reserved free letter. The
// password lives in memory and in the credential objects only: it reaches
// PowerShell on its standard input, never in an argument, a variable of the
// environment, a file or a log. No scheduled task is used.

const (
	hw13ChildVariable   = "SAVEWARDEN_HW13_CHILD"
	hw13AccountVariable = "SAVEWARDEN_HW13_ACCOUNT"
	hw13ChildTimeout    = 5 * time.Minute
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

// hw13PowerShell runs script, giving it stdin, and returns its output with
// redact replaced by a placeholder.
func hw13PowerShell(script, stdin, redact string) (string, error) {
	cmd := exec.Command("pwsh", "-NoProfile", "-NonInteractive", "-Command", script) //nolint:gosec // G702: scripts written by this test; the password goes on stdin only.
	cmd.Stdin = strings.NewReader(stdin)
	output, err := cmd.CombinedOutput()
	text := string(output)
	if redact != "" {
		text = strings.ReplaceAll(text, redact, "<SID>")
	}
	return text, err
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

// TestHW13NonElevatedAccount is the parent: CI only, on a GitHub-hosted
// ephemeral Windows runner only. A start that fails, a timeout, a child that
// fails or is skipped, or an incomplete cleanup fails the qualification.
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
	letters := hwLetterState(t)
	name := "swhw13" + hex.EncodeToString(hw13Random(t, 4))
	password := base64.RawURLEncoding.EncodeToString(hw13Random(t, 24)) + "aA1!"
	dir := filepath.Join(other, "savewarden-hw13-"+hex.EncodeToString(hw13Random(t, 4)))
	if err := os.Mkdir(dir, 0o700); err != nil { //nolint:gosec // G703: a name this test drew below RUNNER_TEMP, on the ephemeral runner.
		t.Fatal(err)
	}

	// The account: Users only. Its SID is kept for the cleanup, never logged.
	output, err := hw13PowerShell(`$ErrorActionPreference = 'Stop'
$secure = ConvertTo-SecureString ([Console]::In.ReadLine()) -AsPlainText -Force
$user = New-LocalUser -Name `+hw13Quote(name)+` -Password $secure -AccountNeverExpires -Description 'Savewarden HW13 test, ephemeral'
Add-LocalGroupMember -SID 'S-1-5-32-545' -Member $user
"SID=" + $user.SID.Value`, password+"\n", "")
	sid := ""
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "SID=") {
			sid = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "SID="))
		}
	}
	t.Cleanup(func() { hw13Cleanup(t, name, sid, dir, letters) })
	if err != nil || sid == "" {
		if sid != "" {
			output = strings.ReplaceAll(output, sid, "<SID>")
		}
		t.Fatalf("create the ephemeral account: %v\n%s", err, output)
	}
	t.Logf("ephemeral standard account %s created, member of Users", name)

	// The dedicated space: inheritance removed; Administrators and SYSTEM
	// full control, the account read and execute only.
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	copied := filepath.Join(dir, "vault.test.exe")
	hw13CopyVerified(t, executable, copied)
	if out, err := exec.Command("icacls", dir, "/inheritance:r", "/grant:r", //nolint:gosec // G702: icacls on the dedicated space, for the account this test created.
		"*S-1-5-32-544:(OI)(CI)F", "*S-1-5-18:(OI)(CI)F", "*"+sid+":(OI)(CI)RX").CombinedOutput(); err != nil {
		t.Fatalf("icacls: %v %s", err, strings.ReplaceAll(string(out), sid, "<SID>"))
	}

	stdout, stderr := filepath.Join(dir, "child.out"), filepath.Join(dir, "child.err")
	script := `$ErrorActionPreference = 'Stop'
$secure = ConvertTo-SecureString ([Console]::In.ReadLine()) -AsPlainText -Force
$credential = [pscredential]::new(` + hw13Quote(name) + `, $secure)
$process = Start-Process -FilePath ` + hw13Quote(copied) + ` -ArgumentList '-test.run=^TestHW13NonElevatedChild$', '-test.v', '-test.count=1' ` +
		`-Credential $credential -LoadUserProfile -WorkingDirectory ` + hw13Quote(dir) +
		` -RedirectStandardOutput ` + hw13Quote(stdout) + ` -RedirectStandardError ` + hw13Quote(stderr) +
		` -Environment @{ ` + hw13ChildVariable + ` = '1'; ` + hw13AccountVariable + ` = ` + hw13Quote(name) + ` } -PassThru
$null = $process.Handle
if (-not $process.WaitForExit(` + fmt.Sprint(hw13ChildTimeout.Milliseconds()) + `)) {
  Stop-Process -Id $process.Id -Force
  "TIMEOUT"
  exit 3
}
"EXIT=" + $process.ExitCode`
	output, err = hw13PowerShell(script, password+"\n", sid)
	childOut, _ := os.ReadFile(stdout) //nolint:gosec // G703: the child output file this test chose in its dedicated space.
	childErr, _ := os.ReadFile(stderr) //nolint:gosec // G703: the child error file this test chose in its dedicated space.
	t.Logf("child standard output:\n%s", bytes.ReplaceAll(childOut, []byte(sid), []byte("<SID>")))
	if len(childErr) > 0 {
		t.Logf("child standard error:\n%s", bytes.ReplaceAll(childErr, []byte(sid), []byte("<SID>")))
	}
	if err != nil || !strings.Contains(output, "EXIT=0") {
		t.Fatalf("the child under the standard account did not succeed: %v\n%s", err, output)
	}
	text := string(childOut)
	if !strings.Contains(text, "--- PASS: TestHW13NonElevatedChild") {
		t.Fatal("the child did not run TestHW13NonElevatedChild to a pass: a skip is not a proof")
	}
	t.Log("HW13 without elevation: proven by the child under the ephemeral standard account")
}

// hw13Cleanup stops the account's processes, deletes its profile and the
// account by their exact SID, removes the dedicated space, and fails the
// test if anything is left.
func hw13Cleanup(t *testing.T, name, sid, dir string, letters string) {
	t.Helper()
	if sid == "" {
		// The account may exist without its SID read back: remove it by its
		// exact name, which the test drew at random.
		script := `$user = Get-LocalUser -Name ` + hw13Quote(name) + ` -ErrorAction SilentlyContinue
if ($user) { Remove-LocalUser -SID $user.SID.Value -ErrorAction Stop }
if (Get-LocalUser -Name ` + hw13Quote(name) + ` -ErrorAction SilentlyContinue) { throw 'the account is still present' }
"CLEAN"`
		if output, err := hw13PowerShell(script, "", ""); err != nil || !strings.Contains(output, "CLEAN") {
			t.Errorf("delete the ephemeral account %s: %v\n%s", name, err, output)
		}
	}
	if sid != "" {
		script := `$ErrorActionPreference = 'Stop'
$sid = ` + hw13Quote(sid) + `
Get-CimInstance Win32_Process | Where-Object { $owner = Invoke-CimMethod -InputObject $_ -MethodName GetOwnerSid; $owner.Sid -eq $sid } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force }
$userProfile = Get-CimInstance Win32_UserProfile | Where-Object SID -eq $sid
if ($userProfile) { Remove-CimInstance -InputObject $userProfile }
Remove-LocalUser -SID $sid
if (Get-LocalUser -SID $sid -ErrorAction SilentlyContinue) { throw 'the account is still present' }
if (Get-CimInstance Win32_UserProfile | Where-Object SID -eq $sid) { throw 'the profile is still present' }
"CLEAN"`
		if output, err := hw13PowerShell(script, "", sid); err != nil || !strings.Contains(output, "CLEAN") {
			t.Errorf("delete the ephemeral account %s and its profile: %v\n%s", name, err, output)
		}
	}
	if err := os.RemoveAll(dir); err != nil { //nolint:gosec // G703: the dedicated space this test created below RUNNER_TEMP.
		t.Errorf("remove the dedicated space: %v", err)
	}
	if got := hwLetterState(t); got != letters {
		t.Errorf("drive letters of the runner changed:\nbefore %s\nafter  %s", letters, got)
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
// refuses as a proof.
func TestHW13NonElevatedChild(t *testing.T) {
	if os.Getenv(hw13ChildVariable) != "1" {
		t.Skip("runs only under the ephemeral account of TestHW13NonElevatedAccount")
	}
	// 1. The effective token: the account created, not elevated, medium
	// integrity, Administrators not active, no administrative privilege.
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	account, _, _, err := user.User.Sid.LookupAccount("")
	if err != nil {
		t.Fatal(err)
	}
	want := os.Getenv(hw13AccountVariable)
	if want == "" || !strings.EqualFold(account, want) {
		t.Fatalf("token of %q, want the ephemeral account %q", account, want)
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
	t.Logf("HW13 child token: account of the test, not elevated, medium integrity, Administrators not active; privileges %s",
		strings.Join(privileges, ", "))

	// 2. The two routes, each on its own, on a reserved free letter of this
	// logon session, restored exactly.
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
		toRoot func()
		toDir  func()
	}{
		{"subst", func() { letter.subst(t, volume+`\`) }, func() { letter.subst(t, sub) }},
		{"DefineDosDevice", func() { letter.define(t, devices[0]) }, func() { letter.define(t, `\??\`+sub) }},
	}
	passed := 0
	for _, r := range routes {
		r.toRoot()
		atRoot := hwCheckRoot(t, letter.name+`\`)
		letter.remove(t)
		r.toDir()
		atDir := hwCheckRoot(t, letter.name+`\`)
		letter.remove(t)
		ok := atRoot.strict() && atRoot.volume == guidRoot && !atDir.strict()
		t.Logf("HW13 child %s to %s\\: strict %t; remapped to a subdirectory: strict %t", r.name, volume, atRoot.strict(), atDir.strict())
		t.Logf("HW13 child route %s: %t", r.name, ok)
		if ok {
			passed++
		}
	}
	if passed == 0 {
		t.Fatal("HW13 falls without elevation: no route passes; stop and the owner's arbitration (T3)")
	}
}
