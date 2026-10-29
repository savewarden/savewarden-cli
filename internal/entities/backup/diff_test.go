package backup_test

import (
	"crypto/sha256"
	"slices"
	"testing"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

func entry(path, content string, modTime time.Time) backup.Entry {
	return backup.Entry{Path: path, Size: int64(len(content)), ModTime: modTime, SHA256: sha256.Sum256([]byte(content))}
}

func TestCompare(t *testing.T) {
	t0 := at
	previous := []backup.Entry{
		entry("same.save", "same", t0), entry("size.save", "abc", t0), entry("time.save", "abc", t0),
		entry("hash.save", "abc", t0), entry("gone.save", "x", t0),
	}
	next := []backup.Entry{
		entry("time.save", "abc", t0.Add(time.Second)), entry("same.save", "same", t0.In(time.FixedZone("CET", 3600))),
		entry("size.save", "abcd", t0), entry("new.save", "n", t0), {Path: "hash.save", Size: 3, ModTime: t0, SHA256: sha256.Sum256([]byte("xyz"))},
	}
	c := backup.Compare(previous, []string{"empty", "kept"}, next, []string{"kept", "fresh"})
	var got []string
	for _, change := range c.Files {
		got = append(got, change.Kind.Marker()+change.Entry.Path)
	}
	want := []string{"-gone.save", "~hash.save", "+new.save", "=same.save", "~size.save", "~time.save"}
	if !slices.Equal(got, want) {
		t.Fatalf("changes = %q, want %q", got, want)
	}
	if c.Added != 1 || c.Modified != 3 || c.Removed != 1 || c.Unchanged != 1 {
		t.Fatalf("counts = %+v", c)
	}
	if !slices.Equal(c.AddedDirectories, []string{"fresh"}) || !slices.Equal(c.RemovedDirectories, []string{"empty"}) {
		t.Fatalf("directories = %q, %q", c.AddedDirectories, c.RemovedDirectories)
	}
	if c.None() || !c.Removals() {
		t.Fatal("None and Removals")
	}
	identical := backup.Compare(next, []string{"kept"}, next, []string{"kept"})
	if !identical.None() || identical.Removals() || identical.Unchanged != len(next) {
		t.Fatalf("identical = %+v", identical)
	}
	if first := backup.Compare(nil, nil, next, nil); first.Added != len(next) || first.Removals() {
		t.Fatalf("first generation = %+v", first)
	}
	if onlyDir := backup.Compare(nil, []string{"old"}, nil, nil); onlyDir.None() || !onlyDir.Removals() {
		t.Fatalf("a removed empty directory is a removal: %+v", onlyDir)
	}
}
