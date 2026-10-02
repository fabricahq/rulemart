package web_test

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// withLibraryAt returns c with newCatalog's library and its rules also vetted as owner/name.
func withLibraryAt(c catalog, owner, name string) catalog {
	lib := exampleRules
	lib.Owner, lib.Name = owner, name
	key := strings.ToLower(owner + "/" + name)
	page := c.pages["example/rules"]
	page.Library = lib
	c.pages[key] = page
	for k, rule := range maps.Clone(c.rules) {
		if path, ok := strings.CutPrefix(k, "example/rules/"); ok {
			rule.Library = lib
			c.rules[key+"/"+path] = rule
		}
	}
	c.libraries = append(c.libraries, views.LibraryCard{Owner: owner, Name: name, Description: "A library owned by " + owner + ".", Rules: 2})
	return c
}

// follow requests path from handler and follows its redirects to the response that isn't one, which it returns with
// every address it reached. More than 5 redirects fail the test, as a loop would.
func follow(t *testing.T, handler http.Handler, path string) (*httptest.ResponseRecorder, []string) {
	t.Helper()
	hops := []string{path}
	for range 6 {
		resp := get(t, handler, path)
		if resp.Code < 300 || resp.Code > 399 {
			return resp, hops
		}
		path = resp.Header().Get("Location")
		hops = append(hops, path)
	}
	t.Fatalf("%s redirects past 5 hops: %q", hops[0], hops)
	return nil, nil
}

// A library whose owner's login is a section's name in another case, such as G, has its page at that spelling; the
// lowercase redirect of the site's sections leaves it alone rather than send it back and forth with the library's
// own redirect to GitHub's spelling. Each spelling of a site page still reaches it.
func TestSectionSpellingsReachTheirPageWithoutALoop(t *testing.T) {
	c := withLibraryAt(newBrowsingCatalog(), "G", "rules")
	c.libraries = append(c.libraries, views.LibraryCard{Owner: "faq", Name: "rules", Description: "A library owned by faq.", Rules: 1})
	handler := newSite(t, c)

	for path, want := range map[string]string{
		"/G/rules":                        "/G/rules",
		"/g/rules":                        "/G/rules",
		"/G/rules?tab=rules":              "/G/rules?tab=rules",
		"/G/rules/techs/go/return-errors": "/G/rules/techs/go/return-errors",
		"/g/RULES/techs/go/return-errors": "/G/rules/techs/go/return-errors",
		"/Browse/techs":                   "/browse/techs",
		"/browse/Techs":                   "/browse/techs",
		"/G/Techs/GO":                     "/g/techs/go",
		"/Groups/techs/go":                "/g/techs/go",
		"/O/faq":                          "/o/faq",
		"/O/FAQ":                          "/o/faq",
	} {
		resp, hops := follow(t, handler, path)
		if resp.Code != http.StatusOK || hops[len(hops)-1] != want {
			t.Errorf("%s: reached %d at %q, want 200 at %q", path, resp.Code, hops, want)
		}
	}
}
