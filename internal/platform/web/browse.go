// The browse pages, and how the pages across libraries, the browse pages, a group's page, and search, shape what they
// read into what they show.

package web

import (
	"cmp"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

const (
	// librariesHref is the path of the libraries page.
	librariesHref = "/libraries"
	// browsePrefix starts the browse pages, one per kind of group, such as /browse/techs.
	browsePrefix = "/browse/"
	// groupPrefix starts a canonical group's page, such as /g/techs/go.
	groupPrefix = "/g/"
	// legacyGroupsHref is the groups page's old address, which redirects to the technologies' browse page, and under
	// which each group's old address redirects to its page.
	legacyGroupsHref = "/groups"
	// searchHref is the path of the search page, which takes the query in its q parameter.
	searchHref = "/search"
)

// groupKind is a kind of group, techs or practices, which the browse pages take as their address's segment.
type groupKind string

const (
	techsKind     groupKind = "techs"
	practicesKind groupKind = "practices"
)

// groupKinds are the kinds in the order pages list them: the browse pages' tabs, and a library's groups and rules.
var groupKinds = []groupKind{techsKind, practicesKind}

// parseGroupKind returns the kind that segment names, matched without regard to case, or false when it names none.
func parseGroupKind(segment string) (groupKind, bool) {
	for _, kind := range groupKinds {
		if strings.EqualFold(segment, string(kind)) {
			return kind, true
		}
	}
	return "", false
}

// kindOf returns the kind of the group or rule whose ID is id, such as techs/go.
func kindOf(id string) groupKind {
	if strings.HasPrefix(id, "practices/") {
		return practicesKind
	}
	return techsKind
}

// name names the kind as pages head it: Technologies or Practices.
func (k groupKind) name() string {
	if k == practicesKind {
		return "Practices"
	}
	return "Technologies"
}

// noun names one group of the kind, as in "other technology groups".
func (k groupKind) noun() string {
	if k == practicesKind {
		return "practice"
	}
	return "technology"
}

// href is the kind's browse page.
func (k groupKind) href() string { return browsePrefix + string(k) }

// othersHref is the page of the kind's groups that aren't canonical.
func (k groupKind) othersHref() string { return k.href() + "/other" }

// exampleGroup is a canonical group ID of the kind, which the other-groups page names to show what one looks like.
func (k groupKind) exampleGroup() string {
	if k == practicesKind {
		return "practices/testing"
	}
	return "techs/go"
}

// order is the kind's place in groupKinds, by which pages list technologies before practices.
func (k groupKind) order() int { return slices.Index(groupKinds, k) }

// searchHrefFor is the address of page of the search for query, which leaves out the first page's number.
func searchHrefFor(query string, page int) string {
	params := url.Values{}
	if query != "" {
		params.Set("q", query)
	}
	if page > 1 {
		params.Set("page", strconv.Itoa(page))
	}
	if len(params) == 0 {
		return searchHref
	}
	return searchHref + "?" + params.Encode()
}

// groupHref is the path of a canonical group's page, such as /g/techs/go.
func groupHref(id string) string {
	kind, name, _ := strings.Cut(id, "/")
	return groupPrefix + url.PathEscape(kind) + "/" + url.PathEscape(name)
}

// libraryRefView is a library named on a page about something else: a group, or a search.
type libraryRefView struct {
	href, owner, name, avatar string
}

func newLibraryRefView(lib views.LibraryRef) libraryRefView {
	return libraryRefView{href: libraryHref(lib.Owner, lib.Name), owner: lib.Owner, name: lib.Name, avatar: lib.OwnerAvatarURL}
}

// fullName returns the library's repository as owner/name.
func (l libraryRefView) fullName() string { return l.owner + "/" + l.name }

// groupSummaryView is a group's row on a browse page, and its tile on the home page.
type groupSummaryView struct {
	label groupLabel
	icon  groupIcon
	// href is a canonical group's page, or for any other group, its section on its library's All rules tab.
	href string
	// blurb says which rules belong in a canonical group, from the canonical list. A group that isn't canonical has
	// no description every library shares, so it has none.
	blurb string
	rules int
	// libraries hold the group, in owner and name order.
	libraries []libraryRefView
}

// groupIndexView is every group that holds current rules in a vetted library, by kind.
type groupIndexView struct {
	techs, practices []groupSummaryView
}

func newGroupIndexView(index views.GroupIndex, iconURL func(file string) string) groupIndexView {
	return groupIndexView{techs: newGroupSummaryViews(index.Techs, iconURL), practices: newGroupSummaryViews(index.Practices, iconURL)}
}

func newGroupSummaryViews(groups []views.GroupSummary, iconURL func(file string) string) []groupSummaryView {
	summaries := make([]groupSummaryView, len(groups))
	for i, g := range groups {
		v := groupSummaryView{label: newGroupLabel(g.Path, g.Canonical), icon: newGroupIcon(g.Canonical, iconURL), rules: g.Rules}
		for _, lib := range g.Libraries {
			v.libraries = append(v.libraries, newLibraryRefView(lib))
		}
		if g.Canonical == nil {
			v.href = v.libraries[0].href + "?tab=rules#" + groupAnchor(g.Path)
		} else {
			v.href, v.blurb = groupHref(g.Path), g.Canonical.Description
		}
		summaries[i] = v
	}
	return summaries
}

// empty reports whether no vetted library holds a group.
func (v groupIndexView) empty() bool { return len(v.techs) == 0 && len(v.practices) == 0 }

// ofKind returns the index's groups of kind, canonical ones first.
func (v groupIndexView) ofKind(kind groupKind) []groupSummaryView {
	if kind == practicesKind {
		return v.practices
	}
	return v.techs
}

// canonicalOnly returns the canonical groups of groups, which the home page shows as tiles.
func canonicalOnly(groups []groupSummaryView) []groupSummaryView {
	var canonical []groupSummaryView
	for _, g := range groups {
		if g.label.canonical {
			canonical = append(canonical, g)
		}
	}
	return canonical
}

// byRuleCount returns groups with the most rules first, and groups with as many by name, as the home page's tiles and
// the browse pages' rows list them.
func byRuleCount(groups []groupSummaryView) []groupSummaryView {
	sorted := slices.Clone(groups)
	slices.SortStableFunc(sorted, func(a, b groupSummaryView) int {
		return cmp.Or(cmp.Compare(b.rules, a.rules), strings.Compare(a.label.name, b.label.name))
	})
	return sorted
}

// othersOnly returns the groups of groups that aren't canonical, each one library's.
func othersOnly(groups []groupSummaryView) []groupSummaryView {
	var others []groupSummaryView
	for _, g := range groups {
		if !g.label.canonical {
			others = append(others, g)
		}
	}
	return others
}

// browseView is what a kind's browse page shows: its canonical groups that hold current rules in a vetted library,
// by rule count, and how many other groups its other page lists.
type browseView struct {
	kind   groupKind
	groups []groupSummaryView
	others int
}

func newBrowseView(kind groupKind, index groupIndexView) browseView {
	groups := index.ofKind(kind)
	return browseView{kind: kind, groups: byRuleCount(canonicalOnly(groups)), others: len(othersOnly(groups))}
}

// otherGroupsView is what a kind's other-groups page shows: one row per library and group ID that isn't canonical.
type otherGroupsView struct {
	kind   groupKind
	groups []groupSummaryView
}

func newOtherGroupsView(kind groupKind, index groupIndexView) otherGroupsView {
	return otherGroupsView{kind: kind, groups: othersOnly(index.ofKind(kind))}
}

// browse shows the browse page of the kind the path names, techs or practices, and is missing for any other.
func (s *server) browse(w http.ResponseWriter, r *http.Request) {
	kind, index, ok := s.browseIndex(w, r, groupKind.href)
	if !ok {
		return
	}
	s.render(w, r, http.StatusOK, browsePage(s.pageChrome(kind.href()), newBrowseView(kind, index)))
}

// otherGroups shows the groups of the kind the path names that aren't canonical.
func (s *server) otherGroups(w http.ResponseWriter, r *http.Request) {
	kind, index, ok := s.browseIndex(w, r, groupKind.othersHref)
	if !ok {
		return
	}
	s.render(w, r, http.StatusOK, otherGroupsPage(s.pageChrome(kind.othersHref()), newOtherGroupsView(kind, index)))
}

// browseIndex reads the groups a browse page shows, for the kind r's path names, and reports whether it did. It
// answers a kind that isn't one with the missing page, another spelling of the kind with a redirect to the page's
// address, which href gives, and a failed read with a failure.
func (s *server) browseIndex(w http.ResponseWriter, r *http.Request, href func(groupKind) string) (groupKind, groupIndexView, bool) {
	kind, ok := parseGroupKind(r.PathValue("kind"))
	if !ok {
		s.notFound(w, r)
		return "", groupIndexView{}, false
	}
	if r.PathValue("kind") != string(kind) {
		redirect(w, r, withQuery(href(kind), r))
		return "", groupIndexView{}, false
	}
	index, err := s.catalog.GroupIndex(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return "", groupIndexView{}, false
	}
	return kind, newGroupIndexView(index, s.assets.iconURL), true
}

// redirectToTechs redirects /browse, which names no kind, and the groups page's old address to the technologies'
// browse page, keeping the query.
func (s *server) redirectToTechs(w http.ResponseWriter, r *http.Request) {
	redirect(w, r, withQuery(techsKind.href(), r))
}

// legacyGroup redirects a group's old address, under the groups page's, to its page, keeping the query.
func (s *server) legacyGroup(w http.ResponseWriter, r *http.Request) {
	redirect(w, r, withQuery(groupHref(r.PathValue("kind")+"/"+r.PathValue("name")), r))
}

// groupPageView is what a canonical group's page shows.
type groupPageView struct {
	href        string
	label       groupLabel
	icon        groupIcon
	description string
	rules       int
	libraries   []groupLibraryView
	// notice is what the page says after adding, or signing in to add, one library's group, if any, and offer the
	// control its notice offers to add after signing in, or nil.
	notice string
	offer  *cartControl
}

// groupLibraryView is one library's rules on a group's page.
type groupLibraryView struct {
	library libraryRefView
	// section is the group's section on the library's All rules tab.
	section string
	rules   []ruleCard
	// cart is the control that adds the library's group to the cart.
	cart cartControl
}

func newGroupPageView(page views.GroupPage, iconURL func(file string) string) groupPageView {
	v := groupPageView{
		href: groupHref(page.Path), label: newGroupLabel(page.Path, &page.Canonical),
		icon: newGroupIcon(&page.Canonical, iconURL), description: page.Canonical.Description,
	}
	for _, lib := range page.Libraries {
		ref := newLibraryRefView(lib.Library)
		section := groupLibraryView{library: ref, section: ref.href + "?tab=rules#" + groupAnchor(page.Path)}
		for _, r := range lib.Rules {
			section.rules = append(section.rules, newRuleCard(ref.href, r))
		}
		v.rules += len(section.rules)
		v.libraries = append(v.libraries, section)
	}
	return v
}

// newRuleCard describes rule r of the library whose page is at libraryHref.
func newRuleCard(libraryHref string, r views.RuleCard) ruleCard {
	return ruleCard{href: libraryHref + "/" + r.Path, id: r.Path, title: r.Title, impact: r.Impact, version: r.Version.String()}
}

// searchView is what the search page shows.
type searchView struct {
	// query is what the visitor searched for, cleaned; it's empty before a search.
	query string
	// tooLong marks a query search didn't run, because it holds more than domain.MaxSearchQueryLength characters.
	tooLong bool
	// total counts every rule that matched, of which results holds the best, and complete those that hold every word
	// to find.
	total, complete int
	// noWords marks a query with no word to find, such as only "the", which matches nothing.
	noWords bool
	// page numbers the page of results shown, from 1, of pages, which stops at app.MaxSearchPage, the last page a search
	// reads.
	page, pages int
	results     []searchResultView
}

// searchResultView is one rule that matched a search.
type searchResultView struct {
	rule       ruleCard
	whenToRead string
	// sourceID is the rule's source-qualified ID, owner/name:rule ID, Code Rules' source:rule form with the library's
	// repository as its source.
	sourceID string
	library  libraryRefView
	group    groupLabel
	icon     groupIcon
	// missing holds the words to find, as the visitor wrote them, that the rule doesn't hold.
	missing []string
}

func newSearchView(query domain.SearchQuery, tooLong bool, results views.SearchResults, page int, iconURL func(file string) string) searchView {
	v := searchView{
		query: query.String(), tooLong: tooLong, total: results.Total, complete: results.Complete, noWords: results.NoWords,
		page: page, pages: min((results.Total+app.SearchPageSize-1)/app.SearchPageSize, app.MaxSearchPage),
	}
	for _, r := range results.Results {
		lib := newLibraryRefView(r.Library)
		v.results = append(v.results, searchResultView{
			rule: newRuleCard(lib.href, r.Rule), whenToRead: plainText(r.WhenToRead, r.WhenToReadHTML), sourceID: lib.fullName() + ":" + r.Rule.Path,
			library: lib, group: newGroupLabel(r.Rule.Group, r.CanonicalGroup), icon: newGroupIcon(r.CanonicalGroup, iconURL),
			missing: r.Missing,
		})
	}
	return v
}

// searched reports whether the page shows a search's outcome: results, none, or a query too long to run.
func (v searchView) searched() bool { return v.query != "" }

// title is the search page's document title, which names the query.
func (v searchView) title() string {
	if v.query == "" {
		return "Search · Rulemart"
	}
	return "“" + v.query + "” · Search · Rulemart"
}

// completeResults returns the page's results that hold every word to find, which search ranks first.
func (v searchView) completeResults() []searchResultView {
	return v.results[:v.partialStart()]
}

// partialResults returns the page's results that lack some of the words to find, which follow those that hold them all.
func (v searchView) partialResults() []searchResultView {
	return v.results[v.partialStart():]
}

// partialStart returns the index of the page's first result that lacks a word, or the number of results when none does.
func (v searchView) partialStart() int {
	for i, r := range v.results {
		if len(r.missing) > 0 {
			return i
		}
	}
	return len(v.results)
}

// pageMissing reports a page past the last of a search's results.
func (v searchView) pageMissing() bool { return v.page > 1 && len(v.results) == 0 }

// pageHref is the address of page n of the search's results.
func (v searchView) pageHref(n int) string { return searchHrefFor(v.query, n) }
