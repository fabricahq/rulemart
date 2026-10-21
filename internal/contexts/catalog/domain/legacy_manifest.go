// Read the license that a library release in RootLayout declares in rule-library.yaml, the manifest before Trail
// Guide moved libraries into .trailguide/.

package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/fabricahq/trailguide/trailguide"
)

// parseLegacyLicense returns the license that text, the rule-library.yaml at location, declares, or nil when it
// declares none. The manifest must have formatVersion 1; its other fields are ignored. Its license has the shape of
// the library section's license, at library paths, so it is checked as Trail Guide checks that one: Trail Guide
// itself no longer parses rule-library.yaml, and the trailguide package has no parser for it.
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
	if _, ok := license["expression"]; ok {
		return nil, fmt.Errorf("%s: license.expression: renamed to license.spdxExpression; move the declaration to that field", location)
	}
	// Declared paths are library paths, which the library section declares at their repository paths.
	if file, ok := license["file"].(string); ok {
		license["file"] = trailguide.RepositoryPath(file)
	}
	if notices, ok := license["notices"].([]any); ok {
		for i, notice := range notices {
			if path, ok := notice.(string); ok {
				notices[i] = trailguide.RepositoryPath(path)
			}
		}
	}
	section, err := json.Marshal(map[string]any{"license": license})
	if err != nil {
		return nil, fmt.Errorf("%s: license: %v", location, err)
	}
	library, err := trailguide.ParseLibrarySection(section, nil, "")
	var validation *trailguide.ValidationError
	if errors.As(err, &validation) {
		return nil, fmt.Errorf("%s: %s: %s", location, strings.TrimPrefix(validation.Location, "."), validation.Problem)
	}
	if err != nil {
		return nil, err
	}
	return library.License, nil
}
