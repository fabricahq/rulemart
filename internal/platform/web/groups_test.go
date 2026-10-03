package web_test

import (
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/platform/web"
)

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
		"Libraries example/ rules 1 other/ go-rules 2 Impact Critical and high Medium and lower Stars Any 10+ 50+ 100+ "+
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

// When no rule passes the filters, the page says so, without counting none, and leads to the list without them; a
// canonical group no library holds says that instead.
func TestGroupPageSaysWhenNoRulePassesTheFiltersOrNoLibraryHoldsIt(t *testing.T) {
	c := newBrowsingCatalog()
	filtered := c.groups["techs/go"]
	filtered.Rules.Rows, filtered.Rules.Total, filtered.Rules.Libraries = nil, 0, 0
	c.groups["techs/go"] = filtered
	handler := newSite(t, c)

	page := get(t, handler, "/g/techs/go?stars=100").Body.String()
	empty := get(t, handler, "/g/practices/accessibility")

	assertShows(t, page, "3 rules from 2 libraries · The Go language.", "Rules No rules match these filters. Clear filters")
	if strings.Contains(visibleText(t, page), "0 rules") {
		t.Error("the page counts no rules beside saying none match")
	}
	if got := links(t, page, "Clear filters"); !slices.Equal(got, []string{"/g/techs/go", "/g/techs/go"}) {
		t.Errorf("Clear filters leads to %q", got)
	}
	if empty.Code != http.StatusOK {
		t.Fatalf("a canonical group no library holds: got %d", empty.Code)
	}
	assertShows(t, empty.Body.String(), "Accessibility Filters", "Rules No library has Accessibility rules yet.")
	if strings.Contains(visibleText(t, empty.Body.String()), "0 rules") {
		t.Error("a group no library holds counts its rules")
	}
}

// While a group's page shows its retired rules, the count under its title still counts its current rules only, so
// showing them doesn't make the group look bigger.
func TestGroupPageCountsOnlyItsCurrentRulesWhileShowingRetiredOnes(t *testing.T) {
	c := newBrowsingCatalog()
	group := c.groups["techs/go"]
	group.Rules.Unfiltered, group.Rules.Total = 4, 4
	c.groups["techs/go"] = group

	page := get(t, newSite(t, c), "/g/techs/go?retired=1").Body.String()

	assertShows(t, page, "Go 3 rules from 2 libraries · The Go language.")
}

// A group whose rules are all retired says so while it hides them, rather than that no library has its rules, and leads
// to the list that shows them, as the sidebar's Show retired rules does.
func TestGroupPageOfOnlyRetiredRulesSaysSoAndOffersToShowThem(t *testing.T) {
	c := newBrowsingCatalog()
	retired := c.groups["practices/accessibility"]
	retired.Rules.RetiredRules = 1
	c.groups["practices/accessibility"] = retired

	page := get(t, newSite(t, c), "/g/practices/accessibility").Body.String()

	assertShows(t, page, "Retired Show retired rules", "Rules Every rule in this group is retired. Show retired rules")
	if strings.Contains(visibleText(t, page), "No library has") {
		t.Error("the page says no library has the group's rules")
	}
	if got := links(t, page, "Show retired rules"); !slices.Equal(got, []string{"/g/practices/accessibility?retired=1"}) {
		t.Errorf("Show retired rules leads to %q", got)
	}
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
	retired.Retired, retired.Replacement = true, &views.RuleRef{Path: "techs/go/close-everything", Title: "Close everything"}
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
	renamed.Rules.Rows[0].Replacement = &views.RuleRef{Path: "techs/go/close-bodies-early", Title: "Close response bodies"}
	renamed.Rules.Rows[0].Renamed = true
	c.groups["techs/golang"] = renamed
	assertShows(t, get(t, newSite(t, c), "/g/techs/golang?retired=1").Body.String(),
		"Close response bodies MEDIUM Retired Renamed to techs/go/close-bodies-early other/go-rules")
	assertShows(t, body,
		"Close response bodies MEDIUM Retired Replaced by Close everything techs/go/close-everything other/go-rules",
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
	if find(unvettedRow, func(n *html.Node) bool { return attribute(n, "title") == "Published by stranger" }) == nil {
		t.Error("another owner's rule doesn't name its owner on its mark")
	}
}
