package postgres_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// cartItem returns the item key names, and fails t if it names none.
func cartItem(t *testing.T, key string) domain.CartItem {
	t.Helper()
	item, err := domain.ParseCartKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return item
}

// cartRules returns the rules a cart's library holds, each as its path, with " retired" when it's retired.
func cartRules(lib views.CartLibrary) []string {
	var rules []string
	for _, r := range lib.Rules {
		entry := r.Path
		if r.RetiredIn > 0 {
			entry += " retired"
		}
		rules = append(rules, entry)
	}
	return rules
}

// Checkout reads, as the web function's role, each library a cart names in any case, vetted or listed, with its
// latest release, and the rules of the groups the cart names of it, current and retired, a rule's group or a whole
// group's, and of no other group: acme's golang group goes unread. A library neither vetted nor listed, or not in
// the catalog, is left out.
func TestCartLibrariesReadTheGroupsACartNames(t *testing.T) {
	c := newListingCatalog(t)
	c.resolve(t, c.list(t, c.account(t, 1), "stranger", "rules"), "23")
	items := []domain.CartItem{
		cartItem(t, "ACME/Backend::Practices/Testing/Verify-Retry-Limits"),
		cartItem(t, "group::acme/backend::techs/go"),
		cartItem(t, "Beta/rules::techs/go/name-packages-plainly"),
		cartItem(t, "stranger/rules::techs/go/use-go"),
		cartItem(t, "gone/library::techs/go/anything"),
	}

	libraries, err := c.web.CartLibraries(context.Background(), vettedBoth, items)

	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(libraries, func(a, b views.CartLibrary) int {
		return strings.Compare(strings.ToLower(a.Library.FullName()), strings.ToLower(b.Library.FullName()))
	})
	var got []string
	for _, lib := range libraries {
		got = append(got, lib.Library.FullName())
		if lib.LatestRelease != 1 || lib.LatestCommit != strings.Repeat("1", 40) {
			t.Errorf("%s: latest release %d at %q, want release 1 at its commit", lib.Library.FullName(), lib.LatestRelease, lib.LatestCommit)
		}
	}
	if want := []string{"acme/backend", "Beta/rules", "stranger/rules"}; !slices.Equal(got, want) {
		t.Fatalf("got libraries %q, want %q", got, want)
	}
	if !libraries[0].Vetted || !libraries[1].Vetted || libraries[2].Vetted {
		t.Errorf("vetted: got %t, %t, %t, want acme and Beta vetted, stranger not", libraries[0].Vetted, libraries[1].Vetted, libraries[2].Vetted)
	}
	// By group, then title, as a library's page lists them: the retired rule has no title stored.
	wantAcme := []string{
		"practices/testing/retry-forever retired", "practices/testing/cover-boundary-cases",
		"practices/testing/verify-retry-limits", "techs/go/return-errors",
	}
	if got := cartRules(libraries[0]); !slices.Equal(got, wantAcme) {
		t.Errorf("acme's rules: got %q, want %q", got, wantAcme)
	}
	first := libraries[0].Rules[1]
	if first.Title != "Cover boundary cases" || first.Version.String() != "1.0.0" || first.Group != "practices/testing" {
		t.Errorf("got %+v, want its title, newest version, and group", first)
	}
	if got := cartRules(libraries[2]); !slices.Equal(got, []string{"techs/go/use-go"}) {
		t.Errorf("stranger's rules: got %q, want only its go group's", got)
	}
}

// A library a cart names that's neither vetted nor listed is left out, as it is from every page.
func TestCartLibrariesLeaveOutALibraryNeitherVettedNorListed(t *testing.T) {
	c := newListingCatalog(t)

	libraries, err := c.web.CartLibraries(context.Background(), vettedBoth, []domain.CartItem{cartItem(t, "stranger/rules::techs/go/use-go")})

	if err != nil || len(libraries) != 0 {
		t.Errorf("got %+v, %v, want no library", libraries, err)
	}
}
