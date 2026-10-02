package web_test

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// An owner's page shows their login as GitHub spells it, their avatar, and their vetted libraries, at /{owner}, named
// as its canonical address, and the header marks Libraries current there.
func TestOwnerPageShowsTheOwnersVettedLibraries(t *testing.T) {
	c := newBrowsingCatalog()
	c.libraries = append(c.libraries, views.LibraryCard{Owner: "example", Name: "web-rules", Description: "Rules for the web.", Rules: 4})
	handler := newSiteAt(t, c, "https://rulemart.example")

	resp := get(t, handler, "/example")

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d", resp.Code)
	}
	page := resp.Body.String()
	assertShows(t, page, "example github.com/example Libraries 2",
		"rules Vetted by Rulemart Example rules for tests. example/rules · 2 rules", "web-rules Rules for the web. example/web-rules · 4 rules")
	if !strings.Contains(page, "<title>example · Rulemart</title>") {
		t.Error("the title doesn't name the owner")
	}
	if !strings.Contains(page, `src="`+exampleRef.OwnerAvatarURL+`"`) {
		t.Error("the page doesn't show the owner's avatar")
	}
	if got := headings(t, page, "h1"); !slices.Equal(got, []string{"example"}) {
		t.Errorf("the page is headed %q", got)
	}
	if got := links(t, page, "Rules for the web."); !slices.Equal(got, []string{"/example/web-rules"}) {
		t.Errorf("the library links %q", got)
	}
	if got := canonicalLinks(t, page); !slices.Equal(got, []string{"https://rulemart.example/example"}) {
		t.Errorf("names %q as canonical", got)
	}
}

// An owner has a page only with a vetted library: a listed, unvetted library gives its owner none, so listing a
// repository can't create a page under Rulemart's address.
func TestOwnerPageIsMissingWithoutAVettedLibrary(t *testing.T) {
	handler := newSite(t, unvettedCatalog())

	if resp := get(t, handler, "/example"); resp.Code != http.StatusOK {
		t.Errorf("the vetted owner's page answered %d", resp.Code)
	}
	for _, path := range []string{"/stranger", "/nobody"} {
		if resp := get(t, handler, path); resp.Code != http.StatusNotFound {
			t.Errorf("%s: got %d, want 404", path, resp.Code)
		}
	}
}

// An owner's page has one address, GitHub's spelling of the login, and /o/{login} is that address only for an owner
// whose login is one of the site's own pages; every other login's /o/ address redirects to /{login}, whether or not
// an owner has a page there.
func TestOwnerPagesRedirectOtherAddressesToTheirOwn(t *testing.T) {
	c := newBrowsingCatalog()
	c.libraries = append(c.libraries, views.LibraryCard{Owner: "faq", Name: "rules", Description: "A library owned by faq.", Rules: 1})
	handler := newSiteAt(t, c, "https://rulemart.example")

	for path, location := range map[string]string{
		"/Example":        "/example",
		"/EXAMPLE?x=1":    "/example?x=1",
		"/o/example":      "/example",
		"/o/Example?x=1":  "/Example?x=1",
		"/o/stranger":     "/stranger",
		"/o/FAQ":          "/o/faq",
		"/example/":       "/example",
		"/o/nobody":       "/nobody",
		"/o/nobody-else/": "/o/nobody-else",
	} {
		resp := get(t, handler, path)
		if resp.Code != http.StatusMovedPermanently || resp.Header().Get("Location") != location {
			t.Errorf("%s: got %d to %q, want 301 to %q", path, resp.Code, resp.Header().Get("Location"), location)
		}
	}
	// The owner named faq has their page under /o/, and the FAQ keeps /faq; their library keeps /faq/rules.
	reserved := get(t, handler, "/o/faq")
	if reserved.Code != http.StatusOK {
		t.Fatalf("/o/faq: got %d", reserved.Code)
	}
	assertShows(t, reserved.Body.String(), "faq github.com/faq Libraries 1", "rules A library owned by faq. faq/rules · 1 rule")
	if got := canonicalLinks(t, reserved.Body.String()); !slices.Equal(got, []string{"https://rulemart.example/o/faq"}) {
		t.Errorf("/o/faq names %q as canonical", got)
	}
	if got := links(t, reserved.Body.String(), "A library owned by faq."); !slices.Equal(got, []string{"/faq/rules"}) {
		t.Errorf("the library links %q", got)
	}
	if page := get(t, handler, "/faq").Body.String(); !strings.Contains(page, "<title>FAQ · Rulemart</title>") {
		t.Error("/faq isn't the FAQ")
	}
}

// A failed owner read is logged with its route, never with the login someone asked for.
func TestOwnerPageFailureLeavesTheLoginOutOfTheLogs(t *testing.T) {
	handler, logs := loggedSite(t, catalog{err: errors.New(`load owner libraries login="secret-login": connection refused`)})

	resp := get(t, handler, "/secret-login")

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d", resp.Code)
	}
	if !strings.Contains(logs.String(), `"route":"/{owner}"`) || strings.Contains(logs.String(), "secret-login") {
		t.Fatalf("the logs are %s", logs)
	}
}

// A vetted library's About panel leads to its owner's page, and its repository to GitHub. An unvetted library's
// owner has no page, so its owner leads to GitHub instead.
func TestLibraryPageLeadsToItsOwner(t *testing.T) {
	handler := newSite(t, unvettedCatalog())

	for path, want := range map[string]string{
		"/example/rules":  "/example",
		"/stranger/rules": "https://github.com/stranger",
	} {
		page := get(t, handler, path).Body.String()
		owner := strings.TrimPrefix(path, "/")
		owner = owner[:strings.Index(owner, "/")]
		if got := links(t, page, owner); !slices.Contains(got, want) {
			t.Errorf("%s: the owner links %q, want %s", path, got, want)
		}
		if got := links(t, page, "rules"); !slices.Contains(got, "https://github.com"+path) {
			t.Errorf("%s: the repository links %q, want GitHub's", path, got)
		}
	}
}
