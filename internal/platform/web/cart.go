// Collect rules in a cart from library and rule pages, confirm adding from an unvetted library, see the cart, and
// check it out as a prompt for a coding agent.

package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// Cart keeps signed-in visitors' carts. catalog/app.Cart implements it.
type Cart interface {
	// Add adds item to the account's cart, confirmed as unvetted when confirmed is true, and fails with
	// app.ErrNotFound when the library's pages don't show the item, app.ErrUnvettedNotConfirmed when the library isn't
	// vetted and confirmed is false, or app.ErrCartFull when the cart holds domain.MaxCartItems other items.
	Add(ctx context.Context, accountID int64, item domain.CartItem, confirmed bool) (domain.CartItem, error)
	// Remove removes item from the account's cart, if it holds it.
	Remove(ctx context.Context, accountID int64, item domain.CartItem) error
	Empty(ctx context.Context, accountID int64) error
	Count(ctx context.Context, accountID int64) (int, error)
	// LibraryItems returns the account's items from the library owner/name.
	LibraryItems(ctx context.Context, accountID int64, owner, name string) ([]domain.CartItem, error)
	// Contents returns the account's cart, each item with its state, and its checkout.
	Contents(ctx context.Context, accountID int64) (views.Cart, error)
}

const (
	// cartHref is the signed-in visitor's cart. POST to it adds the item its library parameter names, with a group or
	// rule parameter, or neither for the whole library, and returns to its return parameter, or the cart. With an
	// unvetted parameter of confirmedUnvetted, it adds an item of a library Rulemart doesn't vet.
	cartHref = accountHref + "/cart"
	// removeFromCartHref removes the item its parameters name, as cartHref's do, with POST, and emptyCartHref every
	// item.
	removeFromCartHref = cartHref + "/remove"
	emptyCartHref      = cartHref + "/empty"
	// confirmCartHref asks the visitor to confirm adding the item its parameters name from a library Rulemart doesn't
	// vet, and checkoutHref shows the cart's checkout. Neither changes anything.
	confirmCartHref = cartHref + "/confirm"
	checkoutHref    = cartHref + "/checkout"
)

// confirmedUnvetted is the unvetted parameter of a form that adds an item the visitor confirmed adding from a library
// Rulemart doesn't vet.
const confirmedUnvetted = "confirmed"

// cartPurpose is the sign-in page's to parameter for a visitor who signs in to add to their cart.
const cartPurpose = "cart"

// cartAvailable reports whether visitors can collect rules in a cart: sign-in is available, and so are carts.
func (s *server) cartAvailable() bool {
	return s.Cart != nil && s.signInAvailable()
}

// cartSignIn returns where a visitor who isn't signed in goes to sign in and add to their cart, returning to back.
func (s *server) cartSignIn(back string) string {
	return s.absolute(signInHref + "?" + url.Values{"return": {back}, "to": {cartPurpose}}.Encode())
}

// cartQuery returns the query that names item to the cart's forms, with back to return to, unless it's empty.
func cartQuery(item domain.CartItem, back string) url.Values {
	query := url.Values{"library": {item.FullName()}}
	switch item.Kind {
	case domain.CartGroup:
		query.Set("group", item.Path)
	case domain.CartRule:
		query.Set("rule", item.Path)
	}
	if back != "" {
		query.Set("return", back)
	}
	return query
}

// cartItemOf returns the item query names, as cartQuery names it, or fails with domain.ErrNotCartItem.
func cartItemOf(query url.Values) (domain.CartItem, error) {
	return domain.ParseCartItem(query.Get("library"), query.Get("group"), query.Get("rule"))
}

// cartReturn returns where a change to the cart returns, by its query: the return parameter, as a path on this site,
// or else the cart.
func cartReturn(query url.Values) string {
	if query.Has("return") {
		return returnPath(query.Get("return"))
	}
	return cartHref
}

// libraryCart returns the signed-in visitor's items from the library owner/name, or nil for anyone else, or when
// carts aren't available.
func (s *server) libraryCart(r *http.Request, owner, name string) ([]domain.CartItem, error) {
	v := visitorOf(r.Context())
	if v.account == nil || !s.cartAvailable() {
		return nil, nil
	}
	return s.Cart.LibraryItems(r.Context(), v.account.ID, owner, name)
}

// cartControl is a way to add an item to the cart: a form that adds it, a link to confirm adding it from an unvetted
// library, or a link that signs a visitor in and returns them, on a page that's the same for everyone. Once the cart
// holds the item, or an item that imports it, it says so, and leads to the cart.
type cartControl struct {
	// shown is false where no one can collect rules in a cart.
	shown bool
	// held is true when the signed-in visitor's cart holds the item, or another that imports it.
	held bool
	// action is where the add form posts, confirm the page that confirms adding from an unvetted library, and signIn
	// the link that signs a visitor in; at most one isn't empty, and none while held.
	action, confirm, signIn string
	// name says what the control adds, to screen readers, such as "Add rule Return errors to your cart".
	name string
}

// newCartControl returns the control that adds item, of a library vetted or not, named name, from the page r asks
// for, where the signed-in visitor's cart holds items of the library.
func (s *server) newCartControl(r *http.Request, vetted bool, items []domain.CartItem, item domain.CartItem, name string) cartControl {
	if !s.cartAvailable() {
		return cartControl{}
	}
	v := visitorOf(r.Context())
	c := cartControl{shown: true, name: name}
	switch query := cartQuery(item, v.here).Encode(); {
	case v.account == nil:
		c.signIn = s.cartSignIn(v.here)
	case holds(items, item):
		c.held = true
	case vetted:
		c.action = cartHref + "?" + query
	default:
		c.confirm = confirmCartHref + "?" + query
	}
	return c
}

// holds reports whether items, a cart's items from one library, hold item or an item that imports it.
func holds(items []domain.CartItem, item domain.CartItem) bool {
	for _, held := range items {
		if held.Kind == item.Kind && strings.EqualFold(held.Path, item.Path) {
			return true
		}
	}
	_, covered := domain.Covering(item, items)
	return covered
}

// addToCart adds the item the query names to the signed-in visitor's cart, and returns to the return parameter, or
// the cart. An item of an unvetted library that the visitor didn't confirm leads to the page that confirms it; a full
// cart returns saying so. A visitor who isn't signed in is sent to sign in and return there, and changes nothing.
func (s *server) addToCart(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	back := cartReturn(query)
	v := visitorOf(r.Context())
	if v.account == nil {
		seeOther(w, r, s.cartSignIn(back))
		return
	}
	item, err := cartItemOf(query)
	if err == nil {
		_, err = s.Cart.Add(r.Context(), v.account.ID, item, query.Get("unvetted") == confirmedUnvetted)
	}
	switch {
	case errors.Is(err, app.ErrUnvettedNotConfirmed):
		seeOther(w, r, confirmCartHref+"?"+cartQuery(item, back).Encode())
	case errors.Is(err, domain.ErrNotCartItem), errors.Is(err, app.ErrNotFound):
		s.cartItemNotFound(w, r)
	case errors.Is(err, app.ErrCartFull):
		setNotice(w, "cart-full")
		seeOther(w, r, back)
	case err != nil:
		s.fail(w, r, err)
	default:
		seeOther(w, r, back)
	}
}

// cartItemNotFound answers a cart action on an item no library's pages show.
func (s *server) cartItemNotFound(w http.ResponseWriter, r *http.Request) {
	s.renderPrivate(w, r, http.StatusNotFound, messagePage(s.chrome, "Not found",
		"Rulemart has no such library, group, or rule to add to your cart."))
}

// removeFromCart removes the item the query names from the signed-in visitor's cart, and returns as addToCart does.
func (s *server) removeFromCart(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	back := cartReturn(query)
	v := visitorOf(r.Context())
	if v.account == nil {
		seeOther(w, r, s.cartSignIn(back))
		return
	}
	item, err := cartItemOf(query)
	if err != nil {
		s.cartItemNotFound(w, r)
		return
	}
	if err := s.Cart.Remove(r.Context(), v.account.ID, item); err != nil {
		s.fail(w, r, err)
		return
	}
	seeOther(w, r, back)
}

// emptyCart removes every item from the signed-in visitor's cart, and returns to it, saying so.
func (s *server) emptyCart(w http.ResponseWriter, r *http.Request) {
	v := visitorOf(r.Context())
	if v.account == nil {
		seeOther(w, r, s.cartSignIn(cartHref))
		return
	}
	if err := s.Cart.Empty(r.Context(), v.account.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	setNotice(w, "cart-emptied")
	seeOther(w, r, cartHref)
}

// cartPage shows the signed-in visitor's cart, or sends anyone else to sign in first.
func (s *server) cartPage(w http.ResponseWriter, r *http.Request) {
	account, ok := s.signedIn(w, r, cartHref)
	if !ok {
		return
	}
	cart, err := s.Cart.Contents(r.Context(), account.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.renderPrivate(w, r, http.StatusOK, cartPage(s.chrome, newCartView(cart, s.assets.iconURL)))
}

// checkoutPage shows the prompt and configuration that import the signed-in visitor's cart, or sends anyone else to
// sign in first.
func (s *server) checkoutPage(w http.ResponseWriter, r *http.Request) {
	account, ok := s.signedIn(w, r, checkoutHref)
	if !ok {
		return
	}
	cart, err := s.Cart.Contents(r.Context(), account.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.renderPrivate(w, r, http.StatusOK, checkoutPage(s.chrome, newCheckoutView(cart)))
}

// confirmCartPage asks the signed-in visitor to confirm adding the item the query names from a library Rulemart
// doesn't vet, with the warning every unvetted page shows, or sends anyone else to sign in first. The item must be one
// its library's pages show.
func (s *server) confirmCartPage(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if _, ok := s.signedIn(w, r, confirmCartHref+"?"+query.Encode()); !ok {
		return
	}
	item, err := cartItemOf(query)
	if err != nil {
		s.cartItemNotFound(w, r)
		return
	}
	page, err := s.catalog.LibraryPage(r.Context(), item.Owner, item.Name)
	if errors.Is(err, app.ErrNotFound) {
		s.cartItemNotFound(w, r)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	view, ok := newConfirmView(page, item, cartReturn(query), s.assets.iconURL)
	if !ok {
		s.cartItemNotFound(w, r)
		return
	}
	s.renderPrivate(w, r, http.StatusOK, confirmCartPage(s.chrome, view))
}

// confirmView is what the page that confirms adding an item from an unvetted library shows.
type confirmView struct {
	library libraryView
	// what names the item, such as "the rule Retry everything", and id is its ID, or the library's name for the whole
	// library.
	what, id string
	// action is where Add to cart posts, and back where Cancel leads.
	action, back string
}

// newConfirmView describes adding item, from the library on page, and returning to back, or reports false when the
// page doesn't show the item.
func newConfirmView(page views.LibraryPage, item domain.CartItem, back string, iconURL func(string) string) (confirmView, bool) {
	lib := newLibraryView(page.Library)
	view := confirmView{library: lib, back: back}
	switch item.Kind {
	case domain.CartLibrary:
		view.what, view.id = "every group of "+lib.fullName(), lib.fullName()
	case domain.CartGroup:
		i := slices.IndexFunc(page.Groups, func(g views.Group) bool { return strings.EqualFold(g.Path, item.Path) })
		if i < 0 {
			return confirmView{}, false
		}
		label := newGroupLabel(page.Groups[i].Path, page.Groups[i].Canonical)
		item.Path, view.id, view.what = label.id, label.id, "the group "+label.id
		if label.canonical {
			view.what = "the group " + label.name
		}
	case domain.CartRule:
		i := slices.IndexFunc(page.Rules, func(r views.RuleCard) bool { return strings.EqualFold(r.Path, item.Path) })
		if i < 0 {
			return confirmView{}, false
		}
		item.Path, view.id, view.what = page.Rules[i].Path, page.Rules[i].Path, "the rule "+titleOrID(page.Rules[i].Title, page.Rules[i].Path)
	}
	item.Owner, item.Name = page.Library.Owner, page.Library.Name
	query := cartQuery(item, back)
	if !page.Library.Vetted {
		query.Set("unvetted", confirmedUnvetted)
	}
	view.action = cartHref + "?" + query.Encode()
	return view, true
}

// cartView is what the cart page shows.
type cartView struct {
	libraries []cartLibraryView
	// items counts every item, and leftOut those checkout leaves out.
	items, leftOut int
}

// cartLibraryView is a library in the cart, with its items.
type cartLibraryView struct {
	owner, name, avatar string
	// href is the library's page, empty for one that's neither vetted nor listed, which has none.
	href string
	// unvetted marks a library the release doesn't vet that a listing names, and gone one neither vetted nor listed.
	unvetted, gone bool
	// tag is the latest release, which checkout pins the library to.
	tag   string
	items []cartItemView
}

// cartItemView is one item in the cart.
type cartItemView struct {
	kind domain.CartItemKind
	// title is what the item is: a rule's title, a canonical group's name, a group's ID, or "Every group" for a whole
	// library; id is a rule's or group's ID, shown under it, and href the item's page, when it has one.
	title, id, href string
	label           groupLabel
	icon            groupIcon
	// rules counts the current rules of a group or whole library.
	rules int
	// ready is false for an item checkout leaves out, and note says why, or which item imports it already. noteHref,
	// when not empty, leads to what the visitor can do about it, which noteLink names.
	ready              bool
	note               string
	noteHref, noteLink string
	// remove is where the item's Remove button posts.
	remove string
}

// newCartView describes cart for its page, whose Remove buttons return to it. iconURL returns where the site serves an
// icon file.
func newCartView(cart views.Cart, iconURL func(string) string) cartView {
	view := cartView{items: cart.Items()}
	for _, lib := range cart.Libraries {
		l := cartLibraryView{
			owner: lib.Library.Owner, name: lib.Library.Name, avatar: lib.Library.OwnerAvatarURL,
			href: libraryHref(lib.Library.Owner, lib.Library.Name), unvetted: !lib.Vetted && lib.Listed,
			gone: !lib.Vetted && !lib.Listed, tag: domain.ReleaseTag(lib.LatestRelease),
		}
		if l.gone {
			l.href = ""
		}
		for _, it := range lib.Items {
			item := newCartItemView(l, it, iconURL)
			if !item.ready {
				view.leftOut++
			}
			l.items = append(l.items, item)
		}
		view.libraries = append(view.libraries, l)
	}
	return view
}

// newCartItemView describes it, an item of the cart's library lib.
func newCartItemView(lib cartLibraryView, it views.CartItem, iconURL func(string) string) cartItemView {
	item := cartItemView{
		kind: it.Item.Kind, id: it.Item.Path, rules: it.Rules, label: newGroupLabel(it.Group, it.CanonicalGroup),
		icon:   newGroupIcon(it.CanonicalGroup, iconURL),
		ready:  it.State == views.CartItemReady || it.State == views.CartItemCovered,
		remove: removeFromCartHref + "?" + cartQuery(it.Item, cartHref).Encode(),
	}
	switch it.Item.Kind {
	case domain.CartLibrary:
		item.title, item.href = "Every group", lib.href
	case domain.CartGroup:
		item.title = it.Item.Path
		if item.label.canonical {
			item.title = item.label.name
		}
		if lib.href != "" {
			item.href = lib.href + "?tab=rules#" + groupAnchor(it.Item.Path)
		}
	case domain.CartRule:
		item.title = titleOrID(it.Title, it.Item.Path)
		if lib.href != "" && it.State != views.CartItemMissing {
			item.href = lib.href + "/" + it.Item.Path
		}
	}
	switch it.State {
	case views.CartItemCovered:
		item.note = "Included with its group, " + it.CoveredBy.Path + "."
		if it.CoveredBy.Kind == domain.CartLibrary {
			item.note = "Included with every group of the library."
		}
	case views.CartItemRetired:
		item.note = "Retired in " + domain.ReleaseTag(it.RetiredIn) + ", so checkout leaves it out."
		item.noteHref, item.noteLink = item.href, "See what replaced it"
	case views.CartItemMissing:
		item.note = "This library no longer has this rule, so checkout leaves it out."
		if it.Item.Kind != domain.CartRule {
			item.note = "This library no longer has rules here, so checkout leaves it out."
		}
	case views.CartItemUnconfirmed:
		item.note = "Rulemart no longer vets this library, so checkout leaves this out until you confirm it."
		item.noteHref = confirmCartHref + "?" + cartQuery(it.Item, cartHref).Encode()
		item.noteLink = "Confirm"
	case views.CartItemGone:
		item.note = "This library is no longer on Rulemart, so checkout leaves it out."
	}
	return item
}

// checkoutView is what the checkout page shows.
type checkoutView struct {
	// prompt is for a coding agent, and config the configuration it adds; both are empty when nothing in the cart can
	// be checked out.
	prompt, config string
	sources        []checkoutSourceView
	// items counts the cart's items, and leftOut those checkout leaves out.
	items, leftOut int
	// unvetted is true when a library checkout imports isn't vetted, which the prompt names.
	unvetted bool
}

// checkoutSourceView is one library a checkout imports.
type checkoutSourceView struct {
	owner, name, avatar, href string
	// source is its source name in the configuration, and tag the release it's pinned to, which tagHref shows.
	source, tag, tagHref string
	// what says what of it the checkout imports, such as "1 group and 2 rules".
	what     string
	unvetted bool
}

// newCheckoutView describes cart's checkout.
func newCheckoutView(cart views.Cart) checkoutView {
	view := checkoutView{prompt: cart.Checkout.Prompt(), config: cart.Checkout.Config(), items: cart.Items()}
	avatars := map[string]string{}
	for _, lib := range cart.Libraries {
		avatars[lib.Library.FullName()] = lib.Library.OwnerAvatarURL
		for _, it := range lib.Items {
			if it.State != views.CartItemReady && it.State != views.CartItemCovered {
				view.leftOut++
			}
		}
	}
	for _, s := range cart.Checkout.Sources {
		lib := libraryView{owner: s.Library.Owner, name: s.Library.Name, href: libraryHref(s.Library.Owner, s.Library.Name)}
		view.sources = append(view.sources, checkoutSourceView{
			owner: lib.owner, name: lib.name, avatar: avatars[s.Library.FullName()], href: lib.href, source: s.Name,
			tag: domain.ReleaseTag(s.Library.Release), tagHref: releaseHref(lib, s.Library.Release), what: importedWhat(s),
			unvetted: !s.Library.Vetted,
		})
		view.unvetted = view.unvetted || !s.Library.Vetted
	}
	return view
}

// importedWhat says what of its library source imports, such as "Every group", or "1 group and 2 rules".
func importedWhat(source domain.CheckoutSource) string {
	if source.All {
		return "Every group"
	}
	var parts []string
	if n := len(source.Groups); n > 0 {
		parts = append(parts, plural(n, "group", "groups"))
	}
	if n := len(source.Rules); n > 0 {
		parts = append(parts, plural(n, "rule", "rules"))
	}
	return strings.Join(parts, " and ")
}

// cartCount says how many items a cart holds, as the header's cart link shows it: up to 99, then 99+.
func cartCount(n int) string {
	if n > 99 {
		return "99+"
	}
	return strconv.Itoa(n)
}
