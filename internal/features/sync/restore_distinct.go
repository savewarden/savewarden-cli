package sync

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/entities/game"
)

// reference is a path a restore area must stay apart from (ADR 0006 §11):
// the vault, the installation, a source of the manifest, or a path the
// snapshot backed up.
type reference struct {
	label    string
	path     string
	volume   string
	segments []string
}

// observedChain holds the identities of the existing components of a path,
// from the root of its volume, root included, and whether the path itself
// exists.
type observedChain struct {
	ids      []backup.FileID
	complete bool
}

func (c observedChain) has(id backup.FileID) bool {
	for _, known := range c.ids {
		if known == id {
			return true
		}
	}
	return false
}

// chainIdentities reads, without following anything, the identities of the
// chain of segments in scope, from the volume root down to the deepest
// component that exists. A link, a reparse point, a file on the way or a
// component that cannot be read is an error: there is then no proof.
func chainIdentities(scope Scope, segments []string) (observedChain, error) {
	root, err := scope.Identity(".")
	if err != nil {
		return observedChain{}, err
	}
	c := observedChain{ids: []backup.FileID{root}}
	for k := 1; k <= len(segments); k++ {
		name := strings.Join(segments[:k], "/")
		info, err := scope.Lstat(name)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return c, nil
		case err != nil:
			return c, err
		case isLink(info.Mode()):
			return c, fmt.Errorf("%s is a link, junction or reparse point; not followed", name)
		case !info.IsDir() && k < len(segments):
			return c, fmt.Errorf("%s is not a directory", name)
		}
		id, err := scope.Identity(name)
		if err != nil {
			return c, err
		}
		c.ids = append(c.ids, id)
	}
	c.complete = true
	return c, nil
}

// distinctDestination applies the observability rule of ADR 0006 §11 to a
// restore destination and every reference: names first on a same letter,
// then identities from the volume roots in both directions. An identity
// that cannot be observed, and a reference on a letter without a volume,
// never prove separation: refused, code 1. No ancestor of the destination
// may hold the marker of a vault or of a restore area.
func (s *Service) distinctDestination(path, volume string, segments []string, refs []reference) error {
	scopes := map[string]Scope{}
	defer func() {
		for _, scope := range scopes {
			_ = scope.Close()
		}
	}()
	open := func(v string) (Scope, error) {
		key := strings.ToUpper(v)
		if scope, ok := scopes[key]; ok {
			return scope, nil
		}
		scope, err := s.files.OpenScope(v)
		if err != nil {
			return nil, err
		}
		scopes[key] = scope
		return scope, nil
	}
	destScope, err := open(volume)
	if errors.Is(err, fs.ErrNotExist) {
		return errDestination(path, "volume %s is not available", volume)
	}
	if err != nil {
		return errDestination(path, "open volume %s: %v", volume, err)
	}
	dest, err := chainIdentities(destScope, segments)
	if err != nil {
		return errDestination(path, "cannot be checked: %v", err)
	}
	// Every existing ancestor, the volume root included, the destination
	// itself excluded: none may be a vault or a restore area.
	ancestors := len(dest.ids)
	if dest.complete {
		ancestors--
	}
	for k := 0; k < ancestors; k++ {
		dir := "."
		if k > 0 {
			dir = strings.Join(segments[:k], "/")
		}
		for _, marker := range []string{backup.MarkerName, backup.RestoreAreaMarkerName} {
			_, err := destScope.Lstat(join(dir, marker))
			switch {
			case err == nil:
				return errDestination(path, "lies in a Savewarden vault or restore area, %s; choose a destination outside it", windowsPath(volume, segments[:k]))
			case !errors.Is(err, fs.ErrNotExist):
				return errDestination(path, "cannot be checked: %v", err)
			}
		}
	}
	deepest := dest.ids[len(dest.ids)-1]
	for _, ref := range refs {
		overlap := fmt.Sprintf("overlaps the %s %s; a restore never lands in the vault, the installation or a source", ref.label, ref.path)
		nested := game.SameSegments(ref.segments, segments) || game.SameSegments(segments, ref.segments)
		if strings.EqualFold(ref.volume, volume) && nested {
			return errDestination(path, "%s", overlap)
		}
		scope, err := open(ref.volume)
		if errors.Is(err, fs.ErrNotExist) {
			return errDestination(path, "separation from the %s %s cannot be proven: its drive has no volume now", ref.label, ref.path)
		}
		if err != nil {
			return errDestination(path, "separation from the %s %s cannot be proven: %v", ref.label, ref.path, err)
		}
		x, err := chainIdentities(scope, ref.segments)
		if err != nil {
			return errDestination(path, "separation from the %s %s cannot be proven: %v", ref.label, ref.path, err)
		}
		switch {
		case x.complete && dest.has(x.ids[len(x.ids)-1]):
			// The destination, or its parent if it is absent, lies in X.
			return errDestination(path, "%s", overlap)
		case dest.complete && x.has(deepest):
			// X, or its deepest existing ancestor, lies in the destination.
			return errDestination(path, "%s", overlap)
		case !x.complete && nested:
			// X resolves to nothing now; its drive may hold another disk
			// while its own volume is mounted under the destination's
			// letter.
			return errDestination(path, "%s", overlap)
		}
	}
	return nil
}
