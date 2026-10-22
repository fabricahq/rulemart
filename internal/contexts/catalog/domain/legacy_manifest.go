// Read the license that a library release in RootLayout declares in rule-library.yaml, the manifest before Trail
// Guide moved libraries into .trailguide/.

package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/fabricahq/trailguide/trailguide"
)

// parseLegacyLicense returns the license that text, the rule-library.yaml at location, declares, or nil when it
// declares none. The manifest must have formatVersion 1; its other fields are ignored. Trail Guide no longer parses
// rule-library.yaml, so this keeps the rules it had: each declared path is a contained path from the repository root,
// kept as written, outside rules and their asset directories in techs/ and practices/. The current library section
// places paths in .trailguide/ and reserves others, such as rules/, so it can't check these; it checks only the
// expression.
func parseLegacyLicense(text []byte, location string) (*trailguide.LicenseDeclaration, error) {
	var manifest struct {
		FormatVersion any       `yaml:"formatVersion"`
		License       yaml.Node `yaml:"license"`
	}
	if err := yaml.Unmarshal(text, &manifest); err != nil {
		return nil, fmt.Errorf("%s: invalid YAML: %v", location, err)
	}
	if version, ok := manifest.FormatVersion.(int); !ok || version != 1 {
		return nil, fmt.Errorf("%s: formatVersion: only library formatVersion 1 is supported", location)
	}
	if manifest.License.IsZero() {
		return nil, nil
	}
	var license map[string]any
	if err := manifest.License.Decode(&license); err != nil || license == nil {
		return nil, fmt.Errorf("%s: license: expected an object", location)
	}
	for field := range license {
		if !slices.Contains([]string{"file", "notices", "spdxExpression", "expression"}, field) {
			return nil, fmt.Errorf("%s: license: unknown field %s", location, field)
		}
	}
	file, err := legacyTermsPath(license["file"], location+": license.file")
	if err != nil {
		return nil, err
	}
	notices, ok := license["notices"].([]any)
	if !ok {
		return nil, fmt.Errorf("%s: license.notices: expected an array of notice paths", location)
	}
	result := trailguide.LicenseDeclaration{Files: []string{file}, AttributionFiles: []string{}}
	for i, notice := range notices {
		path, err := legacyTermsPath(notice, fmt.Sprintf("%s: license.notices[%d]", location, i))
		if err != nil {
			return nil, err
		}
		if !slices.Contains(result.Files, path) && !slices.Contains(result.AttributionFiles, path) {
			result.AttributionFiles = append(result.AttributionFiles, path)
		}
	}
	if _, ok := license["expression"]; ok {
		return nil, fmt.Errorf("%s: license.expression: renamed to license.spdxExpression; move the declaration to that field", location)
	}
	if expression, ok := license["spdxExpression"]; ok {
		text, ok := expression.(string)
		if !ok {
			return nil, fmt.Errorf("%s: license.spdxExpression: expected text", location)
		}
		if err := validateExpression(text); err != nil {
			return nil, fmt.Errorf("%s: license.spdxExpression: %v", location, err)
		}
		result.SPDXExpression = &text
	}
	return &result, nil
}

// legacyTermsPath returns the license or notice path that value declares at location: a contained path from the
// repository root, outside rules and their asset directories, which belong to a rule's version.
func legacyTermsPath(value any, location string) (string, error) {
	path, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s: expected a path", location)
	}
	if !contained(path) {
		return "", fmt.Errorf("%s: expected a contained relative path, got %q", location, path)
	}
	if legacyRuleContent(path) {
		return "", fmt.Errorf("%s: %q belongs to a rule's version; keep license and notice files outside rules and their asset directories, such as at the library root", location, path)
	}
	return path, nil
}

// contained reports whether path is a relative path with forward slashes and no drive prefix, empty, "." or ".."
// segments, or control characters.
func contained(path string) bool {
	if len(path) >= 2 && path[1] == ':' {
		return false
	}
	if strings.ContainsFunc(path, func(r rune) bool { return r == '\\' || r < 32 || r == 127 }) {
		return false
	}
	return !slices.ContainsFunc(strings.Split(path, "/"), func(part string) bool {
		return part == "" || part == "." || part == ".."
	})
}

// legacyRuleContent reports whether path, at the repository root of a library in RootLayout, is a rule file or a file
// in an asset directory inside a group. A group's README.md is neither.
func legacyRuleContent(path string) bool {
	parts := strings.Split(path, "/")
	if parts[0] != "techs" && parts[0] != "practices" {
		return false
	}
	if slices.Contains(parts[1:], "assets") {
		return true
	}
	group, err := trailguide.GroupFromPath(path, path)
	return err == nil && path != group+"/README.md"
}

// validateExpression checks an SPDX expression the way Trail Guide's library section does.
func validateExpression(expression string) error {
	section, err := json.Marshal(map[string]any{"schemaVersion": trailguide.LibrarySchemaVersion, "license": map[string]any{"spdxExpression": expression, "file": "LICENSE", "notices": []string{}}})
	if err != nil {
		return err
	}
	_, err = trailguide.ParseLibrarySection(section, nil, "")
	var validation *trailguide.ValidationError
	if errors.As(err, &validation) {
		return errors.New(validation.Problem)
	}
	return err
}
