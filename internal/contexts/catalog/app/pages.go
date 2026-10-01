// Read what the pages show, finding only the vetted libraries.

package app

import (
	"context"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// ErrNotFound reports a library or rule that isn't in the catalog, isn't vetted, or is retired.
var ErrNotFound = store.ErrNotFound

// Pages reads what the catalog's pages show. It finds only the libraries in Vetted, and reads each page from one
// state of the catalog.
type Pages struct {
	Store  store.Reader
	Vetted []domain.LibraryKey
}

// Libraries returns the vetted libraries, ordered by owner and name.
func (p Pages) Libraries(ctx context.Context) ([]views.LibraryCard, error) {
	return p.Store.Libraries(ctx, p.Vetted)
}

// LibraryPage returns the vetted library owner/name, matched without regard to case, with its groups and current
// rules, or ErrNotFound.
func (p Pages) LibraryPage(ctx context.Context, owner, name string) (views.LibraryPage, error) {
	return p.Store.LibraryPage(ctx, p.Vetted, owner, name)
}

// RulePage returns the current rule at rulePath in the vetted library owner/name, with every version, or
// ErrNotFound.
func (p Pages) RulePage(ctx context.Context, owner, name, rulePath string) (views.RulePage, error) {
	return p.Store.RulePage(ctx, p.Vetted, owner, name, rulePath)
}
