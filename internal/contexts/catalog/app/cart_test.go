package app_test

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// cartStore answers CartLibraries with libraries, the catalog as checkout reads it, whichever items it's asked for,
// and counts the reads.
type cartStore struct {
	libraries []views.CartLibrary
	reads     int
}

func (s *cartStore) CartLibraries(_ context.Context, _ []domain.LibraryKey, _ []domain.CartItem) ([]views.CartLibrary, error) {
	s.reads++
	return s.libraries, nil
}

// cartRule returns a current rule at path, titled by its last part, at version 1.2.0.
func cartRule(path string) views.CartRule {
	parts := strings.Split(path, "/")
	return views.CartRule{Path: path, Group: parts[0] + "/" + parts[1], Title: parts[len(parts)-1], Version: coderules.RuleVersion{Major: 1, Minor: 2}}
}

// newCarts returns Carts over a catalog of acme/rules, vetted, with three Go rules, one retired, and two testing
// rules, and stranger/rules, listed but not vetted, with one Go rule.
func newCarts(t *testing.T) (app.Carts, *cartStore) {
	t.Helper()
	retired := cartRule("techs/go/old-errors")
	retired.RetiredIn = 2
	s := &cartStore{libraries: []views.CartLibrary{
		{
			Library: views.LibraryRef{Owner: "acme", Name: "rules", OwnerAvatarURL: "https://avatars.githubusercontent.com/u/1"},
			Vetted:  true, LatestRelease: 3, LatestCommit: strings.Repeat("a", 40),
			Rules: []views.CartRule{
				cartRule("practices/testing/name-tests"), cartRule("practices/testing/keep-tests-independent"),
				cartRule("techs/go/close-bodies"), retired, cartRule("techs/go/return-errors"),
			},
		},
		{
			Library: views.LibraryRef{Owner: "stranger", Name: "rules"}, LatestRelease: 1, LatestCommit: strings.Repeat("b", 40),
			Rules: []views.CartRule{cartRule("techs/go/use-go")},
		},
	}}
	groups, err := domain.NewCanonicalGroups([]coderules.CanonicalGroup{{ID: "techs/go", Name: "Go"}, {ID: "practices/testing", Name: "Testing"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return app.Carts{Store: s, Groups: groups}, s
}

// states returns each item of checkout as its key and state.
func states(checkout views.Checkout) []string {
	var got []string
	for _, lib := range checkout.Libraries {
		for _, it := range lib.Items {
			got = append(got, it.Key+" "+string(it.State))
		}
	}
	return got
}

// Checkout resolves every key, in the cart's order by library, to its state: a current rule or a group with current
// rules is ready, a retired rule retired, what the library doesn't have missing, an item of a library Rulemart has
// no page for gone, and an unvetted library's items unvetted until the visitor confirms them. A key that names
// nothing is unknown, and a repeated key counts once.
func TestCheckoutResolvesEachItemsState(t *testing.T) {
	carts, _ := newCarts(t)
	cart := app.Cart{Keys: []string{
		"acme/rules::techs/go/return-errors",
		"stranger/rules::techs/go/use-go",
		"group::acme/rules::practices/testing",
		"acme/rules::techs/go/old-errors",
		"acme/rules::techs/go/never-was",
		"group::acme/rules::techs/rust",
		"gone/rules::techs/go/x",
		"not a key",
		"acme/rules::techs/go/return-errors",
	}}

	checkout, err := carts.Checkout(context.Background(), cart, domain.CheckoutTarget{Mode: domain.ProjectUnknown})

	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"acme/rules::techs/go/return-errors ready",
		"group::acme/rules::practices/testing ready",
		"acme/rules::techs/go/old-errors retired",
		"acme/rules::techs/go/never-was missing",
		"group::acme/rules::techs/rust missing",
		"stranger/rules::techs/go/use-go unvetted",
		"gone/rules::techs/go/x gone",
	}
	if got := states(checkout); !slices.Equal(got, want) {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
	if !slices.Equal(checkout.Unknown, []string{"not a key"}) {
		t.Errorf("got unknown %q, want the one key that names nothing", checkout.Unknown)
	}
	if lib := checkout.Libraries[2]; !lib.Gone || lib.Library.FullName() != "gone/rules" {
		t.Errorf("got %+v, want gone/rules gone", lib)
	}
	if pin := checkout.PinExample; pin == nil || *pin != (domain.ReleasePin{Library: "acme/rules", Release: 3}) {
		t.Errorf("got the pin example %+v, want acme/rules at its latest release", pin)
	}
	group := checkout.Libraries[0].Items[1]
	if group.Group.Canonical == nil || group.Group.Canonical.Name != "Testing" || len(group.Rules) != 2 {
		t.Errorf("got %+v, want the Testing group with its two rules", group)
	}
	// Only the ready items are imported, and the unvetted library is named nowhere until it's confirmed.
	for _, want := range []string{"--groups practices/testing", "--rules techs/go/return-errors"} {
		if !strings.Contains(checkout.Commands, want) {
			t.Errorf("the commands lack %q:\n%s", want, checkout.Commands)
		}
	}
	for _, unwanted := range []string{"old-errors", "never-was", "rust", "stranger", "gone/rules"} {
		if strings.Contains(checkout.Commands, unwanted) || strings.Contains(checkout.Prompt, unwanted) {
			t.Errorf("the texts name %q, which checkout leaves out", unwanted)
		}
	}
}

// An unvetted library the visitor confirmed, in any case, is imported, named for review, and pinned to the commit
// Rulemart saw.
func TestCheckoutImportsAConfirmedUnvettedLibraryForReview(t *testing.T) {
	carts, _ := newCarts(t)
	cart := app.Cart{Keys: []string{"stranger/rules::techs/go/use-go"}, Confirmed: map[string]bool{"Stranger/Rules": true}}

	checkout, err := carts.Checkout(context.Background(), cart, domain.CheckoutTarget{Mode: domain.ProjectUnknown})

	if err != nil {
		t.Fatal(err)
	}
	if got := states(checkout); !slices.Equal(got, []string{"stranger/rules::techs/go/use-go ready"}) || !checkout.Libraries[0].Confirmed {
		t.Errorf("got %q, confirmed %t, want the rule ready and its library confirmed", got, checkout.Libraries[0].Confirmed)
	}
	if !strings.Contains(checkout.Commands, "--ref "+strings.Repeat("b", 40)) || !strings.Contains(checkout.Prompt, "Rulemart hasn't vetted stranger/rules") {
		t.Errorf("the texts neither pin nor name the unvetted library:\n%s", checkout.Prompt)
	}
	if checkout.PinExample != nil {
		t.Errorf("got the pin example %+v for a library pinned to its commit already", checkout.PinExample)
	}
}

// A forked rule is copied at its version, and the offer to add the rest of a picked rule's group counts the group's
// current rules the cart doesn't hold: not the retired one, the picked one, or the forked one. Taking the offer
// imports the group whole, so the fork in it gives a reason, and the count goes to 0.
func TestCheckoutForksAndOffersTheRestOfTheGroup(t *testing.T) {
	carts, _ := newCarts(t)
	cart := app.Cart{
		Keys:  []string{"acme/rules::techs/go/return-errors", "acme/rules::techs/go/close-bodies"},
		Forks: map[string]bool{"acme/rules::techs/go/close-bodies": true},
	}

	checkout, err := carts.Checkout(context.Background(), cart, domain.CheckoutTarget{Mode: domain.ProjectUnknown})

	if err != nil {
		t.Fatal(err)
	}
	lib := checkout.Libraries[0]
	if len(lib.RestOfGroups) != 1 || lib.RestOfGroups[0].Path != "techs/go" || lib.RestOfGroupsRules != 0 || lib.RestOfGroupsAdded {
		t.Errorf("got the offer %+v, %d more, added %t, want the Go group with none more", lib.RestOfGroups, lib.RestOfGroupsRules, lib.RestOfGroupsAdded)
	}
	if !lib.Items[1].Fork || !strings.Contains(checkout.Commands, "add rule techs/go/close-bodies \\\n  --from acme@1.2.0") {
		t.Errorf("the fork isn't copied at its version:\n%s", checkout.Commands)
	}

	cart.Keys = cart.Keys[:1]
	checkout, err = carts.Checkout(context.Background(), cart, domain.CheckoutTarget{Mode: domain.ProjectUnknown})
	if err != nil {
		t.Fatal(err)
	}
	if lib := checkout.Libraries[0]; lib.RestOfGroupsRules != 1 {
		t.Errorf("got %d more, want close-bodies, the group's one other current rule", lib.RestOfGroupsRules)
	}

	cart.Keys, cart.RestOfGroups = []string{"acme/rules::techs/go/return-errors", "acme/rules::techs/go/close-bodies"}, map[string]bool{"acme/rules": true}
	checkout, err = carts.Checkout(context.Background(), cart, domain.CheckoutTarget{Mode: domain.ProjectUnknown})
	if err != nil {
		t.Fatal(err)
	}
	if lib := checkout.Libraries[0]; !lib.RestOfGroupsAdded || lib.RestOfGroupsRules != 0 || len(lib.RestOfGroups) != 1 {
		t.Errorf("got added %t, %d more, groups %+v, want the offer taken", lib.RestOfGroupsAdded, lib.RestOfGroupsRules, lib.RestOfGroups)
	}
	if !strings.Contains(checkout.Commands, "--groups techs/go") || !strings.Contains(checkout.Commands, "--reason") {
		t.Errorf("the group isn't imported whole, with the fork's reason:\n%s", checkout.Commands)
	}
}

// A cart holds at most domain.MaxCartItems keys: one more is refused without reading the catalog.
func TestCheckoutRefusesACartOfMoreThanTheMostItems(t *testing.T) {
	carts, s := newCarts(t)
	keys := make([]string, domain.MaxCartItems)
	for i := range keys {
		keys[i] = "acme/rules::techs/go/r" + strconv.Itoa(i)
	}

	if _, err := carts.Checkout(context.Background(), app.Cart{Keys: keys}, domain.CheckoutTarget{Mode: domain.ProjectUnknown}); err != nil {
		t.Fatalf("a full cart: %v", err)
	}
	_, err := carts.Checkout(context.Background(), app.Cart{Keys: append(keys, "acme/rules::techs/go/one-more")}, domain.CheckoutTarget{Mode: domain.ProjectUnknown})

	if !errors.Is(err, app.ErrCartTooLarge) || s.reads != 1 {
		t.Errorf("got %v after %d reads, want app.ErrCartTooLarge without reading", err, s.reads)
	}
}
