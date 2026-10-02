// Collect rules in a cart from library, rule, and group pages, confirm adding from an unvetted library, see and empty
// the cart, and check it out as a prompt for a coding agent.

package web

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// Cart keeps signed-in visitors' carts. catalog/app.Cart implements it.
type Cart interface {
	// Add adds item to the account's cart, confirmed as unvetted when confirmed is true, and returns it as its library
	// spells it. It fails with app.ErrNotFound when the library's pages don't show the item,
	// app.ErrUnvettedNotConfirmed when the library isn't vetted and confirmed is false, or app.ErrCartFull when the
	// cart would hold more than domain.MaxCartItems items. A group or whole library takes the place of the items of it
	// the cart holds.
	Add(ctx context.Context, accountID int64, item domain.CartItem, confirmed bool) (domain.CartItem, error)
	// Remove removes item from the account's cart, if it holds it.
	Remove(ctx context.Context, accountID int64, item domain.CartItem) error
	Empty(ctx context.Context, accountID int64) error
	// Held returns the account's items, with whether the visitor confirmed each as unvetted.
	Held(ctx context.Context, accountID int64) ([]views.HeldCartItem, error)
	// Contents returns the account's cart, each item with its state, and its checkout.
	Contents(ctx context.Context, accountID int64) (views.Cart, error)
}

const (
	// cartHref is the signed-in visitor's cart. POST to it adds the item its library parameter names, with a group or
	// rule parameter, or neither for the whole library, and returns to its return parameter, or the cart. With an
	// unvetted parameter of confirmedUnvetted, it adds an item of a library Rulemart doesn't vet.
	cartHref = accountHref + "/cart"
	// removeFromCartHref removes the item its parameters name, as cartHref's do, with POST. emptyCartHref asks to
	// confirm emptying the cart, and empties it with POST.
	removeFromCartHref = cartHref + "/remove"
	emptyCartHref      = cartHref + "/empty"
	// confirmCartHref asks the visitor to confirm adding the item its parameters name from a library Rulemart doesn't
	// vet, and checkoutHref shows the cart's checkout. Neither changes anything.
	confirmCartHref = cartHref + "/confirm"
	checkoutHref    = cartHref + "/checkout"
)

const (
	// confirmedUnvetted is the unvetted parameter of a form that adds an item the visitor confirmed adding from a
	// library Rulemart doesn't vet.
	confirmedUnvetted = "confirmed"
	// cartPurpose is the sign-in page's to parameter for a visitor who signs in to add to their cart.
	cartPurpose = "cart"
	// cartPromptParam names, on a page's address that a visitor returns to after signing in to add an item, the
	// item, as cartNoticeSubject encodes it. The page takes it off, and offers, once, to add the item.
	cartPromptParam = "add"
	// addedToCartKey, removedFromCartKey, and cartPromptKey are the subjectNotices that the visitor added or removed an
	// item, or signed in to add one.
	addedToCartKey     = "added-to-cart"
	removedFromCartKey = "removed-from-cart"
	cartPromptKey      = "cart-prompt"
)

// cartSubjectNotices are what a cart's subject notice says on a page that doesn't show its item.
var cartSubjectNotices = map[string]string{
	addedToCartKey: "Added to your cart.", removedFromCartKey: "Removed from your cart.", cartPromptKey: "You're signed in.",
}

// cartNoticeSubject returns item as a cart's subject notice, or cartPromptParam, names it: its library, kind, and
// ID, joined by |, which no part holds.
func cartNoticeSubject(item domain.CartItem) string {
	return item.FullName() + "|" + string(item.Kind) + "|" + item.Path
}

// parseCartNoticeSubject returns the item text names, as cartNoticeSubject writes it, or false when it names none.
func parseCartNoticeSubject(text string) (domain.CartItem, bool) {
	parts := strings.Split(text, "|")
	if len(parts) != 3 {
		return domain.CartItem{}, false
	}
	var group, rule string
	switch domain.CartItemKind(parts[1]) {
	case domain.CartLibrary:
		if parts[2] != "" {
			return domain.CartItem{}, false
		}
	case domain.CartGroup:
		group = parts[2]
	case domain.CartRule:
		rule = parts[2]
	default:
		return domain.CartItem{}, false
	}
	item, err := domain.ParseCartItem(parts[0], group, rule)
	return item, err == nil && item.Kind == domain.CartItemKind(parts[1])
}

// namesCartItem reports whether text names a cart item, as a cart's subject notice does.
func namesCartItem(text string) bool {
	_, ok := parseCartNoticeSubject(text)
	return ok
}

// sameItem reports whether a and b are the same item, their names matched without regard to case.
func sameItem(a, b domain.CartItem) bool {
	return a.Kind == b.Kind && strings.EqualFold(a.FullName(), b.FullName()) && strings.EqualFold(a.Path, b.Path)
}

// cartAvailable reports whether visitors can collect rules in a cart: sign-in is available, and so are carts.
func (s *server) cartAvailable() bool {
	return s.Cart != nil && s.signInAvailable()
}

// cartSignIn returns where a visitor who isn't signed in goes to sign in, returning to back. With an item, the page
// they return to offers to add it.
func (s *server) cartSignIn(back string, item *domain.CartItem) string {
	if item != nil {
		separator := "?"
		if strings.Contains(back, "?") {
			separator = "&"
		}
		back += separator + url.Values{cartPromptParam: {cartNoticeSubject(*item)}}.Encode()
	}
	return s.absolute(signInHref + "?" + url.Values{"return": {back}, "to": {cartPurpose}}.Encode())
}

// withoutCartPrompt answers a page's address with cartPromptParam, which a visitor returns to after signing in to add
// an item, and reports whether it did: it redirects to the address without it, with a one-time offer to add the item
// for a signed-in visitor, carried in the notice cookie, so reloading or sharing the page doesn't repeat it. It never
// adds anything.
func (s *server) withoutCartPrompt(w http.ResponseWriter, r *http.Request) bool {
	if !r.URL.Query().Has(cartPromptParam) {
		return false
	}
	var kept []string
	for pair := range strings.SplitSeq(r.URL.RawQuery, "&") {
		if name, _, _ := strings.Cut(pair, "="); name != cartPromptParam {
			kept = append(kept, pair)
		}
	}
	target := url.URL{Path: r.URL.EscapedPath(), RawQuery: strings.Join(kept, "&")}
	item, ok := parseCartNoticeSubject(r.URL.Query().Get(cartPromptParam))
	if visitorOf(r.Context()).account == nil || !ok || !s.cartAvailable() {
		redirect(w, r, target.String())
		return true
	}
	setSubjectNotice(w, cartPromptKey, cartNoticeSubject(item))
	seeOther(w, r, target.String())
	return true
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
func libraryCart(r *http.Request, owner, name string) []views.HeldCartItem {
	var items []views.HeldCartItem
	for _, held := range visitorOf(r.Context()).held {
		if strings.EqualFold(held.Item.FullName(), owner+"/"+name) {
			items = append(items, held)
		}
	}
	return items
}

// heldState is what a cart control says of an item the cart holds, or of one it would add.
type heldState int

const (
	// notHeld is an item the cart doesn't hold, which the control adds.
	notHeld heldState = iota
	// heldReady is an item the cart holds, which checkout imports.
	heldReady
	// heldInGroup and heldInLibrary are items the cart's group or whole library imports.
	heldInGroup
	heldInLibrary
	// heldUnconfirmed is an item of a library Rulemart doesn't vet, which the visitor didn't confirm, so checkout
	// leaves it out until they do.
	heldUnconfirmed
	// heldRetired is a rule a release retired, which checkout leaves out.
	heldRetired
)

// cartControl is a way to add an item to the cart: a form that adds it, a link to confirm adding it from an unvetted
// library, or a link that signs a visitor in and returns them, on a page that's the same for everyone. Once the cart
// holds the item, or an item that imports it, it says so, and leads to the cart, or to the confirmation the item
// needs.
type cartControl struct {
	// shown is false where no one can collect rules in a cart, and for a retired rule the cart doesn't hold.
	shown bool
	held  heldState
	// action is where the add form posts, confirm the page that confirms adding from an unvetted library, signIn the
	// link that signs a visitor in, and href where a held item's control leads; only one isn't empty.
	action, confirm, signIn, href string
	// label is what the control reads while it adds, and what says what it adds, such as "the rule Return errors",
	// to screen readers, after label, and in the notices about the item.
	label, what string
	// id is the control's element ID, which an addition's return address names as its fragment. focused is true when
	// the page that follows adding the item, or signing in to, focuses it, which only one control on a page is, and
	// prompt when it offers to add the item after signing in.
	id              string
	focused, prompt bool
	// notice is what the page says about the item, after adding it, or signing in to: empty for any other control.
	notice string
}

// cartControlID returns the element ID of item's control on a page, unique among the controls a page has.
func cartControlID(item domain.CartItem) string {
	switch item.Kind {
	case domain.CartGroup:
		return "cart-" + slug(item.Owner+"-"+item.Name) + "-" + groupAnchor(item.Path)
	case domain.CartRule:
		return "cart-rule"
	}
	return "cart-library"
}

// slug returns text with each run of characters an element ID shouldn't hold as a hyphen.
func slug(text string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

// newCartControl returns the control, reading label, that adds item, what it says it adds, from a library vetted or
// not, on the page r asks for. A retired rule's control only says what the cart holds.
func (s *server) newCartControl(r *http.Request, vetted, retired bool, item domain.CartItem, label, what string) cartControl {
	if !s.cartAvailable() {
		return cartControl{}
	}
	v := visitorOf(r.Context())
	c := cartControl{shown: true, label: label, what: what, id: cartControlID(item)}
	held := libraryCart(r, item.Owner, item.Name)
	i := slices.IndexFunc(held, func(h views.HeldCartItem) bool { return sameItem(h.Item, item) })
	var covering domain.CartItem
	covered := false
	if i < 0 {
		heldItems := make([]domain.CartItem, len(held))
		for j, h := range held {
			heldItems[j] = h.Item
		}
		covering, covered = domain.Covering(item, heldItems)
	}
	query := cartQuery(item, v.here).Encode()
	switch {
	case v.account == nil && !retired:
		c.signIn = s.cartSignIn(v.here, &item)
	case v.account == nil:
		return cartControl{}
	case i >= 0 && !vetted && !held[i].Confirmed:
		c.held, c.href = heldUnconfirmed, confirmCartHref+"?"+query
	case i >= 0 && retired:
		c.held, c.href = heldRetired, cartHref
	case i >= 0:
		c.held, c.href = heldReady, cartHref
	case covered && covering.Kind == domain.CartLibrary:
		c.held, c.href = heldInLibrary, cartHref
	case covered:
		c.held, c.href = heldInGroup, cartHref
	case retired:
		return cartControl{}
	case vetted:
		c.action = cartHref + "?" + query
	default:
		c.confirm = confirmCartHref + "?" + query
	}
	if subject, ok := parseCartNoticeSubject(v.noticeSubject); ok && sameItem(subject, item) {
		switch v.noticeKey {
		case addedToCartKey:
			c.focused, c.notice = true, "Added "+what+" to your cart."
		case cartPromptKey:
			c.focused, c.prompt = true, c.action != "" || c.confirm != ""
			c.notice = "You're signed in. Add " + what + " to your cart?"
			if !c.prompt {
				c.notice = "You're signed in. Your cart has " + what + " already."
			}
		}
	}
	return c
}

// addToCart adds the item the query names to the signed-in visitor's cart, and returns to the return parameter, or
// the cart, which says what it added and focuses its control. An item of an unvetted library that the visitor didn't
// confirm leads to the page that confirms it; a full cart returns saying so. A visitor who isn't signed in is sent to
// sign in and return there, and changes nothing.
func (s *server) addToCart(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	back := cartReturn(query)
	v := visitorOf(r.Context())
	item, err := cartItemOf(query)
	if v.account == nil {
		var offer *domain.CartItem
		if err == nil {
			offer = &item
		}
		seeOther(w, r, s.cartSignIn(back, offer))
		return
	}
	var added domain.CartItem
	if err == nil {
		added, err = s.Cart.Add(r.Context(), v.account.ID, item, query.Get("unvetted") == confirmedUnvetted)
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
		s.failCart(w, r, err)
	default:
		setSubjectNotice(w, addedToCartKey, cartNoticeSubject(added))
		// A group's control is one of several on a page, which autofocus alone can't bring into view on a long one.
		if added.Kind == domain.CartGroup && !strings.Contains(back, "#") {
			back += "#" + cartControlID(added)
		}
		seeOther(w, r, back)
	}
}

// cartParams are the query parameters that name what a cart's write acts on, which come from the visitor.
var cartParams = []string{"library", "group", "rule", "return"}

// failCart fails a cart's write, as fail does, logging err with each value of cartParams in r's query replaced by the
// parameter's name, such as {library}, longest first, so the logs keep no library or rule a visitor put in a cart, as
// fail keeps no page's path.
func (s *server) failCart(w http.ResponseWriter, r *http.Request, err error) {
	text := err.Error()
	query := r.URL.Query()
	params := slices.Clone(cartParams)
	slices.SortFunc(params, func(a, b string) int { return len(query.Get(b)) - len(query.Get(a)) })
	for _, name := range params {
		if value := query.Get(name); value != "" {
			text = strings.ReplaceAll(text, value, "{"+name+"}")
		}
	}
	s.fail(w, r, errors.New(text))
}

// cartItemNotFound answers a cart action on an item no library's pages show.
func (s *server) cartItemNotFound(w http.ResponseWriter, r *http.Request) {
	s.renderPrivate(w, r, http.StatusNotFound, messagePage(s.chrome, "Not found",
		"Rulemart has no such library, group, or rule to add to your cart."))
}

// removeFromCart removes the item the query names from the signed-in visitor's cart, and returns to the return
// parameter, or the cart, which says what it removed, and focuses the next item's Remove.
func (s *server) removeFromCart(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	back := cartReturn(query)
	v := visitorOf(r.Context())
	if v.account == nil {
		seeOther(w, r, s.cartSignIn(back, nil))
		return
	}
	item, err := cartItemOf(query)
	if err != nil {
		s.cartItemNotFound(w, r)
		return
	}
	if err := s.Cart.Remove(r.Context(), v.account.ID, item); err != nil {
		s.failCart(w, r, err)
		return
	}
	setSubjectNotice(w, removedFromCartKey, cartNoticeSubject(item))
	seeOther(w, r, back)
}

// emptyCartPage asks the signed-in visitor to confirm emptying their cart, or sends anyone else to sign in first.
func (s *server) emptyCartPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.signedIn(w, r, emptyCartHref); !ok {
		return
	}
	s.renderPrivate(w, r, http.StatusOK, emptyCartPage(s.chrome, visitorOf(r.Context()).cartItems))
}

// emptyCart removes every item from the signed-in visitor's cart, and returns to it, saying so.
func (s *server) emptyCart(w http.ResponseWriter, r *http.Request) {
	v := visitorOf(r.Context())
	if v.account == nil {
		seeOther(w, r, s.cartSignIn(cartHref, nil))
		return
	}
	if err := s.Cart.Empty(r.Context(), v.account.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	setNotice(w, "cart-emptied")
	seeOther(w, r, cartHref)
}

// cartPage shows the signed-in visitor's cart, or sends anyone else to sign in first. After removing an item, it says
// which, and focuses the Remove of the item after it, or before it when it was the last, or the way to browse when
// the cart is empty.
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
	view := newCartView(cart, s.assets.iconURL)
	v := visitorOf(r.Context())
	if removed, ok := parseCartNoticeSubject(v.noticeSubject); ok && v.noticeKey == removedFromCartKey {
		view.notice = "Removed " + itemDescription(removed) + " from your cart."
		view.focusAfter(removed)
	}
	view.focusBrowse = v.noticeKey == "cart-emptied" || view.notice != "" && view.items == 0
	s.renderPrivate(w, r, http.StatusOK, cartPage(s.chrome, view))
}

// itemDescription says what item is by its ID, as a page that doesn't show it can: "every group of owner/name", "the
// group techs/go of owner/name", or "the rule techs/go/return-errors of owner/name".
func itemDescription(item domain.CartItem) string {
	switch item.Kind {
	case domain.CartGroup:
		return "the group " + item.Path + " of " + item.FullName()
	case domain.CartRule:
		return "the rule " + item.Path + " of " + item.FullName()
	}
	return "every group of " + item.FullName()
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
// its library's pages show. When the cart holds the item already, it says so, and asks only to confirm one that needs
// it.
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
	view, ok := newConfirmView(page, item, cartReturn(query))
	if !ok {
		s.cartItemNotFound(w, r)
		return
	}
	held := libraryCart(r, page.Library.Owner, page.Library.Name)
	if i := slices.IndexFunc(held, func(h views.HeldCartItem) bool { return sameItem(h.Item, view.item) }); i >= 0 {
		view.held, view.needsConfirming = true, !page.Library.Vetted && !held[i].Confirmed
	}
	s.renderPrivate(w, r, http.StatusOK, confirmCartPage(s.chrome, view))
}

// cartCount says how many items a cart holds, as the header's cart shows it: every number, 100 included.
func cartCount(n int) string { return strconv.Itoa(n) }

// confirmView is what the page that confirms adding an item from an unvetted library shows.
type confirmView struct {
	library libraryView
	// item is the item as its library spells it.
	item domain.CartItem
	// what names the item, such as "the rule Retry everything", and id is its ID, or the library's name for the whole
	// library.
	what, id string
	// action is where Add to cart posts, and back where Cancel leads.
	action, back string
	// held is true when the cart holds the item already, and needsConfirming when it does, but checkout leaves it out
	// until the visitor confirms it.
	held, needsConfirming bool
}

// newConfirmView describes adding item, from the library on page, and returning to back, or reports false when the
// page doesn't show the item.
func newConfirmView(page views.LibraryPage, item domain.CartItem, back string) (confirmView, bool) {
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
	view.item = item
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
	// notice says what the visitor removed, and focusBrowse focuses the way to browse an empty cart, after removing
	// its last item or emptying it.
	notice      string
	focusBrowse bool
}

// focusAfter focuses the Remove of the item after removed, in the cart's order, or of the last item when none comes
// after it.
func (v *cartView) focusAfter(removed domain.CartItem) {
	var last *cartItemView
	for i := range v.libraries {
		lib := &v.libraries[i]
		for j := range lib.items {
			it := &lib.items[j]
			if compareCartItems(it.item, removed) > 0 {
				it.focused = true
				return
			}
			last = it
		}
	}
	if last != nil {
		last.focused = true
	}
}

// compareCartItems orders items as the cart lists them: by library owner and name without regard to case, then the
// whole library, groups, and rules, each by ID.
func compareCartItems(a, b domain.CartItem) int {
	kind := map[domain.CartItemKind]int{domain.CartLibrary: 0, domain.CartGroup: 1, domain.CartRule: 2}
	if c := strings.Compare(strings.ToLower(a.Owner), strings.ToLower(b.Owner)); c != 0 {
		return c
	}
	if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
		return c
	}
	if c := kind[a.Kind] - kind[b.Kind]; c != 0 {
		return c
	}
	return strings.Compare(a.Path, b.Path)
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
	item domain.CartItem
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
	// remove is where the item's Remove button posts, and focused is true when the page focuses it, after removing
	// the item before it.
	remove  string
	focused bool
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
		item: it.Item, kind: it.Item.Kind, id: it.Item.Path, rules: it.Rules, label: newGroupLabel(it.Group, it.CanonicalGroup),
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
	if item.title == item.id {
		// A rule whose title the catalog doesn't have, such as one its library no longer has, shows its ID once.
		item.id = ""
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
		item.note = "Rulemart doesn't vet this library, so checkout leaves this out until you confirm it."
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
	// unvetted is true when a library checkout imports isn't vetted, which the prompt names, and unvettedSources are
	// where their files land, such as .code-rules/vendor/rules/.
	unvetted        bool
	unvettedSources []string
}

// joinCode writes each of texts in code type, joined by commas and and.
func joinCode(texts []string) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		var out strings.Builder
		for i, text := range texts {
			switch {
			case i > 0 && i == len(texts)-1:
				out.WriteString(" and ")
			case i > 0:
				out.WriteString(", ")
			}
			out.WriteString("<code>" + templ.EscapeString(text) + "</code>")
		}
		_, err := io.WriteString(w, out.String())
		return err
	})
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
		// The library's latest release is on the first page of its releases, which its address names without a number.
		lib := libraryView{owner: s.Library.Owner, name: s.Library.Name, href: libraryHref(s.Library.Owner, s.Library.Name), releases: s.Library.Release}
		view.sources = append(view.sources, checkoutSourceView{
			owner: lib.owner, name: lib.name, avatar: avatars[s.Library.FullName()], href: lib.href, source: s.Name,
			tag: domain.ReleaseTag(s.Library.Release), tagHref: releaseHref(lib, s.Library.Release), what: importedWhat(s),
			unvetted: !s.Library.Vetted,
		})
		if !s.Library.Vetted {
			view.unvetted = true
			view.unvettedSources = append(view.unvettedSources, ".code-rules/vendor/"+s.Name+"/")
		}
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
