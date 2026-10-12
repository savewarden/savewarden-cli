package game

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxIDLength   = 64
	maxNameLength = 200
)

var slug = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Validate returns the first invalid field as a *FieldError, in manifest
// order, or nil. It performs no I/O: existence, links and overlaps through
// short names are checked later against the file system.
func (m Manifest) Validate() error {
	if m.SchemaVersion != SchemaVersion {
		return &FieldError{"schema_version", fmt.Sprintf("unsupported schema version %d; this build reads version %d", m.SchemaVersion, SchemaVersion)}
	}
	if err := checkSlug(m.Game.ID); err != nil {
		return &FieldError{"game.id", err.Error()}
	}
	if err := checkName(m.Game.Name); err != nil {
		return &FieldError{"game.name", err.Error()}
	}
	if _, _, err := ParseWindowsAbsolute(m.Installation.Path); err != nil {
		return &FieldError{"installation.path", err.Error()}
	}
	if len(m.Sources) == 0 {
		return &FieldError{"sources", "at least one source is required"}
	}
	seen := map[string]int{}
	segments := make([][]string, len(m.Sources))
	for i, source := range m.Sources {
		field := fmt.Sprintf("sources[%d]", i)
		if err := checkSlug(source.ID); err != nil {
			return &FieldError{field + ".id", err.Error()}
		}
		if first, ok := seen[source.ID]; ok {
			return &FieldError{field + ".id", fmt.Sprintf("duplicate source id %q, already used by sources[%d]", source.ID, first)}
		}
		seen[source.ID] = i
		if !source.Kind.Valid() {
			return &FieldError{field + ".kind", fmt.Sprintf("unknown kind %q; expected save, profile, config, controls or other", source.Kind)}
		}
		if source.Base != BaseGameInstall {
			return &FieldError{field + ".base", fmt.Sprintf("unknown base %q; manifest v1 supports game-install only", source.Base)}
		}
		parsed, err := ParseWindowsRelative(source.Path)
		if err != nil {
			return &FieldError{field + ".path", err.Error()}
		}
		segments[i] = parsed
		for j := range i {
			if SameSegments(segments[j], parsed) || SameSegments(parsed, segments[j]) {
				return &FieldError{field + ".path", fmt.Sprintf("overlaps source %q; sources must not coincide or contain each other", m.Sources[j].ID)}
			}
		}
	}
	return nil
}

// SourceSegments returns the segments of a validated source under the
// installation, from the volume root down to the source directory.
func (m Manifest) SourceSegments(index int) (volume string, segments []string, err error) {
	volume, installation, err := ParseWindowsAbsolute(m.Installation.Path)
	if err != nil {
		return "", nil, err
	}
	relative, err := ParseWindowsRelative(m.Sources[index].Path)
	if err != nil {
		return "", nil, err
	}
	return volume, append(append([]string{}, installation...), relative...), nil
}

func checkSlug(id string) error {
	if id == "" {
		return fmt.Errorf("must not be empty")
	}
	if len(id) > maxIDLength {
		return fmt.Errorf("longer than %d characters", maxIDLength)
	}
	if !slug.MatchString(id) {
		return fmt.Errorf("%q must use lowercase letters, digits and single hyphens, such as assassins-creed-odyssey", id)
	}
	return nil
}

func checkName(name string) error {
	switch {
	case !utf8.ValidString(name):
		return fmt.Errorf("must be valid UTF-8")
	case strings.TrimSpace(name) == "":
		return fmt.Errorf("must not be empty")
	case utf8.RuneCountInString(name) > maxNameLength:
		return fmt.Errorf("longer than %d characters", maxNameLength)
	case strings.IndexFunc(name, unicode.IsControl) >= 0:
		return fmt.Errorf("control characters are not allowed")
	}
	return nil
}
