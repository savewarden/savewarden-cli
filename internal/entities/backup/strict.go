package backup

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// checkStrictJSON accepts exactly one JSON value without duplicate object
// keys at any depth and without trailing data. encoding/json alone keeps the
// last of two duplicate keys silently, which would let a forged field win.
func checkStrictJSON(data []byte) error {
	if !utf8.Valid(data) {
		return errors.New("not valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := strictValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("unexpected data after the JSON value")
	}
	return nil
}

func strictValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	switch token {
	case json.Delim('{'):
		keys := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if keys[name] {
				return fmt.Errorf("duplicate key %q", name)
			}
			keys[name] = true
			if err := strictValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case json.Delim('['):
		for decoder.More() {
			if err := strictValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	}
	return nil
}

// decodeStrict checks data with checkStrictJSON, then decodes it into v,
// refusing unknown fields.
func decodeStrict(data []byte, v any) error {
	if err := checkStrictJSON(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(v)
}

// nullable distinguishes a missing field from an explicit null.
type nullable struct {
	set bool
	raw json.RawMessage
}

func (n *nullable) UnmarshalJSON(data []byte) error {
	n.set = true
	n.raw = append(json.RawMessage(nil), data...)
	return nil
}

// optionalString decodes a field that may be null; it fails when missing.
func (n nullable) optionalString(field string) (string, bool, error) {
	if !n.set {
		return "", false, fmt.Errorf("%s: required field is missing", field)
	}
	if string(n.raw) == "null" {
		return "", false, nil
	}
	var s string
	if err := json.Unmarshal(n.raw, &s); err != nil {
		return "", false, fmt.Errorf("%s: must be a string or null", field)
	}
	return s, true, nil
}

func required[T any](field string, v *T) (T, error) {
	if v == nil {
		var zero T
		return zero, fmt.Errorf("%s: required field is missing", field)
	}
	return *v, nil
}

// timeLayout is RFC 3339 with optional fractional seconds; parseUTC also
// demands the Z suffix.
const timeLayout = time.RFC3339Nano

func parseUTC(field, value string) (time.Time, error) {
	if !strings.HasSuffix(value, "Z") {
		return time.Time{}, fmt.Errorf("%s: %q must be an RFC 3339 date in UTC, ending with Z", field, value)
	}
	t, err := time.Parse(timeLayout, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s: %q is not an RFC 3339 date", field, value)
	}
	return t.UTC(), nil
}

func formatUTC(t time.Time) string { return t.UTC().Format(timeLayout) }

var operationID = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}-[0-9a-f]{8}$`)

// ValidOperationID checks the AAAAMMJJTHHMMSS-xxxxxxxx form of ADR 0006: a
// UTC date and time, then eight lowercase hexadecimal digits.
func ValidOperationID(id string) error {
	if !operationID.MatchString(id) {
		return fmt.Errorf("operation id %q must look like 20261105T101500-1a2b3c4d", id)
	}
	if _, err := time.Parse("20060102T150405", id[:15]); err != nil {
		return fmt.Errorf("operation id %q has an invalid date or time", id)
	}
	return nil
}

func parseSHA256(field, value string) ([32]byte, error) {
	var sum [32]byte
	if len(value) != 64 || strings.ToLower(value) != value {
		return sum, fmt.Errorf("%s: must be 64 lowercase hexadecimal digits", field)
	}
	if _, err := hex.Decode(sum[:], []byte(value)); err != nil {
		return sum, fmt.Errorf("%s: must be 64 lowercase hexadecimal digits", field)
	}
	return sum, nil
}

// validRelative accepts slash-separated relative paths without empty, "."
// or ".." segments, backslashes or NUL characters.
func validRelative(path string) bool {
	if path == "" || strings.ContainsAny(path, "\\\x00") {
		return false
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}
