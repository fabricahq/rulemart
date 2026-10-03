// Package store is the catalog's persistence contract: what the catalog's operations need from storage.
// store/postgres implements it.
package store

import (
	"context"
	"errors"
	"time"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// Writer replaces what the catalog stores about a library, and reads back what ingestion needs to decide whether to
// replace it again.
type Writer interface {
	// ReplaceLibrary makes the catalog's rows for lib match it, in one transaction, and returns how many rows
	// changed. Rows that still exist keep their ids, and a row whose values didn't change isn't written, so
	// replacing a library with itself changes nothing.
	ReplaceLibrary(ctx context.Context, lib domain.Library) (changed int64, err error)
	// Checkpoint returns where the library was last fetched from, the tags of its stored releases, and whether any of
	// its stored versions lacks content, or found false when the catalog has no such library.
	Checkpoint(ctx context.Context, library domain.LibraryKey) (checkpoint domain.Checkpoint, found bool, err error)
	// Listing returns the listing id as the worker checks it, or found false when there's no such listing.
	Listing(ctx context.Context, id int64) (listing domain.Listing, found bool, err error)
	// ResolveListing records that the listing id names the repository library, or fails with ErrAlreadyListed when
	// another listing names it.
	ResolveListing(ctx context.Context, id int64, library domain.LibraryKey) error
	// RecordListingCheck records that a check of the listing id finished now, and why it failed, or that it succeeded
	// when failure is empty, and reports whether it did: it records nothing when the listing asked for another check
	// after requestedAt, when this one started, or is gone.
	RecordListingCheck(ctx context.Context, id int64, requestedAt time.Time, failure string) (recorded bool, err error)
	// ListingsToCheck returns the IDs of the listings the hourly poll checks, in the order they were listed: every one
	// vetted doesn't hold, except one whose check failed before its library ever ingested.
	ListingsToCheck(ctx context.Context, vetted []domain.LibraryKey) ([]int64, error)
}

// Listings adds, reads, removes, and retries an account's listings, as the web function does. A listing is unvetted
// unless vetted holds the repository it names.
type Listings interface {
	// CheckListing reports whether the account may list the repository owner/name, without listing it. It fails with
	// a *ListingConflict when a listing or vetted library already has that name, with ErrAccountListingLimit when the
	// account holds domain.MaxAccountListings unvetted listings, with ErrListingsFull when Rulemart holds
	// domain.MaxUnvettedListings, and with ErrListingTooOften or ErrListingsBusy when the account, or every account
	// together, asked for as many checks as it may.
	CheckListing(ctx context.Context, vetted []domain.LibraryKey, accountID int64, owner, name string) error
	// CreateListing lists the repository owner/name for the account, and returns the listing's ID, after the checks
	// CheckListing makes, in the same transaction, which no other listing can change before it commits. Another
	// account's listing by that name that failed before its library ever ingested doesn't stand in the way: listing
	// removes it.
	CreateListing(ctx context.Context, vetted []domain.LibraryKey, accountID int64, owner, name string) (int64, error)
	// AccountListings returns the account's listings, newest first.
	AccountListings(ctx context.Context, vetted []domain.LibraryKey, accountID int64) ([]views.AccountListing, error)
	// RemoveListing removes the account's listing id, or fails with ErrNotFound when the account has no such listing.
	RemoveListing(ctx context.Context, accountID, id int64) error
	// RetryListing asks the worker to check the account's listing id again, or fails with ErrNotFound when the
	// account has no such listing, ErrListingNotFailed when its last check didn't fail, or ErrListingTooOften or
	// ErrListingsBusy, as CheckListing does.
	RetryListing(ctx context.Context, accountID, id int64) error
}

// Stars stars and unstars rules for accounts, and lists the rules an account's stars count toward, as the web function
// does. Only a current rule of a library vetted holds can be starred. A star stays on its rule, and counts toward it
// while it's current; once it's retired, toward the current rule its chain of replacements reaches, in the same
// library, within domain.MaxReplacements rules, so a renamed rule keeps its stars.
type Stars interface {
	// Star stars the current rule at rulePath in the library owner/name for the account, the library matched without
	// regard to case, and the rule too, preferring the rule spelled exactly so. It fails with ErrNotFound when vetted
	// holds no library by that name, or it has no current rule at rulePath. A rule the account starred already keeps its
	// one star. first is true when the star is the account's only one: it had none, and now has this one.
	Star(ctx context.Context, vetted []domain.LibraryKey, accountID int64, owner, name, rulePath string) (first bool, err error)
	// Unstar removes every star of the account's that counts toward the rule Star finds: on the rule, and on the
	// retired rules it replaced. It does nothing when the account has none, and fails with ErrNotFound as Star does.
	Unstar(ctx context.Context, vetted []domain.LibraryKey, accountID int64, owner, name, rulePath string) error
	// Starred reports whether one of the account's stars counts toward the current rule at rulePath in the library
	// owner/name, matched as Star matches them, vetted or not; it's false when there's no such rule.
	Starred(ctx context.Context, accountID int64, owner, name, rulePath string) (bool, error)
	// AccountStars returns each current rule of a vetted library that the account's stars count toward, once, most
	// recently starred first, with its stars, and which retired rule the account starred in its place, if any. It
	// leaves each rule's CanonicalGroup nil. A star that counts toward no such rule, such as one on a rule retired
	// without a replacement, or in a library that lost its vetting, isn't listed.
	AccountStars(ctx context.Context, vetted []domain.LibraryKey, accountID int64) ([]views.StarredRule, error)
}

// Carts reads what checkout resolves a browser's cart against, as the web function does, writing nothing.
type Carts interface {
	// CartLibraries returns, from one snapshot of the catalog, the libraries items name, each matched by owner and name
	// without regard to case, that vetted holds or a listing names, in no order, each with the rules of the groups
	// items name of it, a rule's or a whole group's, as views.CartLibrary describes.
	CartLibraries(ctx context.Context, vetted []domain.LibraryKey, items []domain.CartItem) ([]views.CartLibrary, error)
}

// ListingConflict reports a repository that can't be listed because it already is, or is vetted.
type ListingConflict struct {
	// Vetted is true when the release's vetted list holds the library, and false when a listing names it.
	Vetted bool
	// Own is true when the listing is the account's own.
	Own bool
	// Checking is true when another account's listing names it, and the worker hasn't checked it yet, which that
	// listing asked for at RequestedAt.
	Checking    bool
	RequestedAt time.Time
	// Library is the library already in the catalog under that name, or the zero LibraryRef when the catalog doesn't
	// store it yet.
	Library views.LibraryRef
}

func (c *ListingConflict) Error() string {
	if c.Vetted {
		return "the library is vetted already"
	}
	return "the library is listed already"
}

// ErrListingNotFailed reports a retry of the account's listing whose last check didn't fail, such as from a page
// left open while it was checked.
var ErrListingNotFailed = errors.New("the listing's last check didn't fail")

// ErrAlreadyListed reports a repository another listing names.
var ErrAlreadyListed = errors.New("another listing names the repository")

// ErrAccountListingLimit reports an account that holds domain.MaxAccountListings unvetted listings.
var ErrAccountListingLimit = errors.New("the account holds as many unvetted listings as it may")

// ErrListingsFull reports that Rulemart holds domain.MaxUnvettedListings unvetted listings.
var ErrListingsFull = errors.New("Rulemart holds as many unvetted listings as it takes")

// ErrListingTooOften reports an account that listed or retried domain.MaxAccountListingRequests times in the last
// day.
var ErrListingTooOften = errors.New("the account asked for as many checks today as it may")

// ErrListingsBusy reports that every account together listed or retried domain.MaxListingRequestsPerHour times in the
// last hour.
var ErrListingsBusy = errors.New("Rulemart took as many listings this hour as it takes")

// Reader reads what the catalog's pages show. Each read sees one committed state of the catalog, so a page never
// mixes two ingestions. Reads across libraries find only the libraries in vetted; a library's own pages also find a
// library a listing names, and say whether it's vetted. Every current rule a read returns carries its stars, counted as
// Stars describes, from the same state.
type Reader interface {
	// Libraries returns the vetted libraries, and with unvetted, the libraries listings name too, ordered by owner and
	// name without regard to case, each saying whether it's vetted.
	Libraries(ctx context.Context, vetted []domain.LibraryKey, unvetted bool) ([]views.LibraryCard, error)
	// UnvettedLibraries returns the libraries listings name that vetted doesn't hold, ordered as Libraries orders them.
	UnvettedLibraries(ctx context.Context, vetted []domain.LibraryKey) ([]views.LibraryCard, error)
	// OwnerLibraries returns the vetted libraries whose owner is login, matched without regard to case, ordered as
	// Libraries orders them, each spelling the owner as the code host does. It's empty when the owner has none.
	OwnerLibraries(ctx context.Context, vetted []domain.LibraryKey, login string) ([]views.LibraryCard, error)
	// HomePage returns the vetted libraries, ordered by owner and name without regard to case, and their groups, as
	// Groups returns them.
	HomePage(ctx context.Context, vetted []domain.LibraryKey) ([]views.LibraryCard, []views.LibraryGroup, error)
	// LibraryPage returns the library owner/name, vetted or listed, matched without regard to case, or ErrNotFound.
	LibraryPage(ctx context.Context, vetted []domain.LibraryKey, owner, name string) (views.LibraryPage, error)
	// RulePage returns the rule at rulePath in the library owner/name, current or retired, both matched without
	// regard to case, as LibraryPage matches the library, with how every rule of the library was replaced, or
	// ErrNotFound. The page's Rule.Path is the library's spelling.
	RulePage(ctx context.Context, vetted []domain.LibraryKey, owner, name, rulePath string) (views.RulePage, error)
	// RuleComparison returns the rule's page, as RulePage does, with the text of its versions from and to, read only
	// when both are stored and hold at most maxBytes together, or ErrNotFound when either isn't a version of the rule.
	RuleComparison(ctx context.Context, vetted []domain.LibraryKey, owner, name, rulePath string, from, to coderules.RuleVersion, maxBytes int64) (views.RuleComparison, error)
	// LibraryHistory returns the library owner/name, matched as LibraryPage matches it, with its releases and
	// every rule's versions, or ErrNotFound.
	LibraryHistory(ctx context.Context, vetted []domain.LibraryKey, owner, name string) (views.LibraryHistory, error)
	// ReleaseComparison returns the library's history, as LibraryHistory does, and the text of each pair of versions
	// that pick chooses from it, keyed by the pair's key, read from the same snapshot: in pick's order, each pair read
	// only when both are stored and hold, with the pairs before it, at most maxBytes.
	ReleaseComparison(ctx context.Context, vetted []domain.LibraryKey, owner, name string, pick func(views.LibraryHistory) []views.VersionPair, maxBytes int64) (views.LibraryHistory, map[string]views.ComparedText, error)
	// Groups returns each group that holds current rules in a vetted library, and with unvetted, in a library a listing
	// names too, once for each library that holds it, in path order and then the library's owner and name, without
	// regard to case.
	Groups(ctx context.Context, vetted []domain.LibraryKey, unvetted bool) ([]views.LibraryGroup, error)
	// Rules returns one page of the list of rules list describes, after its filters and in its order: at most limit
	// rules, which must be at least 1, after the first skip. Its libraries are the vetted ones, and with list.Unvetted,
	// the ones listings name too. A list with a query holds the rules that match it, current and retired; one without
	// holds every current rule, and with list.Retired, every retired one too. Every order puts the current rules that
	// hold every word to find first, then the other current rules, then the retired rules, and each group's rules in a
	// tier together, in the order of the group's first. Each rule's stars are counted as Stars counts them; a retired
	// rule's, or one of a library that isn't vetted, are 0. The results also count the rules that pass the filters,
	// those the list holds before them by library, and its group's retired rules, whichever rules the page holds. It
	// leaves each row's CanonicalGroup nil.
	Rules(ctx context.Context, vetted []domain.LibraryKey, groups []domain.CanonicalGroup, list domain.RuleList, limit, skip int) (views.RuleResults, error)
	// Sitemap returns the vetted libraries, their current rules, and the groups that hold them, at most maxEntries
	// groups and rules in all: the first groups in ID order, then the first rules in owner, name, and ID order, from
	// one state of the catalog.
	Sitemap(ctx context.Context, vetted []domain.LibraryKey, maxEntries int) (views.Sitemap, error)
}

// ErrNotFound reports a library, rule, or rule version that isn't in the catalog, a library that's neither vetted nor
// listed, a rule to star or unstar that isn't a current rule of a vetted library, or an account's listing that it
// doesn't have.
var ErrNotFound = errors.New("not found")
