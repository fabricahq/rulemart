// Store stars: star and unstar libraries for accounts, and list an account's stars, for the web function.

package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres/generated/catalogdb"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

var _ store.Stars = (*Store)(nil)

// Star stars the vetted library owner/name for the account, as store.Stars describes, in one statement, so it's safe
// to repeat after a failed connection.
func (s *Store) Star(ctx context.Context, vetted []domain.LibraryKey, accountID int64, owner, name string) (views.LibraryRef, error) {
	var ref views.LibraryRef
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		row, err := catalogdb.New(pool).StarLibrary(ctx, catalogdb.StarLibraryParams{
			Host: domain.GitHub, Owner: owner, Name: name, Vetted: vettedKeys(vetted), AccountID: accountID,
		})
		ref = views.LibraryRef{Owner: row.Owner, Name: row.Name, OwnerAvatarURL: row.OwnerAvatarUrl}
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return views.LibraryRef{}, fmt.Errorf("star library=%q accountID=%d: %w", owner+"/"+name, accountID, store.ErrNotFound)
	}
	if err != nil {
		return views.LibraryRef{}, fmt.Errorf("star library=%q accountID=%d: %v", owner+"/"+name, accountID, err)
	}
	return ref, nil
}

// Unstar removes the account's star from the library owner/name, as store.Stars describes, in one statement.
func (s *Store) Unstar(ctx context.Context, accountID int64, owner, name string) error {
	var libraries int64
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		var err error
		libraries, err = catalogdb.New(pool).UnstarLibrary(ctx, catalogdb.UnstarLibraryParams{
			AccountID: accountID, Host: domain.GitHub, Owner: owner, Name: name,
		})
		return err
	})
	if err == nil && libraries == 0 {
		return fmt.Errorf("unstar library=%q accountID=%d: %w", owner+"/"+name, accountID, store.ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("unstar library=%q accountID=%d: %v", owner+"/"+name, accountID, err)
	}
	return nil
}

// Starred reports whether the account starred the library owner/name, as store.Stars describes.
func (s *Store) Starred(ctx context.Context, accountID int64, owner, name string) (bool, error) {
	var starred bool
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		var err error
		starred, err = catalogdb.New(pool).IsStarred(ctx, catalogdb.IsStarredParams{
			AccountID: accountID, Host: domain.GitHub, Owner: owner, Name: name,
		})
		return err
	})
	if err != nil {
		return false, fmt.Errorf("read star library=%q accountID=%d: %v", owner+"/"+name, accountID, err)
	}
	return starred, nil
}

// AccountStars returns the libraries the account starred, as store.Stars describes, from one snapshot.
func (s *Store) AccountStars(ctx context.Context, vetted []domain.LibraryKey, accountID int64) ([]views.StarredLibrary, error) {
	var stars []views.StarredLibrary
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		rows, err := q.ListAccountStars(ctx, catalogdb.ListAccountStarsParams{Vetted: vettedKeys(vetted), AccountID: accountID})
		if err != nil {
			return err
		}
		stars = make([]views.StarredLibrary, len(rows))
		for i, row := range rows {
			stars[i] = views.StarredLibrary{
				Library: views.LibraryCard{
					Owner: row.Owner, Name: row.Name, Description: row.Description, OwnerAvatarURL: row.OwnerAvatarUrl,
					Rules: int(row.RuleCount), Stars: int(row.StarCount),
				},
				Vetted: row.Vetted, Listed: row.Listed, StarredAt: row.StarredAt.Time,
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load stars accountID=%d: %v", accountID, err)
	}
	return stars, nil
}
