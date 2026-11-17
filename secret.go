package rauthy

import (
	"encoding/json"
	"fmt"
)

// Secret holds a credential that is redacted during diagnostic formatting.
// Its indirect representation also protects values inside unexported fields,
// where fmt bypasses formatting methods. Use Reveal only at a credential sink.
// The zero value represents an empty credential.
type Secret struct{ v *string }

// NewSecret wraps a credential without exposing it through fmt.
func NewSecret(value string) Secret { return Secret{v: &value} }

// Reveal returns the original credential.
func (s Secret) Reveal() string {
	if s.v == nil {
		return ""
	}
	return *s.v
}

// MarshalJSON encodes the original credential as a JSON string.
func (s Secret) MarshalJSON() ([]byte, error) { return json.Marshal(s.Reveal()) }

// UnmarshalJSON decodes a credential without retaining the input buffer.
func (s *Secret) UnmarshalJSON(raw []byte) error {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("decode secret: %w", err)
	}
	*s = NewSecret(value)
	return nil
}

// String redacts diagnostic output.
func (s Secret) String() string { return "[REDACTED]" }

// GoString redacts Go-syntax diagnostic output.
func (s Secret) GoString() string { return s.String() }

// Format redacts every formatting verb, flag, width and precision.
func (s Secret) Format(state fmt.State, _ rune) { _, _ = fmt.Fprint(state, s.String()) }
