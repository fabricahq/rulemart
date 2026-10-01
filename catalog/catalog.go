// Package catalog holds the list of vetted libraries, which ships with each release.
package catalog

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"

	"go.yaml.in/yaml/v4"
)

//go:embed vetted.yaml
var vettedYAML []byte

// Vetted returns the GitHub repository IDs of the libraries vetted.yaml lists, in file order.
func Vetted() ([]int64, error) {
	return parseVetted(vettedYAML)
}

// vettedFile is vetted.yaml's structure.
type vettedFile struct {
	Libraries []struct {
		GitHubID int64 `yaml:"githubID"`
		// Repository is the library's owner/name when it was vetted, for readers.
		Repository string `yaml:"repository"`
	} `yaml:"libraries"`
}

// parseVetted reads a vetted list, rejecting unknown fields, missing or repeated IDs, and entries without a
// repository name.
func parseVetted(input []byte) ([]int64, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(input))
	decoder.KnownFields(true)
	var file vettedFile
	if err := decoder.Decode(&file); err != nil {
		return nil, fmt.Errorf("read vetted libraries: %v", err)
	}
	if file.Libraries == nil {
		return nil, errors.New("read vetted libraries: expected a libraries list")
	}
	ids := make([]int64, 0, len(file.Libraries))
	seen := map[int64]bool{}
	for i, library := range file.Libraries {
		switch {
		case library.GitHubID <= 0:
			return nil, fmt.Errorf("read vetted libraries: libraries[%d]: expected a GitHub repository ID in githubID", i)
		case library.Repository == "":
			return nil, fmt.Errorf("read vetted libraries: libraries[%d]: expected the repository's owner/name", i)
		case seen[library.GitHubID]:
			return nil, fmt.Errorf("read vetted libraries: libraries[%d]: GitHub repository ID %d is listed twice", i, library.GitHubID)
		}
		seen[library.GitHubID] = true
		ids = append(ids, library.GitHubID)
	}
	return ids, nil
}
