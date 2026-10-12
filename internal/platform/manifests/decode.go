// Package manifests reads manifest.yaml files strictly into game.Manifest.
//
// The YAML library only parses the text into a node tree. Fields are then
// decoded by hand so that every ambiguous construct is refused explicitly:
// several documents, anchors, aliases, merge keys, explicit tags, duplicate
// or unknown keys, YAML 1.1 booleans and implicit type conversions.
// See ADR 0002.
package manifests

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"

	"github.com/savewarden/savewarden-cli/internal/entities/game"
)

// MaxSize is the largest manifest accepted, in bytes.
const MaxSize = 64 << 10

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// Error locates a manifest problem. Line and Column are zero when unknown.
type Error struct {
	File    string
	Line    int
	Column  int
	Field   string
	Message string
}

func (e *Error) Error() string {
	location := e.File
	if e.Line > 0 {
		location += fmt.Sprintf(":%d:%d", e.Line, e.Column)
	}
	if e.Field != "" {
		return location + ": " + e.Field + ": " + e.Message
	}
	return location + ": " + e.Message
}

// Reader reads manifests from the local file system.
type Reader struct{}

// Read loads, decodes and validates the manifest at path.
func (Reader) Read(path string) (game.Manifest, error) {
	info, err := os.Stat(path)
	if err != nil {
		return game.Manifest{}, fmt.Errorf("manifest: %w", err)
	}
	if !info.Mode().IsRegular() {
		return game.Manifest{}, &Error{File: path, Message: "not a regular file"}
	}
	file, err := os.Open(path) //nolint:gosec // G304: the manifest path is the user's explicit input.
	if err != nil {
		return game.Manifest{}, fmt.Errorf("manifest: %w", err)
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, MaxSize+1))
	if err != nil {
		return game.Manifest{}, fmt.Errorf("manifest: %w", err)
	}
	return Decode(path, data)
}

// Decode decodes and validates manifest content. name is used in errors.
func Decode(name string, data []byte) (game.Manifest, error) {
	if len(data) > MaxSize {
		return game.Manifest{}, &Error{File: name, Message: fmt.Sprintf("larger than %d KiB", MaxSize>>10)}
	}
	if bytes.HasPrefix(data, []byte{0xFE, 0xFF}) || bytes.HasPrefix(data, []byte{0xFF, 0xFE}) {
		return game.Manifest{}, &Error{File: name, Message: "UTF-16 is not supported; save the manifest as UTF-8"}
	}
	data = bytes.TrimPrefix(data, utf8BOM)
	if bytes.IndexByte(data, 0) >= 0 {
		return game.Manifest{}, &Error{File: name, Message: "contains a NUL byte; the manifest must be UTF-8 text"}
	}
	if !utf8.Valid(data) {
		return game.Manifest{}, &Error{File: name, Message: "not valid UTF-8"}
	}

	stream := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := stream.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return game.Manifest{}, &Error{File: name, Message: "empty manifest"}
		}
		return game.Manifest{}, &Error{File: name, Message: "invalid YAML: " + err.Error()}
	}
	var extra yaml.Node
	switch err := stream.Decode(&extra); {
	case err == nil:
		return game.Manifest{}, &Error{File: name, Line: extra.Line, Column: extra.Column, Message: "several YAML documents; a manifest holds exactly one"}
	case !errors.Is(err, io.EOF):
		return game.Manifest{}, &Error{File: name, Message: "invalid YAML: " + err.Error()}
	}
	if len(document.Content) != 1 {
		return game.Manifest{}, &Error{File: name, Message: "empty manifest"}
	}
	d := &decoder{file: name, values: map[string]*yaml.Node{}}
	if err := d.refuseAmbiguity(&document); err != nil {
		return game.Manifest{}, err
	}
	manifest, err := d.manifest(document.Content[0])
	if err != nil {
		return game.Manifest{}, err
	}
	if err := manifest.Validate(); err != nil {
		var fieldErr *game.FieldError
		if errors.As(err, &fieldErr) {
			return game.Manifest{}, d.errorAt(d.values[fieldErr.Field], fieldErr.Field, fieldErr.Message)
		}
		return game.Manifest{}, err
	}
	return manifest, nil
}

type decoder struct {
	file   string
	values map[string]*yaml.Node // field path -> value node, for positions
}

func (d *decoder) errorAt(node *yaml.Node, field, message string) error {
	err := &Error{File: d.file, Field: field, Message: message}
	if node != nil {
		err.Line, err.Column = node.Line, node.Column
	}
	return err
}

// refuseAmbiguity rejects constructs whose meaning is easy to misread.
func (d *decoder) refuseAmbiguity(node *yaml.Node) error {
	switch {
	case node.Kind == yaml.AliasNode:
		return d.errorAt(node, "", "aliases are not allowed")
	case node.Anchor != "":
		return d.errorAt(node, "", "anchors are not allowed")
	case node.Style&yaml.TaggedStyle != 0:
		return d.errorAt(node, "", "explicit tags such as "+node.Tag+" are not allowed")
	}
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			if key := node.Content[i]; key.Kind == yaml.ScalarNode && key.ShortTag() == "!!merge" {
				return d.errorAt(key, "", "merge keys (<<) are not allowed")
			}
		}
	}
	for _, child := range node.Content {
		if err := d.refuseAmbiguity(child); err != nil {
			return err
		}
	}
	return nil
}

type field struct {
	name     string
	required bool
	decode   func(value *yaml.Node, path string) error
}

// object decodes a mapping, refusing unknown, duplicate and missing keys.
func (d *decoder) object(node *yaml.Node, path string, fields []field) error {
	if node.Kind != yaml.MappingNode {
		return d.errorAt(node, path, "expected a mapping")
	}
	seen := map[string]*yaml.Node{}
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if key.Kind != yaml.ScalarNode || key.ShortTag() != "!!str" {
			return d.errorAt(key, path, "keys must be plain strings")
		}
		child := join(path, key.Value)
		if first, duplicate := seen[key.Value]; duplicate {
			return d.errorAt(key, child, fmt.Sprintf("duplicate key, first defined at line %d", first.Line))
		}
		seen[key.Value] = key
		spec := lookup(fields, key.Value)
		if spec == nil {
			return d.errorAt(key, child, "unknown field")
		}
		d.values[child] = value
		if err := spec.decode(value, child); err != nil {
			return err
		}
	}
	for _, spec := range fields {
		if spec.required && seen[spec.name] == nil {
			return d.errorAt(node, join(path, spec.name), "required field is missing")
		}
	}
	return nil
}

func (d *decoder) manifest(node *yaml.Node) (game.Manifest, error) {
	var m game.Manifest
	d.values[""] = node
	err := d.object(node, "", []field{
		{"schema_version", true, func(v *yaml.Node, p string) (err error) {
			m.SchemaVersion, err = d.integer(v, p)
			return err
		}},
		{"game", true, func(v *yaml.Node, p string) error {
			return d.object(v, p, []field{
				{"id", true, d.str(&m.Game.ID)},
				{"name", true, d.str(&m.Game.Name)},
			})
		}},
		{"installation", true, func(v *yaml.Node, p string) error {
			return d.object(v, p, []field{{"path", true, d.str(&m.Installation.Path)}})
		}},
		{"sources", true, func(v *yaml.Node, p string) error {
			if v.Kind != yaml.SequenceNode {
				return d.errorAt(v, p, "expected a list of sources")
			}
			m.Sources = make([]game.Source, len(v.Content))
			for i, item := range v.Content {
				itemPath := fmt.Sprintf("%s[%d]", p, i)
				d.values[itemPath] = item
				if err := d.source(item, itemPath, &m.Sources[i]); err != nil {
					return err
				}
			}
			return nil
		}},
	})
	return m, err
}

func (d *decoder) source(node *yaml.Node, path string, s *game.Source) error {
	var kind, base string
	err := d.object(node, path, []field{
		{"id", true, d.str(&s.ID)},
		{"kind", true, d.str(&kind)},
		{"base", true, d.str(&base)},
		{"path", true, d.str(&s.Path)},
		{"required", true, func(v *yaml.Node, p string) (err error) {
			s.Required, err = d.boolean(v, p)
			return err
		}},
	})
	s.Kind, s.Base = game.Kind(kind), game.Base(base)
	return err
}

func (d *decoder) str(target *string) func(*yaml.Node, string) error {
	return func(v *yaml.Node, p string) error {
		if v.Kind != yaml.ScalarNode || v.ShortTag() != "!!str" {
			return d.errorAt(v, p, "expected a string; quote values such as 1943 or true")
		}
		*target = v.Value
		return nil
	}
}

var decimal = regexp.MustCompile(`^(0|[1-9][0-9]{0,8})$`)

func (d *decoder) integer(v *yaml.Node, path string) (int, error) {
	if v.Kind != yaml.ScalarNode || v.ShortTag() != "!!int" || !decimal.MatchString(v.Value) {
		return 0, d.errorAt(v, path, "expected a decimal integer such as 1")
	}
	return strconv.Atoi(v.Value)
}

func (d *decoder) boolean(v *yaml.Node, path string) (bool, error) {
	if v.Kind == yaml.ScalarNode && v.ShortTag() == "!!bool" {
		switch v.Value {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
	}
	return false, d.errorAt(v, path, "expected true or false")
}

func lookup(fields []field, name string) *field {
	for i := range fields {
		if fields[i].name == name {
			return &fields[i]
		}
	}
	return nil
}

func join(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}
