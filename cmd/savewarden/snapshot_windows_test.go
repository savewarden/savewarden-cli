//go:build windows

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var publishedSnapshotLine = regexp.MustCompile(`savewarden: vault: published snapshot ([0-9]{8}T[0-9]{6}-[0-9a-f]{8})\n$`)

// snapshotted publishes current with sync --apply, then a snapshot of it,
// and returns the snapshot operation.
func snapshotted(t *testing.T, s synthetic) string {
	t.Helper()
	if _, stderr, code := s.run(t, "--apply"); code != 0 || publishedLine.FindStringSubmatch(stderr) == nil {
		t.Fatalf("sync --apply: exit %d, stderr %q", code, stderr)
	}
	stdout, stderr, code := s.command(t, "snapshot", "--apply")
	match := publishedSnapshotLine.FindStringSubmatch(stderr)
	if code != 0 || match == nil {
		t.Fatalf("snapshot --apply: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	return match[1]
}

// killedDuringCopy starts snapshot --apply on a large current and kills it
// once its incomplete copy holds more than ten files.
func killedDuringCopy(t *testing.T, s synthetic) {
	t.Helper()
	cmd := exec.Command(s.binary, "snapshot", "--manifest", s.manifest, "--vault", s.vault, "--apply")
	var childErr bytes.Buffer
	cmd.Stderr = &childErr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	snapshots := filepath.Join(s.vault, "games", "synthetic-game", "snapshots")
	deadline := time.After(60 * time.Second)
	for killed := false; !killed; {
		select {
		case err := <-done:
			t.Skipf("the copy ended before it could be interrupted: %v %s", err, childErr.String())
		case <-deadline:
			_ = cmd.Process.Kill()
			t.Fatal("the copy did not start within a minute")
		default:
		}
		if entries, _ := filepath.Glob(filepath.Join(snapshots, "*.incomplete", "saves", "bulk", "*.save")); len(entries) > 10 {
			killed = cmd.Process.Kill() == nil
		}
		time.Sleep(time.Millisecond)
	}
	<-done
}

// Plan B3 §9, binary: sync --apply, snapshot --apply, inspect of the
// snapshot equal to its receipt and to the receipt of current, a second
// --apply that writes nothing, and sync up to date beside the snapshot.
func TestSnapshotEndToEnd(t *testing.T) {
	s := newSynthetic(t, buildBinary(t), 0)
	if _, stderr, code := s.run(t, "--apply"); code != 0 {
		t.Fatalf("sync --apply: exit %d, stderr %q", code, stderr)
	}
	stdout, stderr, code := s.command(t, "snapshot")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "plan: new snapshot\n") {
		t.Fatalf("snapshot plan: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	stdout, stderr, code = s.command(t, "snapshot", "--apply")
	match := publishedSnapshotLine.FindStringSubmatch(stderr)
	if code != 0 || match == nil || !strings.Contains(stdout, "verification: 2 files, 6 bytes, SHA-256 verified\n") {
		t.Fatalf("snapshot --apply: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	op := match[1]
	game := filepath.Join(s.vault, "games", "synthetic-game")
	manifest := filepath.Join(s.base, "snapshot.yaml")
	writeManifest(t, manifest, filepath.Join(game, "snapshots", op))
	inspected, err := exec.Command(s.binary, "inspect", "--manifest", manifest).Output()
	if err != nil {
		t.Fatalf("inspect of the snapshot: %v\n%s", err, inspected)
	}
	sums := receiptEntries(t, filepath.Join(game, "snapshots", "receipts", op+".json"))
	currentReceipts, err := filepath.Glob(filepath.Join(game, "receipts", "*.json"))
	if err != nil || len(currentReceipts) != 1 {
		t.Fatalf("receipts of current: %v, %v", currentReceipts, err)
	}
	if strings.Join(sums, ",") != strings.Join(receiptEntries(t, currentReceipts[0]), ",") || len(sums) != 2 {
		t.Fatalf("the snapshot receipt differs from the receipt of current: %q", sums)
	}
	for _, sum := range sums {
		if !strings.Contains(string(inspected), sum) {
			t.Fatalf("inspect of the snapshot lacks %s:\n%s", sum, inspected)
		}
	}
	before := fingerprint(t, s.vault)
	stdout, stderr, code = s.command(t, "snapshot", "--apply")
	if code != 0 || stderr != "savewarden: vault: not modified\n" || !strings.Contains(stdout, "plan: up to date\n") {
		t.Fatalf("second snapshot --apply: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	if stdout, stderr, code := s.run(t); code != 0 || stderr != "" || !strings.Contains(stdout, "plan: up to date\n") {
		t.Fatalf("sync plan: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	if fingerprint(t, s.vault) != before {
		t.Fatal("the second --apply or the sync plan changed the vault")
	}
}

// readOnlyTwice runs the binary twice with write access denied on the
// whole synthetic tree, and requires identical outputs and an identical
// tree.
func readOnlyTwice(t *testing.T, s synthetic, check func(stdout, stderr string, code int), args ...string) {
	t.Helper()
	denied := denyWrites(t, s.base)
	if !denied && os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Fatal("write access must be denied during the run on CI")
	}
	t.Logf("write access denied during the run: %v", denied)
	before := fingerprint(t, s.base)
	var outputs []string
	for range 2 {
		stdout, stderr, code := s.command(t, "snapshot", args...)
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

// The plan of snapshot writes nothing, not even with write access denied:
// not the vault, not its lock, not the sources.
func TestSnapshotPlanUnderDeniedWrites(t *testing.T) {
	s := newSynthetic(t, buildBinary(t), 0)
	op := snapshotted(t, s)
	if err := os.WriteFile(filepath.Join(s.install, "saves", "1.save"), []byte("ONE"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := s.run(t, "--apply"); code != 0 {
		t.Fatalf("rotation: exit %d, stderr %q", code, stderr)
	}
	readOnlyTwice(t, s, func(stdout, stderr string, code int) {
		if code != 0 || stderr != "" || !strings.Contains(stdout, "plan: new snapshot\n") || !strings.Contains(stdout, op) {
			t.Fatalf("snapshot plan: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
		}
	})
}

// The recovery plan writes nothing either, beside an interrupted snapshot.
func TestSnapshotRecoverPlanUnderDeniedWrites(t *testing.T) {
	s := newSynthetic(t, buildBinary(t), 2000)
	if _, stderr, code := s.run(t, "--apply"); code != 0 {
		t.Fatalf("sync --apply: exit %d, stderr %q", code, stderr)
	}
	killedDuringCopy(t, s)
	readOnlyTwice(t, s, func(stdout, stderr string, code int) {
		if code != 0 || stderr != "savewarden: vault: not modified\n" || !strings.Contains(stdout, "action: record abandoned") ||
			!strings.Contains(stdout, "mode: recovery plan only, nothing written\n") {
			t.Fatalf("recovery plan: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
		}
	}, "--recover")
}

// A file of the incomplete copy kept open by another program refuses the
// switch on real NTFS, access denied, never retried: rolled back at once,
// code 1, the copy kept; once it is closed, a new snapshot is published.
func TestSnapshotRenameRefusedByAnOpenFile(t *testing.T) {
	s := newSynthetic(t, buildBinary(t), 2000)
	if _, stderr, code := s.run(t, "--apply"); code != 0 {
		t.Fatalf("sync --apply: exit %d, stderr %q", code, stderr)
	}
	snapshots := filepath.Join(s.vault, "games", "synthetic-game", "snapshots")
	stop, held := make(chan struct{}), make(chan *os.File, 1)
	go func() {
		for {
			select {
			case <-stop:
				held <- nil
				return
			default:
			}
			if names, _ := filepath.Glob(filepath.Join(snapshots, "*.incomplete", "saves", "1.save")); len(names) == 1 {
				// Opened once written and closed: os.Open shares reading and
				// writing, never deletion.
				if file, err := os.Open(names[0]); err == nil {
					held <- file
					return
				}
			}
			time.Sleep(time.Millisecond)
		}
	}()
	stdout, stderr, code := s.command(t, "snapshot", "--apply")
	close(stop)
	file := <-held
	if file == nil {
		t.Skip("the copy ended before a file of it could be held")
	}
	_ = file.Close()
	if code != 1 || !regexp.MustCompile(`savewarden: vault: rolled back snapshot [0-9T]+-[0-9a-f]{8}\n$`).MatchString(stderr) {
		t.Fatalf("switch with a file open in the copy: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	if kept, _ := filepath.Glob(filepath.Join(snapshots, "*.incomplete")); len(kept) != 1 {
		t.Fatalf("incomplete copies: %v", kept)
	}
	if _, stderr, code := s.command(t, "snapshot", "--apply"); code != 0 || publishedSnapshotLine.FindStringSubmatch(stderr) == nil {
		t.Fatalf("snapshot once the file is closed: exit %d, stderr %q", code, stderr)
	}
}

// A process killed during the copy leaves an operation that --recover
// shows and --recover --apply closes as abandoned, the copy kept; a new
// --apply then publishes.
func TestSnapshotKilledDuringCopyThenRecovered(t *testing.T) {
	s := newSynthetic(t, buildBinary(t), 2000)
	if _, stderr, code := s.run(t, "--apply"); code != 0 {
		t.Fatalf("sync --apply: exit %d, stderr %q", code, stderr)
	}
	killedDuringCopy(t, s)
	snapshots := filepath.Join(s.vault, "games", "synthetic-game", "snapshots")
	stdout, stderr, code := s.command(t, "snapshot")
	if code != 3 || !strings.Contains(stdout, "pending-operation") {
		t.Fatalf("plan beside the interrupted snapshot: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	stdout, stderr, code = s.command(t, "snapshot", "--recover")
	if code != 0 || !strings.Contains(stdout, "action: record abandoned") || stderr != "savewarden: vault: not modified\n" {
		t.Fatalf("recover plan: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	stdout, stderr, code = s.command(t, "snapshot", "--recover", "--apply")
	if code != 0 || !strings.HasSuffix(stderr, " kept incomplete\n") {
		t.Fatalf("recover: exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	if kept, _ := filepath.Glob(filepath.Join(snapshots, "*.incomplete")); len(kept) != 1 {
		t.Fatalf("incomplete copies: %v", kept)
	}
	if _, stderr, code := s.run(t); code != 0 || stderr != "" {
		t.Fatalf("sync plan: exit %d, stderr %q", code, stderr)
	}
	if _, stderr, code := s.command(t, "snapshot", "--apply"); code != 0 || publishedSnapshotLine.FindStringSubmatch(stderr) == nil {
		t.Fatalf("snapshot after recovery: exit %d, stderr %q", code, stderr)
	}
}
