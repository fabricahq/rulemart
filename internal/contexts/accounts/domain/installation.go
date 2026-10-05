// Installations of the GitHub App "Rulemart by Fabrica", which reads private repositories where visitors install it.

package domain

import (
	"errors"
	"strconv"
	"strings"
	"time"
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

// ErrRepeatedDelivery reports a genuine delivery of the GitHub App's webhook that Rulemart already acted on within
// DeliveryMemory, by its ID or its body: GitHub redelivering it, or someone sending a copy of it again.
var ErrRepeatedDelivery = errors.New("Rulemart already acted on this delivery")

// ErrNoDeliveryID reports a genuine delivery of the GitHub App's webhook without an ID Rulemart can record, which GitHub
// always sends.
var ErrNoDeliveryID = errors.New("the delivery has no ID Rulemart can record")

// DeliveryMemory is how long Rulemart remembers a delivery of the GitHub App's webhook it acted on, so that the same
// delivery sent again changes nothing: longer than the three days GitHub lets a delivery be redelivered.
const DeliveryMemory = 7 * 24 * time.Hour

// Delivery is a delivery of the GitHub App's webhook, as it arrived.
type Delivery struct {
	// ID is its X-GitHub-Delivery header, unique to the delivery, and the same when GitHub redelivers it. Unlike Body,
	// GitHub doesn't sign it.
	ID string
	// Event is its X-GitHub-Event header, such as installation, which GitHub doesn't sign either.
	Event string
	Body  []byte
	// Signature is its X-Hub-Signature-256 header: sha256= and the hex HMAC-SHA256 of Body with the webhook's secret.
	Signature string
}

// RecordableID reports whether the delivery's ID is one Rulemart can record: 1 to 100 printable ASCII characters, as
// GitHub's GUIDs are.
func (d Delivery) RecordableID() bool {
	if len(d.ID) == 0 || len(d.ID) > 100 {
		return false
	}
	for _, c := range []byte(d.ID) {
		if c <= ' ' || c > '~' {
			return false
		}
	}
	return true
}

// Installation is an installation of the GitHub App that an account reads private repositories through.
type Installation struct {
	ID int64
	// Account is the login of the GitHub account the app is installed on, the visitor's own or an organization's, as
	// GitHub named it when the visitor added the installation; the account may have been renamed since.
	Account string
	// Suspended is true while the account's owner has suspended the app on GitHub: GitHub refuses it a token, so reads
	// leave out what it reads, and keep it to read through again once it's unsuspended.
	Suspended bool
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
	ID     int64
	Action InstallationAction
}

// InstallationAction is what happened to an installation of the GitHub App.
type InstallationAction int

const (
	// RepositoriesChanged means the repositories the installation may read changed.
	RepositoriesChanged InstallationAction = iota
	// Uninstalled means the app was uninstalled, so the installation reads nothing for anyone, ever again.
	Uninstalled
	// Suspended means the account's owner suspended the app, so it reads nothing until they unsuspend it.
	Suspended
	// Unsuspended means the account's owner unsuspended the app, so it reads what it did before.
	Unsuspended
)
