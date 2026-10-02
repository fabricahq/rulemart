package web_test

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
	"github.com/fabricahq/rulemart/internal/platform/web"
)

var (
	exampleRef = views.LibraryRef{Owner: "example", Name: "rules", OwnerAvatarURL: "https://avatars.githubusercontent.com/u/1?v=4"}
	otherRef   = views.LibraryRef{Owner: "other", Name: "go-rules", OwnerAvatarURL: "https://avatars.githubusercontent.com/u/2?v=4"}
)

// newBrowsingCatalog returns newCatalog's library, with the groups of two libraries: Go, which both hold, Testing,
// which one holds, and techs/golang, which isn't canonical. It finds two rules for "errors".
func newBrowsingCatalog() catalog {
	c := newCatalog()
	c.index = views.GroupIndex{
		Techs: []views.GroupSummary{
			{Path: "techs/go", Canonical: goGroup, Rules: 3, Libraries: []views.LibraryRef{exampleRef, otherRef}},
			{Path: "techs/golang", Rules: 1, Libraries: []views.LibraryRef{otherRef}},
		},
		Practices: []views.GroupSummary{
			{Path: "practices/testing", Canonical: &views.CanonicalGroup{Name: "Testing", Description: "What to test and how.", Icon: testingGroup.Icon},
				Rules: 1, Libraries: []views.LibraryRef{exampleRef}},
		},
	}
	c.groups = map[string]views.GroupPage{
		"techs/go": {Path: "techs/go", Canonical: views.CanonicalGroup{Name: "Go", Description: "The Go language.", Icon: goGroup.Icon},
			Libraries: []views.GroupLibrary{
				{Library: exampleRef, Rules: []views.RuleCard{{Path: "techs/go/return-errors", Group: "techs/go", Title: "Return errors with context",
					Impact: "HIGH", Version: coderules.RuleVersion{Major: 2}}}},
				{Library: otherRef, Rules: []views.RuleCard{
					{Path: "techs/go/close-bodies", Group: "techs/go", Title: "Close response bodies", Impact: "MEDIUM", Version: coderules.RuleVersion{Major: 1}},
					{Path: "techs/go/name-packages", Group: "techs/go", Title: "Name packages plainly", Impact: "LOW", Version: coderules.RuleVersion{Major: 1, Minor: 2}},
				}},
			}},
		"practices/accessibility": {Path: "practices/accessibility", Canonical: views.CanonicalGroup{Name: "Accessibility"}},
	}
	c.results = map[string]views.SearchResults{"errors": {Total: 2, Complete: 2, Results: []views.SearchResult{
		{Library: exampleRef, Rule: views.RuleCard{Path: "techs/go/return-errors", Group: "techs/go", Title: "Return errors with context",
			Impact: "HIGH", Version: coderules.RuleVersion{Major: 2}}, CanonicalGroup: goGroup, WhenToRead: "When a function fails."},
		{Library: otherRef, Rule: views.RuleCard{Path: "techs/golang/wrap-errors", Group: "techs/golang", Title: "Wrap errors",
			Impact: "MEDIUM", Version: coderules.RuleVersion{Major: 1}}, WhenToRead: "When returning an error."},
	}}}
	return c
}

// links returns the href of every link in an HTML body whose visible text includes text.
func links(t *testing.T, body, text string) []string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var hrefs []string
	for n := range doc.Descendants() {
		if n.Type == html.ElementNode && n.Data == "a" && strings.Contains(nodeText(n), text) {
			hrefs = append(hrefs, attribute(n, "href"))
		}
	}
	return hrefs
}

// nodeText returns the text under n, with whitespace collapsed.
func nodeText(n *html.Node) string {
	var text strings.Builder
	for d := range n.Descendants() {
		if d.Type == html.TextNode {
			text.WriteString(d.Data + " ")
		}
	}
	return strings.Join(strings.Fields(text.String()), " ")
}

func TestHomeLeadsWithSearchAndBrowsesCanonicalGroups(t *testing.T) {
	page := get(t, newSite(t, newBrowsingCatalog()), "/").Body.String()

	assertShows(t, page, "Browse by group All groups ›", "Technologies Go 3 rules", "Practices Testing 1 rule")
	if got := links(t, page, "Go 3 rules"); !slices.Equal(got, []string{"/groups/techs/go"}) {
		t.Errorf("Go's tile links %q", got)
	}
	// A group that isn't canonical stands alone, so the home page's tiles, which lead across libraries, leave it out.
	if strings.Contains(visibleText(t, page), "golang") {
		t.Error("the home page shows a group that isn't canonical")
	}
	assertSearchForm(t, page, "home-search", "")
}

// assertSearchForm fails unless page has a search form with the field id, holding value, that submits q to /search
// with GET.
func assertSearchForm(t *testing.T, page, id, value string) {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	input := find(doc, func(n *html.Node) bool { return n.Data == "input" && attribute(n, "id") == id })
	if input == nil || attribute(input, "name") != "q" || attribute(input, "value") != value {
		t.Fatalf("no search field %s holding %q", id, value)
	}
	label := find(doc, func(n *html.Node) bool { return n.Data == "label" && attribute(n, "for") == id })
	if label == nil || nodeText(label) != "Search rules" {
		t.Errorf("the search field %s has no label", id)
	}
	for form := input.Parent; ; form = form.Parent {
		if form == nil {
			t.Fatalf("the search field %s is in no form", id)
		}
		if form.Data == "form" {
			if attribute(form, "action") != "/search" || attribute(form, "method") != "get" {
				t.Fatalf("the search form submits to %s with %s", attribute(form, "action"), attribute(form, "method"))
			}
			return
		}
	}
}

// The search form submits to Rulemart itself, which the security policy must allow, and nowhere else.
func TestPagesLetFormsSubmitOnlyToRulemart(t *testing.T) {
	resp := get(t, newSite(t, newBrowsingCatalog()), "/")

	policy := resp.Header().Get("Content-Security-Policy")
	if !strings.Contains(policy, "form-action 'self';") {
		t.Fatalf("the policy is %q, want form-action 'self'", policy)
	}
}

// Every page but the search page has search in its header; the search page holds its own field.
func TestHeaderSearchesFromEveryPageButSearch(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())

	for _, path := range []string{"/", "/groups", "/groups/techs/go", library, errorsRule} {
		assertSearchForm(t, get(t, handler, path).Body.String(), "header-search", "")
	}
	if page := get(t, handler, "/search").Body.String(); strings.Contains(page, `id="header-search"`) {
		t.Fatal("the search page repeats search in its header")
	}
}

func TestGroupsPageListsTechnologiesThenPracticesAcrossLibraries(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())

	resp := get(t, handler, "/groups")

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d", resp.Code)
	}
	page := resp.Body.String()
	assertShows(t, page,
		"Technologies · 2 Go techs/go 3 rules · 2 libraries › techs/golang not canonical in other/go-rules 1 rule ›",
		"Practices · 1 Testing practices/testing What to test and how. 1 rule · 1 library ›",
	)
	assertFlagsExplainThemselves(t, page, 1)
	// A canonical group leads to its page across libraries; any other group to its section in its one library.
	if got := links(t, page, "techs/go "); !slices.Equal(got, []string{"/groups/techs/go"}) {
		t.Errorf("Go links %q", got)
	}
	if got := links(t, page, "techs/golang"); !slices.Equal(got, []string{"/other/go-rules?tab=rules#group-techs-golang"}) {
		t.Errorf("techs/golang links %q", got)
	}
	if strings.Contains(page, `href="/groups/techs/golang"`) {
		t.Error("a group that isn't canonical links a page across libraries")
	}
}

func TestGroupsPageSaysWhenNoLibraryHasAGroup(t *testing.T) {
	page := get(t, newSite(t, newCatalog()), "/groups").Body.String()

	assertShows(t, page, "No groups yet.")
}

// A canonical group's page names each library its rules come from, in the order the catalog gives, and leads to each
// library and its section for the group.
func TestGroupPageShowsEachLibrarysRules(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())

	resp := get(t, handler, "/groups/techs/go")

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d", resp.Code)
	}
	page := resp.Body.String()
	assertShows(t, page,
		"Groups › Technologies Go techs/go The Go language. Rules 3 in 2 libraries",
		"example/rules 1 rule View in library › Return errors with context HIGH 2.0.0 techs/go/return-errors "+
			"other/go-rules 2 rules View in library › Close response bodies MEDIUM 1.0.0 techs/go/close-bodies "+
			"Name packages plainly LOW 1.2.0 techs/go/name-packages",
	)
	for text, want := range map[string]string{
		"Technologies":               "/groups#technologies",
		"other/go-rules":             "/other/go-rules",
		"Close response bodies":      "/other/go-rules/techs/go/close-bodies",
		"Return errors with context": errorsRule,
	} {
		if got := links(t, page, text); !slices.Contains(got, want) {
			t.Errorf("%s links %q, want %s", text, got, want)
		}
	}
	if got := links(t, page, "View in library"); !slices.Equal(got, []string{
		library + "?tab=rules#group-techs-go", "/other/go-rules?tab=rules#group-techs-go",
	}) {
		t.Errorf("the libraries' sections are %q", got)
	}
}

func TestGroupPageSaysWhenNoLibraryHoldsTheGroup(t *testing.T) {
	resp := get(t, newSite(t, newBrowsingCatalog()), "/groups/practices/accessibility")

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d", resp.Code)
	}
	assertShows(t, resp.Body.String(), "Rules 0", "No vetted library has Accessibility rules yet.")
}

func TestGroupPageAnswersNotFoundForAGroupThatIsntCanonical(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())

	for _, path := range []string{"/groups/techs/golang", "/groups/Techs/Golang", "/groups/techs"} {
		if resp := get(t, handler, path); resp.Code != http.StatusNotFound {
			t.Errorf("%s: got %d", path, resp.Code)
		}
	}
}

func TestBrowsePagesNameTheirAddressAsCanonical(t *testing.T) {
	base, err := web.ParseBaseURL("https://rulemart.example")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := web.New(newBrowsingCatalog(), web.Options{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), BaseURL: base})
	if err != nil {
		t.Fatal(err)
	}

	for path, want := range map[string]string{
		"/groups":            "https://rulemart.example/groups",
		"/groups/techs/go":   "https://rulemart.example/groups/techs/go",
		"/groups/%74echs/go": "https://rulemart.example/groups/techs/go",
	} {
		if got := canonicalLinks(t, get(t, handler, path).Body.String()); !slices.Equal(got, []string{want}) {
			t.Errorf("%s names %q, want %s", path, got, want)
		}
	}
	// Search results aren't content of their own: their page names no address and asks not to be indexed.
	page := get(t, handler, "/search?q=errors").Body.String()
	if got := canonicalLinks(t, page); len(got) != 0 || !strings.Contains(page, `<meta name="robots" content="noindex">`) {
		t.Errorf("the search page names %q, and noindex: %v", got, strings.Contains(page, "noindex"))
	}
}

// Each result names its rule's library, by avatar and source-qualified ID, and its group, as other pages do.
func TestSearchPageShowsEachResultWithItsLibraryAndGroup(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())

	resp := get(t, handler, "/search?q=errors")

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d", resp.Code)
	}
	page := resp.Body.String()
	assertShows(t, page,
		"2 rules match “errors”",
		"Return errors with context HIGH When a function fails. example/rules:techs/go/return-errors Go 2.0.0",
		"Wrap errors MEDIUM When returning an error. other/go-rules:techs/golang/wrap-errors techs/golang not canonical 1.0.0",
	)
	assertFlagsExplainThemselves(t, page, 1)
	assertSearchForm(t, page, "search", "errors")
	if got := links(t, page, "Wrap errors"); !slices.Equal(got, []string{"/other/go-rules/techs/golang/wrap-errors"}) {
		t.Errorf("the result links %q", got)
	}
	if !strings.Contains(page, "<title>“errors” · Search · Rulemart</title>") {
		t.Error("the title doesn't name the query")
	}
}

func TestSearchPageSaysHowManyOfTheMatchesItShows(t *testing.T) {
	c := newBrowsingCatalog()
	results := c.results["errors"]
	results.Total, results.Complete = 87, 87
	c.results["errors"] = results

	page := get(t, newSite(t, c), "/search?q=errors").Body.String()

	assertShows(t, page, "87 rules match “errors” · Page 1 of 5")
}

// A search that matches more rules than a page holds links the pages before and after the one shown.
func TestSearchPageLinksThePagesBeforeAndAfterIt(t *testing.T) {
	c := newBrowsingCatalog()
	first := c.results["errors"]
	first.Total, first.Complete = 45, 45
	c.results["errors"] = first
	c.results["errors page 2"], c.results["errors page 3"] = first, first
	handler := newSite(t, c)

	for path, want := range map[string]struct {
		summary        string
		previous, next []string
	}{
		"/search?q=errors":        {"45 rules match “errors” · Page 1 of 3", nil, []string{"/search?page=2&q=errors"}},
		"/search?q=errors&page=2": {"45 rules match “errors” · Page 2 of 3", []string{"/search?q=errors"}, []string{"/search?page=3&q=errors"}},
		"/search?q=errors&page=3": {"45 rules match “errors” · Page 3 of 3", []string{"/search?page=2&q=errors"}, nil},
	} {
		resp := get(t, handler, path)
		if resp.Code != http.StatusOK {
			t.Fatalf("%s: got %d", path, resp.Code)
		}
		page := resp.Body.String()
		assertShows(t, page, want.summary)
		if got := links(t, page, "Previous"); !slices.Equal(got, want.previous) {
			t.Errorf("%s: Previous links %q, want %q", path, got, want.previous)
		}
		if got := links(t, page, "Next"); !slices.Equal(got, want.next) {
			t.Errorf("%s: Next links %q, want %q", path, got, want.next)
		}
	}
	// One page needs no links between pages.
	if page := get(t, newSite(t, newBrowsingCatalog()), "/search?q=errors").Body.String(); strings.Contains(page, "Page 1 of") {
		t.Error("a single page numbers itself")
	}
}

// A page number has one spelling: the first page's address names none, and a number that isn't a page's leads there.
// A page past the last is missing, and says so on a search page.
func TestSearchPageNumbersRedirectToTheirAddressOrAreMissing(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())

	for path, location := range map[string]string{
		"/search?q=errors&page=1":   "/search?q=errors",
		"/search?q=errors&page=0":   "/search?q=errors",
		"/search?q=errors&page=-2":  "/search?q=errors",
		"/search?q=errors&page=two": "/search?q=errors",
		"/search?q=errors&page=":    "/search?q=errors",
		"/search?q=errors&page=1e3": "/search?q=errors",
		"/search?page=2":            "/search",
		"/search?q=a+b&page=1":      "/search?q=a+b",
	} {
		resp := get(t, handler, path)
		if resp.Code != http.StatusMovedPermanently || resp.Header().Get("Location") != location {
			t.Errorf("%s: got %d to %q, want 301 to %q", path, resp.Code, resp.Header().Get("Location"), location)
		}
	}
	for _, path := range []string{"/search?q=errors&page=2", "/search?q=errors&page=201", "/search?q=errors&page=99999999999999999999", "/search?q=nothing&page=3"} {
		resp := get(t, handler, path)
		if resp.Code != http.StatusNotFound {
			t.Errorf("%s: got %d, want 404", path, resp.Code)
			continue
		}
		page := resp.Body.String()
		assertShows(t, page, "has no page", "Go to the first page")
		assertSearchForm(t, page, "search", strings.Split(strings.TrimPrefix(path, "/search?q="), "&")[0])
	}
}

// A search of several words finds rules that hold only some of them, after those that hold every one, and each such
// result names the words it lacks.
func TestSearchPageNamesTheWordsEachResultLacks(t *testing.T) {
	c := newBrowsingCatalog()
	results := c.results["errors"]
	results.Complete = 1
	results.Results[1].Missing = []string{"handling", `"error chain"`}
	c.results["error handling \"error chain\""] = results
	c.results["errors"] = views.SearchResults{Total: 1, Complete: 0, Results: results.Results[1:]}
	handler := newSite(t, c)

	page := get(t, handler, "/search?q="+url.QueryEscape(`error handling "error chain"`)).Body.String()

	assertShows(t, page,
		// The quoted query is a node of its own, which visible text separates from the punctuation after it.
		`1 rule matches every word of “error handling "error chain"” , and 1 more match some of them`,
		"Return errors with context HIGH When a function fails. example/rules",
		`Wrap errors MEDIUM When returning an error. Missing: handling "error chain" other/go-rules`,
	)
	if !strings.Contains(page, "<s>handling</s>") {
		t.Error("the missing words aren't struck through")
	}
	assertShows(t, get(t, handler, "/search?q=errors").Body.String(), "No rule matches every word of “errors” ; 1 rule matches some of them")
}

// A query of only words search skips, or words to leave out, says why it finds nothing.
func TestSearchPageExplainsAQueryWithNoWordToFind(t *testing.T) {
	c := newBrowsingCatalog()
	c.results["the -errors"] = views.SearchResults{NoWords: true}

	page := get(t, newSite(t, c), "/search?q=the+-errors").Body.String()

	assertShows(t, page, "Nothing to search for in “the -errors”.", "Search skips common words, such as “the”")
	if strings.Contains(visibleText(t, page), "No rules match") {
		t.Error("the page says no rules match")
	}
}

func TestSearchPageStatesBeforeAndWithoutResults(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())
	long := strings.Repeat("retry ", 100)

	for name, tc := range map[string]struct {
		path        string
		shows       []string
		doesntShow  string
		field       string
		wantFocused bool
	}{
		"before a search": {"/search", []string{"Search Find rules in every vetted library"}, "match", "", true},
		"a blank query":   {"/search?q=+%00+", []string{"Search Find rules"}, "match", "", true},
		"no match": {"/search?q=nothing+here", []string{"No rules match “nothing here”.", "Try other words, or browse rules by group"},
			"Search holds", "nothing here", false},
		"a query too long": {"/search?q=" + url.QueryEscape(long), []string{"Search holds up to 200 characters. Shorten yours and search again."},
			"No rules match", strings.TrimSpace(long), false},
	} {
		t.Run(name, func(t *testing.T) {
			resp := get(t, handler, tc.path)

			if resp.Code != http.StatusOK {
				t.Fatalf("got %d", resp.Code)
			}
			page := resp.Body.String()
			assertShows(t, page, tc.shows...)
			if strings.Contains(visibleText(t, page), tc.doesntShow) {
				t.Errorf("the page shows %q", tc.doesntShow)
			}
			assertSearchForm(t, page, "search", tc.field)
			if focused := strings.Contains(page, " autofocus"); focused != tc.wantFocused {
				t.Errorf("the field is focused: %v, want %v", focused, tc.wantFocused)
			}
		})
	}
}

// What a visitor types reaches the page as text, and the catalog as a cleaned query.
func TestSearchPageShowsTheQueryAsTextAndSearchesItCleaned(t *testing.T) {
	var searched []string
	c := newBrowsingCatalog()
	c.searched = &searched
	handler := newSite(t, c)

	page := get(t, handler, "/search?q="+url.QueryEscape("<script>alert(1)</script>\x00\xff \"x")).Body.String()

	assertShows(t, page, "No rules match “<script>alert(1)</script> \"x”.")
	if strings.Contains(page, "<script>alert") {
		t.Fatal("the page holds the query as markup")
	}
	if want := []string{"<script>alert(1)</script> \"x"}; !slices.Equal(searched, want) {
		t.Fatalf("searched for %q, want %q", searched, want)
	}
}

func TestBrowsePagesAreCacheableForAMinute(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())

	for _, path := range []string{"/groups", "/groups/techs/go", "/search", "/search?q=errors", "/search?q=nothing"} {
		if got := get(t, handler, path).Header().Get("Cache-Control"); got != "public, max-age=60" {
			t.Errorf("%s: Cache-Control is %q", path, got)
		}
	}
}

// The header's Groups link marks the part of the site a groups page belongs to.
func TestHeaderMarksTheGroupsLinkOnGroupsPages(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())

	for path, current := range map[string]bool{"/groups": true, "/groups/techs/go": true, "/": false, "/search": false} {
		page := get(t, handler, path).Body.String()
		if got := strings.Contains(page, `href="/groups" aria-current="page"`); got != current {
			t.Errorf("%s: the Groups link is current: %v, want %v", path, got, current)
		}
	}
}

// A failed search is logged with its route, never with what the visitor searched for.
func TestSearchFailureLeavesTheQueryOutOfTheLogs(t *testing.T) {
	handler, logs := loggedSite(t, catalog{err: errors.New("search rules: connection refused")})

	resp := get(t, handler, "/search?q=private+words")

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d", resp.Code)
	}
	if !strings.Contains(logs.String(), `"route":"/search"`) || strings.Contains(logs.String(), "private") {
		t.Fatalf("the logs are %s", logs)
	}
}

// Each page has one address: a group's ID in another case, and a path with a trailing slash, redirect to it,
// keeping the query, as a library's other spellings do.
func TestPagesRedirectOtherSpellingsOfTheirAddress(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())

	for path, location := range map[string]string{
		"/groups/Techs/GO":                       "/groups/techs/go",
		"/groups/techs/Go?ref=x":                 "/groups/techs/go?ref=x",
		"/groups/":                               "/groups",
		"/search/?q=errors&page=2":               "/search?q=errors&page=2",
		"/example/rules/":                        "/example/rules",
		"/example/rules/?tab=rules":              "/example/rules?tab=rules",
		"/example/rules/techs/go/return-errors/": "/example/rules/techs/go/return-errors",
		"/groups/techs/go///":                    "/groups/techs/go",
	} {
		resp := get(t, handler, path)
		if resp.Code != http.StatusMovedPermanently || resp.Header().Get("Location") != location {
			t.Errorf("%s: got %d to %q, want 301 to %q", path, resp.Code, resp.Header().Get("Location"), location)
		}
	}
	// A path that starts with slashes never redirects to another host.
	for _, path := range []string{"//example.com/", "///example.com/", "/\\example.com/", "//example.com//"} {
		location := get(t, handler, path).Header().Get("Location")
		if strings.HasPrefix(location, "//") || strings.HasPrefix(location, "/\\") || strings.Contains(location, "://") {
			t.Errorf("%s redirects to %q", path, location)
		}
	}
	if resp := get(t, handler, "/"); resp.Code != http.StatusOK {
		t.Errorf("/: got %d", resp.Code)
	}
}
