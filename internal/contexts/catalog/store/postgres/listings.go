// Store listings: add, read, remove, and retry an account's listings for the web function, and resolve and record
// checks of them for the worker.

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres/generated/catalogdb"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

var _ store.Listings = (*Store)(nil)

// CheckListing reports whether the account may list the repository owner/name, as store.Listings describes, from one
// snapshot of the listings.
func (s *Store) CheckListing(ctx context.Context, vetted []domain.LibraryKey, accountID int64, owner, name string) error {
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		return checkListing(ctx, q, vetted, accountID, owner, name)
	})
	if err != nil {
		return fmt.Errorf("check listing repository=%q: %w", owner+"/"+name, err)
	}
	return nil
}

// CreateListing lists the repository owner/name for the account, as store.Listings describes. It holds the lock every
// listing takes while it checks and adds, so two listings at once can't both pass a limit, and it's safe to repeat
// after a failed connection, since the transaction either committed nothing or everything.
func (s *Store) CreateListing(ctx context.Context, vetted []domain.LibraryKey, accountID int64, owner, name string) (int64, error) {
	var id int64
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			q := catalogdb.New(tx)
			if err := q.LockListings(ctx); err != nil {
				return fmt.Errorf("lock listings: %v", err)
			}
			if _, err := q.SupersedeFailedListing(ctx, catalogdb.SupersedeFailedListingParams{
				Host: domain.GitHub, Owner: owner, Name: name, AccountID: accountID,
			}); err != nil {
				return fmt.Errorf("supersede a failed listing: %v", err)
			}
			if err := checkListing(ctx, q, vetted, accountID, owner, name); err != nil {
				return err
			}
			var err error
			id, err = q.CreateListing(ctx, catalogdb.CreateListingParams{AccountID: accountID, Host: domain.GitHub, Owner: owner, Name: name})
			if isUniqueViolation(err) {
				return &store.ListingConflict{}
			}
			if err != nil {
				return err
			}
			return q.RecordListingRequest(ctx, accountID)
		})
	})
	if err != nil {
		return 0, fmt.Errorf("list repository=%q accountID=%d: %w", owner+"/"+name, accountID, err)
	}
	return id, nil
}

// checkListing returns why the account can't list the repository owner/name, or nil when it can.
func checkListing(ctx context.Context, q *catalogdb.Queries, vetted []domain.LibraryKey, accountID int64, owner, name string) error {
	keys := vettedKeys(vetted)
	conflict, err := q.FindListingConflict(ctx, catalogdb.FindListingConflictParams{Host: domain.GitHub, Owner: owner, Name: name, Vetted: keys})
	if err != nil {
		return fmt.Errorf("find listings by name: %v", err)
	}
	if c := listingConflict(conflict, accountID); c != nil {
		return c
	}
	counts, err := q.CountUnvettedListings(ctx, catalogdb.CountUnvettedListingsParams{AccountID: accountID, Vetted: keys})
	switch {
	case err != nil:
		return fmt.Errorf("count listings: %v", err)
	case counts.AccountListings >= domain.MaxAccountListings:
		return store.ErrAccountListingLimit
	case counts.AllListings >= domain.MaxUnvettedListings:
		return store.ErrListingsFull
	}
	return checkRequests(ctx, q, accountID)
}

// checkRequests returns why the account can't ask the worker for another check now, or nil when it can.
func checkRequests(ctx context.Context, q *catalogdb.Queries, accountID int64) error {
	requests, err := q.CountListingRequests(ctx, accountID)
	switch {
	case err != nil:
		return fmt.Errorf("count listing requests: %v", err)
	case requests.AccountRequests >= domain.MaxAccountListingRequests:
		return store.ErrListingTooOften
	case requests.AllRequests >= domain.MaxListingRequestsPerHour:
		return store.ErrListingsBusy
	}
	return nil
}

// listingConflict returns what keeps the account from listing a repository, by what FindListingConflict found, or nil
// when nothing does: no listing or vetted library has its name, or only another account's listing whose check
// failed before its library ever ingested, which listing removes.
func listingConflict(found catalogdb.FindListingConflictRow, accountID int64) *store.ListingConflict {
	var library views.LibraryRef
	if found.LibraryOwner != "" {
		library = views.LibraryRef{Owner: found.LibraryOwner, Name: found.LibraryName}
	}
	switch {
	case found.Vetted:
		return &store.ListingConflict{Vetted: true, Library: library}
	case (found.Named && found.NamedAccountID == accountID) || (found.Held && found.HolderAccountID == accountID):
		return &store.ListingConflict{Own: true, Library: library}
	case found.Held:
		return &store.ListingConflict{Library: library}
	case found.Named && !found.NamedFailed:
		return &store.ListingConflict{Checking: true, RequestedAt: found.NamedRequestedAt.Time}
	}
	return nil
}

// AccountListings returns the account's listings, newest first.
func (s *Store) AccountListings(ctx context.Context, vetted []domain.LibraryKey, accountID int64) ([]views.AccountListing, error) {
	var listings []views.AccountListing
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		rows, err := q.ListAccountListings(ctx, catalogdb.ListAccountListingsParams{AccountID: accountID, Vetted: vettedKeys(vetted)})
		if err != nil {
			return err
		}
		listings = make([]views.AccountListing, len(rows))
		for i, row := range rows {
			listings[i] = views.AccountListing{
				ID: row.ID, Owner: row.Owner, Name: row.Name, RepositoryID: row.HostRepositoryID,
				State:   domain.StateOf(row.Vetted, row.Ingested, row.Failure.Valid),
				Failure: row.Failure.String, ListedAt: row.CreatedAt.Time, RequestedAt: row.RequestedAt.Time,
				CheckedAt: row.CheckedAt.Time,
			}
			if row.Ingested {
				listings[i].Library = views.LibraryRef{Owner: row.LibraryOwner, Name: row.LibraryName, OwnerAvatarURL: row.LibraryAvatarUrl}
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load listings accountID=%d: %v", accountID, err)
	}
	return listings, nil
}

// RemoveListing removes the account's listing id, or fails with store.ErrNotFound.
func (s *Store) RemoveListing(ctx context.Context, accountID, id int64) error {
	return s.changeListing(ctx, "remove", accountID, id, func(q *catalogdb.Queries) (int64, error) {
		return q.DeleteListing(ctx, catalogdb.DeleteListingParams{ID: id, AccountID: accountID})
	})
}

// RetryListing asks the worker to check the account's listing id again, or fails with store.ErrNotFound when the
// account has no such listing, store.ErrListingNotFailed when its last check didn't fail, or as checkRequests does. It holds the lock every
// listing takes, as CreateListing does.
func (s *Store) RetryListing(ctx context.Context, accountID, id int64) error {
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			q := catalogdb.New(tx)
			if err := q.LockListings(ctx); err != nil {
				return fmt.Errorf("lock listings: %v", err)
			}
			retried, err := q.RetryListing(ctx, catalogdb.RetryListingParams{ID: id, AccountID: accountID})
			if err != nil {
				return err
			}
			if retried == 0 {
				has, err := q.HasListing(ctx, catalogdb.HasListingParams{ID: id, AccountID: accountID})
				switch {
				case err != nil:
					return err
				case has:
					return store.ErrListingNotFailed
				}
				return store.ErrNotFound
			}
			if err := checkRequests(ctx, q, accountID); err != nil {
				return err
			}
			return q.RecordListingRequest(ctx, accountID)
		})
	})
	if err != nil {
		return fmt.Errorf("retry listing id=%d accountID=%d: %w", id, accountID, err)
	}
	return nil
}

// changeListing runs change, which returns how many listings it changed, and fails with store.ErrNotFound when it
// changed none. what names the change in errors.
func (s *Store) changeListing(ctx context.Context, what string, accountID, id int64, change func(*catalogdb.Queries) (int64, error)) error {
	var changed int64
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		var err error
		changed, err = change(catalogdb.New(pool))
		return err
	})
	switch {
	case err != nil:
		return fmt.Errorf("%s listing id=%d accountID=%d: %v", what, id, accountID, err)
	case changed == 0:
		return fmt.Errorf("%s listing id=%d accountID=%d: %w", what, id, accountID, store.ErrNotFound)
	}
	return nil
}

// Listing returns the listing id as the worker checks it, or found false when there's no such listing.
func (s *Store) Listing(ctx context.Context, id int64) (domain.Listing, bool, error) {
	var row catalogdb.GetListingRow
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		var err error
		row, err = catalogdb.New(pool).GetListing(ctx, id)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Listing{}, false, nil
	}
	if err != nil {
		return domain.Listing{}, false, fmt.Errorf("read listing id=%d: %v", id, err)
	}
	return domain.Listing{
		ID: row.ID, Owner: row.Owner, Name: row.Name, Ingested: row.Ingested, RequestedAt: row.RequestedAt.Time,
		Library: domain.LibraryKey{Host: row.Host, RepositoryID: row.HostRepositoryID},
	}, true, nil
}

// ResolveListing records that the listing id names the repository library, or fails with store.ErrAlreadyListed.
func (s *Store) ResolveListing(ctx context.Context, id int64, library domain.LibraryKey) error {
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		_, err := catalogdb.New(pool).ResolveListing(ctx, catalogdb.ResolveListingParams{ID: id, HostRepositoryID: library.RepositoryID})
		return err
	})
	if isUniqueViolation(err) {
		err = store.ErrAlreadyListed
	}
	if err != nil {
		return fmt.Errorf("resolve listing id=%d to repository=%s: %w", id, library.RepositoryID, err)
	}
	return nil
}

// RecordListingCheck records that a check of the listing id, started when it was requested at requestedAt, finished
// now, and why it failed, or that it succeeded when failure is empty. It reports false, and records nothing, when the
// listing asked for another check since, or is gone.
func (s *Store) RecordListingCheck(ctx context.Context, id int64, requestedAt time.Time, failure string) (bool, error) {
	var recorded int64
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		var err error
		recorded, err = catalogdb.New(pool).RecordListingCheck(ctx, catalogdb.RecordListingCheckParams{
			ID: id, RequestedAt: pgtype.Timestamptz{Time: requestedAt, Valid: true}, Failure: pgtype.Text{String: failure, Valid: failure != ""},
		})
		return err
	})
	if err != nil {
		return false, fmt.Errorf("record check of listing id=%d: %v", id, err)
	}
	return recorded > 0, nil
}

// ListingsToCheck returns the IDs of the listings the hourly poll checks, in the order they were listed.
func (s *Store) ListingsToCheck(ctx context.Context, vetted []domain.LibraryKey) ([]int64, error) {
	var ids []int64
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		var err error
		ids, err = catalogdb.New(pool).ListListingsToCheck(ctx, vettedKeys(vetted))
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list listings to check: %v", err)
	}
	return ids, nil
}

// isUniqueViolation reports whether err is Postgres refusing a row that a unique index already holds.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
