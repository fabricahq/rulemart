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
	"github.com/fabricahq/rulemart/internal/contexts/catalog/jobs"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/render"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git/gittest"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres"
	"github.com/fabricahq/rulemart/internal/platform/database/databasetest"
	"github.com/fabricahq/rulemart/internal/platform/web"
)

// A visitor signs in, lists a library, and the worker checks it, each as its function's role, so a missing grant
// fails this test. The library's pages then show it unvetted, under the warning, while the home page, the libraries,
// the groups, and search leave it out; once vetted, it's browsed like any other, without being listed again.
func TestAVisitorListsALibraryThatTheWorkerIngestsAsUnvetted(t *testing.T) {
	lib := gittest.NewLibrary(t)
	lib.Group("techs/go", "Go")
	lib.Rule("techs/go/return-errors", "Return errors", "Wrap every returned error before retrying.")
	lib.Release(1, `formatVersion: 1
release: 1
rules: {techs/go/return-errors: 1.0.0}
changes: {techs/go/return-errors: {change: new, summaries: [Add the rule.]}}
`)
	_, connString := databasetest.New(t)
	webStore := postgres.New(databasetest.AsWebRole(t, connString))
	groups, err := shipped.CanonicalGroups()
	if err != nil {
		t.Fatal(err)
	}
	queue := &memoryQueue{}
	gitHub := &fakeGitHub{identity: octocat}
	site := func(vetted []domain.LibraryKey) http.Handler {
		handler, err := web.New(app.Pages{Store: webStore, Vetted: vetted, Groups: groups}, web.Options{
			Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
			Accounts: accountsapp.Sessions{Store: accountspostgres.New(databasetest.AsWebRole(t, connString))},
			GitHub:   gitHub,
			Listings: app.Listings{Store: webStore, Vetted: vetted, Queue: queue},
		})
		if err != nil {
			t.Fatal(err)
		}
		return handler
	}
	handler := site(nil)
	signIn := accountsSite{handler: handler, gitHub: gitHub}
	flow, location := startSignIn(t, signIn, "/list")
	session := cookie(callback(t, signIn, url.Values{"code": {authorizedCode}, "state": {location.Query().Get("state")}}, flow), sessionCookie)
	cookies := []*http.Cookie{session}

	listed := send(t, handler, request{method: http.MethodPost, target: "/list?repository=example%2Frules", cookies: cookies})
	if listed.StatusCode != http.StatusSeeOther || len(queue.bodies) != 1 {
		t.Fatalf("listing answered %d and queued %q", listed.StatusCode, queue.bodies)
	}
	assertShows(t, body(t, send(t, handler, request{method: http.MethodGet, target: "/account/listings", cookies: cookies})),
		"example/rules Checking")

	job, err := jobs.Parse(queue.bodies[0])
	if err != nil {
		t.Fatal(err)
	}
	worker := app.Ingester{
		Repositories: repositories{lib.Repository(7)}, Fetch: git.Fetch, List: git.ListReleaseTags, Render: render.Rule,
		Store: postgres.New(databasetest.AsWorkerRole(t, connString)), Limits: domain.DefaultLimits,
	}
	if check, err := worker.CheckListing(context.Background(), nil, job.Listing); err != nil || check.Outcome != app.ListingIngested {
		t.Fatalf("the worker's check: %+v, %v", check, err)
	}

	assertShows(t, body(t, send(t, handler, request{method: http.MethodGet, target: "/account/listings", cookies: cookies})),
		"example/rules Listed, unvetted")
	for _, path := range []string{library, errorsRule, "/unvetted"} {
		page := body(t, send(t, handler, request{method: http.MethodGet, target: path}))
		if !strings.Contains(page, "not been vetted. Be sure to review") {
			t.Errorf("%s doesn't warn", path)
		}
	}
	for _, path := range []string{"/", "/libraries", "/browse/techs", "/g/techs/go", "/search?q=retrying"} {
		if page := body(t, send(t, handler, request{method: http.MethodGet, target: path})); strings.Contains(page, "example/rules") ||
			strings.Contains(page, "Return errors") {
			t.Errorf("%s shows the unvetted library", path)
		}
	}

	vetted := site([]domain.LibraryKey{{Host: domain.GitHub, RepositoryID: "7"}})
	if page := body(t, send(t, vetted, request{method: http.MethodGet, target: library})); strings.Contains(page, "not been vetted") {
		t.Error("the vetted library's page still warns")
	}
	if got := links(t, body(t, send(t, vetted, request{method: http.MethodGet, target: "/libraries"})), "rules"); !slices.Contains(got, library) {
		t.Errorf("the libraries page links %q, want the vetted library", got)
	}
	if page := body(t, send(t, vetted, request{method: http.MethodGet, target: "/search?q=retrying"})); !strings.Contains(page, "Return errors") {
		t.Error("search doesn't find the vetted library's rule")
	}
}

// memoryQueue keeps what's sent to it, as the jobs queue would.
type memoryQueue struct{ bodies []string }

func (q *memoryQueue) Send(_ context.Context, body string) error {
	q.bodies = append(q.bodies, body)
	return nil
}
