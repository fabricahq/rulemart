package domain

import (
	"testing"

	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

var canonicalList = []coderules.CanonicalGroup{
	{ID: "practices/testing", Name: "Testing", Description: "What to test."},
	{ID: "techs/go", Name: "Go", Description: "The Go language."},
}

func TestFindReturnsTheListsNameForACanonicalID(t *testing.T) {
	groups := NewCanonicalGroups(canonicalList)

	for id, name := range map[string]string{"techs/go": "Go", "practices/testing": "Testing"} {
		group, ok := groups.Find(id)
		if !ok || group.ID != id || group.Name != name {
			t.Errorf("Find(%q) = %+v, %v; want %s", id, group, ok, name)
		}
	}
}

// The list has no aliases: only the exact ID is canonical, so a library that names its group differently gets a
// group of its own.
func TestFindReportsNoGroupWhenTheIDIsntExactlyOnTheList(t *testing.T) {
	groups := NewCanonicalGroups(canonicalList)

	for _, id := range []string{
		"techs/golang",       // another name for the same technology
		"practices/go",       // the same name under the other kind
		"techs/g",            // a prefix
		"techs/go-modules",   // an extension
		"techs/go/",          // a trailing separator
		"Techs/Go",           // another case
		" techs/go",          // surrounding space
		"techs/go/return-it", // a rule's path in the group
		"",
	} {
		if group, ok := groups.Find(id); ok {
			t.Errorf("Find(%q) found %+v", id, group)
		}
	}
}

func TestFindReportsNoGroupWhenTheListIsEmpty(t *testing.T) {
	for name, groups := range map[string]CanonicalGroups{
		"the zero value": {},
		"an empty list":  NewCanonicalGroups(nil),
	} {
		if group, ok := groups.Find("techs/go"); ok {
			t.Errorf("%s: found %+v", name, group)
		}
	}
}
