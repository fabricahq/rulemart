package postgres_test

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// A retired rule keeps a page: its last version, how it was retired, and what replaced it, by its newest title.
func TestRulePageReadsARetiredRuleAndItsReplacement(t *testing.T) {
	reader := newCatalog(t)

	page, err := reader.RulePage(context.Background(), vetted, "example", "rules", "practices/testing/check-retry-backoff")

	if err != nil {
		t.Fatal(err)
	}
	r := page.Rule
	if r.Title != "Check retry backoff" || r.HTML != "" || r.Version != v(1, 0, 0) || r.Release != 1 || r.Retirement == nil {
		t.Fatalf("rule is %+v", r)
	}
	retirement := *r.Retirement
	if retirement.Release != 3 || !retirement.RetiredAt.Equal(day(3)) || !slices.Equal(retirement.Summaries, []string{"Merge it."}) {
		t.Errorf("retirement is %+v", retirement)
	}
	if want := (views.RuleRef{Path: "practices/testing/verify-retry-limits", Title: "Verify retry limits"}); retirement.ReplacedBy == nil || *retirement.ReplacedBy != want {
		t.Errorf("replaced by %+v, want %+v", retirement.ReplacedBy, want)
	}
	if len(page.Versions) != 1 || page.Versions[0].Version != v(1, 0, 0) {
		t.Errorf("versions are %+v", page.Versions)
	}
}

// A rule's page names the retired rules that its retirement notes named it the replacement of.
func TestRulePageNamesTheRulesItReplaced(t *testing.T) {
	reader := newCatalog(t)

	page, err := reader.RulePage(context.Background(), vetted, "example", "rules", "practices/testing/verify-retry-limits")

	if err != nil {
		t.Fatal(err)
	}
	if page.Rule.Retirement != nil {
		t.Errorf("a current rule has retirement %+v", page.Rule.Retirement)
	}
	want := []views.RuleRef{{Path: "practices/testing/check-retry-backoff", Title: "Check retry backoff", RetiredIn: 3}}
	if !slices.Equal(page.Replaces, want) {
		t.Errorf("replaces %+v, want %+v", page.Replaces, want)
	}
}

func TestRuleComparisonReadsBothVersionsText(t *testing.T) {
	reader := newCatalog(t)

	comparison, err := reader.RuleComparison(context.Background(), vetted, "example", "rules", "techs/go/return-errors", v(1, 0, 0), v(2, 0, 0), 1<<20)

	if err != nil {
		t.Fatal(err)
	}
	if comparison.Page.Rule.Path != "techs/go/return-errors" || comparison.From != v(1, 0, 0) || comparison.To != v(2, 0, 0) {
		t.Errorf("compared %s %s...%s", comparison.Page.Rule.Path, comparison.From, comparison.To)
	}
	text := comparison.Text
	if text.State != views.TextShown || !strings.Contains(text.Old, "version 1.0.0") || !strings.Contains(text.New, "version 2.0.0") {
		t.Errorf("text is %+v", text)
	}
}

// A page compares a bounded amount of text, and a version a release before every version kept its content stored
// has none to compare.
func TestRuleComparisonReadsNoTextItCantShow(t *testing.T) {
	reader := newCatalog(t)
	ctx := context.Background()
	pair := 2 * int64(len(content("Return errors", v(1, 0, 0)).Markdown))

	for name, tc := range map[string]struct {
		maxBytes int64
		want     views.TextState
	}{
		"exactly the limit": {pair, views.TextShown},
		"a byte over it":    {pair - 1, views.TextTooLarge},
	} {
		t.Run(name, func(t *testing.T) {
			comparison, err := reader.RuleComparison(ctx, vetted, "example", "rules", "techs/go/return-errors", v(1, 0, 0), v(2, 0, 0), tc.maxBytes)

			if err != nil || comparison.Text.State != tc.want {
				t.Fatalf("got %+v, %v; want state %v", comparison.Text, err, tc.want)
			}
			if tc.want != views.TextShown && comparison.Text.Old+comparison.Text.New != "" {
				t.Errorf("read text it can't show: %+v", comparison.Text)
			}
		})
	}
}

func TestRuleComparisonFindsAVersionStoredWithoutItsText(t *testing.T) {
	s, connString := newStore(t)
	replace(t, s, exampleRules)
	lines(t, connString, `UPDATE rule_versions SET title = NULL, impact = NULL, impact_description = NULL, when_to_read = NULL,
		markdown = NULL WHERE html IS NULL RETURNING id::text`)

	comparison, err := s.RuleComparison(context.Background(), vetted, "example", "rules", "techs/go/return-errors", v(1, 0, 0), v(2, 0, 0), 1<<20)

	if err != nil || comparison.Text.State != views.TextMissing {
		t.Fatalf("got %+v, %v; want the text missing", comparison.Text, err)
	}
}

// A library's history is every release, and every rule's versions, oldest first, retired rules included.
func TestLibraryHistoryReadsEveryReleaseAndVersion(t *testing.T) {
	reader := newCatalog(t)

	history, err := reader.LibraryHistory(context.Background(), vetted, "example", "rules")

	if err != nil {
		t.Fatal(err)
	}
	if history.Library.Owner != "example" || len(history.Releases) != 3 || history.Releases[2].Number != 3 ||
		!history.Releases[2].TaggedAt.Equal(day(3)) {
		t.Errorf("library %+v with releases %+v", history.Library, history.Releases)
	}
	var got []string
	for _, r := range history.Rules {
		var versions []string
		for _, version := range r.Versions {
			versions = append(versions, fmt.Sprintf("%s %s %s %v", version.Version, domain.ReleaseTag(version.Release), version.Change, version.Summaries))
		}
		got = append(got, fmt.Sprintf("%s %q: %s; retired in %d by %q %v", r.Path, r.Title, strings.Join(versions, ", "), r.RetiredIn, r.ReplacedBy, r.RetirementSummaries))
	}
	want := []string{
		`practices/legacy/old-habit "Old habit": 1.0.0 release/1 new [Add the rule.]; retired in 3 by "" [Drop it.]`,
		`practices/testing/check-retry-backoff "Check retry backoff": 1.0.0 release/1 new [Add the rule.]; retired in 3 by "practices/testing/verify-retry-limits" [Merge it.]`,
		`practices/testing/verify-retry-limits "Verify retry limits": 1.0.0 release/1 new [Add the rule.], 1.1.0 release/2 minor [Count timeouts.]; retired in 0 by "" []`,
		`techs/go/return-errors "Return errors": 1.0.0 release/1 new [Add the rule.], 2.0.0 release/3 major [Require context. Add an example.]; retired in 0 by "" []`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("rules are\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// Comparing two releases reads the text of each rule that changed between them, and nothing for a rule that didn't,
// or that was added or retired.
func TestReleaseComparisonReadsTheTextOfRulesThatChanged(t *testing.T) {
	reader := newCatalog(t)
	ctx := context.Background()

	_, texts, err := reader.ReleaseComparison(ctx, vetted, "example", "rules", 1, 3, 1<<20)

	if err != nil {
		t.Fatal(err)
	}
	if got := slices.Sorted(maps.Keys(texts)); !slices.Equal(got, []string{"practices/testing/verify-retry-limits", "techs/go/return-errors"}) {
		t.Fatalf("read text of %q", got)
	}
	if text := texts["techs/go/return-errors"]; text.State != views.TextShown || !strings.Contains(text.Old, "1.0.0") || !strings.Contains(text.New, "2.0.0") {
		t.Errorf("return-errors text is %+v", text)
	}

	// Release 2 changed only verify-retry-limits.
	_, texts, err = reader.ReleaseComparison(ctx, vetted, "example", "rules", 1, 2, 1<<20)
	if got := slices.Sorted(maps.Keys(texts)); err != nil || !slices.Equal(got, []string{"practices/testing/verify-retry-limits"}) {
		t.Fatalf("read text of %q, %v", got, err)
	}
}

// Texts are read in path order while they fit what a page compares, so the first rule's pair is read, and the next
// one's, which passes the limit, isn't.
func TestReleaseComparisonReadsTextsWithinItsLimitInPathOrder(t *testing.T) {
	reader := newCatalog(t)
	first := int64(len(content("Verify retry limits", v(1, 0, 0)).Markdown) + len(content("Verify retry limits", v(1, 1, 0)).Markdown))

	_, texts, err := reader.ReleaseComparison(context.Background(), vetted, "example", "rules", 1, 3, first)

	if err != nil {
		t.Fatal(err)
	}
	if texts["practices/testing/verify-retry-limits"].State != views.TextShown || texts["techs/go/return-errors"].State != views.TextTooLarge {
		t.Fatalf("texts are %+v", texts)
	}
}
