// The browse pages, and how the pages across libraries, the browse pages, a group's page, and search, shape what they
// read into what they show.

package web

import (
	"cmp"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

const (
	// librariesHref is the path of the libraries page.
	librariesHref = "/libraries"
	// browsePrefix starts the browse pages, one per kind of group, such as /browse/techs.
	browsePrefix = "/browse/"
	// groupPrefix starts a group's page, such as /g/techs/go.
	groupPrefix = "/g/"
	// legacyGroupsHref is the groups page's old address, which redirects to the technologies' browse page, and under
	// which each group's old address redirects to its page.
	legacyGroupsHref = "/groups"
	// searchHref is the path of the search page, which takes the query in its q parameter.
	searchHref = "/search"
)

// groupKind is a kind of group, techs or practices, which the browse pages and a group's page take as their address's
// second segment.
type groupKind string

const (
	techsKind     groupKind = "techs"
	practicesKind groupKind = "practices"
)

// groupKinds are the kinds in the order pages list them: the browse pages' tabs, and a library's groups and rules.
var groupKinds = []groupKind{techsKind, practicesKind}

// parseGroupKind returns the kind that segment names, matched without regard to case, or false when it names none,
// which siteSpelling redirects to the kind's own spelling.
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

// groupHref is the path of a group's page, such as /g/techs/go.
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
	// href is the group's page, including unvetted libraries when the list it's in does.
	href string
	// blurb says which rules belong in a canonical group, from the canonical list. A group that isn't canonical has
	// no description every library shares, so it has none.
	blurb string
	rules int
	// libraries hold the group, in owner and name order.
	libraries []libraryRefView
	// unvetted marks a group only unvetted libraries hold, in a list that includes them, which its row tags.
	unvetted bool
}

// groupIndexView is every group that holds current rules in a vetted library, by kind.
type groupIndexView struct {
	techs, practices []groupSummaryView
}

// newGroupIndexView describes index, whose groups' links include unvetted libraries when unvetted is true, as the
// index does. iconURL returns where the site serves an icon file.
func newGroupIndexView(index views.GroupIndex, unvetted bool, iconURL func(file string) string) groupIndexView {
	return groupIndexView{
		techs: newGroupSummaryViews(index.Techs, unvetted, iconURL), practices: newGroupSummaryViews(index.Practices, unvetted, iconURL),
	}
}

func newGroupSummaryViews(groups []views.GroupSummary, unvetted bool, iconURL func(file string) string) []groupSummaryView {
	summaries := make([]groupSummaryView, len(groups))
	for i, g := range groups {
		v := groupSummaryView{
			label: newGroupLabel(g.Path, g.Canonical), icon: newGroupIcon(g.Canonical, iconURL), rules: g.Rules,
			href: withUnvetted(groupHref(g.Path), unvetted), unvetted: !g.Vetted,
		}
		for _, lib := range g.Libraries {
			v.libraries = append(v.libraries, newLibraryRefView(lib))
		}
		if g.Canonical != nil {
			v.blurb = g.Canonical.Description
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

// browseView is what a kind's browse page shows: its canonical groups that hold current rules in a vetted library, or
// with unvetted, in a library a listing names too, by rule count, and how many other groups its other page lists.
type browseView struct {
	kind     groupKind
	groups   []groupSummaryView
	others   int
	unvetted bool
}

// described reports whether g's row shows its description: a practice's says which rules belong in it, while a
// technology's name says what it is.
func (v browseView) described(g groupSummaryView) bool {
	return v.kind == practicesKind && g.blurb != ""
}

func newBrowseView(kind groupKind, index groupIndexView, unvetted bool) browseView {
	groups := index.ofKind(kind)
	return browseView{kind: kind, groups: byRuleCount(canonicalOnly(groups)), others: len(othersOnly(groups)), unvetted: unvetted}
}

// othersHref is the kind's other-groups page, including unvetted libraries when this page does.
func (v browseView) othersHref() string { return withUnvetted(v.kind.othersHref(), v.unvetted) }

// otherGroupsView is what a kind's other-groups page shows: one row per group ID that isn't canonical.
type otherGroupsView struct {
	kind     groupKind
	groups   []groupSummaryView
	unvetted bool
}

func newOtherGroupsView(kind groupKind, index groupIndexView, unvetted bool) otherGroupsView {
	return otherGroupsView{kind: kind, groups: othersOnly(index.ofKind(kind)), unvetted: unvetted}
}

// libraryNames names the libraries that hold g, a group that isn't canonical, as its row lists them.
func libraryNames(g groupSummaryView) string {
	names := make([]string, len(g.libraries))
	for i, lib := range g.libraries {
		names[i] = lib.fullName()
	}
	return strings.Join(names, ", ")
}

// browse shows the browse page of kind, including unvetted libraries when its address asks.
func (s *server) browse(kind groupKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		index, unvetted, ok := s.browseIndex(w, r, kind.href())
		if !ok {
			return
		}
		s.render(w, r, http.StatusOK, browsePage(s.pageChrome(kind.href()), newBrowseView(kind, index, unvetted)))
	}
}

// otherGroups shows the groups of kind that aren't canonical, including unvetted libraries when its address asks.
func (s *server) otherGroups(kind groupKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		index, unvetted, ok := s.browseIndex(w, r, kind.othersHref())
		if !ok {
			return
		}
		s.render(w, r, http.StatusOK, otherGroupsPage(s.pageChrome(kind.othersHref()), newOtherGroupsView(kind, index, unvetted)))
	}
}

// browseIndex reads the groups the browse page at path shows, including unvetted libraries when its address asks, and
// reports whether it did. It answers an address that spells the choice another way with a redirect, and a failed read
// with a failure.
func (s *server) browseIndex(w http.ResponseWriter, r *http.Request, path string) (groupIndexView, bool, bool) {
	choices, ok := s.listChoices(w, r, domain.LibraryListPage, path)
	if !ok {
		return groupIndexView{}, false, false
	}
	index, err := s.catalog.GroupIndex(r.Context(), choices.Unvetted)
	if err != nil {
		s.fail(w, r, err)
		return groupIndexView{}, false, false
	}
	return newGroupIndexView(index, choices.Unvetted, s.assets.iconURL), choices.Unvetted, true
}

// redirectToTechs redirects /browse, which names no kind, and the groups page's old address to the technologies'
// browse page, keeping the query.
func (s *server) redirectToTechs(w http.ResponseWriter, r *http.Request) {
	redirect(w, r, withQuery(techsKind.href(), r))
}

// legacyGroup redirects the old address of a group of kind, under the groups page's, to its page, keeping the query.
func (s *server) legacyGroup(kind groupKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		redirect(w, r, withQuery(groupHref(string(kind)+"/"+r.PathValue("name")), r))
	}
}
