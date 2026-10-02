package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/platform/postgrestest"
)

// cartItem returns the item of library, owner/name, of kind at path.
func cartItem(owner, name string, kind domain.CartItemKind, path string) domain.CartItem {
	return domain.CartItem{Owner: owner, Name: name, Kind: kind, Path: path}
}

var (
	acmeLibrary    = cartItem("acme", "backend", domain.CartLibrary, "")
	acmeTesting    = cartItem("acme", "backend", domain.CartGroup, "practices/testing")
	acmeRetryLimit = cartItem("acme", "backend", domain.CartRule, "practices/testing/verify-retry-limits")
	acmeReturn     = cartItem("acme", "backend", domain.CartRule, "techs/go/return-errors")
	betaPackages   = cartItem("Beta", "rules", domain.CartRule, "techs/go/name-packages-plainly")
	strangerUseGo  = cartItem("stranger", "rules", domain.CartRule, "techs/go/use-go")
)

// add adds item to the account's cart as the web function does, with vetted as the release's vetted libraries, and
// fails t if it can't.
func (c listingCatalog) add(t *testing.T, vetted []domain.LibraryKey, accountID int64, item domain.CartItem, confirmed bool) {
	t.Helper()
	if _, err := c.web.AddToCart(context.Background(), vetted, accountID, item, confirmed); err != nil {
		t.Fatalf("add %+v to account %d's cart: %v", item, accountID, err)
	}
}

// cartItems returns the account's items from every library the catalog has, as owner/name kind path.
func (c listingCatalog) cartItems(t *testing.T, accountID int64) []string {
	t.Helper()
	var items []string
	for _, name := range [][2]string{{"acme", "backend"}, {"Beta", "rules"}, {"stranger", "rules"}} {
		found, err := c.web.LibraryCartItems(context.Background(), accountID, name[0], name[1])
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range found {
			items = append(items, fmt.Sprintf("%s %s %s", it.FullName(), it.Kind, it.Path))
		}
	}
	return items
}

// An account adds a whole library, a group, or a rule, by any spelling, and the cart holds each once, as the library
// spells it.
func TestAnAccountAddsItemsToItsCartOnce(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	account, other := c.account(t, 1), c.account(t, 2)

	added, err := c.web.AddToCart(ctx, vettedBoth, account, cartItem("ACME", "Backend", domain.CartRule, "Practices/Testing/Verify-Retry-Limits"), false)
	if err != nil || added != acmeRetryLimit {
		t.Fatalf("got %+v, %v; want %+v, as the library spells it", added, err, acmeRetryLimit)
	}
	c.add(t, vettedBoth, account, acmeRetryLimit, false)
	c.add(t, vettedBoth, account, cartItem("acme", "backend", domain.CartGroup, "PRACTICES/testing"), false)
	c.add(t, vettedBoth, account, cartItem("Acme", "BACKEND", domain.CartLibrary, ""), false)
	c.add(t, vettedBoth, account, betaPackages, false)
	c.add(t, vettedBoth, other, acmeReturn, false)

	want := []string{
		"acme/backend group practices/testing", "acme/backend library ", "acme/backend rule practices/testing/verify-retry-limits",
		"Beta/rules rule techs/go/name-packages-plainly",
	}
	if got := c.cartItems(t, account); !slices.Equal(got, want) {
		t.Errorf("the account's cart holds\n%q\nwant\n%q", got, want)
	}
	if count, err := c.web.CountCart(ctx, account); err != nil || count != 4 {
		t.Errorf("counted %d, %v; want 4", count, err)
	}
	if got := c.cartItems(t, other); !slices.Equal(got, []string{"acme/backend rule techs/go/return-errors"}) {
		t.Errorf("the other account's cart holds %q", got)
	}
}

// Only what a library's pages show can be added: a current rule, or a group that holds current rules, of a vetted or
// listed library.
func TestACartTakesOnlyCurrentItemsOfLibrariesPagesShow(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	account := c.account(t, 1)

	for _, item := range []domain.CartItem{
		cartItem("acme", "backend", domain.CartRule, "practices/testing/retry-forever"),
		cartItem("acme", "backend", domain.CartRule, "practices/testing/no-such-rule"),
		cartItem("acme", "backend", domain.CartRule, "practices/testing"),
		cartItem("acme", "backend", domain.CartGroup, "techs/rust"),
		cartItem("acme", "backend", domain.CartGroup, "techs/go/return-errors"),
		cartItem("nobody", "nothing", domain.CartLibrary, ""),
		strangerUseGo,
	} {
		for _, confirmed := range []bool{false, true} {
			if _, err := c.web.AddToCart(ctx, vettedBoth, account, item, confirmed); !errors.Is(err, store.ErrNotFound) {
				t.Errorf("add %+v, confirmed %v: got %v, want ErrNotFound", item, confirmed, err)
			}
		}
	}
	if count, err := c.web.CountCart(ctx, account); err != nil || count != 0 {
		t.Errorf("counted %d, %v; want an empty cart", count, err)
	}
}

// An item of a library Rulemart doesn't vet needs the visitor's confirmation, which the cart records; one of a vetted
// library records none, and confirming an item again records the confirmation it lacked, such as after its library
// lost its vetting.
func TestAnUnvettedItemNeedsConfirming(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	account := c.account(t, 1)
	c.resolve(t, c.list(t, account, "stranger", "rules"), "23")
	everyone := append([]domain.LibraryKey{{Host: domain.GitHub, RepositoryID: "23"}}, vettedBoth...)
	confirmed := func() map[string]bool {
		t.Helper()
		cart, err := c.web.Cart(ctx, vettedBoth, account)
		if err != nil {
			t.Fatal(err)
		}
		confirmed := map[string]bool{}
		for _, lib := range cart {
			for _, it := range lib.Items {
				confirmed[it.Item.FullName()+" "+it.Item.Path] = it.Confirmed
			}
		}
		return confirmed
	}

	c.add(t, everyone, account, strangerUseGo, true)
	c.add(t, vettedBoth, account, acmeReturn, true)
	if got := confirmed(); got["stranger/rules techs/go/use-go"] || got["acme/backend techs/go/return-errors"] {
		t.Fatalf("items added from vetted libraries record a confirmation: %v", got)
	}
	// stranger/rules isn't vetted now.
	if _, err := c.web.AddToCart(ctx, vettedBoth, account, strangerUseGo, false); !errors.Is(err, store.ErrUnvettedNotConfirmed) {
		t.Fatalf("got %v, want ErrUnvettedNotConfirmed", err)
	}
	c.add(t, vettedBoth, account, strangerUseGo, true)
	if got := confirmed(); !got["stranger/rules techs/go/use-go"] || got["acme/backend techs/go/return-errors"] {
		t.Fatalf("got %v, want only the unvetted item confirmed", got)
	}
	// Beta/rules loses its vetting, and no listing names it, so it has no pages to add from.
	if _, err := c.web.AddToCart(ctx, vettedBoth[:1], account, betaPackages, true); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

// A cart holds at most domain.MaxCartItems items. A full cart still takes an item it holds, and two items added at
// once can't both take its last place.
func TestACartHoldsAtMostMaxCartItems(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	account := c.account(t, 1)
	// Fill the cart but two places with items the library no longer has, which count as any item does.
	postgrestest.Exec(t, c.connString, fmt.Sprintf(`
		INSERT INTO cart_items (account_id, library_id, kind, path)
		SELECT %d, l.id, 'rule', 'techs/go/gone-' || n FROM libraries l, generate_series(1, %d) n
		WHERE l.owner = 'acme'`, account, domain.MaxCartItems-2))
	c.add(t, vettedBoth, account, acmeReturn, false)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, item := range []domain.CartItem{acmeRetryLimit, betaPackages} {
		wg.Go(func() { _, errs[i] = c.web.AddToCart(ctx, vettedBoth, account, item, false) })
	}
	wg.Wait()
	if full := errors.Is(errs[0], store.ErrCartFull) != errors.Is(errs[1], store.ErrCartFull); !full || (errs[0] != nil && errs[1] != nil) {
		t.Fatalf("adding two items at once to a cart with one place left: %v and %v; want one added and one ErrCartFull", errs[0], errs[1])
	}
	if count, err := c.web.CountCart(ctx, account); err != nil || count != domain.MaxCartItems {
		t.Fatalf("counted %d, %v; want %d", count, err, domain.MaxCartItems)
	}
	if _, err := c.web.AddToCart(ctx, vettedBoth, account, acmeTesting, false); !errors.Is(err, store.ErrCartFull) {
		t.Errorf("adding to a full cart: got %v, want ErrCartFull", err)
	}
	c.add(t, vettedBoth, account, acmeReturn, false)
}

// Removing takes only the account's own item, by any spelling, and removing what the cart doesn't hold changes
// nothing; emptying takes every item of the account's, and deleting the account empties its cart.
func TestRemovingAndEmptyingTakeOnlyTheAccountsItems(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	account, other := c.account(t, 1), c.account(t, 2)
	for _, id := range []int64{account, other} {
		c.add(t, vettedBoth, id, acmeReturn, false)
		c.add(t, vettedBoth, id, acmeTesting, false)
		c.add(t, vettedBoth, id, betaPackages, false)
	}

	for range 2 {
		if err := c.web.RemoveFromCart(ctx, account, cartItem("ACME", "backend", domain.CartRule, "Techs/Go/Return-Errors")); err != nil {
			t.Fatal(err)
		}
	}
	// A group and a rule are apart: removing the group's ID as a rule removes nothing.
	if err := c.web.RemoveFromCart(ctx, account, cartItem("acme", "backend", domain.CartRule, "practices/testing")); err != nil {
		t.Fatal(err)
	}
	if err := c.web.RemoveFromCart(ctx, account, cartItem("nobody", "nothing", domain.CartLibrary, "")); err != nil {
		t.Fatalf("removing from a library that isn't there: %v", err)
	}
	if got := c.cartItems(t, account); !slices.Equal(got, []string{"acme/backend group practices/testing", "Beta/rules rule techs/go/name-packages-plainly"}) {
		t.Errorf("the account's cart holds %q", got)
	}
	if err := c.web.EmptyCart(ctx, account); err != nil {
		t.Fatal(err)
	}
	if count, err := c.web.CountCart(ctx, account); err != nil || count != 0 {
		t.Errorf("counted %d, %v after emptying", count, err)
	}
	if count, err := c.web.CountCart(ctx, other); err != nil || count != 3 {
		t.Errorf("the other account's cart counts %d, %v; want its 3 items", count, err)
	}
	postgrestest.Exec(t, c.connString, fmt.Sprintf("DELETE FROM accounts WHERE id = %d", other))
	var left int
	postgrestest.QueryRow(t, c.connString, "SELECT count(*) FROM cart_items", &left)
	if left != 0 {
		t.Errorf("%d items outlive their account", left)
	}
}

// The cart reads each library's items as the catalog has them now: in order, with a rule's group and title, how
// many rules a group or whole library holds, a rule that retired since, one the library no longer has, and a group
// whose rules all retired, and each library's vetting, listing, and latest release.
func TestTheCartReadsItsItemsAsTheCatalogHasThemNow(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	account := c.account(t, 1)
	c.resolve(t, c.list(t, account, "stranger", "rules"), "23")
	c.add(t, vettedBoth, account, strangerUseGo, true)
	c.add(t, vettedBoth, account, betaPackages, false)
	c.add(t, vettedBoth, account, acmeReturn, false)
	c.add(t, vettedBoth, account, acmeRetryLimit, false)
	c.add(t, vettedBoth, account, cartItem("acme", "backend", domain.CartGroup, "techs/golang"), false)
	c.add(t, vettedBoth, account, acmeTesting, false)
	c.add(t, vettedBoth, account, acmeLibrary, false)
	// A second release retires verify-retry-limits and pass-context-first, which leaves techs/golang without
	// current rules, and drops return-errors altogether.
	next := acme
	next.Releases = append(slices.Clone(acme.Releases), domain.Release{Number: 2, CommitID: "2222222222222222222222222222222222222222", TaggedAt: day(2)})
	next.Rules = nil
	for _, r := range acme.Rules {
		switch r.Path {
		case "practices/testing/verify-retry-limits", "techs/golang/pass-context-first":
			r.RetiredIn, r.RetirementSummaries = 2, []string{"Drop it."}
			r.HTML, r.WhenToReadHTML = "", ""
		case "techs/go/return-errors":
			continue
		}
		next.Rules = append(next.Rules, r)
	}
	if _, err := c.worker.ReplaceLibrary(ctx, next); err != nil {
		t.Fatal(err)
	}

	cart, err := c.web.Cart(ctx, vettedBoth, account)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, lib := range cart {
		got = append(got, fmt.Sprintf("%s vetted=%v listed=%v release=%d", lib.Library.FullName(), lib.Vetted, lib.Listed, lib.LatestRelease))
		for _, it := range lib.Items {
			got = append(got, fmt.Sprintf("  %s %q group=%q title=%q rules=%d retired=%d", it.Item.Kind, it.Item.Path, it.Group, it.Title, it.Rules, it.RetiredIn))
		}
	}
	want := []string{
		"acme/backend vetted=true listed=false release=2",
		`  library "" group="" title="" rules=1 retired=0`,
		`  group "practices/testing" group="practices/testing" title="" rules=1 retired=0`,
		`  group "techs/golang" group="techs/golang" title="" rules=0 retired=0`,
		`  rule "practices/testing/verify-retry-limits" group="practices/testing" title="Verify retry limits" rules=0 retired=2`,
		`  rule "techs/go/return-errors" group="" title="" rules=0 retired=0`,
		"Beta/rules vetted=true listed=false release=1",
		`  rule "techs/go/name-packages-plainly" group="techs/go" title="Name packages plainly" rules=0 retired=0`,
		"stranger/rules vetted=false listed=true release=1",
		`  rule "techs/go/use-go" group="techs/go" title="Use Go" rules=0 retired=0`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("got\n%s\nwant\n%s", join(got), join(want))
	}
	if cart[0].Library != (views.LibraryRef{Owner: "acme", Name: "backend", OwnerAvatarURL: acme.Repository.OwnerAvatarURL}) {
		t.Errorf("acme's library is %+v", cart[0].Library)
	}
	if cart[2].Items[0].AddedAt.IsZero() {
		t.Error("an item has no time it was added")
	}
}

// join returns lines joined by newlines, for a failure message.
func join(lines []string) string {
	var text string
	for _, line := range lines {
		text += line + "\n"
	}
	return text
}
