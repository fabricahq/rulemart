package web_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	// owned are the installations visitors may install, and delivered the deliveries acted on.
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

// Deliver acts on a delivery signed "sha256=good", for the installation event, as the fake's change.
func (f *fakeGitHubAccounts) Deliver(_ context.Context, event string, body []byte, signature string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case signature != "sha256=good":
		return accounts.ErrBadSignature
	case event != "installation":
		return accounts.ErrIgnoredEvent
	}
	f.delivered = append(f.delivered, string(body))
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

// GitHub's deliveries carry a body and no browser's headers: a signed one is acted on, an unsigned one refused, and
// one Rulemart has nothing to do for answered, so GitHub doesn't send it again.
func TestTheWebhookActsOnlyOnDeliveriesGitHubSigned(t *testing.T) {
	gitHub := newFakeGitHubAccounts(accounts.Snapshot{})
	site, _ := newGitHubSite(t, gitHub)
	deliver := func(event, signature string) int {
		req := httptest.NewRequest(http.MethodPost, "/account/github/webhook", strings.NewReader(`{"action":"deleted"}`))
		req.Header.Set("X-GitHub-Event", event)
		req.Header.Set("X-Hub-Signature-256", signature)
		req.Header.Set("User-Agent", "GitHub-Hookshot/abc")
		recorder := httptest.NewRecorder()
		site.handler.ServeHTTP(recorder, req)
		_, _ = io.Copy(io.Discard, recorder.Result().Body)
		return recorder.Code
	}
	if got := deliver("installation", "sha256=good"); got != http.StatusNoContent || len(gitHub.delivered) != 1 {
		t.Errorf("a signed delivery: %d, delivered %v", got, gitHub.delivered)
	}
	if got := deliver("installation", "sha256=bad"); got != http.StatusUnauthorized || len(gitHub.delivered) != 1 {
		t.Errorf("an unsigned delivery: %d", got)
	}
	if got := deliver("ping", "sha256=good"); got != http.StatusNoContent {
		t.Errorf("a ping: %d", got)
	}
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
