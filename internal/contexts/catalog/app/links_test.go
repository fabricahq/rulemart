package app_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// replacedTwice is fabricahq/code-rules-test-library's history, in short: release 2 retired check-retry-backoff for
// verify-retries, which it added; release 4 retired verify-retries for verify-retry-limits, and renamed name-tests to
// name-tests-by-behavior, adding the new ID under the old title. Release 5 changed shared files only.
var replacedTwice = views.LibraryHistory{
	Library: views.Library{Owner: "fabricahq", Name: "code-rules-test-library", LatestRelease: 5},
	Releases: []views.Release{
		{Number: 1}, {Number: 2}, {Number: 3}, {Number: 4}, {Number: 5, UpdatesSharedFiles: true},
	},
	Rules: []views.RuleHistory{
		{Path: "practices/testing/check-retry-backoff", Title: "Check retry backoff", RetiredIn: 2,
			ReplacedBy: "practices/testing/verify-retries", RetirementSummaries: []string{"Covered by verify-retries."},
			Versions: []views.Version{titled(published("1.0.0", 1, coderules.ChangeNew, "Add the rule."), "Check retry backoff")}},
		{Path: "practices/testing/verify-retries", Title: "Verify retries", RetiredIn: 4,
			ReplacedBy: "practices/testing/verify-retry-limits", RetirementSummaries: []string{"Folded into the limits rule."},
			Versions: []views.Version{titled(published("1.0.0", 2, coderules.ChangeNew, "Add the rule."), "Verify retries")}},
		{Path: "practices/testing/verify-retry-limits", Title: "Verify retry limits",
			Versions: []views.Version{
				titled(published("1.0.0", 1, coderules.ChangeNew, "Add the rule."), "Verify retry limits"),
				titled(published("2.0.0", 4, coderules.ChangeMajor, "Lower the limit."), "Verify retry limits"),
			}},
		{Path: "techs/go/name-tests", Title: "Name tests after behavior", RetiredIn: 4,
			ReplacedBy: "techs/go/name-tests-by-behavior", RetirementSummaries: []string{"Rename it."},
			Versions: []views.Version{titled(published("1.0.0", 1, coderules.ChangeNew, "Add the rule."), "Name tests after behavior")}},
		{Path: "techs/go/name-tests-by-behavior", Title: "Name tests after behavior",
			Versions: []views.Version{titled(published("1.0.0", 4, coderules.ChangeNew, "Rename it."), "Name tests after behavior")}},
	},
}

// linksOf returns the links the store reads for history.
func linksOf(history views.LibraryHistory) []views.RuleLink {
	var links []views.RuleLink
	for _, r := range history.Rules {
		first, last := r.Versions[0], r.Versions[len(r.Versions)-1]
		links = append(links, views.RuleLink{
			Path: r.Path, Title: r.Title, RetiredIn: r.RetiredIn, ReplacedBy: r.ReplacedBy,
			FirstRelease: first.Release, FirstTitle: first.Title, LastTitle: last.Title,
		})
	}
	return links
}

// A retired rule's replacement chain runs to the rule that's current now, on a release's card and in a comparison,
// naming its first replacement by its title then; a rename shows once, under its new ID, from the old one.
func TestChangesFollowReplacementsAndShowRenames(t *testing.T) {
	h := &histories{history: replacedTwice}
	pages := app.Pages{Store: h}
	ctx := context.Background()

	releases, err := pages.ReleasesPage(ctx, "fabricahq", "code-rules-test-library", 0)
	if err != nil {
		t.Fatal(err)
	}
	comparison, err := pages.ReleaseComparison(ctx, "fabricahq", "code-rules-test-library", 1, 5)
	if err != nil {
		t.Fatal(err)
	}

	byRelease := map[int][]string{}
	for _, notes := range releases.Releases {
		byRelease[notes.Release.Number] = describeChanges(notes.Changes)
	}
	if got, want := byRelease[2], []string{
		`practices/testing/check-retry-backoff "Check retry backoff" retired 1.0.0->0.0.0 [] [Covered by verify-retries.] by practices/testing/verify-retries "Verify retries" by practices/testing/verify-retry-limits "Verify retry limits"`,
		`practices/testing/verify-retries "Verify retries" new 0.0.0->1.0.0 [1.0.0]`,
	}; !slices.Equal(got, want) {
		t.Errorf("release/2 changes\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got, want := byRelease[4], []string{
		`practices/testing/verify-retries "Verify retries" retired 1.0.0->0.0.0 [] [Folded into the limits rule.] by practices/testing/verify-retry-limits "Verify retry limits"`,
		`practices/testing/verify-retry-limits "Verify retry limits" major 1.0.0->2.0.0 [2.0.0]`,
		`techs/go/name-tests-by-behavior "Name tests after behavior" renamed 1.0.0->1.0.0 [1.0.0] from techs/go/name-tests "Name tests after behavior"`,
	}; !slices.Equal(got, want) {
		t.Errorf("release/4 changes\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got, want := describeChanges(comparison.Changes), []string{
		`practices/testing/check-retry-backoff "Check retry backoff" retired 1.0.0->0.0.0 [] [Covered by verify-retries.] by practices/testing/verify-retries "Verify retries" by practices/testing/verify-retry-limits "Verify retry limits"`,
		`practices/testing/verify-retry-limits "Verify retry limits" major 1.0.0->2.0.0 [2.0.0]`,
		`techs/go/name-tests-by-behavior "Name tests after behavior" renamed 1.0.0->1.0.0 [1.0.0] from techs/go/name-tests "Name tests after behavior"`,
	}; !slices.Equal(got, want) {
		t.Errorf("release/1...release/5 changes\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// A rename's text compares the old rule's last version with the new rule's.
	if want := "techs/go/name-tests-by-behavior techs/go/name-tests 1.0.0...techs/go/name-tests-by-behavior 1.0.0"; !strings.Contains(h.compared[0], want) {
		t.Errorf("read the text of %q, want it to include %q", h.compared[0], want)
	}
	if !comparison.SharedFiles {
		t.Error("the comparison doesn't say release/5 changed shared files")
	}
}

func TestComparisonSaysWhenOnlyEarlierReleasesChangedSharedFiles(t *testing.T) {
	comparison, err := app.Pages{Store: &histories{history: replacedTwice}}.ReleaseComparison(context.Background(), "fabricahq", "code-rules-test-library", 1, 4)

	if err != nil || comparison.SharedFiles {
		t.Fatalf("got shared files %v, %v; want none between release/1 and release/4", comparison.SharedFiles, err)
	}
}

// A retired rule's page follows its replacements to the rule current now, and says when it was renamed; the rule
// that replaced or renamed it names it.
func TestRulePagesFollowReplacementsAndRenames(t *testing.T) {
	pages := app.Pages{Store: &histories{history: replacedTwice, links: linksOf(replacedTwice)}}
	ctx := context.Background()
	page := func(path string) views.RulePage {
		t.Helper()
		p, err := pages.RulePage(ctx, "fabricahq", "code-rules-test-library", path)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}

	backoff := page("practices/testing/check-retry-backoff").Rule.Retirement
	if got := backoff.Replacements; len(got) != 2 || got[0].Path != "practices/testing/verify-retries" || got[1].Path != "practices/testing/verify-retry-limits" || backoff.Renamed {
		t.Errorf("check-retry-backoff was replaced by %+v, renamed %v", got, backoff.Renamed)
	}
	if renamed := page("techs/go/name-tests").Rule.Retirement; !renamed.Renamed || len(renamed.Replacements) != 1 {
		t.Errorf("name-tests's retirement is %+v, want a rename", renamed)
	}
	if p := page("techs/go/name-tests-by-behavior"); p.RenamedFrom == nil || p.RenamedFrom.Path != "techs/go/name-tests" || len(p.Replaces) != 0 {
		t.Errorf("name-tests-by-behavior renamed %+v and replaced %+v", p.RenamedFrom, p.Replaces)
	}
	if p := page("practices/testing/verify-retry-limits"); p.RenamedFrom != nil || len(p.Replaces) != 1 || p.Replaces[0].Path != "practices/testing/verify-retries" {
		t.Errorf("verify-retry-limits renamed %+v and replaced %+v", p.RenamedFrom, p.Replaces)
	}

	library, err := pages.LibraryPage(ctx, "fabricahq", "code-rules-test-library")
	if err != nil {
		t.Fatal(err)
	}
	var renamed []string
	replacements := map[string][]string{}
	for _, r := range library.Retired {
		if r.Renamed {
			renamed = append(renamed, r.Path)
		}
		for _, ref := range r.Replacements {
			replacements[r.Path] = append(replacements[r.Path], ref.Path)
		}
	}
	if !slices.Equal(renamed, []string{"techs/go/name-tests"}) {
		t.Errorf("renamed retired rules are %q", renamed)
	}
	// The All rules tab follows a retired rule's replacements to now, as its page does.
	if got := replacements["practices/testing/check-retry-backoff"]; !slices.Equal(got, []string{"practices/testing/verify-retries", "practices/testing/verify-retry-limits"}) {
		t.Errorf("check-retry-backoff's replacements on the All rules tab are %q", got)
	}
}

// A library's records could name replacements in a cycle; following them stops at a rule it already named.
func TestReplacementsStopAtACycle(t *testing.T) {
	cycle := []views.RuleLink{
		{Path: "a", RetiredIn: 2, ReplacedBy: "b", FirstRelease: 1},
		{Path: "b", RetiredIn: 2, ReplacedBy: "a", FirstRelease: 1},
	}
	h := views.LibraryHistory{Rules: []views.RuleHistory{{Path: "a", RetiredIn: 2, Versions: []views.Version{published("1.0.0", 1, coderules.ChangeNew)}}}}

	page, err := app.Pages{Store: &histories{history: h, links: cycle}}.RulePage(context.Background(), "o", "n", "a")

	if err != nil || len(page.Rule.Retirement.Replacements) != 1 {
		t.Fatalf("got %+v, %v", page.Rule.Retirement, err)
	}
}

// A release's card names a replacement by the title it had then, though it was renamed in a later release.
func TestChangesNameAReplacementByItsTitleThen(t *testing.T) {
	history := views.LibraryHistory{
		Library:  views.Library{LatestRelease: 3},
		Releases: []views.Release{{Number: 1}, {Number: 2}, {Number: 3}},
		Rules: []views.RuleHistory{
			{Path: "a/b/new", Title: "Verify retry limits", Versions: []views.Version{
				titled(published("1.0.0", 1, coderules.ChangeNew), "Verify retries"),
				titled(published("1.1.0", 3, coderules.ChangeMinor), "Verify retry limits"),
			}},
			{Path: "a/b/old", Title: "Old", RetiredIn: 2, ReplacedBy: "a/b/new",
				Versions: []views.Version{titled(published("1.0.0", 1, coderules.ChangeNew), "Old")}},
		},
	}

	page, err := app.Pages{Store: &histories{history: history}}.ReleasesPage(context.Background(), "o", "n", 0)

	if err != nil {
		t.Fatal(err)
	}
	if got := describeChanges(page.Releases[1].Changes); !slices.Equal(got, []string{`a/b/old "Old" retired 1.0.0->0.0.0 [] [] by a/b/new "Verify retries"`}) {
		t.Errorf("release/2 changes %q", got)
	}
}

// A long chain of replacements is followed only so far, so a library that retires a rule a release for hundreds of
// releases can't make a page's work grow with the square of its rules.
func TestReplacementsFollowABoundedChain(t *testing.T) {
	var links []views.RuleLink
	for i := range 300 {
		links = append(links, views.RuleLink{Path: fmt.Sprintf("r%03d", i), RetiredIn: i + 2, ReplacedBy: fmt.Sprintf("r%03d", i+1), FirstRelease: 1})
	}
	h := views.LibraryHistory{Rules: []views.RuleHistory{{Path: "r000", RetiredIn: 2, Versions: []views.Version{published("1.0.0", 1, coderules.ChangeNew)}}}}

	page, err := app.Pages{Store: &histories{history: h, links: links}}.RulePage(context.Background(), "o", "n", "r000")

	if err != nil || len(page.Rule.Retirement.Replacements) != app.MaxReplacements {
		t.Fatalf("followed %d replacements, %v; want %d", len(page.Rule.Retirement.Replacements), err, app.MaxReplacements)
	}
}
