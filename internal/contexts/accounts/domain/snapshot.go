// GitHub snapshots: what Rulemart read of a visitor's GitHub account, which the dashboard, the add-a-library picker,
// and checkout's project picker show without reading GitHub on every page view.

package domain

import (
	"errors"
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

// MaxConfigBytes bounds the configuration a read parses to learn whether a repository is a library: the most the
// catalog reads of a library's manifest, its ContentLimits.FileBytes, so a library the catalog would take is one the
// picker finds. The contexts don't import each other, so the web server's tests check the two agree. A larger
// configuration makes no library, rather than failing the read.
const MaxConfigBytes = 1 << 20

// RefreshInterval is how often a visitor may read their GitHub account again: a refresh within a minute of the last
// read keeps it.
const RefreshInterval = time.Minute

// ErrGitHubTokenRefused reports a token GitHub refused, such as one whose user revoked Rulemart's authorization:
// signing in again gives a new one.
var ErrGitHubTokenRefused = errors.New("GitHub refused the token")

// ErrFileTooLarge reports a file larger than the most a read asked for.
var ErrFileTooLarge = errors.New("the file is larger than the read allows")

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
	// Writable is true when the visitor's token may push to it, so they may add it to Rulemart. It is always false
	// for a repository read with an installation's token, which Rulemart never lists.
	Writable bool
}

// RootEntries is what the root of a repository's default branch holds that a read looks for.
type RootEntries struct {
	// LegacyManifest is true when it holds rule-library.yaml, the manifest of a library in the layout from before
	// Syllabus kept a library's files in its directory. Syllabus reads such a repository as that layout's library,
	// whatever else it holds.
	LegacyManifest bool
	// ProjectDirectory is the directory where a project keeps its files, as Syllabus chooses it: ProjectDirectory when
	// the root holds that directory, LegacyProjectDirectory when it holds that directory and nothing named
	// ProjectDirectory, or empty when it holds neither.
	ProjectDirectory string
}

// Snapshot is what one read found in a visitor's GitHub account, and the visitor's organizations'.
type Snapshot struct {
	// ReadAt is when the read that found what it holds finished, or the zero time when no read has yet.
	ReadAt time.Time
	// Organizations are the logins of the visitor's organizations, in the order GitHub listed them.
	Organizations []string
	// Libraries are the repositories that publish a Syllabus library, by full name.
	Libraries []PublishableRepository
	// Projects are the repositories whose provenance file names the libraries they import, by full name.
	Projects []Project
	// Truncated is true when the read left repositories out: the visitor and their organizations hold more than
	// MaxRepositories, so it left the least recently pushed out, or the visitor belongs to more organizations than
	// MaxOrganizations, so it left the rest's out.
	Truncated bool
	// ReadFailed is true when the latest read failed, so what it holds is an older read's results, or nothing.
	ReadFailed bool
}

// ConfigPath returns where a repository whose Syllabus directory is directory, ProjectDirectory or
// LegacyProjectDirectory, keeps its configuration, whose library section makes it a library, from its root.
func ConfigPath(directory string) string { return directory + "/" + coderules.Manifest }

// DeclaresLibrary reports whether config, a repository's configuration, has a library section, the manifest that
// makes the repository a library, as Syllabus validates it. A configuration Syllabus would refuse declares none.
func DeclaresLibrary(config []byte) bool {
	library, err := coderules.ParseLibraryManifest(config, "config.yaml")
	return err == nil && library != nil
}

// PublishableRepository is a repository that holds a library manifest, rule-library.yaml or a library section in
// its configuration, and a library release tag, release/<n>.
type PublishableRepository struct {
	Repository
	// Release is the number of its latest library release.
	Release int
	// Writable is true when the visitor's token may push to it, so they may add it to Rulemart.
	Writable bool
}

// Project is a repository that uses Syllabus: the provenance file in its project directory names its sources.
type Project struct {
	Repository
	// Directory is where it keeps its files: ProjectDirectory, or LegacyProjectDirectory for a project not yet
	// renamed.
	Directory string
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
