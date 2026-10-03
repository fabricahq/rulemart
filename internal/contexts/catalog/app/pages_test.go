package app_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
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
	// libraries are the vetted libraries, which OwnerLibraries answers from.
	libraries []views.LibraryCard
	groups    []views.LibraryGroup
	// ruleResults answers Rules, which records each list it reads, with its limit and skip, and the groups it matches
	// by.
	ruleResults views.RuleResults
	lists       []domain.RuleList
	listPages   [][2]int
	listGroups  []domain.CanonicalGroup
	// unvetted records whether each read of the groups asked for unvetted libraries.
	unvetted []bool
}

func (r *reads) Groups(_ context.Context, _ []domain.LibraryKey, unvetted bool) ([]views.LibraryGroup, error) {
	r.unvetted = append(r.unvetted, unvetted)
	return r.groups, nil
}

func (r *reads) Rules(_ context.Context, _ []domain.LibraryKey, groups []domain.CanonicalGroup, list domain.RuleList, limit, skip int) (views.RuleResults, error) {
	r.lists, r.listPages, r.listGroups = append(r.lists, list), append(r.listPages, [2]int{limit, skip}), groups
	return r.ruleResults, nil
}

func (r *reads) OwnerLibraries(_ context.Context, _ []domain.LibraryKey, login string) ([]views.LibraryCard, error) {
	var owned []views.LibraryCard
	for _, lib := range r.libraries {
		if strings.EqualFold(lib.Owner, login) {
			owned = append(owned, lib)
		}
	}
	return owned, nil
}

func (r *reads) HomePage(context.Context, []domain.LibraryKey) ([]views.LibraryCard, []views.LibraryGroup, error) {
	return []views.LibraryCard{{Owner: "acme", Name: "backend", Rules: 3}}, r.groups, nil
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

// A group is one group however many libraries hold it, so the index combines them under its ID, vetted when any of
// them is. Canonical groups come first, by name, then the others by ID.
func TestGroupIndexCombinesEachGroupsLibraries(t *testing.T) {
	r := &reads{groups: []views.LibraryGroup{
		{Path: "practices/testing", Library: acmeRef, Vetted: true, Rules: 2},
		{Path: "practices/zz-review", Library: betaRef, Rules: 1},
		{Path: "techs/go", Library: acmeRef, Vetted: true, Rules: 1},
		{Path: "techs/go", Library: betaRef, Rules: 3},
		{Path: "techs/golang", Library: acmeRef, Vetted: true, Rules: 1},
		{Path: "techs/golang", Library: betaRef, Rules: 2},
	}}
	pages := app.Pages{Store: r, Groups: canonicalList(t)}

	got, err := pages.GroupIndex(context.Background(), true)

	if err != nil {
		t.Fatal(err)
	}
	want := views.GroupIndex{
		Techs: []views.GroupSummary{
			{Path: "techs/go", Canonical: goView, Rules: 4, Libraries: []views.LibraryRef{acmeRef, betaRef}, Vetted: true},
			{Path: "techs/golang", Rules: 3, Libraries: []views.LibraryRef{acmeRef, betaRef}, Vetted: true},
		},
		Practices: []views.GroupSummary{
			{Path: "practices/testing", Canonical: &views.CanonicalGroup{Name: "Testing", Description: "What to test."}, Rules: 2,
				Libraries: []views.LibraryRef{acmeRef}, Vetted: true},
			{Path: "practices/zz-review", Rules: 1, Libraries: []views.LibraryRef{betaRef}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if !slices.Equal(r.unvetted, []bool{true}) {
		t.Errorf("read groups with unvetted %v, want true", r.unvetted)
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
	index, err := pages.GroupIndex(context.Background(), false)
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

	got, err := app.Pages{Store: r, Groups: groups}.GroupIndex(context.Background(), false)

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

// A group's page lists a canonical group by the list's spelling of its ID, in the choices' order, and names each
// rule's group as pages do.
func TestGroupPageListsACanonicalGroupByTheListsSpelling(t *testing.T) {
	r := &reads{ruleResults: views.RuleResults{Unfiltered: 1, Rows: []views.RuleRow{
		{Library: acmeRef, Rule: views.RuleCard{Path: "techs/go/return-errors", Group: "techs/go"}},
	}}}
	pages := app.Pages{Store: r, Groups: canonicalList(t)}
	choices := domain.ListChoices{Unvetted: true, Filters: domain.RuleFilters{MinStars: 10}, Order: domain.Newest}

	got, err := pages.GroupPage(context.Background(), "Techs/GO", choices)

	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "techs/go" || !reflect.DeepEqual(got.Canonical, goView) || !reflect.DeepEqual(got.Rules.Rows[0].CanonicalGroup, goView) {
		t.Errorf("got %+v, want techs/go, canonical, its row named Go", got)
	}
	want := []domain.RuleList{{Group: "techs/go", ListChoices: choices}}
	if !reflect.DeepEqual(r.lists, want) || !slices.Equal(r.listPages, [][2]int{{app.MaxGroupRules, 0}}) {
		t.Errorf("read %+v, %v; want %+v, the first %d", r.lists, r.listPages, want, app.MaxGroupRules)
	}
}

// A canonical group no library holds yet has its page, which says so; any other group has a page only while a
// library holds it, current or retired, so a made-up ID has none.
func TestGroupHasAPageWhenCanonicalOrHeld(t *testing.T) {
	empty := app.Pages{Store: &reads{}, Groups: canonicalList(t)}
	held := app.Pages{Store: &reads{ruleResults: views.RuleResults{Unfiltered: 1, Rows: []views.RuleRow{
		{Library: acmeRef, Rule: views.RuleCard{Path: "techs/golang/pass-context", Group: "techs/golang"}},
	}}}, Groups: canonicalList(t)}

	if got, err := empty.GroupPage(context.Background(), "practices/accessibility", domain.ListChoices{}); err != nil || got.Canonical.Name != "Accessibility" {
		t.Errorf("a canonical group no library holds: got %+v, %v", got, err)
	}
	if _, err := empty.GroupPage(context.Background(), "techs/golang", domain.ListChoices{}); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("a group no library holds: got %v, want app.ErrNotFound", err)
	}
	got, err := held.GroupPage(context.Background(), "techs/golang", domain.ListChoices{})
	if err != nil || got.Path != "techs/golang" || got.Canonical != nil || got.Rules.Rows[0].CanonicalGroup != nil {
		t.Errorf("a held group that isn't canonical: got %+v, %v", got, err)
	}
	// Its retired rules are hidden until asked for, but the page offers them.
	retired := app.Pages{Store: &reads{ruleResults: views.RuleResults{RetiredRules: 1}}, Groups: canonicalList(t)}
	if got, err := retired.GroupPage(context.Background(), "techs/golang", domain.ListChoices{}); err != nil || got.Rules.RetiredRules != 1 {
		t.Errorf("a group of only retired rules: got %+v, %v", got, err)
	}
}

// Search lists the rules a query matches, or every rule without one, a page at a time, matching groups by the whole
// canonical list and naming each rule's group as pages do.
func TestSearchRulesReadsThePageOfTheListItIsAskedFor(t *testing.T) {
	r := &reads{ruleResults: views.RuleResults{Rows: []views.RuleRow{
		{Library: acmeRef, Rule: views.RuleCard{Path: "techs/go/return-errors", Group: "techs/go"}},
	}}}
	list := canonicalList(t)
	pages := app.Pages{Store: r, Groups: list}
	choices := domain.ListChoices{Filters: domain.RuleFilters{Kind: "techs"}, Order: domain.BestMatch}

	got, err := pages.SearchRules(context.Background(), domain.ParseSearchQuery("errors"), choices, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pages.SearchRules(context.Background(), domain.SearchQuery{}, choices, 1); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(got.Rows[0].CanonicalGroup, goView) || !slices.Equal(r.listGroups, list.All()) {
		t.Errorf("got %+v, matching groups %+v; want Go, by the whole list", got.Rows, r.listGroups)
	}
	want := []domain.RuleList{{Query: domain.ParseSearchQuery("errors"), ListChoices: choices}, {ListChoices: choices}}
	if !reflect.DeepEqual(r.lists, want) || !slices.Equal(r.listPages, [][2]int{{app.SearchPageSize, app.SearchPageSize}, {app.SearchPageSize, 0}}) {
		t.Errorf("read %+v, %v; want %+v, pages 2 and 1", r.lists, r.listPages, want)
	}
}

// A query past the limit isn't run, and a page outside 1 to MaxSearchPage is a mistake; neither reaches the database.
func TestSearchRulesRunsNoQueryTooLongAndNoPageOutOfBounds(t *testing.T) {
	r := &reads{}
	pages := app.Pages{Store: r, Groups: canonicalList(t)}

	long := domain.ParseSearchQuery(strings.Repeat("a", domain.MaxSearchQueryLength+1))
	if _, err := pages.SearchRules(context.Background(), long, domain.ListChoices{}, 1); !errors.Is(err, app.ErrSearchQueryTooLong) {
		t.Errorf("a long query gave %v, want app.ErrSearchQueryTooLong", err)
	}
	for _, page := range []int{0, -1, app.MaxSearchPage + 1} {
		if _, err := pages.SearchRules(context.Background(), domain.ParseSearchQuery("errors"), domain.ListChoices{}, page); err == nil {
			t.Errorf("searched page %d", page)
		}
	}
	if _, err := pages.SearchRules(context.Background(), domain.ParseSearchQuery("errors"), domain.ListChoices{}, app.MaxSearchPage); err != nil {
		t.Errorf("the last page: %v", err)
	}
	if len(r.lists) != 1 {
		t.Errorf("read %+v, want only the last page", r.lists)
	}
}

// An owner's page is their vetted libraries, under the login as the host spells it; an owner with none has no page.
func TestOwnerPageNamesTheOwnerAsTheHostDoesOrIsNotFound(t *testing.T) {
	acme := views.LibraryCard{Owner: "Acme", Name: "backend", OwnerAvatarURL: "https://avatars.example/acme", Rules: 3}
	pages := app.Pages{Store: &reads{libraries: []views.LibraryCard{acme, {Owner: "Acme", Name: "web", Rules: 1}}}, Groups: canonicalList(t)}

	got, err := pages.OwnerPage(context.Background(), "ACME")

	if err != nil {
		t.Fatal(err)
	}
	want := views.OwnerPage{Login: "Acme", AvatarURL: acme.OwnerAvatarURL, Libraries: []views.LibraryCard{acme, {Owner: "Acme", Name: "web", Rules: 1}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if _, err := pages.OwnerPage(context.Background(), "nobody"); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("an owner without a vetted library: got %v, want ErrNotFound", err)
	}
}
