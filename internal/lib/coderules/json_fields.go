// Decode case-sensitive JSON fields while preserving authored text and diagnostic paths.

package coderules

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
)

// jsonObject distinguishes malformed JSON from missing, null, or non-object values.
func jsonObject(input json.RawMessage, location string) (map[string]json.RawMessage, error) {
	if len(input) > 0 && !json.Valid(input) {
		return nil, invalid(location, "invalid JSON")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(input, &fields) != nil || fields == nil {
		return nil, invalid(location, "expected an object")
	}
	return fields, nil
}

// knownJSONFields rejects unknown keys in deterministic order before reading values.
func knownJSONFields(fields map[string]json.RawMessage, allowed []string, location string) error {
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		if !slices.Contains(allowed, key) {
			return invalid(location, "unknown field "+key)
		}
	}
	return nil
}

// jsonText preserves nonblank text and rejects malformed Unicode rather than replacing it.
func jsonText(input json.RawMessage, location string) (string, error) {
	var text string
	if json.Unmarshal(input, &text) != nil || strings.TrimFunc(text, jsWhitespace) == "" {
		return "", invalid(location, "expected nonempty text")
	}
	if !validUnicodeString(input) {
		return "", invalid(location, "expected valid Unicode text: invalid UTF-8 or unpaired surrogate escape")
	}
	return text, nil
}

// jsonPath validates a contained relative file name without cleaning or accessing it.
func jsonPath(input json.RawMessage, location string) (string, error) {
	text, err := jsonText(input, location)
	if err != nil {
		return "", err
	}
	if !contained(text, strings.Split(text, "/")) {
		return "", invalid(location, "expected a contained relative path, got "+quote(text))
	}
	return text, nil
}
