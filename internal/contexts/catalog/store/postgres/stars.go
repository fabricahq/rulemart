// Store stars: star and unstar rules for accounts, list the rules an account's stars count toward, and count each
// rule's stars for the pages, for the web function.

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

// Star stars the current rule at rulePath in the vetted library owner/name for the account, as store.Stars describes,
// in one statement, so it's safe to repeat after a failed connection.
func (s *Store) Star(ctx context.Context, vetted []domain.LibraryKey, accountID int64, owner, name, rulePath string) error {
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		_, err := catalogdb.New(pool).StarRule(ctx, catalogdb.StarRuleParams{
			Host: domain.GitHub, Owner: owner, Name: name, Vetted: vettedKeys(vetted), Path: rulePath, AccountID: accountID,
		})
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("star rule=%q accountID=%d: %w", owner+"/"+name+"/"+rulePath, accountID, store.ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("star rule=%q accountID=%d: %v", owner+"/"+name+"/"+rulePath, accountID, err)
	}
	return nil
}

// Unstar removes the account's stars that count toward the rule, as store.Stars describes, in one statement.
func (s *Store) Unstar(ctx context.Context, vetted []domain.LibraryKey, accountID int64, owner, name, rulePath string) error {
	var rules int64
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		var err error
		rules, err = catalogdb.New(pool).UnstarRule(ctx, catalogdb.UnstarRuleParams{
			Host: domain.GitHub, Owner: owner, Name: name, Vetted: vettedKeys(vetted), Path: rulePath,
			MaxReplacements: domain.MaxReplacements, AccountID: accountID,
		})
		return err
	})
	if err == nil && rules == 0 {
		return fmt.Errorf("unstar rule=%q accountID=%d: %w", owner+"/"+name+"/"+rulePath, accountID, store.ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("unstar rule=%q accountID=%d: %v", owner+"/"+name+"/"+rulePath, accountID, err)
	}
	return nil
}

// Starred reports whether one of the account's stars counts toward the rule, as store.Stars describes.
func (s *Store) Starred(ctx context.Context, accountID int64, owner, name, rulePath string) (bool, error) {
	var starred bool
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		var err error
		starred, err = catalogdb.New(pool).IsRuleStarred(ctx, catalogdb.IsRuleStarredParams{
			AccountID: accountID, Host: domain.GitHub, Owner: owner, Name: name, Path: rulePath,
			MaxReplacements: domain.MaxReplacements,
		})
		return err
	})
	if err != nil {
		return false, fmt.Errorf("read star rule=%q accountID=%d: %v", owner+"/"+name+"/"+rulePath, accountID, err)
	}
	return starred, nil
}

// AccountStars returns the rules the account's stars count toward, as store.Stars describes, from one snapshot.
func (s *Store) AccountStars(ctx context.Context, vetted []domain.LibraryKey, accountID int64) ([]views.StarredRule, error) {
	var starred []views.StarredRule
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		rows, err := q.ListAccountRuleStars(ctx, catalogdb.ListAccountRuleStarsParams{
			Vetted: vettedKeys(vetted), AccountID: accountID, MaxReplacements: domain.MaxReplacements,
		})
		if err != nil {
			return err
		}
		ids := make([]int64, len(rows))
		for i, row := range rows {
			ids[i] = row.ID
		}
		stars, err := ruleStars(ctx, q, ids)
		if err != nil {
			return err
		}
		starred = make([]views.StarredRule, len(rows))
		for i, row := range rows {
			starred[i] = views.StarredRule{
				Library: libraryRef(row.Owner, row.Name, row.OwnerAvatarUrl),
				Rule: views.RuleCard{
					Path: row.Path, Group: row.GroupPath, Title: row.Title, Impact: row.Impact,
					Version: version(row.Major, row.Minor, row.Patch), Stars: stars[row.ID],
				},
				StarredAs: row.StarredAs, StarredAt: row.StarredAt.Time,
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load stars accountID=%d: %v", accountID, err)
	}
	return starred, nil
}

// ruleStars returns how many accounts' stars count toward each of the rules ids names, by id, as store.Stars counts
// them; a rule without stars is missing, so it reads as 0.
func ruleStars(ctx context.Context, q *catalogdb.Queries, ids []int64) (map[int64]int, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := q.CountRuleStars(ctx, catalogdb.CountRuleStarsParams{RuleIds: ids, MaxReplacements: domain.MaxReplacements})
	if err != nil {
		return nil, fmt.Errorf("count rule stars: %v", err)
	}
	stars := make(map[int64]int, len(rows))
	for _, row := range rows {
		stars[row.RuleID] = int(row.Stars)
	}
	return stars, nil
}
