package web_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
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
	"github.com/fabricahq/rulemart/internal/platform/web"
)

// Two visitors sign in and star a rule of a vetted library, as the web function's role, so a missing grant fails this
// test. Pages count both stars for everyone, on the rule's page and wherever it's listed, each visitor's Starred rules
// list only theirs, and deleting one's account takes its star away.
func TestVisitorsStarARuleAndDeletingAnAccountRemovesItsStar(t *testing.T) {
	lib := gittest.NewLibrary(t)
	lib.Group("techs/go", "Go")
	lib.Rule("techs/go/return-errors", "Return errors", "Wrap every returned error.")
	lib.Release(1, `formatVersion: 1
release: 1
rules: {techs/go/return-errors: 1.0.0}
changes: {techs/go/return-errors: {change: new, summaries: [Add the rule.]}}
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
		Stars:    app.Stars{Store: webStore, Vetted: vetted, Groups: groups},
	})
	if err != nil {
		t.Fatal(err)
	}
	site := accountsSite{handler: handler, gitHub: gitHub}
	signIn := func(user int64, login string) []*http.Cookie {
		t.Helper()
		gitHub.identity.GitHubUserID, gitHub.identity.Login = user, login
		flow, location := startSignIn(t, site, errorsRule)
		return []*http.Cookie{cookie(callback(t, site, url.Values{"code": {authorizedCode}, "state": {location.Query().Get("state")}}, flow), sessionCookie)}
	}
	first, second := signIn(1, "first"), signIn(2, "second")
	get := func(path string, cookies []*http.Cookie) string {
		t.Helper()
		return body(t, send(t, handler, request{method: http.MethodGet, target: path, cookies: cookies}))
	}

	for _, cookies := range [][]*http.Cookie{first, second} {
		if resp := send(t, handler, request{method: http.MethodPost, target: starPath, cookies: cookies}); resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("starring answered %d", resp.StatusCode)
		}
	}
	if got := accessibleNames(t, get(errorsRule, nil), "/sign-in"); !slices.Contains(got, "Sign in to star Return errors, 2 stars") {
		t.Errorf("the rule's page names its star link %q, want two stars", got)
	}
	for _, path := range []string{library + "?tab=rules", "/g/techs/go", "/search?q=errors"} {
		assertShows(t, get(path, nil), "2 2 stars")
	}
	assertShows(t, get(errorsRule, first), "Starred 2")
	assertShows(t, get("/account/stars", second), "Return errors HIGH example/rules · Go")

	if resp := send(t, handler, request{method: http.MethodPost, target: "/account/delete", cookies: first}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("deleting the account answered %d", resp.StatusCode)
	}
	if got := accessibleNames(t, get(errorsRule, nil), "/sign-in"); !slices.Contains(got, "Sign in to star Return errors, 1 star") {
		t.Errorf("after deleting an account, the star link is named %q, want one star", got)
	}
	if resp := send(t, handler, request{method: http.MethodPost, target: unstarPath, cookies: second}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("unstarring answered %d", resp.StatusCode)
	}
	assertShows(t, get("/account/stars", second), "You haven't starred any rules yet.")
	if page := get(library+"?tab=rules", nil); strings.Contains(visibleText(t, page), "star") {
		t.Error("the library's rules show stars once none are left")
	}
}
