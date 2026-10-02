// The home page, and how it shapes what it reads into what it shows: the popular groups, each kind's tiles, the first
// libraries, and where List your library leads.

package web

import (
	"net/http"
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
		popular:   slices.Concat(firstOf(techs, popularPerKind), firstOf(practices, popularPerKind)),
		techs:     techs,
		practices: practices,
		libraries: firstOf(libraries, homeLibraries),
		listHref:  listHref,
	}
}

// firstOf returns at most n of items, the first ones. The result shares items' array but ends its capacity with
// them, so appending to it copies rather than writing into items.
func firstOf[T any](items []T, n int) []T {
	k := min(n, len(items))
	return items[:k:k]
}

// empty reports whether no vetted library holds a canonical group, so the home page has no tiles to show.
func (v homeView) empty() bool { return len(v.techs) == 0 && len(v.practices) == 0 }

func (s *server) home(w http.ResponseWriter, r *http.Request) {
	page, err := s.catalog.HomePage(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	cards, err := s.vettedCards(r, page.Libraries)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	view := newHomeView(cards, newGroupIndexView(page.Groups, s.assets.iconURL), s.listYourLibraryHref(r))
	s.render(w, r, http.StatusOK, homePage(s.pageChrome("/"), view))
}

// listYourLibraryHref returns where the home page's List your library leads: the page that lists a library for a
// signed-in visitor, sign-in returning to it for anyone else, and, where listing isn't available, how to get a
// library vetted.
func (s *server) listYourLibraryHref(r *http.Request) string {
	if !s.listingAvailable() {
		return aboutHref + "#get-vetted"
	}
	if visitorOf(r.Context()).account == nil {
		return s.absolute(signInPageHref(listHref))
	}
	return listHref
}
