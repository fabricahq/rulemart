// Installations of the GitHub App "Rulemart by Fabrica", which reads private repositories where visitors install it.

package domain

import (
	"errors"
	"strconv"
	"strings"
)

// ErrNoSuchInstallation reports an installation of the GitHub App that GitHub doesn't know, such as one uninstalled
// since.
var ErrNoSuchInstallation = errors.New("GitHub has no such installation of the app")

// ErrBadSignature reports a delivery of the GitHub App's webhook whose signature isn't the webhook secret's, which
// GitHub didn't send.
var ErrBadSignature = errors.New("the delivery's signature isn't the webhook secret's")

// ErrIgnoredEvent reports a genuine delivery of the GitHub App's webhook that Rulemart has nothing to do for, such as a
// new installation, which the visitor's return to Rulemart records, or one for another app.
var ErrIgnoredEvent = errors.New("Rulemart does nothing for this delivery")

// Installation is an installation of the GitHub App that an account reads private repositories through.
type Installation struct {
	ID int64
	// Account is the login of the GitHub account the app is installed on: the visitor's own, or an organization's.
	Account string
}

// SettingsURL returns where on GitHub the installation's repositories are chosen, or the app uninstalled, for the
// visitor login: their own settings for an installation on their account, and the organization's for one on an
// organization.
func (i Installation) SettingsURL(login string) string {
	if strings.EqualFold(i.Account, login) {
		return "https://github.com/settings/installations/" + strconv.FormatInt(i.ID, 10)
	}
	return "https://github.com/organizations/" + i.Account + "/settings/installations/" + strconv.FormatInt(i.ID, 10)
}

// InstallationAccount is the GitHub account an installation of the GitHub App is on, as GitHub describes it.
type InstallationAccount struct {
	Login string
	ID    int64
	// Organization is true for an organization's account, and false for a user's.
	Organization bool
}

// InstallationChange is what GitHub's webhook says happened to an installation of the GitHub App.
type InstallationChange struct {
	ID int64
	// Removed is true when the app was uninstalled or suspended, so it reads nothing for anyone; otherwise the
	// repositories it may read changed.
	Removed bool
}
