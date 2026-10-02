package app_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// reads stands in for the store's page reads across libraries, answering with fixed values and recording what each
// read asked for. Reads it doesn't override aren't used here.
type reads struct {
	store.Reader
	groups      []views.LibraryGroup
	groupRules  []views.GroupLibrary
	results     views.SearchResults
	searched    []domain.SearchQuery
	searchedFor []domain.CanonicalGroup
	ruleReads   []string
}

func (r *reads) Groups(context.Context, []domain.LibraryKey) ([]views.LibraryGroup, error) {
	return r.groups, nil
}

func (r *reads) HomePage(context.Context, []domain.LibraryKey) ([]views.LibraryCard, []views.LibraryGroup, error) {
	return []views.LibraryCard{{Owner: "acme", Name: "backend", Rules: 3}}, r.groups, nil
}

func (r *reads) GroupRules(_ context.Context, _ []domain.LibraryKey, path string) ([]views.GroupLibrary, error) {
	r.ruleReads = append(r.ruleReads, path)
	return r.groupRules, nil
}

func (r *reads) Search(_ context.Context, _ []domain.LibraryKey, groups []domain.CanonicalGroup, query domain.SearchQuery, limit, skip int) (views.SearchResults, error) {
	r.searched, r.searchedFor = append(r.searched, query), groups
	if limit != app.MaxSearchResults || skip != 0 {
		return views.SearchResults{}, errors.New("unexpected limit")
	}
	return r.results, nil
}

// canonicalList is a canonical group list of Go, Testing, and Accessibility, of which only Go has an icon.
func canonicalList(t *testing.T) domain.CanonicalGroups {
	t.Helper()
	groups, err := domain.NewCanonicalGroups([]coderules.CanonicalGroup{
		{ID: "practices/accessibility", Name: "Accessibility", Description: "Usable by everyone."},
		{ID: "practices/testing", Name: "Testing", Description: "What to test."},
		{ID: "techs/go", Name: "Go", Description: "The Go language."},
	}, map[string]domain.GroupIcon{"techs/go": {File: "devicon/go-original.svg"}})
	if err != nil {
		t.Fatal(err)
	}
	return groups
}

var (
	acmeRef = views.LibraryRef{Owner: "acme", Name: "backend"}
	betaRef = views.LibraryRef{Owner: "Beta", Name: "rules"}
	goView  = &views.CanonicalGroup{Name: "Go", Description: "The Go language.", Icon: views.GroupIcon{File: "devicon/go-original.svg"}}
)

// A canonical group is one group however many libraries hold it, so the index combines them; any other group stands
// alone, so each library's is listed apart. Canonical groups come first, by name, then the others by ID.
func TestGroupIndexCombinesCanonicalGroupsAndKeepsOthersApart(t *testing.T) {
	r := &reads{groups: []views.LibraryGroup{
		{Path: "practices/testing", Library: acmeRef, Rules: 2},
		{Path: "practices/zz-review", Library: betaRef, Rules: 1},
		{Path: "techs/go", Library: acmeRef, Rules: 1},
		{Path: "techs/go", Library: betaRef, Rules: 3},
		{Path: "techs/golang", Library: acmeRef, Rules: 1},
		{Path: "techs/golang", Library: betaRef, Rules: 2},
	}}
	pages := app.Pages{Store: r, Groups: canonicalList(t)}

	got, err := pages.GroupIndex(context.Background())

	if err != nil {
		t.Fatal(err)
	}
	want := views.GroupIndex{
		Techs: []views.GroupSummary{
			{Path: "techs/go", Canonical: goView, Rules: 4, Libraries: []views.LibraryRef{acmeRef, betaRef}},
			{Path: "techs/golang", Rules: 1, Libraries: []views.LibraryRef{acmeRef}},
			{Path: "techs/golang", Rules: 2, Libraries: []views.LibraryRef{betaRef}},
		},
		Practices: []views.GroupSummary{
			{Path: "practices/testing", Canonical: &views.CanonicalGroup{Name: "Testing", Description: "What to test."}, Rules: 2,
				Libraries: []views.LibraryRef{acmeRef}},
			{Path: "practices/zz-review", Rules: 1, Libraries: []views.LibraryRef{betaRef}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// The home page shows the libraries with the same index as the groups page.
func TestHomePageShowsTheLibrariesAndTheGroupIndex(t *testing.T) {
	r := &reads{groups: []views.LibraryGroup{
		{Path: "techs/golang", Library: acmeRef, Rules: 1},
		{Path: "techs/go", Library: acmeRef, Rules: 2},
	}}
	pages := app.Pages{Store: r, Groups: canonicalList(t)}

	got, err := pages.HomePage(context.Background())

	if err != nil {
		t.Fatal(err)
	}
	index, err := pages.GroupIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Libraries) != 1 || got.Libraries[0].Name != "backend" || !reflect.DeepEqual(got.Groups, index) {
		t.Fatalf("got %+v, want acme/backend and %+v", got, index)
	}
}

// Canonical groups sort by the list's name, which needn't follow their IDs.
func TestGroupIndexOrdersCanonicalGroupsByNameWithoutRegardToCase(t *testing.T) {
	groups, err := domain.NewCanonicalGroups([]coderules.CanonicalGroup{
		{ID: "techs/a-lang", Name: "zeta"},
		{ID: "techs/b-lang", Name: "Alpha"},
		{ID: "techs/c-lang", Name: "beta"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := &reads{groups: []views.LibraryGroup{
		{Path: "techs/a-lang", Library: acmeRef, Rules: 1},
		{Path: "techs/b-lang", Library: acmeRef, Rules: 1},
		{Path: "techs/c-lang", Library: acmeRef, Rules: 1},
	}}

	got, err := app.Pages{Store: r, Groups: groups}.GroupIndex(context.Background())

	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, g := range got.Techs {
		names = append(names, g.Canonical.Name)
	}
	if want := []string{"Alpha", "beta", "zeta"}; !slices.Equal(names, want) {
		t.Fatalf("got %q, want %q", names, want)
	}
}

func TestGroupPageReadsACanonicalGroupsRulesInEveryLibrary(t *testing.T) {
	r := &reads{groupRules: []views.GroupLibrary{{Library: acmeRef, Rules: []views.RuleCard{{Path: "techs/go/return-errors"}}}}}
	pages := app.Pages{Store: r, Groups: canonicalList(t)}

	got, err := pages.GroupPage(context.Background(), "techs/go")

	if err != nil {
		t.Fatal(err)
	}
	want := views.GroupPage{Path: "techs/go", Canonical: *goView, Libraries: r.groupRules}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// A canonical group no vetted library holds yet still has its page, which says so.
func TestGroupPageOfACanonicalGroupNoLibraryHoldsHasNoLibraries(t *testing.T) {
	pages := app.Pages{Store: &reads{}, Groups: canonicalList(t)}

	got, err := pages.GroupPage(context.Background(), "practices/accessibility")

	if err != nil || got.Canonical.Name != "Accessibility" || len(got.Libraries) != 0 {
		t.Fatalf("got %+v, %v", got, err)
	}
}

// A group that isn't canonical stands alone in its library, so it has no page across libraries, and the store isn't
// asked about an ID a visitor made up.
func TestGroupPageRefusesAGroupThatIsntCanonical(t *testing.T) {
	r := &reads{}
	pages := app.Pages{Store: r, Groups: canonicalList(t)}

	for _, id := range []string{"techs/golang", "techs/Go", "techs/go/return-errors", "techs", ""} {
		if _, err := pages.GroupPage(context.Background(), id); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("%q: got %v, want app.ErrNotFound", id, err)
		}
	}
	if len(r.ruleReads) != 0 {
		t.Fatalf("read the rules of %q", r.ruleReads)
	}
}

// Search names each result's group as pages do, and matches groups by the whole canonical list.
func TestSearchShowsEachResultsGroupByTheCanonicalList(t *testing.T) {
	r := &reads{results: views.SearchResults{Total: 2, Results: []views.SearchResult{
		{Library: acmeRef, Rule: views.RuleCard{Path: "techs/go/return-errors", Group: "techs/go"}},
		{Library: betaRef, Rule: views.RuleCard{Path: "techs/golang/pass-context", Group: "techs/golang"}},
	}}}
	list := canonicalList(t)
	pages := app.Pages{Store: r, Groups: list}

	got, err := pages.Search(context.Background(), domain.ParseSearchQuery("errors"))

	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Results[0].CanonicalGroup, goView) || got.Results[1].CanonicalGroup != nil || got.Total != 2 {
		t.Fatalf("got %+v", got)
	}
	if !slices.Equal(r.searchedFor, list.All()) {
		t.Fatalf("searched with groups %+v, want the whole list", r.searchedFor)
	}
}

// An empty query has nothing to find, and a query past the limit isn't run, so neither reaches the database.
func TestSearchRunsNoQueryThatIsEmptyOrTooLong(t *testing.T) {
	r := &reads{}
	pages := app.Pages{Store: r, Groups: canonicalList(t)}

	empty, err := pages.Search(context.Background(), domain.ParseSearchQuery("  "))
	if err != nil || len(empty.Results) != 0 || empty.Total != 0 {
		t.Fatalf("an empty query gave %+v, %v", empty, err)
	}
	long := make([]byte, domain.MaxSearchQueryLength+1)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := pages.Search(context.Background(), domain.ParseSearchQuery(string(long))); !errors.Is(err, app.ErrSearchQueryTooLong) {
		t.Fatalf("a long query gave %v, want app.ErrSearchQueryTooLong", err)
	}
	if len(r.searched) != 0 {
		t.Fatalf("searched for %q", r.searched)
	}
}
