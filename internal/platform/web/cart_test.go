package web_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
	"github.com/fabricahq/rulemart/internal/platform/web"
)

// fakeCarts answers every checkout with checkout, or err, and keeps the cart and target it was asked for.
type fakeCarts struct {
	checkout views.Checkout
	err      error
	cart     *app.Cart
	target   domain.CheckoutTarget
}

func (f *fakeCarts) Checkout(_ context.Context, cart app.Cart, target domain.CheckoutTarget) (views.Checkout, error) {
	f.cart, f.target = &cart, target
	return f.checkout, f.err
}

// postCheckout posts body to the checkout as cart-page.js does, from this site unless header says otherwise.
func postCheckout(t *testing.T, handler http.Handler, body string, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/cart/checkout.json", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	for name, values := range header {
		req.Header[name] = values
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

// checkoutAnswer is the part of a checkout's answer these tests read.
type checkoutAnswer struct {
	Libraries []struct {
		FullName, Href, Avatar  string
		Gone, Vetted, Confirmed bool
		Items                   []struct {
			Key, Kind, State, ID, Title, Href, Version, RetiredIn string
			Fork, InGroup                                         bool
			Group                                                 struct {
				ID, Name  string
				Canonical bool
				Icon      *struct{ Src string }
			}
			Rules     []struct{ Title, Href string }
			RuleCount int
		}
		RestOfGroups *struct {
			Groups []string
			Rules  int
			Added  bool
		}
	}
	Unknown           []string
	Repository        string
	RepositoryInvalid bool
	Prompt            string
	Commands          []struct{ Heading, Commands string }
	Pin               *struct{ Library, Release, Option string }
}

// decode reads resp's body as a checkout's answer, failing t unless it's JSON that no cache keeps.
func decode(t *testing.T, resp *httptest.ResponseRecorder) checkoutAnswer {
	t.Helper()
	if got := resp.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type %q, want JSON", got)
	}
	if got := resp.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Errorf("Cache-Control %q, want private, no-store", got)
	}
	var answer checkoutAnswer
	if err := json.Unmarshal(resp.Body.Bytes(), &answer); err != nil {
		t.Fatalf("decode %s: %v", resp.Body, err)
	}
	return answer
}

// oneStep returns the commands of answer, which imports only vetted libraries, so they're one step without a heading,
// failing t when they aren't.
func oneStep(t *testing.T, answer checkoutAnswer) string {
	t.Helper()
	if len(answer.Commands) != 1 || answer.Commands[0].Heading != "" {
		t.Fatalf("got the commands %q, want one step without a heading", answer.Commands)
	}
	return answer.Commands[0].Commands
}

// The checkout answers a cart with each library and item as the catalog resolved them, with their pages, and the texts,
// for the repository the visitor named, as cart-page.js shows them.
func TestCheckoutAnswersWithTheResolvedCart(t *testing.T) {
	goGroup := views.ResolvedGroup{Path: "techs/go", Canonical: &views.CanonicalGroup{Name: "Go", Icon: views.GroupIcon{File: "devicon/go-original.svg"}}}
	carts := &fakeCarts{checkout: views.Checkout{
		Libraries: []views.ResolvedLibrary{
			{
				Library: views.LibraryRef{Owner: "example", Name: "rules", OwnerAvatarURL: "https://avatars.githubusercontent.com/u/7"},
				Vetted:  true, LatestRelease: 6, RestOfGroups: []views.ResolvedGroup{goGroup}, RestOfGroupsRules: 3,
				Items: []views.ResolvedItem{
					{
						Key: "example/rules::techs/go/return-errors", State: views.CartItemReady, Group: goGroup, Title: "Return errors",
						Item:    domain.CartItem{Owner: "example", Name: "rules", Kind: domain.CartRule, Path: "techs/go/return-errors"},
						Version: coderules.RuleVersion{Major: 1, Minor: 2}, Fork: true, InGroup: true,
					},
					{
						Key: "group::example/rules::techs/golang", State: views.CartItemReady, Group: views.ResolvedGroup{Path: "techs/golang"},
						Item:  domain.CartItem{Owner: "example", Name: "rules", Kind: domain.CartGroup, Path: "techs/golang"},
						Rules: []views.CartRule{{Path: "techs/golang/pass-context", Title: "Pass context first"}},
					},
					{
						Key: "example/rules::techs/go/old", State: views.CartItemRetired, Group: goGroup, RetiredIn: 4, Title: "Old",
						Item: domain.CartItem{Owner: "example", Name: "rules", Kind: domain.CartRule, Path: "techs/go/old"},
					},
				},
			},
			{
				Library: views.LibraryRef{Owner: "gone", Name: "rules"}, Gone: true,
				Items: []views.ResolvedItem{{
					Key: "gone/rules::techs/go/x", State: views.CartItemGone, Group: goGroup,
					Item: domain.CartItem{Owner: "gone", Name: "rules", Kind: domain.CartRule, Path: "techs/go/x"},
				}},
			},
		},
		Unknown: []string{"???"}, Prompt: "the prompt",
		Commands:   []domain.CommandStep{{Heading: "Fetch and review", Commands: "the review"}, {Heading: "After you've reviewed, import", Commands: "the import"}},
		PinExample: &domain.ReleasePin{Library: "example/rules", Release: 6},
	}}
	handler := newSiteWith(t, web.Options{Carts: carts})

	resp := postCheckout(t, handler, `{"cart":["example/rules::techs/go/return-errors"],"fork":{"example/rules::techs/go/return-errors":true},
		"restOfGroups":{"example/rules":true},"confirmed":{"stranger/rules":true},"repo":"https://github.com/acme/api/tree/main"}`, nil)

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d: %s", resp.Code, resp.Body)
	}
	answer := decode(t, resp)
	if carts.cart == nil || !slices.Equal(carts.cart.Keys, []string{"example/rules::techs/go/return-errors"}) ||
		!carts.cart.Forks["example/rules::techs/go/return-errors"] || !carts.cart.RestOfGroups["example/rules"] ||
		!carts.cart.Confirmed["stranger/rules"] {
		t.Errorf("checked out %+v, want the cart posted", carts.cart)
	}
	if carts.target.Mode != domain.ProjectUnknown || carts.target.Repository != "acme/api" || answer.Repository != "acme/api" {
		t.Errorf("target %+v, answer's repository %q, want acme/api, a project Rulemart doesn't know", carts.target, answer.Repository)
	}
	lib := answer.Libraries[0]
	if lib.FullName != "example/rules" || lib.Href != "/example/rules" || !lib.Vetted ||
		lib.RestOfGroups == nil || !slices.Equal(lib.RestOfGroups.Groups, []string{"Go"}) || lib.RestOfGroups.Rules != 3 {
		t.Errorf("got the library %+v", lib)
	}
	rule, group, retired := lib.Items[0], lib.Items[1], lib.Items[2]
	if rule.Kind != "rule" || rule.State != "ready" || rule.Title != "Return errors" || rule.Version != "1.2.0" || !rule.Fork || !rule.InGroup ||
		rule.Href != "/example/rules/techs/go/return-errors" || rule.Group.Name != "Go" || rule.Group.Icon == nil ||
		!strings.Contains(rule.Group.Icon.Src, "go-original") {
		t.Errorf("got the rule %+v", rule)
	}
	if group.Kind != "group" || group.Title != "techs/golang" || group.Group.Canonical || group.Group.Icon != nil ||
		group.Href != "/example/rules/techs/golang" || len(group.Rules) != 1 || group.RuleCount != 1 ||
		group.Rules[0].Href != "/example/rules/techs/golang/pass-context" {
		t.Errorf("got the group %+v", group)
	}
	if retired.State != "retired" || retired.RetiredIn != "release/4" || retired.Href != "/example/rules/techs/go/old" {
		t.Errorf("got the retired rule %+v", retired)
	}
	gone := answer.Libraries[1]
	if !gone.Gone || gone.Href != "" || gone.Items[0].State != "gone" || gone.Items[0].Href != "" {
		t.Errorf("got the gone library %+v, want no page", gone)
	}
	steps := []struct{ Heading, Commands string }{{"Fetch and review", "the review"}, {"After you've reviewed, import", "the import"}}
	if !slices.Equal(answer.Unknown, []string{"???"}) || answer.Prompt != "the prompt" || !slices.Equal(answer.Commands, steps) {
		t.Errorf("got unknown %q, prompt %q, commands %q", answer.Unknown, answer.Prompt, answer.Commands)
	}
	if answer.Pin == nil || *answer.Pin != (struct{ Library, Release, Option string }{"example/rules", "release/6", "--ref release/6"}) {
		t.Errorf("got the pin %+v, want example/rules at release/6", answer.Pin)
	}
}

// A checkout's answer must fit a Lambda function's response, so one that wouldn't is refused, before anything of it is
// written, with the failure cart-page.js shows as Rulemart being unable to show the cart.
func TestCheckoutRefusesAnAnswerTooLargeToSend(t *testing.T) {
	carts := &fakeCarts{checkout: views.Checkout{Prompt: strings.Repeat("x", 6<<20), Commands: []domain.CommandStep{{Commands: "the commands"}}}}
	handler := newSiteWith(t, web.Options{Carts: carts})

	resp := postCheckout(t, handler, `{"cart":["example/rules::techs/go/return-errors"]}`, nil)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503", resp.Code)
	}
	if resp.Body.Len() > 1<<10 || !strings.Contains(resp.Body.String(), `"error"`) || strings.Contains(resp.Body.String(), "the commands") {
		t.Errorf("answered %d bytes, starting %.200q, want only the error", resp.Body.Len(), resp.Body)
	}
}

// A repository the visitor wrote that names none is reported, and the texts name no project. With nothing to import,
// the commands are an empty list, which cart-page.js reads as no steps.
func TestCheckoutReportsARepositoryThatNamesNone(t *testing.T) {
	carts := &fakeCarts{}
	handler := newSiteWith(t, web.Options{Carts: carts})

	resp := postCheckout(t, handler, `{"cart":[],"repo":"https://gitlab.com/acme/api"}`, nil)
	answer := decode(t, resp)

	if !strings.Contains(resp.Body.String(), `"commands":[]`) {
		t.Errorf("answered %s, want the commands an empty list", resp.Body)
	}
	if !answer.RepositoryInvalid || answer.Repository != "" || carts.target.Repository != "" {
		t.Errorf("got invalid %t, repository %q, target %+v, want invalid and no project named", answer.RepositoryInvalid, answer.Repository, carts.target)
	}
}

// Another site can't make a visitor's browser post a cart, and what isn't a cart, or holds too many items, is refused
// before or without checking it out.
func TestCheckoutRefusesWhatIsntAVisitorsCart(t *testing.T) {
	for _, c := range []struct {
		name, body string
		header     http.Header
		err        error
		want       int
	}{
		{"another site's", `{"cart":[]}`, http.Header{"Sec-Fetch-Site": {"cross-site"}}, nil, http.StatusForbidden},
		{"form-encoded", `cart=x`, http.Header{"Content-Type": {"application/x-www-form-urlencoded"}}, nil, http.StatusUnsupportedMediaType},
		{"not JSON", `{"cart":`, nil, nil, http.StatusBadRequest},
		{"the wrong shape", `{"cart":"one"}`, nil, nil, http.StatusBadRequest},
		{"too long", `{"cart":["` + strings.Repeat("a", 4*domain.MaxCartItems*domain.MaxCartKeyLength) + `"]}`, nil, nil, http.StatusBadRequest},
		{"too many items", `{"cart":[]}`, nil, app.ErrCartTooLarge, http.StatusBadRequest},
		{"unreadable", `{"cart":[]}`, nil, errors.New("connection refused"), http.StatusServiceUnavailable},
	} {
		t.Run(c.name, func(t *testing.T) {
			carts := &fakeCarts{err: c.err}
			handler := newSiteWith(t, web.Options{Carts: carts})

			resp := postCheckout(t, handler, c.body, c.header)

			if resp.Code != c.want {
				t.Errorf("got %d, want %d", resp.Code, c.want)
			}
			if c.err == nil && carts.cart != nil {
				t.Errorf("checked out %+v", carts.cart)
			}
		})
	}
}
