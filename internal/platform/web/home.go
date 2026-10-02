// Shape what the home page reads into what it shows: the popular groups, each kind's tiles, and the first libraries.

package web

import (
	"cmp"
	"slices"
)

// homeLibraries is how many libraries the home page shows before leading to them all.
const homeLibraries = 4

// popularPerKind is how many groups of each kind the hero names as popular.
const popularPerKind = 2

// homeView is what the home page shows.
type homeView struct {
	// popular are the groups the hero names: the technologies and practices with the most rules, since Rulemart has no
	// traffic data yet.
	popular []groupSummaryView
	// techs and practices are each kind's canonical groups, as tiles, by rule count.
	techs, practices []groupSummaryView
	// libraries are the first vetted libraries, in owner and name order.
	libraries []libraryCard
	// listHref is where List your library leads: the page that lists a library, signing in first when the visitor
	// isn't, or how to get a library vetted when listing isn't available.
	listHref string
}

func newHomeView(libraries []libraryCard, index groupIndexView, listHref string) homeView {
	techs, practices := byRuleCount(canonicalOnly(index.techs)), byRuleCount(canonicalOnly(index.practices))
	return homeView{
		popular:   append(firstOf(techs, popularPerKind), firstOf(practices, popularPerKind)...),
		techs:     techs,
		practices: practices,
		libraries: firstOf(libraries, homeLibraries),
		listHref:  listHref,
	}
}

// byRuleCount returns groups with the most rules first, groups with as many in the order given.
func byRuleCount(groups []groupSummaryView) []groupSummaryView {
	sorted := slices.Clone(groups)
	slices.SortStableFunc(sorted, func(a, b groupSummaryView) int { return cmp.Compare(b.rules, a.rules) })
	return sorted
}

// firstOf returns at most n of items, the first ones.
func firstOf[T any](items []T, n int) []T {
	return items[:min(n, len(items))]
}

// empty reports whether no vetted library holds a canonical group, so the home page has no tiles to show.
func (v homeView) empty() bool { return len(v.techs) == 0 && len(v.practices) == 0 }
