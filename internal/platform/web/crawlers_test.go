package web_test

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/platform/web"
)

// baseURL returns the public origin tests serve pages at.
func baseURL(t *testing.T) web.Options {
	t.Helper()
	base, err := web.ParseBaseURL("https://rulemart.example")
	if err != nil {
		t.Fatal(err)
	}
	return web.Options{BaseURL: base}
}

// robots.txt keeps crawlers out of what's a visitor's own, what each query or pair of versions would multiply, and the
// unvetted area, and names the sitemap on the public origin.
func TestRobotsKeepCrawlersOutOfPrivateAndEndlessPages(t *testing.T) {
	resp := get(t, newSiteWith(t, baseURL(t)), "/robots.txt")

	if resp.Code != http.StatusOK || resp.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("answered %d, %q", resp.Code, resp.Header().Get("Content-Type"))
	}
	lines := strings.Split(resp.Body.String(), "\n")
	for _, want := range []string{"User-agent: *", "Sitemap: https://rulemart.example/sitemap.xml"} {
		if !slices.Contains(lines, want) {
			t.Errorf("robots.txt lacks %q:\n%s", want, resp.Body)
		}
	}
	var rules []string
	for _, line := range lines {
		if rule, ok := strings.CutPrefix(line, "Disallow: "); ok {
			rules = append(rules, rule)
		}
	}
	for path, want := range map[string]bool{
		"/me": true, "/account?x=1": true, "/me/listings": true, "/me?tab=stars?x=1": true, "/signin": true, "/signin?return=%2F": true,
		"/list": true, "/list?repository=a%2Fb": true, "/search": true, "/search?q=retry": true, "/unvetted": true,
		"/example/rules?tab=releases&from=1&to=3": true, "/example/rules/techs/go/x?tab=versions&from=1.0.0&to=2.0.0": true,
		// Pages crawlers may read, among them libraries whose owners' names start like a disallowed page's.
		"/": false, "/libraries": false, "/g/techs/go": false, "/browse/techs": false, "/faq": false, "/example/rules": false, "/example/rules?tab=releases": false,
		"/about": false, "/privacy": false, "/listr/rules": false, "/searchkit/rules": false, "/unvetted-fan/rules": false,
		"/signin-kit/rules": false, "/accountant/rules": false,
	} {
		if got := disallows(rules, path); got != want {
			t.Errorf("robots.txt disallows %s: %v, want %v", path, got, want)
		}
	}
}

// disallows reports whether any of rules, robots.txt Disallow values, matches path, as Google and Bing read them: a
// prefix of the path and query, where * matches any characters and a final $ the end.
func disallows(rules []string, path string) bool {
	for _, rule := range rules {
		pattern := "^" + strings.ReplaceAll(regexp.QuoteMeta(strings.TrimSuffix(rule, "$")), `\*`, ".*")
		if strings.HasSuffix(rule, "$") {
			pattern += "$"
		}
		if regexp.MustCompile(pattern).MatchString(path) {
			return true
		}
	}
	return false
}

// Without a public origin, as locally, robots.txt names no sitemap, and there's none: its addresses must be absolute.
func TestWithoutABaseURLThereIsNoSitemap(t *testing.T) {
	handler := newSiteWith(t, web.Options{})

	if body := get(t, handler, "/robots.txt").Body.String(); strings.Contains(body, "Sitemap:") {
		t.Errorf("robots.txt names a sitemap:\n%s", body)
	}
	if resp := get(t, handler, "/sitemap.xml"); resp.Code != http.StatusNotFound {
		t.Errorf("/sitemap.xml answered %d", resp.Code)
	}
}

// urlset is a sitemap file as the protocol defines it.
type urlset struct {
	XMLName xml.Name `xml:"http://www.sitemaps.org/schemas/sitemap/0.9 urlset"`
	URLs    []struct {
		Loc     string `xml:"loc"`
		LastMod string `xml:"lastmod"`
	} `xml:"url"`
}

// The sitemap lists the site's own pages, the groups' pages, and each vetted library and current rule, by
// their canonical addresses on the public origin, each library and rule with when it last changed.
func TestSitemapListsEveryIndexablePageByItsCanonicalAddress(t *testing.T) {
	c := newBrowsingCatalog()
	c.sitemap = views.Sitemap{
		Libraries: []views.SitemapLibrary{{
			Owner: "example", Name: "rules", Updated: day(3),
			Rules: []views.SitemapRule{{Path: "practices/testing/verify-retry-limits", Updated: day(2)}, {Path: "techs/go/return-errors", Updated: day(3)}},
		}, {Owner: "faq", Name: "go.rules", Updated: day(4)}},
		Groups: []string{"practices/testing", "techs/go"},
	}
	options := baseURL(t)
	options.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	handler, err := web.New(c, options)
	if err != nil {
		t.Fatal(err)
	}

	resp := get(t, handler, "/sitemap.xml")

	if resp.Code != http.StatusOK || resp.Header().Get("Content-Type") != "application/xml; charset=utf-8" ||
		resp.Header().Get("Cache-Control") != "public, max-age=0, s-maxage=60" {
		t.Fatalf("answered %d with %v", resp.Code, resp.Header())
	}
	if !bytes.HasPrefix(resp.Body.Bytes(), []byte(xml.Header)) {
		t.Errorf("the sitemap has no XML declaration")
	}
	var got urlset
	if err := xml.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	var locs []string
	lastMod := map[string]string{}
	for _, u := range got.URLs {
		locs = append(locs, u.Loc)
		lastMod[u.Loc] = u.LastMod
	}
	want := []string{
		"https://rulemart.example/",
		"https://rulemart.example/libraries",
		"https://rulemart.example/browse/techs",
		"https://rulemart.example/browse/techs/other",
		"https://rulemart.example/browse/practices",
		"https://rulemart.example/browse/practices/other",
		"https://rulemart.example/about",
		"https://rulemart.example/privacy",
		"https://rulemart.example/faq",
		"https://rulemart.example/feedback",
		"https://rulemart.example/g/practices/testing",
		"https://rulemart.example/g/techs/go",
		"https://rulemart.example/example",
		"https://rulemart.example/o/faq",
		"https://rulemart.example/example/rules",
		"https://rulemart.example/example/rules/practices/testing/verify-retry-limits",
		"https://rulemart.example/example/rules/techs/go/return-errors",
		"https://rulemart.example/faq/go.rules",
	}
	if !slices.Equal(locs, want) {
		t.Fatalf("lists\n%s\nwant\n%s", strings.Join(locs, "\n"), strings.Join(want, "\n"))
	}
	for loc, date := range map[string]string{
		"https://rulemart.example/example/rules":                                       "2026-09-03",
		"https://rulemart.example/example/rules/practices/testing/verify-retry-limits": "2026-09-02",
		"https://rulemart.example/faq/go.rules":                                        "2026-09-04",
		"https://rulemart.example/example":                                             "",
		"https://rulemart.example/browse/techs":                                        "",
	} {
		if lastMod[loc] != date {
			t.Errorf("%s: lastmod %q, want %q", loc, lastMod[loc], date)
		}
	}
}

// A library whose page's address is one of the site's own pages, such as browse/techs, a browse page, or o/rules, an
// owner's page under /o/, has no page to list, so the sitemap leaves it out; its rules' pages, under it, stay.
func TestSitemapLeavesOutLibraryPagesTheSiteTakes(t *testing.T) {
	c := newBrowsingCatalog()
	rule := []views.SitemapRule{{Path: "techs/go/return-errors", Updated: day(3)}}
	c.sitemap = views.Sitemap{Libraries: []views.SitemapLibrary{
		{Owner: "browse", Name: "Practices", Updated: day(3), Rules: rule},
		{Owner: "browse", Name: "rules", Updated: day(3)},
		{Owner: "browse", Name: "techs", Updated: day(3), Rules: rule},
		{Owner: "g", Name: "techs", Updated: day(3)},
		{Owner: "o", Name: "rules", Updated: day(3), Rules: rule},
	}}
	options := baseURL(t)
	options.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	handler, err := web.New(c, options)
	if err != nil {
		t.Fatal(err)
	}

	var got urlset
	if err := xml.Unmarshal(get(t, handler, "/sitemap.xml").Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	var listed []string
	for _, u := range got.URLs {
		listed = append(listed, strings.TrimPrefix(u.Loc, "https://rulemart.example"))
	}
	// After the site's own pages and the groups', the owners' pages, then the libraries' and their rules'.
	listed = listed[slices.Index(listed, "/o/browse"):]
	want := []string{
		"/o/browse", "/o/g", "/o/o",
		"/browse/Practices/techs/go/return-errors",
		"/browse/rules",
		"/browse/techs/techs/go/return-errors",
		"/g/techs",
		"/o/rules/techs/go/return-errors",
	}
	if !slices.Equal(listed, want) {
		t.Errorf("lists\n%s\nwant\n%s", strings.Join(listed, "\n"), strings.Join(want, "\n"))
	}
}

// A sitemap that leaves out rules past the most it lists says so in the log, so the operator knows to split it.
func TestTruncatedSitemapIsLogged(t *testing.T) {
	c := newBrowsingCatalog()
	c.sitemap = views.Sitemap{Libraries: []views.SitemapLibrary{{Owner: "example", Name: "rules", Updated: day(3)}}, Truncated: true}
	var logs bytes.Buffer
	options := baseURL(t)
	options.Log = slog.New(slog.NewJSONHandler(&logs, nil))
	handler, err := web.New(c, options)
	if err != nil {
		t.Fatal(err)
	}

	if resp := get(t, handler, "/sitemap.xml"); resp.Code != http.StatusOK {
		t.Fatalf("answered %d", resp.Code)
	}
	if !strings.Contains(logs.String(), `"msg":"sitemap truncated"`) {
		t.Errorf("logged %s", logs.String())
	}
}

// A failed read fails the sitemap as it fails a page, uncached, so CloudFront doesn't keep a sitemap missing pages.
func TestSitemapFailsUncachedWhenTheCatalogFails(t *testing.T) {
	c := newBrowsingCatalog()
	c.err = io.ErrUnexpectedEOF
	options := baseURL(t)
	options.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	handler, err := web.New(c, options)
	if err != nil {
		t.Fatal(err)
	}

	resp := get(t, handler, "/sitemap.xml")

	if resp.Code != http.StatusServiceUnavailable || resp.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("answered %d, %q", resp.Code, resp.Header().Get("Cache-Control"))
	}
}

// A sitemap whose rules would pass the most a response can hold stops before it, still valid, and says so in the
// log: a Lambda function's response holds at most 6 MB, and 45,000 long rule addresses take more.
func TestSitemapStaysWithinAResponsesSize(t *testing.T) {
	c := newBrowsingCatalog()
	lib := views.SitemapLibrary{Owner: "example", Name: "rules", Updated: day(3)}
	for i := range 45_000 {
		lib.Rules = append(lib.Rules, views.SitemapRule{
			Path: fmt.Sprintf("practices/testing/%s-%05d", strings.Repeat("keep-tests-independent-", 4), i), Updated: day(2),
		})
	}
	c.sitemap = views.Sitemap{Libraries: []views.SitemapLibrary{lib}}
	var logs bytes.Buffer
	options := baseURL(t)
	options.Log = slog.New(slog.NewJSONHandler(&logs, nil))
	handler, err := web.New(c, options)
	if err != nil {
		t.Fatal(err)
	}

	resp := get(t, handler, "/sitemap.xml")

	if resp.Code != http.StatusOK || resp.Body.Len() > 5<<20 {
		t.Fatalf("answered %d with %d bytes", resp.Code, resp.Body.Len())
	}
	var got urlset
	if err := xml.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if n := len(got.URLs); n < 20_000 || n >= 45_006 {
		t.Errorf("lists %d addresses", n)
	}
	if !strings.Contains(logs.String(), `"msg":"sitemap truncated"`) {
		t.Errorf("logged %s", logs.String())
	}
}

func TestSitemapNamesAnOwnerOnceWhateverCaseTheirLibrariesSpell(t *testing.T) {
	c := newBrowsingCatalog()
	c.sitemap = views.Sitemap{Libraries: []views.SitemapLibrary{
		{Owner: "Acme", Name: "a-rules", Updated: day(3)},
		{Owner: "acme", Name: "b-rules", Updated: day(3)},
		{Owner: "zed", Name: "rules", Updated: day(3)},
	}}
	options := baseURL(t)
	options.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	handler, err := web.New(c, options)
	if err != nil {
		t.Fatal(err)
	}

	var got urlset
	if err := xml.Unmarshal(get(t, handler, "/sitemap.xml").Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	var owners []string
	for _, u := range got.URLs {
		if path := strings.TrimPrefix(u.Loc, "https://rulemart.example"); strings.Count(path, "/") == 1 && (path == "/Acme" || path == "/acme" || path == "/zed") {
			owners = append(owners, path)
		}
	}
	if want := []string{"/Acme", "/zed"}; !slices.Equal(owners, want) {
		t.Errorf("owner pages %v, want %v: one page per owner, spelled as the first library spells it", owners, want)
	}
}
