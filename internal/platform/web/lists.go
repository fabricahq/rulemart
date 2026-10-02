// The pages that list rules across libraries, a group's and search, with the filter sidebar and the sort tabs, and the
// choices every list's address holds, the opt-in to unvetted libraries among them.

package web

import (
	"errors"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// ruleListView is what a page that lists rules shows around them: the filter sidebar and the results head with its sort
// tabs, and the addresses that change one choice and keep the rest.
type ruleListView struct {
	page domain.ListPage
	// path is the page's path, and params its own parameters, which every address of it keeps, such as search's q.
	path    string
	params  url.Values
	choices domain.ListChoices
	// libraries are the sidebar's libraries: those of the rules the list holds before its filters, Fabrica's first.
	libraries []libraryFilterView
	// total counts the rules that pass the filters, complete those of them that hold every word of a search, and
	// libraryCount the libraries they come from. unfiltered counts the rules the list holds before its filters.
	total, complete, libraryCount, unfiltered int
}

// libraryFilterView is a library's checkbox in the filter sidebar.
type libraryFilterView struct {
	// key is the library's value in the address, owner/name in lowercase.
	key, owner, fullName, avatar string
	// fabrica marks Fabrica's library, which the sidebar sets in stronger type, and vetted one the release vets.
	fabrica, vetted, checked bool
	// rules counts the library's rules in the list before its filters.
	rules int
}

func newRuleListView(page domain.ListPage, path string, params url.Values, choices domain.ListChoices, results views.RuleResults) ruleListView {
	v := ruleListView{
		page: page, path: path, params: params, choices: choices, total: results.Total, complete: results.Complete,
		libraryCount: results.Libraries, unfiltered: results.Unfiltered,
	}
	for _, l := range results.LibraryCounts {
		key := domain.LibraryKeyOf(l.Library.Owner, l.Library.Name)
		v.libraries = append(v.libraries, libraryFilterView{
			key: key, owner: l.Library.Owner, fullName: l.Library.FullName(), avatar: l.Library.OwnerAvatarURL,
			fabrica: strings.EqualFold(l.Library.Owner, domain.FabricaOwner), vetted: l.Vetted,
			checked: slices.Contains(choices.Filters.Libraries, key), rules: l.Rules,
		})
	}
	return v
}

// href is the page's address with choices in place of its own.
func (v ruleListView) href(choices domain.ListChoices) string {
	return addressOf(v.path, choices.Values(v.page), v.params)
}

// addressOf is path with the query that choices, the values ListChoices writes, and params, a page's own parameters,
// hold together, in the one spelling a page's own address has.
func addressOf(path string, choices, params url.Values) string {
	for name, values := range params {
		choices[name] = values
	}
	if len(choices) == 0 {
		return path
	}
	return path + "?" + choices.Encode()
}

// filtered reports whether a filter narrows the list, which Clear filters undoes.
func (v ruleListView) filtered() bool { return !v.choices.Filters.IsZero() }

// clearHref is the page's address without its filters, keeping its order and whether it includes unvetted libraries
// and retired rules.
func (v ruleListView) clearHref() string {
	choices := v.choices
	choices.Filters = domain.RuleFilters{}
	return v.href(choices)
}

// sortTab is one tab of the results head's sort control.
type sortTab struct {
	name, href string
	on         bool
}

// sortTabs are the page's orders, each leading to the page in that order with the same filters.
func (v ruleListView) sortTabs() []sortTab {
	var tabs []sortTab
	for _, order := range v.page.Orders() {
		choices := v.choices
		choices.Order = order
		tabs = append(tabs, sortTab{name: orderNames[order], href: v.href(choices), on: order == v.choices.Order})
	}
	return tabs
}

// orderNames name each order as its tab reads.
var orderNames = map[domain.RuleOrder]string{domain.BestMatch: "Best match", domain.MostStarred: "Most starred", domain.Newest: "Newest"}

// formParams are the hidden fields of the sidebar's form: the page's own parameters, and its order unless it's the
// default, so changing a filter keeps them. The form's own fields hold every other choice.
func (v ruleListView) formParams() []formParam {
	var params []formParam
	for _, name := range slices.Sorted(maps.Keys(v.params)) {
		for _, value := range v.params[name] {
			params = append(params, formParam{name, value})
		}
	}
	if order := v.choices.Values(v.page).Get("sort"); order != "" {
		params = append(params, formParam{"sort", order})
	}
	return params
}

// formParam is a hidden field of a form.
type formParam struct{ name, value string }

// minStars is the stars filter's value as its radio button holds it: empty for any.
func (v ruleListView) minStars() string {
	if v.choices.Filters.MinStars == 0 {
		return ""
	}
	return strconv.Itoa(v.choices.Filters.MinStars)
}

// withUnvetted returns href with the choice to include unvetted libraries when unvetted is true, so a link from a page
// that includes them leads to a page that does too.
func withUnvetted(href string, unvetted bool) string {
	if !unvetted {
		return href
	}
	return addressOf(href, domain.ListChoices{Unvetted: true}.Values(domain.LibraryListPage), nil)
}

// groupListView is what a group's page shows.
type groupListView struct {
	// href is the page's own address, without choices.
	href        string
	label       groupLabel
	icon        groupIcon
	description string
	list        ruleListView
	rows        []ruleRowView
}

func newGroupListView(page views.GroupList, choices domain.ListChoices, iconURL func(file string) string) groupListView {
	v := groupListView{
		href: groupHref(page.Path), label: newGroupLabel(page.Path, page.Canonical), icon: newGroupIcon(page.Canonical, iconURL),
	}
	if page.Canonical != nil {
		v.description = page.Canonical.Description
	}
	v.list = newRuleListView(domain.GroupListPage, v.href, nil, choices, page.Rules)
	for _, r := range page.Rules.Rows {
		v.rows = append(v.rows, newListedRuleRow(r))
	}
	return v
}

// truncated reports whether more rules pass the filters than the page lists.
func (v groupListView) truncated() bool { return v.list.total > len(v.rows) }

// title is the group's page's document title.
func (v groupListView) title() string { return v.label.display() + " rules · Rulemart" }

// summary describes the group to search engines: its description, or else what it holds.
func (v groupListView) summary() string {
	if v.description != "" {
		return v.description
	}
	return v.label.id + ": rules for coding agents from the Code Rules libraries that chose this group, on Rulemart."
}

// group shows the page of the group of kind that the path names, with the choices its address holds. It redirects
// another spelling of a canonical group's name, or of the choices, to the page's own address.
func (s *server) group(kind groupKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := string(kind) + "/" + r.PathValue("name")
		choices := domain.ParseListChoices(domain.GroupListPage, r.URL.Query())
		page, err := s.catalog.Group(r.Context(), id, choices)
		if errors.Is(err, app.ErrNotFound) {
			s.notFound(w, r)
			return
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
		view := newGroupListView(page, choices, s.assets.iconURL)
		if page.Path != id || !spelledAs(r, view.list.href(choices)) {
			redirect(w, r, view.list.href(choices))
			return
		}
		s.render(w, r, http.StatusOK, groupPage(s.pageChrome(view.href), view))
	}
}

// spelledAs reports whether r's address has the query of href, the page's own address.
func spelledAs(r *http.Request, href string) bool {
	_, query, _ := strings.Cut(href, "?")
	return r.URL.RawQuery == query
}

// listChoices reads the choices of page that r's address holds, and reports whether the address spells them as the
// page's own address at path does. Otherwise it redirects there and reports false.
func (s *server) listChoices(w http.ResponseWriter, r *http.Request, page domain.ListPage, path string) (domain.ListChoices, bool) {
	choices := domain.ParseListChoices(page, r.URL.Query())
	if own := addressOf(path, choices.Values(page), nil); !spelledAs(r, own) {
		redirect(w, r, own)
		return domain.ListChoices{}, false
	}
	return choices, true
}

// searchView is what the search page shows.
type searchView struct {
	// query is what the visitor searched for, cleaned; it's empty for every rule.
	query string
	// tooLong marks a query search didn't run, because it holds more than domain.MaxSearchQueryLength characters.
	tooLong bool
	// noWords marks a query with no word to find, such as only "the", which matches nothing.
	noWords bool
	// page numbers the page of results shown, from 1, of pages, which stops at app.MaxSearchPage, the last page a search
	// reads.
	page, pages int
	list        ruleListView
	// groups are the page's rules, under their groups, in the order of each group's first rule: first the rules that
	// hold every word of the query, then the rest, each among themselves.
	groups []resultGroupView
}

// resultGroupView is a group's rules on a page of search's results, under a heading that leads to the group's page.
type resultGroupView struct {
	label groupLabel
	icon  groupIcon
	href  string
	// rules counts the group's rules that pass the filters, on this page and others, among the rules that hold every
	// word of the query or the rest, as partial says.
	rules   int
	partial bool
	rows    []ruleRowView
}

func newSearchView(query domain.SearchQuery, choices domain.ListChoices, tooLong bool, results views.RuleResults, page int, iconURL func(file string) string) searchView {
	v := searchView{
		query: query.String(), tooLong: tooLong, noWords: results.NoWords, page: page,
		pages: min((results.Total+app.SearchPageSize-1)/app.SearchPageSize, app.MaxSearchPage),
		list:  newRuleListView(domain.SearchListPage, searchHref, searchParams(query.String()), choices, results),
	}
	for _, r := range results.Rows {
		partial := len(r.Missing) > 0
		if n := len(v.groups); n == 0 || v.groups[n-1].label.id != r.Rule.Group || v.groups[n-1].partial != partial {
			v.groups = append(v.groups, resultGroupView{
				label: newGroupLabel(r.Rule.Group, r.CanonicalGroup), icon: newGroupIcon(r.CanonicalGroup, iconURL),
				href: withUnvetted(groupHref(r.Rule.Group), choices.Unvetted), rules: r.GroupRules, partial: partial,
			})
		}
		last := &v.groups[len(v.groups)-1]
		last.rows = append(last.rows, newListedRuleRow(r))
	}
	return v
}

// searchParams are search's own parameters for query: q, unless it's empty.
func searchParams(query string) url.Values {
	if query == "" {
		return nil
	}
	return url.Values{"q": {query}}
}

// title is the search page's document title, which names the query.
func (v searchView) title() string {
	if v.query == "" {
		return "All rules · Search · Rulemart"
	}
	return "“" + v.query + "” · Search · Rulemart"
}

// heading is the search page's heading.
func (v searchView) heading() string {
	if v.query == "" {
		return "All rules"
	}
	return "Rules matching “" + v.query + "”"
}

// completeGroups returns the page's groups of rules that hold every word of the query, which come first.
func (v searchView) completeGroups() []resultGroupView { return v.groups[:v.partialStart()] }

// partialGroups returns the page's groups of rules that lack some of the query's words, which follow.
func (v searchView) partialGroups() []resultGroupView { return v.groups[v.partialStart():] }

// partialStart returns the index of the page's first group of rules that lack a word, or the number of groups.
func (v searchView) partialStart() int {
	for i, g := range v.groups {
		if g.partial {
			return i
		}
	}
	return len(v.groups)
}

// empty reports whether the page shows no rules.
func (v searchView) empty() bool { return len(v.groups) == 0 }

// pageMissing reports a page past the last of a search's results.
func (v searchView) pageMissing() bool { return v.page > 1 && v.empty() && !v.tooLong && !v.noWords }

// pageHref is the address of page n of the search's results, with the same choices.
func (v searchView) pageHref(n int) string { return searchHrefFor(v.query, v.list.choices, n) }

// searchHrefFor is the address of page of the search for query, or of every rule when it's empty, with choices,
// leaving out the first page's number.
func searchHrefFor(query string, choices domain.ListChoices, page int) string {
	params := searchParams(query)
	if page > 1 {
		if params == nil {
			params = url.Values{}
		}
		params.Set("page", strconv.Itoa(page))
	}
	return addressOf(searchHref, choices.Values(domain.SearchListPage), params)
}

// search shows a page of the rules that the query in the q parameter matches, or of every rule without one, the page
// the page parameter numbers, from 1, with the choices the address holds. Its page names no canonical address and asks
// search engines not to index it, since each query would otherwise be a page of its own. An address that spells its
// choices or page number another way, or names the first page's number, redirects to its own, and a page past the last
// is missing.
func (s *server) search(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	query := domain.ParseSearchQuery(params.Get("q"))
	choices := domain.ParseListChoices(domain.SearchListPage, params)
	page, spelled := searchPageNumber(params)
	own := searchHrefFor(params.Get("q"), choices, page)
	if page > app.MaxSearchPage {
		// A number too large to read keeps its spelling, so the page past the last says so rather than redirecting.
		pageParams := searchParams(params.Get("q"))
		if pageParams == nil {
			pageParams = url.Values{}
		}
		pageParams.Set("page", params.Get("page"))
		own = addressOf(searchHref, choices.Values(domain.SearchListPage), pageParams)
	}
	if !spelled || !spelledAs(r, own) {
		redirect(w, r, own)
		return
	}
	var results views.RuleResults
	var err error
	if page <= app.MaxSearchPage {
		results, err = s.catalog.SearchRules(r.Context(), query, choices, page)
	}
	tooLong := errors.Is(err, app.ErrSearchQueryTooLong)
	if err != nil && !tooLong {
		s.fail(w, r, err)
		return
	}
	view := newSearchView(query, choices, tooLong, results, page, s.assets.iconURL)
	status := http.StatusOK
	if view.pageMissing() {
		status = http.StatusNotFound
	}
	s.render(w, r, status, searchPage(s.chrome, view))
}
