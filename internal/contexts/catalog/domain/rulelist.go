// Say which rules a list of rules across libraries holds, a group's page or a search, and in what order.

package domain

import (
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// ImpactBand narrows a list of rules by the impact their libraries declare.
type ImpactBand string

const (
	// AnyImpact narrows nothing.
	AnyImpact ImpactBand = ""
	// HighImpact keeps rules of CRITICAL or HIGH impact.
	HighImpact ImpactBand = "high"
	// LowerImpact keeps every other rule: MEDIUM-HIGH and below, and a rule without a declared impact.
	LowerImpact ImpactBand = "medium"
)

// RuleOrder orders a list of rules.
type RuleOrder string

const (
	// BestMatch orders a search's rules by how well they match it, as search ranks them.
	BestMatch RuleOrder = "best"
	// MostStarred orders rules by their stars, most first.
	MostStarred RuleOrder = "stars"
	// Newest orders rules by when the library release that first published each one was tagged, newest first.
	Newest RuleOrder = "new"
)

// RuleFilters narrow a list of rules. The zero value narrows nothing.
type RuleFilters struct {
	// Libraries are the libraries to keep, each as LibraryFilterValue spells it, or none to keep every library.
	Libraries []string
	// Mine keeps the visitor's libraries, RuleList.MyLibraries, as their dashboard lists them; it keeps no rule while
	// the visitor has none.
	Mine   bool
	Impact ImpactBand
	// MinStars keeps rules with at least this many stars; 0 keeps every rule.
	MinStars int
	// Kind keeps the rules of one kind of group, techs or practices, or of both when empty.
	Kind string
}

// IsZero reports whether the filters keep every rule.
func (f RuleFilters) IsZero() bool {
	return len(f.Libraries) == 0 && !f.Mine && f.Impact == AnyImpact && f.MinStars == 0 && f.Kind == ""
}

// RuleList says which rules a list across libraries holds, before and after its filters, and in what order: a group's
// page, every rule, or a search's matches.
type RuleList struct {
	// Query is what a search finds; the zero query lists every rule.
	Query SearchQuery
	// Group is the ID of the one group whose rules the list holds, matched exactly, or empty for every group.
	Group string
	// MyLibraries are the libraries Filters.Mine keeps.
	MyLibraries MyLibraries
	ListChoices
}

// MyLibraries are a visitor's libraries, as their dashboard lists them: those whose owner is one of Owners, the
// visitor's own login and their organizations', and those Libraries name as owner/name, the libraries the visitor's
// projects use, each matched without regard to case.
type MyLibraries struct {
	Owners, Libraries []string
}

// HoldsRetired reports whether the list holds retired rules: when it asks for them, or searches for words.
func (l RuleList) HoldsRetired() bool { return l.Retired || !l.Query.IsZero() }

// LibraryFilterValue returns owner/name as RuleFilters.Libraries holds it, and the library filter's address: in
// lowercase, since libraries are matched without regard to case.
func LibraryFilterValue(owner, name string) string { return strings.ToLower(owner + "/" + name) }

// ListPage is a kind of page that lists rules or libraries, which decides the choices its address takes.
type ListPage int

const (
	// GroupListPage is a group's page: its libraries, the visitor's libraries, impact, stars, retired rules, unvetted
	// libraries, and Most starred or Newest.
	GroupListPage ListPage = iota
	// SearchListPage is search: a group's choices, plus the kind of group, and Best match, Most starred, or Newest. It
	// offers retired rules only while it lists every rule, since a search for words always finds them.
	SearchListPage
	// LibraryListPage is a list of libraries or groups, such as the libraries page or a browse page: only unvetted
	// libraries.
	LibraryListPage
)

// Orders returns the orders the page offers, its default first.
func (p ListPage) Orders() []RuleOrder {
	switch p {
	case GroupListPage:
		return []RuleOrder{MostStarred, Newest}
	case SearchListPage:
		return []RuleOrder{BestMatch, MostStarred, Newest}
	}
	return nil
}

// OffersKind reports whether the page filters by the kind of group, techs or practices, as search does; a group's
// page holds one kind.
func (p ListPage) OffersKind() bool { return p == SearchListPage }

// OffersRetired reports whether the page offers its retired rules as a choice, as a group's page and search do; search
// offers it only without a query in QueryParam.
func (p ListPage) OffersRetired() bool { return p == GroupListPage || p == SearchListPage }

// ListChoices are what a visitor chose on a page that lists rules or libraries: which libraries it includes, what it
// filters, and how it orders them. ParseListChoices reads them from an address, and Values writes them back.
type ListChoices struct {
	// Unvetted adds the libraries listings name that the release doesn't vet to the vetted ones.
	Unvetted bool
	// Retired adds retired rules to a list without a query, after every current rule; a search for words always finds
	// them, after its current rules too.
	Retired bool
	Filters RuleFilters
	// Order is one of the page's orders, its default when the address names none; empty on a LibraryListPage. Every
	// order puts a retired rule after the current rules it ties with.
	Order RuleOrder
}

// The query parameters that hold list choices, which ListChoices reads and writes, and a page's form fields name. A
// page's own address holds them only as Values writes them.
const (
	LibrariesParam = "libs"
	MineParam      = "mine"
	ImpactParam    = "impact"
	StarsParam     = "stars"
	KindParam      = "kind"
	SortParam      = "sort"
	UnvettedParam  = "unvetted"
	RetiredParam   = "retired"
)

// QueryParam holds what search finds, which isn't a choice of the list's, but decides whether search offers retired
// rules.
const QueryParam = "q"

// StarThresholds are the least stars the stars filter offers, besides any.
var StarThresholds = []int{10, 50, 100}

// MaxLibraryFilters bounds how many libraries an address may filter by, and so what a crafted one makes the database
// compare; the rest are left out.
const MaxLibraryFilters = 50

// ParseListChoices reads the choices of the page from values, an address's query. It reads only what the page
// offers, and leaves out anything else, or any value it doesn't offer, such as an order of another page's or stars it
// has no threshold for, or retired rules on search for words. A parameter may repeat, as a form without a script sends checkboxes, and libs may also join
// libraries with commas; a filter whose every value is chosen keeps every rule. Libraries are kept as
// LibraryFilterValue spells them, in order, each once.
func ParseListChoices(page ListPage, values map[string][]string) ListChoices {
	choices := ListChoices{Unvetted: has(values[UnvettedParam], "1")}
	if page == LibraryListPage {
		return choices
	}
	choices.Order = page.Orders()[0]
	for _, order := range page.Orders() {
		if has(values[SortParam], string(order)) {
			choices.Order = order
		}
	}
	choices.Filters.Libraries = libraryFilters(values[LibrariesParam])
	choices.Filters.Mine = has(values[MineParam], "1")
	choices.Filters.Impact = ImpactBand(oneOf(values[ImpactParam], string(HighImpact), string(LowerImpact)))
	for _, n := range StarThresholds {
		if has(values[StarsParam], strconv.Itoa(n)) {
			choices.Filters.MinStars = n
		}
	}
	if page.OffersKind() {
		choices.Filters.Kind = oneOf(values[KindParam], "techs", "practices")
	}
	if page.OffersRetired() && (page != SearchListPage || ParseSearchQuery(url.Values(values).Get(QueryParam)).IsZero()) {
		choices.Retired = RetiredChosen(values)
	}
	return choices
}

// RetiredChosen reports whether values, an address's query, chooses to show retired rules, wherever a page offers
// them: on a list of rules, or a library's All rules tab.
func RetiredChosen(values map[string][]string) bool { return has(values[RetiredParam], "1") }

// Values writes the choices as the page's address holds them: each in its parameter, leaving out every default, so
// the page's own address names none, and the libraries joined with commas.
func (c ListChoices) Values(page ListPage) url.Values {
	values := url.Values{}
	if c.Unvetted {
		values.Set(UnvettedParam, "1")
	}
	if c.Retired {
		values.Set(RetiredParam, "1")
	}
	if len(c.Filters.Libraries) > 0 {
		values.Set(LibrariesParam, strings.Join(c.Filters.Libraries, ","))
	}
	if c.Filters.Mine {
		values.Set(MineParam, "1")
	}
	if c.Filters.Impact != AnyImpact {
		values.Set(ImpactParam, string(c.Filters.Impact))
	}
	if c.Filters.MinStars > 0 {
		values.Set(StarsParam, strconv.Itoa(c.Filters.MinStars))
	}
	if c.Filters.Kind != "" {
		values.Set(KindParam, c.Filters.Kind)
	}
	if orders := page.Orders(); len(orders) > 0 && c.Order != orders[0] && c.Order != "" {
		values.Set(SortParam, string(c.Order))
	}
	return values
}

// has reports whether values holds value.
func has(values []string, value string) bool { return slices.Contains(values, value) }

// oneOf returns the one of a and b that values holds, or empty when they hold neither or both, since choosing both
// keeps every rule.
func oneOf(values []string, a, b string) string {
	switch hasA, hasB := has(values, a), has(values, b); {
	case hasA && !hasB:
		return a
	case hasB && !hasA:
		return b
	}
	return ""
}

// libraryFilters returns the libraries values name as owner/name, each value one or several joined with commas, as
// LibraryFilterValue spells them, each once, in order, leaving out what can't be a library's name and any past
// MaxLibraryFilters.
func libraryFilters(values []string) []string {
	var libraries []string
	for _, value := range values {
		for text := range strings.SplitSeq(value, ",") {
			owner, name, ok := strings.Cut(text, "/")
			if !ok || !libraryNamePart(owner) || !libraryNamePart(name) {
				continue
			}
			library := LibraryFilterValue(owner, name)
			if !slices.Contains(libraries, library) && len(libraries) < MaxLibraryFilters {
				libraries = append(libraries, library)
			}
		}
	}
	return libraries
}

// libraryNamePart reports whether text can be a library's owner or name on GitHub.
func libraryNamePart(text string) bool { return len(text) <= 100 && gitHubName.MatchString(text) }
