package domain

import (
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

var canonicalList = []coderules.CanonicalGroup{
	{ID: "practices/testing", Name: "Testing", Description: "What to test."},
	{ID: "techs/go", Name: "Go", Description: "The Go language."},
}

var goIcon = GroupIcon{File: "devicon/go-original.svg"}

// newCanonicalGroups returns canonicalList, with an icon for techs/go only.
func newCanonicalGroups(t *testing.T) CanonicalGroups {
	t.Helper()
	groups, err := NewCanonicalGroups(canonicalList, map[string]GroupIcon{"techs/go": goIcon})
	if err != nil {
		t.Fatal(err)
	}
	return groups
}

func TestFindReturnsTheListsNameAndAnyIconForACanonicalID(t *testing.T) {
	groups := newCanonicalGroups(t)

	for id, want := range map[string]CanonicalGroup{
		"techs/go":          {ID: "techs/go", Name: "Go", Description: "The Go language.", Icon: goIcon},
		"practices/testing": {ID: "practices/testing", Name: "Testing", Description: "What to test."}, // canonical, without an icon
	} {
		if group, ok := groups.Find(id); !ok || group != want {
			t.Errorf("Find(%q) = %+v, %v; want %+v", id, group, ok, want)
		}
	}
}

// Cross-library reads, such as search, pass the whole list as a parameter, in an order that doesn't depend on the
// map that holds it.
func TestAllReturnsEveryCanonicalGroupInIDOrder(t *testing.T) {
	groups := newCanonicalGroups(t)

	got := groups.All()

	want := []CanonicalGroup{
		{ID: "practices/testing", Name: "Testing", Description: "What to test."},
		{ID: "techs/go", Name: "Go", Description: "The Go language.", Icon: goIcon},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if all := (CanonicalGroups{}).All(); len(all) != 0 {
		t.Fatalf("the zero value holds %+v", all)
	}
}

// An icon may only mark a group as one every library shares, so one for a group off the list is a mistake in the
// icons, not a new group.
func TestNewCanonicalGroupsRejectsAnIconForAGroupNotOnTheList(t *testing.T) {
	_, err := NewCanonicalGroups(canonicalList, map[string]GroupIcon{"techs/go": goIcon, "techs/golang": goIcon})

	if err == nil || !strings.Contains(err.Error(), "techs/golang") {
		t.Fatalf("got %v; want an error naming techs/golang", err)
	}
}

// The list has no aliases: only the exact ID is canonical, so a library that names its group differently gets a
// group of its own.
func TestFindReportsNoGroupWhenTheIDIsntExactlyOnTheList(t *testing.T) {
	groups := newCanonicalGroups(t)

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
	empty, err := NewCanonicalGroups(nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	for name, groups := range map[string]CanonicalGroups{"the zero value": {}, "an empty list": empty} {
		if group, ok := groups.Find("techs/go"); ok {
			t.Errorf("%s: found %+v", name, group)
		}
	}
}
