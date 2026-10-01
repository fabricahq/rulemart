// Package catalog holds the list of vetted libraries, which ships with each release.
package catalog

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"regexp"

	"go.yaml.in/yaml/v4"
)

//go:embed vetted.yaml
var vettedYAML []byte

// GitHub is the only code host Rulemart reads libraries from, as the catalog names it. Page URLs name no host,
// so they're GitHub's.
const GitHub = "github"

// Library identifies a library by its code host and the host's repository ID, as the catalog stores it.
type Library struct {
	// Host is the code host; github is the only one.
	Host string
	// RepositoryID is the host's ID for the repository. GitHub's is its numeric repository ID, in decimal.
	RepositoryID string
}

// Vetted returns the libraries vetted.yaml lists, in file order.
func Vetted() ([]Library, error) {
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
func parseVetted(input []byte) ([]Library, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(input))
	decoder.KnownFields(true)
	var file vettedFile
	if err := decoder.Decode(&file); err != nil {
		return nil, fmt.Errorf("read vetted libraries: %v", err)
	}
	if file.Libraries == nil {
		return nil, errors.New("read vetted libraries: expected a libraries list")
	}
	libraries := make([]Library, 0, len(file.Libraries))
	seen := map[Library]bool{}
	for i, entry := range file.Libraries {
		library := Library{Host: entry.Host, RepositoryID: entry.RepositoryID}
		switch {
		case entry.Host != GitHub:
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
