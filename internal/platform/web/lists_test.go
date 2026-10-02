package web_test

import (
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/platform/web"
)

// field is a form field as a test reads it: its name, value, and whether it's on.
type field struct {
	name, value string
	checked     bool
}

// filterFields returns the fields of the page's filters form, with its action and method, in order.
func filterFields(t *testing.T, page string) (action, method string, fields []field) {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	form := find(doc, func(n *html.Node) bool { return n.Data == "form" && attribute(n, "id") == "filters" })
	if form == nil {
		t.Fatal("the page has no filters form")
	}
	for n := range form.Descendants() {
		if n.Type == html.ElementNode && n.Data == "input" {
			_, checked := attributeOf(n, "checked")
			fields = append(fields, field{attribute(n, "name"), attribute(n, "value"), checked})
		}
	}
	return attribute(form, "action"), attribute(form, "method"), fields
}

// attributeOf returns n's attribute key, and whether it has it.
func attributeOf(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

// checkedFields returns the fields that are on or hidden, as name=value, which the form submits.
func checkedFields(fields []field) []string {
	var on []string
	for _, f := range fields {
		if f.checked {
			on = append(on, f.name+"="+f.value)
		}
	}
	return on
}

// fieldNames returns each field's name and value, as name=value.
func fieldNames(fields []field) []string {
	names := make([]string, len(fields))
	for i, f := range fields {
		names[i] = f.name + "=" + f.value
	}
	return names
}

// A group's page lists its rules in one ranked list beside the filters: its crumbs, its title with its icon, how many
// rules it holds and from how many libraries, and each rule's row, with its title, impact, library, and stars.
func TestGroupPageListsItsRulesBesideTheFilters(t *testing.T) {
	resp := get(t, newSite(t, newBrowsingCatalog()), "/g/techs/go")

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d", resp.Code)
	}
	page := resp.Body.String()
	assertShows(t, page,
		"Technologies › techs/go Go 3 rules from 2 libraries · The Go language.",
		"Libraries example/rules 1 other/go-rules 2 Impact Critical and high Medium and lower Stars Any 10+ 50+ 100+ "+
			"Retired Show retired rules Unvetted Include unvetted libraries",
		"3 rules in 2 libraries Most starred Newest",
		"Return errors with context HIGH example/rules 3 3 stars Close response bodies MEDIUM other/go-rules "+
			"Name packages plainly LOW other/go-rules",
	)
	for text, want := range map[string]string{
		"Technologies":               "/browse/techs",
		"Return errors with context": errorsRule,
		"Close response bodies":      "/other/go-rules/techs/go/close-bodies",
		"Newest":                     "/g/techs/go?sort=new",
	} {
		if got := links(t, page, text); !slices.Contains(got, want) {
			t.Errorf("%s links %q, want %s", text, got, want)
		}
	}
	if got := headings(t, page, "h3"); !slices.Equal(got, []string{"Return errors with context", "Close response bodies", "Name packages plainly"}) {
		t.Errorf("the rules are headed %q", got)
	}
	if strings.Contains(visibleText(t, page), "Kind") || strings.Contains(visibleText(t, page), "Clear filters") {
		t.Error("a group's page offers a kind, or clears filters it doesn't have")
	}
	if !strings.Contains(page, `<script src="/_static/`) || !strings.Contains(page, `/filters.js" defer></script>`) {
		t.Error("the page doesn't load the script that submits its filters")
	}
}

// The sidebar is a form that leads to the page with what it holds, so it works without scripts: each library, each
// impact band, each star threshold, retired rules, and unvetted libraries. Every control shows the choices the address
// holds, the sort tabs and Clear filters keep the rest, and the catalog reads them all.
func TestGroupPageControlsShowAndKeepTheAddresssChoices(t *testing.T) {
	var chosen []domain.ListChoices
	c := newBrowsingCatalog()
	c.chosen = &chosen
	handler := newSite(t, c)

	plain := get(t, handler, "/g/techs/go").Body.String()
	chosenPath := "/g/techs/go?impact=medium&libs=other%2Fgo-rules&retired=1&sort=new&stars=10&unvetted=1"
	page := get(t, handler, chosenPath).Body.String()

	action, method, fields := filterFields(t, plain)
	if action != "/g/techs/go" || method != "get" {
		t.Errorf("the form submits to %s with %s", action, method)
	}
	if want := []string{
		"libs=example/rules", "libs=other/go-rules", "impact=high", "impact=medium", "stars=", "stars=10", "stars=50",
		"stars=100", "retired=1", "unvetted=1",
	}; !slices.Equal(fieldNames(fields), want) {
		t.Errorf("the form's fields are %q, want %q", fieldNames(fields), want)
	}
	if got := checkedFields(fields); !slices.Equal(got, []string{"stars="}) {
		t.Errorf("by default, %q are on, want only any stars", got)
	}
	_, _, fields = filterFields(t, page)
	if want := []string{"sort=new", "libs=other/go-rules", "impact=medium", "stars=10", "retired=1", "unvetted=1"}; !slices.Equal(
		append([]string{fieldNames(fields)[0]}, checkedFields(fields)...), want) {
		t.Errorf("with choices, the form holds %q, want %q", fieldNames(fields), want)
	}
	if !strings.Contains(page, `type="hidden" name="sort" value="new"`) {
		t.Error("the form doesn't keep the order")
	}
	if got := links(t, page, "Clear filters"); !slices.Equal(got, []string{"/g/techs/go?retired=1&sort=new&unvetted=1"}) {
		t.Errorf("Clear filters leads to %q", got)
	}
	if got := links(t, page, "Most starred"); !slices.Equal(got, []string{"/g/techs/go?impact=medium&libs=other%2Fgo-rules&retired=1&stars=10&unvetted=1"}) {
		t.Errorf("Most starred leads to %q", got)
	}
	want := domain.ListChoices{Unvetted: true, Retired: true, Filters: domain.RuleFilters{
		Libraries: []string{"other/go-rules"}, Impact: domain.LowerImpact, MinStars: 10,
	}, Order: domain.Newest}
	if len(chosen) != 2 || !equalChoices(chosen[1], want) {
		t.Errorf("read the catalog with %+v, want %+v", chosen, want)
	}
	// A filter's two checkboxes turn each other off with a script, and without one, the form shows its Apply button.
	if strings.Count(plain, "data-exclusive") != 2 || !strings.Contains(plain, "<noscript>") || !strings.Contains(plain, "Apply filters") {
		t.Error("the impact checkboxes aren't exclusive, or the form has no Apply button without scripts")
	}
}

// equalChoices reports whether a and b are the same choices.
func equalChoices(a, b domain.ListChoices) bool {
	return a.Unvetted == b.Unvetted && a.Retired == b.Retired && a.Order == b.Order && a.Filters.Impact == b.Filters.Impact &&
		a.Filters.MinStars == b.Filters.MinStars && a.Filters.Kind == b.Filters.Kind && slices.Equal(a.Filters.Libraries, b.Filters.Libraries)
}

// A list's address has one spelling: defaults, choices the page doesn't offer, unknown parameters, repeated
// libraries, and another order of parameters redirect to it, as a form without a script submits them.
func TestListAddressesRedirectToTheirOwnSpelling(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())

	for path, location := range map[string]string{
		"/g/techs/go?stars=&sort=":                           "/g/techs/go",
		"/g/techs/go?sort=stars&stars=0":                     "/g/techs/go",
		"/g/techs/go?impact=high&impact=medium":              "/g/techs/go",
		"/g/techs/go?kind=techs&ref=x":                       "/g/techs/go",
		"/g/techs/go?libs=example/rules&libs=other/go-rules": "/g/techs/go?libs=example%2Frules%2Cother%2Fgo-rules",
		"/g/techs/go?stars=10&sort=new":                      "/g/techs/go?sort=new&stars=10",
		"/g/techs/GO?sort=new":                               "/g/techs/go?sort=new",
		"/search?q=errors&sort=best&kind=techs":              "/search?kind=techs&q=errors",
		"/search?q=":                                         "/search",
		"/search?retired=1&q=errors":                         "/search?q=errors",
		"/libraries?unvetted=0":                              "/libraries",
		"/libraries?unvetted=1&sort=new":                     "/libraries?unvetted=1",
		"/browse/techs?impact=high":                          "/browse/techs",
		"/browse/techs/other?x=1&unvetted=1":                 "/browse/techs/other?unvetted=1",
	} {
		resp := get(t, handler, path)
		if resp.Code != http.StatusMovedPermanently || resp.Header().Get("Location") != location {
			t.Errorf("%s: got %d to %q, want 301 to %q", path, resp.Code, resp.Header().Get("Location"), location)
		}
		if resp, hops := follow(t, handler, path); resp.Code != http.StatusOK || len(hops) > 2 {
			t.Errorf("%s: reached %d after %q", path, resp.Code, hops)
		}
	}
}

// A group's page with choices in its address asks search engines not to index it, and still names the group's own
// address; without choices, it's indexed.
func TestFilteredGroupPagesAreNotIndexedAndKeepTheirCanonicalAddress(t *testing.T) {
	base, err := web.ParseBaseURL("https://rulemart.example")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := web.New(newBrowsingCatalog(), web.Options{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), BaseURL: base})
	if err != nil {
		t.Fatal(err)
	}

	for path, noindex := range map[string]bool{
		"/g/techs/go":                    false,
		"/g/techs/go?sort=new":           true,
		"/g/techs/go?stars=10":           true,
		"/g/techs/go?unvetted=1":         true,
		"/g/techs/go?retired=1":          true,
		"/libraries":                     false,
		"/libraries?unvetted=1":          true,
		"/browse/techs?unvetted=1":       true,
		"/browse/techs/other?unvetted=1": true,
	} {
		page := get(t, handler, path).Body.String()
		content, _ := robots(t, page)
		if (content == "noindex") != noindex {
			t.Errorf("%s: robots %q, want noindex %v", path, content, noindex)
		}
		want := "https://rulemart.example" + strings.Split(path, "?")[0]
		if got := canonicalLinks(t, page); !slices.Equal(got, []string{want}) {
			t.Errorf("%s names %q, want %s", path, got, want)
		}
	}
}

// When no rule passes the filters, the page says so and leads to the list without them; a canonical group no library
// holds says that instead.
func TestGroupPageSaysWhenNoRulePassesTheFiltersOrNoLibraryHoldsIt(t *testing.T) {
	c := newBrowsingCatalog()
	filtered := c.groups["techs/go"]
	filtered.Rules.Rows, filtered.Rules.Total, filtered.Rules.Libraries = nil, 0, 0
	c.groups["techs/go"] = filtered
	handler := newSite(t, c)

	page := get(t, handler, "/g/techs/go?stars=100").Body.String()
	empty := get(t, handler, "/g/practices/accessibility")

	assertShows(t, page, "3 rules from 2 libraries", "0 rules in 0 libraries", "No rules match these filters. Clear filters")
	if got := links(t, page, "Clear filters"); !slices.Equal(got, []string{"/g/techs/go", "/g/techs/go"}) {
		t.Errorf("Clear filters leads to %q", got)
	}
	if empty.Code != http.StatusOK {
		t.Fatalf("a canonical group no library holds: got %d", empty.Code)
	}
	assertShows(t, empty.Body.String(), "Accessibility 0 rules from 0 libraries", "No library has Accessibility rules yet.")
}

// A group that isn't canonical has a page, which says it holds only the libraries that chose its exact ID; an ID no
// library holds has none.
func TestGroupPageOfAGroupThatIsntCanonicalSaysSo(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())

	resp := get(t, handler, "/g/techs/golang")

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d", resp.Code)
	}
	page := resp.Body.String()
	assertShows(t, page,
		"Technologies › Other groups › techs/golang techs/golang Not canonical",
		"techs/golang isn't a canonical group, so it only includes rules from libraries that chose this exact name.",
		"1 rule from 1 library", "Wrap errors MEDIUM other/go-rules",
	)
	if got := links(t, page, "Other groups"); !slices.Equal(got, []string{"/browse/techs/other"}) {
		t.Errorf("Other groups links %q", got)
	}
	if !strings.Contains(page, "<title>techs/golang rules · Rulemart</title>") {
		t.Error("the title doesn't name the group's ID")
	}
	for _, path := range []string{"/g/techs/zebra", "/g/techs", "/browse/techs/golang"} {
		if resp, hops := follow(t, handler, path); resp.Code != http.StatusNotFound {
			t.Errorf("%s: reached %d at %q", path, resp.Code, hops)
		}
	}
}

// Every list draws a retired rule grayed out, with the Retired chip and the rule that replaced it, and a rule of an
// unvetted library with the Unvetted chip and a nofollow link; Fabrica's rules show Fabrica's mark.
func TestRuleRowsMarkRetiredUnvettedAndFabricasRules(t *testing.T) {
	c := newBrowsingCatalog()
	retired := closeBodiesRow
	retired.Retired, retired.ReplacedBy = true, &views.RuleRef{Path: "techs/go/close-everything", Title: "Close everything"}
	unvetted := namePackagesRow
	unvetted.Library, unvetted.Vetted = views.LibraryRef{Owner: "stranger", Name: "rules"}, false
	fabrica := returnErrorsRow
	fabrica.Library = views.LibraryRef{Owner: "fabricahq", Name: "public-rules"}
	page := c.groups["techs/go"]
	page.Rules.Rows = []views.RuleRow{fabrica, retired, unvetted}
	c.groups["techs/go"] = page

	body := get(t, newSite(t, c), "/g/techs/go?retired=1&unvetted=1").Body.String()

	renamed := c.groups["techs/golang"]
	renamed.Rules.Rows = []views.RuleRow{retired}
	renamed.Rules.Rows[0].ReplacedBy = &views.RuleRef{Path: "techs/go/close-bodies-early", Title: "Close response bodies"}
	c.groups["techs/golang"] = renamed
	assertShows(t, get(t, newSite(t, c), "/g/techs/golang?retired=1").Body.String(),
		"Close response bodies MEDIUM Retired Renamed to techs/go/close-bodies-early other/go-rules")
	assertShows(t, body,
		"Close response bodies MEDIUM Retired Replaced by Close everything other/go-rules",
		"Name packages plainly LOW Unvetted S stranger/rules",
	)
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]*html.Node{}
	for n := range doc.Descendants() {
		if n.Type == html.ElementNode && n.Data == "a" && strings.Contains(attribute(n, "href"), "/techs/go/") {
			rows[attribute(n, "href")] = n
		}
	}
	retiredRow, unvettedRow, fabricaRow := rows["/other/go-rules/techs/go/close-bodies"], rows["/stranger/rules/techs/go/name-packages"], rows["/fabricahq/public-rules/techs/go/return-errors"]
	if retiredRow == nil || unvettedRow == nil || fabricaRow == nil {
		t.Fatalf("got rows %v", rows)
	}
	if _, ok := attributeOf(retiredRow, "data-retired"); !ok {
		t.Error("the retired rule's row isn't marked retired")
	}
	for _, row := range []*html.Node{unvettedRow, fabricaRow} {
		if _, ok := attributeOf(row, "data-retired"); ok {
			t.Errorf("%s is marked retired", attribute(row, "href"))
		}
	}
	if attribute(unvettedRow, "rel") != "nofollow" || attribute(fabricaRow, "rel") != "" {
		t.Errorf("rels are %q and %q, want nofollow on the unvetted rule's only", attribute(unvettedRow, "rel"), attribute(fabricaRow, "rel"))
	}
	if find(fabricaRow, func(n *html.Node) bool { return attribute(n, "title") == "Published by Fabrica" }) == nil {
		t.Error("Fabrica's rule doesn't show Fabrica's mark")
	}
}

// Search lists its rules under their groups, each heading leading to the group's page and counting its rules, the
// rules that hold every word first, and offers Kind besides a group's filters, and three orders.
func TestSearchPageGroupsItsRulesUnderTheirGroups(t *testing.T) {
	c := newBrowsingCatalog()
	partial := closeBodiesRow
	partial.Missing = []string{"handling"}
	results := c.results["errors"]
	results.Rows = append(slices.Clone(results.Rows), partial)
	results.Total, results.Complete = 3, 2
	c.results["errors handling"] = results
	handler := newSite(t, c)

	page := get(t, handler, "/search?q=errors").Body.String()
	words := get(t, handler, "/search?q=errors+handling").Body.String()

	assertShows(t, page,
		"Search Rules matching “errors”",
		"Libraries example/rules 1 other/go-rules 1 Kind Technologies Practices Impact",
		"2 rules in 2 libraries Best match Most starred Newest",
		"Go techs/go 3 Return errors with context HIGH example/rules",
		"techs/golang not canonical 1 Wrap errors MEDIUM other/go-rules",
	)
	if got := headings(t, page, "h2"); !slices.Equal(got, []string{"Go techs/go 3", "techs/golang not canonical 1"}) {
		t.Errorf("the groups are headed %q", got)
	}
	for text, want := range map[string]string{
		"Go techs/go":  "/g/techs/go",
		"techs/golang": "/g/techs/golang",
		"Most starred": "/search?q=errors&sort=stars",
		"Newest":       "/search?q=errors&sort=new",
		"Best match":   "/search?q=errors",
		"Wrap errors":  "/other/go-rules/techs/golang/wrap-errors",
	} {
		if got := links(t, page, text); !slices.Contains(got, want) {
			t.Errorf("%s links %q, want %s", text, got, want)
		}
	}
	_, _, fields := filterFields(t, page)
	if fieldNames(fields)[0] != "q=errors" || !slices.Contains(fieldNames(fields), "kind=practices") || slices.Contains(fieldNames(fields), "retired=1") {
		t.Errorf("the search form's fields are %q, want q, Kind, and no retired rules", fieldNames(fields))
	}
	assertShows(t, words, "3 rules in 2 libraries · 2 match every word",
		"Rules that match some of your words Go techs/go 3 Close response bodies MEDIUM Missing: handling other/go-rules")
	if unvetted := get(t, handler, "/search?q=errors&unvetted=1").Body.String(); !slices.Contains(links(t, unvetted, "Go techs/go"), "/g/techs/go?unvetted=1") {
		t.Errorf("with unvetted libraries, the group leads to %q", links(t, unvetted, "Go techs/go"))
	}
}

// Search without a query lists every rule; one that finds nothing says so, and one whose filters keep nothing leads
// back to the list without them.
func TestSearchPageListsEveryRuleAndSaysWhenNoneMatch(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())

	all := get(t, handler, "/search").Body.String()
	none := get(t, handler, "/search?q=nothing").Body.String()

	assertShows(t, all, "Search All rules", "4 rules in 2 libraries", "Go techs/go 3 Return errors with context")
	if !strings.Contains(all, "<title>All rules · Search · Rulemart</title>") {
		t.Error("the title doesn't say it lists every rule")
	}
	assertShows(t, none, "Rules matching “nothing”", "No rules match. Try a broader word, or tell us what you were looking for .")
	if got := links(t, none, "tell us what you were looking for"); !slices.Equal(got, []string{"/feedback"}) {
		t.Errorf("the empty search leads to %q", got)
	}
}

// Paging a search keeps its choices.
func TestSearchPagesKeepTheirChoices(t *testing.T) {
	c := newBrowsingCatalog()
	results := c.results["errors"]
	results.Total = 45
	c.results["errors"], c.results["errors page 2"] = results, results

	page := get(t, newSite(t, c), "/search?kind=techs&page=2&q=errors").Body.String()

	if got := links(t, page, "Next"); !slices.Equal(got, []string{"/search?kind=techs&page=3&q=errors"}) {
		t.Errorf("Next leads to %q", got)
	}
	if got := links(t, page, "Previous"); !slices.Equal(got, []string{"/search?kind=techs&q=errors"}) {
		t.Errorf("Previous leads to %q", got)
	}
	if got := links(t, page, "Newest"); !slices.Equal(got, []string{"/search?kind=techs&q=errors&sort=new"}) {
		t.Errorf("a sort tab leads to %q, want the first page", got)
	}
}

// The libraries page and the browse pages offer the unvetted libraries as a choice, which tags them; vetted libraries
// carry the check mark, there, on the home page, and on owner pages.
func TestListsOfLibrariesOfferUnvettedOnesAndMarkVettedOnes(t *testing.T) {
	c := newBrowsingCatalog()
	c.unvetted = []views.LibraryCard{{Owner: "stranger", Name: "rules", Description: "Stranger's rules.", Rules: 2}}
	c.unvettedIndex.Techs = append(slices.Clone(c.index.Techs), views.GroupSummary{
		Path: "techs/rust", Canonical: &views.CanonicalGroup{Name: "Rust"}, Rules: 2, Libraries: []views.LibraryRef{{Owner: "stranger", Name: "rules"}},
	})
	handler := newSite(t, c)

	plain := get(t, handler, "/libraries").Body.String()
	opted := get(t, handler, "/libraries?unvetted=1").Body.String()
	browse := get(t, handler, "/browse/techs?unvetted=1").Body.String()

	if !strings.Contains(plain, `name="unvetted" value="1"`) || strings.Contains(plain, `name="unvetted" value="1" checked`) {
		t.Error("the libraries page doesn't offer unvetted libraries, unchosen")
	}
	assertShows(t, plain, "rules Vetted by Rulemart Example rules for tests.")
	if strings.Contains(visibleText(t, plain), "stranger") {
		t.Error("the libraries page lists an unvetted library without the choice")
	}
	assertShows(t, opted, "Every library on Rulemart", "rules Unvetted Stranger's rules. stranger/rules")
	if !strings.Contains(opted, `name="unvetted" value="1" checked`) {
		t.Error("the choice doesn't show it's on")
	}
	if got := links(t, opted, "Stranger's rules."); len(got) != 1 || !strings.Contains(opted, `href="/stranger/rules" rel="nofollow"`) {
		t.Errorf("the unvetted library links %q without nofollow", got)
	}
	if strings.Count(opted, "data-vetted") != 1 {
		t.Error("only the vetted library should carry the check mark")
	}
	assertShows(t, browse, "Include unvetted libraries", "Rust Unvetted 2 rules 1 library")
	if got := links(t, browse, "Rust"); !slices.Equal(got, []string{"/g/techs/rust?unvetted=1"}) {
		t.Errorf("an unvetted group leads to %q", got)
	}
	for _, path := range []string{"/", "/example"} {
		if page := get(t, handler, path).Body.String(); !strings.Contains(page, "data-vetted") || !strings.Contains(page, `title="Vetted by Rulemart"`) {
			t.Errorf("%s doesn't mark the vetted library", path)
		}
	}
}
