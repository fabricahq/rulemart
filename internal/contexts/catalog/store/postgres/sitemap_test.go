package postgres_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/platform/database/databasetest"
)

// The sitemap reads what search engines may index: each vetted library, when its latest release was tagged, its groups
// that hold current rules, each with when the latest of their current versions was published, and its current rules,
// each with when its current version was published, and the groups that hold them. A retired rule, a group with no
// current rule, and a library that's only listed stay out.
func TestSitemapReadsOnlyVettedLibrariesCurrentRulesAndGroups(t *testing.T) {
	reader := newCatalog(t)

	got, err := reader.Sitemap(context.Background(), vetted, 10)

	if err != nil {
		t.Fatal(err)
	}
	want := views.Sitemap{
		Libraries: []views.SitemapLibrary{{
			Owner: "example", Name: "rules", Updated: day(3),
			Groups: []views.SitemapGroup{{Path: "practices/testing", Updated: day(2)}, {Path: "techs/go", Updated: day(3)}},
			Rules: []views.SitemapRule{
				{Path: "practices/testing/verify-retry-limits", Updated: day(2)},
				{Path: "techs/go/return-errors", Updated: day(3)},
			},
		}},
		Groups: []string{"practices/testing", "techs/go"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// Past maxEntries, the sitemap reads the first groups in ID order, then the first library groups and then the first
// rules, both in owner, name, and ID order, and says it left some out.
func TestSitemapReadsAtMostMaxGroupsAndRules(t *testing.T) {
	reader := newCatalog(t)
	library := func(groups []views.SitemapGroup, rules ...views.SitemapRule) []views.SitemapLibrary {
		return []views.SitemapLibrary{{Owner: "example", Name: "rules", Updated: day(3), Groups: groups, Rules: rules}}
	}
	libTesting := views.SitemapGroup{Path: "practices/testing", Updated: day(2)}
	libGo := views.SitemapGroup{Path: "techs/go", Updated: day(3)}
	retry := views.SitemapRule{Path: "practices/testing/verify-retry-limits", Updated: day(2)}
	returnErrors := views.SitemapRule{Path: "techs/go/return-errors", Updated: day(3)}
	groups := []string{"practices/testing", "techs/go"}

	for maxEntries, want := range map[int]views.Sitemap{
		1: {Libraries: library(nil), Groups: []string{"practices/testing"}, Truncated: true},
		3: {Libraries: library([]views.SitemapGroup{libTesting}), Groups: groups, Truncated: true},
		5: {Libraries: library([]views.SitemapGroup{libTesting, libGo}, retry), Groups: groups, Truncated: true},
		6: {Libraries: library([]views.SitemapGroup{libTesting, libGo}, retry, returnErrors), Groups: groups},
	} {
		got, err := reader.Sitemap(context.Background(), vetted, maxEntries)

		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("at most %d: got %+v, want %+v", maxEntries, got, want)
		}
	}
}

// A rule's ID may nest below its group's, as techs/go/errors/wrap does in techs/go, so a library group is the group
// the catalog files the rule in, not its ID's parent, and it is read once however its rules' IDs interleave.
func TestSitemapReadsALibraryGroupOnceWhateverItsRulesIDsNest(t *testing.T) {
	db, connString := databasetest.New(t)
	wrap := rule("techs/go/errors/wrap", "Wrap errors", "When returning an error.", "Wrap it.")
	wrap.Group = "techs/go"
	lib := newLibrary("31", "fabricahq", "rules", []domain.Group{goGroup},
		rule("techs/go/accept-interfaces", "Accept interfaces", "When taking a dependency.", "Take an interface."),
		wrap,
		rule("techs/go/return-errors", "Return errors", "When a function fails.", "Return it."),
	)
	if _, err := postgres.New(db).ReplaceLibrary(context.Background(), lib); err != nil {
		t.Fatal(err)
	}
	reader := postgres.New(databasetest.AsWebRole(t, connString))

	got, err := reader.Sitemap(context.Background(), []domain.LibraryKey{{Host: domain.GitHub, RepositoryID: "31"}}, 10)

	if err != nil {
		t.Fatal(err)
	}
	if want := []views.SitemapGroup{{Path: "techs/go", Updated: day(1)}}; len(got.Libraries) != 1 ||
		!reflect.DeepEqual(got.Libraries[0].Groups, want) {
		t.Fatalf("got %+v, want one library with groups %+v", got.Libraries, want)
	}
}
