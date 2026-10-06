package web_test

import (
	"cmp"
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

// Rules the browsing catalog lists: example's return-errors, starred, other's close-bodies, name-packages, and
// wrap-errors, whose group, techs/golang, isn't canonical.
var (
	returnErrorsRow = views.RuleRow{Library: exampleRef, Vetted: true, Rule: views.RuleCard{Path: "techs/go/return-errors", Group: "techs/go",
		Title: "Return errors with context", Impact: "HIGH", Version: coderules.RuleVersion{Major: 2}, Stars: 3}, CanonicalGroup: goGroup, GroupRules: 3}
	closeBodiesRow = views.RuleRow{Library: otherRef, Vetted: true, Rule: views.RuleCard{Path: "techs/go/close-bodies", Group: "techs/go",
		Title: "Close response bodies", Impact: "MEDIUM", Version: coderules.RuleVersion{Major: 1}}, CanonicalGroup: goGroup, GroupRules: 3}
	namePackagesRow = views.RuleRow{Library: otherRef, Vetted: true, Rule: views.RuleCard{Path: "techs/go/name-packages", Group: "techs/go",
		Title: "Name packages plainly", Impact: "LOW", Version: coderules.RuleVersion{Major: 1, Minor: 2}}, CanonicalGroup: goGroup, GroupRules: 3}
	wrapErrorsRow = views.RuleRow{Library: otherRef, Vetted: true, Rule: views.RuleCard{Path: "techs/golang/wrap-errors", Group: "techs/golang",
		Title: "Wrap errors", Impact: "MEDIUM", Version: coderules.RuleVersion{Major: 1}}, GroupRules: 1}
	// browsingCounts are the libraries of the Go group's rules, as its sidebar counts them.
	browsingCounts = []views.LibraryCount{{Library: exampleRef, Vetted: true, Rules: 1}, {Library: otherRef, Vetted: true, Rules: 2}}
)

// newBrowsingCatalog returns newCatalog's library, with the groups of two libraries: Go, which both hold, Testing,
// which one holds, and techs/golang, which isn't canonical. It finds two rules for "errors", and lists four as every
// rule. Go, and every rule, hold a retired rule too; techs/golang doesn't.
func newBrowsingCatalog() catalog {
	c := newCatalog()
	c.index = views.GroupIndex{
		Techs: []views.GroupSummary{
			{Path: "techs/go", Canonical: goGroup, Rules: 3, Libraries: []views.LibraryRef{exampleRef, otherRef}, Vetted: true},
			{Path: "techs/golang", Rules: 1, Libraries: []views.LibraryRef{otherRef}, Vetted: true},
		},
		Practices: []views.GroupSummary{
			{Path: "practices/testing", Canonical: &views.CanonicalGroup{Name: "Testing", Description: "What to test and how.", Icon: testingGroup.Icon},
				Rules: 1, Libraries: []views.LibraryRef{exampleRef}, Vetted: true},
		},
	}
	c.unvettedIndex = c.index
	c.groups = map[string]views.GroupPage{
		"techs/go": {Path: "techs/go", Canonical: goGroup, Rules: views.RuleResults{
			Rows: []views.RuleRow{returnErrorsRow, closeBodiesRow, namePackagesRow}, Total: 3, Libraries: 2, Unfiltered: 3,
			UnfilteredCurrent: 3, UnfilteredCurrentLibraries: 2, UnfilteredLibraries: browsingCounts, RetiredRules: 1,
		}},
		"techs/golang": {Path: "techs/golang", Rules: views.RuleResults{
			Rows: []views.RuleRow{wrapErrorsRow}, Total: 1, Libraries: 1, Unfiltered: 1, UnfilteredCurrent: 1, UnfilteredCurrentLibraries: 1,
			UnfilteredLibraries: []views.LibraryCount{{Library: otherRef, Vetted: true, Rules: 1}},
		}},
		"practices/accessibility": {Path: "practices/accessibility", Canonical: &views.CanonicalGroup{Name: "Accessibility"}},
	}
	c.results = map[string]views.RuleResults{
		"errors": {Rows: []views.RuleRow{returnErrorsRow, wrapErrorsRow}, Total: 2, Complete: 2, Libraries: 2, Unfiltered: 2,
			UnfilteredLibraries: []views.LibraryCount{{Library: exampleRef, Vetted: true, Rules: 1}, {Library: otherRef, Vetted: true, Rules: 1}}},
		"": {Rows: []views.RuleRow{returnErrorsRow, closeBodiesRow, namePackagesRow, wrapErrorsRow}, Total: 4, Complete: 4,
			Libraries: 2, Unfiltered: 4, UnfilteredLibraries: browsingCounts, RetiredRules: 1},
	}
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

// linksTo returns the href of every link in an HTML body that leads to an address starting with prefix.
func linksTo(t *testing.T, body, prefix string) []string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var hrefs []string
	for n := range doc.Descendants() {
		if href := attribute(n, "href"); n.Type == html.ElementNode && n.Data == "a" && strings.HasPrefix(href, prefix) {
			hrefs = append(hrefs, href)
		}
	}
	return hrefs
}

// groupPrefix starts the address of every group's page across libraries.
const groupPrefix = "/g/"

// nodeText returns the text under n, with whitespace collapsed.
func nodeText(n *html.Node) string {
	var text strings.Builder
	for d := range n.Descendants() {
		writeText(&text, d)
	}
	return strings.Join(strings.Fields(text.String()), " ")
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

// Every page has search in its header, which holds the search page's query; the search page holds its own field too,
// for a phone, whose header has none.
func TestHeaderSearchesFromEveryPage(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())

	for _, path := range []string{"/", "/browse/techs", "/g/techs/go", library, errorsRule, "/search"} {
		assertSearchForm(t, get(t, handler, path).Body.String(), "header-search", "")
	}
	page := get(t, handler, "/search?q=errors").Body.String()
	assertSearchForm(t, page, "header-search", "errors")
	assertSearchForm(t, page, "search", "errors")
}

// A keyboard's first stop on every page skips the header to the page's content.
func TestPagesLetKeyboardsSkipToTheirContent(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())

	for _, path := range []string{"/", "/libraries", "/browse/techs", "/g/techs/go", "/search?q=errors", library, errorsRule, "/missing/page/here"} {
		page := get(t, handler, path).Body.String()
		if got := links(t, page, "Skip to content"); !slices.Equal(got, []string{"#main"}) || !strings.Contains(page, `<main id="main"`) {
			t.Errorf("%s: the skip link leads to %q", path, got)
		}
		if strings.Index(page, "Skip to content") > strings.Index(page, "<header") {
			t.Errorf("%s: the skip link comes after the header", path)
		}
	}
}

func TestBrowsePagesListEachKindsCanonicalGroupsAcrossLibraries(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())

	resp := get(t, handler, "/browse/techs")

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d", resp.Code)
	}
	page := resp.Body.String()
	// A technology's row names it; its description is in its name. The other groups stand apart.
	assertShows(t, page, "Browse Technologies Technologies Practices Include unvetted libraries Go 3 rules 2 libraries", "View other technology groups (1) →")
	if strings.Contains(visibleText(t, page), "golang") || strings.Contains(visibleText(t, page), "The Go language.") {
		t.Error("the technologies page lists a group that isn't canonical, or a technology's description")
	}
	if got := links(t, page, "Go 3 rules"); !slices.Equal(got, []string{"/g/techs/go"}) {
		t.Errorf("Go links %q", got)
	}
	if got := links(t, page, "View other technology groups"); !slices.Equal(got, []string{"/browse/techs/other"}) {
		t.Errorf("the other groups link %q", got)
	}
	if !strings.Contains(page, "<title>Technologies · Rulemart</title>") {
		t.Error("the title doesn't name the kind")
	}

	practices := get(t, handler, "/browse/practices").Body.String()
	// A practice's row says which rules belong in it, and with no other practice group, nothing leads to them. Neither
	// kind's rows show a group's ID.
	assertShows(t, practices, "Browse Practices", "Testing What to test and how. 1 rule 1 library")
	if strings.Contains(visibleText(t, page), "techs/go") || strings.Contains(visibleText(t, practices), "practices/testing") {
		t.Error("a browse row shows its group's ID")
	}
	if strings.Contains(practices, "View other practice groups") {
		t.Error("the practices page leads to other groups when there are none")
	}
	if got := links(t, practices, "Practices"); !slices.Contains(got, "/browse/practices") {
		t.Errorf("the tabs link %q", got)
	}
}

func TestBrowsePagesSayWhenNoLibraryHasAGroupOfTheKind(t *testing.T) {
	handler := newSite(t, newCatalog())

	assertShows(t, get(t, handler, "/browse/techs").Body.String(), "No technology groups yet.")
	assertShows(t, get(t, handler, "/browse/practices/other").Body.String(), "Other practice groups", "None right now.")
}

// The other-groups page lists each group libraries declared that isn't canonical, one row per ID with its libraries,
// leading to the group's page.
func TestOtherGroupsPageListsEachLibrarysOwnGroups(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())

	resp := get(t, handler, "/browse/techs/other")

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d", resp.Code)
	}
	page := resp.Body.String()
	assertShows(t, page,
		"Technologies › Other groups Other technology groups",
		"Rulemart defines canonical “rule groups” like techs/go",
		"techs/golang other/go-rules 1 rule",
	)
	if strings.Contains(visibleText(t, page), "techs/go 3 rules") {
		t.Error("the other groups page lists a canonical group")
	}
	if got := links(t, page, "techs/golang"); !slices.Equal(got, []string{"/g/techs/golang"}) {
		t.Errorf("techs/golang links %q", got)
	}
	if got := links(t, page, "Technologies"); !slices.Contains(got, "/browse/techs") {
		t.Errorf("the crumb links %q", got)
	}
	if !strings.Contains(page, "<title>Other technology groups · Rulemart</title>") {
		t.Error("the title doesn't name the page")
	}
}

// The groups page's old addresses redirect to the prototype's, keeping the query.
func TestOldGroupAddressesRedirectToTheNewOnes(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())

	for path, location := range map[string]string{
		"/groups":                "/browse/techs",
		"/groups?x=1":            "/browse/techs?x=1",
		"/groups/techs/go":       "/g/techs/go",
		"/groups/techs/go?ref=x": "/g/techs/go?ref=x",
		"/groups/Techs/Go":       "/groups/techs/Go",
		"/Groups/techs/go":       "/groups/techs/go",
		"/groups/techs/missing":  "/g/techs/missing",
		"/browse":                "/browse/techs",
	} {
		resp := get(t, handler, path)
		if resp.Code != http.StatusMovedPermanently || resp.Header().Get("Location") != location {
			t.Errorf("%s: got %d to %q, want 301 to %q", path, resp.Code, resp.Header().Get("Location"), location)
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
		"/browse/techs":       "https://rulemart.example/browse/techs",
		"/browse/practices":   "https://rulemart.example/browse/practices",
		"/browse/techs/other": "https://rulemart.example/browse/techs/other",
		"/g/techs/go":         "https://rulemart.example/g/techs/go",
		"/g/%74echs/go":       "https://rulemart.example/g/techs/go",
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

// What a visitor types reaches the page as text, and the catalog as a cleaned query.
func TestSearchPageShowsTheQueryAsTextAndSearchesItCleaned(t *testing.T) {
	var searched []string
	c := newBrowsingCatalog()
	c.searched = &searched
	handler := newSite(t, c)

	page := get(t, handler, "/search?q="+url.QueryEscape("<script>alert(1)</script>\x00\xff \"x")).Body.String()

	assertShows(t, page, "Rules matching “<script>alert(1)</script> \"x”", "No rules match.")
	if strings.Contains(page, "<script>alert") {
		t.Fatal("the page holds the query as markup")
	}
	if want := []string{"<script>alert(1)</script> \"x"}; !slices.Equal(searched, want) {
		t.Fatalf("searched for %q, want %q", searched, want)
	}
}

func TestBrowsePagesAreCacheableForAMinute(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())

	for _, path := range []string{"/browse/techs", "/browse/techs/other", "/g/techs/go", "/search", "/search?q=errors", "/search?q=nothing"} {
		if got := get(t, handler, path).Header().Get("Cache-Control"); got != "public, max-age=0, s-maxage=60" {
			t.Errorf("%s: Cache-Control is %q", path, got)
		}
	}
}

// The header links techs, practices, the libraries, and the FAQ from every page, and marks one current only on its
// own pages: a library's, a rule's, a group's, or an owner's page marks none, as the prototype's don't.
func TestHeaderMarksALinkCurrentOnlyOnItsOwnPages(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())

	for path, current := range map[string]string{
		"/libraries": "/libraries", library: "", errorsRule: "", library + "?tab=releases": "", "/example": "",
		"/browse/techs": "/browse/techs", "/browse/techs/other": "/browse/techs", "/g/techs/go": "",
		"/browse/practices": "/browse/practices", "/g/practices/accessibility": "",
		"/faq": "/faq", "/": "", "/search": "", "/feedback": "",
	} {
		page := get(t, handler, path).Body.String()
		for _, href := range []string{"/browse/techs", "/browse/practices", "/libraries", "/faq"} {
			if !strings.Contains(page, `href="`+href+`"`) {
				t.Errorf("%s: the header doesn't link %s", path, href)
			}
			if got, want := strings.Contains(page, `href="`+href+`" aria-current="true"`), href == current; got != want {
				t.Errorf("%s: the %s link is current: %v, want %v", path, href, got, want)
			}
		}
	}
}

// Below the wide breakpoint, where the header's links hide, a menu button opens the same links, marking one current
// on the same pages as the header does. The button is a <details data-menu>'s summary, so Enter and Space open it
// without JavaScript, and menus.js closes it on Escape.
func TestHeaderMenuOpensTheSectionsAndMarksTheCurrentOne(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())
	sections := []string{"Techs /browse/techs", "Practices /browse/practices", "Libraries /libraries", "FAQ /faq"}

	for path, current := range map[string]string{
		"/libraries": "/libraries", library: "", "/browse/techs": "/browse/techs", "/browse/techs/other": "/browse/techs",
		"/browse/practices": "/browse/practices", "/g/techs/go": "", "/faq": "/faq", "/": "", "/search": "",
	} {
		doc, err := html.Parse(strings.NewReader(get(t, handler, path).Body.String()))
		if err != nil {
			t.Fatal(err)
		}
		header := find(doc, func(n *html.Node) bool { return n.Data == "header" })
		button := find(header, func(n *html.Node) bool { return n.Data == "summary" && attribute(n, "aria-label") == "Menu" })
		if button == nil {
			t.Errorf("%s: the header has no Menu button", path)
			continue
		}
		menu := button.Parent
		if menu.Data != "details" || !slices.ContainsFunc(menu.Attr, func(a html.Attribute) bool { return a.Key == "data-menu" }) {
			t.Errorf("%s: the Menu button isn't the summary of a <details data-menu>", path)
		}
		var got, marked []string
		for n := range menu.Descendants() {
			if n.Type == html.ElementNode && n.Data == "a" {
				got = append(got, nodeText(n)+" "+attribute(n, "href"))
				if attribute(n, "aria-current") == "true" {
					marked = append(marked, attribute(n, "href"))
				}
			}
		}
		if !slices.Equal(got, sections) {
			t.Errorf("%s: the menu links %q, want %q", path, got, sections)
		}
		if want := slices.DeleteFunc([]string{current}, func(s string) bool { return s == "" }); !slices.Equal(marked, want) {
			t.Errorf("%s: the menu marks %q current, want %q", path, marked, want)
		}
	}
}

// Tabbing through the header reaches the Menu button after the header's links, which it stands in for, and before
// the search icon, the cart, and the account control, which keep their places at the header's end; the open menu's
// links come right after its button.
func TestHeaderMenuComesBetweenTheLinksAndTheSearchIconInFocusOrder(t *testing.T) {
	site := newAccountsSite(t, nil)
	token := site.accounts.signedIn(t, octocat)
	sections := []string{"Techs", "Practices", "Libraries", "FAQ"}
	start := slices.Concat([]string{"Fabrica", "Rulemart home", "Search rules"}, sections, []string{"Menu"}, sections, []string{"Search rules", "Cart"})

	for name, test := range map[string]struct {
		cookies []*http.Cookie
		want    []string
	}{
		"signed out": {want: append(slices.Clone(start), "Sign in with GitHub")},
		"signed in":  {cookies: []*http.Cookie{{Name: sessionCookie, Value: string(token)}}, want: append(slices.Clone(start), "Account menu, signed in as octocat", "Dashboard", "Sign out")},
	} {
		page := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/faq", cookies: test.cookies}))
		doc, err := html.Parse(strings.NewReader(page))
		if err != nil {
			t.Fatal(err)
		}
		header := find(doc, func(n *html.Node) bool { return n.Data == "header" })
		var got []string
		for n := range header.Descendants() {
			if n.Type == html.ElementNode && slices.Contains([]string{"a", "input", "summary", "button"}, n.Data) {
				got = append(got, cmp.Or(attribute(n, "aria-label"), attribute(n, "placeholder"), nodeText(n)))
			}
		}
		if !slices.Equal(got, test.want) {
			t.Errorf("%s: the header's focus order is %q, want %q", name, got, test.want)
		}
	}
}

// The libraries page lists every vetted library, as the home page does, under its own address.
func TestLibrariesPageListsTheVettedLibraries(t *testing.T) {
	base, err := web.ParseBaseURL("https://rulemart.example")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := web.New(newBrowsingCatalog(), web.Options{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), BaseURL: base})
	if err != nil {
		t.Fatal(err)
	}

	resp := get(t, handler, "/libraries")

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d", resp.Code)
	}
	page := resp.Body.String()
	assertShows(t, page, "Libraries Libraries Rulemart has vetted Anyone can list a public library. It shows, with a warning, under unvetted libraries", "rules Vetted by Rulemart Example rules for tests. example/rules · 2 rules")
	if got := links(t, page, "Example rules for tests."); !slices.Equal(got, []string{library}) {
		t.Errorf("the library links %q", got)
	}
	if got := canonicalLinks(t, page); !slices.Equal(got, []string{"https://rulemart.example/libraries"}) {
		t.Errorf("the page names %q as its address", got)
	}
	// Each library heads its row, as on the home page, where they're under the Libraries heading.
	if got := headings(t, page, "h2"); !slices.Equal(got, []string{"rules"}) {
		t.Errorf("the libraries page's headings are %q", got)
	}
	home := get(t, handler, "/").Body.String()
	if got := headings(t, home, "h3"); !slices.Contains(got, "rules") {
		t.Errorf("the home page's library isn't a heading under Libraries: %q", got)
	}
	if got := links(t, home, "All libraries"); !slices.Equal(got, []string{"/libraries"}) {
		t.Errorf("the home page links all libraries as %q", got)
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
		"/g/Techs/GO":                            "/g/techs/GO",
		"/g/techs/GO":                            "/g/techs/go",
		"/g/techs/Go?sort=new":                   "/g/techs/go?sort=new",
		"/browse/":                               "/browse",
		"/browse/techs/":                         "/browse/techs",
		"/browse/Techs":                          "/browse/techs",
		"/browse/PRACTICES/other?x=1":            "/browse/practices/other?x=1",
		"/G/techs/go":                            "/g/techs/go",
		"/FAQ":                                   "/faq",
		"/Feedback?x=1":                          "/feedback?x=1",
		"/search/?q=errors&page=2":               "/search?q=errors&page=2",
		"/example/rules/":                        "/example/rules",
		"/example/rules/?tab=rules":              "/example/rules?tab=rules",
		"/example/rules/techs/go/return-errors/": "/example/rules/techs/go/return-errors",
		"/g/techs/go///":                         "/g/techs/go",
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

// headings returns the text of each heading of level in an HTML body, in order.
func headings(t *testing.T, body, level string) []string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for n := range doc.Descendants() {
		if n.Type == html.ElementNode && n.Data == level {
			texts = append(texts, nodeText(n))
		}
	}
	return texts
}

// listItems counts the items of each list of kind, ul or ol, in an HTML body.
func listItems(t *testing.T, body, kind string) []int {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var counts []int
	for n := range doc.Descendants() {
		if n.Type == html.ElementNode && n.Data == kind {
			count := 0
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && c.Data == "li" {
					count++
				}
			}
			counts = append(counts, count)
		}
	}
	return counts
}
