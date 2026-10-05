// Package web serves Rulemart's pages: the vetted libraries, each library's groups, rules, and releases, each rule's
// current version and version history, each rule's assets and the images among them, comparisons of two releases or two rule versions, the groups across libraries by
// kind, each canonical group's rules in every library, search, the FAQ, and feedback; the unvetted libraries, whose
// pages warn that they aren't vetted; signing in with GitHub, signing out, and the signed-in visitor's account; listing
// a library; starring rules; and checking out the cart a visitor's browser keeps. It reads the catalog from its page
// reads, which app.Pages implements, accounts from accounts/app.Sessions, listings from catalog/app.Listings, stars
// from catalog/app.Stars, and checkouts from catalog/app.Carts.
package web

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
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

// maxPageBytes bounds a page Rulemart sends, and a cart's checkout: a Lambda function's response holds at most 6 MB.
// Pages bound what they show from a library, so only a library far past any Rulemart knows reaches it; such a page says
// it's too large, and such a checkout fails.
const maxPageBytes = 5 << 20

// pageCache lets CloudFront keep a page for a minute, so a new library release, or a new star, shows within a
// minute without every visit reaching the function. Browsers keep it too, but ask again before each use, which
// CloudFront answers from its copy, so a browser that signs in or out never shows a page it kept from before, with
// stale stars or listings. CloudFront's cache policy takes s-maxage over max-age. Only a page that's the same for
// every visitor gets it: withPrivateResponses replaces it on any response to a signed-in browser.
const pageCache = "public, max-age=0, s-maxage=60"

// privateCache keeps a response out of every cache: one that depends on who asked, or that sets a cookie.
const privateCache = "private, no-store"

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
	// Stars stars the current rules of vetted libraries for signed-in visitors. Nil, or without a way to sign in,
	// leaves starring out; pages still show the stars the catalog counts.
	Stars Stars
	// GitHubAccounts reads what signed-in visitors' GitHub accounts hold. Nil, or without a way to sign in, leaves the
	// dashboard's libraries, projects, and private repositories out.
	GitHubAccounts GitHubAccounts
	// Carts checks out the carts visitors' browsers keep. Nil leaves checkout out.
	Carts Carts
	// AnalyticsToken is the site token of a Cloudflare Web Analytics site, which every page then loads Cloudflare's
	// beacon with, and the content security policy allows. Empty leaves analytics out: no page loads another site's
	// script. New refuses one that can't be a token.
	AnalyticsToken string
}

// ParseBaseURL parses text as Options.BaseURL: an https origin, or an http one on a loopback host, with no path,
// query, or fragment, such as https://rulemart.fabricahq.com. Empty text gives nil.
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

// checkBaseURL reports whether u is an origin and nothing more, so a page's path appends to it as is: an https one, or
// an http one on a loopback host, such as a local build's http://127.0.0.1:8080.
func checkBaseURL(u *url.URL) error {
	if (u.Scheme != "https" && (u.Scheme != "http" || !isLoopback(u.Hostname()))) || u.Host == "" || u.Opaque != "" ||
		u.User != nil || u.Path != "" || u.RawPath != "" || u.ForceQuery || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("want an https origin, or an http one on a loopback host, with no path, query, or fragment, " +
			"such as https://rulemart.example")
	}
	return nil
}

// isLoopback reports whether host, without a port, names this machine: localhost or a loopback address.
func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

// Catalog reads what the pages show. app.Pages implements it, finding the vetted libraries, and the ones listings name
// too on a library's own pages and in lists that include unvetted libraries.
type Catalog interface {
	HomePage(ctx context.Context) (views.HomePage, error)
	// Libraries and GroupIndex find the libraries listings name too when unvetted is true.
	Libraries(ctx context.Context, unvetted bool) ([]views.LibraryCard, error)
	// UnvettedLibraries returns the libraries listings name that aren't vetted.
	UnvettedLibraries(ctx context.Context) ([]views.LibraryCard, error)
	// OwnerPage returns the owner login, matched without regard to case, with their vetted libraries, or fails with
	// app.ErrNotFound when no vetted library is theirs.
	OwnerPage(ctx context.Context, login string) (views.OwnerPage, error)
	// LibraryPage, ReleasesPage, and RulePage fail with app.ErrNotFound when there's no such library or rule. They find
	// a library a listing names as well as a vetted one, and say which it is.
	LibraryPage(ctx context.Context, owner, name string) (views.LibraryPage, error)
	// ReleasesPage returns the page of releases that holds release, or the first page when release is 0, and fails
	// with app.ErrNotFound when there's no such release either.
	ReleasesPage(ctx context.Context, owner, name string, release int) (views.ReleasesPage, error)
	RulePage(ctx context.Context, owner, name, rulePath string) (views.RulePage, error)
	// AssetPage returns the asset at assetPath, a path in the repository, of the current rule at rulePath in the
	// library owner/name, with the rule's page, or of the first rule that lists it when rulePath is empty, or fails with
	// app.ErrNotFound when there's no such library or rule, or the rule doesn't list such an asset. A rule's own asset's
	// path may spell the rule's asset directory as rulePath spells the rule; the page's paths are the library's spelling.
	AssetPage(ctx context.Context, owner, name, rulePath, assetPath string) (views.AssetPage, error)
	// AssetImage returns the image at assetPath in the library owner/name that Rulemart keeps, one of the own assets of
	// the rule at rulePath, matched as RulePage matches it, or a shared one when rulePath is empty, or fails with
	// app.ErrNotFound when there's no such library, rule, or image. A rule's own asset's path may spell the rule's asset
	// directory as rulePath spells the rule; the image's Rule and Path are the library's spelling.
	AssetImage(ctx context.Context, owner, name, rulePath, assetPath string) (views.AssetContent, error)
	// ReleaseComparison and RuleComparison put the older release or version first, and fail with app.ErrNotFound
	// when there's no such library, rule, release, or version.
	ReleaseComparison(ctx context.Context, owner, name string, from, to int) (views.ReleaseComparison, error)
	RuleComparison(ctx context.Context, owner, name, rulePath string, from, to coderules.RuleVersion) (views.RuleComparison, error)
	GroupIndex(ctx context.Context, unvetted bool) (views.GroupIndex, error)
	// GroupPage returns the rules of the group id that choices keep, or fails with app.ErrNotFound when id isn't canonical
	// and no library holds it. mine are the visitor's libraries, which My libraries keeps.
	GroupPage(ctx context.Context, id string, choices domain.ListChoices, mine domain.MyLibraries) (views.GroupPage, error)
	// SearchRules returns page, from 1 to app.MaxSearchPage, of the rules query finds, or of every rule for the zero
	// query, that choices keep, mine as GroupPage's. It fails with app.ErrSearchQueryTooLong for a query it won't run.
	SearchRules(ctx context.Context, query domain.SearchQuery, choices domain.ListChoices, mine domain.MyLibraries, page int) (views.RuleResults, error)
	// Dashboard returns what a visitor's dashboard shows of the catalog: the libraries whose owner is one of owners, and
	// those of names, as owner/name, the libraries the visitor's projects import.
	Dashboard(ctx context.Context, owners, names []string) (views.Dashboard, error)
	// Sitemap returns the vetted libraries, with their current rules, and the groups that hold them, canonical or not.
	Sitemap(ctx context.Context) (views.Sitemap, error)
}

// server answers page requests.
type server struct {
	catalog Catalog
	assets  *assets
	chrome  chrome
	// routes holds every pattern the mux routes by, the only values route logs.
	routes map[string]bool
	// policies are the content security policies the site sends, with analytics when Options.AnalyticsToken is set.
	policies policies
	// content holds the pages written in Markdown, as this server's options show them.
	content contentPages
	Options
}

// New returns the handler for Rulemart's pages, reading them from catalog.
func New(catalog Catalog, options Options) (http.Handler, error) {
	s, err := newServer(catalog, options)
	if err != nil {
		return nil, err
	}
	return s.handler(), nil
}

// newServer returns the server of Rulemart's pages, reading them from catalog, before it routes any. It refuses
// options it can't serve pages with.
func newServer(catalog Catalog, options Options) (*server, error) {
	if options.BaseURL != nil {
		if err := checkBaseURL(options.BaseURL); err != nil {
			return nil, fmt.Errorf("serve pages at base URL %q: %v", options.BaseURL, err)
		}
	}
	beacon, err := analyticsBeacon(options.AnalyticsToken)
	if err != nil {
		return nil, fmt.Errorf("serve pages with analytics: %v", err)
	}
	assets, err := newAssets()
	if err != nil {
		return nil, err
	}
	s := &server{
		catalog: catalog, assets: assets, Options: options, routes: map[string]bool{}, policies: newPolicies(beacon != ""),
		chrome: chrome{
			beacon:     beacon,
			stylesheet: assets.url("generated/app.css"), script: assets.url("theme.js"), menuScript: assets.url("menus.js"),
			caretScript: assets.url("caret.js"), cartPageScript: assets.url("cart-page.js"), cartCheckoutScript: assets.url("cart-checkout.js"),
			toastScript: assets.url("toast.js"), pollScript: assets.url("poll.js"), starScript: assets.url("star.js"), compareScript: assets.url("compare.js"), cartScript: assets.url("cart.js"), filtersScript: assets.url("filters.js"),
			icon: assets.url("favicon.svg"), touchIcon: assets.url("apple-touch-icon.png"),
			logo: assets.url("rulemart-horizontal-dark.svg"), darkLogo: assets.url("rulemart-horizontal-white.svg"),
			font: assets.url("fonts/inter-latin.woff2"),
		},
	}
	s.content, err = loadContentPages(contentFiles, newContentValues(s.listingAvailable(), options.AnalyticsToken != ""))
	if err != nil {
		return nil, err
	}
	return s, nil
}

// handler routes each page's pattern to its handler, recording every pattern in s.routes, and returns the routes behind
// what every request passes through first, such as the access log and the security headers.
func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	// Every page's handler first finds who the request is from, which the frame shows.
	handle := func(pattern string, handler http.HandlerFunc) {
		mux.HandleFunc(pattern, s.withVisitor(handler))
		s.routes[pattern] = true
	}
	// Browsers ask for /favicon.ico wherever a page names no icon they take, such as for a file that isn't a page.
	mux.HandleFunc(faviconPattern, s.assets.serveFavicon)
	s.routes[faviconPattern] = true
	handle("GET /{$}", s.home)
	// One segment each, so neither can hide a library's page.
	handle("GET "+robotsHref, s.robots)
	handle("GET "+sitemapHref, s.sitemap)
	handle("GET "+aboutHref, s.about)
	// Two segments, so it takes the page of a library named about/vetting, as libraryPageTaken says.
	handle("GET "+vettingHref, s.vetting)
	handle("GET "+privacyHref, s.privacy)
	handle("GET "+faqHref, s.faq)
	handle("GET "+feedbackHref, s.feedback)
	// One segment can't hide a library's page, though GitHub has accounts named libraries, browse, and g. Under
	// browse, g, and groups, only each kind's own pages are the site's: a library's page has two segments and a
	// rule's at least five, so only a browse page takes a library's address, browse/techs or browse/practices, and
	// every other path reaches the library and rule pages.
	handle("GET /libraries", s.libraries)
	handle("GET "+strings.TrimSuffix(browsePrefix, "/"), s.redirectToTechs)
	handle("GET "+legacyGroupsHref, s.redirectToTechs)
	for _, kind := range groupKinds {
		handle("GET "+kind.href(), s.browse(kind))
		handle("GET "+kind.othersHref(), s.otherGroups(kind))
		handle("GET "+groupPrefix+string(kind)+"/{name}", s.group(kind))
		handle("GET "+legacyGroupsHref+"/"+string(kind)+"/{name}", s.legacyGroup(kind))
	}
	handle("GET /search", s.search)
	// One segment, which siteSections reserves from owners. The checkout under it is a POST, so every library of an
	// owner named cart keeps its page, cart/checkout.json too.
	handle("GET "+cartHref, s.cartPage)
	if s.Carts != nil {
		handle("POST "+checkoutHref, s.checkout)
	}
	// Under account, which is reserved whether sign-in is available or not, and GitHub has no account named account.
	handle("GET "+legacyCartHref, s.redirectToCart)
	handle("GET "+legacyCheckoutHref, s.redirectToCart)
	// One segment can't hide a library's page.
	handle("GET "+unvettedHref, s.unvetted)
	if s.Accounts != nil {
		// One segment each, reserved from owners, and pages under /me, which take only the pages of libraries an owner
		// named me publishes, as libraryPageTaken says. GitHub has no account named account.
		handle("GET "+signInHref, s.signInPage)
		handle("POST "+signInHref, s.startSignIn)
		handle("GET "+legacySignInHref, s.legacySignIn)
		handle("GET "+gitHubCallbackHref, s.gitHubCallback)
		handle("POST "+signOutHref, s.signOut)
		handle("GET "+dashboardHref, s.dashboard)
		handle("GET "+legacyAccountHref, s.legacyAccount)
		handle("POST "+signOutEverywhereHref, s.signOutEverywhere)
		handle("POST "+deleteAccountHref, s.deleteAccount)
		s.registerDevSignIn(handle)
		if s.GitHubAccounts != nil {
			handle("POST "+refreshHref, s.refresh)
		}
		if s.Listings != nil {
			handle("GET "+listHref, s.addPage)
			handle("POST "+listHref, s.createListing)
			handle("GET "+runHref, s.runPage)
			handle("GET "+legacyListHref, s.legacyList)
			handle("GET "+listingsHref, s.listingsPage)
			handle("GET "+legacyListingsHref, s.legacyListings)
			handle("GET "+removeListingHref, s.removeListingPage)
			handle("POST "+removeListingHref, s.removeListing)
			handle("POST "+retryListingHref, s.retryListing)
		}
		if s.privateAvailable() {
			handle("GET "+privateHref, s.privatePage)
			handle("POST "+removePrivateHref, s.removePrivate)
			handle("GET "+installedHref, s.installed)
			// GitHub's deliveries aren't a visitor's: no session, no page.
			mux.HandleFunc("POST "+WebhookHref, s.webhook)
			s.routes["POST "+WebhookHref] = true
		}
		if s.Stars != nil {
			handle("GET "+legacyStarredHref, s.legacyStarred)
			// Reserved as signInSections lists, so an owner named stars would have their page under /o/.
			handle("POST "+starsHref, s.starRule)
			handle("POST "+unstarHref, s.unstarRule)
		}
	}
	// An owner's page has one segment, like the site's own pages, which come first, so an owner whose login is one
	// of theirs is at /o/{login} instead.
	handle("GET "+ownerAliasPrefix+"{login}", s.ownerAlias)
	handle(ownerPattern, s.owner)
	handle(libraryPattern, s.library)
	handle(libraryGroupPattern, s.orAsset(s.libraryGroup))
	handle(rulePattern, s.orAsset(s.rule))
	handle(notFoundPattern, s.notFound)
	return s.logRequests(withSecurityHeaders(s.policies.page, withPrivateResponses(s.withSameOriginWrites(withoutTrailingSlash(
		s.withStaticFiles(withSiteSectionsInLowercase(mux)))))))
}

// The routes that take every path the site's own pages don't: an owner's, a library's, a library group's, and a
// rule's pages, which GitHub's spelling of their names addresses, and the missing page. A group's ID has two parts and
// a rule's at least three, so a library group's page takes exactly four segments, and a rule's page the rest.
const (
	ownerPattern        = "GET /{owner}"
	libraryPattern      = "GET /{owner}/{repo}"
	libraryGroupPattern = "GET /{owner}/{repo}/{kind}/{group}"
	rulePattern         = "GET /{owner}/{repo}/{rule...}"
	notFoundPattern     = "/"
)

// catchAllPatterns are the routes of the paths the site's own pages don't take.
var catchAllPatterns = map[string]bool{
	ownerPattern: true, libraryPattern: true, libraryGroupPattern: true, rulePattern: true, notFoundPattern: true,
}

// withStaticFiles answers a request staticPattern takes with the static files, and every other request with next, the
// pages, recording the pattern in s.routes. The static files can't share the pages' mux: staticPattern and
// libraryGroupPattern both match paths such as /_static/<version>/icons/go.svg, and neither matches only some of the
// other's, so ServeMux refuses to hold both. No GitHub login holds an underscore, so no owner's pages are under
// /_static.
func (s *server) withStaticFiles(next http.Handler) http.Handler {
	files := http.NewServeMux()
	files.Handle(staticPattern, s.assets)
	s.routes[staticPattern] = true
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handler also names staticPattern for a path it would redirect to one the pattern takes, such as
		// /_static/missing, which stays a page's path.
		if handler, _ := files.Handler(r); handler == http.Handler(s.assets) {
			files.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// siteSections are the first segments of the site's own pages, which no owner's page shadows: browse, g, o, and the
// old groups, with pages under them, cart, with its checkout, a POST, and libraries, search, unvetted, list, about,
// privacy, faq, and feedback. With signInSections, they're the logins whose owner pages are under /o/.
//
// A library's page has two segments, and a rule's at least five, since a rule's ID has at least three, so the site's
// pages under these sections take only the pages of the libraries libraryPageTaken names, by design: browse/techs and
// browse/practices, the browse pages, about/vetting, the vetting page, and every library owned by o, whose address is
// an owner's page under /o/. GitHub has users named browse and o. Their rules' pages stay, and so does every other
// library's page, such as browse/rules, g/techs, groups/techs, or libraries/rules.
var siteSections = []string{
	"browse", "g", "o", "groups", "libraries", "search", "unvetted", "about", "privacy", "faq", "feedback", "cart",
}

// signInSections are the first segments of the routes that exist only when sign-in is available: the account pages,
// signing in and out, and starring rules. They're reserved like siteSections whether sign-in is available or not.
var signInSections = []string{
	strings.TrimPrefix(accountHref, "/"), strings.TrimPrefix(signInHref, "/"), strings.TrimPrefix(signOutHref, "/"),
	strings.TrimPrefix(legacySignInHref, "/"), strings.TrimPrefix(starsHref, "/"), strings.TrimPrefix(dashboardHref, "/"),
	strings.TrimPrefix(legacyListHref, "/"),
}

// reservedOwner reports whether login, in any case, is the first segment of one of the site's own pages, so its
// owner's page can't be at /{login}. GitHub has users named g, faq, browse, list, and o, among others.
func reservedOwner(login string) bool {
	lower := strings.ToLower(login)
	return slices.Contains(siteSections, lower) || slices.Contains(signInSections, lower)
}

// libraryPageTaken reports whether one of the site's own pages takes the address of the library owner/name's page, in
// any case, as siteSections lists, so the library has no page, and the sitemap leaves its address out. The visitor's
// pages take me/{name}, as an owner named me's libraries' pages, GitHub's callback takes account/{name}, though GitHub
// has no account named account, starring takes stars/remove, and the vetting page takes about/vetting, but not
// about/Vetting, since only a path's first segment redirects to lowercase.
func libraryPageTaken(owner, name string) bool {
	switch strings.ToLower(owner) {
	case strings.Trim(ownerAliasPrefix, "/"), strings.TrimPrefix(accountHref, "/"), strings.TrimPrefix(starsHref, "/"),
		strings.TrimPrefix(dashboardHref, "/"):
		return true
	case strings.Trim(browsePrefix, "/"):
		_, kind := parseGroupKind(name)
		return kind
	case strings.TrimPrefix(aboutHref, "/"):
		return aboutHref+"/"+name == vettingHref
	}
	return false
}

// withSiteSectionsInLowercase redirects a path whose first segment spells one of the site's own pages in another case,
// such as /Groups/techs/go or /SEARCH, or whose kind does under browse, g, and groups, such as /browse/Techs, to the
// path siteSpelling gives, keeping the query, as a library's other spellings redirect. It redirects only when mux
// routes the lowercase path to one of the site's own pages: a path a catch-all takes, such as /G/rules for a library
// whose owner's login is G, is left to that page's own redirect to GitHub's spelling, which a lowercase redirect would
// send back and forth. The target starts with the section, so it stays on the site.
func withSiteSectionsInLowercase(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		target := siteSpelling(path)
		if target == path || catchAllPatterns[routeOf(mux, r, target)] {
			mux.ServeHTTP(w, r)
			return
		}
		redirect(w, r, withQuery(target, r))
	})
}

// siteSpelling returns path, an escaped path, with its first segment in lowercase, and under browse, g, and groups,
// with a second segment that names a group kind in any case spelled as the kind, as the site's own pages spell them.
func siteSpelling(path string) string {
	segments := strings.SplitN(strings.TrimPrefix(path, "/"), "/", 3)
	segments[0] = strings.ToLower(segments[0])
	if len(segments) > 1 && slices.Contains(kindSections, segments[0]) {
		if kind, ok := parseGroupKind(segments[1]); ok {
			segments[1] = string(kind)
		}
	}
	return "/" + strings.Join(segments, "/")
}

// kindSections are the sections whose pages name a group kind as their second segment.
var kindSections = []string{strings.Trim(browsePrefix, "/"), strings.Trim(groupPrefix, "/"), strings.Trim(legacyGroupsHref, "/")}

// routeOf returns the pattern mux routes r to at the escaped path instead of r's own, or the missing page's when it
// routes the path to none, such as one it would clean first.
func routeOf(mux *http.ServeMux, r *http.Request, path string) string {
	unescaped, err := url.PathUnescape(path)
	if err != nil {
		return notFoundPattern
	}
	at := r.Clone(r.Context())
	at.URL.Path, at.URL.RawPath = unescaped, path
	_, pattern := mux.Handler(at)
	return cmp.Or(pattern, notFoundPattern)
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
		redirect(w, r, withQuery(trimmed, r))
	})
}

// libraries lists the vetted libraries, and the libraries listings name too when its address asks.
func (s *server) libraries(w http.ResponseWriter, r *http.Request) {
	choices, ok := s.listChoices(w, r, domain.LibraryListPage, librariesHref)
	if !ok {
		return
	}
	libraries, err := s.catalog.Libraries(r.Context(), choices.Unvetted)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	cards := newLibraryCards(libraries, choices.Unvetted)
	s.render(w, r, http.StatusOK, librariesPage(s.pageChrome(librariesHref), cards, choices.Unvetted, s.listingAvailable()))
}

// redirect answers with a permanent redirect to target, cacheable as pages are.
func redirect(w http.ResponseWriter, r *http.Request, target string) {
	w.Header().Set("Cache-Control", pageCache)
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

// withoutParam returns the address u names without its query's param parameters, which a page takes off by
// redirecting. The rest of the query keeps its order, and the path the spelling the visitor's browser sent, which
// RawPath holds when it differs from Path's, so the address is the one the visitor left. Each name is compared
// decoded, as Query reads it, so a parameter whose name is encoded, such as %73tar, comes off too, and the redirect
// can't lead back to itself.
func withoutParam(u *url.URL, param string) string {
	var kept []string
	for pair := range strings.SplitSeq(u.RawQuery, "&") {
		name, _, _ := strings.Cut(pair, "=")
		if decoded, err := url.QueryUnescape(name); err != nil || decoded != param {
			kept = append(kept, pair)
		}
	}
	target := url.URL{Path: u.Path, RawPath: u.RawPath, RawQuery: strings.Join(kept, "&")}
	return target.String()
}

// withQuery returns target with r's query, if any.
func withQuery(target string, r *http.Request) string {
	if r.URL.RawQuery != "" {
		return target + "?" + r.URL.RawQuery
	}
	return target
}

// library shows a library's tab that the tab parameter names: its groups by default, with the ones the sel parameter
// ticks, its rules, with its retired rules in place when the retired parameter is 1, or its releases, starting at the
// release the until parameter names, if any. With releases to compare in the from and to parameters, the releases tab
// compares them.
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
		if !s.found(w, r, page.Library, "", err) {
			return
		}
		view := newLibraryView(page.Library)
		if tab != rulesTab {
			tab = groupsTab
		}
		contents := newLibraryContents(view, page, s.assets.iconURL)
		selection := newGroupSelection(query, contents.current())
		retired := tab == rulesTab && domain.RetiredChosen(query)
		s.render(w, r, http.StatusOK, libraryPage(s.pageChrome(view.href), view, contents, tab, selection, retired))
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
	if !s.found(w, r, page.Library, "", err) {
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
	if !s.found(w, r, comparison.Library, "", err) {
		return
	}
	view := newLibraryView(comparison.Library)
	s.render(w, r, http.StatusOK, releaseComparisonPage(s.chrome, view, newReleaseComparisonView(view, comparison, parseDiffMode(query.Get("view")))))
}

// ruleComparisonNotFound answers a link to versions the rule doesn't have with its Versions tab saying so, or the
// site's missing page when there's no such rule.
func (s *server) ruleComparisonNotFound(w http.ResponseWriter, r *http.Request) {
	page, err := s.catalog.RulePage(r.Context(), r.PathValue("owner"), r.PathValue("repo"), r.PathValue("rule"))
	if !s.found(w, r, page.Library, page.Rule.Path, err) {
		return
	}
	view, err := s.ruleView(r, page)
	if err != nil {
		s.fail(w, r, err)
		return
	}
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
// from and to parameters, the Versions tab compares them. Returning from signing in to star the rule, it prompts once
// to star it.
func (s *server) rule(w http.ResponseWriter, r *http.Request) {
	if s.withoutStarPrompt(w, r) {
		return
	}
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
	view, err := s.ruleView(r, page)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, rulePage(s.pageChrome(view.href), view, tab))
}

// libraryGroup shows one group of a library, whose ID the kind and group wildcards hold, with the box that adds it to
// the cart, and its links back carrying the groups the sel parameter ticks, or the missing page when the library has
// no current rules in such a group. A group spelled in another case redirects to the library's spelling, as a rule
// does.
func (s *server) libraryGroup(w http.ResponseWriter, r *http.Request) {
	page, err := s.catalog.LibraryPage(r.Context(), r.PathValue("owner"), r.PathValue("repo"))
	if err != nil {
		s.found(w, r, page.Library, "", err)
		return
	}
	view := newLibraryView(page.Library)
	contents := newLibraryContents(view, page, s.assets.iconURL)
	group, ok := contents.group(pathInLibrary(r))
	if !ok {
		s.notFound(w, r)
		return
	}
	if !s.found(w, r, page.Library, group.label.id, nil) {
		return
	}
	selection := newGroupSelection(r.URL.Query(), contents.current())
	s.render(w, r, http.StatusOK, libraryGroupPage(s.pageChrome(libraryGroupHref(view.href, group.label.id)), view, group, selection))
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
	view, err := s.ruleView(r, comparison.Page)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, ruleComparisonPage(s.chrome, view, newRuleComparisonView(view, comparison, parseDiffMode(query.Get("view")))))
}

// ruleView describes the rule on page for the page r asks for, with its star control for the visitor.
func (s *server) ruleView(r *http.Request, page views.RulePage) (ruleView, error) {
	view := s.ruleViewWithoutStar(page)
	var err error
	if view.star, err = s.starControl(r, view, page.Rule.Stars); err != nil {
		return ruleView{}, err
	}
	return view, nil
}

// ruleViewWithoutStar describes the rule on page, with its group's icon, for a page that shows no star control, such as
// an asset's.
func (s *server) ruleViewWithoutStar(page views.RulePage) ruleView {
	view := newRuleView(newLibraryView(page.Library), page)
	view.groupIcon = newGroupIcon(page.Rule.CanonicalGroup, s.assets.iconURL)
	return view
}

// pageChrome returns the frame for the page whose own address is href, the path its links use, which it names on
// BaseURL as its canonical address. Built from what the page shows rather than the request, every spelling of the
// page's path, such as a percent-encoded one, names the same address. A tab's query string shows the same page, so
// the address leaves it out.
func (s *server) pageChrome(href string) chrome {
	c := s.chrome
	if s.BaseURL != nil {
		c.canonical = s.BaseURL.String() + href
		c.socialImage = s.BaseURL.String() + s.assets.url("social.png")
	}
	return c
}

// found reports whether a page's data loaded, for the library lib and, on a rule's or a library group's page, the rule
// or group at rulePath, and is at the path that GitHub's spelling of the library's owner and name, and the library's of
// the rule's or group's ID, give. Otherwise it answers the request itself: with a missing page, a failure, or a
// redirect to that path.
func (s *server) found(w http.ResponseWriter, r *http.Request, lib views.Library, rulePath string, err error) bool {
	if errors.Is(err, app.ErrNotFound) {
		s.notFound(w, r)
		return false
	}
	if err != nil {
		s.fail(w, r, err)
		return false
	}
	if lib.Owner != r.PathValue("owner") || lib.Name != r.PathValue("repo") || rulePath != pathInLibrary(r) {
		canonical := url.URL{Path: libraryHref(lib.Owner, lib.Name), RawQuery: r.URL.RawQuery}
		if rulePath != "" {
			canonical.Path += "/" + rulePath
		}
		redirect(w, r, canonical.String())
		return false
	}
	return true
}

// pathInLibrary returns the ID of the rule or group whose page r asks for, as its path spells it, or "" for any other
// page.
func pathInLibrary(r *http.Request) string {
	if kind := r.PathValue("kind"); kind != "" {
		return kind + "/" + r.PathValue("group")
	}
	return r.PathValue("rule")
}

func (s *server) notFound(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusNotFound, messagePage(s.chrome, "Not found", "Rulemart has no page here."))
}

// fail logs err, as logFailure does, and answers with a page that reveals nothing about the failure.
func (s *server) fail(w http.ResponseWriter, r *http.Request, err error) {
	s.logFailure(r, err)
	s.unavailable(w, r)
}

// logFailure logs err, which fails r with 503, with the request's route pattern and ID, and without what r's path
// names.
func (s *server) logFailure(r *http.Request, err error) {
	s.Log.ErrorContext(r.Context(), "request failed", "route", s.route(r), "method", r.Method, "requestID", s.requestID(r),
		"status", http.StatusServiceUnavailable, "error", withoutPath(err.Error(), r))
}

// withoutPath returns text with the owner, library, or rule that r's path names replaced by the route's wildcards.
// Page reads name what they failed to read as owner/repo or owner/repo/rule, or as a quoted login, and those come
// from the visitor, so the logs keep only the route, as the access log does.
func withoutPath(text string, r *http.Request) string {
	owner, repo, rule := cmp.Or(r.PathValue("owner"), r.PathValue("login")), r.PathValue("repo"), r.PathValue("rule")
	if owner == "" {
		return text
	}
	if repo == "" {
		return strings.ReplaceAll(text, strconv.Quote(owner), "{owner}")
	}
	if _, asset, ok := requestedAsset(r); ok {
		text = strings.ReplaceAll(text, owner+"/"+repo+"/"+asset, "{owner}/{repo}/{asset...}")
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
