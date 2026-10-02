// Read what the pages show, finding only the vetted libraries, and show each group as canonical or not.

package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// ErrNotFound reports a library or rule that isn't in the catalog, isn't vetted, or is retired, or a group that
// isn't canonical.
var ErrNotFound = store.ErrNotFound

// ErrSearchQueryTooLong reports a query of more than domain.MaxSearchQueryLength characters, which search won't run.
var ErrSearchQueryTooLong = errors.New("search query too long")

// SearchPageSize is how many results each page of a search holds.
const SearchPageSize = 20

// MaxSearchPage is the last page of results a search reads, which bounds how far a crafted URL makes the database
// read: 200 pages of 20 is many times the catalog.
const MaxSearchPage = 200

// Pages reads what the catalog's pages show. It finds only the libraries in Vetted, and reads each page from one
// state of the catalog.
type Pages struct {
	Store  store.Reader
	Vetted []domain.LibraryKey
	// Groups is Code Rules' canonical group list, which decides how pages show each group. Pages apply it as they
	// read, rather than ingestion as it stores, so a release that updates the list shows every library by it at once.
	Groups domain.CanonicalGroups
}

// HomePage returns the vetted libraries, ordered by owner and name, and their groups, as GroupIndex orders them, from
// one state of the catalog.
func (p Pages) HomePage(ctx context.Context) (views.HomePage, error) {
	libraries, groups, err := p.Store.HomePage(ctx, p.Vetted)
	if err != nil {
		return views.HomePage{}, err
	}
	return views.HomePage{Libraries: libraries, Groups: p.index(groups)}, nil
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

// GroupIndex returns every group that holds current rules in a vetted library. A canonical group combines every
// library that holds it; any other group stands alone, so each library's is listed apart. Each kind lists its
// canonical groups first, by name without regard to case, then the others by ID and library.
func (p Pages) GroupIndex(ctx context.Context) (views.GroupIndex, error) {
	groups, err := p.Store.Groups(ctx, p.Vetted)
	if err != nil {
		return views.GroupIndex{}, err
	}
	return p.index(groups), nil
}

// index turns each library's groups, in path order and then library order, into the index GroupIndex returns.
func (p Pages) index(groups []views.LibraryGroup) views.GroupIndex {
	var index views.GroupIndex
	for _, summary := range p.summarize(groups) {
		if strings.HasPrefix(summary.Path, "practices/") {
			index.Practices = append(index.Practices, summary)
		} else {
			index.Techs = append(index.Techs, summary)
		}
	}
	return index
}

// summarize turns each library's groups, in path order and then library order, into the index's entries, in its
// order.
func (p Pages) summarize(groups []views.LibraryGroup) []views.GroupSummary {
	var canonical, others []views.GroupSummary
	for _, g := range groups {
		c := p.canonical(g.Path)
		if c == nil {
			others = append(others, views.GroupSummary{Path: g.Path, Rules: g.Rules, Libraries: []views.LibraryRef{g.Library}})
			continue
		}
		if n := len(canonical); n > 0 && canonical[n-1].Path == g.Path {
			canonical[n-1].Rules += g.Rules
			canonical[n-1].Libraries = append(canonical[n-1].Libraries, g.Library)
			continue
		}
		canonical = append(canonical, views.GroupSummary{Path: g.Path, Canonical: c, Rules: g.Rules, Libraries: []views.LibraryRef{g.Library}})
	}
	slices.SortStableFunc(canonical, func(a, b views.GroupSummary) int {
		return strings.Compare(strings.ToLower(a.Canonical.Name), strings.ToLower(b.Canonical.Name))
	})
	return append(canonical, others...)
}

// GroupPage returns the canonical group id with its current rules in every vetted library that holds it, or
// ErrNotFound when id isn't on the canonical group list: any other group stands alone, on its library's page.
func (p Pages) GroupPage(ctx context.Context, id string) (views.GroupPage, error) {
	c := p.canonical(id)
	if c == nil {
		return views.GroupPage{}, fmt.Errorf("load group: %w", ErrNotFound)
	}
	libraries, err := p.Store.GroupRules(ctx, p.Vetted, id)
	if err != nil {
		return views.GroupPage{}, err
	}
	return views.GroupPage{Path: id, Canonical: *c, Libraries: libraries}, nil
}

// Search returns page, counted from 1, of the vetted libraries' current rules that best match query, SearchPageSize
// to a page, with how many matched in all. An empty query matches nothing, and one longer than
// domain.MaxSearchQueryLength fails with ErrSearchQueryTooLong; neither reads the catalog. A page past the last
// holds no results, and page must be from 1 to MaxSearchPage.
func (p Pages) Search(ctx context.Context, query domain.SearchQuery, page int) (views.SearchResults, error) {
	if page < 1 || page > MaxSearchPage {
		return views.SearchResults{}, fmt.Errorf("search: page %d is outside 1 to %d", page, MaxSearchPage)
	}
	if query.IsZero() {
		return views.SearchResults{}, nil
	}
	if query.TooLong() {
		return views.SearchResults{}, fmt.Errorf("search: %w", ErrSearchQueryTooLong)
	}
	results, err := p.Store.Search(ctx, p.Vetted, p.Groups.All(), query, SearchPageSize, (page-1)*SearchPageSize)
	if err != nil {
		return views.SearchResults{}, err
	}
	for i, r := range results.Results {
		results.Results[i].CanonicalGroup = p.canonical(r.Rule.Group)
	}
	return results, nil
}

// canonical returns how pages show the group at path when it's on the canonical group list, or nil when it isn't.
func (p Pages) canonical(path string) *views.CanonicalGroup {
	g, ok := p.Groups.Find(path)
	if !ok {
		return nil
	}
	return &views.CanonicalGroup{Name: g.Name, Description: g.Description, Icon: views.GroupIcon{
		File: g.Icon.File, Monochrome: g.Icon.Monochrome, Narrow: g.Icon.Narrow,
	}}
}
