package web_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"

	accounts "github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
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

// After the browser's Back, every list's form shows the choices of the address it returns to, not the ones the visitor
// last made there, which a browser would otherwise restore into the form's controls.
func TestListFormsShowOnlyTheAddresssChoicesAfterHistoryNavigation(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())

	for _, path := range []string{"/g/techs/go?impact=high", "/search?q=errors", "/libraries", "/browse/techs"} {
		doc, err := html.Parse(strings.NewReader(get(t, handler, path).Body.String()))
		if err != nil {
			t.Fatal(err)
		}
		forms := 0
		for n := range doc.Descendants() {
			if n.Type != html.ElementNode || n.Data != "form" {
				continue
			}
			if _, ok := attributeOf(n, "data-filters"); !ok {
				continue
			}
			forms++
			if attribute(n, "autocomplete") != "off" {
				t.Errorf("%s: a list's form lets the browser restore its controls", path)
			}
		}
		if forms == 0 {
			t.Errorf("%s: no list form", path)
		}
	}
}

// equalChoices reports whether a and b are the same choices.
func equalChoices(a, b domain.ListChoices) bool {
	return a.Unvetted == b.Unvetted && a.Retired == b.Retired && a.Order == b.Order && a.Filters.Impact == b.Filters.Impact &&
		a.Filters.MinStars == b.Filters.MinStars && a.Filters.Kind == b.Filters.Kind && a.Filters.Mine == b.Filters.Mine &&
		slices.Equal(a.Filters.Libraries, b.Filters.Libraries)
}

// A list's address has one spelling, and any other, such as a form submitted without a script, redirects to it in one
// step: on each kind of page that lists rules or libraries, and for a group's ID in another case. ParseListChoices' own
// test covers how each spelling reads.
func TestListAddressesRedirectToTheirOwnSpelling(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())

	for path, location := range map[string]string{
		"/g/techs/go?libs=example/rules&libs=other/go-rules": "/g/techs/go?libs=example%2Frules%2Cother%2Fgo-rules",
		"/g/techs/GO?sort=new":                               "/g/techs/go?sort=new",
		"/search?q=errors&sort=best&kind=techs":              "/search?kind=techs&q=errors",
		"/libraries?unvetted=1&sort=new":                     "/libraries?unvetted=1",
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

// On a phone the sidebar folds into a disclosure named Filters, counting the choices that are on, closed unless a
// filter is on, so the results start near the top.
func TestFilterSidebarFoldsIntoADisclosureThatOpensWithChoices(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())

	for path, want := range map[string]struct {
		summary string
		open    bool
	}{
		"/g/techs/go":                       {"Filters", false},
		"/g/techs/go?impact=high&retired=1": {"Filters · 2", true},
		"/g/techs/go?retired=1&unvetted=1":  {"Filters · 2", false},
		"/search?libs=example%2Frules":      {"Filters · 1", true},
	} {
		doc, err := html.Parse(strings.NewReader(get(t, handler, path).Body.String()))
		if err != nil {
			t.Fatal(err)
		}
		disclosure := find(doc, func(n *html.Node) bool {
			return n.Data == "details" && find(n, func(c *html.Node) bool { return attribute(c, "id") == "filters" }) != nil
		})
		if disclosure == nil {
			t.Fatalf("%s: the filters aren't in a disclosure", path)
		}
		summary := find(disclosure, func(n *html.Node) bool { return n.Data == "summary" })
		if summary == nil {
			t.Fatalf("%s: the disclosure has no summary", path)
		}
		_, open := attributeOf(disclosure, "open")
		if nodeText(summary) != want.summary || open != want.open {
			t.Errorf("%s: got a disclosure open %t, summed up %q; want open %t, %q", path, open, nodeText(summary), want.open, want.summary)
		}
	}
}

// The sidebar says what an unvetted library is under its choice, and leads to what vetting means.
func TestFilterSidebarExplainsUnvettedLibraries(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())

	for _, path := range []string{"/g/techs/go", "/search"} {
		page := get(t, handler, path).Body.String()
		assertShows(t, page, "Include unvetted libraries Libraries anyone listed that Rulemart hasn't reviewed")
		if got := links(t, page, "Libraries anyone listed that Rulemart hasn't reviewed"); !slices.Equal(got, []string{"/about#vetting"}) {
			t.Errorf("%s: the hint leads to %q", path, got)
		}
	}
}

// The sidebar names each library by its repository, beside its owner's avatar, and gives its full name on hover and
// to screen readers.
func TestFilterSidebarNamesLibrariesByRepository(t *testing.T) {
	page := get(t, newSite(t, newBrowsingCatalog()), "/g/techs/go").Body.String()

	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	form := find(doc, func(n *html.Node) bool { return attribute(n, "id") == "filters" })
	var names, titles []string
	for n := range form.Descendants() {
		if n.Type == html.ElementNode && n.Data == "label" && find(n, func(c *html.Node) bool { return attribute(c, "name") == "libs" }) != nil {
			names = append(names, nodeText(n))
			if named := find(n, func(c *html.Node) bool {
				return c.Data == "span" && attribute(c, "title") != "" && attribute(c, "title") != "Vetted by Rulemart"
			}); named != nil {
				titles = append(titles, attribute(named, "title"))
			}
		}
	}
	// nodeText separates the owner, said only to screen readers, from the name, which the markup joins.
	if want := []string{"example/ rules 1", "other/ go-rules 2"}; !slices.Equal(names, want) {
		t.Errorf("the sidebar's libraries read %q to screen readers, want %q", names, want)
	}
	if want := []string{"example/rules", "other/go-rules"}; !slices.Equal(titles, want) {
		t.Errorf("the sidebar's libraries are titled %q, want %q", titles, want)
	}
	if !strings.Contains(page, `<span class="sr-only">example/</span>rules`) {
		t.Error("the sidebar shows more than the repository's name")
	}
}

// The sidebar offers retired rules only on a list that has some, or that shows them already, so it can stop.
func TestFilterSidebarOffersRetiredRulesOnlyWhenTheListHasSome(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())

	for path, want := range map[string]bool{
		"/g/techs/go":               true,
		"/g/techs/golang":           false,
		"/g/techs/golang?retired=1": true,
	} {
		_, _, fields := filterFields(t, get(t, handler, path).Body.String())
		if got := slices.Contains(fieldNames(fields), "retired=1"); got != want {
			t.Errorf("%s: offers retired rules %t, want %t", path, got, want)
		}
	}
}

// Signed out, the Libraries filter has no My libraries, and an address that asks for it shows the whole list, which
// the catalog reads without it, and isn't indexed, as any address with choices.
func TestMyLibrariesIsOfferedOnlyToASignedInVisitor(t *testing.T) {
	var chosen []domain.ListChoices
	var mine []domain.MyLibraries
	c := newBrowsingCatalog()
	c.chosen, c.mine = &chosen, &mine
	handler := newSite(t, c)

	for path, rows := range map[string]string{
		"/g/techs/go?mine=1":      "Return errors with context HIGH example/rules 3 3 stars Close response bodies MEDIUM other/go-rules",
		"/search?mine=1&q=errors": "techs/golang not canonical 1 Wrap errors MEDIUM other/go-rules",
		"/search?mine=1":          "Return errors with context HIGH example/rules 3 3 stars Close response bodies MEDIUM other/go-rules",
	} {
		resp := get(t, handler, path)
		if resp.Code != http.StatusOK {
			t.Fatalf("%s: got %d", path, resp.Code)
		}
		page := resp.Body.String()
		assertShows(t, page, "Filters Libraries example/ rules 1 other/ go-rules", rows)
		if _, _, fields := filterFields(t, page); slices.Contains(fieldNames(fields), "mine=1") {
			t.Errorf("%s offers My libraries signed out", path)
		}
		if strings.Contains(visibleText(t, page), "Clear filters") {
			t.Errorf("%s clears filters it doesn't show", path)
		}
		if content, _ := robots(t, page); content != "noindex" {
			t.Errorf("%s: robots %q, want noindex", path, content)
		}
	}
	for i, choices := range chosen {
		if choices.Filters.Mine || mine[i].Owners != nil || mine[i].Libraries != nil {
			t.Errorf("read the catalog with %+v for %+v, want neither My libraries nor the visitor's", choices, mine[i])
		}
	}
}

// Signed in, the Libraries filter starts with My libraries, which keeps the libraries the dashboard lists: those whose
// owner is the visitor or one of their organizations, and those the visitor's projects use. The sort tabs keep it,
// Clear filters clears it, and the catalog reads the visitor's libraries only when it's on.
func TestMyLibrariesKeepsTheLibrariesOfTheVisitorAndTheirOrganizations(t *testing.T) {
	var chosen []domain.ListChoices
	var read []domain.MyLibraries
	c := newBrowsingCatalog()
	c.chosen, c.mine = &chosen, &read
	site := newDashboardSite(t, octocatsGitHub(), c)

	plain := site.get(t, "/g/techs/go")
	mine := site.get(t, "/g/techs/go?mine=1")
	search := site.get(t, "/search?mine=1&q=errors")

	_, _, fields := filterFields(t, plain)
	if names := fieldNames(fields); len(names) < 2 || names[0] != "mine=1" || names[1] != "libs=example/rules" {
		t.Errorf("the form's fields are %q, want My libraries before the libraries", names)
	}
	if got := checkedFields(fields); !slices.Equal(got, []string{"stars="}) {
		t.Errorf("without the choice, %q are on, want only any stars", got)
	}
	assertShows(t, plain, "Libraries My libraries example/ rules 1 other/ go-rules 2 Impact", "3 rules in 2 libraries")
	_, _, fields = filterFields(t, mine)
	if got := checkedFields(fields); !slices.Equal(got, []string{"mine=1", "stars="}) {
		t.Errorf("with the choice, %q are on, want My libraries and any stars", got)
	}
	assertShows(t, mine, "Filters · 1", "Libraries My libraries example/ rules 1 other/ go-rules 2 Impact",
		"1 rule in 1 library", "Return errors with context HIGH example/rules")
	if strings.Contains(visibleText(t, mine), "Close response bodies") {
		t.Error("My libraries keeps a rule of other/go-rules, which neither octocat nor their organizations own")
	}
	if got := links(t, mine, "Clear filters"); !slices.Equal(got, []string{"/g/techs/go"}) {
		t.Errorf("Clear filters leads to %q", got)
	}
	if got := links(t, mine, "Newest"); !slices.Equal(got, []string{"/g/techs/go?mine=1&sort=new"}) {
		t.Errorf("Newest leads to %q", got)
	}
	assertShows(t, search, "Libraries My libraries", "1 rule in 1 library", "Return errors with context")
	if strings.Contains(visibleText(t, search), "Wrap errors") {
		t.Error("search's My libraries keeps a rule of other/go-rules")
	}
	want := domain.MyLibraries{Owners: []string{"octocat", "octo-org", "example"}, Libraries: []string{"example/rules"}}
	if len(read) != 3 || read[0].Owners != nil || read[0].Libraries != nil || !equalMine(read[1], want) || !equalMine(read[2], want) {
		t.Errorf("read the catalog for the visitor's libraries %+v, want none, then %+v twice", read, want)
	}
	if len(chosen) != 3 || chosen[0].Filters.Mine || !chosen[1].Filters.Mine || !chosen[2].Filters.Mine {
		t.Errorf("read the catalog with %+v, want My libraries on the second and third", chosen)
	}
}

// A visitor whose account and organizations publish no library still has My libraries, which then keeps no rule and
// says so as any filter does.
func TestMyLibrariesOfAVisitorWithoutLibrariesKeepsNoRule(t *testing.T) {
	site := newDashboardSite(t, accounts.Snapshot{ReadAt: time.Now()}, newBrowsingCatalog())

	for _, path := range []string{"/g/techs/go?mine=1", "/search?mine=1"} {
		page := site.get(t, path)

		if _, _, fields := filterFields(t, page); !slices.Contains(checkedFields(fields), "mine=1") {
			t.Errorf("%s doesn't show My libraries on", path)
		}
		assertShows(t, page, "No rules match these filters. Clear filters")
		if strings.Contains(visibleText(t, page), "Return errors with context") {
			t.Errorf("%s keeps a rule the visitor's libraries don't hold", path)
		}
	}
}

// equalMine reports whether a and b name the same visitor's libraries, in order.
func equalMine(a, b domain.MyLibraries) bool {
	return slices.Equal(a.Owners, b.Owners) && slices.Equal(a.Libraries, b.Libraries)
}

// My libraries also keeps the libraries the visitor's projects use, as the dashboard's "Used in your projects" lists
// them, so a visitor who publishes nothing sees the rules of a library one of their projects imports.
func TestMyLibrariesKeepsTheLibrariesTheVisitorsProjectsUse(t *testing.T) {
	snapshot := accounts.Snapshot{ReadAt: time.Now(), Projects: []accounts.Project{{
		Repository: accounts.Repository{Owner: "octocat", Name: "api"},
		Sources:    []accounts.Source{{Name: "go", Library: "Other/Go-Rules"}},
	}}}
	site := newDashboardSite(t, snapshot, newBrowsingCatalog())

	page := site.get(t, "/g/techs/go?mine=1")

	assertShows(t, page, "2 rules in 1 library", "Close response bodies MEDIUM other/go-rules Name packages plainly LOW other/go-rules")
	if strings.Contains(visibleText(t, page), "Return errors with context") {
		t.Error("My libraries keeps a rule of example/rules, which the visitor neither publishes nor uses")
	}
}
