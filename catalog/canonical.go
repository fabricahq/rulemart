// Read Code Rules' canonical group list and Rulemart's icons for its groups, which ship with each release.

package catalog

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"regexp"

	"go.yaml.in/yaml/v4"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// canonicalGroupsYAML is canonical-groups.yaml from fabricahq/code-rules at v0.2.0, commit
// 6a63c7173bb5ae8b37cf22f30b8a4aede1d6b435, copied unchanged. Code Rules owns the list, so update it only by
// copying the file from a newer commit, and update the checksum its test pins with it.
//
//go:embed canonical-groups.yaml
var canonicalGroupsYAML []byte

//go:embed group-icons.yaml
var groupIconsYAML []byte

// CanonicalGroups returns Code Rules' canonical group list, as the vendored parser reads it, with the icons
// group-icons.yaml gives its groups.
func CanonicalGroups() (domain.CanonicalGroups, error) {
	list, err := coderules.ParseCanonicalGroups(canonicalGroupsYAML, "canonical-groups.yaml")
	if err != nil {
		return domain.CanonicalGroups{}, fmt.Errorf("read canonical groups: %v", err)
	}
	icons, err := parseGroupIcons(groupIconsYAML)
	if err != nil {
		return domain.CanonicalGroups{}, err
	}
	return domain.NewCanonicalGroups(list, icons)
}

// groupIcon is an entry of group-icons.yaml.
type groupIcon struct {
	File       string `yaml:"file"`
	Monochrome bool   `yaml:"monochrome"`
	Narrow     bool   `yaml:"narrow"`
}

// iconFile matches an icon's path under the site's icons: an SVG file directly in one icon set's directory.
var iconFile = regexp.MustCompile(`^[a-z0-9-]+/[a-z0-9.-]+\.svg$`)

// parseGroupIcons reads a group icons file, keyed by group ID. It rejects a file of more than one YAML document,
// unknown fields, a group listed twice or out of ID order, and an entry whose file isn't an SVG directly in an icon
// set's directory.
func parseGroupIcons(input []byte) (map[string]domain.GroupIcon, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(input))
	decoder.KnownFields(true)
	var entries map[string]groupIcon
	if err := decoder.Decode(&entries); err != nil {
		return nil, fmt.Errorf("read group icons: %v", err)
	}
	if len(entries) == 0 {
		return nil, errors.New("read group icons: expected a mapping from group ID to icon")
	}
	if err := decoder.Decode(&yaml.Node{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("read group icons: expected one YAML document")
	}
	if err := requireSortedKeys(input); err != nil {
		return nil, fmt.Errorf("read group icons: %v", err)
	}
	icons := make(map[string]domain.GroupIcon, len(entries))
	for id, entry := range entries {
		if !iconFile.MatchString(entry.File) {
			return nil, fmt.Errorf("read group icons: %s: expected file to name an SVG in an icon set's directory, such as devicon/go-original.svg", id)
		}
		icons[id] = domain.GroupIcon{File: entry.File, Monochrome: entry.Monochrome, Narrow: entry.Narrow}
	}
	return icons, nil
}

// requireSortedKeys rejects a YAML mapping whose keys aren't in ascending byte order, which keeps the file easy to
// review.
func requireSortedKeys(input []byte) error {
	var document yaml.Node
	if err := yaml.Unmarshal(input, &document); err != nil {
		return err
	}
	keys := document.Content[0].Content
	for i := 2; i < len(keys); i += 2 {
		if keys[i].Value <= keys[i-2].Value {
			return fmt.Errorf("%s must come before %s: sort entries by group ID", keys[i].Value, keys[i-2].Value)
		}
	}
	return nil
}
