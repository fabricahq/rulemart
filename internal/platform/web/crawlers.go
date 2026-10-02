// What crawlers read: robots.txt, which keeps them out of pages that are a visitor's own or that queries multiply,
// and the sitemap, which lists every page search engines may index by its canonical address.

package web

import (
	"bytes"
	"encoding/xml"
	"net/http"
	"strings"
	"time"
)

const (
	robotsHref  = "/robots.txt"
	sitemapHref = "/sitemap.xml"
)

// disallowed are the paths robots.txt keeps crawlers out of: a visitor's own pages and the actions that take POST,
// signing in, listing, search, whose every query would be a page, the unvetted area, whose libraries' pages also say
// noindex, and comparisons, whose every pair of releases or versions would be a page. Each of these pages also asks
// not to be indexed, for a crawler that ignores robots.txt.
var disallowed = []string{accountHref + "/", signInHref, listHref, searchHref, unvettedHref, "/*from="}

// robots answers GET /robots.txt, naming the sitemap when there's a public origin to name it on.
func (s *server) robots(w http.ResponseWriter, r *http.Request) {
	var body strings.Builder
	body.WriteString("User-agent: *\n")
	for _, path := range disallowed {
		body.WriteString("Disallow: " + path + "\n")
	}
	if s.BaseURL != nil {
		body.WriteString("\nSitemap: " + s.BaseURL.String() + sitemapHref + "\n")
	}
	writeFile(w, r, "text/plain; charset=utf-8", []byte(body.String()))
}

// sitemapURL is one address in a sitemap. LastMod is empty when the page has no one date it changed.
type sitemapURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

// sitemapURLSet is a sitemap file, as https://www.sitemaps.org/protocol.html defines it.
type sitemapURLSet struct {
	XMLName xml.Name     `xml:"http://www.sitemaps.org/schemas/sitemap/0.9 urlset"`
	URLs    []sitemapURL `xml:"url"`
}

// sitemap answers GET /sitemap.xml with every page search engines may index, by its canonical address: the site's
// own pages, each canonical group's, and each vetted library's and its current rules'. Never an unvetted library, a
// search, or a comparison. Its addresses must be absolute, so without a public origin there's no sitemap.
func (s *server) sitemap(w http.ResponseWriter, r *http.Request) {
	if s.BaseURL == nil {
		s.notFound(w, r)
		return
	}
	sitemap, err := s.catalog.Sitemap(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if sitemap.Truncated {
		s.Log.WarnContext(r.Context(), "sitemap truncated", "route", s.route(r), "requestID", s.requestID(r))
	}
	base := s.BaseURL.String()
	set := sitemapURLSet{}
	add := func(href string, updated time.Time) {
		u := sitemapURL{Loc: base + href}
		if !updated.IsZero() {
			u.LastMod = updated.UTC().Format(time.DateOnly)
		}
		set.URLs = append(set.URLs, u)
	}
	for _, href := range []string{"/", librariesHref, groupsHref} {
		add(href, time.Time{})
	}
	for _, id := range sitemap.Groups {
		add(groupHref(id), time.Time{})
	}
	for _, lib := range sitemap.Libraries {
		href := libraryHref(lib.Owner, lib.Name)
		add(href, lib.Updated)
		for _, rule := range lib.Rules {
			add(href+"/"+rule.Path, rule.Updated)
		}
	}
	var body bytes.Buffer
	body.WriteString(xml.Header)
	if err := xml.NewEncoder(&body).Encode(set); err != nil {
		s.fail(w, r, err)
		return
	}
	writeFile(w, r, "application/xml; charset=utf-8", body.Bytes())
}

// writeFile answers with content, a file that's the same for every visitor, of the media type contentType, cacheable
// as pages are.
func writeFile(w http.ResponseWriter, r *http.Request, contentType string, content []byte) {
	header := w.Header()
	header.Set("Content-Type", contentType)
	header.Set("Cache-Control", pageCache)
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(content)
	}
}
