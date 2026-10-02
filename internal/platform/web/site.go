// Package web serves Rulemart's pages: the vetted libraries, each library's groups, rules, and releases, each rule's
// current version and version history, comparisons of two releases or two rule versions, the groups across libraries,
// each canonical group's rules in every library, and search.
// It reads them from the catalog's page reads, which app.Pages implements.
package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// maxPageBytes bounds a page Rulemart sends: a Lambda function's response holds at most 6 MB. Pages bound what they
// show from a library, so only a library far past any Rulemart knows reaches it; such a page says it's too large.
const maxPageBytes = 5 << 20

// pageCache lets CloudFront and browsers keep a page for a minute, so a new library release shows within a minute
// of ingestion without every visit reaching the function.
const pageCache = "public, max-age=60"

// contentSecurityPolicy allows only Rulemart's own files, plus images from GitHub's avatar and raw file hosts,
// which rules and library owners use, and forms that submit to Rulemart, such as search. Rule content comes from
// repositories Rulemart doesn't control, so nothing else may load or run.
const contentSecurityPolicy = "default-src 'none'; script-src 'self'; style-src 'self'; font-src 'self'; " +
	"img-src 'self' https://avatars.githubusercontent.com https://raw.githubusercontent.com; " +
	"base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

// Options configures the handler.
type Options struct {
	// Log receives one line for each request, and the details of failures that pages leave out.
	Log *slog.Logger
	// RequestID returns an identifier for a request that the logs record, such as the Lambda request ID. It may
	// be nil.
	RequestID func(*http.Request) string
	// BaseURL is Rulemart's public origin, such as https://rulemart.fabricahq.com, which each page names as its
	// canonical address, so search engines index that one address for it whichever host served it. It has no path.
	// Nil names none. ParseBaseURL makes one from text.
	BaseURL *url.URL
}

// ParseBaseURL parses text as Options.BaseURL: an https origin with no path, query, or fragment, such as
// https://rulemart.fabricahq.com. Empty text gives nil.
func ParseBaseURL(text string) (*url.URL, error) {
	if text == "" {
		return nil, nil
	}
	u, err := url.Parse(text)
	if err == nil {
		err = checkBaseURL(u)
	}
	if err != nil {
		return nil, fmt.Errorf("parse base URL %q: %v", text, err)
	}
	return u, nil
}

// checkBaseURL reports whether u is an https origin and nothing more, so a page's path appends to it as is.
func checkBaseURL(u *url.URL) error {
	if u.Scheme != "https" || u.Host == "" || u.Opaque != "" || u.User != nil || u.Path != "" || u.RawPath != "" ||
		u.ForceQuery || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("want an https origin with no path, query, or fragment, such as https://rulemart.example")
	}
	return nil
}

// Catalog reads what the pages show. app.Pages implements it, finding only the vetted libraries.
type Catalog interface {
	HomePage(ctx context.Context) (views.HomePage, error)
	Libraries(ctx context.Context) ([]views.LibraryCard, error)
	// LibraryPage, ReleasesPage, and RulePage fail with app.ErrNotFound when there's no such library or rule.
	LibraryPage(ctx context.Context, owner, name string) (views.LibraryPage, error)
	// ReleasesPage returns the page of releases that holds release, or the first page when release is 0, and fails
	// with app.ErrNotFound when there's no such release either.
	ReleasesPage(ctx context.Context, owner, name string, release int) (views.ReleasesPage, error)
	RulePage(ctx context.Context, owner, name, rulePath string) (views.RulePage, error)
	// ReleaseComparison and RuleComparison put the older release or version first, and fail with app.ErrNotFound
	// when there's no such library, rule, release, or version.
	ReleaseComparison(ctx context.Context, owner, name string, from, to int) (views.ReleaseComparison, error)
	RuleComparison(ctx context.Context, owner, name, rulePath string, from, to coderules.RuleVersion) (views.RuleComparison, error)
	GroupIndex(ctx context.Context) (views.GroupIndex, error)
	// GroupPage fails with app.ErrNotFound when id isn't a canonical group's.
	GroupPage(ctx context.Context, id string) (views.GroupPage, error)
	// Search returns page, from 1 to app.MaxSearchPage, of what query finds. It fails with
	// app.ErrSearchQueryTooLong for a query it won't run, and finds nothing for the zero query.
	Search(ctx context.Context, query domain.SearchQuery, page int) (views.SearchResults, error)
}

// server answers page requests.
type server struct {
	catalog Catalog
	assets  *assets
	chrome  chrome
	// routes holds every pattern the mux routes by, the only values route logs.
	routes map[string]bool
	Options
}

// New returns the handler for Rulemart's pages, reading them from catalog.
func New(catalog Catalog, options Options) (http.Handler, error) {
	if options.BaseURL != nil {
		if err := checkBaseURL(options.BaseURL); err != nil {
			return nil, fmt.Errorf("serve pages at base URL %q: %v", options.BaseURL, err)
		}
	}
	assets, err := newAssets()
	if err != nil {
		return nil, err
	}
	s := &server{
		catalog: catalog, assets: assets, Options: options, routes: map[string]bool{},
		chrome: chrome{
			stylesheet: assets.url("generated/app.css"), script: assets.url("theme.js"), icon: assets.url("favicon.svg"),
			font: assets.url("fonts/inter-latin.woff2"),
		},
	}
	mux := http.NewServeMux()
	handle := func(pattern string, handler http.HandlerFunc) {
		mux.HandleFunc(pattern, handler)
		s.routes[pattern] = true
	}
	handle("GET /{$}", s.home)
	handle("GET /_static/{version}/{file...}", assets.serve)
	// GitHub has no account named groups or search, so these can't hide a library's page. /libraries has one
	// segment, so it can't either, though GitHub has an account named libraries.
	handle("GET /libraries", s.libraries)
	handle("GET /groups", s.groups)
	handle("GET /groups/{kind}/{name}", s.group)
	handle("GET /search", s.search)
	handle("GET /{owner}/{repo}", s.library)
	handle("GET /{owner}/{repo}/{rule...}", s.rule)
	handle("/", s.notFound)
	return s.logRequests(withSecurityHeaders(withoutTrailingSlash(mux))), nil
}

// withoutTrailingSlash redirects a path that ends with a slash, such as /groups/, to the same path without it,
// keeping the query, since no page's address ends with one. A path whose trimmed form starts with two slashes, or a
// slash and a backslash, would name another host, so next answers it: the mux cleans it to a path on this site.
func withoutTrailingSlash(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		trimmed := strings.TrimRight(path, "/")
		if trimmed == path || trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, `/\`) {
			next.ServeHTTP(w, r)
			return
		}
		target := trimmed
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		redirect(w, r, target)
	})
}

func (s *server) home(w http.ResponseWriter, r *http.Request) {
	page, err := s.catalog.HomePage(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, homePage(s.pageChrome("/"), newLibraryCards(page.Libraries), newGroupIndexView(page.Groups, s.assets.iconURL)))
}

func (s *server) libraries(w http.ResponseWriter, r *http.Request) {
	libraries, err := s.catalog.Libraries(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, librariesPage(s.pageChrome(librariesHref), newLibraryCards(libraries)))
}

func (s *server) groups(w http.ResponseWriter, r *http.Request) {
	index, err := s.catalog.GroupIndex(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, groupsPage(s.pageChrome(groupsHref), newGroupIndexView(index, s.assets.iconURL)))
}

func (s *server) group(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("kind") + "/" + r.PathValue("name")
	page, err := s.catalog.GroupPage(r.Context(), id)
	if errors.Is(err, app.ErrNotFound) {
		s.notFound(w, r)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	view := newGroupPageView(page, s.assets.iconURL)
	if page.Path != id {
		target := view.href
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		redirect(w, r, target)
		return
	}
	s.render(w, r, http.StatusOK, groupPage(s.pageChrome(view.href), view))
}

// search shows a page of the results of the query in the q parameter, the page the page parameter numbers, from 1.
// Its page names no canonical address and asks search engines not to index it, since each query would otherwise be a
// page of its own. A page number that isn't one, the first page's number, and a page number without a query redirect
// to the address without it, another spelling of a page's number, such as 02, to its own, and a page past the last
// is missing.
func (s *server) search(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	query := domain.ParseSearchQuery(params.Get("q"))
	page, spelled := searchPageNumber(params)
	if query.IsZero() {
		page = 1
	}
	if !spelled || (query.IsZero() && params.Has("page")) {
		redirect(w, r, searchHrefFor(params.Get("q"), page))
		return
	}
	var results views.SearchResults
	var err error
	if page <= app.MaxSearchPage {
		results, err = s.catalog.Search(r.Context(), query, page)
	}
	tooLong := errors.Is(err, app.ErrSearchQueryTooLong)
	if err != nil && !tooLong {
		s.fail(w, r, err)
		return
	}
	view := newSearchView(query, tooLong, results, page, s.assets.iconURL)
	status := http.StatusOK
	if view.pageMissing() {
		status = http.StatusNotFound
	}
	s.render(w, r, status, searchPage(s.chrome, view))
}

// searchPageNumber returns the page params number, and whether they spell it as its address does: the first page
// by no number, and any other in digits without a sign or leading zeros. A number that isn't a page's is 1, and a
// number too large to hold is past app.MaxSearchPage.
func searchPageNumber(params url.Values) (page int, spelled bool) {
	if !params.Has("page") {
		return 1, true
	}
	text := params.Get("page")
	page, err := strconv.Atoi(text)
	switch {
	case errors.Is(err, strconv.ErrRange) && text[0] >= '1' && text[0] <= '9':
		return app.MaxSearchPage + 1, strings.Trim(text, "0123456789") == ""
	case err != nil || page < 1:
		return 1, false
	}
	return page, page > 1 && text == strconv.Itoa(page)
}

// redirect answers with a permanent redirect to target, cacheable as pages are.
func redirect(w http.ResponseWriter, r *http.Request, target string) {
	w.Header().Set("Cache-Control", pageCache)
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

// library shows a library's tab that the tab parameter names: its groups by default, its rules, or its releases,
// starting at the release the until parameter names, if any. With releases to compare in the from and to
// parameters, the releases tab compares them.
func (s *server) library(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	owner, name := r.PathValue("owner"), r.PathValue("repo")
	switch tab := libraryTab(query.Get("tab")); {
	case tab == releasesTab && (query.Has("from") || query.Has("to")):
		s.releaseComparison(w, r, owner, name)
	case tab == releasesTab:
		s.releases(w, r, owner, name)
	default:
		page, err := s.catalog.LibraryPage(r.Context(), owner, name)
		if !s.found(w, r, page.Library, err) {
			return
		}
		view := newLibraryView(page.Library)
		if tab != rulesTab {
			tab = groupsTab
		}
		s.render(w, r, http.StatusOK, libraryPage(s.pageChrome(view.href), view, newLibraryContents(view, page, s.assets.iconURL), tab))
	}
}

// releases shows the page of the library's releases that holds the release the release parameter numbers, or the
// first page. The first page has one address, so asked for a release on it, it redirects there, and the browser keeps
// the link's fragment, which leads to the release's card.
func (s *server) releases(w http.ResponseWriter, r *http.Request, owner, name string) {
	release := 0
	if text := r.URL.Query().Get("release"); r.URL.Query().Has("release") {
		var err error
		if release, err = parseReleaseNumber(text); err != nil {
			s.releasesNotFound(w, r, owner, name)
			return
		}
	}
	page, err := s.catalog.ReleasesPage(r.Context(), owner, name, release)
	if release != 0 && errors.Is(err, app.ErrNotFound) {
		s.releasesNotFound(w, r, owner, name)
		return
	}
	if !s.found(w, r, page.Library, err) {
		return
	}
	view := newLibraryView(page.Library)
	if release != 0 && page.Newer == 0 {
		w.Header().Set("Cache-Control", pageCache)
		http.Redirect(w, r, releasesHref(view), http.StatusFound)
		return
	}
	s.render(w, r, http.StatusOK, releasesPage(s.pageChrome(view.href), view, newReleasesView(view, page)))
}

// releasesNotFound answers a link to releases the library doesn't have, or can't compare, with its Library releases
// tab saying so, or the site's missing page when there's no such library.
func (s *server) releasesNotFound(w http.ResponseWriter, r *http.Request, owner, name string) {
	page, err := s.catalog.ReleasesPage(r.Context(), owner, name, 0)
	if !s.found(w, r, page.Library, err) {
		return
	}
	view := newLibraryView(page.Library)
	s.render(w, r, http.StatusNotFound, releasesNotFoundPage(s.chrome, view, newReleasesView(view, page),
		"This library has no such release to show or compare."))
}

// releaseComparison compares the library's releases that the from and to parameters number. Comparisons name no
// canonical address and ask search engines not to index them, since every pair would be a page of its own.
func (s *server) releaseComparison(w http.ResponseWriter, r *http.Request, owner, name string) {
	query := r.URL.Query()
	from, fromErr := parseReleaseNumber(query.Get("from"))
	to, toErr := parseReleaseNumber(query.Get("to"))
	if fromErr != nil || toErr != nil {
		s.releasesNotFound(w, r, owner, name)
		return
	}
	comparison, err := s.catalog.ReleaseComparison(r.Context(), owner, name, from, to)
	if errors.Is(err, app.ErrNotFound) {
		s.releasesNotFound(w, r, owner, name)
		return
	}
	if !s.found(w, r, comparison.Library, err) {
		return
	}
	view := newLibraryView(comparison.Library)
	s.render(w, r, http.StatusOK, releaseComparisonPage(s.chrome, view, newReleaseComparisonView(view, comparison, parseDiffMode(query.Get("view")))))
}

// ruleComparisonNotFound answers a link to versions the rule doesn't have with its Versions tab saying so, or the
// site's missing page when there's no such rule.
func (s *server) ruleComparisonNotFound(w http.ResponseWriter, r *http.Request) {
	page, err := s.catalog.RulePage(r.Context(), r.PathValue("owner"), r.PathValue("repo"), r.PathValue("rule"))
	if !s.found(w, r, page.Library, err) {
		return
	}
	view := newRuleView(newLibraryView(page.Library), page)
	from, to := versionPicker(page.Versions)
	s.render(w, r, http.StatusNotFound, ruleComparisonNotFoundPage(s.chrome, view, from, to,
		"This rule has no such version to compare."))
}

// parseReleaseNumber reads a release's number as its tag names it, such as 4, without a sign or leading zeros.
func parseReleaseNumber(text string) (int, error) {
	n, err := strconv.Atoi(text)
	if err != nil || n < 1 || strconv.Itoa(n) != text {
		return 0, fmt.Errorf("parse release number %q: want a positive number", text)
	}
	return n, nil
}

// rule shows a rule's page, or its Versions tab when the tab parameter names it. With versions to compare in the
// from and to parameters, the Versions tab compares them.
func (s *server) rule(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	tab := ruleTab(query.Get("tab"))
	if tab == versionsTab && (query.Has("from") || query.Has("to")) {
		s.ruleComparison(w, r)
		return
	}
	page, err := s.catalog.RulePage(r.Context(), r.PathValue("owner"), r.PathValue("repo"), r.PathValue("rule"))
	if !s.found(w, r, page.Library, err) {
		return
	}
	if tab != versionsTab {
		tab = contentTab
	}
	view := newRuleView(newLibraryView(page.Library), page)
	s.render(w, r, http.StatusOK, rulePage(s.pageChrome(view.href), view, tab))
}

// ruleComparison compares the rule's versions that the from and to parameters name. Like a comparison of releases, it
// names no canonical address and asks search engines not to index it.
func (s *server) ruleComparison(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	from, fromErr := coderules.ParseRuleVersion(query.Get("from"), "from")
	to, toErr := coderules.ParseRuleVersion(query.Get("to"), "to")
	if fromErr != nil || toErr != nil {
		s.ruleComparisonNotFound(w, r)
		return
	}
	comparison, err := s.catalog.RuleComparison(r.Context(), r.PathValue("owner"), r.PathValue("repo"), r.PathValue("rule"), from, to)
	if errors.Is(err, app.ErrNotFound) {
		s.ruleComparisonNotFound(w, r)
		return
	}
	if !s.found(w, r, comparison.Page.Library, err) {
		return
	}
	view := newRuleView(newLibraryView(comparison.Page.Library), comparison.Page)
	s.render(w, r, http.StatusOK, ruleComparisonPage(s.chrome, view, newRuleComparisonView(view, comparison, parseDiffMode(query.Get("view")))))
}

// pageChrome returns the frame for the page whose own address is href, the path its links use, which it names on
// BaseURL as its canonical address. Built from what the page shows rather than the request, every spelling of the
// page's path, such as a percent-encoded one, names the same address. A tab's query string shows the same page, so
// the address leaves it out.
func (s *server) pageChrome(href string) chrome {
	c := s.chrome
	if s.BaseURL != nil {
		c.canonical = s.BaseURL.String() + href
	}
	return c
}

// found reports whether a page's data loaded, for the library lib, and is at the path GitHub's spelling of the
// library's owner and name gives. Otherwise it answers the request itself: with a missing page, a failure, or a
// redirect to that path.
func (s *server) found(w http.ResponseWriter, r *http.Request, lib views.Library, err error) bool {
	if errors.Is(err, app.ErrNotFound) {
		s.notFound(w, r)
		return false
	}
	if err != nil {
		s.fail(w, r, err)
		return false
	}
	if lib.Owner != r.PathValue("owner") || lib.Name != r.PathValue("repo") {
		canonical := url.URL{Path: libraryHref(lib.Owner, lib.Name), RawQuery: r.URL.RawQuery}
		if rule := r.PathValue("rule"); rule != "" {
			canonical.Path += "/" + rule
		}
		redirect(w, r, canonical.String())
		return false
	}
	return true
}

func (s *server) notFound(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusNotFound, messagePage(s.chrome, "Not found", "Rulemart has no page here."))
}

// fail logs err with the request's route pattern and ID, and answers with a page that reveals nothing about the
// failure.
func (s *server) fail(w http.ResponseWriter, r *http.Request, err error) {
	s.Log.ErrorContext(r.Context(), "request failed", "route", s.route(r), "method", r.Method, "requestID", s.requestID(r),
		"status", http.StatusServiceUnavailable, "error", withoutPath(err.Error(), r))
	s.unavailable(w, r)
}

// withoutPath returns text with the library or rule that r's path names replaced by the route's wildcards. Page reads
// name what they failed to read as owner/repo or owner/repo/rule, and those come from the visitor, so the logs keep
// only the route, as the access log does.
func withoutPath(text string, r *http.Request) string {
	owner, repo, rule := r.PathValue("owner"), r.PathValue("repo"), r.PathValue("rule")
	if owner == "" || repo == "" {
		return text
	}
	if rule != "" {
		text = strings.ReplaceAll(text, owner+"/"+repo+"/"+rule, "{owner}/{repo}/{rule...}")
	}
	return strings.ReplaceAll(text, owner+"/"+repo, "{owner}/{repo}")
}

// unavailable answers with a page that says Rulemart can't show this one right now, and that can't be cached.
func (s *server) unavailable(w http.ResponseWriter, r *http.Request) {
	var page bytes.Buffer
	_ = messagePage(s.chrome, "Unavailable", "Rulemart can't show this page right now. Try again in a minute.").Render(r.Context(), &page)
	write(w, r, http.StatusServiceUnavailable, "no-store", page.Bytes())
}

// render writes page with status, cacheable for a minute. It renders the whole page before writing, so a failure
// leaves no partial page, and a page larger than maxPageBytes is replaced by one that says so.
func (s *server) render(w http.ResponseWriter, r *http.Request, status int, page templ.Component) {
	var body bytes.Buffer
	if err := page.Render(r.Context(), &body); err != nil {
		s.fail(w, r, err)
		return
	}
	if body.Len() > maxPageBytes {
		s.Log.WarnContext(r.Context(), "page too large", "route", s.route(r), "method", r.Method, "requestID", s.requestID(r),
			"bytes", body.Len())
		body.Reset()
		if err := messagePage(s.chrome, "Too large", "This page is too large to show.").Render(r.Context(), &body); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	write(w, r, status, pageCache, body.Bytes())
}

func write(w http.ResponseWriter, r *http.Request, status int, cache string, body []byte) {
	header := w.Header()
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("Cache-Control", cache)
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

// withSecurityHeaders adds the headers every response carries.
func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("Content-Security-Policy", contentSecurityPolicy)
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}
