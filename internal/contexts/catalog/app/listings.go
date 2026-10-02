// List a library: an account lists a repository, the worker checks it, and its lister removes or retries it.

package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/jobs"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// What can keep an account from listing a repository.
var (
	// ErrInvalidRepository reports text that doesn't name a GitHub repository.
	ErrInvalidRepository = domain.ErrInvalidRepository
	// ErrAccountListingLimit reports an account that holds domain.MaxAccountListings unvetted listings.
	ErrAccountListingLimit = store.ErrAccountListingLimit
	// ErrListingsFull reports that Rulemart holds domain.MaxUnvettedListings unvetted listings.
	ErrListingsFull = store.ErrListingsFull
	// ErrListingTooOften reports an account that listed or retried domain.MaxAccountListingRequests times today.
	ErrListingTooOften = store.ErrListingTooOften
	// ErrListingsBusy reports that every account together listed or retried domain.MaxListingRequestsPerHour times in
	// the last hour.
	ErrListingsBusy = store.ErrListingsBusy
	// ErrNotQueued reports a listing that was added, or retried, but whose check couldn't be queued, so the worker
	// checks it at its next hourly poll.
	ErrNotQueued = errors.New("the listing's check wasn't queued")
)

// ListingConflict reports a repository that a listing names already, or that's vetted.
type ListingConflict = store.ListingConflict

// Queue sends a message to the worker's jobs queue.
type Queue interface {
	Send(ctx context.Context, body string) error
}

// Listings lists libraries for accounts, as the web function does. A listing is unvetted unless Vetted holds the
// repository it names.
type Listings struct {
	Store  store.Listings
	Vetted []domain.LibraryKey
	// Queue receives a job for each listing to check at once; nil leaves every listing for the hourly poll.
	Queue Queue
}

// Repository is a repository a lister names, as owner/name.
type Repository struct {
	Owner, Name string
}

// FullName returns the repository as owner/name.
func (r Repository) FullName() string { return r.Owner + "/" + r.Name }

// Check returns the repository text names, and whether the account may list it, without listing it: it fails with
// ErrInvalidRepository, a *ListingConflict, ErrAccountListingLimit, ErrListingsFull, ErrListingTooOften, or
// ErrListingsBusy. It doesn't ask the code host;
// the worker does, when it checks the listing.
func (l Listings) Check(ctx context.Context, accountID int64, text string) (Repository, error) {
	repo, err := parseRepository(text)
	if err != nil {
		return Repository{}, err
	}
	return repo, l.Store.CheckListing(ctx, l.Vetted, accountID, repo.Owner, repo.Name)
}

// List lists the repository text names for the account, after the checks Check makes, and queues its check. It
// returns the listing's ID. When only queueing failed, it returns the ID with an error that wraps ErrNotQueued: the
// listing waits for the next poll.
func (l Listings) List(ctx context.Context, accountID int64, text string) (int64, error) {
	repo, err := parseRepository(text)
	if err != nil {
		return 0, err
	}
	id, err := l.Store.CreateListing(ctx, l.Vetted, accountID, repo.Owner, repo.Name)
	if err != nil {
		return 0, err
	}
	return id, l.queue(ctx, id)
}

// AccountListings returns the account's listings, newest first.
func (l Listings) AccountListings(ctx context.Context, accountID int64) ([]views.AccountListing, error) {
	return l.Store.AccountListings(ctx, l.Vetted, accountID)
}

// Remove removes the account's listing id, or fails with ErrNotFound when the account has no such listing.
func (l Listings) Remove(ctx context.Context, accountID, id int64) error {
	return l.Store.RemoveListing(ctx, accountID, id)
}

// Retry asks the worker to check the account's listing id again, and queues the check, as List does. It fails with
// ErrNotFound when the account has no such listing, or its last check didn't fail, and with ErrListingTooOften or
// ErrListingsBusy as Check does.
func (l Listings) Retry(ctx context.Context, accountID, id int64) error {
	if err := l.Store.RetryListing(ctx, accountID, id); err != nil {
		return err
	}
	return l.queue(ctx, id)
}

// queue sends the listing's check to the queue, if there is one.
func (l Listings) queue(ctx context.Context, id int64) error {
	if l.Queue == nil {
		return nil
	}
	if err := l.Queue.Send(ctx, jobs.CheckListing(id)); err != nil {
		return fmt.Errorf("queue check of listing id=%d: %w: %v", id, ErrNotQueued, err)
	}
	return nil
}

// parseRepository returns the GitHub repository text names, or fails with ErrInvalidRepository.
func parseRepository(text string) (Repository, error) {
	owner, name, err := domain.ParseListedRepository(text)
	if err != nil {
		return Repository{}, err
	}
	return Repository{Owner: owner, Name: name}, nil
}

// ListingOutcome is what checking a listing did.
type ListingOutcome string

const (
	// ListingSkipped is a check of a listing that's gone, or that names a vetted library, which its own job updates.
	ListingSkipped ListingOutcome = "skipped"
	// ListingUnchanged is a check that found the library's release tags as the catalog stored them.
	ListingUnchanged ListingOutcome = "unchanged"
	// ListingIngested is a check that ingested the library.
	ListingIngested ListingOutcome = "ingested"
	// ListingRefused is a check that failed because of the repository, which it recorded for the lister.
	ListingRefused ListingOutcome = "refused"
)

// ListingCheck is what one check of a listing did.
type ListingCheck struct {
	Outcome ListingOutcome
	// Listing is the listing checked, resolved when the check resolved it, or the zero Listing when it's gone.
	Listing domain.Listing
	// Failure is why a refused check failed, as the listing records it.
	Failure string
	// Update is what updating the library did, when the check got that far.
	Update Update
}

// LibraryError is an ingestion's failure that the library's repository caused, such as a missing release tag, an
// invalid record, or more content than ingestion's limits allow, rather than Rulemart or its database.
type LibraryError struct {
	Err error
	// Reason says what went wrong in words the listing's lister can act on: Err's text without where it happened, or a
	// sentence of its own.
	Reason string
}

func (e *LibraryError) Error() string { return e.Err.Error() }
func (e *LibraryError) Unwrap() error { return e.Err }

// recordTimeout bounds recording a check's outcome, which runs even after the check ran out of time.
const recordTimeout = 5 * time.Second

// CheckListing checks the listing id: it looks the repository up on its code host, the first time, then updates the
// library as Update does, ingesting it when its release tags aren't the ones the catalog stored, and records that the
// check succeeded. A listing that names a vetted library is skipped, since the library's own job updates it. A failure
// the repository caused, such as a missing or private repository, a repository another listing names, or one that
// isn't a Code Rules library, is recorded on the listing for its lister, and the check succeeds as refused. Any other
// failure, such as the database's or the code host's API's, fails the check, so it's tried again.
func (in Ingester) CheckListing(ctx context.Context, vetted []domain.LibraryKey, id int64) (ListingCheck, error) {
	check, err := in.checkListing(ctx, vetted, id)
	var libraryErr *LibraryError
	if errors.As(err, &libraryErr) {
		check.Outcome, check.Failure = ListingRefused, domain.Failure(cmp.Or(libraryErr.Reason, libraryErr.Error()))
		err = in.recordCheck(ctx, id, check.Failure)
	} else if err == nil && check.Listing.ID != 0 {
		err = in.recordCheck(ctx, id, "")
	}
	if err != nil {
		return ListingCheck{Listing: check.Listing}, fmt.Errorf("check listing id=%d: %v", id, err)
	}
	return check, nil
}

func (in Ingester) checkListing(ctx context.Context, vetted []domain.LibraryKey, id int64) (ListingCheck, error) {
	listing, found, err := in.Store.Listing(ctx, id)
	if err != nil || !found {
		return ListingCheck{Outcome: ListingSkipped}, err
	}
	check := ListingCheck{Listing: listing}
	var repo *domain.Repository
	if !listing.Resolved() {
		found, err := in.Repositories.Repository(ctx, listing.Owner, listing.Name)
		if err != nil {
			return check, libraryError(err, domain.ErrNoPublicRepository,
				"GitHub has no public repository by this name. Check its owner and name, and that it's public.")
		}
		library := domain.LibraryKey{Host: found.Host, RepositoryID: found.ID}
		err = in.Store.ResolveListing(ctx, id, library)
		if errors.Is(err, store.ErrAlreadyListed) {
			reason := fmt.Sprintf("Another listing names this repository, which GitHub calls %s now.", found.FullName())
			err = &LibraryError{Err: fmt.Errorf("resolve listing id=%d: %v", id, err), Reason: reason}
		}
		if err != nil {
			return check, err
		}
		check.Listing.Library, repo = library, &found
	}
	if slices.Contains(vetted, check.Listing.Library) {
		// The library's own job updates it.
		check.Outcome = ListingSkipped
		return check, nil
	}
	check.Update, err = in.updateFrom(ctx, check.Listing.Library, repo)
	check.Outcome = ListingUnchanged
	if check.Update.Ingested {
		check.Outcome = ListingIngested
	}
	return check, err
}

// libraryError returns err as a *LibraryError for reason when it's target, which the repository caused, and as it is
// otherwise.
func libraryError(err, target error, reason string) error {
	if errors.Is(err, target) {
		return &LibraryError{Err: err, Reason: reason}
	}
	return err
}

// recordCheck records how a check of the listing id went, even when ctx has ended, within recordTimeout.
func (in Ingester) recordCheck(ctx context.Context, id int64, failure string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordTimeout)
	defer cancel()
	return in.Store.RecordListingCheck(ctx, id, failure)
}

// ListingsToCheck returns the IDs of the listings the hourly poll checks: every one vetted doesn't hold, except one
// whose check failed before its library ever ingested, which waits for its lister to try again.
func (in Ingester) ListingsToCheck(ctx context.Context, vetted []domain.LibraryKey) ([]int64, error) {
	return in.Store.ListingsToCheck(ctx, vetted)
}
