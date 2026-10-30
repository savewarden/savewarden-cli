package sync_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/entities/game"
	"github.com/savewarden/savewarden-cli/internal/features/sync"
)

// withProfile declares the saves, required, and the optional sources named.
func withProfile(optional ...string) game.Manifest {
	sources := []game.Source{source("saves", "saves", true)}
	for _, id := range optional {
		sources = append(sources, source(id, id, false))
	}
	return manifest(sources...)
}

// policyWorld publishes a first generation of m whose optional sources
// each hold one file.
func policyWorld(t *testing.T, m game.Manifest) *fakeFS {
	t.Helper()
	f := world()
	for _, s := range m.Sources[1:] {
		f.put(installPath+`\`+s.Path+`\p.cfg`, f.file(s.ID))
	}
	result, err := service(f, m).Apply(context.Background(), sync.Request{ManifestPath: "m.yaml", VaultPath: vaultPath},
		writer(newFakeVaults(f), &fakeClock{now: at}))
	if err != nil || result.Effect != sync.EffectPublished {
		t.Fatalf("first publication: %+v, %v", result, err)
	}
	return f
}

type policyRun struct {
	result sync.Result
	err    error
	calls  int
	plan   sync.Plan
}

// applyPolicy plans, then applies the rotation of op1 by op3 with the
// given flags.
func applyPolicy(t *testing.T, f *fakeFS, m game.Manifest, allow bool, accept ...string) policyRun {
	t.Helper()
	request := sync.Request{ManifestPath: "m.yaml", VaultPath: vaultPath, AllowRemovals: allow, AcceptMissing: accept}
	var r policyRun
	r.plan, _ = service(f, m).Plan(context.Background(), request)
	vaults := newFakeVaults(f)
	r.result, r.err = service(f, m).Apply(context.Background(), request,
		sync.Writer{Vaults: vaults, Clock: &fakeClock{now: at.Add(timeLater)}, Random: strings.NewReader("\x2b\x3c\x4d\x5e"), ToolVersion: "dev"})
	r.calls = len(vaults.calls)
	return r
}

func (r policyRun) blockedBy(t *testing.T, code backup.Code) {
	t.Helper()
	var blocked *sync.BlockedError
	if !errors.As(r.err, &blocked) || r.calls != 0 || !strings.Contains(strings.Join(findings(r.plan), ","), "blocking "+string(code)) {
		t.Fatalf("want blocked by %s: %+v, %v, %d calls, findings %q", code, r.result, r.err, r.calls, findings(r.plan))
	}
}

func (r policyRun) published(t *testing.T) {
	t.Helper()
	if r.err != nil || r.result.Effect != sync.EffectPublished || r.plan.Blocking() != 0 {
		t.Fatalf("want published: %+v, %v, findings %q", r.result, r.err, findings(r.plan))
	}
}

func (r policyRun) invalid(t *testing.T, mention string) {
	t.Helper()
	var invocation *sync.InvocationError
	if !errors.As(r.err, &invocation) || r.calls != 0 || !strings.Contains(r.err.Error(), mention) {
		t.Fatalf("want an invalid invocation mentioning %q: %+v, %v, %d calls", mention, r.result, r.err, r.calls)
	}
}

func receiptSource(t *testing.T, f *fakeFS, op, id string) backup.ReceiptSource {
	t.Helper()
	receipt, err := backup.DecodeReceipt(f.get(gameDir + `\receipts\` + op + `.json`).data)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := receipt.Source(id)
	if !ok {
		t.Fatalf("receipt of %s lacks %s", op, id)
	}
	return s
}

// D5: --allow-removals accepts files removed from a present source, never
// an absent one; neither flag grants the other's permission.
func TestAllowRemovalsCoversRemovedFilesOnly(t *testing.T) {
	m := withProfile("profile")
	f := policyWorld(t, m)
	delete(f.get(installPath+`\saves`).children, "1.save")
	before := snapshot(f, `F:\`)
	applyPolicy(t, f, m, false).blockedBy(t, sync.CodeFilesRemoved)
	applyPolicy(t, f, m, false, "saves").invalid(t, "is required")
	if snapshot(f, `F:\`) != before {
		t.Fatal("a refused run wrote")
	}
	// A removal and a vanished source: each flag covers its own case only.
	both := policyWorld(t, m)
	delete(both.get(installPath+`\saves`).children, "1.save")
	delete(both.get(installPath).children, "profile")
	applyPolicy(t, both, m, false, "profile").blockedBy(t, sync.CodeFilesRemoved)
	applyPolicy(t, both, m, true).blockedBy(t, sync.CodeSourceVanished)

	r := applyPolicy(t, f, m, true)
	r.published(t)
	if !strings.Contains(strings.Join(findings(r.plan), ","), "info files-removed saves 1.save") {
		t.Fatalf("the accepted removal is not listed: %q", findings(r.plan))
	}
	if f.get(gameDir+`\current\saves\1.save`) != nil || f.get(gameDir+`\retained\`+op1+`\saves\1.save`) == nil {
		t.Fatal("the removed file is not kept in retained only")
	}
}

// V1: --accept-missing-source covers an optional source absent and known
// to the receipt of current; its files stay in retained, and the new
// receipt says missing and accepted.
func TestAcceptMissingSourceCoversVanishedSourceOnly(t *testing.T) {
	m := withProfile("profile")
	f := policyWorld(t, m)
	delete(f.get(installPath).children, "profile")
	applyPolicy(t, f, m, false).blockedBy(t, sync.CodeSourceVanished)
	applyPolicy(t, f, m, true).blockedBy(t, sync.CodeSourceVanished)
	r := applyPolicy(t, f, m, false, "profile")
	r.published(t)
	if !strings.Contains(strings.Join(findings(r.plan), ","), "info source-vanished profile") {
		t.Fatalf("the accepted absence is not listed: %q", findings(r.plan))
	}
	if s := receiptSource(t, f, op3, "profile"); s.State != backup.SourceMissing || !s.Accepted || len(s.Entries) != 0 {
		t.Fatalf("receipt source %+v", s)
	}
	if string(f.get(gameDir+`\retained\`+op1+`\profile\p.cfg`).data) != "profile" || f.get(gameDir+`\current\profile`) != nil {
		t.Fatal("the files of the missing source are not kept in retained only")
	}
}

// A required source absent, or a source removed from the manifest, blocks
// whatever the flags.
func TestRequiredOrRemovedSourcesStayBlocked(t *testing.T) {
	m := withProfile("profile")
	f := policyWorld(t, m)
	delete(f.get(installPath).children, "saves")
	applyPolicy(t, f, m, true).blockedBy(t, sync.CodeSourceMissing)
	applyPolicy(t, f, m, true, "saves").invalid(t, "is required")

	f = policyWorld(t, m)
	without := withProfile()
	applyPolicy(t, f, without, true).blockedBy(t, sync.CodeSourceRemoved)
	applyPolicy(t, f, without, true, "profile").invalid(t, "removed from the manifest")
}

// Naming a source that the flag does not cover is an invalid invocation:
// code 1, nothing written.
func TestInvalidAcceptMissingSourceIsCode1(t *testing.T) {
	m := withProfile("profile")
	published := policyWorld(t, m)
	for _, tt := range []struct {
		name   string
		f      *fakeFS
		m      game.Manifest
		accept []string
		want   string
	}{
		{"required source", published, m, []string{"saves"}, "is required"},
		{"present source", published, m, []string{"profile"}, "is present"},
		{"unknown source", published, m, []string{"nope"}, "is not a source of the manifest"},
		{"source removed from the manifest", published, withProfile(), []string{"profile"}, "removed from the manifest"},
		{"named twice", published, m, []string{"profile", "profile"}, "given twice"},
		{"absent from the receipt", func() *fakeFS {
			f := world()
			f.mkdir(vaultPath)
			return f
		}(), m, []string{"profile"}, "not in the receipt of the current generation"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := snapshot(tt.f, `F:\`)
			applyPolicy(t, tt.f, tt.m, false, tt.accept...).invalid(t, tt.want)
			if snapshot(tt.f, `F:\`) != before {
				t.Fatal("an invalid invocation wrote")
			}
		})
	}
}

func TestTwoAcceptedMissingSources(t *testing.T) {
	m := withProfile("profile", "extra")
	f := policyWorld(t, m)
	delete(f.get(installPath).children, "profile")
	delete(f.get(installPath).children, "extra")
	applyPolicy(t, f, m, false, "profile").blockedBy(t, sync.CodeSourceVanished)
	applyPolicy(t, f, m, false, "profile", "extra").published(t)
	for _, id := range []string{"profile", "extra"} {
		if s := receiptSource(t, f, op3, id); s.State != backup.SourceMissing || !s.Accepted {
			t.Fatalf("%s: %+v", id, s)
		}
	}
}
