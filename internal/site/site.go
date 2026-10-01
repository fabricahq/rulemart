// Package site serves Rulemart's pages: the vetted libraries, each library's groups and rules, and each rule's
// current version and version history. It reads the catalog that ingestion writes, and shows only the libraries
// in the vetted list it's given.
package site

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/a-h/templ"
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
	// Log receives the details of failures that pages leave out.
	Log *slog.Logger
	// RequestID returns an identifier for a request that the logs record, such as the Lambda request ID. It may
	// be nil.
	RequestID func(*http.Request) string
}

// server answers page requests.
type server struct {
	store  *Store
	assets *assets
	chrome chrome
	Options
}

// New returns the handler for Rulemart's pages, reading the catalog from store.
func New(store *Store, options Options) (http.Handler, error) {
	assets, err := newAssets()
	if err != nil {
		return nil, err
	}
	s := &server{
		store: store, assets: assets, Options: options,
		chrome: chrome{
			stylesheet: assets.url("app.css"), script: assets.url("theme.js"), icon: assets.url("favicon.svg"),
			font: assets.url("fonts/inter-latin.woff2"),
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /_static/{version}/{file...}", assets.serve)
	mux.HandleFunc("GET /{owner}/{repo}", s.library)
	mux.HandleFunc("GET /{owner}/{repo}/{rule...}", s.rule)
	mux.HandleFunc("/", s.notFound)
	return withSecurityHeaders(mux), nil
}

func (s *server) home(w http.ResponseWriter, r *http.Request) {
	libraries, err := s.store.libraries(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, homePage(s.chrome, newLibraryCards(libraries)))
}

func (s *server) library(w http.ResponseWriter, r *http.Request) {
	lib, ok := s.findLibrary(w, r)
	if !ok {
		return
	}
	contents, err := s.store.contents(r.Context(), lib.githubID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	view := newLibraryView(lib)
	s.render(w, r, http.StatusOK, libraryPage(s.chrome, view, newLibraryContents(view, contents), r.URL.Query().Get("tab") == "rules"))
}

func (s *server) rule(w http.ResponseWriter, r *http.Request) {
	lib, ok := s.findLibrary(w, r)
	if !ok {
		return
	}
	found, err := s.store.rule(r.Context(), lib.githubID, r.PathValue("rule"))
	if errors.Is(err, errNotFound) {
		s.notFound(w, r)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, rulePage(s.chrome, newRuleView(newLibraryView(lib), found), r.URL.Query().Get("tab") == "versions"))
}

// findLibrary returns the vetted library the request's path names. When there's none, or the path spells its
// owner or name differently from GitHub, it answers the request itself, with a missing page or a redirect to the
// path as GitHub spells it, and reports false.
func (s *server) findLibrary(w http.ResponseWriter, r *http.Request) (library, bool) {
	owner, name := r.PathValue("owner"), r.PathValue("repo")
	lib, err := s.store.library(r.Context(), owner, name)
	if errors.Is(err, errNotFound) {
		s.notFound(w, r)
		return library{}, false
	}
	if err != nil {
		s.fail(w, r, err)
		return library{}, false
	}
	if lib.owner != owner || lib.name != name {
		canonical := url.URL{Path: libraryHref(lib.owner, lib.name), RawQuery: r.URL.RawQuery}
		if rule := r.PathValue("rule"); rule != "" {
			canonical.Path += "/" + rule
		}
		w.Header().Set("Cache-Control", pageCache)
		http.Redirect(w, r, canonical.String(), http.StatusMovedPermanently)
		return library{}, false
	}
	return lib, true
}

func (s *server) notFound(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusNotFound, messagePage(s.chrome, "Not found", "Rulemart has no page here."))
}

// fail logs err with the request's route and ID, and answers with a page that reveals nothing about the failure.
func (s *server) fail(w http.ResponseWriter, r *http.Request, err error) {
	requestID := ""
	if s.RequestID != nil {
		requestID = s.RequestID(r)
	}
	s.Log.ErrorContext(r.Context(), "request failed", "route", r.URL.Path, "method", r.Method, "requestID", requestID,
		"status", http.StatusServiceUnavailable, "error", err.Error())
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
