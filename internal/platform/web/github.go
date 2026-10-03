// What a signed-in visitor's GitHub account holds, which the dashboard, the add-a-library picker, checkout's project
// picker, and the lists' My libraries show; the GitHub App's return to Rulemart after a visitor installs it; and its
// webhook.

package web

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	accountsapp "github.com/fabricahq/rulemart/internal/contexts/accounts/app"
	accounts "github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
)

// GitHubAccounts reads what signed-in visitors' GitHub accounts hold, and the GitHub App that reads their private
// repositories. accounts/app.GitHubAccounts implements it.
type GitHubAccounts interface {
	// Snapshot returns the account's snapshot, reading GitHub with session's token when Rulemart has none, and Refresh
	// reads it again unless Rulemart did within a minute. Both fail with accountsapp.ErrNoGitHubToken when the session
	// keeps no token GitHub takes, with an error wrapping accountsapp.ErrGitHubRead beside a snapshot that says why
	// when the read failed, and with accountsapp.ErrGitHubReading while another request makes the account's first read.
	Snapshot(ctx context.Context, account accounts.Account, session accounts.SessionToken) (accounts.Snapshot, error)
	Refresh(ctx context.Context, account accounts.Account, session accounts.SessionToken) (accounts.Snapshot, error)
	// Installations returns the installations of the GitHub App the account reads private repositories through.
	Installations(ctx context.Context, accountID int64) ([]accounts.Installation, error)
	// PrivateRepositories reports whether there's a GitHub App for visitors to install, and InstallURL its install page.
	PrivateRepositories() bool
	InstallURL() string
	// Install records installation id for the account, once GitHub confirms it's the visitor's, and reads GitHub again;
	// it fails with accountsapp.ErrNotYourInstallation otherwise. ForgetInstallations forgets every one.
	Install(ctx context.Context, account accounts.Account, session accounts.SessionToken, id int64) (accounts.Snapshot, error)
	ForgetInstallations(ctx context.Context, accountID int64) error
	// Deliver acts on a delivery of the GitHub App's webhook, failing with accounts.ErrBadSignature for one GitHub didn't
	// sign and accounts.ErrIgnoredEvent for one it does nothing for.
	Deliver(ctx context.Context, event string, body []byte, signature string) error
}

const (
	// installedHref is where GitHub returns a visitor who installed the GitHub App, with installation_id and
	// setup_action: the app's setup URL.
	installedHref = "/me/github/installed"
	// webhookHref is the GitHub App's webhook URL. GitHub, which has no account named account, POSTs to it, and a path
	// under account takes no library's page.
	webhookHref = accountHref + "/github/webhook"
)

// maxWebhookBytes bounds a delivery the webhook reads: GitHub caps a payload at 25 MB, but an installation's events,
// even naming every repository it changed, are far smaller, and a Lambda function's request holds at most 6 MB.
const maxWebhookBytes = 4 << 20

// readingRefreshSeconds is how long a page waits before loading again while another request makes the visitor's first
// read of GitHub, which usually takes a few seconds.
const readingRefreshSeconds = 3

// gitHubView is what a page that shows the visitor's GitHub account knows of it.
type gitHubView struct {
	// available is false when Rulemart can't read visitors' GitHub accounts, so pages leave them out.
	available bool
	// snapshot is what Rulemart last read, and readAt says when, or is empty when it never read it.
	snapshot accounts.Snapshot
	readAt   string
	// failed is true when the latest read failed, and reading when another request is making the account's first read,
	// so there's nothing to show yet.
	failed, reading bool
	// signInAgain is the sign-in page for signing in again, when the session keeps no token GitHub takes, or empty.
	signInAgain string
	// refresh is where the Refresh button posts.
	refresh string
	// private is true when the visitor installed the GitHub App, and privateAvailable when there's an app to install.
	private, privateAvailable bool
	installations             []accounts.Installation
}

// gitHubView returns what the page back knows of the signed-in account's GitHub account, reading it when Rulemart has
// none. It answers the request itself with a failure, and returns false, when a read fails for a reason a page
// can't show.
func (s *server) gitHubView(w http.ResponseWriter, r *http.Request, account accounts.Account, back string) (gitHubView, bool) {
	if s.GitHubAccounts == nil {
		return gitHubView{}, true
	}
	view := gitHubView{available: true, privateAvailable: s.privateAvailable(), refresh: refreshHref + returnQuery(back)}
	snapshot, err := s.GitHubAccounts.Snapshot(r.Context(), account, visitorOf(r.Context()).token)
	switch {
	case errors.Is(err, accountsapp.ErrNoGitHubToken):
		view.signInAgain = s.absolute(signInAgainHref(back))
	case errors.Is(err, accountsapp.ErrGitHubRead):
		s.logFailure(r, err)
	case errors.Is(err, accountsapp.ErrGitHubReading):
		// Another request is making the first read, which takes seconds: say so, and load the page again shortly
		// to show it, with or without scripts.
		view.reading = true
		w.Header().Set("Refresh", strconv.Itoa(readingRefreshSeconds))
	case err != nil:
		s.fail(w, r, err)
		return gitHubView{}, false
	}
	view.snapshot, view.failed = snapshot, snapshot.ReadFailed
	if !snapshot.ReadAt.IsZero() {
		view.readAt = moment(snapshot.ReadAt, time.Now())
	}
	installations, err := s.GitHubAccounts.Installations(r.Context(), account.ID)
	if err != nil {
		s.fail(w, r, err)
		return gitHubView{}, false
	}
	view.installations, view.private = installations, len(installations) > 0
	return view, true
}

// myLibraries returns choices as the visitor r comes from can make them, and the libraries their My libraries keeps,
// those their dashboard lists, as Rulemart last read their GitHub account: the libraries they and their organizations
// publish, and those their projects use. Signed out, the choices leave out My libraries, which a list offers only to
// a signed-in visitor. It reads the visitor's GitHub account only while My libraries is on; while that read fails or
// is under way, it keeps the visitor's own libraries and what an earlier read found, as the dashboard does. It answers
// the request with a failure, and returns false, when reading the account fails otherwise.
func (s *server) myLibraries(w http.ResponseWriter, r *http.Request, choices domain.ListChoices) (domain.ListChoices, domain.MyLibraries, bool) {
	v := visitorOf(r.Context())
	if v.account == nil {
		choices.Filters.Mine = false
		return choices, domain.MyLibraries{}, true
	}
	if !choices.Filters.Mine {
		return choices, domain.MyLibraries{}, true
	}
	var snapshot accounts.Snapshot
	if s.GitHubAccounts != nil {
		var err error
		snapshot, err = s.GitHubAccounts.Snapshot(r.Context(), *v.account, v.token)
		switch {
		case errors.Is(err, accountsapp.ErrGitHubRead):
			s.logFailure(r, err)
		case err != nil && !errors.Is(err, accountsapp.ErrNoGitHubToken) && !errors.Is(err, accountsapp.ErrGitHubReading):
			s.fail(w, r, err)
			return domain.ListChoices{}, domain.MyLibraries{}, false
		}
	}
	return choices, domain.MyLibraries{Owners: snapshot.Owners(v.account.Login), Libraries: snapshot.LibraryNames()}, true
}

// refresh reads the signed-in visitor's GitHub account again, and returns to the return parameter, one of the pages
// that show it, the dashboard by default, which says how the read went. A read that succeeded, now or within the
// minute, says so in a toast.
func (s *server) refresh(w http.ResponseWriter, r *http.Request) {
	back := returnPath(r.URL.Query().Get("return"))
	if path, _, _ := strings.Cut(back, "?"); !signedInPage(path) && path != cartHref {
		back = dashboardHref
	}
	v := visitorOf(r.Context())
	if v.account == nil {
		seeOther(w, r, s.absolute(signInPageHref(back)))
		return
	}
	snapshot, err := s.GitHubAccounts.Refresh(r.Context(), *v.account, v.token)
	switch {
	case err == nil && !snapshot.ReadFailed && time.Since(snapshot.ReadAt) < accounts.RefreshInterval:
		// Rulemart reads GitHub at most once a minute, so a press within the minute says so rather than nothing.
		setNotice(w, "refreshed")
	case errors.Is(err, accountsapp.ErrGitHubRead):
		s.logFailure(r, err)
	case errors.Is(err, accountsapp.ErrGitHubReading):
		// The page back says another request is reading GitHub, and loads again once it may have finished.
	case errors.Is(err, accountsapp.ErrNoGitHubToken):
		seeOther(w, r, s.absolute(signInAgainHref(back)))
		return
	case err != nil:
		s.fail(w, r, err)
		return
	}
	seeOther(w, r, back)
}

// installed records the installation of the GitHub App GitHub returned the signed-in visitor with, and returns to the
// dashboard, which then shows their private projects. An organization whose owners must approve the app returns with
// setup_action=request and no installation yet.
func (s *server) installed(w http.ResponseWriter, r *http.Request) {
	account, ok := s.signedIn(w, r, r.URL.RequestURI())
	if !ok {
		return
	}
	query := r.URL.Query()
	if query.Get("setup_action") == "request" {
		setNotice(w, "private-requested")
		seeOther(w, r, dashboardHref)
		return
	}
	id, err := strconv.ParseInt(query.Get("installation_id"), 10, 64)
	if err != nil || id <= 0 {
		s.renderPrivate(w, r, http.StatusBadRequest, messagePage(s.chrome, "Not installed", "GitHub didn't say which installation of Rulemart by Fabrica to use. Try installing it again."))
		return
	}
	_, err = s.GitHubAccounts.Install(r.Context(), account, visitorOf(r.Context()).token, id)
	switch {
	case errors.Is(err, accountsapp.ErrNotYourInstallation):
		s.Log.WarnContext(r.Context(), "installation refused", "route", s.route(r), "requestID", s.requestID(r), "accountID", account.ID)
		s.renderPrivate(w, r, http.StatusForbidden, messagePage(s.chrome, "Not your installation",
			"That installation of Rulemart by Fabrica isn't on your GitHub account or an organization you own, so Rulemart can't read through it for you."))
		return
	case errors.Is(err, accountsapp.ErrNoGitHubToken):
		// The visitor is signed in, so the plain sign-in page would send them straight back here.
		seeOther(w, r, s.absolute(signInAgainHref(returnPath(r.URL.RequestURI()))))
		return
	case errors.Is(err, accountsapp.ErrGitHubRead):
		// The installation is recorded; the dashboard says the read failed.
		s.logFailure(r, err)
	case errors.Is(err, accountsapp.ErrGitHubReading):
		// The installation is recorded; the dashboard says another request is reading GitHub with it.
	case err != nil:
		s.fail(w, r, err)
		return
	}
	s.Log.InfoContext(r.Context(), "installed GitHub App", "route", s.route(r), "requestID", s.requestID(r), "accountID", account.ID)
	setNotice(w, "private-added")
	seeOther(w, r, dashboardHref)
}

// webhook acts on a delivery of the GitHub App's webhook, which GitHub signs with the webhook's secret. It answers 204
// for a delivery it acted on or has nothing to do for, so GitHub doesn't send it again, and 401 for one GitHub didn't
// sign.
func (s *server) webhook(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", privateCache)
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBytes))
	if err != nil {
		http.Error(w, "the delivery is too large", http.StatusRequestEntityTooLarge)
		return
	}
	err = s.GitHubAccounts.Deliver(r.Context(), r.Header.Get("X-GitHub-Event"), body, r.Header.Get("X-Hub-Signature-256"))
	switch {
	case err == nil, errors.Is(err, accounts.ErrIgnoredEvent):
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, accounts.ErrBadSignature):
		s.Log.WarnContext(r.Context(), "webhook refused", "route", s.route(r), "requestID", s.requestID(r), "reason", "bad signature")
		http.Error(w, "the signature isn't the webhook's", http.StatusUnauthorized)
	default:
		s.logFailure(r, err)
		http.Error(w, "Rulemart can't take this delivery right now", http.StatusServiceUnavailable)
	}
}
