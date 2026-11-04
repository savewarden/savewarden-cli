//go:build windows

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

var publishedRestoreLine = regexp.MustCompile(`savewarden: destination: published restore ([0-9]{8}T[0-9]{6}-[0-9a-f]{8})\n$`)

// area names a restore destination below the synthetic base, its parent
// created.
func (s synthetic) area(t *testing.T, name string) string {
	t.Helper()
	parent := filepath.Join(s.base, "Restaurations")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(parent, name)
}

// restore runs restore of snapshot snap to dest with the synthetic manifest
// and vault.
func (s synthetic) restore(t *testing.T, snap, dest string, args ...string) (string, string, int) {
	t.Helper()
	return s.exec(t, append([]string{"restore", "--manifest", s.manifest, "--vault", s.vault, "--snapshot", snap, "--to", dest}, args...)...)
}

// recoverArea runs restore --recover on dest, which never names the vault.
func (s synthetic) recoverArea(t *testing.T, dest string, args ...string) (string, string, int) {
	t.Helper()
	return s.exec(t, append([]string{"restore", "--manifest", s.manifest, "--to", dest, "--recover"}, args...)...)
}

// restored publishes a restore of snap to dest and returns its operation.
func (s synthetic) restored(t *testing.T, snap, dest string) string {
	t.Helper()
	stdout, stderr, code := s.restore(t, snap, dest, "--apply")
	match := publishedRestoreLine.FindStringSubmatch(stderr)
	if code != 0 || match == nil {
		t.Fatalf("restore --apply: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	return match[1]
}

// whileCopying starts the binary with args and runs during once the
// incomplete copy matched by pattern holds more than ten files. It returns
// the exit code and stderr of the binary, and whether during ran.
func whileCopying(t *testing.T, s synthetic, pattern string, during func(cmd *exec.Cmd), args ...string) (int, string, bool) {
	t.Helper()
	cmd := exec.Command(s.binary, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	deadline := time.After(60 * time.Second)
	ran := false
	for !ran {
		select {
		case err := <-done:
			return exitCode(t, err), stderr.String(), false
		case <-deadline:
			_ = cmd.Process.Kill()
			t.Fatal("the copy did not start within a minute")
		default:
		}
		if entries, _ := filepath.Glob(pattern); len(entries) > 10 {
			during(cmd)
			ran = true
		}
		time.Sleep(time.Millisecond)
	}
	return exitCode(t, <-done), stderr.String(), true
}

func exitCode(t *testing.T, err error) int {
	t.Helper()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exit):
		return exit.ExitCode()
	}
	t.Fatal(err)
	return -1
}

// notExercised ends a test whose race could not be staged, the copy ending
// too early: a skip on a developer's machine, a failure on CI, where the
// proof must have run (security review, P2).
func notExercised(t *testing.T, format string, args ...any) {
	t.Helper()
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Fatalf("not exercised on CI: "+format, args...)
	}
	t.Skipf(format, args...)
}

// incompleteFiles matches the bulk files of an incomplete restore in dest.
func incompleteFiles(dest string) string {
	return filepath.Join(dest, "*.incomplete", "saves", "bulk", "*.save")
}

// Plan B4 §9, binary: sync --apply, snapshot --apply, restore plan and
// --apply, inspect of the restore equal to the snapshot receipt; the vault
// identical, sync and snapshot up to date, nothing to recover, and the
// restore reported by the next plan.
func TestRestoreEndToEnd(t *testing.T) {
	s := newSynthetic(t, buildBinary(t), 0)
	snap := snapshotted(t, s)
	vault := fingerprint(t, s.vault)
	dest := s.area(t, "synthétique")
	stdout, stderr, code := s.restore(t, snap, dest)
	if code != 0 || stderr != "" || !strings.Contains(stdout, "snapshot: "+snap+", from ") || !strings.Contains(stdout, ", verified\n") ||
		!strings.Contains(stdout, "(new restore area)\n") || !strings.Contains(stdout, "plan: new restore\n") {
		t.Fatalf("restore plan: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	if _, err := os.Lstat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the plan wrote the destination: %v", err)
	}
	stdout, stderr, code = s.restore(t, snap, dest, "--apply")
	match := publishedRestoreLine.FindStringSubmatch(stderr)
	if code != 0 || match == nil || !strings.Contains(stdout, "verification: 2 files, 6 bytes, SHA-256 verified\n") {
		t.Fatalf("restore --apply: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	op := match[1]
	manifest := filepath.Join(s.base, "restore.yaml")
	writeManifest(t, manifest, filepath.Join(dest, op))
	inspected, err := exec.Command(s.binary, "inspect", "--manifest", manifest).Output()
	if err != nil {
		t.Fatalf("inspect of the restore: %v\n%s", err, inspected)
	}
	sums := receiptEntries(t, filepath.Join(s.vault, "games", "synthetic-game", "snapshots", "receipts", snap+".json"))
	if restoreSums := receiptEntries(t, filepath.Join(dest, "receipts", op+".json")); strings.Join(restoreSums, ",") != strings.Join(sums, ",") || len(sums) != 2 {
		t.Fatalf("the restore receipt differs from the snapshot receipt: %q, %q", restoreSums, sums)
	}
	for _, sum := range sums {
		if !strings.Contains(string(inspected), sum) {
			t.Fatalf("inspect of the restore lacks %s:\n%s", sum, inspected)
		}
	}
	if fingerprint(t, s.vault) != vault {
		t.Fatal("the restore changed the vault")
	}
	if stdout, stderr, code := s.run(t); code != 0 || stderr != "" || !strings.Contains(stdout, "plan: up to date\n") {
		t.Fatalf("sync plan: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	if stdout, stderr, code := s.command(t, "snapshot"); code != 0 || stderr != "" || !strings.Contains(stdout, "plan: up to date\n") {
		t.Fatalf("snapshot plan: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	if stdout, stderr, code := s.recoverArea(t, dest, "--apply"); code != 0 || stderr != "savewarden: destination: not modified\n" ||
		!strings.Contains(stdout, "action: nothing to recover\n") {
		t.Fatalf("recover: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	stdout, stderr, code = s.restore(t, snap, dest)
	if code != 0 || stderr != "" || !strings.Contains(stdout, "  "+op+"  of "+snap+"  present\n") || !strings.Contains(stdout, "snapshot-already-restored") {
		t.Fatalf("plan after: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
}

// R4: a restore reads a vault whose write access is denied: plan, --apply
// and recovery succeed, and the vault stays identical.
func TestRestoreFromAWriteDeniedVault(t *testing.T) {
	s := newSynthetic(t, buildBinary(t), 0)
	snap := snapshotted(t, s)
	denied := denyWrites(t, s.vault)
	if !denied && os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Fatal("write access must be denied on the vault on CI")
	}
	t.Logf("write access denied on the vault: %v", denied)
	vault := fingerprint(t, s.vault)
	dest := s.area(t, "synthétique")
	if stdout, stderr, code := s.restore(t, snap, dest); code != 0 || stderr != "" {
		t.Fatalf("plan: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	s.restored(t, snap, dest)
	if _, stderr, code := s.recoverArea(t, dest, "--apply"); code != 0 || stderr != "savewarden: destination: not modified\n" {
		t.Fatalf("recover: exit %d, stderr %q", code, stderr)
	}
	if fingerprint(t, s.vault) != vault {
		t.Fatal("the restore changed the vault")
	}
}

// deniedTwice runs the binary twice with write access denied on the whole
// synthetic tree, and requires identical outputs and an identical tree.
func deniedTwice(t *testing.T, s synthetic, check func(stdout, stderr string, code int), args ...string) {
	t.Helper()
	denied := denyWrites(t, s.base)
	if !denied && os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Fatal("write access must be denied during the run on CI")
	}
	t.Logf("write access denied during the run: %v", denied)
	before := fingerprint(t, s.base)
	var outputs []string
	for range 2 {
		stdout, stderr, code := s.exec(t, args...)
		check(stdout, stderr, code)
		outputs = append(outputs, stdout+stderr)
	}
	if outputs[0] != outputs[1] {
		t.Fatalf("two runs differ:\n%s\n---\n%s", outputs[0], outputs[1])
	}
	if fingerprint(t, s.base) != before {
		t.Fatal("the run changed the tree")
	}
}

// The plan of restore writes nothing, not even with write access denied.
func TestRestorePlanUnderDeniedWrites(t *testing.T) {
	s := newSynthetic(t, buildBinary(t), 0)
	snap := snapshotted(t, s)
	dest := s.area(t, "synthétique")
	deniedTwice(t, s, func(stdout, stderr string, code int) {
		if code != 0 || stderr != "" || !strings.Contains(stdout, "plan: new restore\n") {
			t.Fatalf("restore plan: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
		}
	}, "restore", "--manifest", s.manifest, "--vault", s.vault, "--snapshot", snap, "--to", dest)
}

// killedDuringRestore starts restore --apply of snap to dest and kills it
// once its incomplete copy holds more than ten files.
func killedDuringRestore(t *testing.T, s synthetic, snap, dest string) {
	t.Helper()
	_, stderr, ran := whileCopying(t, s, incompleteFiles(dest), func(cmd *exec.Cmd) {
		if err := cmd.Process.Kill(); err != nil {
			t.Fatal(err)
		}
	}, "restore", "--manifest", s.manifest, "--vault", s.vault, "--snapshot", snap, "--to", dest, "--apply")
	if !ran {
		notExercised(t, "the copy ended before it could be interrupted: %s", stderr)
	}
}

// The recovery plan writes nothing either, beside an interrupted restore.
func TestRestoreRecoverPlanUnderDeniedWrites(t *testing.T) {
	s := newSynthetic(t, buildBinary(t), 2000)
	snap := snapshotted(t, s)
	dest := s.area(t, "synthétique")
	killedDuringRestore(t, s, snap, dest)
	deniedTwice(t, s, func(stdout, stderr string, code int) {
		if code != 0 || stderr != "savewarden: destination: not modified\n" || !strings.Contains(stdout, "action: record abandoned") ||
			!strings.Contains(stdout, "mode: recovery plan only, nothing written\n") || strings.Contains(stdout, "vault") {
			t.Fatalf("recovery plan: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
		}
	}, "restore", "--manifest", s.manifest, "--to", dest, "--recover")
}

// A file of the incomplete copy kept open by another program refuses the
// switch on real NTFS: rolled back, code 1, the copy kept; once it is
// closed, a new restore is published.
func TestRestoreRenameRefusedByAnOpenFile(t *testing.T) {
	s := newSynthetic(t, buildBinary(t), 2000)
	snap := snapshotted(t, s)
	dest := s.area(t, "synthétique")
	stop, held := make(chan struct{}), make(chan *os.File, 1)
	go func() {
		for {
			select {
			case <-stop:
				held <- nil
				return
			default:
			}
			if names, _ := filepath.Glob(filepath.Join(dest, "*.incomplete", "saves", "1.save")); len(names) == 1 {
				// os.Open shares reading and writing, never deletion.
				if file, err := os.Open(names[0]); err == nil {
					held <- file
					return
				}
			}
			time.Sleep(time.Millisecond)
		}
	}()
	stdout, stderr, code := s.restore(t, snap, dest, "--apply")
	close(stop)
	file := <-held
	if file == nil {
		notExercised(t, "the copy ended before a file of it could be held")
	}
	_ = file.Close()
	if code != 1 || !regexp.MustCompile(`savewarden: destination: rolled back restore [0-9T]+-[0-9a-f]{8}\n$`).MatchString(stderr) {
		t.Fatalf("switch with a file open in the copy: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	if kept, _ := filepath.Glob(filepath.Join(dest, "*.incomplete")); len(kept) != 1 {
		t.Fatalf("incomplete copies: %v", kept)
	}
	s.restored(t, snap, dest)
}

// A process killed during the copy leaves an operation that the plan
// reports, code 3, that --recover shows and --recover --apply closes as
// abandoned, the copy kept; a new --apply then publishes.
func TestRestoreKilledDuringCopyThenRecovered(t *testing.T) {
	s := newSynthetic(t, buildBinary(t), 2000)
	snap := snapshotted(t, s)
	dest := s.area(t, "synthétique")
	killedDuringRestore(t, s, snap, dest)
	if stdout, stderr, code := s.restore(t, snap, dest); code != 3 || !strings.Contains(stdout, "pending-operation") {
		t.Fatalf("plan beside the interrupted restore: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	if stdout, stderr, code := s.recoverArea(t, dest); code != 0 || !strings.Contains(stdout, "action: record abandoned") || stderr != "savewarden: destination: not modified\n" {
		t.Fatalf("recover plan: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	if stdout, stderr, code := s.recoverArea(t, dest, "--apply"); code != 0 || !strings.HasSuffix(stderr, " kept incomplete\n") {
		t.Fatalf("recover: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	if kept, _ := filepath.Glob(filepath.Join(dest, "*.incomplete")); len(kept) != 1 {
		t.Fatalf("incomplete copies: %v", kept)
	}
	s.restored(t, snap, dest)
}

// substAlias gives a free drive letter to dir with subst, removed at
// cleanup or by the returned function, and checks that both names reach
// the same directory. On CI, an alias that cannot be given is a failure,
// never a skip (plan B4, C4).
func substAlias(t *testing.T, dir string) (string, func()) {
	t.Helper()
	for letter := 'Z'; letter >= 'M'; letter-- {
		drive := string(letter) + ":"
		if _, err := os.Stat(drive + `\`); err == nil {
			continue
		}
		output, err := exec.Command("subst", drive, dir).CombinedOutput()
		if err != nil {
			continue
		}
		removed := false
		remove := func() {
			if removed {
				return
			}
			removed = true
			if output, err := exec.Command("subst", drive, "/D").CombinedOutput(); err != nil {
				t.Errorf("remove the alias %s: %v %s", drive, err, output)
			}
		}
		t.Cleanup(remove)
		a, errA := os.Stat(drive + `\`)
		b, errB := os.Stat(dir)
		if errA != nil || errB != nil || !os.SameFile(a, b) {
			t.Fatalf("precondition: %s and %s are not the same directory: %v %v %s", drive, dir, errA, errB, output)
		}
		return drive, remove
	}
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Fatal("no drive letter could be given by subst on CI")
	}
	t.Skip("no drive letter could be given by subst")
	return "", nil
}

// refusedUnchanged runs args, requires code 1 mentioning want, and the
// synthetic tree unchanged.
func refusedUnchanged(t *testing.T, s synthetic, want string, args ...string) {
	t.Helper()
	before := fingerprint(t, s.base)
	stdout, stderr, code := s.exec(t, args...)
	if code != 1 || !strings.Contains(stderr, want) {
		t.Fatalf("%q: exit %d, stderr %q, stdout:\n%s", args, code, stderr, stdout)
	}
	if fingerprint(t, s.base) != before {
		t.Fatalf("%q changed the tree", args)
	}
}

// C4: two drive letters naming the same directory, a subst alias, never let
// a restore land in a source or the vault, at creation or recovery; an
// alias removed while the manifest still names it proves no separation.
func TestRestoreRefusesASubstAlias(t *testing.T) {
	binary := buildBinary(t)
	s := newSynthetic(t, binary, 0)
	snap := snapshotted(t, s)
	saves := filepath.Join(s.install, "saves")

	// The area under an alias of the source, at creation and recovery.
	drive, remove := substAlias(t, saves)
	refusedUnchanged(t, s, "overlaps the", "restore", "--manifest", s.manifest, "--vault", s.vault, "--snapshot", snap, "--to", drive+`\Restauration`, "--apply")
	zone := filepath.Join(saves, "zone")
	if err := os.MkdirAll(zone, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(zone, "savewarden-restore.json.tmp-1"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	refusedUnchanged(t, s, "overlaps the", "restore", "--manifest", s.manifest, "--to", drive+`\zone`, "--recover", "--apply")
	remove()

	// The area under an alias of the vault, and the vault named by its alias.
	drive, remove = substAlias(t, s.vault)
	refusedUnchanged(t, s, "lies in a Savewarden vault or restore area", "restore", "--manifest", s.manifest, "--vault", s.vault, "--snapshot", snap,
		"--to", drive+`\Restauration`, "--apply")
	refusedUnchanged(t, s, "lies in a Savewarden vault or restore area", "restore", "--manifest", s.manifest, "--vault", drive+`\`, "--snapshot", snap,
		"--to", filepath.Join(s.vault, "Restauration"), "--apply")
	remove()

	// The source named by an alias, the area by the real path: only the
	// identities can tell, the names differing by their drive letter.
	s2 := newSynthetic(t, binary, 0)
	drive, remove = substAlias(t, s2.base)
	aliased := filepath.Join(s2.base, "alias.yaml")
	writeManifest(t, aliased, drive+`\`+filepath.Base(s2.install))
	if _, stderr, code := s2.exec(t, "sync", "--manifest", aliased, "--vault", s2.vault, "--apply"); code != 0 {
		t.Fatalf("sync through the alias: exit %d, stderr %q", code, stderr)
	}
	_, stderr, code := s2.exec(t, "snapshot", "--manifest", aliased, "--vault", s2.vault, "--apply")
	match := publishedSnapshotLine.FindStringSubmatch(stderr)
	if code != 0 || match == nil {
		t.Fatalf("snapshot through the alias: exit %d, stderr %q", code, stderr)
	}
	refusedUnchanged(t, s2, "overlaps the", "restore", "--manifest", aliased, "--vault", s2.vault, "--snapshot", match[1],
		"--to", filepath.Join(s2.install, "saves", "Restauration"), "--apply")
	remove()
	// The alias removed while the manifest still names it.
	refusedUnchanged(t, s2, "cannot be proven", "restore", "--manifest", aliased, "--vault", s2.vault, "--snapshot", match[1],
		"--to", s2.area(t, "synthétique"), "--apply")
}

// V1: two runs that validate the marker of the same destination at once,
// two recoveries of an interrupted initialization, or recoveries started
// one after another while a creation runs, validate one marker only; each
// ends in code 0 or 1, never 3, and the area obtained is accepted
// afterwards. The count of runs where both wrote a temporary, a real race,
// is logged: the scheduler decides it.
func TestConcurrentMarkerRecovery(t *testing.T) {
	s := newSynthetic(t, buildBinary(t), 0)
	snap := snapshotted(t, s)
	start := func(args ...string) (*exec.Cmd, *bytes.Buffer) {
		cmd := exec.Command(s.binary, args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		return cmd, &stderr
	}
	recoverArgs := func(dest string) []string {
		return []string{"restore", "--manifest", s.manifest, "--to", dest, "--recover", "--apply"}
	}
	raced := map[string]int{}
	defer func() { t.Logf("runs where both processes wrote a temporary, out of 20: %v", raced) }()
	for i := range 20 {
		for _, mode := range []string{"two recoveries", "recovery and creation"} {
			dest := s.area(t, fmt.Sprintf("course-%02d-%s", i, strings.ReplaceAll(mode, " ", "-")))
			if err := os.Mkdir(dest, 0o700); err != nil {
				t.Fatal(err)
			}
			var codes []int
			var errA, errB *bytes.Buffer
			if mode == "two recoveries" {
				if err := os.WriteFile(filepath.Join(dest, "savewarden-restore.json.tmp-1"), []byte("{"), 0o600); err != nil {
					t.Fatal(err)
				}
				var a, b *exec.Cmd
				a, errA = start(recoverArgs(dest)...)
				b, errB = start(recoverArgs(dest)...)
				codes = []int{exitCode(t, a.Wait()), exitCode(t, b.Wait())}
			} else {
				// Recoveries one after another, as long as the creation runs:
				// each finds the destination empty, holding the creation's
				// temporary, or marked.
				var creation *exec.Cmd
				creation, errA = start("restore", "--manifest", s.manifest, "--vault", s.vault, "--snapshot", snap, "--to", dest, "--apply")
				done := make(chan error, 1)
				go func() { done <- creation.Wait() }()
				errB = &bytes.Buffer{}
				for running := true; running; {
					select {
					case err := <-done:
						codes, running = append(codes, exitCode(t, err)), false
					default:
						recovery := exec.Command(s.binary, recoverArgs(dest)...)
						recovery.Stderr = errB
						codes = append(codes, exitCode(t, recovery.Run()))
					}
				}
			}
			if slices.ContainsFunc(codes, func(code int) bool { return code != 0 && code != 1 }) {
				t.Fatalf("%s %d: codes %v\n%s\n%s", mode, i, codes, errA, errB)
			}
			entries, err := os.ReadDir(dest)
			if err != nil {
				t.Fatal(err)
			}
			markers, temporaries := 0, 0
			for _, entry := range entries {
				switch {
				case entry.Name() == "savewarden-restore.json":
					markers++
				case strings.HasPrefix(entry.Name(), "savewarden-restore.json.tmp-"):
					temporaries++
				}
			}
			if markers != 1 {
				t.Fatalf("%s %d: %d markers, codes %v\n%s\n%s", mode, i, markers, codes, errA, errB)
			}
			// A temporary left beside the marker, beyond the one the test laid
			// for two recoveries, is the attempt of the run that lost.
			if (mode == "two recoveries" && temporaries > 1) || (mode != "two recoveries" && temporaries > 0) {
				raced[mode]++
			}
			if i == 0 {
				if stdout, stderr, code := s.restore(t, snap, dest); code != 0 {
					t.Fatalf("%s: plan of the area: exit %d, stderr %q, stdout:\n%s", mode, code, stderr, stdout)
				}
				s.restored(t, snap, dest)
			}
		}
	}
}

// After published, the copy replaced by a junction to a witness directory
// is restore-replaced, never followed: the witness stays identical through
// a plan, a new restore and a recovery.
func TestHandedOverJunctionIsNeverFollowed(t *testing.T) {
	s := newSynthetic(t, buildBinary(t), 0)
	snap := snapshotted(t, s)
	dest := s.area(t, "synthétique")
	op := s.restored(t, snap, dest)
	witness := filepath.Join(s.base, "Témoin")
	if err := os.MkdirAll(witness, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(witness, "témoin.txt"), []byte("synthetic witness\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The user moves the copy out and leaves a junction in its place.
	if err := os.Rename(filepath.Join(dest, op), filepath.Join(s.base, "Déplacée")); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(dest, op), witness).CombinedOutput(); err != nil {
		t.Fatalf("junction: %v %s", err, output)
	}
	before := fingerprint(t, witness)
	stdout, stderr, code := s.restore(t, snap, dest)
	if code != 0 || stderr != "" || !strings.Contains(stdout, "  "+op+"  of "+snap+"  replaced\n") || !strings.Contains(stdout, "restore-replaced") {
		t.Fatalf("plan: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	s.restored(t, snap, dest)
	if _, stderr, code := s.recoverArea(t, dest, "--apply"); code != 0 || stderr != "savewarden: destination: not modified\n" {
		t.Fatalf("recover: exit %d, stderr %q", code, stderr)
	}
	if fingerprint(t, witness) != before {
		t.Fatal("the witness behind the junction changed")
	}
}

// X1: a restore runs while a real rotation by sync --apply and a real
// snapshot --apply publish, none blocking another; then a restore runs
// while a rotation copies.
func TestRestoreBesideRealSyncAndSnapshot(t *testing.T) {
	s := newSynthetic(t, buildBinary(t), 2000)
	snap := snapshotted(t, s)
	change := func(content string) {
		if err := os.WriteFile(filepath.Join(s.install, "saves", "1.save"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	dest := s.area(t, "pendant")
	code, stderr, ran := whileCopying(t, s, incompleteFiles(dest), func(*exec.Cmd) {
		change("ONE")
		if _, stderr, code := s.run(t, "--apply"); code != 0 || publishedLine.FindStringSubmatch(stderr) == nil {
			t.Errorf("rotation beside the restore: exit %d, stderr %q", code, stderr)
		}
		if _, stderr, code := s.command(t, "snapshot", "--apply"); code != 0 || publishedSnapshotLine.FindStringSubmatch(stderr) == nil {
			t.Errorf("snapshot beside the restore: exit %d, stderr %q", code, stderr)
		}
	}, "restore", "--manifest", s.manifest, "--vault", s.vault, "--snapshot", snap, "--to", dest, "--apply")
	if !ran {
		notExercised(t, "the restore ended before the others could run: %s", stderr)
	}
	if code != 0 || publishedRestoreLine.FindStringSubmatch(stderr) == nil {
		t.Fatalf("restore beside sync and snapshot: exit %d, stderr %q", code, stderr)
	}
	// The reverse: a restore while a rotation copies its staging.
	change("UNO")
	staging := filepath.Join(s.vault, "games", "synthetic-game", "staging", "*", "saves", "bulk", "*.save")
	other := s.area(t, "pendant la rotation")
	code, stderr, ran = whileCopying(t, s, staging, func(*exec.Cmd) { s.restored(t, snap, other) },
		"sync", "--manifest", s.manifest, "--vault", s.vault, "--apply")
	if !ran {
		notExercised(t, "the rotation ended before the restore could run: %s", stderr)
	}
	if code != 0 || publishedLine.FindStringSubmatch(stderr) == nil {
		t.Fatalf("rotation beside the restore: exit %d, stderr %q", code, stderr)
	}
}
