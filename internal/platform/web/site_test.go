package web_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
	"github.com/fabricahq/rulemart/internal/platform/web"
)

const (
	library    = "/example/rules"
	retryRule  = library + "/practices/testing/verify-retry-limits"
	errorsRule = library + "/techs/go/return-errors"
)

// catalog serves the pages' reads from memory, matching libraries without regard to case as the store does.
type catalog struct {
	libraries []views.LibraryCard
	// pages are keyed by lowercase owner/name, and rules by lowercase owner/name, then /<rule path>.
	pages map[string]views.LibraryPage
	rules map[string]views.RulePage
	index views.GroupIndex
	// groups are the canonical groups' pages, keyed by ID.
	groups map[string]views.GroupPage
	// results are keyed by the query that finds them, followed for pages after the first by " page " and the page's
	// number; any other query or page finds nothing.
	results map[string]views.SearchResults
	// searched records each query searched, when it isn't nil.
	searched *[]string
	// err, when set, fails every read.
	err error
}

func (c catalog) HomePage(context.Context) (views.HomePage, error) {
	return views.HomePage{Libraries: c.libraries, Groups: c.index}, c.err
}

func (c catalog) Libraries(context.Context) ([]views.LibraryCard, error) { return c.libraries, c.err }

func (c catalog) GroupIndex(context.Context) (views.GroupIndex, error) { return c.index, c.err }

// GroupPage matches id without regard to case, as app.Pages does.
func (c catalog) GroupPage(_ context.Context, id string) (views.GroupPage, error) {
	page, ok := c.groups[strings.ToLower(id)]
	if c.err == nil && !ok {
		return page, fmt.Errorf("load group: %w", app.ErrNotFound)
	}
	return page, c.err
}

// Search answers as app.Pages does: nothing for the zero query, and app.ErrSearchQueryTooLong for a long one.
func (c catalog) Search(_ context.Context, query domain.SearchQuery, page int) (views.SearchResults, error) {
	if c.err != nil || query.IsZero() {
		return views.SearchResults{}, c.err
	}
	if query.TooLong() {
		return views.SearchResults{}, fmt.Errorf("search: %w", app.ErrSearchQueryTooLong)
	}
	if c.searched != nil {
		*c.searched = append(*c.searched, query.String())
	}
	if page > 1 {
		return c.results[fmt.Sprintf("%s page %d", query, page)], nil
	}
	return c.results[query.String()], nil
}

func (c catalog) LibraryPage(_ context.Context, owner, name string) (views.LibraryPage, error) {
	page, ok := c.pages[strings.ToLower(owner+"/"+name)]
	if c.err == nil && !ok {
		return page, fmt.Errorf("load library %s/%s: %w", owner, name, app.ErrNotFound)
	}
	return page, c.err
}

func (c catalog) RulePage(_ context.Context, owner, name, rulePath string) (views.RulePage, error) {
	page, ok := c.rules[strings.ToLower(owner+"/"+name)+"/"+rulePath]
	if c.err == nil && !ok {
		return page, fmt.Errorf("load rule %s/%s/%s: %w", owner, name, rulePath, app.ErrNotFound)
	}
	return page, c.err
}

// day returns noon UTC on day n of September 2026.
func day(n int) time.Time { return time.Date(2026, 9, n, 12, 0, 0, 0, time.UTC) }

// exampleRules is a library whose second release changed verify-retry-limits to 1.1.0, and whose third changed
// return-errors to 2.0.0, a major change.
var exampleRules = views.Library{
	Owner: "example", Name: "rules", Description: "Example rules for tests.",
	OwnerAvatarURL: "https://avatars.githubusercontent.com/u/1?v=4", LicenseExpression: "MIT", LicenseFile: "LICENSE",
	LatestRelease: 3, LatestTaggedAt: day(3),
}

// goGroup and testingGroup are how the canonical group list shows techs/go and practices/testing.
var (
	goGroup      = &views.CanonicalGroup{Name: "Go", Description: "The Go language.", Icon: views.GroupIcon{File: "devicon/go-original.svg"}}
	testingGroup = &views.CanonicalGroup{Name: "Testing", Description: "What to test and how.", Icon: views.GroupIcon{File: "lucide/flask-conical.svg", Monochrome: true}}
)

// newCatalog returns a catalog that holds exampleRules.
func newCatalog() catalog {
	returnErrors := views.RulePage{
		Library: exampleRules,
		Rule: views.Rule{
			Path: "techs/go/return-errors", Group: "techs/go", CanonicalGroup: goGroup, Title: "Return errors with context",
			Impact: "HIGH", WhenToRead: "When changing return errors with context.",
			HTML: "<p>Wrap every returned error.</p>\n", Version: coderules.RuleVersion{Major: 2}, Release: 3, PublishedAt: day(3),
		},
		Versions: []views.Version{
			{Version: coderules.RuleVersion{Major: 2}, Release: 3, PublishedAt: day(3), Change: coderules.ChangeMajor,
				Summaries: []string{"Require context on every error.", "Add an example."}},
			{Version: coderules.RuleVersion{Major: 1}, Release: 1, PublishedAt: day(1), Change: coderules.ChangeNew,
				Summaries: []string{"Add the rule."}},
		},
	}
	retryLimits := views.RulePage{
		Library: exampleRules,
		Rule: views.Rule{
			Path: "practices/testing/verify-retry-limits", Group: "practices/testing", CanonicalGroup: testingGroup,
			Title: "Verify retry limits", Impact: "HIGH", WhenToRead: "When changing verify retry limits.",
			HTML: "<p>Stop after a fixed number of attempts.</p>\n", Version: coderules.RuleVersion{Major: 1, Minor: 1},
			Release: 2, PublishedAt: day(2),
		},
		Versions: []views.Version{
			{Version: coderules.RuleVersion{Major: 1, Minor: 1}, Release: 2, PublishedAt: day(2), Change: coderules.ChangeMinor,
				Summaries: []string{"Count timeouts as attempts."}},
			{Version: coderules.RuleVersion{Major: 1}, Release: 1, PublishedAt: day(1), Change: coderules.ChangeNew,
				Summaries: []string{"Add the rule."}},
		},
	}
	return catalog{
		libraries: []views.LibraryCard{{
			Owner: "example", Name: "rules", Description: "Example rules for tests.",
			OwnerAvatarURL: exampleRules.OwnerAvatarURL, Rules: 2,
		}},
		pages: map[string]views.LibraryPage{"example/rules": {
			Library: exampleRules,
			Groups: []views.Group{
				{Path: "practices/testing", Canonical: testingGroup, Description: "Testing rules.", WhenToRead: "When the work involves testing.", Rules: 1},
				{Path: "techs/go", Canonical: goGroup, Description: "Go rules.", WhenToRead: "When the work involves go.", Rules: 1},
			},
			Rules: []views.RuleCard{
				{Path: retryLimits.Rule.Path, Group: "practices/testing", Title: retryLimits.Rule.Title, Impact: "HIGH", Version: retryLimits.Rule.Version},
				{Path: returnErrors.Rule.Path, Group: "techs/go", Title: returnErrors.Rule.Title, Impact: "HIGH", Version: returnErrors.Rule.Version},
			},
		}},
		rules: map[string]views.RulePage{
			"example/rules/" + returnErrors.Rule.Path: returnErrors,
			"example/rules/" + retryLimits.Rule.Path:  retryLimits,
		},
	}
}

// newSite returns the pages' handler, reading from c.
func newSite(t *testing.T, c web.Catalog) http.Handler {
	t.Helper()
	handler, err := web.New(c, web.Options{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

// get requests path from handler.
func get(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}

// visibleText returns the text a visitor reads in an HTML body, with whitespace collapsed.
func visibleText(t *testing.T, body string) string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "head" || n.Data == "script") {
			return
		}
		writeText(&text, n)
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return strings.Join(strings.Fields(text.String()), " ")
}

// writeText adds n's text to text, if it's a text node, separating it from the next node's. A <wbr> only lets a line
// break inside a word, so the text either side of it is one word.
func writeText(text *strings.Builder, n *html.Node) {
	switch {
	case n.Type == html.TextNode:
		text.WriteString(n.Data + " ")
	case n.Type == html.ElementNode && n.Data == "wbr":
		trimmed := strings.TrimSuffix(text.String(), " ")
		text.Reset()
		text.WriteString(trimmed)
	}
}

// assertShows fails unless the page's visible text includes each of want.
func assertShows(t *testing.T, page string, want ...string) {
	t.Helper()
	text := visibleText(t, page)
	for _, w := range want {
		if !strings.Contains(text, w) {
			t.Errorf("the page doesn't show %q:\n%s", w, text)
		}
	}
}

func TestHomeListsTheLibraries(t *testing.T) {
	handler := newSite(t, newCatalog())

	resp := get(t, handler, "/")

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d", resp.Code)
	}
	assertShows(t, resp.Body.String(), "Libraries", "rules Example rules for tests.", "example/rules 2 rules")
}

func TestLibraryPageShowsGroupsAndLatestRelease(t *testing.T) {
	handler := newSite(t, newCatalog())

	resp := get(t, handler, library)

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d", resp.Code)
	}
	assertShows(t, resp.Body.String(),
		"example / rules Example rules for tests.",
		"Groups 2", "All rules 2",
		"Technologies · 1 Go techs/go The Go language. 1 rule ›",
		"Practices · 1 Testing practices/testing What to test and how. 1 rule ›",
		"License MIT", "Latest library release release/3", "Updated 3 Sep 2026",
	)
	if !strings.Contains(resp.Body.String(), `href="https://github.com/example/rules/releases/tag/release/3"`) {
		t.Fatal("the latest release doesn't link its GitHub Release page")
	}
}

func TestLibraryRulesTabListsCurrentRulesByGroup(t *testing.T) {
	handler := newSite(t, newCatalog())

	resp := get(t, handler, library+"?tab=rules")

	assertShows(t, resp.Body.String(),
		"Go techs/go Return errors with context HIGH 2.0.0 techs/go/return-errors",
		"Testing practices/testing Verify retry limits HIGH 1.1.0 practices/testing/verify-retry-limits",
	)
}

func TestRulePageShowsTheCurrentVersion(t *testing.T) {
	handler := newSite(t, newCatalog())

	resp := get(t, handler, errorsRule)

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d", resp.Code)
	}
	page := resp.Body.String()
	assertShows(t, page,
		"rules › Go techs/go", "Return errors with context", "HIGH 2.0.0", "Rule Versions 2",
		"When to apply When changing return errors with context.", "Wrap every returned error.",
		"Updated 3 Sep 2026", "File return-errors.md",
	)
	if !strings.Contains(page, `href="https://github.com/example/rules/blob/release/3/techs/go/return-errors.md"`) {
		t.Fatal("the file doesn't link to the release that published the current version")
	}
}

// Reading guidance is Markdown, rendered at ingestion as the body is: the page shows its markup, and places that show
// text, such as the page's description and search results, show its text. Guidance a release stored without HTML
// shows as written.
func TestReadingGuidanceShowsAsRenderedMarkdownOrAsText(t *testing.T) {
	c := newBrowsingCatalog()
	key := "example/rules/techs/go/return-errors"
	page := c.rules[key]
	page.Rule.WhenToRead = "When JSX renders with `&&` and <b>a number</b>."
	page.Rule.WhenToReadHTML = "<p>When JSX renders with <code>&amp;&amp;</code> and &lt;b&gt;a number&lt;/b&gt;.</p>\n"
	c.rules[key] = page
	results := c.results["errors"]
	results.Results[0].WhenToRead, results.Results[0].WhenToReadHTML = page.Rule.WhenToRead, page.Rule.WhenToReadHTML
	c.results["errors"] = results
	handler := newSite(t, c)

	body := get(t, handler, errorsRule).Body.String()

	if !strings.Contains(body, "When to apply</b> <p>When JSX renders with <code>&amp;&amp;</code> and &lt;b&gt;a number&lt;/b&gt;.</p>") {
		t.Error("the page doesn't show the guidance's HTML")
	}
	if !strings.Contains(body, `<meta name="description" content="When JSX renders with &amp;&amp; and &lt;b&gt;a number&lt;/b&gt;.">`) {
		t.Error("the page's description isn't the guidance's text")
	}
	search := get(t, handler, "/search?q=errors").Body.String()
	assertShows(t, search, "When JSX renders with && and <b>a number</b>. example/rules")
	if strings.Contains(search, "`") || strings.Contains(search, "<code>&amp;") {
		t.Error("the search result shows the guidance's Markdown or markup")
	}

	page.Rule.WhenToReadHTML = ""
	c.rules[key] = page
	assertShows(t, get(t, newSite(t, c), errorsRule).Body.String(), "When to apply When JSX renders with `&&` and <b>a number</b>.")
}

func TestRuleVersionsTabListsEveryVersionNewestFirst(t *testing.T) {
	handler := newSite(t, newCatalog())

	resp := get(t, handler, errorsRule+"?tab=versions")

	page := resp.Body.String()
	assertShows(t, page,
		"2.0.0 Latest Major release/3 3 Sep 2026 Require context on every error. Add an example. Release notes "+
			"1.0.0 release/1 1 Sep 2026 Add the rule. Release notes",
	)
	if strings.Count(visibleText(t, page), "Major") != 2 { // The explanation, and the one major version.
		t.Fatal("a version that isn't major is marked Major")
	}
	for _, want := range []string{
		`href="https://github.com/example/rules/releases/tag/release/3"`,
		`href="https://github.com/example/rules/releases/tag/release/1"`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("no Release notes link %s", want)
		}
	}
}

func TestPagesAnswerNotFound(t *testing.T) {
	handler := newSite(t, newCatalog())
	for name, path := range map[string]string{
		"an unknown library":        "/example/missing",
		"an unknown library's rule": "/stranger/rules/techs/go/return-errors",
		"an unknown rule":           library + "/techs/go/missing",
		"a group":                   library + "/techs/go",
		"another path":              "/example",
	} {
		t.Run(name, func(t *testing.T) {
			resp := get(t, handler, path)

			if resp.Code != http.StatusNotFound {
				t.Fatalf("got %d", resp.Code)
			}
			assertShows(t, resp.Body.String(), "Not found")
		})
	}
}

func TestPagesRedirectToTheLibrarysSpelling(t *testing.T) {
	handler := newSite(t, newCatalog())

	resp := get(t, handler, "/Example/Rules/techs/go/return-errors?tab=versions")

	if resp.Code != http.StatusMovedPermanently || resp.Header().Get("Location") != errorsRule+"?tab=versions" {
		t.Fatalf("got %d to %q", resp.Code, resp.Header().Get("Location"))
	}
}

// canonicalLinks returns the href of every canonical link in an HTML body.
func canonicalLinks(t *testing.T, body string) []string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var hrefs []string
	for n := range doc.Descendants() {
		if n.Type == html.ElementNode && n.Data == "link" && attribute(n, "rel") == "canonical" {
			hrefs = append(hrefs, attribute(n, "href"))
		}
	}
	return hrefs
}

// The CDN's own domain serves the same pages as Rulemart's, so each page names its address on the base URL, and a
// tab, which shows the same page, adds nothing to it.
func TestPagesNameTheirAddressOnTheBaseURLAsCanonical(t *testing.T) {
	base, err := web.ParseBaseURL("https://rulemart.example")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := web.New(newCatalog(), web.Options{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), BaseURL: base})
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"/":                          "https://rulemart.example/",
		library + "?tab=rules":       "https://rulemart.example" + library,
		errorsRule + "?tab=versions": "https://rulemart.example" + errorsRule,
		// Percent-encoded spellings of the same page name the page's own address, not the request's spelling.
		"/%65xample/rules":                        "https://rulemart.example" + library,
		"/example/rules/techs/go/return-%65rrors": "https://rulemart.example" + errorsRule,
	} {
		t.Run(path, func(t *testing.T) {
			resp := get(t, handler, path)

			if got := canonicalLinks(t, resp.Body.String()); resp.Code != http.StatusOK || len(got) != 1 || got[0] != want {
				t.Fatalf("got %d with canonical links %q, want %q", resp.Code, got, want)
			}
		})
	}
	t.Run("a missing page", func(t *testing.T) {
		resp := get(t, handler, "/example/missing")

		if got := canonicalLinks(t, resp.Body.String()); resp.Code != http.StatusNotFound || len(got) != 0 {
			t.Fatalf("got %d with canonical links %q, want none", resp.Code, got)
		}
	})
}

func TestPagesNameNoCanonicalAddressWithoutABaseURL(t *testing.T) {
	resp := get(t, newSite(t, newCatalog()), library)

	if got := canonicalLinks(t, resp.Body.String()); len(got) != 0 {
		t.Fatalf("canonical links %q, want none", got)
	}
}

// A page's path is appended to the base URL as it is, so anything but a bare https origin would name wrong addresses.
func TestParseBaseURLAcceptsOnlyAnHTTPSOrigin(t *testing.T) {
	if got, err := web.ParseBaseURL("https://rulemart.example"); err != nil || got.String() != "https://rulemart.example" {
		t.Fatalf("got %v, %v", got, err)
	}
	if got, err := web.ParseBaseURL(""); err != nil || got != nil {
		t.Fatalf("empty text gave %v, %v, want none", got, err)
	}
	for _, text := range []string{
		"http://rulemart.example", "https://rulemart.example/", "https://rulemart.example/catalog",
		"https://rulemart.example?q", "https://rulemart.example#top", "https://user@rulemart.example", "https://",
		"rulemart.example", "https:rulemart.example",
	} {
		if got, err := web.ParseBaseURL(text); err == nil {
			t.Errorf("accepted %q as %v", text, got)
		}
	}
}

func TestNewRejectsABaseURLThatIsntAnOrigin(t *testing.T) {
	_, err := web.New(newCatalog(), web.Options{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), BaseURL: &url.URL{Scheme: "https", Host: "rulemart.example", Path: "/catalog"},
	})
	if err == nil {
		t.Fatal("accepted a base URL with a path")
	}
}

// attribute returns the value of n's attribute key, or "".
func attribute(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

// theme.js finds the footer's theme menu by these hooks, so a page without them would show no way to choose a theme.
func TestPagesOfferTheThemeMenuItsScriptDrives(t *testing.T) {
	handler := newSite(t, newCatalog())

	page := get(t, handler, "/").Body.String()

	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	control := find(doc, func(n *html.Node) bool { return attribute(n, "id") == "theme-control" })
	if control == nil || control.Data != "details" {
		t.Fatal("no theme menu")
	}
	if summary := find(control, func(n *html.Node) bool { return n.Data == "summary" }); summary == nil || attribute(summary, "aria-label") != "Color theme: System" {
		t.Fatal("the theme menu has no labeled button")
	}
	for _, theme := range []string{"light", "dark", "system"} {
		icon := find(control, func(n *html.Node) bool { return attribute(n, "data-theme-icon") == theme })
		choice := find(control, func(n *html.Node) bool { return attribute(n, "data-theme-choice") == theme })
		if icon == nil || choice == nil || choice.Data != "button" || attribute(choice, "aria-pressed") == "" {
			t.Errorf("the theme menu lacks the %s icon or choice", theme)
		}
	}
}

// find returns the first node under n, n included, that matches.
func find(n *html.Node, matches func(*html.Node) bool) *html.Node {
	if n.Type == html.ElementNode && matches(n) {
		return n
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if found := find(child, matches); found != nil {
			return found
		}
	}
	return nil
}

func TestPagesAreCacheableForAMinute(t *testing.T) {
	handler := newSite(t, newCatalog())
	for _, path := range []string{"/", library, retryRule, retryRule + "?tab=versions", "/example/missing"} {
		resp := get(t, handler, path)
		if got := resp.Header().Get("Cache-Control"); got != "public, max-age=60" {
			t.Errorf("%s: Cache-Control is %q", path, got)
		}
		if resp.Header().Get("Content-Security-Policy") == "" {
			t.Errorf("%s: no Content-Security-Policy", path)
		}
	}
}

func TestStaticFilesAreCachedForAYearUnderTheirVersion(t *testing.T) {
	handler := newSite(t, newCatalog())
	stylesheet := regexp.MustCompile(`href="(/_static/[0-9a-f]+/generated/app\.css)"`).FindStringSubmatch(get(t, handler, "/").Body.String())
	if stylesheet == nil {
		t.Fatal("the page links no stylesheet")
	}

	current := get(t, handler, stylesheet[1])
	stale := get(t, handler, "/_static/000000000000/generated/app.css")

	if current.Code != http.StatusOK || current.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" ||
		!strings.HasPrefix(current.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("the current stylesheet answered %d with %v", current.Code, current.Header())
	}
	if stale.Code != http.StatusOK || stale.Header().Get("Cache-Control") != "public, max-age=60" {
		t.Fatalf("another version's stylesheet answered %d with %v", stale.Code, stale.Header())
	}
}

const internalDetail = "AccessDeniedException: arn:aws:sts::123456789012:assumed-role/rulemart-web is not authorized to perform ssm:GetParameter on /rulemart/database-url"

// A catalog failure is logged with the request's route pattern, as the access log records it, and ID, and visitors
// get a fixed message that can't be cached.
func TestPagesLogFailuresAndKeepThemOutOfResponses(t *testing.T) {
	var logs bytes.Buffer
	handler, err := web.New(catalog{err: errors.New(internalDetail)}, web.Options{
		Log:       slog.New(slog.NewJSONHandler(&logs, nil)),
		RequestID: func(*http.Request) string { return "request-123" },
	})
	if err != nil {
		t.Fatal(err)
	}

	resp := get(t, handler, library)

	if resp.Code != http.StatusServiceUnavailable || resp.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("got %d with Cache-Control %q", resp.Code, resp.Header().Get("Cache-Control"))
	}
	for _, leak := range []string{"arn:", "AccessDenied", "GetParameter", "/rulemart/"} {
		if strings.Contains(resp.Body.String(), leak) {
			t.Fatalf("the response exposes %q", leak)
		}
	}
	assertShows(t, resp.Body.String(), "Rulemart can't show this page right now.")
	for _, want := range []string{internalDetail, `"route":"/{owner}/{repo}"`, `"requestID":"request-123"`} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("the logs %s don't include %s", logs.String(), want)
		}
	}
	if strings.Contains(logs.String(), library) {
		t.Fatalf("the logs %s hold the path", logs.String())
	}
}

// Hovering an impact label explains what the level means, in Code Rules' terms: how serious the problem is that
// the rule helps prevent, not how much code applying it changes.
func TestImpactLabelsExplainTheirLevel(t *testing.T) {
	handler := newSite(t, newCatalog())
	const high = `title="High impact: this rule helps prevent substantial correctness, reliability, or maintainability problems."`

	for _, path := range []string{library + "?tab=rules", errorsRule} {
		if page := get(t, handler, path).Body.String(); !strings.Contains(page, high) {
			t.Errorf("%s: the HIGH label has no explanation", path)
		}
	}
}

const (
	mixed          = "/example/mixed"
	passContext    = mixed + "/techs/golang/pass-context-first"
	returnErrorsGo = mixed + "/techs/go/return-errors"
)

// newMixedCatalog returns a catalog holding the library example/mixed, whose groups are canonical with an icon
// (techs/go and practices/testing), canonical without one (techs/goose), and not canonical (techs/golang).
func newMixedCatalog() catalog {
	lib := views.Library{Owner: "example", Name: "mixed", LatestRelease: 1, LatestTaggedAt: day(1)}
	goose := &views.CanonicalGroup{Name: "Goose"}
	rule := func(path, group string, canonical *views.CanonicalGroup, title string) views.RulePage {
		return views.RulePage{Library: lib, Rule: views.Rule{
			Path: path, Group: group, CanonicalGroup: canonical, Title: title, Impact: "HIGH", WhenToRead: "When " + title + ".",
			Version: coderules.RuleVersion{Major: 1}, Release: 1, PublishedAt: day(1),
		}}
	}
	rules := []views.RulePage{
		rule("practices/testing/verify-retry-limits", "practices/testing", testingGroup, "Verify retry limits"),
		rule("techs/go/return-errors", "techs/go", goGroup, "Return errors"),
		rule("techs/golang/pass-context-first", "techs/golang", nil, "Pass context first"),
		rule("techs/goose/one-change-per-migration", "techs/goose", goose, "One change per migration"),
	}
	page := views.LibraryPage{Library: lib, Groups: []views.Group{
		{Path: "practices/testing", Canonical: testingGroup, WhenToRead: "When testing.", Rules: 1},
		{Path: "techs/go", Canonical: goGroup, Rules: 1},
		{Path: "techs/golang", Description: "More Go rules.", WhenToRead: "When writing Go.", Rules: 1},
		{Path: "techs/goose", Canonical: goose, Rules: 1},
	}}
	c := catalog{pages: map[string]views.LibraryPage{}, rules: map[string]views.RulePage{}}
	for _, r := range rules {
		page.Rules = append(page.Rules, views.RuleCard{Path: r.Rule.Path, Group: r.Rule.Group, Title: r.Rule.Title, Impact: "HIGH", Version: r.Rule.Version})
		c.rules["example/mixed/"+r.Rule.Path] = r
	}
	c.pages["example/mixed"] = page
	return c
}

// notCanonicalFlags returns the elements that flag a group as not canonical, by their visible text.
func notCanonicalFlags(t *testing.T, page string) []*html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	var flags []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.FirstChild != nil && n.FirstChild == n.LastChild &&
			n.FirstChild.Type == html.TextNode && strings.TrimSpace(n.FirstChild.Data) == "not canonical" {
			flags = append(flags, n)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return flags
}

// assertFlagsExplainThemselves fails unless page flags exactly want groups as not canonical, each with an
// explanation on hover.
func assertFlagsExplainThemselves(t *testing.T, page string, want int) {
	t.Helper()
	flags := notCanonicalFlags(t, page)
	if len(flags) != want {
		t.Fatalf("%d groups are flagged as not canonical, want %d", len(flags), want)
	}
	for _, flag := range flags {
		if !strings.Contains(attribute(flag, "title"), "canonical group list") {
			t.Errorf("the flag doesn't explain itself: title=%q", attribute(flag, "title"))
		}
	}
}

// A canonical group goes by the list's name, with its icon when Rulemart has one, and any other group by its ID,
// flagged, so a library can't pass off its own group as one that every library shares.
func TestLibraryPageShowsCanonicalGroupsByNameAndOtherGroupsByIDFlagged(t *testing.T) {
	handler := newSite(t, newMixedCatalog())

	page := get(t, handler, mixed).Body.String()

	assertShows(t, page,
		// A canonical group is described by the canonical list, as the groups page describes it, and any other group by
		// its library.
		"Technologies · 3 Go techs/go The Go language. 1 rule ›",
		"techs/golang not canonical More Go rules. 1 rule ›",
		"Goose techs/goose 1 rule ›",
		"Practices · 1 Testing practices/testing What to test and how. 1 rule ›",
	)
	if text := visibleText(t, page); strings.Contains(text, "When testing.") || strings.Contains(text, "When writing Go.") {
		t.Error("the page shows a group's reading guidance")
	}
	assertFlagsExplainThemselves(t, page, 1)
	icons := regexp.MustCompile(`<img[^>]* src="(/_static/[^"]+)"`).FindAllStringSubmatch(page, -1)
	if len(icons) != 2 || !strings.HasSuffix(icons[0][1], "/icons/devicon/go-original.svg") ||
		!strings.HasSuffix(icons[1][1], "/icons/lucide/flask-conical.svg") {
		t.Fatalf("the group icons are %q, want Go's and then Testing's", icons)
	}
	for _, icon := range icons {
		resp := get(t, handler, icon[1])
		if resp.Code != http.StatusOK || resp.Header().Get("Content-Type") != "image/svg+xml" {
			t.Errorf("%s answered %d with %q", icon[1], resp.Code, resp.Header().Get("Content-Type"))
		}
	}
}

func TestLibraryRulesTabShowsCanonicalGroupsByNameAndOtherGroupsByIDFlagged(t *testing.T) {
	handler := newSite(t, newMixedCatalog())

	page := get(t, handler, mixed+"?tab=rules").Body.String()

	assertShows(t, page,
		"Go techs/go Return errors HIGH 1.0.0 techs/go/return-errors",
		"techs/golang not canonical Pass context first HIGH 1.0.0 techs/golang/pass-context-first",
		"Goose techs/goose One change per migration",
		"Testing practices/testing Verify retry limits",
	)
	assertFlagsExplainThemselves(t, page, 1)
}

func TestRulePageNamesItsGroupAsTheLibraryPageDoes(t *testing.T) {
	handler := newSite(t, newMixedCatalog())

	canonical := get(t, handler, returnErrorsGo).Body.String()
	other := get(t, handler, passContext).Body.String()

	assertShows(t, canonical, "mixed › Go techs/go Go rules in all libraries ›")
	assertFlagsExplainThemselves(t, canonical, 0)
	assertShows(t, other, "mixed › techs/golang not canonical")
	assertFlagsExplainThemselves(t, other, 1)
	// A canonical group's rules in every library are a page of their own; any other group stands alone.
	if got := links(t, canonical, "rules in all libraries"); !slices.Equal(got, []string{"/groups/techs/go"}) {
		t.Errorf("the rule page links %q across libraries", got)
	}
	if got := links(t, other, "rules in all libraries"); len(got) != 0 {
		t.Errorf("a group that isn't canonical links %q across libraries", got)
	}
}

// A library's canonical groups lead to their rules in the library, and to their page across libraries.
func TestLibraryPageLinksCanonicalGroupsAcrossLibraries(t *testing.T) {
	page := get(t, newSite(t, newMixedCatalog()), mixed).Body.String()

	if got := links(t, page, "All libraries"); !slices.Equal(got, []string{"/groups/techs/go", "/groups/techs/goose", "/groups/practices/testing"}) {
		t.Errorf("the groups link %q across libraries", got)
	}
	if got := links(t, page, "techs/golang"); !slices.Equal(got, []string{mixed + "?tab=rules#group-techs-golang"}) {
		t.Errorf("techs/golang links %q", got)
	}
}
