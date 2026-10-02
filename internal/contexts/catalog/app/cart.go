// Keep carts: an account adds and removes whole libraries, groups, and rules, and checks its cart out as the Code
// Rules configuration that imports them.

package app

import (
	"context"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// ErrUnvettedNotConfirmed reports an item to add from a library Rulemart doesn't vet, which the visitor didn't
// confirm.
var ErrUnvettedNotConfirmed = store.ErrUnvettedNotConfirmed

// ErrCartFull reports a cart that holds domain.MaxCartItems items.
var ErrCartFull = store.ErrCartFull

// Cart keeps accounts' carts, as the web function does. An item can be added from a library Vetted holds, or after
// the visitor confirms it, from one a listing names.
type Cart struct {
	Store  store.Cart
	Vetted []domain.LibraryKey
	// Groups is Code Rules' canonical group list, which decides how the cart shows each group.
	Groups domain.CanonicalGroups
}

// Add adds item to the account's cart, and returns it as its library spells it. confirmed says the visitor confirmed
// adding it from a library Rulemart doesn't vet. It fails with ErrNotFound when the library's pages don't show the
// item, ErrUnvettedNotConfirmed when the library isn't vetted and confirmed is false, and ErrCartFull when the cart
// would hold more than domain.MaxCartItems items. Adding an item twice keeps one, and a group or whole library takes
// the place of the items of it the cart holds.
func (c Cart) Add(ctx context.Context, accountID int64, item domain.CartItem, confirmed bool) (domain.CartItem, error) {
	return c.Store.AddToCart(ctx, c.Vetted, accountID, item, confirmed)
}

// Remove removes item from the account's cart, if it holds it.
func (c Cart) Remove(ctx context.Context, accountID int64, item domain.CartItem) error {
	return c.Store.RemoveFromCart(ctx, accountID, item)
}

// Empty removes every item from the account's cart.
func (c Cart) Empty(ctx context.Context, accountID int64) error {
	return c.Store.EmptyCart(ctx, accountID)
}

// Held returns the account's items, with whether the visitor confirmed each as unvetted.
func (c Cart) Held(ctx context.Context, accountID int64) ([]views.HeldCartItem, error) {
	return c.Store.HeldCartItems(ctx, accountID)
}

// Contents returns the account's cart, each item with its state, and the checkout of every item it can import, each
// library pinned to its latest release.
func (c Cart) Contents(ctx context.Context, accountID int64) (views.Cart, error) {
	libraries, err := c.Store.Cart(ctx, c.Vetted, accountID)
	if err != nil {
		return views.Cart{}, err
	}
	checkout := make([]domain.CheckoutLibrary, 0, len(libraries))
	for _, lib := range libraries {
		var importable []domain.CartItem
		for i := range lib.Items {
			it := &lib.Items[i]
			it.CanonicalGroup = canonicalGroup(c.Groups, it.Group)
			if it.State = itemState(lib, *it); it.State == views.CartItemReady {
				importable = append(importable, it.Item)
			}
		}
		for i := range lib.Items {
			it := &lib.Items[i]
			if covering, ok := domain.Covering(it.Item, importable); ok && it.State == views.CartItemReady {
				it.State, it.CoveredBy = views.CartItemCovered, covering
			}
		}
		checkout = append(checkout, domain.CheckoutLibrary{
			Owner: lib.Library.Owner, Name: lib.Library.Name, Release: lib.LatestRelease, Vetted: lib.Vetted, Items: importable,
		})
	}
	return views.Cart{Libraries: libraries, Checkout: domain.NewCheckout(checkout)}, nil
}

// itemState returns whether checkout imports it, an item of lib, as the store read it, or why it doesn't, before
// another item covers it.
func itemState(lib views.CartLibrary, it views.CartItem) views.CartItemState {
	switch {
	case !lib.Vetted && !lib.Listed:
		return views.CartItemGone
	case !lib.Vetted && !it.Confirmed:
		return views.CartItemUnconfirmed
	case it.Item.Kind == domain.CartRule && it.Group == "":
		return views.CartItemMissing
	case it.Item.Kind == domain.CartRule && it.RetiredIn > 0:
		return views.CartItemRetired
	case it.Item.Kind != domain.CartRule && it.Rules == 0:
		return views.CartItemMissing
	}
	return views.CartItemReady
}
