package postgres_test

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/platform/database/databasetest"
)

// completeIDs returns the source-qualified IDs of the results that hold every term to find.
func completeIDs(results views.RuleResults) []string {
	ids := []string{}
	for _, r := range results.Rows {
		if len(r.Missing) == 0 {
			ids = append(ids, r.Library.FullName()+":"+r.Rule.Path)
		}
	}
	return ids
}

// search runs query against reader with canonicalGroups, best match first, returning at most 50 results.
func search(t *testing.T, reader *postgres.Store, query string) views.RuleResults {
	t.Helper()
	results, err := reader.Rules(context.Background(), vettedBoth, canonicalGroups, searchList(query), 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	return results
}

// searchList is the list of the rules query matches, best first.
func searchList(query string) domain.RuleList {
	return domain.RuleList{Query: domain.ParseSearchQuery(query), ListChoices: domain.ListChoices{Order: domain.BestMatch}}
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
	first := got.Rows[0]
	wantFirst := views.RuleRow{
		Library: views.LibraryRef{Owner: "acme", Name: "backend", OwnerAvatarURL: acme.Repository.OwnerAvatarURL},
		Vetted:  true,
		Rule: views.RuleCard{Path: "practices/testing/verify-retry-limits", Group: "practices/testing", Title: "Verify retry limits",
			Impact: "HIGH", Version: v(1, 0, 0)},
		GroupRules: 2,
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
		got, err := reader.Rules(context.Background(), vettedBoth, canonicalGroups, searchList("retry"), tc.limit, tc.skip)

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
		if _, err := reader.Rules(context.Background(), vettedBoth, canonicalGroups, searchList("retry"), tc.limit, tc.skip); err == nil {
			t.Errorf("searched with limit %d after %d", tc.limit, tc.skip)
		}
	}
}

// A visitor copies the IDs pages show: a rule's ID or part of it, its group's ID, its library's owner and name, and
// its source-qualified ID. Each finds what it names, retired rules too, and a group's ID never finds a group whose ID
// only starts the same.
func TestSearchFindsRulesByTheIDsPagesShow(t *testing.T) {
	reader := newLibraries(t)

	retryLimits := "acme/backend:practices/testing/verify-retry-limits"
	boundaries := "acme/backend:practices/testing/cover-boundary-cases"
	acmeGo := "acme/backend:techs/go/return-errors"
	betaGo := "Beta/rules:techs/go/name-packages-plainly"
	golang := "acme/backend:techs/golang/pass-context-first"
	retired := "acme/backend:practices/testing/retry-forever"
	for query, want := range map[string][]string{
		"verify-retry-limits":     {retryLimits},
		"retry-limits":            {retryLimits},
		"practices/testing":       {boundaries, retryLimits, retired},
		"techs/go":                {acmeGo, betaGo},
		"techs/golang":            {golang},
		"acme/backend":            {boundaries, retryLimits, acmeGo, golang, retired},
		"Beta/rules":              {betaGo},
		"backend":                 {boundaries, retryLimits, acmeGo, golang, retired},
		"Beta/rules:techs/go":     {betaGo},
		retryLimits:               {retryLimits},
		"acme/backend:techs":      {acmeGo, golang},
		"stranger/rules":          {},
		"techs/go -return-errors": {betaGo},
		// Only the alternative that joins words matches IDs: practices alone finds nothing, as techs does.
		"practices or techs/golang": {golang},
		"retry-limits -practices":   {retryLimits},
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
		if noWords && (len(got.Rows) > 0 || got.Total > 0) {
			t.Errorf("%q found %q", query, sourceIDs(got))
		}
	}
}

// publicRules is shaped like fabricahq/public-rules: rules whose titles, reading guidance, impact descriptions, and
// bodies share common words, as real rules do, so ranking has to tell a rule about a subject from one that mentions it.
var (
	errorBoundaries = summarized(rule("techs/tanstack-query/err-error-boundaries", "Reset query errors when an error boundary retries",
		"Before writing or reviewing React error boundaries around TanStack Query.", "Call reset when the boundary retries."),
		"An error boundary that retries without resetting shows the same error again.")
	retryLimits = rule("practices/testing/verify-retry-limits", "Verify retry limits",
		"When adding or changing retries.", "Test that every retry policy stops.")
	contractErrors = rule("techs/go/errors-use-contract-errors-deliberately", "Expose error identity only for contract errors",
		"Before defining sentinel or typed errors in Go.", "Wrap with %w only the errors callers check.")
	returnErrors = rule("techs/go/return-errors", "Return errors to callers",
		"When a function fails.", "Handle each error once, by returning it.")
	sharedRequests = summarized(rule("techs/react/client-share-data-requests", "Share client data requests through a caching data layer",
		"Before writing client-side data fetching in React.", "Fetch through one cache."),
		"Fetching in each component requires hand-written loading, error, and race handling.")
	narrowUnknown = rule("techs/typescript/narrow-unknown-values", "Narrow unknown values before use",
		"Before reading a value of type unknown.", "A type guard replaces error handling at each use.")
	gooseDirectory = rule("techs/goose/migrations-directory-contains-only-sql", "Keep the SQL migration directory for migration files only",
		"Before adding files to the goose migration directory, or adding a Go migration.", "Keep notes elsewhere.")
	gooseAnnotations = rule("techs/goose/annotate-statements", "Annotate statements that need it",
		"Before writing a migration with several statements.", "Mark each statement block.")
	independentTests = rule("practices/testing/keep-tests-independent", "Keep tests independent",
		"When tests share data or services.", "A test that depends on another's state fails alone.")

	publicRules = newLibrary("31", "fabricahq", "public-rules", []domain.Group{
		{Path: "practices/testing", Name: "Testing"}, {Path: "techs/go", Name: "Go"}, {Path: "techs/goose", Name: "Goose"},
		{Path: "techs/react", Name: "React"}, {Path: "techs/tanstack-query", Name: "TanStack Query"},
		{Path: "techs/typescript", Name: "TypeScript"},
	}, errorBoundaries, retryLimits, contractErrors, returnErrors, sharedRequests, narrowUnknown, gooseDirectory,
		gooseAnnotations, independentTests)
	publicRulesGroups = []domain.CanonicalGroup{
		{ID: "practices/testing", Name: "Testing"}, {ID: "techs/go", Name: "Go"}, {ID: "techs/goose", Name: "Goose"},
		{ID: "techs/react", Name: "React"}, {ID: "techs/tanstack-query", Name: "TanStack Query"},
		{ID: "techs/typescript", Name: "TypeScript"},
	}
)

// summarized returns r with impactDescription on its current version.
func summarized(r domain.Rule, impactDescription string) domain.Rule {
	r.Versions = slices.Clone(r.Versions)
	r.Versions[len(r.Versions)-1].Content.ImpactDescription = impactDescription
	return r
}

// searchPublicRules stores publicRules in a new database and returns a function that searches it as the web
// function's role.
func searchPublicRules(t *testing.T) func(query string) views.RuleResults {
	t.Helper()
	db, connString := databasetest.New(t)
	if _, err := postgres.New(db).ReplaceLibrary(context.Background(), publicRules); err != nil {
		t.Fatal(err)
	}
	reader := postgres.New(databasetest.AsWebRole(t, connString))
	vetted := []domain.LibraryKey{{Host: domain.GitHub, RepositoryID: "31"}}
	return func(query string) views.RuleResults {
		t.Helper()
		results, err := reader.Rules(context.Background(), vetted, publicRulesGroups, searchList(query), 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		return results
	}
}

// paths returns each result's rule ID.
func paths(results views.RuleResults) []string {
	ids := []string{}
	for _, r := range results.Rows {
		ids = append(ids, r.Rule.Path)
	}
	return ids
}

// A title made mostly of the word searched for says the rule is about it, more than a long title that mentions it.
// Leaving a word out keeps that order rather than falling back to titles.
func TestSearchRanksATitleAboutTheWordAboveALongerTitleThatMentionsIt(t *testing.T) {
	search := searchPublicRules(t)

	for _, query := range []string{"retry", "retry -sentinel", "retries"} {
		got := paths(search(query))
		want := []string{retryLimits.Path, errorBoundaries.Path}
		if !slices.Equal(got, want) {
			t.Errorf("%q found %q, want %q", query, got, want)
		}
	}
}

// A rule that holds every word comes before any that lacks one, as the results' summary counts them. Within each,
// rules rank by where they hold the words, and each result that lacks a word names it.
func TestSearchRanksRulesHoldingEveryWordFirstAndNamesTheTermsOthersLack(t *testing.T) {
	search := searchPublicRules(t)

	got := search("error handling")

	want := []string{
		returnErrors.Path,    // both words, error in its title
		sharedRequests.Path,  // both words, in its impact description
		narrowUnknown.Path,   // both words, only in its body
		contractErrors.Path,  // error in its title, without handling
		errorBoundaries.Path, // error in a longer title, without handling
	}
	if !slices.Equal(paths(got), want) || got.Total != 5 || got.Complete != 3 {
		t.Fatalf("got %q of %d, %d complete; want %q of 5, 3 complete", paths(got), got.Total, got.Complete, want)
	}
	for _, r := range got.Rows {
		var wantMissing []string
		if r.Rule.Path == contractErrors.Path || r.Rule.Path == errorBoundaries.Path {
			wantMissing = []string{"handling"}
		}
		if !slices.Equal(r.Missing, wantMissing) {
			t.Errorf("%s lacks %q, want %q", r.Rule.Path, r.Missing, wantMissing)
		}
	}
}

// A technology's rules come before a rule that only names it, and a short word never matches a longer one it starts:
// go finds no goose rule that doesn't mention Go.
func TestSearchRanksAGroupsRulesAboveRulesThatMentionItsName(t *testing.T) {
	search := searchPublicRules(t)

	got := paths(search("go"))

	want := []string{contractErrors.Path, returnErrors.Path, gooseDirectory.Path}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// A rule's ID, copied from its page, finds that rule first, though other rules hold the same words.
func TestSearchRanksTheRuleAnIDNamesFirst(t *testing.T) {
	search := searchPublicRules(t)

	for _, query := range []string{"keep-tests-independent", "fabricahq/public-rules:practices/testing/keep-tests-independent"} {
		got := paths(search(query))
		if len(got) == 0 || got[0] != independentTests.Path {
			t.Errorf("%q found %q, want %s first", query, got, independentTests.Path)
		}
	}
}

// However strongly a rule matches some of the words, it never comes before one that holds them all.
func TestSearchNeverRanksARuleLackingAWordAboveOneHoldingEvery(t *testing.T) {
	search := searchPublicRules(t)

	for _, query := range []string{"error handling", "retry go", "typescript testing", "react errors", "goose go migration", "tests retries"} {
		got := search(query)
		seenPartial, complete := false, 0
		for _, r := range got.Rows {
			if len(r.Missing) > 0 {
				seenPartial = true
				continue
			}
			complete++
			if seenPartial {
				t.Errorf("%q ranks %s, which holds every word, after a rule that lacks one: %q", query, r.Rule.Path, paths(got))
			}
		}
		if complete != got.Complete {
			t.Errorf("%q: %d results hold every word, but the count says %d", query, complete, got.Complete)
		}
	}
}
