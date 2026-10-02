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
		if !reflect.DeepEqual(got.LibraryCounts, want) || got.Unfiltered != 4 {
			t.Errorf("%s: got %d unfiltered from %+v, want 4 from %+v", tc.name, got.Unfiltered, got.LibraryCounts, want)
		}
	}
}

// Nothing of a listed library is read unless the list asks for unvetted libraries; then its rules join the list,
// unvetted and without stars, after the vetted rules they tie with, and the sidebar counts it.
func TestAListReadsUnvettedLibrariesOnlyWhenAsked(t *testing.T) {
	c := newRuleLists(t)

	without := c.listRules(t, goList(domain.RuleFilters{}))
	with := c.listRules(t, domain.RuleList{Group: "techs/go", ListChoices: domain.ListChoices{Unvetted: true, Order: domain.MostStarred}})

	if slices.Contains(sourceIDs(without), aardvark) || len(without.LibraryCounts) != 2 {
		t.Errorf("without unvetted libraries: got %q from %+v", sourceIDs(without), without.LibraryCounts)
	}
	if want := []string{handleErrors, zapErrors, nameThings, yieldErrors, aardvark}; !slices.Equal(sourceIDs(with), want) {
		t.Fatalf("with unvetted libraries: got %q, want %q", sourceIDs(with), want)
	}
	if last := with.Rows[4]; last.Vetted || last.Rule.Stars != 0 || !with.Rows[0].Vetted {
		t.Errorf("got the listed library's rule %+v, the first %+v; want it unvetted without stars", last, with.Rows[0])
	}
	var owners []string
	for _, l := range with.LibraryCounts {
		owners = append(owners, l.Library.Owner)
	}
	if want := []string{"fabricahq", "aardvark", "zeta"}; !slices.Equal(owners, want) || with.LibraryCounts[1].Vetted {
		t.Errorf("got the sidebar's libraries %+v, want %q, aardvark unvetted", with.LibraryCounts, want)
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

// A group's list holds its retired rules only when asked, after the current rules, each with its replacement.
func TestAGroupListsItsRetiredRulesWhenAsked(t *testing.T) {
	c := newRuleLists(t)

	got := c.listRules(t, domain.RuleList{Group: "techs/go", ListChoices: domain.ListChoices{Retired: true, Order: domain.Newest}})

	if want := []string{nameThings, zapErrors, yieldErrors, handleErrors, oldErrors}; !slices.Equal(sourceIDs(got), want) {
		t.Fatalf("got %q, want %q", sourceIDs(got), want)
	}
	retired := got.Rows[4]
	want := views.RuleRow{
		Library: views.LibraryRef{Owner: "fabricahq", Name: "rules", OwnerAvatarURL: fabricaRules.Repository.OwnerAvatarURL},
		Vetted:  true,
		Rule: views.RuleCard{Path: "techs/go/old-errors", Group: "techs/go", Title: "Old errors", Impact: "HIGH",
			Version: coderules.RuleVersion{Major: 1}},
		Retired: true, ReplacedBy: &views.RuleRef{Path: "techs/go/handle-errors", Title: "Handle errors"}, GroupRules: 5,
	}
	if !reflect.DeepEqual(retired, want) {
		t.Errorf("got %+v, want %+v", retired, want)
	}
}

// Search finds a retired rule, ranked after the current rules that match as well, and every rule groups with its
// group's, in the order of each group's best rule.
func TestSearchRanksRetiredRulesBelowCurrentOnesAndGroupsByGroup(t *testing.T) {
	c := newRuleLists(t)

	got := c.listRules(t, domain.RuleList{Query: domain.ParseSearchQuery("errors"), ListChoices: domain.ListChoices{Order: domain.BestMatch}})

	ids := sourceIDs(got)
	if len(ids) != 6 || !slices.Contains(ids, oldErrors) {
		t.Fatalf("got %q, want 6 rules with old-errors", ids)
	}
	var groups []string
	for i, r := range got.Rows {
		if r.Retired && r.Rule.Path != "techs/go/old-errors" {
			t.Errorf("%s is retired", ids[i])
		}
		if n := len(groups); n == 0 || groups[n-1] != r.Rule.Group {
			if slices.Contains(groups, r.Rule.Group) {
				t.Errorf("got %s apart from its group's other rules: %q", ids[i], ids)
			}
			groups = append(groups, r.Rule.Group)
		}
	}
	goRows := slices.IndexFunc(got.Rows, func(r views.RuleRow) bool { return r.Rule.Group == "techs/go" })
	if goRows < 0 || got.Rows[goRows].GroupRules != 4 {
		t.Fatalf("got %+v, want techs/go's 4 rules", got.Rows)
	}
	if ids[goRows+3] != oldErrors {
		t.Errorf("got techs/go's rules %q, want old-errors after the current ones that match as well", ids[goRows:goRows+4])
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
	if len(past.Rows) != 0 || past.Unfiltered != 6 || len(past.LibraryCounts) != 2 {
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
