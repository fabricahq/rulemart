package web_test

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// parsePage parses an HTML body, failing t if it can't.
func parsePage(t *testing.T, body string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// withAttribute returns a matcher of the elements with the attribute key.
func withAttribute(key string) func(*html.Node) bool {
	return func(n *html.Node) bool { return hasAttribute(n, key) }
}

// loadsScript reports whether doc loads the static script named file.
func loadsScript(doc *html.Node, file string) bool {
	return find(doc, func(n *html.Node) bool { return n.Data == "script" && strings.HasSuffix(attribute(n, "src"), "/"+file) }) != nil
}

// Every page's header links the cart for everyone, signed in or not, telling the script that paints its count and
// keeps the cart how much a cart holds, and loads that script and the one that shows its toasts, but not the cart
// page's own scripts.
func TestEveryPageLinksTheCartAndLoadsItsScript(t *testing.T) {
	handler := newSite(t, newCatalog())

	for _, path := range []string{"/", library, errorsRule, "/missing/page/here"} {
		doc := parsePage(t, get(t, handler, path).Body.String())

		link := find(doc, withAttribute("data-cart-link"))
		if link == nil || attribute(link, "href") != "/cart" || find(link, withAttribute("data-cart-count")) == nil {
			t.Fatalf("%s: no link to the cart with its count", path)
		}
		if hasAttribute(link, "title") {
			t.Errorf("%s: the cart link's title repeats its name as its description", path)
		}
		if attribute(link, "data-cart-max-items") != strconv.Itoa(domain.MaxCartItems) ||
			attribute(link, "data-cart-max-key-length") != strconv.Itoa(domain.MaxCartKeyLength) {
			t.Errorf("%s: the cart link doesn't say how much a cart holds", path)
		}
		for _, script := range []string{"cart.js", "toast.js"} {
			if !loadsScript(doc, script) {
				t.Errorf("%s: doesn't load %s", path, script)
			}
		}
		for _, script := range []string{"cart-page.js", "cart-checkout.js"} {
			if loadsScript(doc, script) {
				t.Errorf("%s: loads the cart page's %s", path, script)
			}
		}
	}
}

// A current rule's page offers to add the rule or its whole group, by the keys the browser's cart keeps, in the
// prototype's dialog, which names the rule, its group, and the group's other rules.
func TestARulesPageOffersTheRuleOrItsGroup(t *testing.T) {
	c := newCatalog()
	page := c.rules["example/rules/techs/go/return-errors"]
	page.Links = append(page.Links, linkOf("techs/go/return-errors", 0), linkOf("techs/go/close-bodies", 0), linkOf("techs/go/gone", 2),
		linkOf("practices/testing/verify-retry-limits", 0))
	c.rules["example/rules/techs/go/return-errors"] = page
	handler := newSite(t, c)

	body := get(t, handler, errorsRule).Body.String()

	control := find(parsePage(t, body), withAttribute("data-cart-control"))
	if control == nil {
		t.Fatal("no cart control")
	}
	for key, want := range map[string]string{
		"data-cart-rule": "example/rules::techs/go/return-errors", "data-cart-group": "group::example/rules::techs/go",
		"data-cart-library": "example/rules", "data-cart-vetted": "true", "data-cart-group-name": "Go",
	} {
		if got := attribute(control, key); got != want {
			t.Errorf("%s is %q, want %q", key, got, want)
		}
	}
	assertShows(t, body, "Add to cart", "What would you like to add?",
		"Just this rule Adds only “Return errors with context.” It stays in sync with example/rules, and nothing else from the group is added.",
		"The whole Go group Adds this rule and the 1 other Go rule from example/rules.",
		"At checkout, you'll pick which project these go into, and you can fork a rule instead of staying in sync.")
	if strings.Contains(visibleText(t, body), unvettedWarning+" Anyone signed in can list a library on Rulemart, and Rulemart hasn't reviewed example/rules") {
		t.Error("a vetted library's dialog warns that it isn't vetted")
	}
}

// An unvetted library's rule page warns in its dialog first, and asks the visitor to confirm adding from it.
func TestAnUnvettedRulesDialogWarnsFirst(t *testing.T) {
	handler := newSite(t, unvettedCatalog())

	body := get(t, handler, unvettedLibrary+"/techs/go/return-errors").Body.String()

	doc := parsePage(t, body)
	control := find(doc, withAttribute("data-cart-control"))
	if control == nil || attribute(control, "data-cart-vetted") != "false" {
		t.Fatal("no cart control that says the library isn't vetted")
	}
	confirm := find(doc, func(n *html.Node) bool { return attribute(n, "data-cart-step") == "confirm" })
	if confirm == nil || find(confirm, withAttribute("data-cart-confirm")) == nil {
		t.Fatal("the dialog doesn't ask to confirm first")
	}
	assertShows(t, body, "Rulemart hasn't reviewed stranger/rules. At checkout, the prompt asks your agent to review its rules before following them. Add from it anyway Cancel")
	// The choices, which show once the visitor confirms, say checkout pins the library rather than keeping it in sync,
	// and that it can't be forked from yet.
	choices := strings.Join(strings.Fields(body), " ")
	for _, want := range []string{
		"It's pinned to the commit you review, and nothing else from the group is added.",
		"from stranger/rules. It's pinned to the commit you review, so rules the library adds later don't arrive.",
		"At checkout, you'll pick which project these go into. Forking waits until the rules are vetted, so you get exactly the reviewed text.</p>",
	} {
		if !strings.Contains(choices, want) {
			t.Errorf("the dialog's choices lack %q", want)
		}
	}
	if strings.Contains(choices, "stays in sync") || strings.Contains(choices, "arrive when you update") {
		t.Error("an unvetted library's dialog says its rules stay in sync, though checkout pins them")
	}
	if strings.Contains(choices, "you can fork") {
		t.Error("an unvetted library's dialog offers to fork, though checkout keeps its rules at the reviewed commit")
	}
}

// Without JavaScript the cart can't work, so its controls hide, and a rule's page and a library group's page say why
// there's nothing to add with.
func TestPagesThatAddSayTheCartNeedsJavaScript(t *testing.T) {
	handler := newSite(t, newCatalog())

	for _, path := range []string{errorsRule, library + "/techs/go"} {
		doc := parsePage(t, get(t, handler, path).Body.String())

		noscript := find(doc, func(n *html.Node) bool {
			return n.Data == "noscript" && n.FirstChild != nil &&
				strings.Contains(n.FirstChild.Data, "Your cart lives in your browser, so adding to it needs JavaScript.")
		})
		if noscript == nil {
			t.Errorf("%s: without JavaScript, the page doesn't say why it can't add to the cart", path)
		}
	}
}

// A retired rule can't be added: its page has no cart control.
func TestARetiredRuleOffersNothingToAdd(t *testing.T) {
	handler := newSite(t, newStarCatalog())

	body := get(t, handler, retiredRule).Body.String()

	if find(parsePage(t, body), withAttribute("data-cart-control")) != nil {
		t.Error("a retired rule's page offers to add it")
	}
}

// A library's Groups tab picks groups for the Add to cart box beside them, each by its key, naming the library whose
// box takes it, and leads to each group's page; the other tabs have no box.
func TestALibrarysGroupsOfferThemToTheCart(t *testing.T) {
	handler := newSite(t, newCatalog())

	doc := parsePage(t, get(t, handler, library).Body.String())

	panel := find(doc, withAttribute("data-cart-groups"))
	if panel == nil {
		t.Fatal("no Add to cart box")
	}
	for _, key := range []string{"group::example/rules::practices/testing", "group::example/rules::techs/go"} {
		pick := find(doc, func(n *html.Node) bool { return attribute(n, "data-cart-pick-group") == key })
		if pick == nil || attribute(pick, "data-cart-library") != attribute(panel, "data-cart-library") {
			t.Errorf("no checkbox picks %s for the box of %s", key, attribute(panel, "data-cart-library"))
		}
	}
	if find(doc, func(n *html.Node) bool { return n.Data == "a" && attribute(n, "href") == library+"/techs/go" }) == nil {
		t.Error("no link to the Go group's page")
	}
	if find(parsePage(t, get(t, handler, library+"?tab=rules").Body.String()), withAttribute("data-cart-groups")) != nil {
		t.Error("the All rules tab has the Add to cart box")
	}
}

// A library's group has a page of its own, with its rules and the Whole group box that adds it; another spelling of
// its library or ID redirects there, and a group the library doesn't have is missing, as is a group's ID in one
// segment, whose slash is percent-encoded, which no link writes.
func TestALibrarysGroupHasAPageThatAddsIt(t *testing.T) {
	handler := newSite(t, newCatalog())

	resp := get(t, handler, library+"/techs/go")

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d", resp.Code)
	}
	body := resp.Body.String()
	assertShows(t, body, "rules › techs/go", "Go", "1 rule in example/rules", "Return errors with context",
		"← Back to all groups in example/rules",
		"Whole group Adds all 1 Go rule from example/rules.", "Add Go group to cart", "See Go rules from every library →")
	if text := visibleText(t, body); strings.Contains(text, "The Go language.") {
		t.Error("a technology's page shows its blurb, though its name says what it is")
	}
	control := find(parsePage(t, body), withAttribute("data-cart-control"))
	if control == nil || attribute(control, "data-cart-group") != "group::example/rules::techs/go" || hasAttribute(control, "data-cart-rule") {
		t.Errorf("got the control %+v, want one that adds the group", control)
	}
	for _, spelling := range []string{library + "/Techs/Go", "/Example/Rules/techs/go"} {
		if resp := get(t, handler, spelling); resp.Code != http.StatusMovedPermanently || resp.Header().Get("Location") != library+"/techs/go" {
			t.Errorf("%s: got %d to %q", spelling, resp.Code, resp.Header().Get("Location"))
		}
	}
	for _, missing := range []string{library + "/techs/rust", library + "/techs%2Fgo", "/example/missing/techs/go"} {
		if resp := get(t, handler, missing); resp.Code != http.StatusNotFound {
			t.Errorf("%s: got %d", missing, resp.Code)
		}
	}
}

// A practice's page in a library says what belongs in it, and its links back to the library's groups carry the groups
// the Groups tab ticked; a group that isn't canonical is flagged, and isn't offered across libraries from here.
func TestALibrarysGroupPageLeadsBackWithTheTickedGroups(t *testing.T) {
	practice := get(t, newSite(t, newCatalog()), library+"/practices/testing?sel=techs/go").Body.String()
	golang := get(t, newSite(t, newMixedCatalog()), mixed+"/techs/golang").Body.String()

	assertShows(t, practice, "1 rule in example/rules · What to test and how.")
	if got := links(t, practice, "rules"); !slices.Contains(got, library+"?sel=techs/go") || slices.Contains(got, library) {
		t.Errorf("the page leads back to %q, want the ticked groups", got)
	}
	if got := links(t, practice, "Back to all groups"); !slices.Equal(got, []string{library + "?sel=techs/go"}) {
		t.Errorf("Back leads to %q", got)
	}
	assertShows(t, golang, "techs/golang not canonical 1 rule in example/mixed")
	if strings.Contains(visibleText(t, golang), "from every library") {
		t.Error("a group that isn't canonical is offered across libraries")
	}
}

// linkOf returns how the rule at path relates to its replacement: current when retiredIn is 0.
func linkOf(path string, retiredIn int) views.RuleLink {
	return views.RuleLink{Path: path, RetiredIn: retiredIn}
}

// The signed-in cart's old addresses, its page and its checkout, redirect permanently to the cart's page, keeping the
// query, whether sign-in is available or not.
func TestOldCartAddressesRedirectToTheCart(t *testing.T) {
	for name, handler := range map[string]http.Handler{
		"without sign-in": newSite(t, newCatalog()),
		"with sign-in":    newAccountsSite(t, nil).handler,
	} {
		for path, location := range map[string]string{
			"/account/cart":          "/cart",
			"/account/cart/checkout": "/cart",
			"/account/cart?ref=x":    "/cart?ref=x",
			"/Account/cart":          "/account/cart",
		} {
			resp := get(t, handler, path)
			if resp.Code != http.StatusMovedPermanently || resp.Header().Get("Location") != location {
				t.Errorf("%s: %s: got %d to %q, want 301 to %q", name, path, resp.Code, resp.Header().Get("Location"), location)
			}
		}
	}
}

// The cart's page is a shell cart-page.js fills, the same for every visitor who isn't signed in: the empty state and
// the three cards, each hidden until the script shows the one the cart needs, and, without JavaScript, a message that
// the cart needs it. Signed out, where the rules go offers to sign in or to enter a repository; signed in, it says
// Rulemart found no projects, since it reads none yet. Search engines don't index it.
func TestTheCartsPageIsAShellForTheScript(t *testing.T) {
	site := newAccountsSite(t, nil)
	token := site.accounts.signedIn(t, octocat)

	signedOut := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/cart"}))
	signedIn := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/cart", cookies: []*http.Cookie{{Name: sessionCookie, Value: string(token)}}}))

	doc := parsePage(t, signedOut)
	for _, script := range []string{"cart-page.js", "cart-checkout.js"} {
		if !loadsScript(doc, script) {
			t.Errorf("the page doesn't load %s, which fills it", script)
		}
	}
	if page := find(doc, withAttribute("data-cart-page")); page == nil || attribute(page, "data-cart-checkout") != "/cart/checkout.json" {
		t.Error("the page doesn't name where it checks out")
	}
	for _, part := range []string{"data-cart-page", "data-cart-empty", "data-cart-full", "data-cart-libraries", "data-cart-preview", "data-cart-status", "data-cart-repo"} {
		if find(doc, withAttribute(part)) == nil {
			t.Errorf("the page has no %s", part)
		}
	}
	if !strings.Contains(signedOut, `<meta name="robots" content="noindex">`) {
		t.Error("search engines may index the cart")
	}
	if link := find(doc, withAttribute("data-cart-link")); link == nil || attribute(link, "aria-current") != "page" {
		t.Error("the header's cart isn't marked as the current page")
	}
	noscript := find(doc, func(n *html.Node) bool {
		return n.Data == "noscript" && strings.Contains(n.FirstChild.Data, "Your cart needs JavaScript")
	})
	if noscript == nil {
		t.Error("without JavaScript, the page doesn't say the cart needs it")
	}
	page := strings.Join(strings.Fields(signedOut), " ")
	for _, want := range []string{
		"Your cart is empty", "Checkout", "What you&#39;re adding", "Where it goes", "Finish checkout",
		"Pick from your projects", "Sign in to choose a project and see when its rules have updates.", "Enter your project",
		"GitHub repository <span class=\"text-faint\">(optional, so the prompt names it)</span>", "Copy prompt for agent",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	signedInText := strings.Join(strings.Fields(signedIn), " ")
	if strings.Contains(signedInText, "Pick from your projects") || !strings.Contains(signedInText, "We didn't find any of your projects using Code Rules: repositories with <code>.code-rules/generated/provenance.json</code>. Enter the project below, and the prompt names it.</p><b class=\"mb-2.5 block text-[13px] font-semibold\">Enter your project</b>") {
		t.Error("signed in, the page offers to sign in, or doesn't say it found no projects, what counts as one, and to enter it")
	}
}
