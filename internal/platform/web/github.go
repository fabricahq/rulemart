// What a signed-in visitor's GitHub account holds, which the dashboard, the add-a-library picker, and checkout's
// project picker show; the GitHub App's return to Rulemart after a visitor installs it; and its webhook.

package web

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"

	accountsapp "github.com/fabricahq/rulemart/internal/contexts/accounts/app"
	accounts "github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/github"
)

// GitHubAccounts reads what signed-in visitors' GitHub accounts hold, and the GitHub App that reads their private
// repositories. accounts/app.GitHubAccounts implements it.
type GitHubAccounts interface {
	// Snapshot returns the account's snapshot, reading GitHub with session's token when Rulemart has none, and Refresh
	// reads it again unless Rulemart did within a minute. Both fail with accountsapp.ErrNoGitHubToken when the session
	// keeps no token GitHub takes, and with an error wrapping accountsapp.ErrGitHubRead beside a snapshot that says why
	// when the read failed.
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
	// Deliver acts on a delivery of the GitHub App's webhook, failing with github.ErrBadSignature for one GitHub didn't
	// sign and github.ErrIgnoredEvent for one it does nothing for.
	Deliver(ctx context.Context, event string, body []byte, signature string) error
}

const (
	// privateHref is the page that offers to include the visitor's private projects, by installing the GitHub App, and
	// removePrivateHref forgets the visitor's installations, with POST.
	privateHref       = dashboardHref + "/private"
	removePrivateHref = dashboardHref + "/github/remove"
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

// privateAvailable reports whether visitors can let Rulemart read their private repositories.
func (s *server) privateAvailable() bool {
	return s.GitHubAccounts != nil && s.GitHubAccounts.PrivateRepositories()
}

// privatePage shows the signed-in visitor what installing the GitHub App lets Rulemart read, with the way to install
// it, or once they have, the way to stop, or sends anyone else to sign in first.
func (s *server) privatePage(w http.ResponseWriter, r *http.Request) {
	account, ok := s.signedIn(w, r, privateHref)
	if !ok {
		return
	}
	installations, err := s.GitHubAccounts.Installations(r.Context(), account.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	view := privateView{installURL: s.GitHubAccounts.InstallURL(), installed: len(installations) > 0}
	for _, in := range installations {
		view.settings = append(view.settings, installationSettings{account: in.Account, href: in.SettingsURL(account.Login)})
	}
	s.renderPrivate(w, r, http.StatusOK, privatePage(s.chrome, view))
}

// privateView is what the page about private projects shows.
type privateView struct {
	installURL string
	installed  bool
	// settings are where on GitHub each installation's repositories are chosen.
	settings []installationSettings
}

// installationSettings is an installation's settings page on GitHub, for the account it's on.
type installationSettings struct{ account, href string }

// removePrivate stops reading the signed-in visitor's private repositories, and returns to the page about them, which
// says how to uninstall the app on GitHub too.
func (s *server) removePrivate(w http.ResponseWriter, r *http.Request) {
	v := visitorOf(r.Context())
	if v.account == nil {
		seeOther(w, r, s.absolute(signInPageHref(privateHref)))
		return
	}
	if err := s.GitHubAccounts.ForgetInstallations(r.Context(), v.account.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	setNotice(w, "private-removed")
	seeOther(w, r, privateHref)
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
		seeOther(w, r, s.absolute(signInPageHref(returnPath(r.URL.RequestURI()))))
		return
	case errors.Is(err, accountsapp.ErrGitHubRead):
		// The installation is recorded; the dashboard says the read failed.
		s.logFailure(r, err)
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
	case err == nil, errors.Is(err, github.ErrIgnoredEvent):
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, github.ErrBadSignature):
		s.Log.WarnContext(r.Context(), "webhook refused", "route", s.route(r), "requestID", s.requestID(r), "reason", "bad signature")
		http.Error(w, "the signature isn't the webhook's", http.StatusUnauthorized)
	default:
		s.logFailure(r, err)
		http.Error(w, "Rulemart can't take this delivery right now", http.StatusServiceUnavailable)
	}
}
