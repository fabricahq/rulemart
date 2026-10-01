// Decode paired UTF-16 escapes in YAML double-quoted scalars without changing authored text.

package coderules

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"

	"go.yaml.in/yaml/v4"
)

var yamlSurrogatePair = regexp.MustCompile(`\\(?:u[Dd][89AaBb][0-9A-Fa-f]{2}|U0000[Dd][89AaBb][0-9A-Fa-f]{2})\\(?:u[Dd][CcDdEeFf][0-9A-Fa-f]{2}|U0000[Dd][CcDdEeFf][0-9A-Fa-f]{2})`)

// yamlScalarEscapes locates quoted scalars with one masked parse, then repairs only those source spans.
// Masking keeps every line and column intact. Its values are discarded, so plain,
// single-quoted and block text never inherit the temporary replacements.
func yamlScalarEscapes(text string) string {
	if !yamlSurrogatePair.MatchString(text) {
		return text
	}
	masked := yamlSurrogatePair.ReplaceAllStringFunc(text, maskYAMLPair)
	var document yaml.Node
	if yaml.NewDecoder(strings.NewReader(masked)).Decode(&document) != nil {
		// Let the normal decoder diagnose the original malformed document.
		return text
	}
	chars := []rune(text)
	lines := []int{0}
	for i, c := range chars {
		if c == '\r' && i+1 < len(chars) && chars[i+1] == '\n' {
			continue
		}
		if strings.ContainsRune("\r\n\u0085\u2028\u2029", c) {
			lines = append(lines, i+1)
		}
	}
	var starts []int
	quotedYAMLStarts(&document, chars, lines, &starts)
	slices.Sort(starts)
	var result strings.Builder
	last := 0
	for _, start := range starts {
		result.WriteString(string(chars[last:start]))
		scalar, end := decodeYAMLPairs(chars, start)
		result.WriteString(scalar)
		last = end
	}
	result.WriteString(string(chars[last:]))
	return result.String()
}

// maskYAMLPair replaces each surrogate with an equally wide valid escape for the location-only parse.
func maskYAMLPair(pair string) string {
	parts := strings.Split(pair[1:], "\\")
	for i, part := range parts {
		parts[i] = "\\" + part[:len(part)-4] + "FFFD"
	}
	return strings.Join(parts, "")
}

// quotedYAMLStarts records double-quoted scalar positions without following aliases.
func quotedYAMLStarts(node *yaml.Node, chars []rune, lines []int, starts *[]int) {
	if node.Kind == yaml.ScalarNode && node.Style&yaml.DoubleQuotedStyle != 0 {
		i := lines[node.Line-1] + node.Column - 1
		// Node positions can precede tag/anchor properties, including comments
		// between a property and its scalar. Quotes within anchors are literal.
		for i < len(chars) {
			switch chars[i] {
			case ' ', '\t', '\r', '\n', '\u0085', '\u2028', '\u2029':
				i++
			case '#':
				for i < len(chars) && !strings.ContainsRune("\r\n\u0085\u2028\u2029", chars[i]) {
					i++
				}
			case '&', '!':
				if chars[i] == '!' && i+1 < len(chars) && chars[i+1] == '<' {
					for i < len(chars) && chars[i] != '>' {
						i++
					}
					if i < len(chars) {
						i++
					}
					continue
				}
				for i < len(chars) && !strings.ContainsRune(" \t\r\n\u0085\u2028\u2029[]{},", chars[i]) {
					i++
				}
			case '"':
				*starts = append(*starts, i)
				return
			default:
				return
			}
		}
	}
	for _, child := range node.Content {
		quotedYAMLStarts(child, chars, lines, starts)
	}
}

// decodeYAMLPairs repairs valid pairs in one quoted scalar and returns its exclusive end position.
func decodeYAMLPairs(chars []rune, start int) (string, int) {
	var result strings.Builder
	result.WriteRune('"')
	for i := start + 1; i < len(chars); {
		if chars[i] == '"' {
			result.WriteRune('"')
			return result.String(), i + 1
		}
		if chars[i] != '\\' {
			result.WriteRune(chars[i])
			i++
			continue
		}
		high, highSize := yamlUnicodeEscape(chars[i:])
		if high >= 0xD800 && high <= 0xDBFF {
			low, lowSize := yamlUnicodeEscape(chars[i+highSize:])
			if low >= 0xDC00 && low <= 0xDFFF {
				result.WriteRune(utf16.DecodeRune(high, low))
				i += highSize + lowSize
				continue
			}
		}
		// Consume the escape as a unit so \\uD83D remains literal text.
		end := min(i+2, len(chars))
		result.WriteString(string(chars[i:end]))
		i = end
	}
	return result.String(), len(chars)
}

// yamlUnicodeEscape reads a four- or eight-digit YAML Unicode escape, returning zero length otherwise.
func yamlUnicodeEscape(chars []rune) (rune, int) {
	if len(chars) < 2 || chars[0] != '\\' {
		return 0, 0
	}
	size := 0
	switch chars[1] {
	case 'u':
		size = 6
	case 'U':
		size = 10
	default:
		return 0, 0
	}
	if len(chars) < size {
		return 0, 0
	}
	value, err := strconv.ParseUint(string(chars[2:size]), 16, 32)
	if err != nil {
		return 0, 0
	}
	return rune(value), size
}
