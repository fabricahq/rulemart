// Package web serves Rulemart's pages: the vetted libraries, each library's groups, rules, and releases, each rule's
// current version and version history, comparisons of two releases or two rule versions, the groups across libraries,
// each canonical group's rules in every library, and search; the unvetted libraries, whose pages warn that they
// aren't vetted; signing in with GitHub, signing out, and the signed-in visitor's account; listing a library;
// starring one; and collecting rules in a cart and checking it out. It reads the catalog from its page reads, which
// app.Pages implements, accounts from accounts/app.Sessions, listings from catalog/app.Listings, stars from
// catalog/app.Stars, and carts from catalog/app.Cart.
package web

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
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

// pageCache lets CloudFront keep a page for a minute, so a new library release, or a new star, shows within a
// minute without every visit reaching the function. Browsers keep it too, but ask again before each use, which
// CloudFront answers from its copy, so a browser that signs in or out never shows a page it kept from before, with
// stale stars or listings. CloudFront's cache policy takes s-maxage over max-age. Only a page that's the same for
// every visitor gets it: withPrivateResponses replaces it on any response to a signed-in browser.
const pageCache = "public, max-age=0, s-maxage=60"

// privateCache keeps a response out of every cache: one that depends on who asked, or that sets a cookie.
const privateCache = "private, no-store"

// contentSecurityPolicy allows only Rulemart's own files, plus images from GitHub's avatar and raw file hosts,
// which rules and library owners use, and forms that submit to Rulemart, such as search. Rule content comes from
// repositories Rulemart doesn't control, so nothing else may load or run.
const contentSecurityPolicy = "default-src 'none'; script-src 'self'; style-src 'self'; font-src 'self'; " +
	"img-src 'self' https://avatars.githubusercontent.com https://raw.githubusercontent.com; " +
	"base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

// signInContentSecurityPolicy is contentSecurityPolicy for the sign-in page, whose GitHub form posts here to be
// redirected to GitHub's authorization page. Browsers check a form's redirects against form-action too, so it
// allows that one page as well, and only on the page that needs it.
var signInContentSecurityPolicy = strings.Replace(contentSecurityPolicy, "form-action 'self';",
	"form-action 'self' https://github.com/login/oauth/authorize;", 1)

// Options configures the handler.
type Options struct {
	// Log receives one line for each request, and the details of failures that pages leave out.
	Log *slog.Logger
	// RequestID returns an identifier for a request that the logs record, such as the Lambda request ID. It may
	// be nil.
	RequestID func(*http.Request) string
	// BaseURL is Rulemart's public origin, such as https://rulemart.fabricahq.com, which each page names as its
	// canonical address, so search engines index that one address for it whichever host served it. It has no path.
	// Nil names none. ParseBaseURL makes one from text. Sign-in links and GitHub's callback are on it too.
	BaseURL *url.URL
	// Accounts signs visitors in and out. Nil leaves accounts out: pages offer no sign-in.
	Accounts Accounts
	// GitHub signs visitors in with GitHub. Nil, with Accounts, leaves only a local build's test users to sign in as.
	GitHub GitHub
	// Listings lists libraries for signed-in visitors. Nil, or without a way to sign in, leaves listing out.
	Listings Listings
	// Stars stars vetted libraries for signed-in visitors, and pages count their stars. Nil leaves stars out; without a
	// way to sign in, pages only count them.
	Stars Stars
	// Cart keeps signed-in visitors' carts. Nil, or without a way to sign in, leaves carts out.
	Cart Cart
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
	// UnvettedLibraries returns the libraries listings name that aren't vetted.
	UnvettedLibraries(ctx context.Context) ([]views.LibraryCard, error)
	// LibraryPage, ReleasesPage, and RulePage fail with app.ErrNotFound when there's no such library or rule. They find
	// a library a listing names as well as a vetted one, and say which it is.
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
			stylesheet: assets.url("generated/app.css"), script: assets.url("theme.js"), menuScript: assets.url("menus.js"),
			caretScript: assets.url("caret.js"),
			copyScript:  assets.url("copy.js"),
			icon:        assets.url("favicon.svg"),
			font:        assets.url("fonts/inter-latin.woff2"),
		},
	}
	mux := http.NewServeMux()
	// Every page's handler first finds who the request is from, which the frame shows.
	handle := func(pattern string, handler http.HandlerFunc) {
		mux.HandleFunc(pattern, s.withVisitor(handler))
		s.routes[pattern] = true
	}
	mux.HandleFunc(staticPattern, assets.serve)
	s.routes[staticPattern] = true
	handle("GET /{$}", s.home)
	// GitHub has no account named groups or search, so these can't hide a library's page. /libraries has one
	// segment, so it can't either, though GitHub has an account named libraries.
	handle("GET /libraries", s.libraries)
	handle("GET /groups", s.groups)
	handle("GET /groups/{kind}/{name}", s.group)
	handle("GET /search", s.search)
	// One segment can't hide a library's page.
	handle("GET "+unvettedHref, s.unvetted)
	if options.Accounts != nil {
		// Single segments can't hide a library's page, and GitHub has no account named account.
		handle("GET "+signInHref, s.signInPage)
		handle("POST "+signInHref, s.startSignIn)
		handle("GET "+gitHubCallbackHref, s.gitHubCallback)
		handle("POST "+signOutHref, s.signOut)
		handle("GET "+accountHref, s.accountPage)
		handle("POST "+signOutEverywhereHref, s.signOutEverywhere)
		handle("POST "+deleteAccountHref, s.deleteAccount)
		s.registerDevSignIn(handle)
		if options.Listings != nil {
			handle("GET "+listHref, s.listPage)
			handle("POST "+listHref, s.createListing)
			handle("GET "+listingsHref, s.listingsPage)
			handle("GET "+removeListingHref, s.removeListingPage)
			handle("POST "+removeListingHref, s.removeListing)
			handle("POST "+retryListingHref, s.retryListing)
		}
		if options.Stars != nil {
			handle("GET "+starsHref, s.starsPage)
			handle("POST "+starsHref, s.starLibrary)
			handle("POST "+unstarHref, s.unstarLibrary)
		}
		if options.Cart != nil {
			handle("GET "+cartHref, s.cartPage)
			handle("POST "+cartHref, s.addToCart)
			handle("POST "+removeFromCartHref, s.removeFromCart)
			handle("POST "+emptyCartHref, s.emptyCart)
			handle("GET "+confirmCartHref, s.confirmCartPage)
			handle("GET "+checkoutHref, s.checkoutPage)
		}
	}
	handle("GET /{owner}/{repo}", s.library)
	handle("GET /{owner}/{repo}/{rule...}", s.rule)
	handle("/", s.notFound)
	return s.logRequests(withSecurityHeaders(withPrivateResponses(s.withSameOriginWrites(withoutTrailingSlash(withSiteSectionsInLowercase(mux)))))), nil
}

// siteSections are the first segments of the site's own pages, which no library owner shadows: groups for every
// path under it, and libraries, search, unvetted, and list as a whole path, since GitHub has an account named
// libraries, whose libraries' pages are /libraries/{repo}, and may have others.
var siteSections = map[string]bool{"groups": true, "libraries": false, "search": false, "unvetted": false, "list": false}

// withSiteSectionsInLowercase redirects a path whose first segment spells one of siteSections in another case, such as
// /Groups or /SEARCH, to the same path with that segment in lowercase, keeping the query, as a library's other
// spellings redirect. The target starts with the section, so it stays on the site.
func withSiteSectionsInLowercase(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		first, rest, nested := strings.Cut(strings.TrimPrefix(r.URL.EscapedPath(), "/"), "/")
		section := strings.ToLower(first)
		withSubpaths, ok := siteSections[section]
		if !ok || first == section || (nested && !withSubpaths) {
			next.ServeHTTP(w, r)
			return
		}
		target := "/" + section
		if nested {
			target += "/" + rest
		}
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		redirect(w, r, target)
	})
}

// staticPattern is the route of the static files, which are the same for every visitor.
const staticPattern = "GET /_static/{version}/{file...}"

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
	cards, err := s.vettedCards(r, page.Libraries)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, homePage(s.pageChrome("/"), cards, newGroupIndexView(page.Groups, s.assets.iconURL)))
}

func (s *server) libraries(w http.ResponseWriter, r *http.Request) {
	libraries, err := s.catalog.Libraries(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	cards, err := s.vettedCards(r, libraries)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, librariesPage(s.pageChrome(librariesHref), cards, s.listingAvailable()))
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
// parameters, the releases tab compares them. Returning from signing in to star the library, it prompts once to star
// it.
func (s *server) library(w http.ResponseWriter, r *http.Request) {
	if s.withoutStarPrompt(w, r) {
		return
	}
	query := r.URL.Query()
	owner, name := r.PathValue("owner"), r.PathValue("repo")
	switch tab := libraryTab(query.Get("tab")); {
	case tab == releasesTab && (query.Has("from") || query.Has("to")):
		s.releaseComparison(w, r, owner, name)
	case tab == releasesTab:
		s.releases(w, r, owner, name)
	default:
		page, err := s.catalog.LibraryPage(r.Context(), owner, name)
		if !s.found(w, r, page.Library, "", err) {
			return
		}
		view, err := s.libraryView(r, page.Library)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if tab != rulesTab {
			tab = groupsTab
		}
		contents := s.withGroupCarts(r, view, newLibraryContents(view, page, s.assets.iconURL))
		s.render(w, r, http.StatusOK, libraryPage(s.pageChrome(view.href), view, contents, tab))
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
	if !s.found(w, r, page.Library, "", err) {
		return
	}
	view, err := s.libraryView(r, page.Library)
	if err != nil {
		s.fail(w, r, err)
		return
	}
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
	if !s.found(w, r, page.Library, "", err) {
		return
	}
	view, err := s.libraryView(r, page.Library)
	if err != nil {
		s.fail(w, r, err)
		return
	}
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
	if !s.found(w, r, comparison.Library, "", err) {
		return
	}
	view, err := s.libraryView(r, comparison.Library)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, releaseComparisonPage(s.chrome, view, newReleaseComparisonView(view, comparison, parseDiffMode(query.Get("view")))))
}

// ruleComparisonNotFound answers a link to versions the rule doesn't have with its Versions tab saying so, or the
// site's missing page when there's no such rule.
func (s *server) ruleComparisonNotFound(w http.ResponseWriter, r *http.Request) {
	page, err := s.catalog.RulePage(r.Context(), r.PathValue("owner"), r.PathValue("repo"), r.PathValue("rule"))
	if !s.found(w, r, page.Library, page.Rule.Path, err) {
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
	if !s.found(w, r, page.Library, page.Rule.Path, err) {
		return
	}
	if tab != versionsTab {
		tab = contentTab
	}
	view := newRuleView(newLibraryView(page.Library), page)
	if view.retired == nil {
		items, err := s.libraryCart(r, page.Library.Owner, page.Library.Name)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		item := domain.CartItem{Owner: page.Library.Owner, Name: page.Library.Name, Kind: domain.CartRule, Path: page.Rule.Path}
		view.cart = s.newCartControl(r, page.Library.Vetted, items, item, "Add the rule "+view.title+" to your cart")
	}
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
	if !s.found(w, r, comparison.Page.Library, comparison.Page.Rule.Path, err) {
		return
	}
	view := newRuleView(newLibraryView(comparison.Page.Library), comparison.Page)
	s.render(w, r, http.StatusOK, ruleComparisonPage(s.chrome, view, newRuleComparisonView(view, comparison, parseDiffMode(query.Get("view")))))
}

// libraryView describes lib for the page r asks for, with the controls a library's pages show for the visitor: its
// star, and the cart's control that adds the whole library, with the signed-in visitor's items from it, which it
// reads.
func (s *server) libraryView(r *http.Request, lib views.Library) (libraryView, error) {
	view := newLibraryView(lib)
	var err error
	if view.star, err = s.starControl(r, lib, view.href); err != nil {
		return libraryView{}, err
	}
	if view.cartItems, err = s.libraryCart(r, lib.Owner, lib.Name); err != nil {
		return libraryView{}, err
	}
	whole := domain.CartItem{Owner: lib.Owner, Name: lib.Name, Kind: domain.CartLibrary}
	view.cart = s.newCartControl(r, lib.Vetted, view.cartItems, whole, "Add every group of "+lib.FullName()+" to your cart")
	return view, nil
}

// withGroupCarts gives each group of contents, a library's groups on the page r asks for, the cart's control that
// adds it.
func (s *server) withGroupCarts(r *http.Request, lib libraryView, contents libraryContents) libraryContents {
	for _, groups := range [][]groupView{contents.techs, contents.practices} {
		for i, g := range groups {
			item := domain.CartItem{Owner: lib.owner, Name: lib.name, Kind: domain.CartGroup, Path: g.label.id}
			name := g.label.id
			if g.label.canonical {
				name = g.label.name
			}
			groups[i].cart = s.newCartControl(r, lib.vetted, lib.cartItems, item, "Add the group "+name+" to your cart")
		}
	}
	return contents
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

// found reports whether a page's data loaded, for the library lib and, on a rule's page, the rule at rulePath, and is
// at the path that GitHub's spelling of the library's owner and name, and the library's of the rule's ID, give.
// Otherwise it answers the request itself: with a missing page, a failure, or a redirect to that path.
func (s *server) found(w http.ResponseWriter, r *http.Request, lib views.Library, rulePath string, err error) bool {
	if errors.Is(err, app.ErrNotFound) {
		s.notFound(w, r)
		return false
	}
	if err != nil {
		s.fail(w, r, err)
		return false
	}
	if lib.Owner != r.PathValue("owner") || lib.Name != r.PathValue("repo") || rulePath != r.PathValue("rule") {
		canonical := url.URL{Path: libraryHref(lib.Owner, lib.Name), RawQuery: r.URL.RawQuery}
		if rulePath != "" {
			canonical.Path += "/" + rulePath
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
		"status", http.StatusServiceUnavailable, "error", withoutQuery(withoutPath(err.Error(), r), r))
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

// withoutQuery returns text with each value of r's query parameters replaced by the parameter's name in braces, such
// as {library}, longest first. Writes name what they act on in the query, such as a cart's library and rule, and those
// come from the visitor, so the logs keep only the parameter, as the access log keeps no query.
func withoutQuery(text string, r *http.Request) string {
	type param struct{ name, value string }
	var params []param
	for name, values := range r.URL.Query() {
		for _, value := range values {
			if value != "" {
				params = append(params, param{name, value})
			}
		}
	}
	slices.SortFunc(params, func(a, b param) int { return cmp.Compare(len(b.value), len(a.value)) })
	for _, p := range params {
		text = strings.ReplaceAll(text, p.value, "{"+p.name+"}")
	}
	return text
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
	s.renderWith(w, r, status, pageCache, page)
}

// renderPrivate writes page as render does, but uncacheable, for a page that belongs to one visitor.
func (s *server) renderPrivate(w http.ResponseWriter, r *http.Request, status int, page templ.Component) {
	s.renderWith(w, r, status, privateCache, page)
}

// renderWith writes page with status and the Cache-Control cache, as render describes.
func (s *server) renderWith(w http.ResponseWriter, r *http.Request, status int, cache string, page templ.Component) {
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
	// The page shows its notice, if any, so the next page mustn't again.
	if visitorOf(r.Context()).noticeKey != "" {
		clearCookie(w, noticeCookie)
	}
	write(w, r, status, cache, body.Bytes())
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
