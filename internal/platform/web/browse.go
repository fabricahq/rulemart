// Shape what the pages across libraries read, the groups and search, into what they show.

package web

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

const (
	// groupsHref is the path of the groups page.
	groupsHref = "/groups"
	// searchHref is the path of the search page, which takes the query in its q parameter.
	searchHref = "/search"
)

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

// groupHref is the path of a canonical group's page, such as /groups/techs/go.
func groupHref(id string) string {
	kind, name, _ := strings.Cut(id, "/")
	return groupsHref + "/" + url.PathEscape(kind) + "/" + url.PathEscape(name)
}

// kindName names a group's kind as the groups page heads it: Technologies or Practices.
func kindName(id string) string {
	if strings.HasPrefix(id, "practices/") {
		return "Practices"
	}
	return "Technologies"
}

// kindAnchor is the fragment of a kind's section on the groups page.
func kindAnchor(id string) string {
	return strings.ToLower(kindName(id))
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

// groupSummaryView is a group's row on the groups page, and its tile on the home page.
type groupSummaryView struct {
	label groupLabel
	icon  groupIcon
	// href is a canonical group's page, or for any other group, its section on its library's All rules tab.
	href string
	// blurb says which rules belong in a canonical practice, from the canonical list. Technology names explain
	// themselves, and a group that isn't canonical has no description every library shares, so neither has one.
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
		switch {
		case g.Canonical == nil:
			v.href = v.libraries[0].href + "?tab=rules#" + groupAnchor(g.Path)
		case strings.HasPrefix(g.Path, "practices/"):
			v.href, v.blurb = groupHref(g.Path), g.Canonical.Description
		default:
			v.href = groupHref(g.Path)
		}
		summaries[i] = v
	}
	return summaries
}

// empty reports whether no vetted library holds a group.
func (v groupIndexView) empty() bool { return len(v.techs) == 0 && len(v.practices) == 0 }

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

// groupPageView is what a canonical group's page shows.
type groupPageView struct {
	href        string
	label       groupLabel
	icon        groupIcon
	description string
	rules       int
	libraries   []groupLibraryView
}

// groupLibraryView is one library's rules on a group's page.
type groupLibraryView struct {
	library libraryRefView
	// section is the group's section on the library's All rules tab.
	section string
	rules   []ruleCard
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
	// page numbers the page of results shown, from 1, of pages.
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
		page: page, pages: (results.Total + app.SearchPageSize - 1) / app.SearchPageSize,
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

// pageMissing reports a page past the last of a search's results.
func (v searchView) pageMissing() bool { return v.page > 1 && len(v.results) == 0 }

// pageHref is the address of page n of the search's results.
func (v searchView) pageHref(n int) string { return searchHrefFor(v.query, n) }
