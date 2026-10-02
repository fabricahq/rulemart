package web

import (
	"fmt"
	"slices"
	"testing"
)

// canonicalTile returns a canonical group's tile with rules rules, named by its ID.
func canonicalTile(id string, rules int) groupSummaryView {
	return groupSummaryView{label: groupLabel{id: id, name: id, canonical: true}, rules: rules}
}

// tileIDs returns the ID of each of groups, in order.
func tileIDs(groups []groupSummaryView) []string {
	ids := make([]string, len(groups))
	for i, g := range groups {
		ids[i] = g.label.id
	}
	return ids
}

// libraryCards returns n library cards, named library1 to libraryN.
func libraryCards(n int) []libraryCard {
	cards := make([]libraryCard, n)
	for i := range cards {
		cards[i] = libraryCard{owner: "example", name: fmt.Sprintf("library%d", i+1)}
	}
	return cards
}

// The hero names the two technologies and the two practices with the most rules, technologies first, and the tiles
// list each kind's canonical groups by rule count, keeping the catalog's order between groups with as many.
func TestHomeViewRanksGroupsByRuleCount(t *testing.T) {
	index := groupIndexView{
		techs: []groupSummaryView{
			canonicalTile("techs/go", 3), canonicalTile("techs/rust", 1), canonicalTile("techs/typescript", 7),
			canonicalTile("techs/python", 3), {label: groupLabel{id: "techs/golang"}, rules: 9},
		},
		practices: []groupSummaryView{
			canonicalTile("practices/testing", 1), canonicalTile("practices/comments", 4), canonicalTile("practices/error-handling", 2),
		},
	}

	v := newHomeView(nil, index, listHref)

	if want := []string{"techs/typescript", "techs/go", "practices/comments", "practices/error-handling"}; !slices.Equal(tileIDs(v.popular), want) {
		t.Errorf("popular = %q, want %q", tileIDs(v.popular), want)
	}
	if want := []string{"techs/typescript", "techs/go", "techs/python", "techs/rust"}; !slices.Equal(tileIDs(v.techs), want) {
		t.Errorf("techs = %q, want %q", tileIDs(v.techs), want)
	}
	if want := []string{"practices/comments", "practices/error-handling", "practices/testing"}; !slices.Equal(tileIDs(v.practices), want) {
		t.Errorf("practices = %q, want %q", tileIDs(v.practices), want)
	}
}

// The hero names as many groups of a kind as it has, up to two.
func TestHomeViewNamesAtMostTwoPopularGroupsOfEachKind(t *testing.T) {
	for groups, want := range map[int][]string{
		0: {},
		1: {"techs/1", "practices/1"},
		2: {"techs/1", "techs/2", "practices/1", "practices/2"},
		5: {"techs/1", "techs/2", "practices/1", "practices/2"},
	} {
		var index groupIndexView
		for i := range groups {
			index.techs = append(index.techs, canonicalTile(fmt.Sprintf("techs/%d", i+1), 10-i))
			index.practices = append(index.practices, canonicalTile(fmt.Sprintf("practices/%d", i+1), 10-i))
		}

		v := newHomeView(nil, index, listHref)

		if !slices.Equal(tileIDs(v.popular), want) {
			t.Errorf("with %d groups of each kind, popular = %q, want %q", groups, tileIDs(v.popular), want)
		}
	}
}

// The home page shows the first four libraries, in the order given, or all of them when there are fewer.
func TestHomeViewShowsAtMostTheFirstFourLibraries(t *testing.T) {
	for libraries, want := range map[int]int{0: 0, 3: 3, 4: 4, 5: 4} {
		cards := libraryCards(libraries)

		v := newHomeView(cards, groupIndexView{}, listHref)

		if !slices.Equal(v.libraries, cards[:want]) {
			t.Errorf("with %d libraries, the home page shows %v", libraries, v.libraries)
		}
	}
}

// The view's lists share no array with each other or with what it was given: naming the popular practices once wrote
// over the third and fourth technology tiles, and appending to a list that shares its input's array writes into it.
func TestHomeViewListsShareNoArrayWithTheirInputs(t *testing.T) {
	var index groupIndexView
	for i := range 9 {
		index.techs = append(index.techs, canonicalTile(fmt.Sprintf("techs/%d", i+1), 20-i))
	}
	index.practices = []groupSummaryView{canonicalTile("practices/testing", 30), canonicalTile("practices/comments", 29)}
	cards := libraryCards(5)

	v := newHomeView(cards, index, listHref)
	_ = append(v.libraries, libraryCard{name: "appended"})
	_ = append(v.popular, canonicalTile("techs/appended", 0))

	if !slices.Equal(tileIDs(v.techs), tileIDs(index.techs)) {
		t.Errorf("the technology tiles are %q, want every technology, %q", tileIDs(v.techs), tileIDs(index.techs))
	}
	if cards[4].name != "library5" {
		t.Errorf("appending to the home page's libraries changed the fifth library given to %q", cards[4].name)
	}
}
