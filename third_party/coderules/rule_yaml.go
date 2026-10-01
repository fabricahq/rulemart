// Adapt YAML nodes to the rule format without coercing authored text fields.

package coderules

import (
	"fmt"
	"io"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v4"
)

// The reference uses YAML 1.2 core scalars. go-yaml also recognizes legacy dates,
// binary integers and numeric separators. Classify scalars here rather than
// decoding into Go strings, which would also coerce numbers and booleans.
var (
	yamlNull      = regexp.MustCompile(`^(?:~|null|Null|NULL)?$`)
	yamlBool      = regexp.MustCompile(`^(?:true|True|TRUE|false|False|FALSE)$`)
	yamlInt       = regexp.MustCompile(`^(?:[-+]?[0-9]+|0o[0-7]+|0x[0-9a-fA-F]+)$`)
	yamlFloat     = regexp.MustCompile(`^(?:[-+]?(?:\.[0-9]+|[0-9]+\.[0-9]*)(?:[eE][-+]?[0-9]+)?|[-+]?[0-9]+[eE][-+]?[0-9]+|[-+]?\.(?:inf|Inf|INF)|\.nan|\.NaN|\.NAN)$`)
	yamlTimestamp = regexp.MustCompile(`^[0-9]{4}-[0-9]{1,2}-[0-9]{1,2}(?:(?:t|T|[ \t]+)[0-9]{1,2}:[0-9]{1,2}:[0-9]{1,2}(?:\.[0-9]+)?(?:[ \t]*(?:Z|[-+][012]?[0-9](?::[0-9]{2})?))?)?$`)
)

// ruleYAML parses one frontmatter object and rejects malformed YAML, duplicate keys, and aliases.
func ruleYAML(text, location string) (map[string]*yaml.Node, error) {
	decoder := yaml.NewDecoder(strings.NewReader(yamlScalarEscapes(text)))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if err == io.EOF {
			return nil, invalid(location, "YAML frontmatter is empty; add title, impact, impactDescription, and whenToRead")
		}
		return nil, invalid(location, "invalid YAML: "+err.Error())
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, invalid(location, "invalid YAML: "+err.Error())
		}
		return nil, invalid(location, "invalid YAML: expected a single document")
	}
	// Validate syntax-level constraints throughout extension fields too. Never
	// follow aliases, including recursive ones, or expand their contents.
	aliases, err := inspectYAML(&document, location)
	if err != nil {
		return nil, err
	}
	if aliases {
		return nil, invalid(location, "YAML aliases are unsupported")
	}
	if len(document.Content) == 0 {
		return nil, invalid(location, "YAML frontmatter is empty; add title, impact, impactDescription, and whenToRead")
	}
	return yamlObject(document.Content[0], location)
}

// inspectYAML checks mapping keys and explicit tags throughout the tree and reports aliases without following them.
func inspectYAML(node *yaml.Node, location string) (bool, error) {
	aliases := node.Kind == yaml.AliasNode
	if node.Kind == yaml.MappingNode {
		seen := make(map[string]bool)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode {
				continue
			}
			kind, value := yamlScalar(key)
			// Object-valued keys and NaN are not equal scalar keys in the reference.
			if kind == "object" || kind == "nan" {
				continue
			}
			identity := kind + ":" + value
			if seen[identity] {
				return false, invalid(location, fmt.Sprintf("invalid YAML: duplicate mapping key %s at line %d, column %d", quote(key.Value), key.Line, key.Column))
			}
			seen[identity] = true
		}
	}
	if node.Style&yaml.TaggedStyle != 0 {
		switch node.ShortTag() {
		case "!!timestamp":
			if node.Kind == yaml.ScalarNode && !yamlTimestamp.MatchString(node.Value) {
				return false, invalid(location, fmt.Sprintf("invalid YAML: !!timestamp expects a date at line %d, column %d", node.Line, node.Column))
			}
		case "!!set":
			if node.Kind != yaml.MappingNode {
				return false, invalid(location, "invalid YAML: !!set expects a mapping")
			}
			for i := 1; i < len(node.Content); i += 2 {
				kind, _ := yamlScalar(node.Content[i])
				if kind != "null" {
					return false, invalid(location, "invalid YAML: set items must have null values")
				}
			}
		case "!!omap", "!!pairs":
			if node.Kind != yaml.SequenceNode {
				return false, invalid(location, "invalid YAML: ordered pairs expect a sequence")
			}
			for _, entry := range node.Content {
				if entry.Kind == yaml.MappingNode && len(entry.Content) > 2 {
					return false, invalid(location, "invalid YAML: each pair must have its own sequence indicator")
				}
			}
			if node.ShortTag() == "!!omap" {
				seen := make(map[string]bool)
				for _, entry := range node.Content {
					key := entry
					if entry.Kind == yaml.MappingNode {
						if len(entry.Content) == 0 {
							key = &yaml.Node{Kind: yaml.ScalarNode}
						} else {
							key = entry.Content[0]
						}
					}
					kind, value := yamlScalar(key)
					if kind == "object" {
						continue
					}
					identity := kind + ":" + value
					if seen[identity] {
						return false, invalid(location, "invalid YAML: duplicate ordered-map key "+quote(key.Value))
					}
					seen[identity] = true
				}
			}
		}
	}
	for _, child := range node.Content {
		found, err := inspectYAML(child, location)
		if err != nil {
			return false, err
		}
		aliases = aliases || found
	}
	return aliases, nil
}

// yamlObject extracts named fields and explicit merge defaults using the reference object semantics.
func yamlObject(node *yaml.Node, location string) (map[string]*yaml.Node, error) {
	// JS Object.entries sees no own fields on sets or ordered maps.
	if node.ShortTag() == "!!set" || node.ShortTag() == "!!omap" {
		return map[string]*yaml.Node{}, nil
	}
	if node.Kind != yaml.MappingNode {
		return nil, invalid(location, "expected an object")
	}
	result := make(map[string]*yaml.Node)
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if key.ShortTag() == "!!merge" && key.Style&yaml.TaggedStyle != 0 {
			sources := []*yaml.Node{value}
			if value.Kind == yaml.SequenceNode {
				sources = value.Content
			}
			for _, source := range sources {
				if source.Kind != yaml.MappingNode {
					return nil, invalid(location, "YAML aliases are unsupported")
				}
				fields, err := yamlObject(source, location)
				if err != nil {
					return nil, err
				}
				for name, field := range fields {
					if _, exists := result[name]; !exists {
						result[name] = field
					}
				}
			}
		}
	}
	// Own fields override merge defaults regardless of declaration order. Reject
	// non-string names so unknown fields cannot disappear during extraction.
	// This structural error precedes field-level checks such as license declarations.
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.ShortTag() == "!!merge" && key.Style&yaml.TaggedStyle != 0 {
			continue
		}
		if !yamlString(key) {
			return nil, invalid(location, "field names must be strings")
		}
		result[key.Value] = node.Content[i+1]
	}
	return result, nil
}

// yamlString reports whether the reference core schema treats a node as text.
func yamlString(node *yaml.Node) bool {
	kind, _ := yamlScalar(node)
	return kind == "string"
}

// yamlScalar returns a type and equality key, not a decoded application value.
// Explicit scalar tags that do not match their grammar fall back to text, as in
// the reference. Numeric keys use JS Number precision for duplicate detection.
func yamlScalar(node *yaml.Node) (string, string) {
	if node.Kind != yaml.ScalarNode {
		return "object", ""
	}
	tag := ""
	if node.Style&yaml.TaggedStyle != 0 {
		tag = node.ShortTag()
		switch tag {
		case "!!timestamp", "!!binary":
			return "object", ""
		case "!!null", "!!bool", "!!int", "!!float":
		default:
			return "string", node.Value
		}
	} else if node.Style&(yaml.DoubleQuotedStyle|yaml.SingleQuotedStyle|yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		return "string", node.Value
	}
	value := node.Value
	if (tag == "" || tag == "!!null") && yamlNull.MatchString(value) {
		return "null", ""
	}
	if (tag == "" || tag == "!!bool") && yamlBool.MatchString(value) {
		return "bool", strings.ToLower(value)
	}
	if (tag == "" || tag == "!!int") && yamlInt.MatchString(value) {
		base := 10
		digits := value
		if strings.HasPrefix(value, "0o") {
			base, digits = 8, value[2:]
		}
		if strings.HasPrefix(value, "0x") {
			base, digits = 16, value[2:]
		}
		integer, _ := new(big.Int).SetString(digits, base)
		number, _ := new(big.Float).SetInt(integer).Float64()
		return "number", strconv.FormatFloat(number, 'g', -1, 64)
	}
	if (tag == "" || tag == "!!float") && yamlFloat.MatchString(value) {
		lower := strings.ToLower(value)
		if lower == ".nan" {
			return "nan", ""
		}
		number, _ := strconv.ParseFloat(value, 64)
		if strings.HasSuffix(lower, ".inf") {
			number = math.Inf(1)
			if value[0] == '-' {
				number = math.Inf(-1)
			}
		}
		if number == 0 {
			number = 0
		} // JS treats -0 and 0 as equal keys.
		return "number", strconv.FormatFloat(number, 'g', -1, 64)
	}
	return "string", value
}

// yamlArray reports whether a node represents a reference array rather than an ordered map.
func yamlArray(node *yaml.Node) bool {
	return node.Kind == yaml.SequenceNode && node.ShortTag() != "!!omap"
}

// yamlEntries returns array elements, wrapping scalar YAML pairs as one-entry objects.
func yamlEntries(node *yaml.Node) []*yaml.Node {
	if node.ShortTag() != "!!pairs" {
		return node.Content
	}
	entries := make([]*yaml.Node, 0, len(node.Content))
	for _, entry := range node.Content {
		if entry.Kind == yaml.MappingNode {
			entries = append(entries, entry)
			continue
		}
		entries = append(entries, &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{entry, {Kind: yaml.ScalarNode, Tag: "!!null"}}})
	}
	return entries
}
