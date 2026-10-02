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
	// 39, neither starting nor ending with a hyphen. GitHub also refuses two hyphens in a row, which
	// ParseListedRepository checks apart.
	gitHubOwner = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)
	// gitHubRepository matches what GitHub allows in a repository's name. GitHub also refuses . and .., and a name
	// that ends in .git, which ParseListedRepository checks apart.
	gitHubRepository = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
)

// reservedOwners are first path segments GitHub keeps for its own pages, which no user or organization can take, so
// settings/profile or orgs/fabricahq names no repository. The list holds the ones a lister might paste.
var reservedOwners = map[string]bool{
	"about": true, "apps": true, "codespaces": true, "collections": true, "enterprise": true, "explore": true,
	"features": true, "issues": true, "join": true, "login": true, "logout": true, "marketplace": true, "new": true,
	"notifications": true, "organizations": true, "orgs": true, "pricing": true, "pulls": true, "search": true,
	"settings": true, "signup": true, "sponsors": true, "topics": true, "trending": true,
}

// repositoryAddresses are what may come before owner/name when a lister gives a repository's address, compared
// without regard to case.
var repositoryAddresses = []string{
	"https://github.com/", "https://www.github.com/", "http://github.com/", "http://www.github.com/",
	"github.com/", "www.github.com/",
}

// ParseListedRepository returns the owner and name of the GitHub repository text names, as a lister may give it:
// owner/name, or its address, such as https://github.com/owner/name, github.com/owner/name, or the address of a
// page inside it, such as .../tree/main, whose query and fragment it ignores. Either may end with a slash, and with
// .git in any case, as a clone URL does: GitHub refuses a repository name that ends in .git, so one .git is never part
// of a name. It fails with ErrInvalidRepository for anything else, such as another host, a name GitHub wouldn't
// allow, or one of GitHub's own pages, such as settings/profile.
func ParseListedRepository(text string) (owner, name string, err error) {
	text = strings.TrimSpace(text)
	path := text
	for _, prefix := range repositoryAddresses {
		if len(text) >= len(prefix) && strings.EqualFold(text[:len(prefix)], prefix) {
			path, _, _ = strings.Cut(text[len(prefix):], "#")
			path, _, _ = strings.Cut(path, "?")
			// A page inside the repository, such as owner/name/tree/main, names the repository first.
			if parts := strings.SplitN(path, "/", 3); len(parts) == 3 {
				path = parts[0] + "/" + parts[1]
			}
			break
		}
	}
	path = strings.TrimSuffix(path, "/")
	if strings.HasSuffix(strings.ToLower(path), ".git") {
		path = path[:len(path)-len(".git")]
	}
	owner, name, ok := strings.Cut(path, "/")
	if !ok || !gitHubOwner.MatchString(owner) || strings.Contains(owner, "--") || reservedOwners[strings.ToLower(owner)] ||
		!gitHubRepository.MatchString(name) || name == "." || name == ".." || strings.HasSuffix(strings.ToLower(name), ".git") {
		return "", "", fmt.Errorf("parse repository %q: %w", text, ErrInvalidRepository)
	}
	return owner, name, nil
}

// Failure returns text as a listing stores why its check failed: a sentence, trimmed, starting with a capital letter
// and ending with a period, within MaxFailureLength bytes, cut at a character's boundary.
func Failure(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "The check failed."
	}
	first, size := utf8.DecodeRuneInString(text)
	text = string(unicode.ToUpper(first)) + text[size:]
	if !strings.ContainsAny(text[len(text)-1:], ".!?") {
		text += "."
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
