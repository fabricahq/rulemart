// Star rules: an account stars and unstars the current rules of vetted libraries, and lists the rules its stars
// count toward.

package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// Stars stars rules for accounts, as the web function does. Only a current rule of a library Vetted holds can be
// starred. A star on a rule that's since been retired counts toward the rule that replaced it, as store.Stars
// describes.
type Stars struct {
	Store  store.Stars
	Vetted []domain.LibraryKey
	// Groups is Code Rules' canonical group list, which decides how the starred list shows each rule's group.
	Groups domain.CanonicalGroups
}

// Star stars the current rule at rulePath in the library library names, as owner/name, for the account. It fails with
// ErrNotFound when library isn't owner/name, Vetted holds no library by that name, or it has no current rule at
// rulePath. Starring a rule twice keeps one star. first is true when the star is the account's only one: it had none,
// and now has this one.
func (s Stars) Star(ctx context.Context, accountID int64, library, rulePath string) (first bool, err error) {
	owner, name, err := parseRuleAddress(library, rulePath)
	if err != nil {
		return false, err
	}
	return s.Store.Star(ctx, s.Vetted, accountID, owner, name, rulePath)
}

// Unstar removes every star of the account's that counts toward the rule Star finds, and does nothing when it has
// none. For a rule Star doesn't find, such as one retired since, or of a library that lost its vetting, it removes the
// account's own star on that rule, which counts toward no rule, so the visitor can let it go. It fails with ErrNotFound
// when it finds neither the rule nor such a star.
func (s Stars) Unstar(ctx context.Context, accountID int64, library, rulePath string) error {
	owner, name, err := parseRuleAddress(library, rulePath)
	if err != nil {
		return err
	}
	err = s.Store.Unstar(ctx, s.Vetted, accountID, owner, name, rulePath)
	if errors.Is(err, ErrNotFound) {
		return s.Store.RemoveStar(ctx, accountID, owner, name, rulePath)
	}
	return err
}

// UncountedStars returns the account's stars that count toward no current rule of a vetted library, which AccountStars
// leaves out, most recently starred first.
func (s Stars) UncountedStars(ctx context.Context, accountID int64) ([]views.UncountedStar, error) {
	return s.Store.UncountedStars(ctx, s.Vetted, accountID)
}

// Starred reports whether one of the account's stars counts toward the current rule at rulePath in the library
// owner/name.
func (s Stars) Starred(ctx context.Context, accountID int64, owner, name, rulePath string) (bool, error) {
	return s.Store.Starred(ctx, accountID, owner, name, rulePath)
}

// AccountStars returns each current rule of a vetted library that the account's stars count toward, most recently
// starred first, each with its group as the canonical list shows it, and the retired rule the account starred in its
// place, if any.
func (s Stars) AccountStars(ctx context.Context, accountID int64) ([]views.StarredRule, error) {
	starred, err := s.Store.AccountStars(ctx, s.Vetted, accountID)
	if err != nil {
		return nil, err
	}
	for i, r := range starred {
		starred[i].CanonicalGroup = canonicalGroup(s.Groups, r.Rule.Group)
	}
	return starred, nil
}

// parseRuleAddress parses a rule's address: it returns the owner and name of library, which names a library as
// owner/name, as page addresses do, and checks that rulePath is non-empty and that both are text the catalog can
// store, as domain.Storable says. It fails with ErrNotFound otherwise, since such an address names no rule.
func parseRuleAddress(library, rulePath string) (owner, name string, err error) {
	owner, name, ok := strings.Cut(library, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") || !domain.Storable(library) ||
		rulePath == "" || !domain.Storable(rulePath) {
		return "", "", fmt.Errorf("find rule %q in library %q: want owner/name and a rule ID: %w", rulePath, library, ErrNotFound)
	}
	return owner, name, nil
}
