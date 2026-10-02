package app_test

import (
	"context"
	"slices"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// fakeCartStore returns its libraries as the account's cart, and records the vetted libraries each read passes. Its
// other methods aren't called.
type fakeCartStore struct {
	cartStore
	libraries []views.CartLibrary
	vetted    []domain.LibraryKey
}

// cartStore is store.Cart, embedded under another name, since a field named Cart would hide the method.
type cartStore = store.Cart

func (f *fakeCartStore) Cart(_ context.Context, vetted []domain.LibraryKey, _ int64) ([]views.CartLibrary, error) {
	f.vetted = vetted
	return f.libraries, nil
}

// cartItem returns a cart item of library, owner/name, read as the store reads it.
func cartItem(library views.LibraryRef, kind domain.CartItemKind, path string, adjust func(*views.CartItem)) views.CartItem {
	it := views.CartItem{Item: domain.CartItem{Owner: library.Owner, Name: library.Name, Kind: kind, Path: path}}
	adjust(&it)
	return it
}

// A cart's items say whether checkout imports each, or why it leaves it out, and checkout imports each library's
// importable items, pinned to its latest release.
func TestTheCartSaysWhatCheckoutImportsAndWhy(t *testing.T) {
	acme := views.LibraryRef{Owner: "acme", Name: "backend"}
	beta := views.LibraryRef{Owner: "Beta", Name: "rules"}
	stranger := views.LibraryRef{Owner: "stranger", Name: "rules"}
	gone := views.LibraryRef{Owner: "gone", Name: "rules"}
	none := func(*views.CartItem) {}
	fake := &fakeCartStore{libraries: []views.CartLibrary{
		{Library: acme, Vetted: true, LatestRelease: 4, Items: []views.CartItem{
			cartItem(acme, domain.CartGroup, "techs/go", func(it *views.CartItem) { it.Group, it.Rules = "techs/go", 3 }),
			cartItem(acme, domain.CartGroup, "techs/golang", func(it *views.CartItem) { it.Group = "techs/golang" }),
			cartItem(acme, domain.CartRule, "practices/testing/cover-edges", func(it *views.CartItem) { it.Group = "practices/testing" }),
			cartItem(acme, domain.CartRule, "practices/testing/retry-forever", func(it *views.CartItem) {
				it.Group, it.RetiredIn = "practices/testing", 3
			}),
			cartItem(acme, domain.CartRule, "techs/go/return-errors", func(it *views.CartItem) { it.Group = "techs/go" }),
			cartItem(acme, domain.CartRule, "techs/golang/pass-context", func(it *views.CartItem) { it.Group = "techs/golang" }),
			cartItem(acme, domain.CartRule, "techs/go/vanished", none),
		}},
		// Beta/rules lost its vetting, and a listing names it.
		{Library: beta, Listed: true, LatestRelease: 2, Items: []views.CartItem{
			cartItem(beta, domain.CartRule, "techs/go/name-packages", func(it *views.CartItem) { it.Group = "techs/go" }),
		}},
		{Library: stranger, Listed: true, LatestRelease: 1, Items: []views.CartItem{
			cartItem(stranger, domain.CartLibrary, "", func(it *views.CartItem) { it.Rules, it.Confirmed = 2, true }),
		}},
		{Library: gone, LatestRelease: 7, Items: []views.CartItem{
			cartItem(gone, domain.CartRule, "techs/go/gone", func(it *views.CartItem) { it.Group, it.Confirmed = "techs/go", true }),
		}},
	}}
	groups, err := domain.NewCanonicalGroups([]coderules.CanonicalGroup{{ID: "techs/go", Name: "Go"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	vetted := []domain.LibraryKey{{Host: domain.GitHub, RepositoryID: "1"}}
	cart, err := app.Cart{Store: fake, Vetted: vetted, Groups: groups}.Contents(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(fake.vetted, vetted) {
		t.Errorf("read the cart with vetted %v, want the release's", fake.vetted)
	}
	var states []string
	for _, lib := range cart.Libraries {
		for _, it := range lib.Items {
			state := it.Item.FullName() + " " + it.Item.Path + " " + string(it.State)
			if it.State == views.CartItemCovered {
				state += " by " + it.CoveredBy.Path
			}
			if it.CanonicalGroup != nil {
				state += " in " + it.CanonicalGroup.Name
			}
			states = append(states, state)
		}
	}
	want := []string{
		"acme/backend techs/go ready in Go",
		"acme/backend techs/golang missing",
		"acme/backend practices/testing/cover-edges ready",
		"acme/backend practices/testing/retry-forever retired",
		"acme/backend techs/go/return-errors covered by techs/go in Go",
		// Its group has no current rules, which doesn't change the rule's own state.
		"acme/backend techs/golang/pass-context ready",
		"acme/backend techs/go/vanished missing",
		"Beta/rules techs/go/name-packages unconfirmed in Go",
		"stranger/rules  ready",
		"gone/rules techs/go/gone gone in Go",
	}
	if !slices.Equal(states, want) {
		t.Errorf("got states\n%q\nwant\n%q", states, want)
	}

	var sources []string
	for _, s := range cart.Checkout.Sources {
		sources = append(sources, s.Name+" "+s.Library.FullName()+" "+domain.ReleaseTag(s.Library.Release))
		if s.Library.FullName() == "acme/backend" {
			if !slices.Equal(s.Groups, []string{"techs/go"}) || !slices.Equal(s.Rules, []string{"practices/testing/cover-edges", "techs/golang/pass-context"}) {
				t.Errorf("acme imports groups %q and rules %q", s.Groups, s.Rules)
			}
		}
	}
	if want := []string{"backend acme/backend release/4", "rules stranger/rules release/1"}; !slices.Equal(sources, want) {
		t.Errorf("checkout imports %q, want %q", sources, want)
	}
	if cart.Items() != 10 {
		t.Errorf("counted %d items, want 10", cart.Items())
	}
}
