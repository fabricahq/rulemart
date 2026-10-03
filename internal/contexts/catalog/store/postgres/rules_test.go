package postgres_test

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// listedRule returns a current rule at path titled title, of impact, that library release first published.
func listedRule(path, title, impact string, first int) domain.Rule {
	r := rule(path, title, "When "+strings.ToLower(title)+".", title+".")
	r.Versions[0].Release = first
	r.Versions[0].Content.Impact = impact
	return r
}

// The catalog the lists read: two vetted libraries and a listed one, whose rules hold stars.
//
//   - fabricahq/rules: release 1 on day 1 added handle-errors (HIGH), test-errors (CRITICAL), and old-errors; release 2
//     on day 5 added name-things (MEDIUM) and retired old-errors, replaced by handle-errors.
//   - zeta/go: release 1 on day 3 added zap-errors (LOW), yield-errors (MEDIUM), and techs/golang's go-errors
//     (MEDIUM-HIGH).
//   - aardvark/rules, listed: release 1 on day 9 added aardvark-errors (HIGH).
//
// handle-errors has 3 stars, one of them given to old-errors before it was retired; zap-errors has 3; name-things 1;
// yield-errors none.
// aardvark-errors holds 5, which count for nothing while its library isn't vetted.
var (
	fabricaRules = func() domain.Library {
		old := listedRule("techs/go/old-errors", "Old errors", "HIGH", 1)
		old.RetiredIn, old.ReplacedBy, old.RetirementSummaries = 2, "techs/go/handle-errors", []string{"Fold it in."}
		old.WhenToReadHTML = ""
		lib := newLibrary("31", "fabricahq", "rules", []domain.Group{goGroup, testingGroup},
			listedRule("techs/go/handle-errors", "Handle errors", "HIGH", 1),
			listedRule("techs/go/name-things", "Name things", "MEDIUM", 2),
			old,
			listedRule("practices/testing/test-errors", "Test errors", "CRITICAL", 1),
		)
		lib.Releases = append(lib.Releases, domain.Release{Number: 2, CommitID: strings.Repeat("2", 40), TaggedAt: day(5)})
		return lib
	}()
	zetaGo = func() domain.Library {
		lib := newLibrary("32", "zeta", "go", []domain.Group{goGroup, golangGroup},
			listedRule("techs/go/zap-errors", "Zap errors", "LOW", 1),
			listedRule("techs/go/yield-errors", "Yield errors", "MEDIUM", 1),
			listedRule("techs/golang/go-errors", "Go errors", "MEDIUM-HIGH", 1),
		)
		lib.Releases[0].TaggedAt = day(3)
		return lib
	}()
	aardvarkRules = func() domain.Library {
		lib := newLibrary("33", "aardvark", "rules", []domain.Group{goGroup},
			listedRule("techs/go/aardvark-errors", "Aardvark errors", "HIGH", 1))
		lib.Releases[0].TaggedAt = day(9)
		return lib
	}()
	vettedLists = []domain.LibraryKey{{Host: domain.GitHub, RepositoryID: "31"}, {Host: domain.GitHub, RepositoryID: "32"}}
)

// newRuleLists stores the libraries the lists read, lists aardvark/rules, stars their rules, and returns a catalog
// that reads them as the web function does.
func newRuleLists(t *testing.T) listingCatalog {
	t.Helper()
	c := newListingCatalog(t)
	ctx := context.Background()
	writer := c.worker
	for _, lib := range []domain.Library{fabricaRules, zetaGo, aardvarkRules} {
		if _, err := writer.ReplaceLibrary(ctx, lib); err != nil {
			t.Fatal(err)
		}
	}
	listing, err := c.web.CreateListing(ctx, vettedLists, c.account(t, 100), "aardvark", "rules")
	if err != nil {
		t.Fatal(err)
	}
	c.resolve(t, listing, "33")
	stars := map[string][]int{
		"31:techs/go/handle-errors":   {1, 2},
		"31:techs/go/old-errors":      {3},
		"32:techs/go/zap-errors":      {1, 2, 3},
		"31:techs/go/name-things":     {4},
		"33:techs/go/aardvark-errors": {1, 2, 3, 4, 5},
	}
	accounts := map[int]int64{}
	for key, starrers := range stars {
		repository, path, _ := strings.Cut(key, ":")
		for _, n := range starrers {
			if accounts[n] == 0 {
				accounts[n] = c.account(t, n)
			}
			c.starAsOwner(t, accounts[n], repository, path)
		}
	}
	return c
}

// listRules lists the rules list describes, at most 50, failing t if it can't.
func (c listingCatalog) listRules(t *testing.T, list domain.RuleList) views.RuleResults {
	t.Helper()
	results, err := c.web.Rules(context.Background(), vettedLists, canonicalGroups, list, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	return results
}

// sourceIDs returns each result's source-qualified rule ID, owner/name:rule ID.
func sourceIDs(results views.RuleResults) []string {
	ids := []string{}
	for _, r := range results.Rows {
		ids = append(ids, r.Library.FullName()+":"+r.Rule.Path)
	}
	return ids
}

const (
	handleErrors = "fabricahq/rules:techs/go/handle-errors"
	nameThings   = "fabricahq/rules:techs/go/name-things"
	oldErrors    = "fabricahq/rules:techs/go/old-errors"
	testErrors   = "fabricahq/rules:practices/testing/test-errors"
	zapErrors    = "zeta/go:techs/go/zap-errors"
	yieldErrors  = "zeta/go:techs/go/yield-errors"
	goErrors     = "zeta/go:techs/golang/go-errors"
	aardvark     = "aardvark/rules:techs/go/aardvark-errors"
)

// goList is techs/go's page, most starred first, with filters.
func goList(filters domain.RuleFilters) domain.RuleList {
	return domain.RuleList{Group: "techs/go", ListChoices: domain.ListChoices{Filters: filters, Order: domain.MostStarred}}
}

// A group's list orders its rules by stars, a tie falling to Fabrica's libraries, or by the release that first
// published each, newest first, counting stars through replacements and leaving out retired rules.
func TestAGroupsRulesListInEachOrder(t *testing.T) {
	c := newRuleLists(t)

	for _, tc := range []struct {
		order domain.RuleOrder
		want  []string
	}{
		{domain.MostStarred, []string{handleErrors, zapErrors, nameThings, yieldErrors}},
		{domain.Newest, []string{nameThings, zapErrors, yieldErrors, handleErrors}},
	} {
		got := c.listRules(t, domain.RuleList{Group: "techs/go", ListChoices: domain.ListChoices{Order: tc.order}})
		if !slices.Equal(sourceIDs(got), tc.want) {
			t.Errorf("%s: got %q, want %q", tc.order, sourceIDs(got), tc.want)
		}
		if got.Total != 4 || got.Libraries != 2 || got.Unfiltered != 4 {
			t.Errorf("%s: got %d rules in %d libraries of %d, want 4 in 2 of 4", tc.order, got.Total, got.Libraries, got.Unfiltered)
		}
	}
	got := c.listRules(t, goList(domain.RuleFilters{}))
	stars := map[string]int{}
	for _, r := range got.Rows {
		stars[r.Library.FullName()+":"+r.Rule.Path] = r.Rule.Stars
	}
	if want := map[string]int{handleErrors: 3, zapErrors: 3, nameThings: 1, yieldErrors: 0}; !reflect.DeepEqual(stars, want) {
		t.Errorf("got stars %v, want %v", stars, want)
	}
}

// Each filter keeps the rules it names, alone and with the others, and none of them changes what the list holds
// before them, which the sidebar counts.
func TestAGroupsFiltersKeepTheRulesTheyName(t *testing.T) {
	c := newRuleLists(t)

	for _, tc := range []struct {
		name    string
		filters domain.RuleFilters
		want    []string
	}{
		{"a library", domain.RuleFilters{Libraries: []string{"zeta/go"}}, []string{zapErrors, yieldErrors}},
		{"two libraries", domain.RuleFilters{Libraries: []string{"zeta/go", "fabricahq/rules"}}, []string{handleErrors, zapErrors, nameThings, yieldErrors}},
		{"a library the list lacks", domain.RuleFilters{Libraries: []string{"nobody/rules"}}, []string{}},
		{"critical and high", domain.RuleFilters{Impact: domain.HighImpact}, []string{handleErrors}},
		{"medium and lower", domain.RuleFilters{Impact: domain.LowerImpact}, []string{zapErrors, nameThings, yieldErrors}},
		{"3 stars", domain.RuleFilters{MinStars: 3}, []string{handleErrors, zapErrors}},
		{"4 stars", domain.RuleFilters{MinStars: 4}, []string{}},
		{"a library and an impact", domain.RuleFilters{Libraries: []string{"fabricahq/rules"}, Impact: domain.LowerImpact}, []string{nameThings}},
		{"an impact and stars", domain.RuleFilters{Impact: domain.LowerImpact, MinStars: 3}, []string{zapErrors}},
		{"every filter, matching none", domain.RuleFilters{Libraries: []string{"fabricahq/rules"}, Impact: domain.LowerImpact, MinStars: 3}, []string{}},
	} {
		got := c.listRules(t, goList(tc.filters))
		if !slices.Equal(sourceIDs(got), tc.want) || got.Total != len(tc.want) {
			t.Errorf("%s: got %q of %d, want %q", tc.name, sourceIDs(got), got.Total, tc.want)
		}
		want := []views.LibraryCount{
			{Library: views.LibraryRef{Owner: "fabricahq", Name: "rules", OwnerAvatarURL: fabricaRules.Repository.OwnerAvatarURL}, Vetted: true, Rules: 2},
			{Library: views.LibraryRef{Owner: "zeta", Name: "go", OwnerAvatarURL: zetaGo.Repository.OwnerAvatarURL}, Vetted: true, Rules: 2},
		}
		if !reflect.DeepEqual(got.UnfilteredLibraries, want) || got.Unfiltered != 4 {
			t.Errorf("%s: got %d unfiltered from %+v, want 4 from %+v", tc.name, got.Unfiltered, got.UnfilteredLibraries, want)
		}
	}
}

// My libraries keeps the rules of the libraries whose owner is one of the list's owners, without regard to case, and
// with the other filters, those that pass them all; a visitor whose owners publish nothing in the list sees none. It
// doesn't change what the list holds before its filters, which the sidebar counts.
func TestMyLibrariesKeepTheRulesOfTheOwnersLibraries(t *testing.T) {
	c := newRuleLists(t)

	for _, tc := range []struct {
		name    string
		filters domain.RuleFilters
		owners  []string
		want    []string
	}{
		{"an organization", domain.RuleFilters{Mine: true}, []string{"octocat", "Zeta"}, []string{zapErrors, yieldErrors}},
		{"the visitor and an organization", domain.RuleFilters{Mine: true}, []string{"FabricaHQ", "zeta"}, []string{handleErrors, zapErrors, nameThings, yieldErrors}},
		{"owners who publish nothing here", domain.RuleFilters{Mine: true}, []string{"octocat", "aardvark"}, []string{}},
		{"no owners", domain.RuleFilters{Mine: true}, nil, []string{}},
		{"owners without my libraries", domain.RuleFilters{}, []string{"zeta"}, []string{handleErrors, zapErrors, nameThings, yieldErrors}},
		{"and a library of another owner", domain.RuleFilters{Mine: true, Libraries: []string{"fabricahq/rules"}}, []string{"zeta"}, []string{}},
		{"and an impact", domain.RuleFilters{Mine: true, Impact: domain.LowerImpact}, []string{"fabricahq"}, []string{nameThings}},
	} {
		list := goList(tc.filters)
		list.Owners = tc.owners
		got := c.listRules(t, list)
		if !slices.Equal(sourceIDs(got), tc.want) || got.Total != len(tc.want) {
			t.Errorf("%s: got %q of %d, want %q", tc.name, sourceIDs(got), got.Total, tc.want)
		}
		if got.Unfiltered != 4 || len(got.UnfilteredLibraries) != 2 {
			t.Errorf("%s: got %d unfiltered from %+v, want 4 from 2 libraries", tc.name, got.Unfiltered, got.UnfilteredLibraries)
		}
	}
}

// Nothing of a listed library is read unless the list asks for unvetted libraries; then its rules join the list,
// unvetted and without stars, after the vetted rules they tie with, and the sidebar counts it.
func TestAListReadsUnvettedLibrariesOnlyWhenAsked(t *testing.T) {
	c := newRuleLists(t)

	without := c.listRules(t, goList(domain.RuleFilters{}))
	with := c.listRules(t, domain.RuleList{Group: "techs/go", ListChoices: domain.ListChoices{Unvetted: true, Order: domain.MostStarred}})

	if slices.Contains(sourceIDs(without), aardvark) || len(without.UnfilteredLibraries) != 2 {
		t.Errorf("without unvetted libraries: got %q from %+v", sourceIDs(without), without.UnfilteredLibraries)
	}
	if want := []string{handleErrors, zapErrors, nameThings, yieldErrors, aardvark}; !slices.Equal(sourceIDs(with), want) {
		t.Fatalf("with unvetted libraries: got %q, want %q", sourceIDs(with), want)
	}
	if last := with.Rows[4]; last.Vetted || last.Rule.Stars != 0 || !with.Rows[0].Vetted {
		t.Errorf("got the listed library's rule %+v, the first %+v; want it unvetted without stars", last, with.Rows[0])
	}
	var owners []string
	for _, l := range with.UnfilteredLibraries {
		owners = append(owners, l.Library.Owner)
	}
	if want := []string{"fabricahq", "aardvark", "zeta"}; !slices.Equal(owners, want) || with.UnfilteredLibraries[1].Vetted {
		t.Errorf("got the sidebar's libraries %+v, want %q, aardvark unvetted", with.UnfilteredLibraries, want)
	}
	search := c.listRules(t, domain.RuleList{Query: domain.ParseSearchQuery("aardvark"), ListChoices: domain.ListChoices{Order: domain.BestMatch}})
	if len(search.Rows) != 0 {
		t.Errorf("search found %q without unvetted libraries", sourceIDs(search))
	}
	search = c.listRules(t, domain.RuleList{Query: domain.ParseSearchQuery("aardvark"), ListChoices: domain.ListChoices{Unvetted: true, Order: domain.BestMatch}})
	if !slices.Equal(sourceIDs(search), []string{aardvark}) {
		t.Errorf("search found %q with unvetted libraries, want %q", sourceIDs(search), aardvark)
	}
}

// A group whose ID isn't canonical lists the rules of the libraries that chose exactly that ID.
func TestAGroupThatIsntCanonicalListsTheRulesOfThatExactID(t *testing.T) {
	c := newRuleLists(t)

	got := c.listRules(t, domain.RuleList{Group: "techs/golang", ListChoices: domain.ListChoices{Order: domain.MostStarred}})

	if !slices.Equal(sourceIDs(got), []string{goErrors}) {
		t.Errorf("got %q, want %q", sourceIDs(got), goErrors)
	}
	if got := c.listRules(t, domain.RuleList{Group: "techs/gol", ListChoices: domain.ListChoices{Order: domain.MostStarred}}); len(got.Rows) != 0 || got.Unfiltered != 0 {
		t.Errorf("techs/gol: got %q", sourceIDs(got))
	}
}

// A group's list holds its retired rules only when asked, after the current rules, each with how its library's rules
// were replaced, counted apart from them.
func TestAGroupListsItsRetiredRulesWhenAsked(t *testing.T) {
	c := newRuleLists(t)

	got := c.listRules(t, domain.RuleList{Group: "techs/go", ListChoices: domain.ListChoices{Retired: true, Order: domain.Newest}})

	if want := []string{nameThings, zapErrors, yieldErrors, handleErrors, oldErrors}; !slices.Equal(sourceIDs(got), want) {
		t.Fatalf("got %q, want %q", sourceIDs(got), want)
	}
	retired := got.Rows[4]
	links := retired.Links
	retired.Links = nil
	want := views.RuleRow{
		Library: views.LibraryRef{Owner: "fabricahq", Name: "rules", OwnerAvatarURL: fabricaRules.Repository.OwnerAvatarURL},
		Vetted:  true,
		Rule: views.RuleCard{Path: "techs/go/old-errors", Group: "techs/go", Title: "Old errors", Impact: "HIGH",
			Version: coderules.RuleVersion{Major: 1}},
		Retired: true, GroupRules: 1,
	}
	if !reflect.DeepEqual(retired, want) {
		t.Errorf("got %+v, want %+v", retired, want)
	}
	// The row carries its library's links, which pages follow to its replacement; a current rule's carries none.
	old := slices.IndexFunc(links, func(l views.RuleLink) bool { return l.Path == "techs/go/old-errors" })
	if len(links) != 4 || old < 0 || links[old].ReplacedBy != "techs/go/handle-errors" || links[old].RetiredIn != 2 {
		t.Errorf("got the retired rule's links %+v, want fabricahq/rules's 4, old-errors replaced by handle-errors", links)
	}
	for _, r := range got.Rows[:4] {
		if r.Links != nil {
			t.Errorf("the current %s carries links", r.Rule.Path)
		}
	}
}

// Search finds a retired rule after every current rule, even one holding fewer of the query's words, and every current
// rule groups with its group's, in the order of each group's best rule. Retired rules group by group after them.
func TestSearchListsRetiredRulesAfterEveryCurrentRule(t *testing.T) {
	c := newRuleLists(t)

	for _, query := range []string{"errors", "old errors"} {
		got := c.listRules(t, domain.RuleList{Query: domain.ParseSearchQuery(query), ListChoices: domain.ListChoices{Order: domain.BestMatch}})

		ids := sourceIDs(got)
		if len(ids) != 6 || ids[5] != oldErrors || !got.Rows[5].Retired {
			t.Fatalf("%q: got %q, want 6 rules ending with the retired old-errors", query, ids)
		}
		var groups []string
		for i, r := range got.Rows[:5] {
			if r.Retired {
				t.Errorf("%q: %s is retired", query, ids[i])
			}
			if n := len(groups); n == 0 || groups[n-1] != r.Rule.Group {
				if slices.Contains(groups, r.Rule.Group) {
					t.Errorf("%q: got %s apart from its group's other rules: %q", query, ids[i], ids)
				}
				groups = append(groups, r.Rule.Group)
			}
		}
		goRow := slices.IndexFunc(got.Rows, func(r views.RuleRow) bool { return r.Rule.Group == "techs/go" })
		if got.Rows[goRow].GroupRules != 3 || got.Rows[5].GroupRules != 1 {
			t.Errorf("%q: got techs/go's current rules counted %d and its retired ones %d, want 3 and 1", query, got.Rows[goRow].GroupRules, got.Rows[5].GroupRules)
		}
	}
}

// A list holds retired rules when it shows them, a search for words always and any other list when asked, and the
// sidebar's counts include them then, so narrowing a list never raises a count, while a page's own count of its rules
// counts current ones only. Every list knows how many retired rules it could show.
func TestAListCountsItsRetiredRulesOnlyWhenItShowsThem(t *testing.T) {
	c := newRuleLists(t)
	fabrica := func(results views.RuleResults) int {
		for _, l := range results.UnfilteredLibraries {
			if l.Library.Owner == "fabricahq" {
				return l.Rules
			}
		}
		return 0
	}

	for _, tc := range []struct {
		name                                string
		list                                domain.RuleList
		unfiltered, current, fabrica, total int
	}{
		{"every rule", domain.RuleList{ListChoices: domain.ListChoices{Order: domain.MostStarred}}, 6, 6, 3, 6},
		{"every rule with retired ones", domain.RuleList{ListChoices: domain.ListChoices{Retired: true, Order: domain.MostStarred}}, 7, 6, 4, 7},
		{"a group", goList(domain.RuleFilters{}), 4, 4, 2, 4},
		{"a group with retired ones", domain.RuleList{Group: "techs/go", ListChoices: domain.ListChoices{Retired: true, Order: domain.MostStarred}}, 5, 4, 3, 5},
		{"a search", domain.RuleList{Query: domain.ParseSearchQuery("errors"), ListChoices: domain.ListChoices{Order: domain.BestMatch}}, 6, 5, 3, 6},
	} {
		got := c.listRules(t, tc.list)
		if got.Unfiltered != tc.unfiltered || got.UnfilteredCurrent != tc.current || fabrica(got) != tc.fabrica || got.Total != tc.total ||
			got.RetiredRules != 1 {
			t.Errorf("%s: got %d rules, %d current, %d of fabricahq/rules, %d passing, %d retired; want %d, %d, %d, %d, 1",
				tc.name, got.Unfiltered, got.UnfilteredCurrent, fabrica(got), got.Total, got.RetiredRules, tc.unfiltered, tc.current,
				tc.fabrica, tc.total)
		}
	}
	if got := c.listRules(t, domain.RuleList{Group: "techs/golang", ListChoices: domain.ListChoices{Order: domain.MostStarred}}); got.RetiredRules != 0 {
		t.Errorf("techs/golang: got %d retired rules, want none", got.RetiredRules)
	}
}

// A group whose rules are all retired still counts them while it hides them, so its page exists and offers them, and
// lists them when asked.
func TestAGroupOfOnlyRetiredRulesCountsThemWhileHidingThem(t *testing.T) {
	c := newRuleLists(t)
	legacy := domain.Group{Path: "techs/legacy", Name: "Legacy", Description: "Old rules.", WhenToRead: "Never."}
	gone := listedRule("techs/legacy/gone", "Gone", "LOW", 1)
	gone.RetiredIn, gone.RetirementSummaries, gone.WhenToReadHTML = 2, []string{"Drop it."}, ""
	relic := newLibrary("34", "relic", "rules", []domain.Group{legacy}, gone)
	relic.Releases = append(relic.Releases, domain.Release{Number: 2, CommitID: strings.Repeat("2", 40), TaggedAt: day(5)})
	if _, err := c.worker.ReplaceLibrary(context.Background(), relic); err != nil {
		t.Fatal(err)
	}
	vetted := append(slices.Clone(vettedLists), domain.LibraryKey{Host: domain.GitHub, RepositoryID: "34"})
	list := func(retired bool) views.RuleResults {
		t.Helper()
		got, err := c.web.Rules(context.Background(), vetted, canonicalGroups,
			domain.RuleList{Group: "techs/legacy", ListChoices: domain.ListChoices{Retired: retired, Order: domain.MostStarred}}, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	hidden, shown := list(false), list(true)

	if len(hidden.Rows) != 0 || hidden.Unfiltered != 0 || hidden.RetiredRules != 1 {
		t.Errorf("hidden: got %q, %d rules, %d retired; want none, 0, 1", sourceIDs(hidden), hidden.Unfiltered, hidden.RetiredRules)
	}
	if want := []string{"relic/rules:techs/legacy/gone"}; !slices.Equal(sourceIDs(shown), want) || !shown.Rows[0].Retired ||
		shown.Unfiltered != 1 || shown.RetiredRules != 1 {
		t.Errorf("shown: got %q, %d rules, %d retired; want %q retired, 1, 1", sourceIDs(shown), shown.Unfiltered, shown.RetiredRules, want)
	}
}

// A page counts the libraries its current rules come from, while the sidebar lists every library of the rules the list
// holds: a library whose only rules in a group are retired joins the sidebar while retired rules show, but not the
// page's count.
func TestAListCountsOnlyTheLibrariesOfItsCurrentRulesForThePage(t *testing.T) {
	c := newRuleLists(t)
	gone := listedRule("techs/go/gone", "Gone", "LOW", 1)
	gone.RetiredIn, gone.RetirementSummaries, gone.WhenToReadHTML = 2, []string{"Drop it."}, ""
	relic := newLibrary("34", "relic", "rules", []domain.Group{goGroup}, gone)
	relic.Releases = append(relic.Releases, domain.Release{Number: 2, CommitID: strings.Repeat("2", 40), TaggedAt: day(5)})
	if _, err := c.worker.ReplaceLibrary(context.Background(), relic); err != nil {
		t.Fatal(err)
	}
	vetted := append(slices.Clone(vettedLists), domain.LibraryKey{Host: domain.GitHub, RepositoryID: "34"})

	for _, tc := range []struct {
		name               string
		retired            bool
		sidebar, ofCurrent int
	}{
		{"hiding retired rules", false, 2, 2},
		{"showing retired rules", true, 3, 2},
	} {
		got, err := c.web.Rules(context.Background(), vetted, canonicalGroups,
			domain.RuleList{Group: "techs/go", ListChoices: domain.ListChoices{Retired: tc.retired, Order: domain.MostStarred}}, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.UnfilteredLibraries) != tc.sidebar || got.UnfilteredCurrentLibraries != tc.ofCurrent {
			t.Errorf("%s: got %d libraries in the sidebar and %d of current rules, want %d and %d",
				tc.name, len(got.UnfilteredLibraries), got.UnfilteredCurrentLibraries, tc.sidebar, tc.ofCurrent)
		}
	}
}

// Search's Kind keeps one kind of group, and its other orders sort its matches as a group's do.
func TestSearchFiltersByKindAndSortsByStars(t *testing.T) {
	c := newRuleLists(t)
	errors := domain.ParseSearchQuery("errors")

	practices := c.listRules(t, domain.RuleList{Query: errors, ListChoices: domain.ListChoices{Filters: domain.RuleFilters{Kind: "practices"}, Order: domain.BestMatch}})
	starred := c.listRules(t, domain.RuleList{Query: errors, ListChoices: domain.ListChoices{Filters: domain.RuleFilters{Kind: "techs", MinStars: 1}, Order: domain.MostStarred}})

	if !slices.Equal(sourceIDs(practices), []string{testErrors}) || practices.Unfiltered != 6 {
		t.Errorf("practices: got %q of %d", sourceIDs(practices), practices.Unfiltered)
	}
	if want := []string{handleErrors, zapErrors}; !slices.Equal(sourceIDs(starred), want) {
		t.Errorf("techs, most starred, with a star: got %q, want %q", sourceIDs(starred), want)
	}
}

// A list without a query holds every current rule of the vetted libraries, and a page of it skips what came before.
func TestEveryRuleListsInPages(t *testing.T) {
	c := newRuleLists(t)
	all := domain.RuleList{ListChoices: domain.ListChoices{Order: domain.MostStarred}}

	first, err := c.web.Rules(context.Background(), vettedLists, canonicalGroups, all, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	rest, err := c.web.Rules(context.Background(), vettedLists, canonicalGroups, all, 10, 2)
	if err != nil {
		t.Fatal(err)
	}
	past, err := c.web.Rules(context.Background(), vettedLists, canonicalGroups, all, 10, 50)
	if err != nil {
		t.Fatal(err)
	}

	ids := append(sourceIDs(first), sourceIDs(rest)...)
	if len(ids) != 6 || first.Total != 6 || slices.Contains(ids, oldErrors) {
		t.Errorf("got %q of %d, want every current rule, 6", ids, first.Total)
	}
	if len(past.Rows) != 0 || past.Unfiltered != 6 || len(past.UnfilteredLibraries) != 2 {
		t.Errorf("past the last page: got %+v", past)
	}
}

// A query with no word to find matches nothing, and says so.
func TestAListOfAQueryWithoutWordsSaysSo(t *testing.T) {
	c := newRuleLists(t)

	for _, query := range []string{"the", "-errors", "..."} {
		got := c.listRules(t, domain.RuleList{Query: domain.ParseSearchQuery(query), ListChoices: domain.ListChoices{Order: domain.BestMatch}})
		if !got.NoWords || len(got.Rows) != 0 {
			t.Errorf("%q: got %+v, want no words", query, got)
		}
	}
	if got := c.listRules(t, domain.RuleList{Query: domain.ParseSearchQuery("zebra"), ListChoices: domain.ListChoices{Order: domain.BestMatch}}); got.NoWords || len(got.Rows) != 0 {
		t.Errorf("zebra: got %+v, want no rules, with words", got)
	}
}
