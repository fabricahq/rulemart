// The page that offers to include a visitor's private projects, by installing the GitHub App, and the way to stop.

package web

import "net/http"

const (
	// privateHref is the page that offers to include the visitor's private projects, by installing the GitHub App, and
	// removePrivateHref forgets the visitor's installations, with POST.
	privateHref       = dashboardHref + "/private"
	removePrivateHref = dashboardHref + "/github/remove"
)

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

// removePrivate stops reading the signed-in visitor's private repositories, and returns to the dashboard, saying so,
// as the prototype's disconnect does. The page about them said, beside the button, how to uninstall the app on GitHub
// too.
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
	seeOther(w, r, dashboardHref)
}
