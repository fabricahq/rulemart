// Package catalog holds the data the catalog ships with each release: the vetted libraries and Code Rules' canonical
// group list.
package catalog

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"regexp"

	"go.yaml.in/yaml/v4"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
)

//go:embed vetted.yaml
var vettedYAML []byte

// Vetted returns the libraries vetted.yaml lists, in file order.
func Vetted() ([]domain.LibraryKey, error) {
	return parseVetted(vettedYAML)
}

// vettedFile is vetted.yaml's structure.
type vettedFile struct {
	Libraries []struct {
		Host         string `yaml:"host"`
		RepositoryID string `yaml:"repositoryID"`
		// Repository is the library's owner/name when it was vetted, for readers.
		Repository string `yaml:"repository"`
	} `yaml:"libraries"`
}

// gitHubRepositoryID matches GitHub's numeric repository IDs.
var gitHubRepositoryID = regexp.MustCompile(`^[1-9][0-9]*$`)

// parseVetted reads a vetted list, rejecting unknown fields, hosts other than github, repository IDs that aren't
// GitHub's, libraries listed twice, and entries without a repository name.
func parseVetted(input []byte) ([]domain.LibraryKey, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(input))
	decoder.KnownFields(true)
	var file vettedFile
	if err := decoder.Decode(&file); err != nil {
		return nil, fmt.Errorf("read vetted libraries: %v", err)
	}
	if file.Libraries == nil {
		return nil, errors.New("read vetted libraries: expected a libraries list")
	}
	libraries := make([]domain.LibraryKey, 0, len(file.Libraries))
	seen := map[domain.LibraryKey]bool{}
	for i, entry := range file.Libraries {
		library := domain.LibraryKey{Host: entry.Host, RepositoryID: entry.RepositoryID}
		switch {
		case entry.Host != domain.GitHub:
			return nil, fmt.Errorf("read vetted libraries: libraries[%d]: expected host github, the only code host Rulemart reads", i)
		case !gitHubRepositoryID.MatchString(entry.RepositoryID):
			return nil, fmt.Errorf("read vetted libraries: libraries[%d]: expected GitHub's numeric repository ID in repositoryID", i)
		case entry.Repository == "":
			return nil, fmt.Errorf("read vetted libraries: libraries[%d]: expected the repository's owner/name", i)
		case seen[library]:
			return nil, fmt.Errorf("read vetted libraries: libraries[%d]: %s repository %s is listed twice", i, entry.Host, entry.RepositoryID)
		}
		seen[library] = true
		libraries = append(libraries, library)
	}
	return libraries, nil
}
