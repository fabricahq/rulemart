// Decode authored YAML with one shared strict policy before applying each document schema.

package coderules

import (
	"bytes"
	"encoding/json"
	"go.yaml.in/yaml/v4"
	"io"
	"unicode/utf8"
)

func authoredYAML(input []byte, location string) (*yaml.Node, []byte, error) {
	document, err := authoredDocument(input, location)
	if err != nil {
		return nil, nil, err
	}
	data, err := strictJSON(document.Content[0], location)
	return document, data, err
}

// authoredDocument decodes exactly one YAML document with content and checks the rules that hold for every node
// of an authored document: no anchors, aliases, explicit tags, or duplicate keys, scalar mapping keys, and at most
// 32 levels of nesting. It interprets no scalar values, so callers can ignore fields whose values they can't read.
func authoredDocument(input []byte, location string) (*yaml.Node, error) {
	if !utf8.Valid(input) {
		return nil, invalid(location, "expected UTF-8 YAML")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(input))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, invalid(location, "expected one YAML document")
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, invalid(location, "expected one YAML document")
	}
	if len(document.Content) != 1 {
		return nil, invalid(location, "expected a mapping")
	}
	if err := authoredHygiene(document.Content[0], location, 0); err != nil {
		return nil, err
	}
	return &document, nil
}

// authoredHygiene checks node and everything below it against authoredDocument's rules.
func authoredHygiene(node *yaml.Node, location string, depth int) error {
	if depth > 32 {
		return invalid(location, "YAML nesting is too deep")
	}
	if node.Anchor != "" || node.Kind == yaml.AliasNode || node.Style&yaml.TaggedStyle != 0 {
		return invalid(location, "anchors, aliases, and explicit tags are not supported")
	}
	switch node.Kind {
	case yaml.MappingNode:
		seen := map[string]bool{}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.Anchor != "" || key.Style&yaml.TaggedStyle != 0 {
				return invalid(location, "mapping keys must be strings; quote wildcard selectors")
			}
			if seen[key.Value] {
				return invalid(location+"."+key.Value, "duplicate field")
			}
			seen[key.Value] = true
			if err := authoredHygiene(node.Content[i+1], location+"."+key.Value, depth+1); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if err := authoredHygiene(child, location, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

// strictJSON converts node to JSON, accepting only strings, finite numbers, booleans, and null among scalars, as
// every authored field a reader interprets requires.
func strictJSON(node *yaml.Node, location string) ([]byte, error) {
	value, err := authoredValue(node, location, 0)
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func authoredValue(node *yaml.Node, location string, depth int) (any, error) {
	if depth > 32 {
		return nil, invalid(location, "YAML nesting is too deep")
	}
	if node.Anchor != "" || node.Kind == yaml.AliasNode || node.Style&yaml.TaggedStyle != 0 {
		return nil, invalid(location, "anchors, aliases, and explicit tags are not supported")
	}
	switch node.Kind {
	case yaml.MappingNode:
		result := map[string]any{}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || key.Anchor != "" || key.Style&yaml.TaggedStyle != 0 {
				return nil, invalid(location, "mapping keys must be strings; quote wildcard selectors")
			}
			if _, exists := result[key.Value]; exists {
				return nil, invalid(location+"."+key.Value, "duplicate field")
			}
			value, err := authoredValue(node.Content[i+1], location+"."+key.Value, depth+1)
			if err != nil {
				return nil, err
			}
			result[key.Value] = value
		}
		return result, nil
	case yaml.SequenceNode:
		result := make([]any, 0, len(node.Content))
		for _, child := range node.Content {
			value, err := authoredValue(child, location, depth+1)
			if err != nil {
				return nil, err
			}
			result = append(result, value)
		}
		return result, nil
	case yaml.ScalarNode:
		switch node.Tag {
		case "!!str":
			return node.Value, nil
		case "!!int", "!!float", "!!bool", "!!null":
			var value any
			if err := node.Decode(&value); err != nil {
				return nil, invalid(location, "invalid scalar")
			}
			if _, err := json.Marshal(value); err != nil {
				return nil, invalid(location, "expected a finite JSON scalar")
			}
			return value, nil
		}
	}
	return nil, invalid(location, "unsupported YAML value; quote strings such as dates and wildcard selectors")
}
