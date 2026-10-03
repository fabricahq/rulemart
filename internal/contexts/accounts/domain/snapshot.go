// GitHub snapshots: what Rulemart read of a visitor's GitHub account, which the dashboard, the add-a-library picker,
// and checkout's project picker show without reading GitHub on every page view.

package domain

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// MaxRepositories is how many repositories one read looks into, the most recently pushed first, so a visitor in many
// large organizations can't make a read take minutes or spend their rate limit. The dashboard says when a read
// stopped there.
const MaxRepositories = 200

// MaxOrganizations is how many of the visitor's organizations a read lists repositories of: one page of GitHub's list.
const MaxOrganizations = 100

// MaxProvenanceBytes bounds the provenance file a read parses. A project's grows with its rules, about a kilobyte a
// rule, so this holds a thousand.
const MaxProvenanceBytes = 1 << 20

// RefreshInterval is how often a visitor may read their GitHub account again: a refresh within a minute of the last
// read keeps it.
const RefreshInterval = time.Minute

// ErrGitHubTokenRefused reports a token GitHub refused, such as one whose user revoked Rulemart's authorization:
// signing in again gives a new one.
var ErrGitHubTokenRefused = errors.New("GitHub refused the token")

// ErrNoSuchInstallation reports an installation of the GitHub App that GitHub doesn't know, such as one uninstalled
// since.
var ErrNoSuchInstallation = errors.New("GitHub has no such installation of the app")

// Repository is a GitHub repository, as owner/name, and whether it's private.
type Repository struct {
	Owner, Name string
	Private     bool
}

// FullName returns the repository as owner/name.
func (r Repository) FullName() string { return r.Owner + "/" + r.Name }

// GitHubRepository is a repository GitHub listed for a read, before Rulemart looked into it.
type GitHubRepository struct {
	Repository
	// PushedAt is when someone last pushed to it, which orders a read's repositories, the newest first.
	PushedAt time.Time
	// Installation is the installation of the GitHub App that reads it, for a private repository, or 0 for one the
	// visitor's own token reads.
	Installation int64
}

// RootEntries is what the root of a repository's default branch holds that a read looks for.
type RootEntries struct {
	// Manifest is true when it holds rule-library.yaml, the file that makes a repository a Code Rules library.
	Manifest bool
	// CodeRules is true when it holds the .code-rules directory, where a project keeps its rules.
	CodeRules bool
}

// Snapshot is what one read found in a visitor's GitHub account, and the visitor's organizations'.
type Snapshot struct {
	// ReadAt is when the read that found what it holds finished, or the zero time when no read has yet.
	ReadAt time.Time
	// Organizations are the logins of the visitor's organizations, in the order GitHub listed them.
	Organizations []string
	// Libraries are the repositories that publish a Code Rules library, by full name.
	Libraries []PublishableRepository
	// Projects are the repositories whose provenance file names the libraries they import, by full name.
	Projects []Project
	// Truncated is true when the visitor and their organizations hold more repositories than MaxRepositories, so the
	// read left the least recently pushed out.
	Truncated bool
	// Failure says why the latest read failed, so its pages may show an older read's results, or none; it's empty when
	// the latest read succeeded.
	Failure string
}

// PublishableRepository is a repository that holds a rule-library.yaml and a library release tag, release/<n>.
type PublishableRepository struct {
	Repository
	// Release is the number of its latest library release.
	Release int
}

// Project is a repository that uses Code Rules: its .code-rules/generated/provenance.json names its sources.
type Project struct {
	Repository
	// Sources are the libraries it imports, by source name.
	Sources []Source
}

// Source is a library a project imports, as its provenance file records it.
type Source struct {
	// Name is the project's name for the source, which its rules' IDs start with.
	Name string
	// Library is the source's repository on GitHub, as owner/name, or empty for a source on another host.
	Library string
	// Release is the library release the project last synced, or 0 when its provenance names none.
	Release int
	// Groups are the groups the project imports whole.
	Groups []string
	// Rules are the source's rules the project holds at a published version, by ID, which pages compare with the
	// library's current versions.
	Rules []PinnedRule
}

// PinnedRule is a library's rule at the version a project holds.
type PinnedRule struct {
	// Path is the rule's ID in its library, such as techs/go/return-errors.
	Path    string
	Version coderules.RuleVersion
}

// LibraryNames returns the libraries the snapshot's projects import, as owner/name, each once, in the order the
// projects first name them.
func (s Snapshot) LibraryNames() []string {
	var names []string
	seen := map[string]bool{}
	for _, p := range s.Projects {
		for _, source := range p.Sources {
			if key := strings.ToLower(source.Library); source.Library != "" && !seen[key] {
				seen[key] = true
				names = append(names, source.Library)
			}
		}
	}
	return names
}

// Owners returns the logins whose libraries count as the visitor's: login, then the visitor's organizations.
func (s Snapshot) Owners(login string) []string {
	return append([]string{login}, s.Organizations...)
}

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
