package web_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	accountsapp "github.com/fabricahq/rulemart/internal/contexts/accounts/app"
	accounts "github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/platform/web"
)

// fakeGitHubAccounts keeps each account's snapshot and installations in memory, as accounts/app.GitHubAccounts does in
// Postgres, and reads snapshot, or fails with err, when an account has none or refreshes.
type fakeGitHubAccounts struct {
	mu sync.Mutex
	// snapshot is what a read finds, and err, when set, is how it fails.
	snapshot accounts.Snapshot
	err      error
	// kept are the snapshots kept, by account ID, and installations each account's.
	kept          map[int64]accounts.Snapshot
	installations map[int64][]accounts.Installation
	// reads counts reads of GitHub.
	reads int
	// app is false for a server without the GitHub App.
	app bool
	// owned are the installations visitors may install, and delivered the IDs of the deliveries acted on.
	owned     map[int64]bool
	delivered []string
}

func newFakeGitHubAccounts(snapshot accounts.Snapshot) *fakeGitHubAccounts {
	return &fakeGitHubAccounts{
		snapshot: snapshot, kept: map[int64]accounts.Snapshot{}, installations: map[int64][]accounts.Installation{}, app: true,
		owned: map[int64]bool{},
	}
}

func (f *fakeGitHubAccounts) read(accountID int64) (accounts.Snapshot, error) {
	f.reads++
	if f.err != nil {
		return f.snapshot, f.err
	}
	f.kept[accountID] = f.snapshot
	return f.snapshot, nil
}

func (f *fakeGitHubAccounts) Snapshot(_ context.Context, account accounts.Account, _ accounts.SessionToken) (accounts.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.kept[account.ID]; ok {
		return s, nil
	}
	return f.read(account.ID)
}

func (f *fakeGitHubAccounts) Refresh(_ context.Context, account accounts.Account, _ accounts.SessionToken) (accounts.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.read(account.ID)
}

func (f *fakeGitHubAccounts) Installations(_ context.Context, accountID int64) ([]accounts.Installation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.installations[accountID], nil
}

func (f *fakeGitHubAccounts) PrivateRepositories() bool { return f.app }
func (f *fakeGitHubAccounts) InstallURL() string {
	return "https://github.com/apps/rulemart-by-fabrica/installations/new"
}

func (f *fakeGitHubAccounts) Install(_ context.Context, account accounts.Account, _ accounts.SessionToken, id int64) (accounts.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.owned[id] {
		return accounts.Snapshot{}, accountsapp.ErrNotYourInstallation
	}
	f.installations[account.ID] = append(f.installations[account.ID], accounts.Installation{ID: id, Account: account.Login})
	return f.read(account.ID)
}

func (f *fakeGitHubAccounts) ForgetInstallations(_ context.Context, accountID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.installations, accountID)
	delete(f.kept, accountID)
	return nil
}

// Deliver acts on a delivery signed "sha256=good", for the installation event, once per ID, recording its ID.
func (f *fakeGitHubAccounts) Deliver(_ context.Context, delivery accounts.Delivery) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case delivery.Signature != "sha256=good":
		return accounts.ErrBadSignature
	case delivery.Event != "installation":
		return accounts.ErrIgnoredEvent
	case delivery.ID == "":
		return accounts.ErrNoDeliveryID
	case slices.Contains(f.delivered, delivery.ID):
		return accounts.ErrRepeatedDelivery
	}
	f.delivered = append(f.delivered, delivery.ID)
	return nil
}

// newGitHubSite returns the pages with sign-in and gitHub, and octocat's session.
func newGitHubSite(t *testing.T, gitHub *fakeGitHubAccounts) (accountsSite, *http.Cookie) {
	t.Helper()
	site := newAccountsSite(t, func(o *web.Options) { o.GitHubAccounts = gitHub })
	session := &http.Cookie{Name: sessionCookie, Value: string(site.accounts.signedIn(t, octocat))}
	return site, session
}

// GitHub returns a visitor who installed the app with the installation's ID, which Rulemart records once it's theirs,
// and the dashboard then says it sees their private repos. Anyone else's installation is refused, and a visitor who
// isn't signed in signs in and comes back to finish.
func TestReturningFromInstallingTheAppRecordsTheVisitorsInstallation(t *testing.T) {
	gitHub := newFakeGitHubAccounts(accounts.Snapshot{})
	gitHub.owned[77] = true
	site, session := newGitHubSite(t, gitHub)

	resp := send(t, site.handler, request{method: http.MethodGet, target: "/me/github/installed?installation_id=77&setup_action=install", cookies: []*http.Cookie{session}})
	if resp.StatusCode != http.StatusSeeOther || cookie(resp, noticeCookie).Value != "private-added" {
		t.Fatalf("answered %d, setting %v", resp.StatusCode, resp.Cookies())
	}
	if got := gitHub.installations[1]; len(got) != 1 || got[0].ID != 77 {
		t.Errorf("recorded %+v", got)
	}

	refused := send(t, site.handler, request{method: http.MethodGet, target: "/me/github/installed?installation_id=78&setup_action=install", cookies: []*http.Cookie{session}})
	if refused.StatusCode != http.StatusForbidden || !strings.Contains(body(t, refused), "on your GitHub account or an organization you own") {
		t.Errorf("another's installation answered %d", refused.StatusCode)
	}
	for _, target := range []string{"/me/github/installed", "/me/github/installed?installation_id=-1", "/me/github/installed?installation_id=x"} {
		if resp := send(t, site.handler, request{method: http.MethodGet, target: target, cookies: []*http.Cookie{session}}); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s answered %d", target, resp.StatusCode)
		}
	}
	signedOut := send(t, site.handler, request{method: http.MethodGet, target: "/me/github/installed?installation_id=77"})
	if signedOut.StatusCode != http.StatusSeeOther || !strings.Contains(signedOut.Header.Get("Location"), "installation_id%3D77") {
		t.Errorf("signed out, answered %d to %q", signedOut.StatusCode, signedOut.Header.Get("Location"))
	}
}

// A signed-in visitor whose session keeps no GitHub token Rulemart can use, returning from installing the app, is asked
// to sign in again, coming back to finish: the sign-in page shows its form rather than sending them back to the
// callback, which would send them to it again.
func TestReturningFromInstallingTheAppWithoutAGitHubTokenAsksToSignInAgain(t *testing.T) {
	gitHub := newFakeGitHubAccounts(accounts.Snapshot{})
	gitHub.owned[77], gitHub.err = true, accountsapp.ErrNoGitHubToken
	site, session := newGitHubSite(t, gitHub)

	resp := send(t, site.handler, request{method: http.MethodGet, target: "/me/github/installed?installation_id=77", cookies: []*http.Cookie{session}})
	location, err := url.Parse(resp.Header.Get("Location"))
	if resp.StatusCode != http.StatusSeeOther || err != nil {
		t.Fatalf("answered %d to %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if back := location.Query().Get("return"); back != "/me/github/installed?installation_id=77" {
		t.Errorf("returns to %q, want the callback", back)
	}

	signIn := send(t, site.handler, request{method: http.MethodGet, target: location.RequestURI(), cookies: []*http.Cookie{session}})
	if signIn.StatusCode != http.StatusOK {
		t.Fatalf("the sign-in page answered %d to %q, want its form", signIn.StatusCode, signIn.Header.Get("Location"))
	}
	if page := body(t, signIn); !strings.Contains(page, "Sign in again so Rulemart can read your repositories") || !strings.Contains(page, "Continue with GitHub") {
		t.Errorf("the sign-in page lacks its form:\n%s", page)
	}
}

// An organization whose owners must approve the app returns the visitor without an installation, and the dashboard
// says what's next.
func TestReturningFromRequestingTheAppSaysTheOwnersMustApprove(t *testing.T) {
	site, session := newGitHubSite(t, newFakeGitHubAccounts(accounts.Snapshot{}))
	resp := send(t, site.handler, request{method: http.MethodGet, target: "/me/github/installed?setup_action=request", cookies: []*http.Cookie{session}})
	if resp.StatusCode != http.StatusSeeOther || cookie(resp, noticeCookie).Value != "private-requested" {
		t.Errorf("answered %d, setting %v", resp.StatusCode, resp.Cookies())
	}
}

// GitHub's deliveries carry a body and no browser's headers: a signed one is acted on, an unsigned one refused, one
// Rulemart has nothing to do for answered, so GitHub doesn't send it again, and one it already acted on answered as
// done, without acting again.
func TestTheWebhookActsOnlyOnDeliveriesGitHubSigned(t *testing.T) {
	gitHub := newFakeGitHubAccounts(accounts.Snapshot{})
	site, _ := newGitHubSite(t, gitHub)
	deliver := func(id, event, signature string) int {
		req := httptest.NewRequest(http.MethodPost, "/account/github/webhook", strings.NewReader(`{"action":"deleted"}`))
		if id != "" {
			req.Header.Set("X-GitHub-Delivery", id)
		}
		req.Header.Set("X-GitHub-Event", event)
		req.Header.Set("X-Hub-Signature-256", signature)
		req.Header.Set("User-Agent", "GitHub-Hookshot/abc")
		recorder := httptest.NewRecorder()
		site.handler.ServeHTTP(recorder, req)
		_, _ = io.Copy(io.Discard, recorder.Result().Body)
		return recorder.Code
	}
	if got := deliver("delivery-1", "installation", "sha256=good"); got != http.StatusNoContent || len(gitHub.delivered) != 1 {
		t.Errorf("a signed delivery: %d, delivered %v", got, gitHub.delivered)
	}
	if got := deliver("delivery-1", "installation", "sha256=good"); got != http.StatusOK || len(gitHub.delivered) != 1 {
		t.Errorf("the same delivery again: %d, delivered %v", got, gitHub.delivered)
	}
	if got := deliver("delivery-2", "installation", "sha256=bad"); got != http.StatusUnauthorized || len(gitHub.delivered) != 1 {
		t.Errorf("an unsigned delivery: %d", got)
	}
	if got := deliver("", "installation", "sha256=good"); got != http.StatusBadRequest || len(gitHub.delivered) != 1 {
		t.Errorf("a signed delivery without an ID: %d", got)
	}
	if got := deliver("delivery-3", "ping", "sha256=good"); got != http.StatusNoContent {
		t.Errorf("a ping: %d", got)
	}
}

// GitHub's deliveries aren't a visitor's, so the webhook answers them before anything reads a session or checks that a
// browser started the request on this site, through the site and alone: a delivery with a cross-site browser's headers
// and cookies that a page would clear is decided by its signature, and its response sets no cookie and renders no page.
func TestTheWebhookIgnoresABrowsersHeadersAndCookies(t *testing.T) {
	gitHub := newFakeGitHubAccounts(accounts.Snapshot{})
	site, _ := newGitHubSite(t, gitHub)
	for name, handler := range map[string]http.Handler{"the site": site.handler, "the webhook alone": site.handler.Webhook()} {
		for signature, want := range map[string]int{"sha256=good": http.StatusNoContent, "sha256=bad": http.StatusUnauthorized} {
			t.Run(name+" "+signature, func(t *testing.T) {
				resp := send(t, handler, request{
					method: http.MethodPost, target: "/account/github/webhook",
					cookies: []*http.Cookie{{Name: sessionCookie, Value: "stale"}, {Name: noticeCookie, Value: "unknown"}},
					header: http.Header{
						"Origin": {"https://attacker.example"}, "Sec-Fetch-Site": {"cross-site"},
						"X-Github-Delivery": {name + signature}, "X-Github-Event": {"installation"}, "X-Hub-Signature-256": {signature},
					},
				})
				if resp.StatusCode != want || len(resp.Cookies()) != 0 || strings.Contains(resp.Header.Get("Content-Type"), "html") {
					t.Errorf("answered %d, %q, setting %v; want %d, setting nothing", resp.StatusCode, resp.Header.Get("Content-Type"), resp.Cookies(), want)
				}
			})
		}
	}
}

// The webhook alone, which the web function's webhook alias answers with, answers nothing but a delivery: every other
// method on the webhook's path, every other path, and every other spelling of it gets a plain 404 that sets no cookie
// and can't be cached, as does a delivery without the GitHub App.
func TestTheWebhookAloneAnswersNothingButDeliveries(t *testing.T) {
	refused := func(t *testing.T, resp *http.Response) {
		t.Helper()
		if resp.StatusCode != http.StatusNotFound || len(resp.Cookies()) != 0 || resp.Header.Get("Cache-Control") != "no-store" ||
			!strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain") {
			t.Errorf("answered %d, %q, %q, setting %v; want a plain 404 that sets no cookie and can't be cached",
				resp.StatusCode, resp.Header.Get("Content-Type"), resp.Header.Get("Cache-Control"), resp.Cookies())
		}
	}
	delivery := http.Header{"X-Github-Delivery": {"delivery-1"}, "X-Github-Event": {"installation"}, "X-Hub-Signature-256": {"sha256=good"}}
	stale := []*http.Cookie{{Name: sessionCookie, Value: "stale"}, {Name: noticeCookie, Value: "unknown"}}
	gitHub := newFakeGitHubAccounts(accounts.Snapshot{})
	site, _ := newGitHubSite(t, gitHub)
	for _, tc := range []struct{ method, target string }{
		{http.MethodGet, "/account/github/webhook"},
		{http.MethodHead, "/account/github/webhook"},
		{http.MethodPut, "/account/github/webhook"},
		{http.MethodGet, "/"},
		{http.MethodGet, "/privacy"},
		{http.MethodPost, "/account/signin"},
		{http.MethodPost, "/account/github/webhook/"},
		{http.MethodPost, "/account/github/%77ebhook"},
		{http.MethodPost, "/Account/github/webhook"},
	} {
		t.Run(tc.method+" "+tc.target, func(t *testing.T) {
			refused(t, send(t, site.handler.Webhook(), request{method: tc.method, target: tc.target, cookies: stale, header: delivery}))
		})
	}
	if len(gitHub.delivered) != 0 {
		t.Errorf("delivered %v", gitHub.delivered)
	}

	t.Run("without the app", func(t *testing.T) {
		gitHub := newFakeGitHubAccounts(accounts.Snapshot{})
		gitHub.app = false
		site, _ := newGitHubSite(t, gitHub)
		refused(t, send(t, site.handler.Webhook(), request{method: http.MethodPost, target: "/account/github/webhook", header: delivery}))
	})
}

// Without the GitHub App, there's no installation to return from and no webhook.
func TestWithoutTheAppThereIsNoInstallationOrWebhook(t *testing.T) {
	gitHub := newFakeGitHubAccounts(accounts.Snapshot{})
	gitHub.app = false
	site, session := newGitHubSite(t, gitHub)
	if resp := send(t, site.handler, request{method: http.MethodGet, target: "/me/github/installed?installation_id=1", cookies: []*http.Cookie{session}}); resp.StatusCode != http.StatusNotFound {
		t.Errorf("the installed page answered %d", resp.StatusCode)
	}
	if resp := send(t, site.handler, request{method: http.MethodPost, target: "/account/github/webhook"}); resp.StatusCode != http.StatusNotFound {
		t.Errorf("the webhook answered %d", resp.StatusCode)
	}
}
