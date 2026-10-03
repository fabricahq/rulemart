// What crawlers read: robots.txt, which keeps them out of pages that are a visitor's own or that queries multiply,
// and the sitemap, which lists every page search engines may index by its canonical address.

package web

import (
	"bytes"
	"encoding/xml"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

const (
	robotsHref  = "/robots.txt"
	sitemapHref = "/sitemap.xml"
)

// disallowed are the paths robots.txt keeps crawlers out of: a visitor's own pages and the actions that take POST,
// the cart, whose page each browser fills from what it keeps, signing in, listing, search, whose every query would be a page, the unvetted area, whose libraries' pages also say
// noindex, and comparisons, whose every pair of releases or versions would be a page. Each of these pages also asks
// not to be indexed, for a crawler that ignores robots.txt. A rule matches any path it starts, so each one-segment
// page is disallowed alone and with a query, by $ and ?: /me alone would also keep crawlers off /meta/rules, a
// library's page.
var disallowed = func() []string {
	rules := []string{accountHref + "/", dashboardHref + "/"}
	for _, page := range []string{accountHref, dashboardHref, cartHref, signInHref, legacySignInHref, legacyListHref, searchHref, unvettedHref} {
		rules = append(rules, page+"$", page+"?")
	}
	return append(rules, "/*from=")
}()

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
	writeFile(w, r, "text/plain; charset=utf-8", pageCache, []byte(body.String()))
}

// newSitemapFile returns the sitemap file listing sitemap's pages on base, the site's own first, then each group's,
// then each owner's, then each library's, unless one of the site's pages takes its address, and its groups' and rules',
// each group's page before its rules, within maxBytes, and whether it lists them all: it stops before the address that would pass maxBytes, since a Lambda
// function's response holds at most 6 MB.
func newSitemapFile(base string, sitemap views.Sitemap, maxBytes int) ([]byte, bool) {
	const open = `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`
	const end = "</urlset>\n"
	var body, entry bytes.Buffer
	body.WriteString(xml.Header + open)
	encoder := xml.NewEncoder(&entry)
	add := func(href string, updated time.Time) bool {
		u := sitemapURL{Loc: base + href}
		if !updated.IsZero() {
			u.LastMod = updated.UTC().Format(time.DateOnly)
		}
		entry.Reset()
		// Encoding a struct of two strings can't fail.
		_ = encoder.EncodeElement(u, xml.StartElement{Name: xml.Name{Local: "url"}})
		_ = encoder.Flush()
		if body.Len()+entry.Len()+len(end) > maxBytes {
			return false
		}
		body.Write(entry.Bytes())
		return true
	}
	complete := func() bool {
		pages := []string{"/", librariesHref}
		for _, kind := range groupKinds {
			pages = append(pages, kind.href(), kind.othersHref())
		}
		for _, href := range append(pages, aboutHref, privacyHref, faqHref, feedbackHref) {
			if !add(href, time.Time{}) {
				return false
			}
		}
		for _, id := range sitemap.Groups {
			if !add(groupHref(id), time.Time{}) {
				return false
			}
		}
		for _, login := range owners(sitemap.Libraries) {
			if !add(ownerHref(login), time.Time{}) {
				return false
			}
		}
		for _, lib := range sitemap.Libraries {
			href := libraryHref(lib.Owner, lib.Name)
			if !libraryPageTaken(lib.Owner, lib.Name) && !add(href, lib.Updated) {
				return false
			}
			for i, rule := range lib.Rules {
				// Rules come in path order, so a group's rules are together: its page goes before the first of them.
				if group := path.Dir(rule.Path); i == 0 || path.Dir(lib.Rules[i-1].Path) != group {
					if !add(libraryGroupHref(href, group), groupUpdated(lib.Rules[i:], group)) {
						return false
					}
				}
				if !add(href+"/"+rule.Path, rule.Updated) {
					return false
				}
			}
		}
		return true
	}()
	body.WriteString(end)
	return body.Bytes(), complete
}

// groupUpdated returns when the latest of the group's rules at the start of rules changed.
func groupUpdated(rules []views.SitemapRule, group string) time.Time {
	var updated time.Time
	for _, rule := range rules {
		if path.Dir(rule.Path) != group {
			break
		}
		if rule.Updated.After(updated) {
			updated = rule.Updated
		}
	}
	return updated
}

// owners returns each owner of libraries once, in the libraries' order, spelled as the first of their libraries
// spells them. Libraries ingested at different times may spell one owner in different cases; the owner's page
// matches without regard to case and redirects every other spelling to the first, so the sitemap names that one.
func owners(libraries []views.SitemapLibrary) []string {
	var logins []string
	for _, lib := range libraries {
		if n := len(logins); n == 0 || !strings.EqualFold(logins[n-1], lib.Owner) {
			logins = append(logins, lib.Owner)
		}
	}
	return logins
}

// sitemapURL is one address in a sitemap. LastMod is empty when the page has no one date it changed.
type sitemapURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

// sitemap answers GET /sitemap.xml with every page search engines may index, by its canonical address: the site's
// own pages, each group's of a vetted library, each owner's, and each vetted library's and its groups' and current
// rules'. Never an unvetted library, a search, a comparison, a rule's asset, or a visitor's own page. Its addresses must be absolute, so without a public origin there's no sitemap.
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
	body, complete := newSitemapFile(s.BaseURL.String(), sitemap, maxPageBytes)
	if sitemap.Truncated || !complete {
		s.Log.WarnContext(r.Context(), "sitemap truncated", "route", s.route(r), "requestID", s.requestID(r))
	}
	writeFile(w, r, "application/xml; charset=utf-8", pageCache, body)
}
