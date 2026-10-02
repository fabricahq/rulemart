// Say which rules a list of rules across libraries holds, a group's page or a search, and in what order.

package domain

import (
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// FabricaOwner owns Fabrica's libraries, which lists put first among libraries otherwise equal, while stars are few.
const FabricaOwner = "fabricahq"

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
	Impact    ImpactBand
	// MinStars keeps rules with at least this many stars; 0 keeps every rule.
	MinStars int
	// Kind keeps the rules of one kind of group, techs or practices, or of both when empty.
	Kind string
}

// IsZero reports whether the filters keep every rule.
func (f RuleFilters) IsZero() bool {
	return len(f.Libraries) == 0 && f.Impact == AnyImpact && f.MinStars == 0 && f.Kind == ""
}

// RuleList says which rules a list across libraries holds, before and after its filters, and in what order: a group's
// page, every rule, or a search's matches.
type RuleList struct {
	// Query is what a search finds; the zero query lists every rule.
	Query SearchQuery
	// Group is the ID of the one group whose rules the list holds, matched exactly, or empty for every group.
	Group string
	ListChoices
}

// HoldsRetired reports whether the list holds retired rules: when it asks for them, or searches for words.
func (l RuleList) HoldsRetired() bool { return l.Retired || !l.Query.IsZero() }

// LibraryFilterValue returns owner/name as RuleFilters.Libraries holds it, and the library filter's address: in
// lowercase, since libraries are matched without regard to case.
func LibraryFilterValue(owner, name string) string { return strings.ToLower(owner + "/" + name) }

// ListPage is a kind of page that lists rules or libraries, which decides the choices its address takes.
type ListPage int

const (
	// GroupListPage is a group's page: its libraries, impact, stars, retired rules, unvetted libraries, and Most
	// starred or Newest.
	GroupListPage ListPage = iota
	// SearchListPage is search: a group's choices but retired rules, which search always finds, plus the kind of
	// group, and Best match, Most starred, or Newest.
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

// ListChoices are what a visitor chose on a page that lists rules or libraries: which libraries it includes, what it
// filters, and how it orders them. ParseListChoices reads them from an address, and Values writes them back.
type ListChoices struct {
	// Unvetted adds the libraries listings name that the release doesn't vet to the vetted ones.
	Unvetted bool
	// Retired adds retired rules to a list without a query; a search for words always finds them.
	Retired bool
	Filters RuleFilters
	// Order is one of the page's orders, its default when the address names none; empty on a LibraryListPage. Every
	// order puts a retired rule after the current rules it ties with.
	Order RuleOrder
}

// The query parameters that hold list choices.
const (
	librariesParam = "libs"
	impactParam    = "impact"
	starsParam     = "stars"
	kindParam      = "kind"
	sortParam      = "sort"
	unvettedParam  = "unvetted"
	retiredParam   = "retired"
)

// ListChoiceParams are the query parameters ListChoices reads and writes, which a page's address holds only as Values
// writes them.
var ListChoiceParams = []string{librariesParam, impactParam, starsParam, kindParam, sortParam, unvettedParam, retiredParam}

// StarThresholds are the least stars the stars filter offers, besides any.
var StarThresholds = []int{10, 50, 100}

// MaxLibraryFilters bounds how many libraries an address may filter by, and so what a crafted one makes the database
// compare; the rest are left out.
const MaxLibraryFilters = 50

// ParseListChoices reads the choices of the page from values, an address's query. It reads only what the page
// offers, and leaves out anything else, or any value it doesn't offer, such as an order of another page's or stars it
// has no threshold for. A parameter may repeat, as a form without a script sends checkboxes, and libs may also join
// libraries with commas; a filter whose every value is chosen keeps every rule. Libraries are kept as
// LibraryFilterValue spells them, in order, each once.
func ParseListChoices(page ListPage, values map[string][]string) ListChoices {
	choices := ListChoices{Unvetted: has(values[unvettedParam], "1")}
	if page == LibraryListPage {
		return choices
	}
	choices.Order = page.Orders()[0]
	for _, order := range page.Orders() {
		if has(values[sortParam], string(order)) {
			choices.Order = order
		}
	}
	choices.Filters.Libraries = libraryFilters(values[librariesParam])
	choices.Filters.Impact = ImpactBand(oneOf(values[impactParam], string(HighImpact), string(LowerImpact)))
	for _, n := range StarThresholds {
		if has(values[starsParam], strconv.Itoa(n)) {
			choices.Filters.MinStars = n
		}
	}
	if page == SearchListPage {
		choices.Filters.Kind = oneOf(values[kindParam], "techs", "practices")
	} else {
		choices.Retired = has(values[retiredParam], "1")
	}
	return choices
}

// Values writes the choices as the page's address holds them: each in its parameter, leaving out every default, so
// the page's own address names none, and the libraries joined with commas.
func (c ListChoices) Values(page ListPage) url.Values {
	values := url.Values{}
	if c.Unvetted {
		values.Set(unvettedParam, "1")
	}
	if c.Retired {
		values.Set(retiredParam, "1")
	}
	if len(c.Filters.Libraries) > 0 {
		values.Set(librariesParam, strings.Join(c.Filters.Libraries, ","))
	}
	if c.Filters.Impact != AnyImpact {
		values.Set(impactParam, string(c.Filters.Impact))
	}
	if c.Filters.MinStars > 0 {
		values.Set(starsParam, strconv.Itoa(c.Filters.MinStars))
	}
	if c.Filters.Kind != "" {
		values.Set(kindParam, c.Filters.Kind)
	}
	if orders := page.Orders(); len(orders) > 0 && c.Order != orders[0] && c.Order != "" {
		values.Set(sortParam, string(c.Order))
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
