//go:build rulemartdev

package web_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/platform/web"
)

// A local build signs in as a test user the way GitHub's callback signs in a GitHub user, so signed-in pages can be
// tried in a browser without an OAuth app.
func TestALocalBuildSignsInAsATestUser(t *testing.T) {
	site := newAccountsSite(t, func(o *web.Options) { o.GitHub = nil })

	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/sign-in?return=%2Fgroups"}))
	resp := send(t, site.handler, request{method: http.MethodPost, target: "/account/dev-sign-in?as=test_user&return=%2Fgroups"})

	assertShows(t, page, "Local build", "Sign in as test_user", "Sign in as test_user_2")
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/groups" {
		t.Fatalf("answered %d to %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	session := cookie(resp, sessionCookie)
	assertCookieAttributes(t, session, 30*24*60*60)
	signedIn := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/groups", cookies: []*http.Cookie{session}}))
	assertShows(t, signedIn, "Signed in as test_user")
	// Only the GitHub button signs in with GitHub, so the header's link shows no GitHub mark without it.
	if signInLinkHasMark(t, body(t, send(t, site.handler, request{method: http.MethodGet, target: "/groups"}))) {
		t.Error("without GitHub, the Sign in link shows GitHub's mark")
	}
	// A test user is no GitHub user: the account page says so, and links no GitHub profile.
	account := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/account", cookies: []*http.Cookie{session}}))
	assertShows(t, account, "Local test user")
	if strings.Contains(account, "github.com/test_user") || strings.Contains(visibleText(t, account), "Signed in with GitHub") {
		t.Error("the account page presents a test user as a GitHub user")
	}
}

func TestALocalBuildSignsInOnlyItsTestUsersAndOnlyFromItself(t *testing.T) {
	site := newAccountsSite(t, nil)

	stranger := send(t, site.handler, request{method: http.MethodPost, target: "/account/dev-sign-in?as=octocat"})
	crossSite := send(t, site.handler, request{method: http.MethodPost, target: "/account/dev-sign-in?as=test_user",
		header: http.Header{"Sec-Fetch-Site": {"cross-site"}}})

	if stranger.StatusCode != http.StatusNotFound || cookie(stranger, sessionCookie) != nil {
		t.Errorf("signing in as a user who isn't a test user answered %d", stranger.StatusCode)
	}
	if crossSite.StatusCode != http.StatusForbidden || cookie(crossSite, sessionCookie) != nil {
		t.Errorf("another site's dev sign-in answered %d", crossSite.StatusCode)
	}
}
