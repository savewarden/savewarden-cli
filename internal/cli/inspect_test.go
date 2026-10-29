package cli_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/savewarden/savewarden-cli/internal/cli"
	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/entities/game"
	"github.com/savewarden/savewarden-cli/internal/features/inspect"
)

type fakeInspector struct {
	report  inspect.Report
	err     error
	request inspect.Request
}

func (f *fakeInspector) Inspect(_ context.Context, request inspect.Request) (inspect.Report, error) {
	f.request = request
	return f.report, f.err
}

var modTime = time.Date(2026, 10, 9, 16, 32, 8, 123456700, time.UTC)

func baseReport() inspect.Report {
	return inspect.Report{
		ManifestPath: `C:\manifests\synthetic.yaml`,
		Game:         game.Game{ID: "synthetic-game", Name: "Synthetic Game"},
		Installation: `D:\Games\Synthetic Game`,
		Sources: []inspect.SourceReport{{
			Source: game.Source{ID: "saves", Kind: game.KindSave, Required: true},
			Path:   `D:\Games\Synthetic Game\saves`,
			Status: inspect.StatusInventoried,
			Entries: []backup.Entry{
				{Path: "1.save", Size: 3, ModTime: modTime, SHA256: sha256.Sum256([]byte("one"))},
				{Path: "Profil été/2.save", Size: 0, ModTime: modTime, SHA256: sha256.Sum256(nil)},
			},
			Bytes: 3,
		}},
	}
}

func execute(t *testing.T, inspector cli.Inspector, args ...string) (string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	root := cli.NewRootCommand("dev", &stdout, &stderr, inspector, nil)
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	return stdout.String(), err
}

func TestRenderCompleteInventory(t *testing.T) {
	inspector := &fakeInspector{report: baseReport()}
	out, err := execute(t, inspector, "inspect", "--manifest", `C:\manifests\synthetic.yaml`)
	if err != nil {
		t.Fatal(err)
	}
	if inspector.request.ManifestPath != `C:\manifests\synthetic.yaml` {
		t.Fatalf("request = %+v", inspector.request)
	}
	want := `manifest: C:\manifests\synthetic.yaml
game: synthetic-game (Synthetic Game)
installation: D:\Games\Synthetic Game

source saves (save, required): D:\Games\Synthetic Game\saves
  status: inventoried
  files: 2  bytes: 3
  7692c3ad3540bb803c020b3aee66cd8887123234ea0c6e7143c0add73ff431ed  3  2026-10-09T16:32:08.1234567Z  1.save
  e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855  0  2026-10-09T16:32:08.1234567Z  Profil été/2.save

findings: none
`
	if out != want {
		t.Fatalf("output:\n%s\nwant:\n%s", out, want)
	}
}

func TestRenderFindingsAndExitCodes(t *testing.T) {
	report := baseReport()
	report.Sources = append(report.Sources,
		inspect.SourceReport{Source: game.Source{ID: "config", Kind: game.KindConfig}, Path: `D:\Games\Synthetic Game\config`, Status: inspect.StatusMissing})
	report.Findings = []backup.Finding{
		{Severity: backup.Blocking, Code: inspect.CodeLinkNotFollowed, Source: "saves", Path: "jn", Detail: "junction or reparse point; not followed"},
		{Severity: backup.Info, Code: inspect.CodeSourceMissing, Source: "config", Path: `D:\Games\Synthetic Game\config`, Detail: "does not exist"},
	}
	out, err := execute(t, &fakeInspector{report: report}, "inspect", "--manifest", "m.yaml")
	var exit *cli.ExitError
	if !errors.As(err, &exit) || exit.Code != cli.ExitIncomplete || exit.Error() != "inspection incomplete: 1 blocking finding(s)" {
		t.Fatalf("error = %v", err)
	}
	for _, line := range []string{
		"source config (config, optional): D:\\Games\\Synthetic Game\\config\n  status: missing\n\nfindings:",
		"findings: 1 blocking, 1 info\n",
		"  blocking  link-not-followed  saves  jn  junction or reparse point; not followed\n",
		"  info  source-missing  config  D:\\Games\\Synthetic Game\\config  does not exist\n",
	} {
		if !strings.Contains(out, line) {
			t.Fatalf("output lacks %q:\n%s", line, out)
		}
	}

	report.Findings = report.Findings[1:]
	if _, err := execute(t, &fakeInspector{report: report}, "inspect", "--manifest", "m.yaml"); err != nil {
		t.Fatalf("information only must succeed: %v", err)
	}
}

func TestInspectInvocationErrors(t *testing.T) {
	failure := errors.New("m.yaml:2:1: game.id: must not be empty")
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"missing flag", []string{"inspect"}, `required flag(s) "manifest" not set`},
		{"positional argument", []string{"inspect", "--manifest", "m.yaml", "extra"}, "unknown command"},
		{"unknown flag", []string{"inspect", "--manifest", "m.yaml", "--json"}, "unknown flag"},
		{"service error", []string{"inspect", "--manifest", "m.yaml"}, failure.Error()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := execute(t, &fakeInspector{err: failure}, tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.want) || out != "" {
				t.Fatalf("error = %v, stdout = %q", err, out)
			}
			var exit *cli.ExitError
			if errors.As(err, &exit) {
				t.Fatalf("invocation errors must keep exit code 1: %v", err)
			}
		})
	}
}

func TestPrintableEscapesForgedOutput(t *testing.T) {
	report := baseReport()
	report.Sources[0].Entries = []backup.Entry{
		{Path: "evil\nfindings: none", ModTime: modTime},
		{Path: " padded ", ModTime: modTime},
		{Path: `"quoted"`, ModTime: modTime},
		{Path: "bad\xffutf8", ModTime: modTime},
		{Path: "tab\there", ModTime: modTime},
		{Path: "normal name é.save", ModTime: modTime},
	}
	out := cli.Render(report)
	for _, want := range []string{`"evil\nfindings: none"`, `" padded "`, `"\"quoted\""`, `"bad\xffutf8"`, `"tab\there"`, "  normal name é.save\n"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %s:\n%s", want, out)
		}
	}
	if strings.Count(out, "\nfindings:") != 1 {
		t.Fatalf("a file name forged a findings line:\n%s", out)
	}
}

// partialWriter accepts limit bytes, then fails, like a full disk or a
// closed pipe in the middle of the report.
type partialWriter struct {
	limit   int
	written bytes.Buffer
}

func (w *partialWriter) Write(p []byte) (int, error) {
	room := w.limit - w.written.Len()
	if room >= len(p) {
		return w.written.Write(p)
	}
	n, _ := w.written.Write(p[:max(room, 0)])
	return n, errors.New("disk full")
}

// A failed write cannot take back the prefix already written: stdout may
// hold a truncated report, and the error, never an ExitError, makes it exit 1.
func TestFailedWriteLeavesOnlyAPrefixAndAnError(t *testing.T) {
	clean := baseReport()
	blocking := baseReport()
	blocking.Findings = []backup.Finding{{Severity: backup.Blocking, Code: inspect.CodeUnreadable, Source: "saves", Path: "x", Detail: "locked"}}
	for name, report := range map[string]inspect.Report{"clean": clean, "blocking": blocking} {
		t.Run(name, func(t *testing.T) {
			full := cli.Render(report)
			stdout := &partialWriter{limit: 40}
			root := cli.NewRootCommand("dev", stdout, &bytes.Buffer{}, &fakeInspector{report: report}, nil)
			root.SetArgs([]string{"inspect", "--manifest", "m.yaml"})
			err := root.ExecuteContext(context.Background())
			var exit *cli.ExitError
			if err == nil || err.Error() != "disk full" || errors.As(err, &exit) {
				t.Fatalf("error = %v; a failed write must surface as a plain error", err)
			}
			got := stdout.written.String()
			if got == "" || got == full || !strings.HasPrefix(full, got) {
				t.Fatalf("stdout = %q, want a strict prefix of the report", got)
			}
		})
	}
}
