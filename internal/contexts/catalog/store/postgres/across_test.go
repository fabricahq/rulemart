package postgres_test

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
	"github.com/fabricahq/rulemart/internal/platform/database/databasetest"
)

// rule returns a current rule at path, in the group its path names, with one version and the given content.
func rule(path, title, whenToRead, body string) domain.Rule {
	c := content(title)
	c.WhenToRead = whenToRead
	c.Markdown = "---\ntitle: " + title + "\n---\n\n" + body + "\n"
	group := path[:strings.LastIndex(path, "/")]
	return domain.Rule{Path: path, Group: group, Content: c, Versions: []domain.Version{
		{Number: v(1, 0, 0), Release: 1, Change: coderules.ChangeNew, Summaries: []string{"Add the rule."}},
	}}
}

// library returns a library of one release, holding groups and rules, at GitHub repository id, owner/name.
func newLibrary(id, owner, name string, groups []domain.Group, rules ...domain.Rule) domain.Library {
	return domain.Library{
		Repository: domain.Repository{Host: domain.GitHub, ID: id, Owner: owner, Name: name, Description: "Rules by " + owner + ".",
			OwnerAvatarURL: "https://avatars.githubusercontent.com/u/" + id + "?v=4"},
		Releases: []domain.Release{{Number: 1, CommitID: strings.Repeat("1", 40), TaggedAt: day(1)}},
		Groups:   groups,
		Rules:    rules,
	}
}

var (
	goGroup      = domain.Group{Path: "techs/go", Name: "Go", Description: "Go rules.", WhenToRead: "When writing Go."}
	testingGroup = domain.Group{Path: "practices/testing", Name: "Testing", Description: "Testing rules.", WhenToRead: "When testing."}
	// golangGroup isn't canonical. Its library declares a name that pages and search never use.
	golangGroup = domain.Group{Path: "techs/golang", Name: "Zebra", Description: "More Go rules.", WhenToRead: "When writing Go."}
)

// The two vetted libraries are acme/backend and Beta/rules: case-insensitively acme sorts first, though byte order
// would put Beta first. stranger/rules isn't vetted.
var (
	acme = newLibrary("21", "acme", "backend", []domain.Group{testingGroup, goGroup, golangGroup},
		rule("practices/testing/verify-retry-limits", "Verify retry limits", "When code calls a service.", "Stop after a fixed number of attempts."),
		rule("practices/testing/cover-boundary-cases", "Cover boundary cases", "When a retry policy changes.", "Test the first and last attempts."),
		rule("techs/go/return-errors", "Return errors with context", "When a function fails.", "Wrap each error with what failed."),
		rule("techs/golang/pass-context-first", "Pass context first", "When a function takes a context.", "Put it first."),
		// Retired: it keeps its group in the catalog, but no page or search shows it.
		domain.Rule{Path: "practices/testing/retry-forever", Group: "practices/testing", RetiredIn: 1,
			RetirementSummaries: []string{"Drop it."},
			Versions:            []domain.Version{{Number: v(1, 0, 0), Release: 1, Change: coderules.ChangeNew, Summaries: []string{"Add the rule."}}}},
	)
	beta = newLibrary("22", "Beta", "rules", []domain.Group{goGroup},
		rule("techs/go/name-packages-plainly", "Name packages plainly", "When adding a package.", "Avoid a name that needs a retry to read."),
	)
	stranger = newLibrary("23", "stranger", "rules", []domain.Group{testingGroup, goGroup},
		rule("practices/testing/retry-everything", "Retry everything", "When anything fails.", "Retry it."),
		rule("techs/go/use-go", "Use Go", "When writing Go.", "Write Go."),
	)
	vettedBoth = []domain.LibraryKey{{Host: domain.GitHub, RepositoryID: "21"}, {Host: domain.GitHub, RepositoryID: "22"}}
	// canonicalGroups is the part of the canonical list these tests use. techs/golang isn't on it.
	canonicalGroups = []domain.CanonicalGroup{
		{ID: "practices/testing", Name: "Testing"},
		{ID: "techs/go", Name: "Go"},
	}
)

// newLibraries stores acme, beta, and stranger in a new database, and returns a Store that reads it as the web
// function's role.
func newLibraries(t *testing.T) *postgres.Store {
	t.Helper()
	db, connString := databasetest.New(t)
	writer := postgres.New(db)
	for _, lib := range []domain.Library{acme, beta, stranger} {
		if _, err := writer.ReplaceLibrary(context.Background(), lib); err != nil {
			t.Fatal(err)
		}
	}
	return postgres.New(databasetest.AsWebRole(t, connString))
}

// sourceIDs returns each result's source-qualified rule ID, owner/name:rule ID.
func sourceIDs(results views.SearchResults) []string {
	ids := []string{}
	for _, r := range results.Results {
		ids = append(ids, r.Library.FullName()+":"+r.Rule.Path)
	}
	return ids
}

// completeIDs returns the source-qualified IDs of the results that hold every term to find.
func completeIDs(results views.SearchResults) []string {
	ids := []string{}
	for _, r := range results.Results {
		if len(r.Missing) == 0 {
			ids = append(ids, r.Library.FullName()+":"+r.Rule.Path)
		}
	}
	return ids
}

// search runs query against reader with canonicalGroups, returning at most 50 results.
func search(t *testing.T, reader *postgres.Store, query string) views.SearchResults {
	t.Helper()
	results, err := reader.Search(context.Background(), vettedBoth, canonicalGroups, domain.ParseSearchQuery(query), 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	return results
}

// A title says what a rule is about, its reading guidance and impact when it applies, and its body how: a match ranks
// by where it is, not by how often the body repeats a word.
func TestSearchRanksTitleMatchesThenSummaryMatchesThenBodyMatches(t *testing.T) {
	reader := newLibraries(t)

	got := search(t, reader, "retry")

	want := []string{
		"acme/backend:practices/testing/verify-retry-limits",  // in its title
		"acme/backend:practices/testing/cover-boundary-cases", // in its reading guidance
		"Beta/rules:techs/go/name-packages-plainly",           // only in its body
	}
	if !slices.Equal(sourceIDs(got), want) || got.Total != 3 {
		t.Fatalf("got %q of %d, want %q of 3", sourceIDs(got), got.Total, want)
	}
	first := got.Results[0]
	wantFirst := views.SearchResult{
		Library: views.LibraryRef{Owner: "acme", Name: "backend", OwnerAvatarURL: acme.Repository.OwnerAvatarURL},
		Rule: views.RuleCard{Path: "practices/testing/verify-retry-limits", Group: "practices/testing", Title: "Verify retry limits",
			Impact: "HIGH", Version: v(1, 0, 0)},
		WhenToRead: "When code calls a service.",
	}
	if !reflect.DeepEqual(first, wantFirst) {
		t.Fatalf("the first result is %+v, want %+v", first, wantFirst)
	}
}

// A visitor searching for a technology finds its rules though their text never names it, by the canonical list's
// name for their group, or the ID of a group that isn't canonical, but never by what a library calls its group.
func TestSearchMatchesGroupsByTheirCanonicalNameOrIDOnly(t *testing.T) {
	reader := newLibraries(t)

	for query, want := range map[string][]string{
		// Both match only by their group's name, so they rank equally, in title order.
		"go": {"Beta/rules:techs/go/name-packages-plainly", "acme/backend:techs/go/return-errors"},
		// techs/golang isn't canonical, so it goes by its ID.
		"golang": {"acme/backend:techs/golang/pass-context-first"},
		// acme names techs/golang Zebra, and declares no canonical name a page would show.
		"zebra": {},
		// The group's kind isn't part of its name.
		"techs": {},
		// A query's words may match partly the rule's text and partly its group's name.
		"retry go":                {"Beta/rules:techs/go/name-packages-plainly"},
		`"retry" golang -context`: {},
	} {
		if got := completeIDs(search(t, reader, query)); !slices.Equal(got, want) {
			t.Errorf("%q found %q, want %q", query, got, want)
		}
	}
}

func TestSearchReadsTheVisitorsSyntaxAndStemsWords(t *testing.T) {
	reader := newLibraries(t)

	for query, want := range map[string][]string{
		`"retry limits"`:   {"acme/backend:practices/testing/verify-retry-limits"},
		"retry -verify":    {"acme/backend:practices/testing/cover-boundary-cases", "Beta/rules:techs/go/name-packages-plainly"},
		"retries":          {"acme/backend:practices/testing/verify-retry-limits", "acme/backend:practices/testing/cover-boundary-cases", "Beta/rules:techs/go/name-packages-plainly"},
		"attempts or fail": {"acme/backend:practices/testing/cover-boundary-cases", "acme/backend:techs/go/return-errors", "acme/backend:practices/testing/verify-retry-limits"},
		// Only stop words, only punctuation, and unbalanced syntax match nothing, without an error.
		"the":        {},
		"!!! &&| :*": {},
		`"retry`:     {"acme/backend:practices/testing/verify-retry-limits", "acme/backend:practices/testing/cover-boundary-cases", "Beta/rules:techs/go/name-packages-plainly"},
		"-":          {},
	} {
		// Ranking has its own test; this one checks what matches.
		got := sourceIDs(search(t, reader, query))
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%q found %q, want %q", query, got, want)
		}
	}
}

func TestSearchReturnsTheBestMatchesUpToItsLimitAndCountsThemAll(t *testing.T) {
	reader := newLibraries(t)

	for _, tc := range []struct {
		limit, skip int
		want        []string
	}{
		{2, 0, []string{"acme/backend:practices/testing/verify-retry-limits", "acme/backend:practices/testing/cover-boundary-cases"}},
		{2, 2, []string{"Beta/rules:techs/go/name-packages-plainly"}},
		{1, 3, []string{}},
	} {
		got, err := reader.Search(context.Background(), vettedBoth, canonicalGroups, domain.ParseSearchQuery("retry"), tc.limit, tc.skip)

		if err != nil {
			t.Fatal(err)
		}
		wantTotal := 3
		if len(tc.want) == 0 {
			wantTotal = 0 // a page past the last counts nothing
		}
		if !slices.Equal(sourceIDs(got), tc.want) || got.Total != wantTotal || got.NoWords {
			t.Errorf("limit %d after %d: got %q of %d (no words %v), want %q of %d", tc.limit, tc.skip, sourceIDs(got), got.Total,
				got.NoWords, tc.want, wantTotal)
		}
	}
}

// The limit bounds the results a page shows, so a search for none is a mistake, not an empty page with a total, and
// so is skipping fewer than none.
func TestSearchRefusesALimitBelowOneOrANegativeSkip(t *testing.T) {
	reader := newLibraries(t)

	for _, tc := range []struct{ limit, skip int }{{0, 0}, {-1, 0}, {1, -1}} {
		if _, err := reader.Search(context.Background(), vettedBoth, canonicalGroups, domain.ParseSearchQuery("retry"), tc.limit, tc.skip); err == nil {
			t.Errorf("searched with limit %d after %d", tc.limit, tc.skip)
		}
	}
}

// A visitor copies the IDs pages show: a rule's ID or part of it, its group's ID, its library's owner and name, and
// its source-qualified ID. Each finds what it names, and a group's ID never finds a group whose ID only starts the
// same.
func TestSearchFindsRulesByTheIDsPagesShow(t *testing.T) {
	reader := newLibraries(t)

	retryLimits := "acme/backend:practices/testing/verify-retry-limits"
	boundaries := "acme/backend:practices/testing/cover-boundary-cases"
	acmeGo := "acme/backend:techs/go/return-errors"
	betaGo := "Beta/rules:techs/go/name-packages-plainly"
	golang := "acme/backend:techs/golang/pass-context-first"
	for query, want := range map[string][]string{
		"verify-retry-limits":     {retryLimits},
		"retry-limits":            {retryLimits},
		"practices/testing":       {boundaries, retryLimits},
		"techs/go":                {acmeGo, betaGo},
		"techs/golang":            {golang},
		"acme/backend":            {boundaries, retryLimits, acmeGo, golang},
		"Beta/rules":              {betaGo},
		"backend":                 {boundaries, retryLimits, acmeGo, golang},
		"Beta/rules:techs/go":     {betaGo},
		retryLimits:               {retryLimits},
		"acme/backend:techs":      {acmeGo, golang},
		"stranger/rules":          {},
		"techs/go -return-errors": {betaGo},
	} {
		got := sourceIDs(search(t, reader, query))
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%q found %q, want %q", query, got, want)
		}
	}
}

// A search with nothing to look for matches nothing, and says so, so the page can explain why.
func TestSearchReportsAQueryWithNoWordToFind(t *testing.T) {
	reader := newLibraries(t)

	for query, noWords := range map[string]bool{
		"the":                        true,
		"the of and":                 true,
		"-retry":                     true,
		"!!! &&| :*":                 true,
		`""`:                         true,
		"zebra":                      false,
		"the retry":                  false,
		"retry -verify -cover -name": false,
	} {
		got := search(t, reader, query)
		if got.NoWords != noWords {
			t.Errorf("%q: NoWords is %v, want %v", query, got.NoWords, noWords)
		}
		if noWords && (len(got.Results) > 0 || got.Total > 0) {
			t.Errorf("%q found %q", query, sourceIDs(got))
		}
	}
}

func TestGroupsListEachVettedLibrarysGroupsWithCurrentRules(t *testing.T) {
	reader := newLibraries(t)

	got, err := reader.Groups(context.Background(), vettedBoth)

	if err != nil {
		t.Fatal(err)
	}
	acmeRef := views.LibraryRef{Owner: "acme", Name: "backend", OwnerAvatarURL: acme.Repository.OwnerAvatarURL}
	betaRef := views.LibraryRef{Owner: "Beta", Name: "rules", OwnerAvatarURL: beta.Repository.OwnerAvatarURL}
	want := []views.LibraryGroup{
		{Path: "practices/testing", Library: acmeRef, Rules: 2},
		{Path: "techs/go", Library: acmeRef, Rules: 1},
		{Path: "techs/go", Library: betaRef, Rules: 1},
		{Path: "techs/golang", Library: acmeRef, Rules: 1},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestGroupRulesListEveryVettedLibrarysRulesInTheGroup(t *testing.T) {
	reader := newLibraries(t)

	got, err := reader.GroupRules(context.Background(), vettedBoth, "techs/go")

	if err != nil {
		t.Fatal(err)
	}
	want := []views.GroupLibrary{
		{
			Library: views.LibraryRef{Owner: "acme", Name: "backend", OwnerAvatarURL: acme.Repository.OwnerAvatarURL},
			Rules: []views.RuleCard{{Path: "techs/go/return-errors", Group: "techs/go", Title: "Return errors with context",
				Impact: "HIGH", Version: v(1, 0, 0)}},
		},
		{
			Library: views.LibraryRef{Owner: "Beta", Name: "rules", OwnerAvatarURL: beta.Repository.OwnerAvatarURL},
			Rules: []views.RuleCard{{Path: "techs/go/name-packages-plainly", Group: "techs/go", Title: "Name packages plainly",
				Impact: "HIGH", Version: v(1, 0, 0)}},
		},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].Library != want[i].Library || !slices.Equal(got[i].Rules, want[i].Rules) {
			t.Errorf("library %d is %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestGroupRulesAreEmptyForAGroupNoVettedLibraryHolds(t *testing.T) {
	reader := newLibraries(t)

	got, err := reader.GroupRules(context.Background(), vettedBoth, "techs/rust")

	if err != nil || len(got) != 0 {
		t.Fatalf("got %+v, %v; want nothing", got, err)
	}
}
