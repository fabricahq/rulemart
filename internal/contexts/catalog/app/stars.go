// Star libraries: an account stars and unstars vetted libraries, and lists its stars.

package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// Stars stars libraries for accounts, as the web function does. Only a library Vetted holds can be starred.
type Stars struct {
	Store  store.Stars
	Vetted []domain.LibraryKey
}

// Star stars the library library names, as owner/name, for the account, and returns it as the code host spells it
// now. It fails with ErrNotFound when library isn't owner/name, or Vetted holds no library by that name. Starring a
// library twice keeps one star.
func (s Stars) Star(ctx context.Context, accountID int64, library string) (views.LibraryRef, error) {
	owner, name, err := parseLibraryName(library)
	if err != nil {
		return views.LibraryRef{}, err
	}
	return s.Store.Star(ctx, s.Vetted, accountID, owner, name)
}

// Unstar removes the account's star from the library library names, as owner/name, vetted or not, and does nothing
// when the account hasn't starred it. It fails with ErrNotFound when library isn't owner/name.
func (s Stars) Unstar(ctx context.Context, accountID int64, library string) error {
	owner, name, err := parseLibraryName(library)
	if err != nil {
		return err
	}
	return s.Store.Unstar(ctx, accountID, owner, name)
}

// Starred reports whether the account starred the library owner/name.
func (s Stars) Starred(ctx context.Context, accountID int64, owner, name string) (bool, error) {
	return s.Store.Starred(ctx, accountID, owner, name)
}

// AccountStars returns the libraries the account starred, most recently starred first, each with whether Vetted
// holds it, and whether a listing names it.
func (s Stars) AccountStars(ctx context.Context, accountID int64) ([]views.StarredLibrary, error) {
	return s.Store.AccountStars(ctx, s.Vetted, accountID)
}

// parseLibraryName returns the owner and name text names as owner/name, as page addresses name a library, or fails
// with ErrNotFound when it names none, as text the catalog can't hold names none.
func parseLibraryName(text string) (owner, name string, err error) {
	owner, name, ok := strings.Cut(text, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") || !domain.Storable(text) {
		return "", "", fmt.Errorf("find library %q: want owner/name: %w", text, ErrNotFound)
	}
	return owner, name, nil
}
