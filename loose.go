package cpanel

import (
	"bytes"
	"encoding/json"
)

// LooseString is a string with tolerant JSON decoding: cPanel's envelopes
// usually carry plain strings in their errors/messages/warnings lists, but
// some functions embed structured objects. LooseString accepts both; objects
// are preserved as their raw JSON representation.
type LooseString string

// String returns the underlying string.
func (s LooseString) String() string { return string(s) }

// UnmarshalJSON implements json.Unmarshaler.
func (s *LooseString) UnmarshalJSON(b []byte) error {
	if bytes.EqualFold(b, []byte("null")) {
		*s = ""
		return nil
	}
	var str string
	if err := json.Unmarshal(b, &str); err == nil {
		*s = LooseString(str)
		return nil
	}
	// Preserve structured values verbatim so no information is lost.
	*s = LooseString(compactJSON(b))
	return nil
}

// MarshalJSON implements json.Marshaler.
func (s LooseString) MarshalJSON() ([]byte, error) {
	var anyVal any
	if err := json.Unmarshal([]byte(s), &anyVal); err == nil {
		switch anyVal.(type) {
		case map[string]any, []any:
			// Round-trip structured payloads verbatim.
			return []byte(s), nil
		}
	}
	return json.Marshal(string(s))
}

func compactJSON(b []byte) []byte {
	var buf bytes.Buffer
	if err := json.Compact(&buf, b); err != nil {
		return b
	}
	return buf.Bytes()
}

func looseStringsToStrings(in []LooseString) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = string(s)
	}
	return out
}
