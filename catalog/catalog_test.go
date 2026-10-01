package catalog

import (
	"slices"
	"testing"
)

// The shipped list must parse, or the web function couldn't start.
func TestVettedListsTheTestLibrary(t *testing.T) {
	ids, err := Vetted()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(ids, 1398540739) {
		t.Fatalf("vetted libraries are %v, without fabricahq/code-rules-test-library", ids)
	}
}

func TestParseVettedRejectsAmbiguousLists(t *testing.T) {
	for name, input := range map[string]string{
		"a repeated ID":    "libraries:\n  - {githubID: 1, repository: a/b}\n  - {githubID: 1, repository: a/c}\n",
		"a missing ID":     "libraries:\n  - {repository: a/b}\n",
		"a missing name":   "libraries:\n  - {githubID: 1}\n",
		"an unknown field": "libraries:\n  - {githubID: 1, repository: a/b, vetted: yes}\n",
		"no list":          "{}\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseVetted([]byte(input)); err == nil {
				t.Fatal("accepted the list")
			}
		})
	}
}
