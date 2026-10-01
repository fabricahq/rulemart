package site_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"golang.org/x/net/html"

	"github.com/fabricahq/rulemart/catalog"
	"github.com/fabricahq/rulemart/internal/database"
	"github.com/fabricahq/rulemart/internal/ingest"
	"github.com/fabricahq/rulemart/internal/ingest/ingesttest"
	"github.com/fabricahq/rulemart/internal/site"
)

// vetted lists the library with GitHub repository ID vettedID.
var vetted = []catalog.Library{{Host: catalog.GitHub, RepositoryID: "7"}}

const (
	vettedID   = 7
	unvettedID = 8
	library    = "/example/rules"
	retryRule  = library + "/practices/testing/verify-retry-limits"
	errorsRule = library + "/techs/go/return-errors"
)

// hostileHTML is raw HTML in the current version of a rule, which a library could publish to attack visitors.
const hostileHTML = "<script>alert(1)</script>\n\nPress <img src=x onerror=alert(2)> to <a href=\"javascript:alert(3)\">continue</a>."

// newSite ingests a vetted library with three releases, and an unvetted one, and returns the pages' handler.
func newSite(t *testing.T) http.Handler {
	t.Helper()
	db, connString := ingesttest.NewDatabase(t)
	store := ingest.NewStore(db)

	lib := ingesttest.NewLibrary(t)
	lib.Group("practices/testing", "Testing")
	lib.Group("techs/go", "Go")
	lib.Group("practices/legacy", "Legacy")
	lib.Rule("practices/legacy/old-habit", "Old habit", "A rule a later release retires with its group.")
	lib.Rule("practices/testing/verify-retry-limits", "Verify retry limits", "Stop after a fixed number of attempts.")
	lib.Rule("practices/testing/check-retry-backoff", "Check retry backoff", "Wait longer after each attempt.")
	lib.Rule("techs/go/return-errors", "Return errors", "Return errors instead of panicking.")
	lib.Release(1, `formatVersion: 1
release: 1
rules:
  practices/legacy/old-habit: 1.0.0
  practices/testing/check-retry-backoff: 1.0.0
  practices/testing/verify-retry-limits: 1.0.0
  techs/go/return-errors: 1.0.0
changes:
  practices/legacy/old-habit: {change: new, summaries: [Add the rule.]}
  practices/testing/check-retry-backoff: {change: new, summaries: [Add the rule.]}
  practices/testing/verify-retry-limits: {change: new, summaries: [Add the rule.]}
  techs/go/return-errors: {change: new, summaries: [Add the rule.]}
`)
	lib.Rule("practices/testing/verify-retry-limits", "Verify retry limits", "Stop after a fixed number of attempts, timeouts included.")
	lib.Release(2, `formatVersion: 1
release: 2
rules:
  practices/legacy/old-habit: 1.0.0
  practices/testing/check-retry-backoff: 1.0.0
  practices/testing/verify-retry-limits: 1.1.0
  techs/go/return-errors: 1.0.0
changes:
  practices/testing/verify-retry-limits: {change: minor, from: 1.0.0, summaries: [Count timeouts as attempts.]}
`)
	lib.Rule("techs/go/return-errors", "Return errors with context", "Wrap every returned error.\n\n"+hostileHTML)
	lib.Remove("practices/testing/check-retry-backoff.md")
	lib.Remove("practices/legacy/old-habit.md")
	lib.Remove("practices/legacy/_group.yaml")
	lib.Release(3, `formatVersion: 1
release: 3
rules:
  practices/testing/verify-retry-limits: 1.1.0
  techs/go/return-errors: 2.0.0
changes:
  techs/go/return-errors: {change: major, from: 1.0.0, summaries: [Require context on every error., Add an example.]}
retired:
  practices/legacy/old-habit: {lastVersion: 1.0.0, summaries: [Drop the legacy group.]}
  practices/testing/check-retry-backoff: {lastVersion: 1.0.0, summaries: [Merge into verify-retry-limits.]}
`)
	if _, err := ingest.Ingest(context.Background(), store, lib.Repository(vettedID)); err != nil {
		t.Fatal(err)
	}
	unvetted := lib.Repository(unvettedID)
	unvetted.Owner, unvetted.Name = "stranger", "unvetted-rules"
	if _, err := ingest.Ingest(context.Background(), store, unvetted); err != nil {
		t.Fatal(err)
	}

	// Pages read as the web function's role, so a table the migrations don't grant it fails these tests.
	web := ingesttest.NewWebDatabase(t, connString)
	handler, err := site.New(site.NewStore(web, vetted), site.Options{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
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
		if n.Type == html.TextNode {
			text.WriteString(n.Data + " ")
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return strings.Join(strings.Fields(text.String()), " ")
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

func TestHomeListsOnlyVettedLibraries(t *testing.T) {
	handler := newSite(t)

	resp := get(t, handler, "/")

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d", resp.Code)
	}
	assertShows(t, resp.Body.String(), "Libraries", "rules Example rules for tests.", "example/rules · 2 rules")
	if strings.Contains(resp.Body.String(), "unvetted-rules") {
		t.Fatal("the home page lists an unvetted library")
	}
}

func TestLibraryPageShowsGroupsAndLatestRelease(t *testing.T) {
	handler := newSite(t)

	resp := get(t, handler, library)

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d", resp.Code)
	}
	assertShows(t, resp.Body.String(),
		"Groups 2", "All rules 2",
		"Technologies · 1 Go techs/go 1 rule ›",
		"Practices · 1 Testing practices/testing When the work involves testing. 1 rule ›",
		"License MIT", "Latest library release release/3", "Updated 3 Sep 2026",
	)
	if !strings.Contains(resp.Body.String(), `href="https://github.com/example/rules/releases/tag/release/3"`) {
		t.Fatal("the latest release doesn't link its GitHub Release page")
	}
	// The catalog keeps a group whose rules are all retired, but the page lists only groups with current rules.
	for _, tab := range []string{library, library + "?tab=rules"} {
		if page := get(t, handler, tab).Body.String(); strings.Contains(page, "practices/legacy") {
			t.Errorf("%s lists the retired group practices/legacy", tab)
		}
	}
}

func TestLibraryRulesTabListsCurrentRulesByGroup(t *testing.T) {
	handler := newSite(t)

	resp := get(t, handler, library+"?tab=rules")

	assertShows(t, resp.Body.String(),
		"Go techs/go Return errors with context HIGH 2.0.0 techs/go/return-errors",
		"Testing practices/testing Verify retry limits HIGH 1.1.0 practices/testing/verify-retry-limits",
	)
	if strings.Contains(resp.Body.String(), "Check retry backoff") {
		t.Fatal("the rules tab lists a retired rule")
	}
}

func TestRulePageShowsTheCurrentVersion(t *testing.T) {
	handler := newSite(t)

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

func TestRuleVersionsTabListsEveryVersionNewestFirst(t *testing.T) {
	handler := newSite(t)

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
	handler := newSite(t)
	for name, path := range map[string]string{
		"an unknown library":         "/example/missing",
		"an unvetted library":        "/stranger/unvetted-rules",
		"an unvetted library's rule": "/stranger/unvetted-rules/techs/go/return-errors",
		"an unknown rule":            library + "/techs/go/missing",
		"a retired rule":             library + "/practices/testing/check-retry-backoff",
		"a group":                    library + "/techs/go",
		"another path":               "/example",
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
	handler := newSite(t)

	resp := get(t, handler, "/Example/Rules/techs/go/return-errors?tab=versions")

	if resp.Code != http.StatusMovedPermanently || resp.Header().Get("Location") != errorsRule+"?tab=versions" {
		t.Fatalf("got %d to %q", resp.Code, resp.Header().Get("Location"))
	}
}

// A rule's raw HTML must reach visitors as text: the page shows it escaped, and runs or loads nothing from it.
func TestRulePageShowsRawHTMLAsText(t *testing.T) {
	handler := newSite(t)

	resp := get(t, handler, errorsRule)

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d", resp.Code)
	}
	assertShows(t, resp.Body.String(), "<script>alert(1)</script>", "Press <img src=x onerror=alert(2)> to")
	doc, err := html.Parse(strings.NewReader(resp.Body.String()))
	if err != nil {
		t.Fatal(err)
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, attr := range n.Attr {
				if strings.HasPrefix(attr.Key, "on") || strings.Contains(attr.Val, "javascript:") {
					t.Errorf("<%s> has %s=%q", n.Data, attr.Key, attr.Val)
				}
			}
			if n.Data == "script" && !strings.HasPrefix(attribute(n, "src"), "/_static/") {
				t.Errorf("a script that isn't Rulemart's own: src=%q", attribute(n, "src"))
			}
			if n.Data == "img" && !strings.HasPrefix(attribute(n, "src"), "https://") {
				t.Errorf("an image from the rule: src=%q", attribute(n, "src"))
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
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
	handler := newSite(t)

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
	handler := newSite(t)
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
	handler := newSite(t)
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

// failingParameter stands in for an SSM parameter that can't be read, with an error naming internal details.
type failingParameter struct{}

const internalDetail = "AccessDeniedException: arn:aws:sts::123456789012:assumed-role/rulemart-web is not authorized to perform ssm:GetParameter on /rulemart/database-url"

func (failingParameter) GetParameter(context.Context, *ssm.GetParameterInput, ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	return nil, errors.New(internalDetail)
}

// A database failure is logged with the request's route and ID, and visitors get a fixed message that can't be
// cached.
func TestPagesLogFailuresAndKeepThemOutOfResponses(t *testing.T) {
	var logs bytes.Buffer
	db := database.New(failingParameter{}, "/rulemart/database-url", 1)
	handler, err := site.New(site.NewStore(db, vetted), site.Options{
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
	for _, want := range []string{internalDetail, `"route":"/example/rules"`, `"requestID":"request-123"`} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("the logs %s don't include %s", logs.String(), want)
		}
	}
}

// Hovering an impact label explains what the level means, in Code Rules' terms: how serious the problem is that
// the rule helps prevent, not how much code applying it changes.
func TestImpactLabelsExplainTheirLevel(t *testing.T) {
	handler := newSite(t)
	const high = `title="High impact: this rule helps prevent substantial correctness, reliability, or maintainability problems."`

	for _, path := range []string{library + "?tab=rules", errorsRule} {
		if page := get(t, handler, path).Body.String(); !strings.Contains(page, high) {
			t.Errorf("%s: the HIGH label has no explanation", path)
		}
	}
}
