// The filter sidebar and sort tabs of a page that lists rules across libraries, a group's or search, and the choices
// every list's address holds, the opt-in to unvetted libraries among them.

package web

import (
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

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

// libraryFilterView is a library's checkbox in the filter sidebar, which sets Fabrica's in stronger type.
type libraryFilterView struct {
	libraryRefView
	// value is the library's value in the address, owner/name in lowercase.
	value string
	// vetted marks a library the release vets, and checked one the filter keeps.
	vetted, checked bool
	// rules counts the library's rules in the list before its filters.
	rules int
}

func newRuleListView(page domain.ListPage, path string, params url.Values, choices domain.ListChoices, results views.RuleResults) ruleListView {
	v := ruleListView{
		page: page, path: path, params: params, choices: choices, total: results.Total, complete: results.Complete,
		libraryCount: results.Libraries, unfiltered: results.Unfiltered,
	}
	for _, l := range results.LibraryCounts {
		value := domain.LibraryFilterValue(l.Library.Owner, l.Library.Name)
		v.libraries = append(v.libraries, libraryFilterView{
			libraryRefView: newLibraryRefView(l.Library), value: value, vetted: l.Vetted,
			checked: slices.Contains(choices.Filters.Libraries, value), rules: l.Rules,
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
	if order := v.choices.Values(v.page).Get(domain.SortParam); order != "" {
		params = append(params, formParam{domain.SortParam, order})
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
