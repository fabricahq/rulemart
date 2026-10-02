// Listings: a signed-in account's request that Rulemart show a repository's library, unvetted until vetted.

package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// MaxAccountListings is how many unvetted listings one account may hold, so one account can't fill the unvetted
// area. A vetted listing doesn't count.
const MaxAccountListings = 5

// MaxUnvettedListings is how many unvetted listings Rulemart holds at once. It bounds the unvetted area, the worker's
// hourly checks, and what someone with many accounts can add.
const MaxUnvettedListings = 500

// MaxAccountListingRequests is how many times a day one account may list or try a listing again, each of which makes
// the worker check a repository, so removing and listing again can't keep it busy.
const MaxAccountListingRequests = 20

// MaxListingRequestsPerHour is how many times an hour every account together may list or try a listing again, which
// bounds the checks visitors can ask of the worker, and of GitHub's API, however many accounts ask.
const MaxListingRequestsPerHour = 100

// MaxFailureLength bounds the reason a listing's check failed, as the listings table stores it.
const MaxFailureLength = 1000

// Listing is one account's listing of a repository's library, as the worker checks it.
type Listing struct {
	ID int64
	// Owner and Name are the repository's as its lister gave them.
	Owner, Name string
	// Library is the repository's key once the worker has resolved it; its RepositoryID is empty until then.
	Library LibraryKey
	// Ingested reports whether the catalog stores the library the listing names.
	Ingested bool
	// RequestedAt is when the listing last asked for a check, which tells a check's result from a newer one's.
	RequestedAt time.Time
}

// FullName returns the listed repository as its lister gave it, owner/name.
func (l Listing) FullName() string { return l.Owner + "/" + l.Name }

// Resolved reports whether the worker has found the listed repository's ID on its code host.
func (l Listing) Resolved() bool { return l.Library.RepositoryID != "" }

// ListingState is what a listing's lister sees of it.
type ListingState string

const (
	// ListingChecking is a listing the worker hasn't finished checking.
	ListingChecking ListingState = "checking"
	// ListingListed is a listing whose library the catalog stores, unvetted.
	ListingListed ListingState = "listed"
	// ListingVetted is a listing whose library the release's vetted list names.
	ListingVetted ListingState = "vetted"
	// ListingFailed is a listing whose library never ingested; its failure says why.
	ListingFailed ListingState = "failed"
)

// StateOf returns what a listing's lister sees: vetted when the release vets its library, listed once the catalog
// stores the library, even when a later check failed, failed when a check failed before the library ever ingested,
// and checking otherwise.
func StateOf(vetted, ingested, failed bool) ListingState {
	switch {
	case vetted:
		return ListingVetted
	case ingested:
		return ListingListed
	case failed:
		return ListingFailed
	}
	return ListingChecking
}

// ErrInvalidRepository reports text that doesn't name a GitHub repository.
var ErrInvalidRepository = errors.New("not a GitHub repository")

var (
	// gitHubOwner matches what GitHub allows in a user or organization's name: letters, digits, and hyphens, at most
	// 39. Older names may have hyphens where new ones can't, so it doesn't check where they fall.
	gitHubOwner = regexp.MustCompile(`^[A-Za-z0-9-]{1,39}$`)
	// gitHubRepository matches what GitHub allows in a repository's name.
	gitHubRepository = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
)

// repositoryPrefixes are what may come before owner/name when a lister gives a repository's address.
var repositoryPrefixes = []string{"https://github.com/", "https://www.github.com/", "github.com/", "www.github.com/"}

// ParseListedRepository returns the owner and name of the GitHub repository text names, as a lister may give it:
// owner/name, github.com/owner/name, or its https URL, with or without a trailing slash, and the URL with or without
// .git. It fails with
// ErrInvalidRepository for anything else, such as another host, a page inside a repository, a query, or a name
// GitHub wouldn't allow.
func ParseListedRepository(text string) (owner, name string, err error) {
	text = strings.TrimSpace(text)
	path := text
	for _, prefix := range repositoryPrefixes {
		if rest, ok := strings.CutPrefix(text, prefix); ok {
			// Only an address may end in .git, as a clone URL does; owner/name.git names a repository called name.git.
			path = strings.TrimSuffix(strings.TrimSuffix(rest, "/"), ".git")
			break
		}
	}
	path = strings.TrimSuffix(path, "/")
	owner, name, ok := strings.Cut(path, "/")
	if !ok || !gitHubOwner.MatchString(owner) || !gitHubRepository.MatchString(name) || name == "." || name == ".." {
		return "", "", fmt.Errorf("parse repository %q: %w", text, ErrInvalidRepository)
	}
	return owner, name, nil
}

// Failure returns text as a listing stores why its check failed: trimmed, starting with a capital letter, and within
// MaxFailureLength bytes, cut at a character's boundary.
func Failure(text string) string {
	text = strings.TrimSpace(text)
	if first, size := utf8.DecodeRuneInString(text); size > 0 {
		text = string(unicode.ToUpper(first)) + text[size:]
	}
	if text == "" {
		return "the check failed"
	}
	if len(text) <= MaxFailureLength {
		return text
	}
	cut := MaxFailureLength - len("…")
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "…"
}
