// Parse a group's display text and reading guidance without accessing files.

package coderules

import (
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// GroupMetadata describes a group for selection. Text has no surrounding whitespace.
type GroupMetadata struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	WhenToRead  string `json:"whenToRead" yaml:"whenToRead"`
}

// ParseGroupMetadata validates a group's JSON document and rejects unknown fields.
// License declarations belong to the library manifest and get specific errors.
// Field names are case-sensitive; repeated JSON keys use their last value.
// Text fields reject malformed Unicode instead of silently replacing it.
// On error, the returned metadata is the zero value.
func ParseGroupMetadata(input json.RawMessage, location string) (GroupMetadata, error) {
	if !json.Valid(input) {
		return GroupMetadata{}, invalid(location, "invalid JSON")
	}
	// Raw fields preserve exact key matching and defer decoding field values.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(input, &fields); err != nil || fields == nil {
		return GroupMetadata{}, invalid(location, "expected an object")
	}
	for _, key := range []string{"license", "licenses"} {
		if _, present := fields[key]; present {
			return GroupMetadata{}, invalid(location+"."+key, "declare one license for the whole library in rule-library.yaml; group-level licenses are unsupported")
		}
	}
	// Sort keys so multiple unknown fields produce a deterministic first error.
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		switch key {
		case "name", "description", "whenToRead":
		default:
			return GroupMetadata{}, invalid(location+"."+key, "unknown field; allowed fields: name, description, whenToRead")
		}
	}
	name, err := metadataText(fields["name"], location+".name")
	if err != nil {
		return GroupMetadata{}, err
	}
	description, err := metadataText(fields["description"], location+".description")
	if err != nil {
		return GroupMetadata{}, err
	}
	guidance, err := metadataText(fields["whenToRead"], location+".whenToRead")
	if err != nil {
		return GroupMetadata{}, err
	}
	return GroupMetadata{Name: name, Description: description, WhenToRead: guidance}, nil
}

func metadataText(input json.RawMessage, location string) (string, error) {
	if len(input) > 0 && input[0] == '"' && !validUnicodeString(input) {
		return "", invalid(location, "expected valid Unicode text: invalid UTF-8 or unpaired surrogate escape")
	}
	var text string
	if err := json.Unmarshal(input, &text); err != nil {
		return "", invalid(location, "expected nonempty text")
	}
	text = strings.TrimFunc(text, jsWhitespace)
	if text == "" {
		return "", invalid(location, "expected nonempty text")
	}
	return text, nil
}

// validUnicodeString checks a syntactically valid JSON string. ParseGroupMetadata
// checks syntax first, so escape lengths and hex digits are already validated.
// encoding/json otherwise replaces invalid UTF-8 and lone surrogates with U+FFFD.
func validUnicodeString(input json.RawMessage) bool {
	if !utf8.Valid(input) {
		return false
	}
	for i := 0; i < len(input); i++ {
		if input[i] != '\\' {
			continue
		}
		i++
		if input[i] != 'u' {
			continue
		}
		code, err := strconv.ParseUint(string(input[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if code >= 0xdc00 && code <= 0xdfff {
			return false
		}
		if code >= 0xd800 && code <= 0xdbff {
			if i+6 >= len(input) || input[i+1] != '\\' || input[i+2] != 'u' {
				return false
			}
			low, err := strconv.ParseUint(string(input[i+3:i+7]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}

// ParseGroupMetadataYAML validates one authored YAML group using the group metadata schema.
func ParseGroupMetadataYAML(input []byte, location string) (GroupMetadata, error) {
	_, data, err := authoredYAML(input, location)
	if err != nil {
		return GroupMetadata{}, err
	}
	return ParseGroupMetadata(data, location)
}
