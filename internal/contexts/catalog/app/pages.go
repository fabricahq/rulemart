// Read what the pages show, finding only the vetted libraries across libraries, and listed ones too on a library's own
// pages, and show each group as canonical or not.

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

// ErrNotFound reports a library or rule that isn't in the catalog, a library that's neither vetted nor listed, a
// group that isn't canonical, a rule to star or unstar that isn't a current rule of a vetted library, an account's
// listing it doesn't have, or an item to add to a cart that its library doesn't have.
var ErrNotFound = store.ErrNotFound

// ErrSearchQueryTooLong reports a query of more than domain.MaxSearchQueryLength characters, which search won't run.
var ErrSearchQueryTooLong = errors.New("search query too long")

// SearchPageSize is how many results each page of a search holds.
const SearchPageSize = 20

// MaxSearchPage is the last page of results a search reads, which bounds how far a crafted URL makes the database
// read: 200 pages of 20 is many times the catalog.
const MaxSearchPage = 200

// Pages reads what the catalog's pages show, each page from one state of the catalog. Pages across libraries find only
// the libraries in Vetted; a library's own pages also find one a listing names, and say it isn't vetted.
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

// Libraries returns the vetted libraries, and with unvetted, the libraries listings name too, ordered by owner and
// name.
func (p Pages) Libraries(ctx context.Context, unvetted bool) ([]views.LibraryCard, error) {
	return p.Store.Libraries(ctx, p.Vetted, unvetted)
}

// OwnerPage returns the owner login, matched without regard to case, with their vetted libraries, ordered by name, or
// ErrNotFound when no vetted library is theirs: a listed, unvetted library gives its owner no page. The page's Login
// is the code host's spelling.
func (p Pages) OwnerPage(ctx context.Context, login string) (views.OwnerPage, error) {
	libraries, err := p.Store.OwnerLibraries(ctx, p.Vetted, login)
	if err != nil {
		return views.OwnerPage{}, err
	}
	if len(libraries) == 0 {
		return views.OwnerPage{}, fmt.Errorf("load owner: %w", ErrNotFound)
	}
	return views.OwnerPage{Login: libraries[0].Owner, AvatarURL: libraries[0].OwnerAvatarURL, Libraries: libraries}, nil
}

// UnvettedLibraries returns the libraries listings name that aren't vetted, ordered by owner and name.
func (p Pages) UnvettedLibraries(ctx context.Context) ([]views.LibraryCard, error) {
	return p.Store.UnvettedLibraries(ctx, p.Vetted)
}

// LibraryPage returns the library owner/name, vetted or listed, matched without regard to case, with its groups,
// current rules, and retired rules, each with its chain of replacements to now and whether it was renamed, or
// ErrNotFound.
func (p Pages) LibraryPage(ctx context.Context, owner, name string) (views.LibraryPage, error) {
	page, err := p.Store.LibraryPage(ctx, p.Vetted, owner, name)
	if err != nil {
		return views.LibraryPage{}, err
	}
	for i, g := range page.Groups {
		page.Groups[i].Canonical = p.canonical(g.Path)
	}
	links := newRuleLinks(page.Links)
	for i, r := range page.Retired {
		page.Retired[i].Replacements, page.Retired[i].Renamed = links.replacements(r.Path), links.renamed(r.Path)
	}
	return page, nil
}

// RulePage returns the rule at rulePath in the library owner/name, current or retired, with every version, the
// rules it replaced or renamed, and while it's retired, its chain of replacements to now, or ErrNotFound.
func (p Pages) RulePage(ctx context.Context, owner, name, rulePath string) (views.RulePage, error) {
	page, err := p.Store.RulePage(ctx, p.Vetted, owner, name, rulePath)
	if err != nil {
		return views.RulePage{}, err
	}
	page.Rule.CanonicalGroup = p.canonical(page.Rule.Group)
	links := newRuleLinks(page.Links)
	if retirement := page.Rule.Retirement; retirement != nil {
		retirement.Replacements, retirement.Renamed = links.replacements(page.Rule.Path), links.renamed(page.Rule.Path)
	}
	page.RenamedFrom, page.Replaces = links.replaced(page.Rule.Path)
	return page, nil
}

// GroupIndex returns every group that holds current rules in a vetted library, and with unvetted, in a library a
// listing names too. A group combines every library that holds it, under its ID. Each kind lists its canonical groups
// first, by name without regard to case, then the others by ID.
func (p Pages) GroupIndex(ctx context.Context, unvetted bool) (views.GroupIndex, error) {
	groups, err := p.Store.Groups(ctx, p.Vetted, unvetted)
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
// order: a group's libraries combined, canonical groups first.
func (p Pages) summarize(groups []views.LibraryGroup) []views.GroupSummary {
	var canonical, others []views.GroupSummary
	for _, g := range groups {
		c := p.canonical(g.Path)
		summaries := &others
		if c != nil {
			summaries = &canonical
		}
		if n := len(*summaries); n > 0 && (*summaries)[n-1].Path == g.Path {
			last := &(*summaries)[n-1]
			last.Rules += g.Rules
			last.Libraries = append(last.Libraries, g.Library)
			last.Vetted = last.Vetted || g.Vetted
			continue
		}
		*summaries = append(*summaries, views.GroupSummary{
			Path: g.Path, Canonical: c, Rules: g.Rules, Libraries: []views.LibraryRef{g.Library}, Vetted: g.Vetted,
		})
	}
	slices.SortStableFunc(canonical, func(a, b views.GroupSummary) int {
		return strings.Compare(strings.ToLower(a.Canonical.Name), strings.ToLower(b.Canonical.Name))
	})
	return append(canonical, others...)
}

// MaxGroupRules is the most rules a group's page lists, which bounds the page a group shared by many libraries makes.
const MaxGroupRules = 500

// GroupPage returns the rules of the group id that choices keep, in their order, at most MaxGroupRules: a canonical
// group's, matched without regard to case, in every library that holds it, or any other group's, in the libraries that
// chose exactly that ID. It fails with ErrNotFound for a group that isn't canonical and holds no rule before the
// filters, so a made-up ID has no page. The page's Path is the list's spelling of a canonical group's ID.
func (p Pages) GroupPage(ctx context.Context, id string, choices domain.ListChoices) (views.GroupPage, error) {
	page := views.GroupPage{Path: id}
	if g, ok := p.Groups.FindIgnoringCase(id); ok {
		page.Path, page.Canonical = g.ID, p.canonical(g.ID)
	}
	var err error
	page.Rules, err = p.rules(ctx, domain.RuleList{Group: page.Path, ListChoices: choices}, MaxGroupRules, 0)
	if err != nil {
		return views.GroupPage{}, err
	}
	if page.Canonical == nil && page.Rules.Unfiltered == 0 {
		return views.GroupPage{}, fmt.Errorf("load group: %w", ErrNotFound)
	}
	return page, nil
}

// SearchRules returns page, counted from 1, of the rules that match query, or of every rule for the zero query, that
// choices keep, in their order, SearchPageSize to a page. A query longer than domain.MaxSearchQueryLength fails with
// ErrSearchQueryTooLong without reading the catalog. A page past the last holds no rules, and page must be from 1 to
// MaxSearchPage.
func (p Pages) SearchRules(ctx context.Context, query domain.SearchQuery, choices domain.ListChoices, page int) (views.RuleResults, error) {
	if page < 1 || page > MaxSearchPage {
		return views.RuleResults{}, fmt.Errorf("search: page %d is outside 1 to %d", page, MaxSearchPage)
	}
	if query.TooLong() {
		return views.RuleResults{}, fmt.Errorf("search: %w", ErrSearchQueryTooLong)
	}
	return p.rules(ctx, domain.RuleList{Query: query, ListChoices: choices}, SearchPageSize, (page-1)*SearchPageSize)
}

// rules reads a page of list, at most limit rules after the first skip, and names each one's group as pages do.
func (p Pages) rules(ctx context.Context, list domain.RuleList, limit, skip int) (views.RuleResults, error) {
	results, err := p.Store.Rules(ctx, p.Vetted, p.Groups.All(), list, limit, skip)
	if err != nil {
		return views.RuleResults{}, err
	}
	for i, r := range results.Rows {
		results.Rows[i].CanonicalGroup = p.canonical(r.Rule.Group)
	}
	return results, nil
}

// canonical returns how pages show the group at path when it's on the canonical group list, or nil when it isn't.
func (p Pages) canonical(path string) *views.CanonicalGroup {
	return canonicalGroup(p.Groups, path)
}

// canonicalGroup returns how pages show the group at path when it's on groups, or nil when it isn't.
func canonicalGroup(groups domain.CanonicalGroups, path string) *views.CanonicalGroup {
	g, ok := groups.Find(path)
	if !ok {
		return nil
	}
	return &views.CanonicalGroup{Name: g.Name, Description: g.Description, Icon: views.GroupIcon{
		File: g.Icon.File, Monochrome: g.Icon.Monochrome, Narrow: g.Icon.Narrow, LightTile: g.Icon.LightTile,
	}}
}
