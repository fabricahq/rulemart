package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// canonicalGroupsSHA256 is the SHA-256 of canonical-groups.yaml in fabricahq/code-rules at v0.2.0, commit
// 6a63c7173bb5ae8b37cf22f30b8a4aede1d6b435. Change it only when copying the file from a newer commit.
const canonicalGroupsSHA256 = "dfdee35ba92d40a32c973309cf6941579bfdbf9fc2641e302d348aab7d4326f2"

// Code Rules owns the list, so Rulemart's copy must be its file at the pinned commit, never edited here: an alias or
// a renamed group added here would make Rulemart disagree with every other tool that reads the list.
func TestCanonicalGroupListIsCodeRulesFileAtThePinnedCommit(t *testing.T) {
	digest := sha256.Sum256(canonicalGroupsYAML)

	if got := hex.EncodeToString(digest[:]); got != canonicalGroupsSHA256 {
		t.Fatalf("canonical-groups.yaml has SHA-256 %s, not Code Rules' %s: copy the file from the pinned commit "+
			"again, or update the pin with it", got, canonicalGroupsSHA256)
	}
}

// The shipped list must parse with the vendored parser, or the web function couldn't start.
func TestCanonicalGroupsReadsTheShippedList(t *testing.T) {
	groups, err := CanonicalGroups()
	if err != nil {
		t.Fatal(err)
	}

	for id, name := range map[string]string{"techs/go": "Go", "practices/testing": "Testing", "techs/cpp": "C++"} {
		if group, ok := groups.Find(id); !ok || group.Name != name {
			t.Errorf("Find(%q) = %+v, %v; want %s", id, group, ok, name)
		}
	}
	if group, ok := groups.Find("techs/golang"); ok {
		t.Errorf("techs/golang, which isn't on the list, is canonical as %+v", group)
	}
}
