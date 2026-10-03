package web_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"

	accountsapp "github.com/fabricahq/rulemart/internal/contexts/accounts/app"
	accounts "github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/github"
	"github.com/fabricahq/rulemart/internal/platform/web"
)

const (
	sessionCookie = "__Host-rulemart-session"
	signInCookie  = "__Host-rulemart-sign-in"
	noticeCookie  = "__Host-rulemart-notice"
	// accountSlotClass is a class only the header's account slot has.
	accountSlotClass = "account-slot"
	// gitHubMarkPath starts the outline of GitHub's mark.
	gitHubMarkPath = "M8 0C3.58 0"
)

// fakeAccounts keeps accounts and sessions in memory, as accounts/app.Sessions does in Postgres.
type fakeAccounts struct {
	mu sync.Mutex
	// sessions maps each live token to its account's GitHub user ID.
	sessions map[accounts.SessionToken]int64
	accounts map[int64]accounts.Account
	// replaced records the token each sign-in replaced, "" for none.
	replaced []accounts.SessionToken
	// gitHubTokens are the GitHub token each live session keeps, "" for none.
	gitHubTokens map[accounts.SessionToken]string
	// deleted records the accounts deleted, by ID.
	deleted []int64
	// err, when set, fails every call.
	err error
}

func newFakeAccounts() *fakeAccounts {
	return &fakeAccounts{
		sessions: map[accounts.SessionToken]int64{}, accounts: map[int64]accounts.Account{},
		gitHubTokens: map[accounts.SessionToken]string{},
	}
}

func (f *fakeAccounts) SignIn(_ context.Context, identity accounts.Identity, gitHubToken string, replacing accounts.SessionToken) (accounts.Account, accounts.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return accounts.Account{}, accounts.Session{}, f.err
	}
	f.replaced = append(f.replaced, replacing)
	delete(f.sessions, replacing)
	account, ok := f.accounts[identity.GitHubUserID]
	if !ok {
		account = accounts.Account{ID: int64(len(f.accounts) + 1), CreatedAt: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)}
	}
	account.Identity = identity
	f.accounts[identity.GitHubUserID] = account
	token := accounts.NewSessionToken()
	f.sessions[token] = identity.GitHubUserID
	f.gitHubTokens[token] = gitHubToken
	return account, accounts.Session{Token: token, ExpiresAt: time.Now().Add(accounts.SessionLifetime)}, nil
}

func (f *fakeAccounts) Account(_ context.Context, token accounts.SessionToken) (accounts.Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return accounts.Account{}, f.err
	}
	id, ok := f.sessions[token]
	if !ok {
		return accounts.Account{}, accountsapp.ErrSignedOut
	}
	return f.accounts[id], nil
}

func (f *fakeAccounts) SignOut(_ context.Context, token accounts.SessionToken) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.sessions, token)
	return f.err
}

// SignOutEverywhere ends every session of token's account, while token's session lasts.
func (f *fakeAccounts) SignOutEverywhere(_ context.Context, token accounts.SessionToken) error {
	_, err := f.endEverySession(token)
	return err
}

func (f *fakeAccounts) DeleteAccount(_ context.Context, token accounts.SessionToken) error {
	accountID, err := f.endEverySession(token)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, accountID)
	return nil
}

// endEverySession ends every session of token's account, and returns the account's ID, or fails with
// accountsapp.ErrSignedOut when token's session has ended.
func (f *fakeAccounts) endEverySession(token accounts.SessionToken) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return 0, f.err
	}
	githubID, ok := f.sessions[token]
	if !ok {
		return 0, accountsapp.ErrSignedOut
	}
	for other, id := range f.sessions {
		if id == githubID {
			delete(f.sessions, other)
		}
	}
	return f.accounts[githubID].ID, nil
}

// signedIn adds a session for identity directly, and returns its token.
func (f *fakeAccounts) signedIn(t *testing.T, identity accounts.Identity) accounts.SessionToken {
	t.Helper()
	_, session, err := f.SignIn(context.Background(), identity, "", "")
	if err != nil {
		t.Fatal(err)
	}
	return session.Token
}

func (f *fakeAccounts) live(token accounts.SessionToken) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.sessions[token]
	return ok
}

// fakeGitHub stands in for GitHub: it hands out authorization URLs, and identifies whoever it was told authorized,
// as GitHub would, only for the code it issued and the verifier whose challenge the authorization carried.
type fakeGitHub struct {
	mu sync.Mutex
	// challenge and redirect are what the last authorization carried.
	challenge, redirect string
	// identity is who authorizes; err, when set, fails Identify as a refused code would.
	identity accounts.Identity
	err      error
	// identified counts calls to Identify.
	identified int
}

const authorizedCode = "code-from-github"

func (g *fakeGitHub) AuthorizationURL(state, challenge, redirectURI string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.challenge, g.redirect = challenge, redirectURI
	return "https://github.com/login/oauth/authorize?" + url.Values{"state": {state}, "code_challenge": {challenge}, "redirect_uri": {redirectURI}}.Encode()
}

// issuedToken is the GitHub token fakeGitHub issues for authorizedCode.
const issuedToken = "gho_issued-for-the-authorized-code"

func (g *fakeGitHub) Identify(_ context.Context, code, verifier, redirectURI string) (accounts.Identity, string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.identified++
	switch {
	case g.err != nil:
		return accounts.Identity{}, "", g.err
	case code != authorizedCode:
		return accounts.Identity{}, "", fmt.Errorf("sign in with GitHub: bad_verification_code: %w", github.ErrCodeRefused)
	case github.Challenge(verifier) != g.challenge:
		return accounts.Identity{}, "", errors.New("the verifier doesn't match the challenge")
	case redirectURI != g.redirect:
		return accounts.Identity{}, "", errors.New("the redirect URI differs from the authorization's")
	}
	return g.identity, issuedToken, nil
}

// testTokenKeys seals the GitHub tokens of sessions stored in Postgres in tests.
var testTokenKeys = accountsapp.FixedTokenKey{Key: accounts.NewTokenKey()}

var octocat = accounts.Identity{GitHubUserID: 583231, Login: "octocat", AvatarURL: "https://avatars.githubusercontent.com/u/583231?v=4"}

// accountsSite is the pages' handler with sign-in, its fakes, and its logs.
type accountsSite struct {
	handler  http.Handler
	accounts *fakeAccounts
	gitHub   *fakeGitHub
	logs     *bytes.Buffer
}

// newAccountsSite returns the pages with sign-in through fakes, adjusting its options with adjust, if not nil.
func newAccountsSite(t *testing.T, adjust func(*web.Options)) accountsSite {
	t.Helper()
	site := accountsSite{accounts: newFakeAccounts(), gitHub: &fakeGitHub{identity: octocat}, logs: &bytes.Buffer{}}
	options := web.Options{Log: slog.New(slog.NewJSONHandler(site.logs, nil)), Accounts: site.accounts, GitHub: site.gitHub}
	if adjust != nil {
		adjust(&options)
	}
	handler, err := web.New(newCatalog(), options)
	if err != nil {
		t.Fatal(err)
	}
	site.handler = handler
	return site
}

// request is a request to send to a site: a method and target, the cookies the browser holds, and its headers.
type request struct {
	method, target string
	cookies        []*http.Cookie
	header         http.Header
}

// send sends r to handler as a browser would on the same site, unless r's header says otherwise.
func send(t *testing.T, handler http.Handler, r request) *http.Response {
	t.Helper()
	req := httptest.NewRequest(r.method, r.target, nil)
	if r.method != http.MethodGet && r.method != http.MethodHead {
		req.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	for name, values := range r.header {
		req.Header[name] = values
	}
	for _, cookie := range r.cookies {
		req.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder.Result()
}

// cookie returns the cookie resp sets named name, or nil.
func cookie(resp *http.Response, name string) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// body returns resp's body as text.
func body(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// assertCookieAttributes fails unless c is set the way every Rulemart cookie is: secure, for the whole site, out of
// scripts' reach, and sent across sites only with top-level navigations, for maxAge seconds.
func assertCookieAttributes(t *testing.T, c *http.Cookie, maxAge int) {
	t.Helper()
	if c == nil {
		t.Fatal("the cookie isn't set")
	}
	if !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.Domain != "" || c.MaxAge != maxAge {
		t.Errorf("the cookie %s is set as %q, want Secure, HttpOnly, SameSite=Lax, Path=/, no Domain, and Max-Age=%d", c.Name, c.Raw, maxAge)
	}
}

// startSignIn starts signing in from the page back, and returns the flow's cookie and GitHub's authorization URL.
func startSignIn(t *testing.T, site accountsSite, back string, cookies ...*http.Cookie) (*http.Cookie, *url.URL) {
	t.Helper()
	resp := send(t, site.handler, request{method: http.MethodPost, target: "/signin?" + url.Values{"return": {back}}.Encode(), cookies: cookies})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("starting sign-in answered %d", resp.StatusCode)
	}
	location, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return cookie(resp, signInCookie), location
}

// callback returns from GitHub with query, holding cookies, as GitHub's redirect makes the browser do.
func callback(t *testing.T, site accountsSite, query url.Values, cookies ...*http.Cookie) *http.Response {
	t.Helper()
	header := http.Header{"Sec-Fetch-Site": {"cross-site"}}
	return send(t, site.handler, request{method: http.MethodGet, target: "/account/github/callback?" + query.Encode(), cookies: cookies, header: header})
}

func TestSignInSendsTheVisitorToGitHubWithStateAndAChallenge(t *testing.T) {
	site := newAccountsSite(t, nil)

	flow, location := startSignIn(t, site, "/browse/techs")

	if location.Host != "github.com" || location.Query().Get("state") == "" || location.Query().Get("code_challenge") == "" {
		t.Fatalf("sent the visitor to %s", location)
	}
	if got := location.Query().Get("redirect_uri"); got != "http://example.com/account/github/callback" {
		t.Errorf("GitHub sends the visitor back to %q", got)
	}
	assertCookieAttributes(t, flow, int((10 * time.Minute).Seconds()))
	for _, secret := range []string{location.Query().Get("code_challenge")} {
		if strings.Contains(flow.Value, secret) {
			t.Error("the flow's cookie holds the challenge instead of the verifier")
		}
	}
	if !strings.HasPrefix(flow.Value, location.Query().Get("state")+".") {
		t.Error("the flow's cookie doesn't hold the state GitHub returns")
	}
}

func TestSignInWithGitHubSignsTheVisitorInAndReturnsThemWhereTheyStarted(t *testing.T) {
	site := newAccountsSite(t, nil)
	flow, location := startSignIn(t, site, "/browse/techs?tab=all")

	resp := callback(t, site, url.Values{"code": {authorizedCode}, "state": {location.Query().Get("state")}}, flow)

	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/browse/techs?tab=all" {
		t.Fatalf("answered %d to %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	session := cookie(resp, sessionCookie)
	assertCookieAttributes(t, session, int(accounts.SessionLifetime.Seconds()))
	if cleared := cookie(resp, signInCookie); cleared == nil || cleared.MaxAge >= 0 {
		t.Error("the flow's cookie wasn't cleared")
	}
	if got := resp.Header.Get("Cache-Control"); got != "private, no-store" {
		t.Errorf("Cache-Control is %q", got)
	}
	page := send(t, site.handler, request{method: http.MethodGet, target: "/", cookies: []*http.Cookie{session}})
	assertShows(t, body(t, page), "Signed in as @octocat")
	// The session keeps GitHub's token, which the dashboard reads the visitor's repositories with.
	if got := site.accounts.gitHubTokens[accounts.SessionToken(session.Value)]; got != issuedToken {
		t.Errorf("the session keeps the GitHub token %q, want %q", got, issuedToken)
	}
	for _, leak := range []string{authorizedCode, location.Query().Get("state"), session.Value, issuedToken} {
		if strings.Contains(site.logs.String(), leak) {
			t.Errorf("the logs hold %q: %s", leak, site.logs)
		}
	}
}

// Signing in from no page in particular, such as the sign-in page opened directly, lands on the dashboard, and every
// sign-in says who is signed in, once, as a status toast, as the prototype's does.
func TestSignInWithNoReturnPathLandsOnTheDashboardSayingWhoSignedIn(t *testing.T) {
	site := newAccountsSite(t, nil)

	signInPage := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/signin"}))
	flow, location := startSignIn(t, site, "")
	resp := callback(t, site, url.Values{"code": {authorizedCode}, "state": {location.Query().Get("state")}}, flow)

	if !strings.Contains(signInPage, `action="/signin?return=%2Fme"`) {
		t.Error("the sign-in page opened directly doesn't return to the dashboard")
	}
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/me" {
		t.Fatalf("answered %d to %q, want /me", resp.StatusCode, resp.Header.Get("Location"))
	}
	notice := cookie(resp, noticeCookie)
	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/browse/techs", cookies: []*http.Cookie{cookie(resp, sessionCookie), notice}}))
	if got := toastText(t, page); got != "Signed in as @octocat" {
		t.Errorf("the page after signing in toasts %q", got)
	}
	if kind := noticeToast(t, page); kind != "status" {
		t.Errorf("the notice is a %q toast, want a status toast", kind)
	}
}

// toastText returns the text of the notice page shows as a toast where scripts run, or empty.
func toastText(t *testing.T, page string) string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	for n := range doc.Descendants() {
		if n.Type == html.ElementNode && hasAttribute(n, "data-toast-text") && !inTemplate(n) {
			return nodeText(n)
		}
	}
	return ""
}

// A session planted in the browser before sign-in, say by someone who could set its cookies, ends at sign-in: the
// browser gets a new token, and the old one signs no one in.
func TestSignInReplacesTheSessionTheBrowserHeld(t *testing.T) {
	site := newAccountsSite(t, nil)
	planted := site.accounts.signedIn(t, accounts.Identity{GitHubUserID: 1, Login: "mallory"})
	held := &http.Cookie{Name: sessionCookie, Value: string(planted)}
	flow, location := startSignIn(t, site, "/", held)

	resp := callback(t, site, url.Values{"code": {authorizedCode}, "state": {location.Query().Get("state")}}, flow, held)

	session := cookie(resp, sessionCookie)
	if session == nil || session.Value == string(planted) {
		t.Fatal("the browser kept its token")
	}
	if site.accounts.live(planted) {
		t.Error("the replaced session still signs someone in")
	}
}

// Every way a callback fails leaves the visitor signed out, clears the flow, and says what happened, without asking
// GitHub about a code the browser didn't start.
func TestSignInCallbackRefusesWhatThisBrowserDidNotStart(t *testing.T) {
	for name, tc := range map[string]struct {
		query      func(state string) url.Values
		flow       func(*http.Cookie) *http.Cookie
		gitHubErr  error
		wantStatus int
		wantNotice string
		wantAsked  bool
	}{
		"no sign-in in progress": {
			query:      func(state string) url.Values { return url.Values{"code": {authorizedCode}, "state": {state}} },
			flow:       func(*http.Cookie) *http.Cookie { return nil },
			wantStatus: http.StatusBadRequest, wantNotice: "That sign-in expired",
		},
		"another state": {
			query:      func(string) url.Values { return url.Values{"code": {authorizedCode}, "state": {github.NewState()}} },
			wantStatus: http.StatusBadRequest, wantNotice: "That sign-in expired",
		},
		"no state": {
			query:      func(string) url.Values { return url.Values{"code": {authorizedCode}} },
			wantStatus: http.StatusBadRequest, wantNotice: "That sign-in expired",
		},
		"a flow cookie someone edited": {
			query:      func(state string) url.Values { return url.Values{"code": {authorizedCode}, "state": {state}} },
			flow:       func(c *http.Cookie) *http.Cookie { return &http.Cookie{Name: c.Name, Value: "not.a-flow"} },
			wantStatus: http.StatusBadRequest, wantNotice: "That sign-in expired",
		},
		"the visitor canceled on GitHub": {
			query: func(state string) url.Values {
				return url.Values{"error": {"access_denied"}, "error_description": {"The user has denied your application access."}, "state": {state}}
			},
			wantStatus: http.StatusOK, wantNotice: "You didn't authorize Rulemart",
		},
		"GitHub returned no code": {
			query:      func(state string) url.Values { return url.Values{"state": {state}} },
			wantStatus: http.StatusBadRequest, wantNotice: "GitHub couldn't confirm who you are",
		},
		"GitHub refused the code": {
			query:      func(state string) url.Values { return url.Values{"code": {"another-code"}, "state": {state}} },
			wantStatus: http.StatusBadRequest, wantNotice: "That sign-in didn't complete. Sign in again.", wantAsked: true,
		},
		"GitHub is down": {
			query:      func(state string) url.Values { return url.Values{"code": {authorizedCode}, "state": {state}} },
			gitHubErr:  errors.New("GitHub answered 503 Service Unavailable"),
			wantStatus: http.StatusBadGateway, wantNotice: "GitHub couldn't confirm who you are", wantAsked: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			site := newAccountsSite(t, nil)
			site.gitHub.err = tc.gitHubErr
			flow, location := startSignIn(t, site, "/browse/techs")
			if tc.flow != nil {
				flow = tc.flow(flow)
			}
			var cookies []*http.Cookie
			if flow != nil {
				cookies = append(cookies, flow)
			}

			resp := callback(t, site, tc.query(location.Query().Get("state")), cookies...)

			if resp.StatusCode != tc.wantStatus {
				t.Errorf("answered %d, want %d", resp.StatusCode, tc.wantStatus)
			}
			if session := cookie(resp, sessionCookie); session != nil && session.MaxAge >= 0 {
				t.Error("signed the visitor in")
			}
			if cleared := cookie(resp, signInCookie); cleared == nil || cleared.MaxAge >= 0 {
				t.Error("the flow's cookie wasn't cleared")
			}
			page := body(t, resp)
			assertShows(t, page, tc.wantNotice)
			if got := resp.Header.Get("Cache-Control"); got != "private, no-store" {
				t.Errorf("Cache-Control is %q", got)
			}
			if asked := site.gitHub.identified > 0; asked != tc.wantAsked {
				t.Errorf("asked GitHub: %v, want %v", asked, tc.wantAsked)
			}
			if strings.Contains(site.logs.String(), authorizedCode) || strings.Contains(site.logs.String(), location.Query().Get("state")) {
				t.Errorf("the logs hold the code or state: %s", site.logs)
			}
		})
	}
}

// A visitor already signed in who reaches a callback that can't complete, such as by going Back to it, stays signed
// in and goes on without an error, since there's nothing for them to do.
func TestASignedInVisitorPastAFailedCallbackGoesOnSignedIn(t *testing.T) {
	site := newAccountsSite(t, nil)
	token := site.accounts.signedIn(t, octocat)
	session := &http.Cookie{Name: sessionCookie, Value: string(token)}
	flow, location := startSignIn(t, site, "/browse/techs", session)
	for name, tc := range map[string]struct {
		query   url.Values
		cookies []*http.Cookie
		want    string
	}{
		"without a sign-in in progress": {url.Values{"code": {"x"}, "state": {"y"}}, []*http.Cookie{session}, "/"},
		"with another state":            {url.Values{"code": {"x"}, "state": {"y"}}, []*http.Cookie{session, flow}, "/browse/techs"},
		"after canceling on GitHub":     {url.Values{"error": {"access_denied"}, "state": {location.Query().Get("state")}}, []*http.Cookie{session, flow}, "/browse/techs"},
		"with a code GitHub refuses":    {url.Values{"code": {"x"}, "state": {location.Query().Get("state")}}, []*http.Cookie{session, flow}, "/browse/techs"},
	} {
		t.Run(name, func(t *testing.T) {
			resp := callback(t, site, tc.query, tc.cookies...)
			if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != tc.want {
				t.Errorf("answered %d to %q, want a redirect to %s", resp.StatusCode, resp.Header.Get("Location"), tc.want)
			}
			if c := cookie(resp, sessionCookie); c != nil || !site.accounts.live(token) {
				t.Error("the callback changed the visitor's session")
			}
		})
	}
}

// Where to return comes from the visitor's URL, so it may only be a path on this site.
func TestSignInAndOutReturnOnlyToPathsOnThisSite(t *testing.T) {
	for target, want := range map[string]string{
		"/browse/techs":                   "/browse/techs",
		"/search?q=retry&page=2":          "/search?q=retry&page=2",
		"/example/rules?tab=releases":     "/example/rules?tab=releases",
		"/browse/techs#techs":             "/browse/techs",
		"https://evil.example/":           "/",
		"//evil.example/":                 "/",
		"///evil.example/":                "/",
		`/\evil.example/`:                 "/",
		`/browse/techs\..\evil`:           "/",
		"/%09/evil.example":               "/%09/evil.example",
		"/\t/evil.example":                "/",
		"/\n/evil.example":                "/",
		"javascript:alert(1)":             "/",
		"groups":                          "/",
		"/signin":                         "/",
		"/signout?return=/":               "/",
		"/account/github/callback?x=1":    "/",
		"/me/account/delete":              "/",
		"/me/account/sign-out-everywhere": "/",
		"/" + strings.Repeat("a", 2000):   "/",
	} {
		t.Run(target, func(t *testing.T) {
			site := newAccountsSite(t, nil)
			out := send(t, site.handler, request{method: http.MethodPost, target: "/signout?" + url.Values{"return": {target}}.Encode()})
			if got := out.Header.Get("Location"); out.StatusCode != http.StatusSeeOther || got != want {
				t.Errorf("signing out returned %d to %q, want %q", out.StatusCode, got, want)
			}

			flow, location := startSignIn(t, site, target)
			in := callback(t, site, url.Values{"code": {authorizedCode}, "state": {location.Query().Get("state")}}, flow)
			if got := in.Header.Get("Location"); got != want {
				t.Errorf("signing in returned to %q, want %q", got, want)
			}
		})
	}
}

// A flow cookie is the browser's to change, so the return path it holds is checked again when it comes back.
func TestSignInChecksTheReturnPathItsCookieHoldsAgain(t *testing.T) {
	site := newAccountsSite(t, nil)
	flow, location := startSignIn(t, site, "/browse/techs")
	parts := strings.Split(flow.Value, ".")
	flow.Value = parts[0] + "." + parts[1] + "." + "Ly9ldmlsLmV4YW1wbGU" // base64url of //evil.example

	resp := callback(t, site, url.Values{"code": {authorizedCode}, "state": {location.Query().Get("state")}}, flow)

	if got := resp.Header.Get("Location"); got != "/" {
		t.Errorf("returned to %q, want /", got)
	}
}

func TestSignOutEndsTheSessionAndClearsItsCookie(t *testing.T) {
	site := newAccountsSite(t, nil)
	token := site.accounts.signedIn(t, octocat)
	held := &http.Cookie{Name: sessionCookie, Value: string(token)}

	resp := send(t, site.handler, request{method: http.MethodPost, target: "/signout?return=%2Fbrowse%2Ftechs", cookies: []*http.Cookie{held}})

	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/browse/techs" {
		t.Fatalf("answered %d to %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if cleared := cookie(resp, sessionCookie); cleared == nil || cleared.MaxAge >= 0 || !cleared.Secure {
		t.Error("the session cookie wasn't cleared")
	}
	if site.accounts.live(token) {
		t.Error("the session still signs the visitor in")
	}
}

// Another site can't make a visitor's browser sign out or act as them: browsers say where a request came from.
func TestStateChangingRequestsFromAnotherSiteAreRefused(t *testing.T) {
	baseURL, err := web.ParseBaseURL("https://rulemart.example")
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		header http.Header
		want   int
	}{
		"from another site":                    {http.Header{"Sec-Fetch-Site": {"cross-site"}}, http.StatusForbidden},
		"from a sibling site":                  {http.Header{"Sec-Fetch-Site": {"same-site"}}, http.StatusForbidden},
		"from another origin, by Origin alone": {http.Header{"Sec-Fetch-Site": nil, "Origin": {"https://evil.example"}}, http.StatusForbidden},
		"from this site":                       {http.Header{"Sec-Fetch-Site": {"same-origin"}}, http.StatusSeeOther},
		"typed in the address bar":             {http.Header{"Sec-Fetch-Site": {"none"}}, http.StatusSeeOther},
		"from the public origin, by Origin":    {http.Header{"Sec-Fetch-Site": nil, "Origin": {"https://rulemart.example"}}, http.StatusSeeOther},
		"from no browser":                      {http.Header{"Sec-Fetch-Site": nil}, http.StatusSeeOther},
	} {
		t.Run(name, func(t *testing.T) {
			site := newAccountsSite(t, func(o *web.Options) { o.BaseURL = baseURL })
			token := site.accounts.signedIn(t, octocat)
			for _, target := range []string{"/signout", "/me/account/sign-out-everywhere", "/me/account/delete", "/signin"} {
				// Every request to the function arrives at the Function URL's host, not the public origin.
				resp := send(t, site.handler, request{method: http.MethodPost, target: "https://abc.lambda-url.us-west-2.on.aws" + target,
					cookies: []*http.Cookie{{Name: sessionCookie, Value: string(token)}}, header: tc.header})
				if resp.StatusCode != tc.want {
					t.Errorf("POST %s answered %d, want %d", target, resp.StatusCode, tc.want)
				}
				if tc.want == http.StatusForbidden && (!site.accounts.live(token) || resp.Header.Get("Cache-Control") != "private, no-store") {
					t.Errorf("POST %s ended the session, or its refusal can be cached", target)
				}
			}
		})
	}
}

// CloudFront caches what the function marks public, keyed without cookies, so a page for one signed-in visitor must
// never be marked public, and a page for everyone must vary with cookies in browsers.
func TestOnlyPagesForEveryoneCanBeCached(t *testing.T) {
	site := newAccountsSite(t, nil)
	token := site.accounts.signedIn(t, octocat)
	session := &http.Cookie{Name: sessionCookie, Value: string(token)}
	ended := &http.Cookie{Name: sessionCookie, Value: string(accounts.NewSessionToken())}
	malformed := &http.Cookie{Name: sessionCookie, Value: "not-a-token"}
	// /_static/missing and /_static/v1 aren't static files: the missing page answers them, for its visitor.
	for _, path := range []string{"/", library, retryRule, "/browse/techs", "/search?q=retry", "/example/missing", "/libraries/", "/_static/missing", "/_static/v1/"} {
		for name, tc := range map[string]struct {
			cookie *http.Cookie
			want   string
		}{
			"signed out":            {nil, "public, max-age=0, s-maxage=60"},
			"signed in":             {session, "private, no-store"},
			"with an ended session": {ended, "private, no-store"},
			"with a broken cookie":  {malformed, "private, no-store"},
		} {
			var cookies []*http.Cookie
			if tc.cookie != nil {
				cookies = append(cookies, tc.cookie)
			}
			resp := send(t, site.handler, request{method: http.MethodGet, target: path, cookies: cookies})
			if got := resp.Header.Get("Cache-Control"); got != tc.want {
				t.Errorf("%s %s: Cache-Control is %q, want %q", path, name, got, tc.want)
			}
			if got := resp.Header.Get("Vary"); got != "Cookie" {
				t.Errorf("%s %s: Vary is %q, want Cookie", path, name, got)
			}
		}
	}
	for _, path := range []string{"/signin", "/me"} {
		resp := send(t, site.handler, request{method: http.MethodGet, target: path})
		if got := resp.Header.Get("Cache-Control"); got != "private, no-store" {
			t.Errorf("%s: Cache-Control is %q", path, got)
		}
	}
}

func TestACookieThatSignsNoOneInIsCleared(t *testing.T) {
	site := newAccountsSite(t, nil)
	for name, value := range map[string]string{
		"an ended session": string(accounts.NewSessionToken()),
		"a broken cookie":  "not-a-token",
	} {
		resp := send(t, site.handler, request{method: http.MethodGet, target: "/", cookies: []*http.Cookie{{Name: sessionCookie, Value: value}}})
		if cleared := cookie(resp, sessionCookie); cleared == nil || cleared.MaxAge >= 0 {
			t.Errorf("%s: the cookie wasn't cleared", name)
		}
		assertShows(t, body(t, resp), "Sign in")
	}
}

// Static files are the same for everyone and set no cookie, so they stay cacheable for a year even for a signed-in
// browser.
func TestStaticFilesStayCacheableForASignedInBrowser(t *testing.T) {
	site := newAccountsSite(t, nil)
	token := site.accounts.signedIn(t, octocat)
	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/"}))
	start := strings.Index(page, "/_static/")
	stylesheet := page[start : start+strings.Index(page[start:], `"`)]

	resp := send(t, site.handler, request{method: http.MethodGet, target: stylesheet, cookies: []*http.Cookie{{Name: sessionCookie, Value: string(token)}}})

	if got := resp.Header.Get("Cache-Control"); resp.StatusCode != http.StatusOK || got != "public, max-age=31536000, immutable" {
		t.Errorf("%s answered %d with Cache-Control %q", stylesheet, resp.StatusCode, got)
	}
}

// A failure to read who's signed in fails the page, rather than showing a signed-in visitor a signed-out one.
func TestAPageFailsWhenItCantReadTheSession(t *testing.T) {
	site := newAccountsSite(t, nil)
	token := site.accounts.signedIn(t, octocat)
	site.accounts.err = errors.New("connect to the database: timeout")

	resp := send(t, site.handler, request{method: http.MethodGet, target: "/", cookies: []*http.Cookie{{Name: sessionCookie, Value: string(token)}}})

	if resp.StatusCode != http.StatusServiceUnavailable || resp.Header.Get("Cache-Control") != "private, no-store" {
		t.Errorf("answered %d with Cache-Control %q", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}
}

func TestTheHeaderOffersSignInReturningToThePage(t *testing.T) {
	site := newAccountsSite(t, nil)

	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/search?q=retry"}))

	link := signInHref(t, page)
	if link != "/signin?return=%2Fsearch%3Fq%3Dretry" {
		t.Errorf("Sign in leads to %q", link)
	}
	// On the home page it needs no return.
	if link := signInHref(t, body(t, send(t, site.handler, request{method: http.MethodGet, target: "/"}))); link != "/signin" {
		t.Errorf("on the home page, Sign in leads to %q", link)
	}
}

// On CloudFront's own domain, sign-in still happens on the public origin, where GitHub sends visitors back.
func TestSignInLinksAndGitHubsCallbackAreOnTheBaseURL(t *testing.T) {
	baseURL, err := web.ParseBaseURL("https://rulemart.example")
	if err != nil {
		t.Fatal(err)
	}
	site := newAccountsSite(t, func(o *web.Options) { o.BaseURL = baseURL })

	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: "https://d111111abcdef8.cloudfront.net/browse/techs"}))
	_, location := startSignIn(t, site, "/browse/techs")

	if link := signInHref(t, page); link != "https://rulemart.example/signin?return=%2Fbrowse%2Ftechs" {
		t.Errorf("Sign in leads to %q", link)
	}
	if got := location.Query().Get("redirect_uri"); got != "https://rulemart.example/account/github/callback" {
		t.Errorf("GitHub sends the visitor back to %q", got)
	}
}

func TestTheHeaderShowsTheSignedInVisitorsMenu(t *testing.T) {
	site := newAccountsSite(t, nil)
	token := site.accounts.signedIn(t, octocat)

	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/browse/techs", cookies: []*http.Cookie{{Name: sessionCookie, Value: string(token)}}}))

	assertShows(t, page, "Signed in as @octocat", "Dashboard", "Sign out")
	if strings.Contains(visibleText(t, page), "Sign in ") {
		t.Error("a signed-in visitor is offered sign-in")
	}
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	form := find(doc, func(n *html.Node) bool { return n.Data == "form" && attribute(n, "method") == "post" })
	if form == nil || attribute(form, "action") != "/signout?return=%2Fbrowse%2Ftechs" {
		t.Errorf("the sign-out form is %v", form)
	}
	if find(form, func(n *html.Node) bool { return n.Data == "input" }) != nil {
		t.Error("the sign-out form sends a body, which CloudFront can't forward")
	}
	if img := find(doc, func(n *html.Node) bool { return n.Data == "img" && attribute(n, "src") == octocat.AvatarURL }); img == nil {
		t.Error("the header doesn't show the visitor's avatar")
	}
}

func TestTheAccountPageShowsWhatRulemartKeepsOnlyToItsOwner(t *testing.T) {
	site := newAccountsSite(t, nil)
	token := site.accounts.signedIn(t, octocat)

	signedOut := send(t, site.handler, request{method: http.MethodGet, target: "/me"})
	signedIn := send(t, site.handler, request{method: http.MethodGet, target: "/me", cookies: []*http.Cookie{{Name: sessionCookie, Value: string(token)}}})

	if signedOut.StatusCode != http.StatusSeeOther || signedOut.Header.Get("Location") != "/signin?return=%2Fme" {
		t.Errorf("signed out, the account page answered %d to %q", signedOut.StatusCode, signedOut.Header.Get("Location"))
	}
	page := body(t, signedIn)
	assertShows(t, page, "octocat", "GitHub user ID 583231", "Username octocat", "Account created 2 Oct 2026", "Sign out everywhere", "Delete my account")
	if !strings.Contains(page, `<meta name="robots" content="noindex">`) {
		t.Error("the account page can be indexed")
	}
}

func TestSignOutEverywhereEndsEverySessionOfTheAccount(t *testing.T) {
	site := newAccountsSite(t, nil)
	here := site.accounts.signedIn(t, octocat)
	elsewhere := site.accounts.signedIn(t, octocat)
	other := site.accounts.signedIn(t, accounts.Identity{GitHubUserID: 2, Login: "hubot"})

	resp := send(t, site.handler, request{method: http.MethodPost, target: "/me/account/sign-out-everywhere", cookies: []*http.Cookie{{Name: sessionCookie, Value: string(here)}}})

	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/" {
		t.Fatalf("answered %d to %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if site.accounts.live(here) || site.accounts.live(elsewhere) || !site.accounts.live(other) {
		t.Error("didn't end exactly the account's sessions")
	}
	if cleared := cookie(resp, sessionCookie); cleared == nil || cleared.MaxAge >= 0 {
		t.Error("the session cookie wasn't cleared")
	}
	assertShows(t, followNotice(t, site, resp, "/"), "You're signed out of every browser.")
}

func TestDeleteAccountDeletesOnlyTheSignedInAccount(t *testing.T) {
	site := newAccountsSite(t, nil)
	token := site.accounts.signedIn(t, octocat)

	signedOut := send(t, site.handler, request{method: http.MethodPost, target: "/me/account/delete"})
	resp := send(t, site.handler, request{method: http.MethodPost, target: "/me/account/delete", cookies: []*http.Cookie{{Name: sessionCookie, Value: string(token)}}})

	if signedOut.StatusCode != http.StatusSeeOther || signedOut.Header.Get("Location") != "/" {
		t.Errorf("signed out, deleting answered %d to %q", signedOut.StatusCode, signedOut.Header.Get("Location"))
	}
	// Post, redirect, get: the page that says so is an ordinary page, which reloads and goes back like one.
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/" || len(site.accounts.deleted) != 1 || site.accounts.deleted[0] != 1 {
		t.Fatalf("answered %d to %q, deleting %v", resp.StatusCode, resp.Header.Get("Location"), site.accounts.deleted)
	}
	page := followNotice(t, site, resp, "/")
	assertShows(t, page, "Rulemart deleted your account and signed you out everywhere.", "Sign in")
	if strings.Contains(visibleText(t, page), "Signed in as") {
		t.Error("the page still shows the deleted account signed in")
	}
}

// followNotice follows resp's redirect to target as a browser would, holding the cookies resp set, checks that the
// page clears the notice it shows and can't be cached, and returns the page.
func followNotice(t *testing.T, site accountsSite, resp *http.Response, target string) string {
	t.Helper()
	notice := cookie(resp, noticeCookie)
	if notice == nil || notice.MaxAge <= 0 || notice.MaxAge > 60 || !notice.Secure || !notice.HttpOnly {
		t.Fatalf("the redirect sets the notice cookie as %v", notice)
	}
	page := send(t, site.handler, request{method: http.MethodGet, target: target, cookies: []*http.Cookie{notice}})
	if cleared := cookie(page, noticeCookie); cleared == nil || cleared.MaxAge >= 0 {
		t.Error("the page that showed the notice didn't clear it")
	}
	if got := page.Header.Get("Cache-Control"); got != "private, no-store" {
		t.Errorf("the page with the notice has Cache-Control %q", got)
	}
	return body(t, page)
}

func TestSignOutSaysSoOnThePageItReturnsTo(t *testing.T) {
	site := newAccountsSite(t, nil)
	token := site.accounts.signedIn(t, octocat)

	resp := send(t, site.handler, request{method: http.MethodPost, target: "/signout?return=%2Fbrowse%2Ftechs", cookies: []*http.Cookie{{Name: sessionCookie, Value: string(token)}}})

	page := followNotice(t, site, resp, "/browse/techs")
	assertShows(t, page, "You're signed out.")
	// Where scripts run, it's a toast that only reports, rather than a banner that moves the page.
	if kind := noticeToast(t, page); kind != "status" {
		t.Errorf("the notice is a %q toast, want a status toast", kind)
	}
	// The notice shows once.
	again := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/browse/techs"}))
	if strings.Contains(visibleText(t, again), "signed out") {
		t.Error("a page without the notice cookie shows the notice")
	}
}

// A notice cookie only names a notice; anything else shows nothing, and is cleared.
func TestANoticeCookieShowsOnlyRulemartsOwnNotices(t *testing.T) {
	site := newAccountsSite(t, nil)

	resp := send(t, site.handler, request{method: http.MethodGet, target: "/browse/techs", cookies: []*http.Cookie{{Name: noticeCookie, Value: "<script>alert(1)</script>"}}})

	if page := body(t, resp); strings.Contains(page, "alert(1)") || strings.Contains(visibleText(t, page), "signed out") {
		t.Error("the page shows the cookie's value or a notice")
	}
	if cleared := cookie(resp, noticeCookie); cleared == nil || cleared.MaxAge >= 0 {
		t.Error("an unknown notice wasn't cleared")
	}
}

// Signing out from a page only a signed-in visitor can see, such as the account page, returns home rather than to a
// sign-in page.
func TestSigningOutFromTheAccountPageReturnsHome(t *testing.T) {
	site := newAccountsSite(t, nil)
	for _, target := range []string{"/signout?return=%2Fme", "/signout?return=%2Fme%3Ftab%3Dstars", "/signout?return=%2Fme%2Fadd"} {
		token := site.accounts.signedIn(t, octocat)
		resp := send(t, site.handler, request{method: http.MethodPost, target: target, cookies: []*http.Cookie{{Name: sessionCookie, Value: string(token)}}})
		if got := resp.Header.Get("Location"); got != "/" {
			t.Errorf("POST %s returned to %q, want /", target, got)
		}
	}
	// A stale tab's sign-out everywhere, after its session ended elsewhere, also goes home, saying the visitor is
	// signed out.
	stale := send(t, site.handler, request{method: http.MethodPost, target: "/me/account/sign-out-everywhere"})
	if stale.StatusCode != http.StatusSeeOther || stale.Header.Get("Location") != "/" {
		t.Errorf("a stale sign-out everywhere answered %d to %q", stale.StatusCode, stale.Header.Get("Location"))
	}
	assertShows(t, followNotice(t, site, stale, "/"), "You're signed out.")
}

// Sent to sign in from the account page, a visitor is told why.
func TestTheSignInPageSaysWhySignInIsNeededForTheAccount(t *testing.T) {
	site := newAccountsSite(t, nil)
	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/signin?return=%2Fme"}))
	assertShows(t, page, "Sign in to see your dashboard.")
}

// On the account page, the menu marks Account as the current page.
func TestTheMenuMarksTheAccountPageCurrent(t *testing.T) {
	site := newAccountsSite(t, nil)
	token := site.accounts.signedIn(t, octocat)
	for path, want := range map[string]string{"/me": "page", "/browse/techs": ""} {
		page := body(t, send(t, site.handler, request{method: http.MethodGet, target: path, cookies: []*http.Cookie{{Name: sessionCookie, Value: string(token)}}}))
		doc, err := html.Parse(strings.NewReader(page))
		if err != nil {
			t.Fatal(err)
		}
		link := find(doc, func(n *html.Node) bool { return n.Data == "a" && attribute(n, "href") == "/me" })
		if link == nil || attribute(link, "aria-current") != want {
			t.Errorf("%s: the menu's Account link is %v, want aria-current %q", path, link, want)
		}
	}
}

// The sign-in page's header shows Sign in as every page's does, marked as the current page and leading to the page
// itself, with the same return, so the header neither changes nor leaves a gap there.
func TestTheSignInPagesHeaderShowsSignInAsCurrent(t *testing.T) {
	site := newAccountsSite(t, nil)
	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/signin?return=%2Fbrowse%2Ftechs"}))
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	header := find(doc, func(n *html.Node) bool { return n.Data == "header" })
	link := find(header, func(n *html.Node) bool { return n.Data == "a" && strings.HasPrefix(nodeText(n), "Sign in") })
	if link == nil {
		t.Fatal("the sign-in page's header has no Sign in link")
	}
	if href := attribute(link, "href"); !strings.HasSuffix(href, "/signin?return=%2Fbrowse%2Ftechs") {
		t.Errorf("Sign in leads to %q, want the sign-in page returning to /browse/techs", href)
	}
	if attribute(link, "aria-current") != "page" {
		t.Error("Sign in isn't marked as the current page")
	}
	if find(header, func(n *html.Node) bool { return strings.Contains(attribute(n, "class"), "invisible") }) != nil {
		t.Error("the header holds an invisible placeholder")
	}
}

// The header's sign-in link shows GitHub's mark only when it leads to signing in with GitHub.
func TestTheHeaderShowsGitHubsMarkOnlyForGitHubSignIn(t *testing.T) {
	site := newAccountsSite(t, nil)
	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/browse/techs"}))
	if !signInLinkHasMark(t, page) {
		t.Error("with GitHub, the Sign in link has no GitHub mark")
	}
}

// signInLinkHasMark reports whether page's Sign in link holds GitHub's mark.
func signInLinkHasMark(t *testing.T, page string) bool {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	link := find(doc, func(n *html.Node) bool {
		return n.Data == "a" && strings.HasPrefix(strings.TrimSpace(visibleTextOf(n)), "Sign in")
	})
	if link == nil {
		t.Fatal("no Sign in link")
	}
	return find(link, func(n *html.Node) bool {
		return n.Data == "path" && strings.HasPrefix(attribute(n, "d"), gitHubMarkPath)
	}) != nil
}

func TestASignedInVisitorIsSentOnFromTheSignInPage(t *testing.T) {
	site := newAccountsSite(t, nil)
	token := site.accounts.signedIn(t, octocat)

	resp := send(t, site.handler, request{method: http.MethodGet, target: "/signin?return=%2Fbrowse%2Ftechs", cookies: []*http.Cookie{{Name: sessionCookie, Value: string(token)}}})

	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/browse/techs" {
		t.Errorf("answered %d to %q", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestTheSignInPageSaysWhatRulemartReadsFromGitHub(t *testing.T) {
	site := newAccountsSite(t, nil)

	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/signin?return=%2Fbrowse%2Ftechs"}))

	assertShows(t, page, "Sign in to Rulemart", "Continue with GitHub", "Rulemart reads your public profile and public repos, and never writes to GitHub.", "Star the rules you find useful")
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	form := find(doc, func(n *html.Node) bool {
		return n.Data == "form" && attribute(n, "action") == "/signin?return=%2Fbrowse%2Ftechs"
	})
	if form == nil || attribute(form, "method") != "post" {
		t.Error("the GitHub button doesn't post to start signing in, returning to /browse/techs")
	}
}

// With GitHub sign-in configured, the page reads as the prototype's: the perks, Continue with GitHub, and the footnote
// about what Rulemart reads under it. A local build lists its test users after all of that, not in the button's place.
func TestTheSignInPageShowsContinueWithGitHubUnderThePerksWithTheFootnoteBelow(t *testing.T) {
	site := newAccountsSite(t, nil)

	text := visibleText(t, body(t, send(t, site.handler, request{method: http.MethodGet, target: "/signin"})))

	order := []string{
		"Publish your libraries and see who uses them",
		"Continue with GitHub",
		"Browsing needs no account. Rulemart reads your public profile and public repos, and never writes to GitHub.",
	}
	if web.DevSignIn {
		order = append(order, "Local build", "Sign in as ")
	}
	at := 0
	for _, want := range order {
		i := strings.Index(text[at:], want)
		if i < 0 {
			t.Fatalf("%q doesn't follow %q in %q", want, text[:at], text)
		}
		at += i + len(want)
	}
}

// Without accounts, or without GitHub in a release build, pages offer no way to sign in that doesn't work.
func TestPagesOfferNoSignInThatIsNotAvailable(t *testing.T) {
	without := map[string]func(*web.Options){
		"accounts": func(o *web.Options) { o.Accounts, o.GitHub = nil, nil },
		"GitHub":   func(o *web.Options) { o.GitHub = nil },
	}
	for name, adjust := range without {
		t.Run("without "+name, func(t *testing.T) {
			if web.DevSignIn && name == "GitHub" {
				t.Skip("a dev build offers test users without GitHub")
			}
			site := newAccountsSite(t, adjust)
			page := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/"}))
			if strings.Contains(visibleText(t, page), "Sign in") {
				t.Error("the header offers sign-in")
			}
			if resp := send(t, site.handler, request{method: http.MethodPost, target: "/signin"}); resp.StatusCode != http.StatusNotFound {
				t.Errorf("POST /signin answered %d", resp.StatusCode)
			}
			// Without accounts there's no sign-in page at all; without GitHub, it says sign-in isn't available.
			signIn := send(t, site.handler, request{method: http.MethodGet, target: "/signin"})
			text := visibleText(t, body(t, signIn))
			wantText := map[string]string{"accounts": "Rulemart has no page here", "GitHub": "Sign-in isn't available yet"}[name]
			if signIn.StatusCode != http.StatusNotFound || !strings.Contains(text, wantText) || strings.Contains(text, "Continue with GitHub") {
				t.Errorf("GET /signin answered %d: %s", signIn.StatusCode, text)
			}
		})
	}
}

// findLink returns the href of the first link in page whose text is text, or fails the test.
// signInHref returns where the header's Sign in link leads, which says "with GitHub" when it does.
func signInHref(t *testing.T, page string) string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	link := find(doc, func(n *html.Node) bool {
		return n.Data == "a" && strings.HasPrefix(strings.TrimSpace(visibleTextOf(n)), "Sign in")
	})
	if link == nil {
		t.Fatalf("no link reads Sign in")
	}
	return attribute(link, "href")
}

// visibleTextOf returns the text under n.
func visibleTextOf(n *html.Node) string {
	var text strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			text.WriteString(n.Data)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return text.String()
}

// Browsers check a form's redirects against the content security policy's form-action, so the sign-in form, which
// posts here and is redirected to GitHub, needs GitHub's authorization page allowed, and nothing else of GitHub's,
// and no other page needs it.
func TestOnlyTheSignInPageMayRedirectAFormToGitHubsAuthorization(t *testing.T) {
	site := newAccountsSite(t, nil)
	for path, want := range map[string]string{
		"/signin":                  "'self' https://github.com/login/oauth/authorize",
		"/account/github/callback": "'self' https://github.com/login/oauth/authorize",
		"/":                        "'self'",
		"/browse/techs":            "'self'",
	} {
		resp := send(t, site.handler, request{method: http.MethodGet, target: path})
		var formAction string
		for directive := range strings.SplitSeq(resp.Header.Get("Content-Security-Policy"), ";") {
			if fields := strings.Fields(directive); len(fields) > 0 && fields[0] == "form-action" {
				formAction = strings.Join(fields[1:], " ")
			}
		}
		if formAction != want {
			t.Errorf("%s: form-action allows %q, want %q", path, formAction, want)
		}
	}
}
