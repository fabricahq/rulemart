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

// newPostgresSite ingests a vetted library, example/rules, whose rule has a file of its own and a shared one, into a
// new database, and returns every page, read and written as the web function's role, with a signed-in visitor's
// cookies.
func newPostgresSite(t *testing.T) (http.Handler, []*http.Cookie) {
	t.Helper()
	lib := gittest.NewLibrary(t)
	lib.Group("techs/go", "Go")
	lib.Rule("techs/go/return-errors", "Return errors", "Wrap every returned error. See [the glossary](../../assets/glossary.md).")
	lib.Write("techs/go/assets/return-errors/notes.md", "Notes.\n")
	lib.Write("assets/glossary.md", "Terms.\n")
	lib.Release(1, `formatVersion: 1
release: 1
rules: {techs/go/return-errors: 1.0.0}
changes: {techs/go/return-errors: {change: new, summaries: [Add the rule.]}}
`)
	db, connString := databasetest.New(t)
	repo := lib.Repository(7)
	ingester := app.Ingester{Repositories: repositories{repo}, Fetch: git.Fetch, Renderer: render.Renderer{}, Store: postgres.New(db), Limits: domain.DefaultLimits}
	if _, err := ingester.Ingest(context.Background(), "https://github.com/"+repo.FullName()); err != nil {
		t.Fatal(err)
	}
	groups, err := shipped.CanonicalGroups()
	if err != nil {
		t.Fatal(err)
	}
	vetted := []domain.LibraryKey{{Host: repo.Host, RepositoryID: repo.ID}}
	webStore := postgres.New(databasetest.AsWebRole(t, connString))
	gitHub := &fakeGitHub{identity: octocat}
	handler, err := web.New(app.Pages{Store: webStore, Vetted: vetted, Groups: groups}, web.Options{
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Accounts: accountsapp.Sessions{Store: accountspostgres.New(databasetest.AsWebRole(t, connString)), TokenKeys: testTokenKeys},
		GitHub:   gitHub,
		Listings: app.Listings{Store: webStore, Vetted: vetted},
		Stars:    app.Stars{Store: webStore, Vetted: vetted},
	})
	if err != nil {
		t.Fatal(err)
	}
	site := accountsSite{handler: handler, gitHub: gitHub}
	flow, location := startSignIn(t, site, "/")
	session := cookie(callback(t, site, url.Values{"code": {authorizedCode}, "state": {location.Query().Get("state")}}, flow), sessionCookie)
	return handler, []*http.Cookie{session}
}

// Text Postgres can't hold, a NUL byte or bytes that aren't UTF-8, anywhere in an address never fails a page, for
// visitors signed in or not: in a path it names nothing, so the page is missing; in a parameter, each page reads it
// as it reads any other text it doesn't know, and a rule to star by such a name is missing too.
func TestTextPostgresCantHoldIsRefusedNotFailed(t *testing.T) {
	handler, cookies := newPostgresSite(t)

	for _, bad := range []string{"%00", "%FF", "%C3%28"} {
		for _, r := range []struct {
			method, target string
			// status is what a visitor gets signed in, or 0 for any answer but a failure.
			status int
		}{
			{http.MethodGet, "/example/rules" + bad, http.StatusNotFound},
			{http.MethodGet, "/example" + bad + "/rules", http.StatusNotFound},
			{http.MethodGet, "/example/rules/techs/go/return-errors" + bad, http.StatusNotFound},
			{http.MethodGet, "/example/rules/techs/go" + bad + "/return-errors?tab=versions&from=1.0.0&to=1.0.0", http.StatusNotFound},
			{http.MethodGet, "/g/techs/go" + bad, http.StatusNotFound},
			{http.MethodGet, "/example/rules/techs/go/return-errors/assets/notes.md" + bad, http.StatusNotFound},
			{http.MethodGet, "/example/rules/techs/go/return-errors" + bad + "/assets/notes.md", http.StatusNotFound},
			{http.MethodGet, "/example/rules/assets/glossary.md" + bad, http.StatusNotFound},
			{http.MethodGet, "/example/rules/assets/glossary.md?raw=1" + bad, 0},
			// A shared asset's page shows it with the first rule that lists it when the rule named is none.
			{http.MethodGet, "/example/rules/assets/glossary.md?rule=techs%2Fgo%2Freturn-errors" + bad, http.StatusOK},
			{http.MethodGet, "/example/rules?tab=rules" + bad, 0},
			{http.MethodGet, "/example/rules?tab=releases&release=1" + bad, 0},
			{http.MethodGet, "/example/rules?tab=releases&from=1&to=1" + bad, 0},
			{http.MethodGet, "/example/rules/techs/go/return-errors?tab=versions&from=1.0.0&to=1.0.0" + bad, 0},
			{http.MethodGet, "/search?q=retry" + bad, 0},
			{http.MethodGet, "/list?repository=example" + bad + "%2Frules", 0},
			{http.MethodGet, "/signin?return=%2Fexample" + bad, 0},
			{http.MethodPost, "/list?repository=example%2Frules" + bad, 0},
			{http.MethodPost, "/stars?library=example%2Frules" + bad + "&rule=techs%2Fgo%2Freturn-errors", http.StatusNotFound},
			{http.MethodPost, "/stars?library=example%2Frules&rule=techs%2Fgo%2Freturn-errors" + bad, http.StatusNotFound},
			{http.MethodPost, "/stars/remove?library=example%2Frules&rule=techs%2Fgo%2Freturn-errors" + bad, http.StatusNotFound},
			{http.MethodPost, "/stars?library=example%2Frules&rule=techs%2Fgo%2Freturn-errors&return=%2Fexample" + bad, 0},
			{http.MethodPost, "/me/listings/remove?listing=1" + bad, 0},
			{http.MethodPost, "/me/listings/retry?listing=1" + bad, 0},
		} {
			for _, signedIn := range []bool{false, true} {
				req := request{method: r.method, target: r.target}
				if signedIn {
					req.cookies = cookies
				}
				resp := send(t, handler, req)
				if resp.StatusCode >= 500 || (signedIn && r.status != 0 && resp.StatusCode != r.status) {
					t.Errorf("%s %s (signed in %v): got %d, want %d", r.method, r.target, signedIn, resp.StatusCode, r.status)
				}
			}
		}
	}
}
