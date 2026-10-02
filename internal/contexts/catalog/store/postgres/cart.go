// Store carts: add and remove an account's cart items, and read its cart with the catalog, for the web function.

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

var _ store.Cart = (*Store)(nil)

// AddToCart adds item to the account's cart, as store.Cart describes. It holds the account's row while it checks,
// folds, and adds, so two items added at once can't both take the cart's last place, and it's safe to repeat after a failed
// connection, since the transaction either committed nothing or everything.
func (s *Store) AddToCart(ctx context.Context, vetted []domain.LibraryKey, accountID int64, item domain.CartItem, confirmed bool) (domain.CartItem, error) {
	var added domain.CartItem
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			q := catalogdb.New(tx)
			if _, err := q.LockCart(ctx, accountID); errors.Is(err, pgx.ErrNoRows) {
				return store.ErrNotFound
			} else if err != nil {
				return fmt.Errorf("lock the cart: %v", err)
			}
			target, err := q.FindCartTarget(ctx, catalogdb.FindCartTargetParams{
				Vetted: vettedKeys(vetted), Kind: string(item.Kind), Path: item.Path,
				Host: domain.GitHub, Owner: item.Owner, Name: item.Name,
			})
			switch {
			case errors.Is(err, pgx.ErrNoRows) || (err == nil && !target.Found):
				return store.ErrNotFound
			case err != nil:
				return fmt.Errorf("find the item: %v", err)
			case !target.Vetted && !confirmed:
				return store.ErrUnvettedNotConfirmed
			}
			added = domain.CartItem{Owner: target.Owner, Name: target.Name, Kind: item.Kind, Path: target.Path}
			covered, err := q.CartCovers(ctx, catalogdb.CartCoversParams{
				AccountID: accountID, LibraryID: target.LibraryID, Kind: string(item.Kind), GroupPath: added.Group(),
			})
			if err != nil {
				return fmt.Errorf("find what covers the item: %v", err)
			}
			if covered {
				return nil
			}
			if err := q.FoldCartItems(ctx, catalogdb.FoldCartItemsParams{
				AccountID: accountID, LibraryID: target.LibraryID, Kind: string(item.Kind), Path: target.Path,
			}); err != nil {
				return fmt.Errorf("remove what the item covers: %v", err)
			}
			key := catalogdb.CartHoldsParams{AccountID: accountID, LibraryID: target.LibraryID, Kind: string(item.Kind), Path: target.Path}
			holds, err := q.CartHolds(ctx, key)
			if err != nil {
				return fmt.Errorf("find the item in the cart: %v", err)
			}
			if !holds {
				count, err := q.CountCartItems(ctx, accountID)
				if err != nil {
					return fmt.Errorf("count the cart's items: %v", err)
				}
				if count >= domain.MaxCartItems {
					return store.ErrCartFull
				}
			}
			return q.AddCartItem(ctx, catalogdb.AddCartItemParams{
				AccountID: accountID, LibraryID: target.LibraryID, Kind: key.Kind, Path: key.Path,
				Confirmed: confirmed && !target.Vetted,
			})
		})
	})
	if err != nil {
		return domain.CartItem{}, fmt.Errorf("add to cart library=%q kind=%s path=%q accountID=%d: %w", item.FullName(), item.Kind, item.Path, accountID, cartError(err))
	}
	return added, nil
}

// cartError returns err as callers may see it: the store's own errors, which callers act on, as they are, and any
// other, such as a driver's, only as text.
func cartError(err error) error {
	for _, contract := range []error{store.ErrNotFound, store.ErrUnvettedNotConfirmed, store.ErrCartFull} {
		if errors.Is(err, contract) {
			return contract
		}
	}
	return errors.New(err.Error())
}

// RemoveFromCart removes item from the account's cart, as store.Cart describes.
func (s *Store) RemoveFromCart(ctx context.Context, accountID int64, item domain.CartItem) error {
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		return catalogdb.New(pool).RemoveCartItem(ctx, catalogdb.RemoveCartItemParams{
			AccountID: accountID, Host: domain.GitHub, Owner: item.Owner, Name: item.Name, Kind: string(item.Kind), Path: item.Path,
		})
	})
	if err != nil {
		return fmt.Errorf("remove from cart library=%q kind=%s path=%q accountID=%d: %v", item.FullName(), item.Kind, item.Path, accountID, err)
	}
	return nil
}

// EmptyCart removes every item from the account's cart.
func (s *Store) EmptyCart(ctx context.Context, accountID int64) error {
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		return catalogdb.New(pool).EmptyCart(ctx, accountID)
	})
	if err != nil {
		return fmt.Errorf("empty cart accountID=%d: %v", accountID, err)
	}
	return nil
}

// HeldCartItems returns the account's items, as store.Cart describes.
func (s *Store) HeldCartItems(ctx context.Context, accountID int64) ([]views.HeldCartItem, error) {
	var items []views.HeldCartItem
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		rows, err := catalogdb.New(pool).ListHeldCartItems(ctx, accountID)
		items = make([]views.HeldCartItem, len(rows))
		for i, row := range rows {
			items[i] = views.HeldCartItem{
				Item:      domain.CartItem{Owner: row.Owner, Name: row.Name, Kind: domain.CartItemKind(row.Kind), Path: row.Path},
				Confirmed: row.Confirmed,
			}
		}
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("read cart accountID=%d: %v", accountID, err)
	}
	return items, nil
}

// Cart returns the account's cart from one snapshot of the catalog, as store.Cart describes.
func (s *Store) Cart(ctx context.Context, vetted []domain.LibraryKey, accountID int64) ([]views.CartLibrary, error) {
	var libraries []views.CartLibrary
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		rows, err := q.ListCartItems(ctx, catalogdb.ListCartItemsParams{Vetted: vettedKeys(vetted), AccountID: accountID})
		if err != nil {
			return err
		}
		libraries = nil
		for _, row := range rows {
			ref := views.LibraryRef{Owner: row.Owner, Name: row.Name, OwnerAvatarURL: row.OwnerAvatarUrl}
			if n := len(libraries); n == 0 || libraries[n-1].Library != ref {
				libraries = append(libraries, views.CartLibrary{
					Library: ref, Vetted: row.Vetted, Listed: row.Listed, LatestRelease: int(row.LatestRelease),
				})
			}
			item := views.CartItem{
				Item:      domain.CartItem{Owner: row.Owner, Name: row.Name, Kind: domain.CartItemKind(row.Kind), Path: row.Path},
				Confirmed: row.Confirmed, AddedAt: row.AddedAt.Time,
			}
			switch item.Item.Kind {
			case domain.CartRule:
				if row.RuleFound {
					item.Group, item.Title, item.RetiredIn = row.RuleGroup, row.RuleTitle, int(row.RuleRetiredIn)
				}
			case domain.CartGroup:
				item.Group, item.Rules = row.Path, int(row.CurrentRules)
			case domain.CartLibrary:
				item.Rules = int(row.CurrentRules)
			}
			lib := &libraries[len(libraries)-1]
			lib.Items = append(lib.Items, item)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load cart accountID=%d: %v", accountID, err)
	}
	return libraries, nil
}
