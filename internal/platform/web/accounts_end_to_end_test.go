package web_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"testing"

	accountsapp "github.com/fabricahq/rulemart/internal/contexts/accounts/app"
	accountspostgres "github.com/fabricahq/rulemart/internal/contexts/accounts/store/postgres"
	"github.com/fabricahq/rulemart/internal/platform/database/databasetest"
	"github.com/fabricahq/rulemart/internal/platform/web"
)

// A visitor signs in with GitHub, browses signed in, and signs out, with accounts stored as the web function's role
// stores them, so a missing grant fails this test, and a signed-out cookie stops working.
func TestAVisitorSignsInBrowsesAndSignsOutWithStoredSessions(t *testing.T) {
	_, connString := databasetest.New(t)
	gitHub := &fakeGitHub{identity: octocat}
	handler, err := web.New(newCatalog(), web.Options{
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Accounts: accountsapp.Sessions{Store: accountspostgres.New(databasetest.AsWebRole(t, connString))},
		GitHub:   gitHub,
	})
	if err != nil {
		t.Fatal(err)
	}
	site := accountsSite{handler: handler, gitHub: gitHub}

	flow, location := startSignIn(t, site, library)
	signedIn := callback(t, site, url.Values{"code": {authorizedCode}, "state": {location.Query().Get("state")}}, flow)
	session := cookie(signedIn, sessionCookie)
	if signedIn.StatusCode != http.StatusSeeOther || session == nil {
		t.Fatalf("signing in answered %d", signedIn.StatusCode)
	}
	page := send(t, handler, request{method: http.MethodGet, target: library, cookies: []*http.Cookie{session}})
	assertShows(t, body(t, page), "Signed in as octocat")
	account := send(t, handler, request{method: http.MethodGet, target: "/account", cookies: []*http.Cookie{session}})
	assertShows(t, body(t, account), "GitHub user ID 583231")

	out := send(t, handler, request{method: http.MethodPost, target: "/sign-out", cookies: []*http.Cookie{session}})
	if out.StatusCode != http.StatusSeeOther {
		t.Fatalf("signing out answered %d", out.StatusCode)
	}

	// The browser may keep the cookie, as one that missed the sign-out's response would: it signs no one in.
	after := send(t, handler, request{method: http.MethodGet, target: "/account", cookies: []*http.Cookie{session}})
	if after.StatusCode != http.StatusSeeOther || after.Header.Get("Location") != "/sign-in?return=%2Faccount" {
		t.Errorf("after sign-out, the account page answered %d to %q", after.StatusCode, after.Header.Get("Location"))
	}
}
