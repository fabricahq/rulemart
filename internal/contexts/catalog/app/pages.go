// Read what the pages show, finding only the vetted libraries, and show each group as canonical or not.

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
	// Groups is Code Rules' canonical group list, which decides how pages show each group. Pages apply it as they
	// read, rather than ingestion as it stores, so a release that updates the list shows every library by it at once.
	Groups domain.CanonicalGroups
}

// Libraries returns the vetted libraries, ordered by owner and name.
func (p Pages) Libraries(ctx context.Context) ([]views.LibraryCard, error) {
	return p.Store.Libraries(ctx, p.Vetted)
}

// LibraryPage returns the vetted library owner/name, matched without regard to case, with its groups and current
// rules, or ErrNotFound.
func (p Pages) LibraryPage(ctx context.Context, owner, name string) (views.LibraryPage, error) {
	page, err := p.Store.LibraryPage(ctx, p.Vetted, owner, name)
	if err != nil {
		return views.LibraryPage{}, err
	}
	for i, g := range page.Groups {
		page.Groups[i].Canonical = p.canonical(g.Path)
	}
	return page, nil
}

// RulePage returns the current rule at rulePath in the vetted library owner/name, with every version, or
// ErrNotFound.
func (p Pages) RulePage(ctx context.Context, owner, name, rulePath string) (views.RulePage, error) {
	page, err := p.Store.RulePage(ctx, p.Vetted, owner, name, rulePath)
	if err != nil {
		return views.RulePage{}, err
	}
	page.Rule.CanonicalGroup = p.canonical(page.Rule.Group)
	return page, nil
}

// canonical returns how pages show the group at path when it's on the canonical group list, or nil when it isn't.
func (p Pages) canonical(path string) *views.CanonicalGroup {
	g, ok := p.Groups.Find(path)
	if !ok {
		return nil
	}
	return &views.CanonicalGroup{Name: g.Name, Icon: views.GroupIcon{
		File: g.Icon.File, Monochrome: g.Icon.Monochrome, Narrow: g.Icon.Narrow,
	}}
}
