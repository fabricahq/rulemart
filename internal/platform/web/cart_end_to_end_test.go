package web_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"

	shipped "github.com/fabricahq/rulemart/catalog"
	accountsapp "github.com/fabricahq/rulemart/internal/contexts/accounts/app"
	accountspostgres "github.com/fabricahq/rulemart/internal/contexts/accounts/store/postgres"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/render"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git/gittest"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres"
	"github.com/fabricahq/rulemart/internal/platform/database/databasetest"
	"github.com/fabricahq/rulemart/internal/platform/postgrestest"
	"github.com/fabricahq/rulemart/internal/platform/web"
)

// A visitor signs in and fills a cart from a library's pages, as the web function's role, so a missing grant fails
// this test: a group, a rule the group imports already, and another rule. The pages say what the cart holds, checkout
// imports the group and the other rule pinned to the library's release, another visitor's cart stays empty, and
// deleting the account empties its cart.
func TestAVisitorFillsACartAndChecksItOut(t *testing.T) {
	lib := gittest.NewLibrary(t)
	lib.Group("techs/go", "Go")
	lib.Group("practices/testing", "Testing")
	lib.Rule("techs/go/return-errors", "Return errors", "Wrap every returned error.")
	lib.Rule("techs/go/close-bodies", "Close bodies", "Close every response body.")
	lib.Rule("practices/testing/verify-retries", "Verify retries", "Test each retry.")
	lib.Release(1, `formatVersion: 1
release: 1
rules: {techs/go/return-errors: 1.0.0, techs/go/close-bodies: 1.0.0, practices/testing/verify-retries: 1.0.0}
changes:
  techs/go/return-errors: {change: new, summaries: [Add the rule.]}
  techs/go/close-bodies: {change: new, summaries: [Add the rule.]}
  practices/testing/verify-retries: {change: new, summaries: [Add the rule.]}
`)
	db, connString := databasetest.New(t)
	repo := lib.Repository(7)
	ingester := app.Ingester{Repositories: repositories{repo}, Fetch: git.Fetch, Render: render.Rule, Store: postgres.New(db), Limits: domain.DefaultLimits}
	if _, err := ingester.Ingest(context.Background(), "https://github.com/"+repo.FullName()); err != nil {
		t.Fatal(err)
	}
	groups, err := shipped.CanonicalGroups()
	if err != nil {
		t.Fatal(err)
	}
	vetted := []domain.LibraryKey{{Host: repo.Host, RepositoryID: repo.ID}}
	webStore := postgres.New(databasetest.AsWebRole(t, connString))
	gitHub := &fakeGitHub{}
	handler, err := web.New(app.Pages{Store: webStore, Vetted: vetted, Groups: groups}, web.Options{
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Accounts: accountsapp.Sessions{Store: accountspostgres.New(databasetest.AsWebRole(t, connString))},
		GitHub:   gitHub,
		Cart:     app.Cart{Store: webStore, Vetted: vetted, Groups: groups},
	})
	if err != nil {
		t.Fatal(err)
	}
	site := accountsSite{handler: handler, gitHub: gitHub}
	signIn := func(user int64, login string) []*http.Cookie {
		t.Helper()
		gitHub.identity.GitHubUserID, gitHub.identity.Login = user, login
		flow, location := startSignIn(t, site, library)
		return []*http.Cookie{cookie(callback(t, site, url.Values{"code": {authorizedCode}, "state": {location.Query().Get("state")}}, flow), sessionCookie)}
	}
	first, second := signIn(1, "first"), signIn(2, "second")
	post := func(target string) {
		t.Helper()
		if resp := send(t, handler, request{method: http.MethodPost, target: target, cookies: first}); resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("%s answered %d", target, resp.StatusCode)
		}
	}

	post(cartPath("/account/cart", clone(goGroupItem), library))
	post(cartPath("/account/cart", clone(errorsItem), errorsRule))
	post(cartPath("/account/cart", url.Values{"library": {"Example/Rules"}, "rule": {"Practices/Testing/Verify-Retries"}}, ""))

	assertShows(t, body(t, send(t, handler, request{method: http.MethodGet, target: errorsRule, cookies: first})), "In your cart. See your cart")
	assertShows(t, body(t, send(t, handler, request{method: http.MethodGet, target: "/account/cart", cookies: first})),
		"Go techs/go · 2 rules", "Return errors techs/go/return-errors Included with its group, techs/go.",
		"Verify retries practices/testing/verify-retries", "3 items")
	checkout := body(t, send(t, handler, request{method: http.MethodGet, target: "/account/cart/checkout", cookies: first}))
	assertShows(t, checkout, "example/rules 1 group and 1 rule, pinned to release/1 Source rules")
	config := "sources:\n  rules:\n    repository: https://github.com/example/rules.git\n    groups:\n      - techs/go\n" +
		"    rules:\n      - practices/testing/verify-retries\n    ref: release/1\n"
	if !strings.Contains(checkout, config) {
		t.Errorf("checkout lacks the configuration\n%s", config)
	}
	assertShows(t, body(t, send(t, handler, request{method: http.MethodGet, target: "/account/cart", cookies: second})), "Your cart is empty.")

	if resp := send(t, handler, request{method: http.MethodPost, target: "/account/delete", cookies: first}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("deleting the account answered %d", resp.StatusCode)
	}
	var left int
	postgrestest.QueryRow(t, connString, "SELECT count(*) FROM cart_items", &left)
	if left != 0 {
		t.Errorf("%d cart items outlive their account", left)
	}
}
