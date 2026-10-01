// Validate library manifest declarations and return safe paths for callers to read.

package coderules

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// LicenseDeclaration records authored terms and paths, without interpreting legal meaning.
// A nil SPDXExpression means the library declared a file but no SPDX identifier.
type LicenseDeclaration struct {
	SPDXExpression   *string  `json:"spdxExpression"`
	Files            []string `json:"files"`
	AttributionFiles []string `json:"attributionFiles"`
}

// ParseLibraryLicense validates authored YAML and returns safe declared paths without checking file presence.
// Declared paths must be library-wide files, never a rule's Markdown file or a file in an asset directory inside
// a group. Missing licensing returns nil, not an inferred license. Unknown manifest fields are allowed;
// license fields are strict. Callers read retained bytes through their confined file reader.
func ParseLibraryLicense(text []byte, source string) (*LicenseDeclaration, error) {
	location := source + "/rule-library.yaml"
	if !utf8.Valid(text) {
		return nil, invalid(location, "expected UTF-8 text")
	}
	_, data, err := authoredYAML(text, location)
	if err != nil {
		return nil, err
	}
	manifest, err := jsonObject(data, location)
	if err != nil {
		return nil, err
	}
	var format float64
	if json.Unmarshal(manifest["formatVersion"], &format) != nil || format != 1 {
		return nil, invalid(location+": formatVersion", "only library formatVersion 1 is supported")
	}
	raw, ok := manifest["license"]
	if !ok {
		return nil, nil
	}
	fields, err := jsonObject(raw, location+": license")
	if err != nil {
		return nil, err
	}
	location += ": license"
	if err := knownJSONFields(fields, []string{"file", "notices", "spdxExpression", "expression"}, location); err != nil {
		return nil, err
	}
	file, err := termsPath(fields["file"], location+".file")
	if err != nil {
		return nil, err
	}
	var rawNotices []json.RawMessage
	if json.Unmarshal(fields["notices"], &rawNotices) != nil || rawNotices == nil {
		return nil, invalid(location+".notices", "expected an array of notice paths")
	}
	notices := make([]string, len(rawNotices))
	for i, raw := range rawNotices {
		path, err := termsPath(raw, fmt.Sprintf("%s.notices[%d]", location, i))
		if err != nil {
			return nil, err
		}
		notices[i] = path
	}
	if _, ok := fields["expression"]; ok {
		return nil, invalid(location+".expression", "renamed to license.spdxExpression; move the declaration to that field")
	}
	var expression *string
	if raw, ok := fields["spdxExpression"]; ok {
		text, err := jsonText(raw, location+".spdxExpression")
		if err != nil {
			return nil, err
		}
		if strings.ContainsFunc(text, controlCharacter) {
			return nil, invalid(location+".spdxExpression", "expected a single-line license expression")
		}
		if !validSPDXExpression(text) {
			return nil, invalid(location+".spdxExpression", "expected an SPDX expression using recognized identifiers or LicenseRef- custom terms")
		}
		expression = &text
	}
	result := LicenseDeclaration{SPDXExpression: expression, Files: []string{file}, AttributionFiles: []string{}}
	seen := map[string]bool{file: true}
	for _, path := range notices {
		if !seen[path] {
			result.AttributionFiles = append(result.AttributionFiles, path)
			seen[path] = true
		}
	}
	return &result, nil
}

// termsPath reads a declared license or notice path, which must be a library-wide file: a rule's Markdown file or
// a file in an asset directory inside a group belongs to that rule's version, so declaring it would let a library
// release replace a rule version's content.
func termsPath(input json.RawMessage, location string) (string, error) {
	path, err := jsonPath(input, location)
	if err != nil {
		return "", err
	}
	if isRuleContent(path) {
		return "", invalid(location, quote(path)+" belongs to a rule's version; keep license and notice files outside rules and their asset directories, such as at the library root")
	}
	return path, nil
}

// controlCharacter identifies bytes that cannot occur in a single-line SPDX expression.
func controlCharacter(r rune) bool { return r < 32 || r == 127 }
