package postgres_test

import (
	"context"
	"slices"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/platform/database/databasetest"
)

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

// summarized returns r with impactDescription.
func summarized(r domain.Rule, impactDescription string) domain.Rule {
	c := *r.Content
	c.ImpactDescription = impactDescription
	r.Content = &c
	return r
}

// searchPublicRules stores publicRules in a new database and returns a function that searches it as the web
// function's role.
func searchPublicRules(t *testing.T) func(query string) views.SearchResults {
	t.Helper()
	db, connString := databasetest.New(t)
	if _, err := postgres.New(db).ReplaceLibrary(context.Background(), publicRules); err != nil {
		t.Fatal(err)
	}
	reader := postgres.New(databasetest.AsWebRole(t, connString))
	vetted := []domain.LibraryKey{{Host: domain.GitHub, RepositoryID: "31"}}
	return func(query string) views.SearchResults {
		t.Helper()
		results, err := reader.Search(context.Background(), vetted, publicRulesGroups, domain.ParseSearchQuery(query), 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		return results
	}
}

// paths returns each result's rule ID.
func paths(results views.SearchResults) []string {
	ids := []string{}
	for _, r := range results.Results {
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
	for _, r := range got.Results {
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
		for _, r := range got.Results {
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
