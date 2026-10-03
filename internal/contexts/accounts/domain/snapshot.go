// GitHub snapshots: what Rulemart read of a visitor's GitHub account, which the dashboard, the add-a-library picker,
// and checkout's project picker show without reading GitHub on every page view.

package domain

import (
	"errors"
	"strings"
	"time"
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
	// ReadFailed is true when the latest read failed, so what it holds is an older read's results, or nothing.
	ReadFailed bool
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
