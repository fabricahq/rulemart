package web_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
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

// Two visitors sign in and star a vetted library, as the web function's role, so a missing grant fails this test.
// Pages count both stars for everyone, each visitor's stars page lists only theirs, and deleting one's account takes
// its star away.
func TestVisitorsStarALibraryAndDeletingAnAccountRemovesItsStar(t *testing.T) {
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
		Stars:    app.Stars{Store: webStore, Vetted: vetted},
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

	for _, cookies := range [][]*http.Cookie{first, second} {
		if resp := send(t, handler, request{method: http.MethodPost, target: starPath, cookies: cookies}); resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("starring answered %d", resp.StatusCode)
		}
	}
	assertShows(t, body(t, send(t, handler, request{method: http.MethodGet, target: library})), "Star, 2 stars")
	assertShows(t, body(t, send(t, handler, request{method: http.MethodGet, target: "/libraries"})), "2 stars")
	assertShows(t, body(t, send(t, handler, request{method: http.MethodGet, target: library, cookies: first})), "Starred, 2 stars. Unstar")
	assertShows(t, body(t, send(t, handler, request{method: http.MethodGet, target: "/account/stars", cookies: second})),
		"example/rules · Starred")

	if resp := send(t, handler, request{method: http.MethodPost, target: "/account/delete", cookies: first}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("deleting the account answered %d", resp.StatusCode)
	}
	assertShows(t, body(t, send(t, handler, request{method: http.MethodGet, target: library})), "Star, 1 star")
	if resp := send(t, handler, request{method: http.MethodPost, target: unstarPath, cookies: second}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("unstarring answered %d", resp.StatusCode)
	}
	assertShows(t, body(t, send(t, handler, request{method: http.MethodGet, target: "/account/stars", cookies: second})),
		"You haven't starred a library.")
	assertShows(t, body(t, send(t, handler, request{method: http.MethodGet, target: library})), "Star, 0 stars")
}
