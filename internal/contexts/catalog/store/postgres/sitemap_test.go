package postgres_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// The sitemap reads what search engines may index: each vetted library, when its latest release was tagged, and its
// current rules, each with when its current version was published, and the groups that hold them. A retired rule, a
// group with no current rule, and a library that's only listed stay out.
func TestSitemapReadsOnlyVettedLibrariesCurrentRulesAndGroups(t *testing.T) {
	reader := newCatalog(t)

	got, err := reader.Sitemap(context.Background(), vetted, 10)

	if err != nil {
		t.Fatal(err)
	}
	want := views.Sitemap{
		Libraries: []views.SitemapLibrary{{
			Owner: "example", Name: "rules", Updated: day(3),
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

// Past maxRules, the sitemap reads the first rules in owner, name, and ID order, and says it left some out.
func TestSitemapReadsAtMostMaxRules(t *testing.T) {
	reader := newCatalog(t)

	for maxRules, want := range map[int]views.Sitemap{
		1: {Libraries: []views.SitemapLibrary{{Owner: "example", Name: "rules", Updated: day(3),
			Rules: []views.SitemapRule{{Path: "practices/testing/verify-retry-limits", Updated: day(2)}}}}, Truncated: true},
		2: {Libraries: []views.SitemapLibrary{{Owner: "example", Name: "rules", Updated: day(3),
			Rules: []views.SitemapRule{{Path: "practices/testing/verify-retry-limits", Updated: day(2)}, {Path: "techs/go/return-errors", Updated: day(3)}}}}},
	} {
		got, err := reader.Sitemap(context.Background(), vetted, maxRules)

		if err != nil {
			t.Fatal(err)
		}
		got.Groups = nil
		if !reflect.DeepEqual(got, want) {
			t.Errorf("at most %d: got %+v, want %+v", maxRules, got, want)
		}
	}
}
