// Package game holds the declarative model of a protected game: its
// identity, its installation and the save sources a manifest declares.
// It depends on the standard library only and performs no I/O.
package game

// SchemaVersion is the only manifest schema version this build reads.
// Version 1 stays provisional until v0.1.0; see ADR 0002.
const SchemaVersion = 1

// Kind qualifies the data a source holds.
type Kind string

// Source kinds accepted by manifest v1.
const (
	KindSave     Kind = "save"
	KindProfile  Kind = "profile"
	KindConfig   Kind = "config"
	KindControls Kind = "controls"
	KindOther    Kind = "other"
)

// Valid reports whether k is a kind defined by manifest v1.
func (k Kind) Valid() bool {
	switch k {
	case KindSave, KindProfile, KindConfig, KindControls, KindOther:
		return true
	}
	return false
}

// Base names the directory a source path is relative to.
type Base string

// BaseGameInstall resolves a source under the installation directory.
// It is the only base of manifest v1.
const BaseGameInstall Base = "game-install"

// Manifest is the validated content of one manifest.yaml.
type Manifest struct {
	SchemaVersion int
	Game          Game
	Installation  Installation
	Sources       []Source
}

// Game identifies the protected game.
type Game struct {
	ID   string
	Name string
}

// Installation locates the game on this machine.
type Installation struct {
	// Path is an absolute Windows path with a drive letter, such as D:\Games\X.
	Path string
}

// Source declares one directory whose files are protected.
type Source struct {
	ID       string
	Kind     Kind
	Base     Base
	Path     string // relative Windows path, backslash separated
	Required bool
}

// FieldError reports the first invalid field of a manifest. Field uses the
// manifest key path, for example "sources[0].path".
type FieldError struct {
	Field   string
	Message string
}

func (e *FieldError) Error() string { return e.Field + ": " + e.Message }
