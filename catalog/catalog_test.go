package catalog

import (
	"slices"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
)

// The shipped list must parse, or the web function couldn't start.
func TestVettedListsTheTestLibrary(t *testing.T) {
	libraries, err := Vetted()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(libraries, domain.LibraryKey{Host: "github", RepositoryID: "1398540739"}) {
		t.Fatalf("vetted libraries are %v, without fabricahq/code-rules-test-library", libraries)
	}
}

func TestParseVettedRejectsAmbiguousLists(t *testing.T) {
	for name, input := range map[string]string{
		"a repeated library":   "libraries:\n  - {host: github, repositoryID: 1, repository: a/b}\n  - {host: github, repositoryID: 1, repository: a/c}\n",
		"a missing host":       "libraries:\n  - {repositoryID: 1, repository: a/b}\n",
		"another host":         "libraries:\n  - {host: gitlab, repositoryID: 1, repository: a/b}\n",
		"a missing ID":         "libraries:\n  - {host: github, repository: a/b}\n",
		"an ID that isn't one": "libraries:\n  - {host: github, repositoryID: fabricahq/rules, repository: a/b}\n",
		"a missing name":       "libraries:\n  - {host: github, repositoryID: 1}\n",
		"an unknown field":     "libraries:\n  - {host: github, repositoryID: 1, repository: a/b, vetted: yes}\n",
		"no list":              "{}\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseVetted([]byte(input)); err == nil {
				t.Fatal("accepted the list")
			}
		})
	}
}
