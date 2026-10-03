package web_test

import (
	"net/http"
	"strings"
	"testing"

	"golang.org/x/net/html"

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

// Every page's header links the cart for everyone, signed in or not, and loads the script that paints its count and
// keeps the cart, and the one that shows its toasts.
func TestEveryPageLinksTheCartAndLoadsItsScript(t *testing.T) {
	handler := newSite(t, newCatalog())

	for _, path := range []string{"/", library, errorsRule, "/missing/page/here"} {
		doc := parsePage(t, get(t, handler, path).Body.String())

		link := find(doc, withAttribute("data-cart-link"))
		if link == nil || attribute(link, "href") != "/cart" || find(link, withAttribute("data-cart-count")) == nil {
			t.Errorf("%s: no link to the cart with its count", path)
		}
		for _, script := range []string{"cart.js", "toast.js"} {
			if find(doc, func(n *html.Node) bool { return n.Data == "script" && strings.Contains(attribute(n, "src"), script) }) == nil {
				t.Errorf("%s: doesn't load %s", path, script)
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
}

// A retired rule can't be added: its page has no cart control.
func TestARetiredRuleOffersNothingToAdd(t *testing.T) {
	handler := newSite(t, newStarCatalog())

	body := get(t, handler, retiredRule).Body.String()

	if find(parsePage(t, body), withAttribute("data-cart-control")) != nil {
		t.Error("a retired rule's page offers to add it")
	}
}

// A library's Groups tab picks groups for the Add to cart box beside them, each by its key, and leads to each
// group's page; the other tabs have no box.
func TestALibrarysGroupsOfferThemToTheCart(t *testing.T) {
	handler := newSite(t, newCatalog())

	doc := parsePage(t, get(t, handler, library).Body.String())

	if find(doc, withAttribute("data-cart-groups")) == nil {
		t.Fatal("no Add to cart box")
	}
	for _, key := range []string{"group::example/rules::practices/testing", "group::example/rules::techs/go"} {
		if find(doc, func(n *html.Node) bool { return attribute(n, "data-cart-pick-group") == key }) == nil {
			t.Errorf("no checkbox picks %s", key)
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
// its ID redirects there, and a group the library doesn't have is missing.
func TestALibrarysGroupHasAPageThatAddsIt(t *testing.T) {
	handler := newSite(t, newCatalog())

	resp := get(t, handler, library+"/techs/go")

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d", resp.Code)
	}
	body := resp.Body.String()
	assertShows(t, body, "example/rules › techs/go", "Go", "1 rule in example/rules", "Return errors with context",
		"Whole group Adds all 1 Go rule from example/rules.", "Add Go group to cart", "See Go rules from every library →")
	control := find(parsePage(t, body), withAttribute("data-cart-control"))
	if control == nil || attribute(control, "data-cart-group") != "group::example/rules::techs/go" || hasAttribute(control, "data-cart-rule") {
		t.Errorf("got the control %+v, want one that adds the group", control)
	}
	if resp := get(t, handler, library+"/Techs/Go"); resp.Code != http.StatusMovedPermanently || resp.Header().Get("Location") != library+"/techs/go" {
		t.Errorf("another spelling: got %d to %q", resp.Code, resp.Header().Get("Location"))
	}
	if resp := get(t, handler, library+"/techs/rust"); resp.Code != http.StatusNotFound {
		t.Errorf("a group the library doesn't have: got %d", resp.Code)
	}
}

// linkOf returns how the rule at path relates to its replacement: current when retiredIn is 0.
func linkOf(path string, retiredIn int) views.RuleLink {
	return views.RuleLink{Path: path, RetiredIn: retiredIn}
}
