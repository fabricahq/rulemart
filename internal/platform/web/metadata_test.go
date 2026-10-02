package web_test

import (
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// metas returns the content of each meta element in body, keyed by its property, or its name when it has none.
func metas(t *testing.T, body string) map[string]string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for n := range doc.Descendants() {
		if n.Type == html.ElementNode && n.Data == "meta" {
			key := attribute(n, "property")
			if key == "" {
				key = attribute(n, "name")
			}
			found[key] = attribute(n, "content")
		}
	}
	return found
}

// A link to a page search engines may index shows its title, description, and address on social sites and in chat,
// with Rulemart's image, which the site serves.
func TestIndexablePagesDescribeThemselvesToSocialSites(t *testing.T) {
	c := unvettedCatalog()
	handler := newSiteAt(t, c, "https://rulemart.example")

	for _, path := range []string{"/", "/libraries", "/groups", library, library + "?tab=rules", errorsRule} {
		page := get(t, handler, path).Body.String()
		meta := metas(t, page)
		canonical := canonicalLinks(t, page)
		if len(canonical) != 1 || meta["og:url"] != canonical[0] {
			t.Errorf("%s: og:url %q, canonical %q", path, meta["og:url"], canonical)
		}
		if meta["og:title"] == "" || meta["og:description"] == "" || meta["og:description"] != meta["description"] ||
			meta["og:site_name"] != "Rulemart" || meta["twitter:card"] != "summary_large_image" {
			t.Errorf("%s: %v", path, meta)
		}
		image, ok := strings.CutPrefix(meta["og:image"], "https://rulemart.example/")
		if !ok {
			t.Errorf("%s: og:image %q isn't on the public origin", path, meta["og:image"])
			continue
		}
		if resp := get(t, handler, "/"+image); resp.Code != http.StatusOK || resp.Header().Get("Content-Type") != "image/png" {
			t.Errorf("%s: the image answered %d, %q", path, resp.Code, resp.Header().Get("Content-Type"))
		}
	}
}

// A page with no address of its own, an unvetted library's, a search, a comparison, or a missing page, shows no
// card, so sharing one borrows nothing of Rulemart's; and without a public origin no page has one.
func TestPagesWithoutAnAddressOfTheirOwnShowNoSocialCard(t *testing.T) {
	handler := newSiteAt(t, unvettedCatalog(), "https://rulemart.example")
	for _, path := range []string{
		unvettedLibrary, unvettedLibrary + "/techs/go/return-errors", "/unvetted", "/search?q=errors",
		library + "?tab=releases&from=1&to=3", "/example/missing",
	} {
		if meta := metas(t, get(t, handler, path).Body.String()); meta["og:url"] != "" || meta["og:image"] != "" {
			t.Errorf("%s: shows a card: %v", path, meta)
		}
	}
	if meta := metas(t, get(t, newSite(t, unvettedCatalog()), library).Body.String()); meta["og:url"] != "" {
		t.Errorf("without a base URL, a library shows a card: %v", meta)
	}
}

// A library without a description of its own, or a rule without reading guidance, still describes itself, and a
// long description is cut at a word, short enough for a search result.
func TestPageDescriptionsFallBackAndStayShort(t *testing.T) {
	c := newCatalog()
	page := c.pages["example/rules"]
	page.Library.Description = ""
	c.pages["example/rules"] = page
	rule := c.rules["example/rules/techs/go/return-errors"]
	rule.Rule.WhenToRead, rule.Rule.WhenToReadHTML = strings.Repeat("When changing errors and their wrapping. ", 20), ""
	c.rules["example/rules/techs/go/return-errors"] = rule
	handler := newSite(t, c)

	if got := metas(t, get(t, handler, library).Body.String())["description"]; got != "example/rules: 2 rules for coding agents, in a Code Rules library on Rulemart." {
		t.Errorf("the library's description is %q", got)
	}
	got := metas(t, get(t, handler, errorsRule).Body.String())["description"]
	cut, ok := strings.CutSuffix(got, "…")
	if n := utf8.RuneCountInString(got); !ok || n > 200 || n < 150 || !strings.HasPrefix(rule.Rule.WhenToRead, cut+" ") {
		t.Errorf("the rule's description is %d characters, not cut at a word: %q", n, got)
	}
}

// Browsers ask for /favicon.ico at the root, and phones for the touch icon a page names.
func TestTheSiteServesItsIcons(t *testing.T) {
	handler := newSite(t, newCatalog())

	if resp := get(t, handler, "/favicon.ico"); resp.Code != http.StatusOK || resp.Header().Get("Content-Type") != "image/x-icon" {
		t.Errorf("/favicon.ico answered %d, %q", resp.Code, resp.Header().Get("Content-Type"))
	}
	doc, err := html.Parse(get(t, handler, "/").Body)
	if err != nil {
		t.Fatal(err)
	}
	touch := find(doc, func(n *html.Node) bool { return n.Data == "link" && attribute(n, "rel") == "apple-touch-icon" })
	if touch == nil {
		t.Fatal("the page names no touch icon")
	}
	if resp := get(t, handler, attribute(touch, "href")); resp.Code != http.StatusOK || resp.Header().Get("Content-Type") != "image/png" {
		t.Errorf("the touch icon answered %d, %q", resp.Code, resp.Header().Get("Content-Type"))
	}
}
