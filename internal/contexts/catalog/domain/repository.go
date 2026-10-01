// Package domain holds the catalog's language and rules: the libraries Rulemart lists, the repositories that
// publish them, and the rules their releases publish. It assembles a library from plain release snapshots: it
// checks that their records form one history, reads only the files the catalog keeps within a content budget, and
// has each current rule's Markdown rendered within what's left of it. It reads no network, disk, or database
// itself, and leaves Markdown to the render package.
package domain

// GitHub is the only code host Rulemart reads libraries from, as the catalog names it. Page URLs name no host,
// so they're GitHub's.
const GitHub = "github"

// LibraryKey identifies a library by its code host and the host's repository ID, as the catalog stores it.
type LibraryKey struct {
	// Host is the code host; github is the only one.
	Host string
	// RepositoryID is the host's ID for the repository. GitHub's is its numeric repository ID, in decimal.
	RepositoryID string
}

// Repository is a library's repository, as its code host describes it.
type Repository struct {
	// Host is the code host, such as GitHub, and ID the host's ID for the repository, which identifies the library
	// across renames and transfers. GitHub's is its numeric repository ID, in decimal.
	Host, ID    string
	Owner, Name string
	Description string
	// OwnerAvatarURL is the owner's avatar on the host's avatar host; empty when unknown.
	OwnerAvatarURL string
	// CloneURL is where ingestion fetches the release tags, such as https://github.com/owner/name.git.
	CloneURL string
}

// FullName returns the repository's owner/name.
func (r Repository) FullName() string { return r.Owner + "/" + r.Name }
