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

// Stars stars and unstars libraries for accounts, and lists an account's stars, as the web function does. Only a
// library vetted holds can be starred.
type Stars interface {
	// Star stars the library owner/name for the account, matched without regard to case, and returns it as the code
	// host spells it now. It fails with ErrNotFound when vetted holds no library by that name. A library the account
	// starred already keeps its one star.
	Star(ctx context.Context, vetted []domain.LibraryKey, accountID int64, owner, name string) (views.LibraryRef, error)
	// Unstar removes the account's star from the library owner/name, matched as Star matches it, vetted or not, and
	// does nothing when the account hasn't starred it.
	Unstar(ctx context.Context, accountID int64, owner, name string) error
	// Starred reports whether the account starred the library owner/name, matched as Star matches it.
	Starred(ctx context.Context, accountID int64, owner, name string) (bool, error)
	// AccountStars returns the libraries the account starred, most recently starred first, each with whether vetted
	// holds it, and whether a listing names it.
	AccountStars(ctx context.Context, vetted []domain.LibraryKey, accountID int64) ([]views.StarredLibrary, error)
}

// Cart adds and removes an account's cart items, and reads its cart, as the web function does. An item can be added
// from a library vetted holds, or one a listing names.
type Cart interface {
	// AddToCart adds item to the account's cart, its library and ID matched without regard to case, and returns it as
	// the library spells it. confirmed records that the visitor confirmed adding it from a library vetted doesn't
	// hold. Adding an item the cart holds keeps one, and records a confirmation it lacked. It fails with ErrNotFound
	// when the library isn't vetted or listed, or has no such group with current rules, or current rule; with
	// ErrUnvettedNotConfirmed when vetted doesn't hold the library and confirmed is false; and with ErrCartFull when
	// the cart holds domain.MaxCartItems other items.
	AddToCart(ctx context.Context, vetted []domain.LibraryKey, accountID int64, item domain.CartItem, confirmed bool) (domain.CartItem, error)
	// RemoveFromCart removes item from the account's cart, matched as AddToCart matches it, and does nothing when the
	// cart doesn't hold it.
	RemoveFromCart(ctx context.Context, accountID int64, item domain.CartItem) error
	// EmptyCart removes every item from the account's cart.
	EmptyCart(ctx context.Context, accountID int64) error
	// CountCart returns how many items the account's cart holds.
	CountCart(ctx context.Context, accountID int64) (int, error)
	// LibraryCartItems returns the account's items from the library owner/name, matched without regard to case.
	LibraryCartItems(ctx context.Context, accountID int64, owner, name string) ([]domain.CartItem, error)
	// Cart returns the account's cart from one snapshot of the catalog: its libraries, in owner and name order, each
	// with whether vetted holds it, and its items, as views.CartLibrary orders them, each with its group, title, and
	// rules as the catalog has them now. It leaves each item's CanonicalGroup nil, and its State empty.
	Cart(ctx context.Context, vetted []domain.LibraryKey, accountID int64) ([]views.CartLibrary, error)
}

// ErrUnvettedNotConfirmed reports an item to add to a cart from a library Rulemart doesn't vet, which the visitor
// didn't confirm.
var ErrUnvettedNotConfirmed = errors.New("adding from an unvetted library needs confirming")

// ErrCartFull reports a cart that holds domain.MaxCartItems items.
var ErrCartFull = errors.New("the cart holds as many items as it may")

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
// library a listing names, and say whether it's vetted.
type Reader interface {
	// Libraries returns the vetted libraries, ordered by owner and name without regard to case.
	Libraries(ctx context.Context, vetted []domain.LibraryKey) ([]views.LibraryCard, error)
	// UnvettedLibraries returns the libraries listings name that vetted doesn't hold, ordered as Libraries orders them.
	UnvettedLibraries(ctx context.Context, vetted []domain.LibraryKey) ([]views.LibraryCard, error)
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
	// Groups returns each group that holds current rules in a vetted library, once for each library that holds it,
	// in path order and then the library's owner and name, without regard to case.
	Groups(ctx context.Context, vetted []domain.LibraryKey) ([]views.LibraryGroup, error)
	// GroupRules returns the current rules of the group at path in each vetted library that holds it, by library in
	// owner and name order, and each library's in title order. It's empty when no vetted library holds the group.
	GroupRules(ctx context.Context, vetted []domain.LibraryKey, path string) ([]views.GroupLibrary, error)
	// Search returns one page of the vetted libraries' current rules that match query, best first: at most limit of
	// them, which must be at least 1, after the best skip, with how many matched in all. A rule matches when it holds
	// at least one of the query's terms to find and none of those to leave out, by its title, reading guidance,
	// impact description, body, library name, and group's name: the name groups gives a canonical group, and the name
	// part of any group's ID. A term that joins words with -, /, or : also matches the rule's ID, its group's, and its
	// library's owner and name. Rules that hold more of the terms, in more telling places, come first. It leaves each
	// result's CanonicalGroup nil.
	Search(ctx context.Context, vetted []domain.LibraryKey, groups []domain.CanonicalGroup, query domain.SearchQuery, limit, skip int) (views.SearchResults, error)
}

// ErrNotFound reports a library, rule, or rule version that isn't in the catalog, a library that's neither vetted nor
// listed, a library to star that isn't vetted, an account's listing that it doesn't have, or an item to add to a cart
// that its library doesn't have.
var ErrNotFound = errors.New("not found")
