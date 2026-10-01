// Parse the canonical group list: the group IDs that tools may treat as the same group across libraries.

package coderules

import (
	"encoding/json"
	"strings"
	"unicode"

	"go.yaml.in/yaml/v4"
)

// CanonicalGroup is one entry on the canonical group list. Text has no surrounding whitespace.
type CanonicalGroup struct {
	ID string
	// Name is the group's display name, unique on the list under Unicode case folding.
	Name string
	// Description is one line saying which rules belong in the group.
	Description string
}

// ParseCanonicalGroups validates a canonical group list and returns its entries in file order.
// The list is one strict YAML mapping from group ID to an entry with exactly a name and a description.
// IDs follow ValidateGroupID and must be in ascending byte order, so a list has no duplicates.
// Names and descriptions are single lines, and names are unique under Unicode case folding.
// On error, the result is nil.
func ParseCanonicalGroups(input []byte, location string) ([]CanonicalGroup, error) {
	document, data, err := authoredYAML(input, location)
	if err != nil {
		return nil, err
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, invalid(location, "expected a mapping from group ID to name and description")
	}
	if len(root.Content) == 0 {
		return nil, invalid(location, "expected at least one group")
	}
	entries, err := jsonObject(data, location)
	if err != nil {
		return nil, err
	}
	groups := make([]CanonicalGroup, 0, len(entries))
	// The YAML node keeps the authored key order, which the decoded JSON object loses.
	for i := 0; i < len(root.Content); i += 2 {
		id := root.Content[i].Value
		entryLocation := location + "." + id
		if err := ValidateGroupID(id, entryLocation); err != nil {
			return nil, err
		}
		if len(groups) > 0 && id <= groups[len(groups)-1].ID {
			return nil, invalid(entryLocation, quote(id)+" must come before "+quote(groups[len(groups)-1].ID)+": sort entries by group ID")
		}
		group, err := canonicalGroup(id, entries[id], entryLocation)
		if err != nil {
			return nil, err
		}
		// Unicode case folding equates names that lowercasing keeps apart, such as Σ and ς.
		for _, earlier := range groups {
			if strings.EqualFold(group.Name, earlier.Name) {
				return nil, invalid(entryLocation+".name", quote(group.Name)+" is already the name of "+quote(earlier.ID))
			}
		}
		groups = append(groups, group)
	}
	return groups, nil
}

func canonicalGroup(id string, input json.RawMessage, location string) (CanonicalGroup, error) {
	fields, err := jsonObject(input, location)
	if err != nil {
		return CanonicalGroup{}, err
	}
	if err := knownJSONFields(fields, []string{"name", "description"}, location); err != nil {
		return CanonicalGroup{}, err
	}
	name, err := canonicalText(fields["name"], location+".name")
	if err != nil {
		return CanonicalGroup{}, err
	}
	description, err := canonicalText(fields["description"], location+".description")
	if err != nil {
		return CanonicalGroup{}, err
	}
	return CanonicalGroup{ID: id, Name: name, Description: description}, nil
}

// canonicalText returns trimmed, nonempty text that fits on one line of a catalog or warning.
// It rejects every control character, including tabs and the vertical separators LF, VT, FF, CR, and NEL,
// as well as the Unicode line and paragraph separators.
func canonicalText(input json.RawMessage, location string) (string, error) {
	text, err := metadataText(input, location)
	if err != nil {
		return "", err
	}
	if strings.ContainsFunc(text, controlOrLineSeparator) {
		return "", invalid(location, "expected one line of text without control characters")
	}
	return text, nil
}

func controlOrLineSeparator(r rune) bool {
	return unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp)
}
