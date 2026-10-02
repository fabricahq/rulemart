// Package web serves Rulemart's pages: the vetted libraries, each library's groups and rules, and each rule's
// current version and version history. It reads them from the catalog's page reads, which app.Pages implements.
package web

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/a-h/templ"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// pageCache lets CloudFront and browsers keep a page for a minute, so a new library release shows within a minute
// of ingestion without every visit reaching the function.
const pageCache = "public, max-age=60"

// contentSecurityPolicy allows only Rulemart's own files, plus images from GitHub's avatar and raw file hosts,
// which rules and library owners use. Rule content comes from repositories Rulemart doesn't control, so nothing
// else may load or run.
const contentSecurityPolicy = "default-src 'none'; script-src 'self'; style-src 'self'; font-src 'self'; " +
	"img-src 'self' https://avatars.githubusercontent.com https://raw.githubusercontent.com; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// Options configures the handler.
type Options struct {
	// Log receives one line for each request, and the details of failures that pages leave out.
	Log *slog.Logger
	// RequestID returns an identifier for a request that the logs record, such as the Lambda request ID. It may
	// be nil.
	RequestID func(*http.Request) string
}

// Catalog reads what the pages show. app.Pages implements it, finding only the vetted libraries.
type Catalog interface {
	Libraries(ctx context.Context) ([]views.LibraryCard, error)
	// LibraryPage and RulePage fail with app.ErrNotFound when there's no such library or current rule.
	LibraryPage(ctx context.Context, owner, name string) (views.LibraryPage, error)
	RulePage(ctx context.Context, owner, name, rulePath string) (views.RulePage, error)
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
	handle("GET /{owner}/{repo}", s.library)
	handle("GET /{owner}/{repo}/{rule...}", s.rule)
	handle("/", s.notFound)
	return s.logRequests(withSecurityHeaders(mux)), nil
}

func (s *server) home(w http.ResponseWriter, r *http.Request) {
	libraries, err := s.catalog.Libraries(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, homePage(s.chrome, newLibraryCards(libraries)))
}

func (s *server) library(w http.ResponseWriter, r *http.Request) {
	page, err := s.catalog.LibraryPage(r.Context(), r.PathValue("owner"), r.PathValue("repo"))
	if !s.found(w, r, page.Library, err) {
		return
	}
	view := newLibraryView(page.Library)
	contents := newLibraryContents(view, page, s.assets.iconURL)
	s.render(w, r, http.StatusOK, libraryPage(s.chrome, view, contents, r.URL.Query().Get("tab") == "rules"))
}

func (s *server) rule(w http.ResponseWriter, r *http.Request) {
	page, err := s.catalog.RulePage(r.Context(), r.PathValue("owner"), r.PathValue("repo"), r.PathValue("rule"))
	if !s.found(w, r, page.Library, err) {
		return
	}
	s.render(w, r, http.StatusOK, rulePage(s.chrome, newRuleView(newLibraryView(page.Library), page), r.URL.Query().Get("tab") == "versions"))
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
		w.Header().Set("Cache-Control", pageCache)
		http.Redirect(w, r, canonical.String(), http.StatusMovedPermanently)
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
// leaves no partial page.
func (s *server) render(w http.ResponseWriter, r *http.Request, status int, page templ.Component) {
	var body bytes.Buffer
	if err := page.Render(r.Context(), &body); err != nil {
		s.fail(w, r, err)
		return
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
