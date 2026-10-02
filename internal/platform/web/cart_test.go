package web_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/platform/web"
)

// fakeCart keeps carts in memory, as catalog/app.Cart does in Postgres: it adds only what the catalog's library pages
// show, and an unvetted library's items only when confirmed.
type fakeCart struct {
	mu      sync.Mutex
	catalog catalog
	// items holds each account's items, as owner/name kind path, with whether each was confirmed as unvetted.
	items map[int64]map[string]bool
	// contents is what Contents returns for each account.
	contents map[int64]views.Cart
	// err fails every call, heldErr only Held, and writeErr only Add and Remove.
	err, heldErr, writeErr error
}

func newFakeCart(c catalog) *fakeCart {
	return &fakeCart{catalog: c, items: map[int64]map[string]bool{}, contents: map[int64]views.Cart{}}
}

// key returns how the fake keeps item.
func key(item domain.CartItem) string {
	return fmt.Sprintf("%s %s %s", item.FullName(), item.Kind, item.Path)
}

func (f *fakeCart) Add(_ context.Context, accountID int64, item domain.CartItem, confirmed bool) (domain.CartItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := errors.Join(f.err, f.writeErr); err != nil {
		return domain.CartItem{}, err
	}
	page, ok := f.catalog.pages[strings.ToLower(item.FullName())]
	found := ok && (item.Kind == domain.CartLibrary ||
		slices.ContainsFunc(page.Groups, func(g views.Group) bool { return item.Kind == domain.CartGroup && strings.EqualFold(g.Path, item.Path) }) ||
		slices.ContainsFunc(page.Rules, func(r views.RuleCard) bool {
			return item.Kind == domain.CartRule && strings.EqualFold(r.Path, item.Path)
		}))
	switch {
	case !found:
		return domain.CartItem{}, fmt.Errorf("add %+v: %w", item, app.ErrNotFound)
	case !page.Library.Vetted && !confirmed:
		return domain.CartItem{}, fmt.Errorf("add %+v: %w", item, app.ErrUnvettedNotConfirmed)
	}
	item.Owner, item.Name, item.Path = page.Library.Owner, page.Library.Name, strings.ToLower(item.Path)
	cart := f.items[accountID]
	if cart == nil {
		cart = map[string]bool{}
		f.items[accountID] = cart
	}
	if _, held := cart[key(item)]; !held && len(cart) >= domain.MaxCartItems {
		return domain.CartItem{}, fmt.Errorf("add %+v: %w", item, app.ErrCartFull)
	}
	cart[key(item)] = cart[key(item)] || (confirmed && !page.Library.Vetted)
	return item, nil
}

func (f *fakeCart) Remove(_ context.Context, accountID int64, item domain.CartItem) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for held := range f.items[accountID] {
		if strings.EqualFold(held, key(item)) {
			delete(f.items[accountID], held)
		}
	}
	return errors.Join(f.err, f.writeErr)
}

func (f *fakeCart) Empty(_ context.Context, accountID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.items, accountID)
	return f.err
}

func (f *fakeCart) Held(_ context.Context, accountID int64) ([]views.HeldCartItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var items []views.HeldCartItem
	for held, confirmed := range f.items[accountID] {
		parts := strings.SplitN(held, " ", 3)
		o, n, _ := strings.Cut(parts[0], "/")
		items = append(items, views.HeldCartItem{
			Item: domain.CartItem{Owner: o, Name: n, Kind: domain.CartItemKind(parts[1]), Path: parts[2]}, Confirmed: confirmed,
		})
	}
	return items, errors.Join(f.err, f.heldErr)
}

func (f *fakeCart) Contents(_ context.Context, accountID int64) (views.Cart, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.contents[accountID], f.err
}

// all returns every item every account holds, as "<account> <owner/name> <kind> <path>", sorted, with confirmed
// after an unvetted one the visitor confirmed.
func (f *fakeCart) all() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var all []string
	for account, cart := range f.items {
		for item, confirmed := range cart {
			line := fmt.Sprintf("%d %s", account, item)
			if confirmed {
				line += " confirmed"
			}
			all = append(all, line)
		}
	}
	slices.Sort(all)
	return all
}

// cartSite is the pages of unvettedCatalog with sign-in and carts through fakes, and octocat's session cookie.
type cartSite struct {
	listingSite
	cart *fakeCart
}

func newCartSite(t *testing.T) cartSite {
	t.Helper()
	return newCartSiteWith(t, func(*web.Options) {})
}

// newCartSiteWith returns a cartSite whose options adjust changes.
func newCartSiteWith(t *testing.T, adjust func(*web.Options)) cartSite {
	t.Helper()
	c := unvettedCatalog()
	// Tests add group pages through the fake's catalog, whose maps are the site's.
	if c.groups == nil {
		c.groups = map[string]views.GroupPage{}
	}
	cart := newFakeCart(c)
	accounts := newFakeAccounts()
	site := accountsSite{accounts: accounts, gitHub: &fakeGitHub{identity: octocat}, logs: &bytes.Buffer{}}
	options := web.Options{Log: slog.New(slog.NewJSONHandler(site.logs, nil)), Accounts: accounts, GitHub: site.gitHub, Cart: cart}
	adjust(&options)
	handler, err := web.New(c, options)
	if err != nil {
		t.Fatal(err)
	}
	site.handler = handler
	token := accounts.signedIn(t, octocat)
	return cartSite{
		listingSite: listingSite{accountsSite: site, session: &http.Cookie{Name: sessionCookie, Value: string(token)}},
		cart:        cart,
	}
}

// cartPath returns where a form posts to add the item query names, as path, returning to back unless it's empty.
func cartPath(path string, query url.Values, back string) string {
	if back != "" {
		query.Set("return", back)
	}
	return path + "?" + query.Encode()
}

var (
	wholeLibrary = url.Values{"library": {"example/rules"}}
	goGroupItem  = url.Values{"library": {"example/rules"}, "group": {"techs/go"}}
	errorsItem   = url.Values{"library": {"example/rules"}, "rule": {"techs/go/return-errors"}}
	retryItem    = url.Values{"library": {"example/rules"}, "rule": {"practices/testing/verify-retry-limits"}}
	strangerItem = url.Values{"library": {"stranger/rules"}, "rule": {"techs/go/return-errors"}}
)

// clone returns a copy of query, to change.
func clone(query url.Values) url.Values {
	c := url.Values{}
	for k, v := range query {
		c[k] = slices.Clone(v)
	}
	return c
}

// A visitor who isn't signed in sees ways to add a whole library, each group, and a rule, each a link that signs them
// in and returns them to the page, which stays the same for everyone and cached.
func TestPagesOfferAVisitorWhoIsntSignedInToSignInAndAddToTheirCart(t *testing.T) {
	site := newCartSite(t)

	for path, labels := range map[string][]string{
		library:                      {"Add library to cart", "Add"},
		library + "?tab=rules":       {"Add library to cart"},
		library + "?tab=releases":    {"Add library to cart"},
		errorsRule:                   {"Add to cart"},
		errorsRule + "?tab=versions": {"Add to cart"},
		unvettedLibrary:              {"Add library to cart", "Add"},
		unvettedLibrary + "/techs/go/return-errors": {"Add to cart"},
	} {
		resp := send(t, site.handler, request{method: http.MethodGet, target: path})
		if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != "public, max-age=0, s-maxage=60" {
			t.Fatalf("%s: got %d, cached as %q; want a public page", path, resp.StatusCode, resp.Header.Get("Cache-Control"))
		}
		page := body(t, resp)
		for _, label := range labels {
			got := links(t, page, label)
			if len(got) == 0 {
				t.Errorf("%s: no link reads %s", path, label)
			}
			for _, href := range got {
				// Each leads to sign in, returning to the page, which then offers to add its item.
				target, err := url.Parse(href)
				back, _ := url.Parse(target.Query().Get("return"))
				if err != nil || target.Path != "/sign-in" || target.Query().Get("to") != "cart" || back.Path != strings.Split(path, "?")[0] ||
					!back.Query().Has("add") {
					t.Errorf("%s: %s leads to %q", path, label, href)
				}
			}
		}
		if actions := formActions(t, page); slices.ContainsFunc(actions, func(a string) bool { return strings.HasPrefix(a, "/account/cart") }) {
			t.Errorf("%s: a public page has a cart form %q", path, actions)
		}
	}
	if got := links(t, body(t, send(t, site.handler, request{method: http.MethodGet, target: library})), "Add"); len(got) != 3 {
		t.Errorf("the library's page has %d ways to add, want the library and each of its 2 groups", len(got))
	}
	assertShows(t, body(t, send(t, site.handler, request{method: http.MethodGet, target: "/sign-in?return=%2Fexample%2Frules&to=cart"})),
		"Sign in to collect rules in your cart. You'll come back to this page.")
}

// A signed-in visitor adds a whole library, a group, and a rule from their pages, each returning to the page, which then
// says the cart holds it, as it does a rule its group imports. Their pages are never cached.
func TestASignedInVisitorAddsToTheirCartFromPages(t *testing.T) {
	site := newCartSite(t)

	resp := site.signedInGet(t, library)
	if resp.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("a signed-in visitor's page is cached as %q", resp.Header.Get("Cache-Control"))
	}
	actions := formActions(t, body(t, resp))
	for _, want := range []string{
		cartPath("/account/cart", clone(wholeLibrary), library),
		cartPath("/account/cart", clone(goGroupItem), library),
		cartPath("/account/cart", url.Values{"library": {"example/rules"}, "group": {"practices/testing"}}, library),
	} {
		if !slices.Contains(actions, want) {
			t.Errorf("the library's forms post to %q, missing %q", actions, want)
		}
	}
	added := site.signedInPost(t, cartPath("/account/cart", clone(goGroupItem), library))
	if added.StatusCode != http.StatusSeeOther || added.Header.Get("Location") != library+"#cart-example-rules-group-techs-go" || added.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("adding answered %d to %q, cached as %q", added.StatusCode, added.Header.Get("Location"), added.Header.Get("Cache-Control"))
	}
	ruleForm := cartPath("/account/cart", clone(retryItem), retryRule)
	if got := formActions(t, body(t, site.signedInGet(t, retryRule))); !slices.Contains(got, ruleForm) {
		t.Fatalf("the rule's forms post to %q, want %q", got, ruleForm)
	}
	site.signedInPost(t, ruleForm)

	if got := site.cart.all(); !slices.Equal(got, []string{
		"1 example/rules group techs/go", "1 example/rules rule practices/testing/verify-retry-limits",
	}) {
		t.Fatalf("the cart holds %q", got)
	}

	for path, text := range map[string]string{retryRule: "In cart", errorsRule: "Included with its group (in cart)"} {
		page := body(t, site.signedInGet(t, path))
		if got := links(t, page, text); !slices.Equal(got, []string{"/account/cart"}) {
			t.Errorf("%s: %s leads to %q", path, text, got)
		}
	}
	page := body(t, site.signedInGet(t, library))
	if got := links(t, page, "In cart"); len(got) != 1 {
		t.Errorf("the library's page shows %d items in the cart, want its Go group", len(got))
	}
}

// The page an addition returns to says what it added, once, and focuses the control that says the cart holds it, so
// keyboard and screen reader users land where they were; a group's, one of several on the page, the address's
// fragment also brings into view.
func TestTheReturnPageSaysWhatWasAdded(t *testing.T) {
	for _, c := range []struct {
		held           []string
		item           url.Values
		back, location string
		notice, id     string
	}{
		{nil, errorsItem, errorsRule, errorsRule, "Added the rule Return errors with context to your cart.", "cart-rule"},
		{nil, wholeLibrary, library + "?tab=rules", library + "?tab=rules", "Added every group of example/rules to your cart.", "cart-library"},
		{nil, goGroupItem, library, library + "#cart-example-rules-group-techs-go", "Added the group Go to your cart.", "cart-example-rules-group-techs-go"},
		{
			[]string{"example/rules rule techs/go/return-errors"}, goGroupItem, library, library + "#cart-example-rules-group-techs-go",
			"Added the group Go to your cart, in place of 1 item of it you'd added before.", "cart-example-rules-group-techs-go",
		},
		{
			[]string{"example/rules group techs/go"}, errorsItem, errorsRule, errorsRule,
			"Your cart has the rule Return errors with context already, with its group.", "cart-rule",
		},
	} {
		site := newCartSite(t)
		site.cart.items[octocatID] = map[string]bool{}
		for _, held := range c.held {
			site.cart.items[octocatID][held] = false
		}
		resp := site.signedInPost(t, cartPath("/account/cart", clone(c.item), c.back))
		if resp.Header.Get("Location") != c.location {
			t.Fatalf("adding %v returned to %q, want %q", c.item, resp.Header.Get("Location"), c.location)
		}
		page := body(t, send(t, site.handler, request{method: http.MethodGet, target: c.back, cookies: []*http.Cookie{site.session, cookie(resp, noticeCookie)}}))
		assertShows(t, page, c.notice)
		if got := strings.Count(page, " autofocus"); got != 1 || !strings.Contains(page, `id="`+c.id+`"`) {
			t.Errorf("adding %v: the page has %d autofocused controls, and wants one, %s", c.item, got, c.id)
		}
		if got := strings.Count(page, " data-autofocused"); got != 1 {
			t.Errorf("adding %v: the page marks %d controls as focused by the page, and wants one", c.item, got)
		}
		if again := body(t, site.signedInGet(t, c.back)); strings.Contains(again, " autofocus") || strings.Contains(again, " data-autofocused") || strings.Contains(visibleText(t, again), "Added") {
			t.Errorf("adding %v: the page says so again on the next visit", c.item)
		}
	}
	site := newCartSite(t)
	// Another page, which doesn't show the item, says only that something was added.
	resp := site.signedInPost(t, cartPath("/account/cart", clone(retryItem), "/"))
	assertShows(t, body(t, send(t, site.handler, request{method: http.MethodGet, target: "/", cookies: []*http.Cookie{site.session, cookie(resp, noticeCookie)}})),
		"Added to your cart.")

}

// A group named in another case than its library's returns to the fragment its control has, which the library's
// spelling names.
func TestAGroupInAnotherCaseReturnsToItsControl(t *testing.T) {
	site := newCartSite(t)

	resp := site.signedInPost(t, cartPath("/account/cart", url.Values{"library": {"Example/Rules"}, "group": {"TECHS/GO"}}, library))
	if want := library + "#cart-example-rules-group-techs-go"; resp.Header.Get("Location") != want {
		t.Fatalf("returned to %q, want %q", resp.Header.Get("Location"), want)
	}
}

// A failed change to the cart logs why, without the library, group, or rule the visitor named, as a failed page read
// logs only its route.
func TestAFailedCartChangeLogsNoNames(t *testing.T) {
	site := newCartSite(t)
	site.cart.writeErr = fmt.Errorf("add to cart library=%q kind=rule path=%q: the database is down", "example/rules", "techs/go/return-errors")

	for _, target := range []string{
		cartPath("/account/cart", clone(errorsItem), errorsRule), cartPath("/account/cart/remove", clone(errorsItem), ""),
	} {
		if resp := site.signedInPost(t, target); resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("%s: got %d, want 503", target, resp.StatusCode)
		}
	}
	logs := site.logs.String()
	for _, name := range []string{"example/rules", "return-errors"} {
		if strings.Contains(logs, name) {
			t.Errorf("the logs name %q:\n%s", name, logs)
		}
	}
	if !strings.Contains(logs, "the database is down") || !strings.Contains(logs, "{library}") {
		t.Errorf("the logs don't say why, with the parameter in its place:\n%s", logs)
	}
}

// A failed page keeps its error whole, even where the text matches a parameter's value, such as a tab's.
func TestAFailedPageLogsItsWholeError(t *testing.T) {
	site := newCartSite(t)
	site.cart.err = errors.New("read cart: the rules table is locked")

	if resp := site.signedInGet(t, library+"?tab=rules"); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503", resp.StatusCode)
	}
	if logs := site.logs.String(); !strings.Contains(logs, "the rules table is locked") {
		t.Errorf("the logs lost the error's words:\n%s", logs)
	}
}

// A rule's page says how the cart holds it: by itself, with its group or its whole library, still needing
// confirmation from an unvetted library, or, retired, left out of checkout. Each leads where the visitor acts on it,
// and names the rule to screen readers.
func TestARulesPageSaysHowTheCartHoldsIt(t *testing.T) {
	site := newCartSite(t)
	retired := site.cart.catalog.rules["example/rules/techs/go/return-errors"]
	retired.Rule.Path, retired.Rule.Retirement = "techs/go/old-errors", &views.Retirement{Release: 3, RetiredAt: day(3), Summaries: []string{"Drop it."}}
	site.cart.catalog.rules["example/rules/techs/go/old-errors"] = retired
	site.cart.items[octocatID] = map[string]bool{
		"example/rules rule practices/testing/verify-retry-limits": false,
		"example/rules group techs/go":                             false,
		"example/rules rule techs/go/old-errors":                   false,
		"stranger/rules rule techs/go/return-errors":               false,
	}
	strangerRule := unvettedLibrary + "/techs/go/return-errors"
	for path, want := range map[string]struct{ text, href, name string }{
		retryRule:                        {"In cart", "/account/cart", "In cart: the rule Verify retry limits. See your cart"},
		errorsRule:                       {"Included with its group (in cart)", "/account/cart", "Included with its group (in cart): the rule Return errors with context. See your cart"},
		library + "/techs/go/old-errors": {"In cart, left out of checkout because it's retired", "/account/cart", "In cart, left out of checkout because it's retired: the rule Return errors with context. See your cart"},
		strangerRule: {"In cart, needs confirming", cartPath("/account/cart/confirm", clone(strangerItem), strangerRule),
			"In cart, needs confirming: the rule Return errors with context. Confirm it"},
	} {
		page := body(t, site.signedInGet(t, path))
		if got := links(t, page, want.text); !slices.Equal(got, []string{want.href}) {
			t.Errorf("%s: %q leads to %q, want %q", path, want.text, got, want.href)
		}
		if !strings.Contains(page, `aria-label="`+htmlEscape(want.name)+`"`) {
			t.Errorf("%s: no control is named %q", path, want.name)
		}
	}
	delete(site.cart.items[octocatID], "example/rules group techs/go")
	site.cart.items[octocatID]["example/rules library "] = false
	if got := links(t, body(t, site.signedInGet(t, errorsRule)), "Included with the library (in cart)"); len(got) != 1 {
		t.Errorf("a rule the whole library brings doesn't say so")
	}
	if page := body(t, send(t, site.handler, request{method: http.MethodGet, target: library + "/techs/go/old-errors"})); strings.Contains(visibleText(t, page), "cart") {
		t.Error("a retired rule's page offers a visitor who isn't signed in to add it")
	}
}

// A visitor who signs in from a page's Add returns to it offered, once, to add that item, with its control focused;
// nothing is added without their click, and an item the cart holds already says so.
func TestSigningInToAddOffersTheItemOnce(t *testing.T) {
	site := newCartSite(t)
	back := errorsRule + "?add=" + url.QueryEscape("example/rules|rule|techs/go/return-errors")

	signedOut := send(t, site.handler, request{method: http.MethodGet, target: back})
	if signedOut.StatusCode != http.StatusMovedPermanently || signedOut.Header.Get("Location") != errorsRule {
		t.Fatalf("signed out: answered %d to %q, want the page without the offer", signedOut.StatusCode, signedOut.Header.Get("Location"))
	}
	resp := site.signedInGet(t, back)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != errorsRule {
		t.Fatalf("signed in: answered %d to %q, want the page without the offer", resp.StatusCode, resp.Header.Get("Location"))
	}
	offer := cookie(resp, noticeCookie)
	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: errorsRule, cookies: []*http.Cookie{site.session, offer}}))
	assertShows(t, page, "You're signed in. Add the rule Return errors with context to your cart?")
	// The notice holds a button that adds the rule, focused, so the offer and its answer are on screen together.
	if strings.Count(page, " autofocus") != 1 || !strings.Contains(page, "ring-ink") {
		t.Error("the offer doesn't focus its button, and outline the control")
	}
	// A toast can't hold the button, so the offer stays a banner where scripts run.
	if kind := noticeToast(t, page); kind != "" {
		t.Errorf("the offer is a %q toast, want a banner", kind)
	}
	action := cartPath("/account/cart", clone(errorsItem), errorsRule)
	if got := formActions(t, page); countOf(got, action) != 2 {
		t.Errorf("the notice holds no button that adds the rule: %q", got)
	}
	if got := site.cart.all(); len(got) != 0 {
		t.Fatalf("signing in added %q", got)
	}
	site.signedInPost(t, cartPath("/account/cart", clone(errorsItem), ""))
	page = body(t, send(t, site.handler, request{method: http.MethodGet, target: errorsRule, cookies: []*http.Cookie{site.session, offer}}))
	assertShows(t, page, "You're signed in. Your cart has the rule Return errors with context already.")
	// With nothing to offer, it only reports, as a toast.
	if kind := noticeToast(t, page); kind != "status" {
		t.Errorf("the notice is a %q toast, want a status toast", kind)
	}
	// An offer for an item the page doesn't show says nothing at all.
	away := site.signedInGet(t, cartPath(library, url.Values{"add": {"example/rules|rule|techs/go/return-errors"}}, ""))
	if page := body(t, send(t, site.handler, request{method: http.MethodGet, target: library, cookies: []*http.Cookie{site.session, cookie(away, noticeCookie)}})); strings.Contains(visibleText(t, page), "signed in.") {
		t.Error("a page without the offered item says the visitor signed in")
	}
	// An offer the address names, and no cookie, says nothing: only this site's cookie can.
	for _, bad := range []string{"nobody%7Crule%7Cx", "example%2Frules%7Crule%7Ctechs%2Fgo%2Freturn-errors%3Cscript%3E"} {
		resp := site.signedInGet(t, errorsRule+"?add="+bad)
		if resp.StatusCode != http.StatusMovedPermanently && resp.StatusCode != http.StatusSeeOther {
			t.Errorf("%s: answered %d", bad, resp.StatusCode)
		}
		if c := cookie(resp, noticeCookie); c != nil && c.Value != "" && resp.StatusCode == http.StatusSeeOther && strings.Contains(c.Value, "script") {
			t.Errorf("%s: the offer names %q", bad, c.Value)
		}
	}
}

// A page's address with the cart's offer redirects once to the same address without it, however the parameter's name
// is encoded, keeping the rest of the query in order and the path as the visitor's browser spelled it, so the redirect
// leads to the page.
func TestTheCartOfferRedirectsOnceToTheAddressWithoutIt(t *testing.T) {
	site := newCartSite(t)
	add := url.QueryEscape("example/rules|rule|techs/go/return-errors")

	for target, want := range map[string]string{
		errorsRule + "?%61dd=" + add:                         errorsRule,
		errorsRule + "?a=1&add=" + add + "&b=2":              errorsRule + "?a=1&b=2",
		"/%65xample/rules/techs/go/return-errors?add=" + add: "/%65xample/rules/techs/go/return-errors",
		"/%65xample/rules?tab=rules&add=" + add:              "/%65xample/rules?tab=rules",
	} {
		assertRedirectsToPage(t, site.handler, target, want)
	}
}

// Removing an item says which, and focuses the next item's Remove, or the last one's after removing the last item;
// emptying asks first.
func TestRemovingFocusesTheNextItem(t *testing.T) {
	site := newCartSite(t)
	cart := exampleCart()
	site.cart.contents[octocatID] = cart
	removed := cart.Libraries[0].Items[1].Item
	resp := site.signedInPost(t, cartPath("/account/cart/remove", url.Values{"library": {"example/rules"}, "rule": {removed.Path}}, "/account/cart"))
	cart.Libraries[0].Items = slices.Delete(cart.Libraries[0].Items, 1, 2)
	site.cart.contents[octocatID] = cart
	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/account/cart", cookies: []*http.Cookie{site.session, cookie(resp, noticeCookie)}}))
	assertShows(t, page, "Removed the rule Verify retry limits of example/rules from your cart.")
	// The store lists items by ID, so the item after the removed one is techs/go/return-errors.
	next := `aria-label="Remove Return errors with context from your cart" autofocus`
	if !strings.Contains(page, next) || strings.Count(page, " autofocus") != 1 {
		t.Errorf("the page doesn't focus the next item's Remove")
	}

	site.signedInPost(t, cartPath("/account/cart", clone(errorsItem), ""))
	confirm := site.signedInGet(t, "/account/cart/empty")
	if confirm.StatusCode != http.StatusOK || confirm.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("the confirmation answered %d", confirm.StatusCode)
	}
	page = body(t, confirm)
	assertShows(t, page, "Empty your cart?", "This removes 1 item from your cart.", "Cancel")
	if got := formActions(t, page); !slices.Contains(got, "/account/cart/empty") {
		t.Errorf("the confirmation's forms post to %q", got)
	}
}

// The header shows a full cart's count in full.
func TestTheHeaderCountsAFullCart(t *testing.T) {
	site := newCartSite(t)
	site.cart.items[octocatID] = map[string]bool{}
	for i := range domain.MaxCartItems {
		site.cart.items[octocatID][fmt.Sprintf("example/rules rule techs/go/filler-%d", i)] = false
	}
	if page := body(t, site.signedInGet(t, "/")); !strings.Contains(page, ">100</span>") || strings.Contains(page, "99+") {
		t.Error("the header doesn't show 100")
	}
}

// A canonical group's page across libraries offers each library's group, and a library's All rules tab each group.
func TestGroupPagesAndAllRulesOfferGroups(t *testing.T) {
	site := newCartSite(t)
	site.cart.catalog.groups["techs/go"] = views.GroupPage{Path: "techs/go", Canonical: *goGroup, Libraries: []views.GroupLibrary{
		{Library: views.LibraryRef{Owner: "example", Name: "rules"}, Rules: []views.RuleCard{{Path: "techs/go/return-errors", Group: "techs/go", Title: "Return errors with context"}}},
	}}

	page := body(t, site.signedInGet(t, "/g/techs/go"))
	action := cartPath("/account/cart", clone(goGroupItem), "/g/techs/go")
	if got := formActions(t, page); !slices.Contains(got, action) {
		t.Errorf("the group's page posts to %q, want %q", got, action)
	}
	if !strings.Contains(page, `aria-label="Add this group: the group Go of example/rules"`) {
		t.Error("the group's control doesn't name its library")
	}
	resp := site.signedInPost(t, action)
	assertShows(t, body(t, send(t, site.handler, request{method: http.MethodGet, target: "/g/techs/go", cookies: []*http.Cookie{site.session, cookie(resp, noticeCookie)}})),
		"Added the group Go of example/rules to your cart.")

	rules := body(t, site.signedInGet(t, library+"?tab=rules"))
	testing := cartPath("/account/cart", url.Values{"library": {"example/rules"}, "group": {"practices/testing"}}, library+"?tab=rules")
	if got := formActions(t, rules); !slices.Contains(got, testing) {
		t.Errorf("the All rules tab posts to %q, want %q", got, testing)
	}
}

// The confirmation for an item the cart holds already says so, and adds nothing more.
func TestTheConfirmationForAHeldItemSaysSo(t *testing.T) {
	site := newCartSite(t)
	site.cart.items[octocatID] = map[string]bool{"stranger/rules rule techs/go/return-errors": true}

	page := body(t, site.signedInGet(t, cartPath("/account/cart/confirm", clone(strangerItem), "")))
	assertShows(t, page, "Your cart has this already.")
	if got := formActions(t, page); slices.ContainsFunc(got, func(a string) bool { return strings.HasPrefix(a, "/account/cart?") }) {
		t.Errorf("the page offers to add it again: %q", got)
	}
	site.cart.items[octocatID]["stranger/rules rule techs/go/return-errors"] = false
	assertShows(t, body(t, site.signedInGet(t, cartPath("/account/cart/confirm", clone(strangerItem), ""))),
		"Your cart has this, but checkout leaves it out until you confirm it")
}

// Adding twice, as a double click or a reload might, lands where once does.
func TestAddingTwiceIsLikeOnce(t *testing.T) {
	site := newCartSite(t)

	for range 2 {
		if resp := site.signedInPost(t, cartPath("/account/cart", clone(errorsItem), "")); resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/account/cart" {
			t.Fatalf("adding answered %d to %q, want the cart", resp.StatusCode, resp.Header.Get("Location"))
		}
	}
	if got := site.cart.all(); !slices.Equal(got, []string{"1 example/rules rule techs/go/return-errors"}) {
		t.Fatalf("the cart holds %q", got)
	}
}

// An unvetted library's pages lead a signed-in visitor to confirm adding from it, under the warning; adding without
// confirming leads there too, and confirming adds it, recorded as confirmed.
func TestAddingFromAnUnvettedLibraryNeedsConfirming(t *testing.T) {
	site := newCartSite(t)
	back := unvettedLibrary + "/techs/go/return-errors"
	confirm := cartPath("/account/cart/confirm", clone(strangerItem), back)

	if got := links(t, body(t, site.signedInGet(t, back)), "Add to cart"); !slices.Equal(got, []string{confirm}) {
		t.Fatalf("Add to cart leads to %q, want %q", got, confirm)
	}
	unconfirmed := site.signedInPost(t, cartPath("/account/cart", clone(strangerItem), back))
	if unconfirmed.StatusCode != http.StatusSeeOther || unconfirmed.Header.Get("Location") != confirm {
		t.Fatalf("adding unconfirmed answered %d to %q, want %q", unconfirmed.StatusCode, unconfirmed.Header.Get("Location"), confirm)
	}
	if got := site.cart.all(); len(got) != 0 {
		t.Fatalf("the cart holds %q before confirming", got)
	}

	resp := site.signedInGet(t, confirm)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("the confirmation answered %d, cached as %q", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}
	page := body(t, resp)
	assertShows(t, page, "This library has not been vetted. Tread carefully.", "Add the rule Return errors with context?",
		"techs/go/return-errors", "Add to cart anyway")
	if content, _ := robots(t, page); content != "noindex" {
		t.Errorf("robots %q, want noindex", content)
	}
	if got := links(t, page, "Cancel"); !slices.Equal(got, []string{back}) {
		t.Errorf("Cancel leads to %q, want %q", got, back)
	}
	confirmed := clone(strangerItem)
	confirmed.Set("unvetted", "confirmed")
	action := cartPath("/account/cart", confirmed, back)
	if got := formActions(t, page); !slices.Contains(got, action) {
		t.Fatalf("the confirmation's forms post to %q, want %q", got, action)
	}
	if resp := site.signedInPost(t, action); resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != back {
		t.Fatalf("confirming answered %d to %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if got := site.cart.all(); !slices.Equal(got, []string{"1 stranger/rules rule techs/go/return-errors confirmed"}) {
		t.Fatalf("the cart holds %q", got)
	}
}

// The confirmation names a group by its canonical name, and the whole library by name, and is missing for an item the
// library doesn't have, or for a visitor who isn't signed in, whom it sends to sign in.
func TestTheConfirmationNamesWhatItAdds(t *testing.T) {
	site := newCartSite(t)

	group := url.Values{"library": {"stranger/rules"}, "group": {"techs/go"}}
	assertShows(t, body(t, site.signedInGet(t, cartPath("/account/cart/confirm", group, ""))), "Add the group Go?")
	assertShows(t, body(t, site.signedInGet(t, "/account/cart/confirm?library=stranger%2Frules")), "Add every group of stranger/rules?")
	for _, query := range []string{"library=stranger%2Frules&rule=techs%2Fgo%2Fnothing", "library=nobody%2Fnothing", "library=stranger", "library=stranger%2Frules&group=techs%2Frust"} {
		if resp := site.signedInGet(t, "/account/cart/confirm?"+query); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: got %d, want 404", query, resp.StatusCode)
		}
	}
	target := cartPath("/account/cart/confirm", clone(strangerItem), "")
	resp := send(t, site.handler, request{method: http.MethodGet, target: target})
	want := "/sign-in?" + url.Values{"return": {target}}.Encode()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != want {
		t.Errorf("signed out: answered %d to %q, want %q", resp.StatusCode, resp.Header.Get("Location"), want)
	}
}

// Adding what no library's pages show is missing, and changes nothing.
func TestAddingWhatPagesDontShowIsMissing(t *testing.T) {
	site := newCartSite(t)

	for _, query := range []string{
		"library=example%2Frules&rule=techs%2Fgo%2Fnothing", "library=nobody%2Fnothing", "library=example", "",
		"library=example%2Frules&group=techs%2Fgo&rule=techs%2Fgo%2Freturn-errors",
		"library=exa%00mple%2Frules", "library=example%2Frul%FFes", "library=example%2Frules&rule=techs%2Fgo%2Fre%00turn",
	} {
		resp := site.signedInPost(t, "/account/cart?"+query)
		if resp.StatusCode != http.StatusNotFound || resp.Header.Get("Cache-Control") != "private, no-store" {
			t.Errorf("%s: got %d, cached as %q; want a private 404", query, resp.StatusCode, resp.Header.Get("Cache-Control"))
		}
	}
	if got := site.cart.all(); len(got) != 0 {
		t.Fatalf("the cart holds %q", got)
	}
}

// A full cart returns to the page saying so, once, and still takes what it holds.
func TestAFullCartSaysSo(t *testing.T) {
	site := newCartSite(t)
	site.cart.items[octocatID] = map[string]bool{}
	for i := range domain.MaxCartItems {
		site.cart.items[octocatID][fmt.Sprintf("example/rules rule techs/go/filler-%d", i)] = false
	}

	resp := site.signedInPost(t, cartPath("/account/cart", clone(errorsItem), errorsRule))
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != errorsRule {
		t.Fatalf("answered %d to %q, want the rule's page", resp.StatusCode, resp.Header.Get("Location"))
	}
	notice := cookie(resp, noticeCookie)
	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: errorsRule, cookies: []*http.Cookie{site.session, notice}}))
	assertShows(t, page, "Your cart holds 100 items, as many as it can.")
}

// A visitor who isn't signed in, such as one whose session ended in another tab, is sent to sign in and return to
// where they were, and nothing changes.
func TestChangingTheCartSignedOutSignsInAndReturns(t *testing.T) {
	site := newCartSite(t)

	for target, back := range map[string]string{
		cartPath("/account/cart", clone(errorsItem), errorsRule):             errorsRule + "?add=example%2Frules%7Crule%7Ctechs%2Fgo%2Freturn-errors",
		cartPath("/account/cart/remove", clone(errorsItem), "/account/cart"): "/account/cart",
		"/account/cart/empty": "/account/cart",
	} {
		resp := send(t, site.handler, request{method: http.MethodPost, target: target})
		want := "/sign-in?" + url.Values{"return": {back}, "to": {"cart"}}.Encode()
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != want {
			t.Errorf("%s: answered %d to %q, want %q", target, resp.StatusCode, resp.Header.Get("Location"), want)
		}
	}
	if got := site.cart.all(); len(got) != 0 {
		t.Fatalf("the cart holds %q", got)
	}
}

// A change to the cart returns only to a path on this site.
func TestCartChangesReturnOnlyToPathsOnThisSite(t *testing.T) {
	site := newCartSite(t)

	for _, back := range []string{"//evil.example", "https://evil.example/", `/\evil.example`} {
		resp := site.signedInPost(t, cartPath("/account/cart", clone(errorsItem), back))
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/" {
			t.Errorf("return %q: answered %d to %q, want home", back, resp.StatusCode, resp.Header.Get("Location"))
		}
	}
}

// Another site can't change a visitor's cart.
func TestAnotherSiteCantChangeAVisitorsCart(t *testing.T) {
	site := newCartSite(t)
	site.signedInPost(t, cartPath("/account/cart", clone(errorsItem), ""))

	for _, header := range []http.Header{
		{"Sec-Fetch-Site": {"cross-site"}},
		{"Sec-Fetch-Site": {"same-site"}},
		{"Sec-Fetch-Site": nil, "Origin": {"https://evil.example"}},
	} {
		for _, target := range []string{
			cartPath("/account/cart", clone(retryItem), ""), cartPath("/account/cart/remove", clone(errorsItem), ""), "/account/cart/empty",
		} {
			resp := send(t, site.handler, request{method: http.MethodPost, target: target, cookies: []*http.Cookie{site.session}, header: header})
			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("%s with %v: got %d, want 403", target, header, resp.StatusCode)
			}
		}
	}
	if got := site.cart.all(); !slices.Equal(got, []string{"1 example/rules rule techs/go/return-errors"}) {
		t.Fatalf("the cart holds %q", got)
	}
}

// Removing takes one item and returns to the cart; emptying takes them all, and says so.
func TestRemovingAndEmptyingTheCart(t *testing.T) {
	site := newCartSite(t)
	for _, item := range []url.Values{errorsItem, retryItem, goGroupItem} {
		site.signedInPost(t, cartPath("/account/cart", clone(item), ""))
	}

	resp := site.signedInPost(t, cartPath("/account/cart/remove", url.Values{"library": {"Example/Rules"}, "rule": {"Techs/Go/Return-Errors"}}, "/account/cart"))
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/account/cart" {
		t.Fatalf("removing answered %d to %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if got := site.cart.all(); !slices.Equal(got, []string{"1 example/rules group techs/go", "1 example/rules rule practices/testing/verify-retry-limits"}) {
		t.Fatalf("the cart holds %q", got)
	}
	if resp := site.signedInPost(t, "/account/cart/remove?library=example"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("removing what names no item answered %d, want 404", resp.StatusCode)
	}
	emptied := site.signedInPost(t, "/account/cart/empty")
	if emptied.StatusCode != http.StatusSeeOther || emptied.Header.Get("Location") != "/account/cart" {
		t.Fatalf("emptying answered %d to %q", emptied.StatusCode, emptied.Header.Get("Location"))
	}
	if got := site.cart.all(); len(got) != 0 {
		t.Fatalf("the cart holds %q after emptying", got)
	}
	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/account/cart", cookies: []*http.Cookie{site.session, cookie(emptied, noticeCookie)}}))
	assertShows(t, page, "You emptied your cart.", "There's nothing in your cart.")
	if !strings.Contains(page, `href="/libraries" autofocus`) {
		t.Error("the empty cart doesn't focus the way to browse")
	}
}

// exampleCart is a cart holding a group and a rule of example/rules, a rule its group covers, a retired rule, and an
// unvetted library's rule the visitor confirmed, and one they didn't.
func exampleCart() views.Cart {
	example := views.LibraryRef{Owner: "example", Name: "rules", OwnerAvatarURL: exampleRules.OwnerAvatarURL}
	stranger := views.LibraryRef{Owner: "stranger", Name: "rules"}
	item := func(lib views.LibraryRef, kind domain.CartItemKind, path string) domain.CartItem {
		return domain.CartItem{Owner: lib.Owner, Name: lib.Name, Kind: kind, Path: path}
	}
	goItem := item(example, domain.CartGroup, "techs/go")
	libraries := []views.CartLibrary{
		{Library: example, Vetted: true, LatestRelease: 3, Items: []views.CartItem{
			{Item: goItem, Group: "techs/go", CanonicalGroup: goGroup, Rules: 1, State: views.CartItemReady},
			{Item: item(example, domain.CartRule, "practices/testing/verify-retry-limits"), Group: "practices/testing",
				Title: "Verify retry limits", State: views.CartItemReady},
			{Item: item(example, domain.CartRule, "practices/testing/retry-forever"), Group: "practices/testing",
				Title: "Retry forever", RetiredIn: 2, State: views.CartItemRetired},
			{Item: item(example, domain.CartRule, "techs/go/return-errors"), Group: "techs/go", Title: "Return errors with context",
				State: views.CartItemCovered, CoveredBy: goItem},
		}},
		{Library: stranger, Listed: true, LatestRelease: 3, Items: []views.CartItem{
			{Item: item(stranger, domain.CartRule, "techs/go/return-errors"), Group: "techs/go", Title: "Return errors with context",
				Confirmed: true, State: views.CartItemReady},
			{Item: item(stranger, domain.CartRule, "practices/testing/verify-retry-limits"), Group: "practices/testing",
				Title: "Verify retry limits", State: views.CartItemUnconfirmed},
		}},
	}
	return views.Cart{Libraries: libraries, Checkout: domain.NewCheckout([]domain.CheckoutLibrary{
		{Owner: "example", Name: "rules", Release: 3, Vetted: true, Items: []domain.CartItem{goItem, item(example, domain.CartRule, "practices/testing/verify-retry-limits")}},
		{Owner: "stranger", Name: "rules", Release: 3, Items: []domain.CartItem{item(stranger, domain.CartRule, "techs/go/return-errors")}},
	})}
}

// The cart page lists each library's items, says why checkout leaves some out and what imports others already, and
// leads to checkout; others are asked to sign in and come back.
func TestTheCartPageListsTheVisitorsItems(t *testing.T) {
	site := newCartSite(t)
	site.cart.contents[octocatID] = exampleCart()

	signedOut := send(t, site.handler, request{method: http.MethodGet, target: "/account/cart"})
	if want := "/sign-in?return=%2Faccount%2Fcart"; signedOut.StatusCode != http.StatusSeeOther || signedOut.Header.Get("Location") != want {
		t.Fatalf("signed out: answered %d to %q, want %q", signedOut.StatusCode, signedOut.Header.Get("Location"), want)
	}
	assertShows(t, body(t, send(t, site.handler, request{method: http.MethodGet, target: "/sign-in?return=%2Faccount%2Fcart"})),
		"Sign in to see your cart.")

	resp := site.signedInGet(t, "/account/cart")
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("got %d, cached as %q", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}
	page := body(t, resp)
	assertShows(t, page,
		"example/rules release/3",
		"Go techs/go · 1 rule",
		"Verify retry limits practices/testing/verify-retry-limits",
		"Retry forever practices/testing/retry-forever Retired in release/2, so checkout leaves it out. See what replaced it",
		"Return errors with context techs/go/return-errors Included with its group, techs/go.",
		"stranger/rules Unvetted release/3",
		"Rulemart doesn't vet this library, so checkout leaves this out until you confirm it. Confirm",
		"6 items · 2 left out of checkout",
	)
	if content, _ := robots(t, page); content != "noindex" {
		t.Errorf("robots %q, want noindex", content)
	}
	if got := rels(t, page, unvettedLibrary); !slices.Equal(got, []string{"nofollow"}) {
		t.Errorf("the unvetted library's link has rel %q, want nofollow", got)
	}
	confirm := cartPath("/account/cart/confirm", url.Values{"library": {"stranger/rules"}, "rule": {"practices/testing/verify-retry-limits"}}, "/account/cart")
	if got := links(t, page, "Confirm"); !slices.Equal(got, []string{confirm}) {
		t.Errorf("Confirm leads to %q, want %q", got, confirm)
	}
	if got := links(t, page, "Check out"); !slices.Equal(got, []string{"/account/cart/checkout"}) {
		t.Errorf("Check out leads to %q", got)
	}
	remove := cartPath("/account/cart/remove", clone(retryItem), "/account/cart")
	if got := formActions(t, page); !slices.Contains(got, remove) || slices.Contains(got, "/account/cart/empty") {
		t.Errorf("the page's forms post to %q, want %q, and no emptying without confirming", got, remove)
	}
	if got := links(t, page, "Empty cart"); !slices.Equal(got, []string{"/account/cart/empty"}) {
		t.Errorf("Empty cart leads to %q", got)
	}
}

func TestTheCartPageSaysWhenItsEmpty(t *testing.T) {
	site := newCartSite(t)

	assertShows(t, body(t, site.signedInGet(t, "/account/cart")), "There's nothing in your cart. Add a library, a group, or a rule from its page.")
	assertShows(t, body(t, site.signedInGet(t, "/account/cart/checkout")), "Your cart is empty, so there's nothing to check out.")
}

// Checkout shows each library it imports, pinned to its release, the prompt with the unvetted library named, and the
// configuration, and says what it leaves out. The Copy buttons need its script.
func TestCheckoutShowsThePromptAndConfiguration(t *testing.T) {
	site := newCartSite(t)
	cart := exampleCart()
	site.cart.contents[octocatID] = cart

	resp := site.signedInGet(t, "/account/cart/checkout")
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("got %d, cached as %q", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}
	page := body(t, resp)
	assertShows(t, page,
		"Checkout leaves out 2 items in your cart. See why",
		"example/rules 1 group and 1 rule, pinned to release/3 Source example-rules",
		"stranger/rules Unvetted 1 rule, pinned to release/3 Source stranger-rules",
		"The prompt names each library Rulemart hasn't vetted",
	)
	if !strings.Contains(page, htmlEscape(cart.Checkout.Config())) {
		t.Errorf("the page doesn't show\n%s", cart.Checkout.Config())
	}
	// The prompt keeps each word with a hyphen, slash, or colon on one line, in spans that leave its text as it is.
	if text := strings.Join(strings.Fields(visibleText(t, page)), " "); !strings.Contains(text, strings.Join(strings.Fields(cart.Checkout.Prompt()), " ")) {
		t.Errorf("the page doesn't show the prompt\n%s", cart.Checkout.Prompt())
	}
	if !strings.Contains(page, `<span class="whitespace-nowrap">(https://code-rules.fabricahq.com).</span>`) {
		t.Error("the prompt may break inside its URLs")
	}
	if !strings.Contains(cart.Checkout.Prompt(), "Rulemart hasn't vetted this library") || !strings.Contains(cart.Checkout.Prompt(), "`stranger/rules`, source `stranger-rules`") {
		t.Errorf("the prompt doesn't name the unvetted library:\n%s", cart.Checkout.Prompt())
	}
	// The plain path is numbered, in the order Code Rules needs: create the configuration, put the sources in it, then
	// import and check.
	assertShows(t, page,
		"From your repository's root, create the configuration, unless .code-rules/config.yaml exists already: Copy code-rules project init",
		"In .code-rules/config.yaml , replace sources: {} with these sources.",
		"Import the rules, and check the generated files: Copy code-rules project sync code-rules project check",
		"Rulemart hasn't vetted stranger/rules . Before you add it, read techs/go/return-errors.md outside your repository:",
		"git -C \"$d\" fetch -q --depth 1 https://github.com/stranger/rules.git refs/tags/release/3",
		"If that source names another ref , decide before changing it",
	)
	for _, want := range []string{
		`data-copy="prompt" hidden`, `data-copy="config" hidden`, `data-copy="init-command" hidden`, `data-copy="sync-commands" hidden`,
		`role="status" data-copy-status="prompt"`, "/copy.js", "<ol",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	signedOut := send(t, site.handler, request{method: http.MethodGet, target: "/account/cart/checkout"})
	if want := "/sign-in?return=%2Faccount%2Fcart%2Fcheckout"; signedOut.Header.Get("Location") != want {
		t.Errorf("signed out: answered %d to %q, want %q", signedOut.StatusCode, signedOut.Header.Get("Location"), want)
	}
}

// htmlEscape returns text as templ writes it in an element.
func htmlEscape(text string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;").Replace(text)
}

// Checkout with nothing it can import says so, and leads back to the cart.
func TestCheckoutWithNothingToImportSaysSo(t *testing.T) {
	site := newCartSite(t)
	cart := exampleCart()
	cart.Libraries = cart.Libraries[1:]
	cart.Libraries[0].Items = cart.Libraries[0].Items[1:]
	cart.Checkout = domain.NewCheckout(nil)
	site.cart.contents[octocatID] = cart

	page := body(t, site.signedInGet(t, "/account/cart/checkout"))
	assertShows(t, page, "Checkout leaves out every item in your cart.", "Nothing in your cart can be checked out.")
	if strings.Contains(page, `id="prompt"`) {
		t.Error("the page shows a prompt")
	}
}

// The header leads a signed-in visitor to their cart, with its count, as their account menu does, and the account page
// says Rulemart keeps it, and that deleting the account removes it.
func TestTheHeaderAndAccountNameTheVisitorsCart(t *testing.T) {
	site := newCartSite(t)
	site.signedInPost(t, cartPath("/account/cart", clone(errorsItem), ""))
	site.signedInPost(t, cartPath("/account/cart", clone(retryItem), ""))

	page := body(t, site.signedInGet(t, "/"))
	if got := links(t, page, "Your cart"); !slices.Equal(got, []string{"/account/cart"}) {
		t.Errorf("the menu leads to %q", got)
	}
	for _, want := range []string{`href="/account/cart" aria-label="Your cart, 2 items"`, "Account menu, signed in as octocat"} {
		if !strings.Contains(page, want) {
			t.Errorf("the header lacks %q", want)
		}
	}
	assertShows(t, page, "Your cart 2 items")
	if strings.Contains(body(t, send(t, site.handler, request{method: http.MethodGet, target: "/"})), "/account/cart") {
		t.Error("a page for everyone links a cart")
	}
	assertShows(t, body(t, site.signedInGet(t, "/account")), "For your cart, it keeps what you add", "It removes your stars, your cart, and your listings")
	signedOut := site.signedInPost(t, "/sign-out?return=%2Faccount%2Fcart%2Fcheckout")
	if signedOut.Header.Get("Location") != "/" {
		t.Errorf("signing out of checkout returns to %q, want home", signedOut.Header.Get("Location"))
	}
}

// A failure to read the cart fails the page, rather than offering to add what the cart holds, or showing a wrong
// count; pages for visitors who aren't signed in read no cart.
func TestAFailedCartReadFailsThePage(t *testing.T) {
	site := newCartSite(t)
	site.cart.heldErr = errors.New("the database is down")

	if resp := site.signedInGet(t, "/"); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("a failed count: got %d, want 503", resp.StatusCode)
	}
	site.cart.heldErr, site.cart.err = nil, errors.New("the database is down")
	for _, path := range []string{library, errorsRule, "/account/cart", "/account/cart/checkout"} {
		if resp := site.signedInGet(t, path); resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("%s: got %d, want 503", path, resp.StatusCode)
		}
	}
	if resp := send(t, site.handler, request{method: http.MethodGet, target: library}); resp.StatusCode != http.StatusOK {
		t.Fatalf("a visitor who isn't signed in got %d", resp.StatusCode)
	}
}

// A retired rule's page offers nothing to add.
func TestARetiredRuleOffersNothingToAdd(t *testing.T) {
	site := newCartSite(t)
	retired := site.cart.catalog.rules["example/rules/techs/go/return-errors"]
	retired.Rule.Path, retired.Rule.Retirement = "techs/go/old-errors", &views.Retirement{Release: 3, RetiredAt: day(3), Summaries: []string{"Drop it."}}
	// The fake's maps are the site's, so the site finds the retired rule too.
	site.cart.catalog.rules["example/rules/techs/go/old-errors"] = retired
	page := body(t, site.signedInGet(t, library+"/techs/go/old-errors"))
	if strings.Contains(visibleText(t, page), "Add to cart") {
		t.Error("a retired rule's page offers to add it")
	}
}

// Without carts, or where no one can sign in, pages offer none, and the cart's addresses are missing.
func TestWithoutCartsPagesOfferNone(t *testing.T) {
	for name, adjust := range map[string]func(*web.Options){
		"without carts":   func(o *web.Options) { o.Cart = nil },
		"without sign-in": func(o *web.Options) { o.Accounts, o.GitHub = nil, nil },
	} {
		site := newCartSiteWith(t, adjust)
		if page := body(t, site.signedInGet(t, library)); strings.Contains(visibleText(t, page), "to cart") || strings.Contains(page, "/account/cart") {
			t.Errorf("%s: the library's page offers a cart", name)
		}
		if resp := site.signedInGet(t, "/account/cart"); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: the cart answered %d, want 404", name, resp.StatusCode)
		}
	}
}

// countOf counts the elements of s that equal v.
func countOf(s []string, v string) int {
	n := 0
	for _, e := range s {
		if e == v {
			n++
		}
	}
	return n
}
