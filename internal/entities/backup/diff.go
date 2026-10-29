package backup

import "sort"

// ChangeKind classifies one file between the previous generation and a new
// inventory.
type ChangeKind int

// Change kinds.
const (
	Unchanged ChangeKind = iota
	Added
	Modified
	Removed
)

func (k ChangeKind) String() string {
	switch k {
	case Added:
		return "added"
	case Modified:
		return "modified"
	case Removed:
		return "removed"
	}
	return "unchanged"
}

// Marker is the one-character prefix of a change in a plan.
func (k ChangeKind) Marker() string {
	switch k {
	case Added:
		return "+"
	case Modified:
		return "~"
	case Removed:
		return "-"
	}
	return "="
}

// Change is one file of a comparison. Entry is the new file, or the
// previous one when it was removed.
type Change struct {
	Kind  ChangeKind
	Entry Entry
}

// Changes compares the files and directories of two generations.
type Changes struct {
	Files              []Change // sorted by path, byte order
	AddedDirectories   []string
	RemovedDirectories []string
	Added              int
	Modified           int
	Removed            int
	Unchanged          int
}

// None reports whether the two generations are identical.
func (c Changes) None() bool {
	return c.Added == 0 && c.Modified == 0 && c.Removed == 0 &&
		len(c.AddedDirectories) == 0 && len(c.RemovedDirectories) == 0
}

// Removals reports whether files or directories would disappear.
func (c Changes) Removals() bool { return c.Removed > 0 || len(c.RemovedDirectories) > 0 }

// Compare lists what changes from the previous files and directories to the
// next ones. A file is modified when its size, modification time or SHA-256
// differs. Inputs need not be sorted; outputs are.
func Compare(previous []Entry, previousDirs []string, next []Entry, nextDirs []string) Changes {
	var c Changes
	before := make(map[string]Entry, len(previous))
	for _, entry := range previous {
		before[entry.Path] = entry
	}
	seen := make(map[string]bool, len(next))
	for _, entry := range next {
		seen[entry.Path] = true
		old, ok := before[entry.Path]
		switch {
		case !ok:
			c.Files = append(c.Files, Change{Added, entry})
			c.Added++
		case old.Size != entry.Size || !old.ModTime.Equal(entry.ModTime) || old.SHA256 != entry.SHA256:
			c.Files = append(c.Files, Change{Modified, entry})
			c.Modified++
		default:
			c.Files = append(c.Files, Change{Unchanged, entry})
			c.Unchanged++
		}
	}
	for _, entry := range previous {
		if !seen[entry.Path] {
			c.Files = append(c.Files, Change{Removed, entry})
			c.Removed++
		}
	}
	sort.Slice(c.Files, func(a, b int) bool { return c.Files[a].Entry.Path < c.Files[b].Entry.Path })
	c.AddedDirectories = difference(nextDirs, previousDirs)
	c.RemovedDirectories = difference(previousDirs, nextDirs)
	return c
}

// difference returns the sorted elements of a absent from b.
func difference(a, b []string) []string {
	in := make(map[string]bool, len(b))
	for _, s := range b {
		in[s] = true
	}
	var out []string
	for _, s := range a {
		if !in[s] {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
