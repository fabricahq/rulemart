package web_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

func TestHomeListsTheLibraries(t *testing.T) {
	handler := newSite(t, newCatalog())

	resp := get(t, handler, "/")

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d", resp.Code)
	}
	assertShows(t, resp.Body.String(), "Libraries", "rules Example rules for tests.", "example/rules · 2 rules")
}

func TestHomeLeadsWithSearchAndBrowsesCanonicalGroups(t *testing.T) {
	page := get(t, newSite(t, newBrowsingCatalog()), "/").Body.String()

	assertShows(t, page, "Popular Go Testing", "Technologies Browse all → Go 3 rules · 2 libraries", "Practices Browse all → Testing 1 rule · 1 library")
	if got := links(t, page, "Go"); !slices.Equal(got, []string{"/g/techs/go", "/g/techs/go"}) {
		t.Errorf("Go's chip and tile link %q", got)
	}
	if got := links(t, page, "Browse all"); !slices.Equal(got, []string{"/browse/techs", "/browse/practices"}) {
		t.Errorf("the bands lead to %q", got)
	}
	// A group that isn't canonical stands alone, so the home page's tiles, which lead across libraries, leave it out.
	if strings.Contains(visibleText(t, page), "golang") {
		t.Error("the home page shows a group that isn't canonical")
	}
	assertSearchForm(t, page, "home-search", "")
}

// Naming the popular groups leaves the tiles whole: with many technologies, the practices named as popular once
// overwrote the third and fourth technology tiles.
func TestHomeTilesStayWholeWhenPopularGroupsAreNamed(t *testing.T) {
	c := newBrowsingCatalog()
	var techs []views.GroupSummary
	for i, name := range []string{"Go", "Rust", "TypeScript", "React", "Python", "Docker", "Terraform", "Kubernetes", "Next.js"} {
		techs = append(techs, views.GroupSummary{Path: "techs/" + strings.ToLower(name), Canonical: &views.CanonicalGroup{Name: name, Icon: goGroup.Icon},
			Rules: 20 - i, Libraries: []views.LibraryRef{exampleRef}})
	}
	c.index = views.GroupIndex{Techs: techs, Practices: []views.GroupSummary{
		{Path: "practices/testing", Canonical: &views.CanonicalGroup{Name: "Testing", Icon: goGroup.Icon}, Rules: 30, Libraries: []views.LibraryRef{exampleRef}},
		{Path: "practices/comments", Canonical: &views.CanonicalGroup{Name: "Comments", Icon: goGroup.Icon}, Rules: 29, Libraries: []views.LibraryRef{exampleRef}},
	}}

	page := get(t, newSite(t, c), "/").Body.String()

	assertShows(t, page, "Popular Go Rust Testing Comments", "Technologies Browse all → Go 20 rules · 1 library Rust 19 rules · 1 library TypeScript 18 rules · 1 library React 17 rules · 1 library")
}

// The home page shows the first four libraries, in the catalog's order, and leads to them all.
func TestHomeShowsTheFirstFourLibraries(t *testing.T) {
	c := newBrowsingCatalog()
	for _, name := range []string{"second", "third", "fourth", "fifth"} {
		c.libraries = append(c.libraries, views.LibraryCard{Owner: "example", Name: name, Rules: 1})
	}

	page := get(t, newSite(t, c), "/").Body.String()

	if got := headings(t, page, "h3"); !slices.Equal(got, []string{"rules", "second", "third", "fourth", "Have a public Code Rules library?"}) {
		t.Errorf("the home page's headings are %q", got)
	}
	if got := links(t, page, "All libraries"); !slices.Equal(got, []string{"/libraries"}) {
		t.Errorf("All libraries leads to %q", got)
	}
}

// Without a way to list a library, List your library explains how a library gets vetted; with one, the listing
// tests cover where it leads.
func TestHomeLeadsToGettingVettedWhenListingIsntAvailable(t *testing.T) {
	page := get(t, newSite(t, newBrowsingCatalog()), "/").Body.String()

	assertShows(t, page, "Stock the shelves Have a public Code Rules library? List it on Rulemart in a minute.")
	if got := links(t, page, "List your library"); !slices.Equal(got, []string{"/about#get-vetted"}) {
		t.Errorf("List your library leads to %q", got)
	}
}
