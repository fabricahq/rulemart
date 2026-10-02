package app_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// histories stands in for the store's reads of library histories and rule comparisons, recording what each asked
// for.
type histories struct {
	store.Reader
	history views.LibraryHistory
	texts   map[string]views.ComparedText
	// compared records each comparison of releases asked for, as "from...to", and each rule comparison's versions.
	compared []string
	maxBytes []int64
}

func (h *histories) LibraryHistory(context.Context, []domain.LibraryKey, string, string) (views.LibraryHistory, error) {
	return h.history, nil
}

func (h *histories) ReleaseComparison(_ context.Context, _ []domain.LibraryKey, _, _ string, from, to int, maxBytes int64) (views.LibraryHistory, map[string]views.ComparedText, error) {
	h.compared, h.maxBytes = append(h.compared, fmt.Sprintf("%d...%d", from, to)), append(h.maxBytes, maxBytes)
	return h.history, h.texts, nil
}

func (h *histories) RuleComparison(_ context.Context, _ []domain.LibraryKey, _, _, path string, from, to coderules.RuleVersion, maxBytes int64) (views.RuleComparison, error) {
	h.compared, h.maxBytes = append(h.compared, path+" "+from.String()+"..."+to.String()), append(h.maxBytes, maxBytes)
	return views.RuleComparison{Page: views.RulePage{Rule: views.Rule{Group: "techs/go"}}, From: from, To: to}, nil
}

// day returns noon UTC on day n of September 2026.
func day(n int) time.Time { return time.Date(2026, 9, n, 12, 0, 0, 0, time.UTC) }

func version(text string) coderules.RuleVersion {
	v, err := coderules.ParseRuleVersion(text, "test")
	if err != nil {
		panic(err)
	}
	return v
}

// published returns a version published in release n.
func published(text string, n int, change coderules.Change, summaries ...string) views.Version {
	return views.Version{Version: version(text), Release: n, Change: change, Summaries: summaries}
}

// fourReleases is a library whose release 2 changed verify-retry-limits and retired check-retry-backoff for it,
// release 3 changed nothing but shared files, and release 4 changed return-errors twice over, added name-tests, and
// retired a rule release 2 added.
var fourReleases = views.LibraryHistory{
	Library: views.Library{Owner: "example", Name: "rules", LatestRelease: 4},
	Releases: []views.Release{
		{Number: 1, TaggedAt: day(1)}, {Number: 2, TaggedAt: day(2), UpdatesSharedFiles: true},
		{Number: 3, TaggedAt: day(3), UpdatesSharedFiles: true}, {Number: 4, TaggedAt: day(4)},
	},
	Rules: []views.RuleHistory{
		{Path: "practices/testing/check-retry-backoff", Title: "Check retry backoff", RetiredIn: 2,
			ReplacedBy: "practices/testing/verify-retry-limits", RetirementSummaries: []string{"Merge it."},
			Versions: []views.Version{published("1.0.0", 1, coderules.ChangeNew, "Add the rule.")}},
		{Path: "practices/testing/short-lived", Title: "Short lived", RetiredIn: 4, RetirementSummaries: []string{"Drop it."},
			Versions: []views.Version{published("1.0.0", 2, coderules.ChangeNew, "Try it.")}},
		{Path: "practices/testing/verify-retry-limits", Title: "Verify retry limits",
			Versions: []views.Version{
				published("1.0.0", 1, coderules.ChangeNew, "Add the rule."),
				published("2.0.0", 2, coderules.ChangeMajor, "Lower the limit."),
			}},
		{Path: "techs/go/name-tests", Title: "Name tests",
			Versions: []views.Version{published("1.0.0", 4, coderules.ChangeNew, "Add the rule.")}},
		{Path: "techs/go/return-errors", Title: "Return errors",
			Versions: []views.Version{
				published("1.0.0", 1, coderules.ChangeNew, "Add the rule."),
				published("1.0.1", 2, coderules.ChangePatch, "Fix a typo."),
				published("1.1.0", 4, coderules.ChangeMinor, "Add an example."),
			}},
	},
}

// describeChanges writes each change as "<path> <change> <from>-><to> [<versions>] <retirement>".
func describeChanges(changes []views.RuleChange) []string {
	var result []string
	for _, c := range changes {
		var versions []string
		for _, v := range c.Versions {
			versions = append(versions, v.Version.String())
		}
		line := fmt.Sprintf("%s %q %s %s->%s [%s]", c.Rule.Path, c.Rule.Title, c.Change, c.From, c.To, strings.Join(versions, " "))
		if c.Change == coderules.ChangeRetired {
			line += fmt.Sprintf(" %v", c.RetirementSummaries)
			if c.ReplacedBy != nil {
				line += fmt.Sprintf(" by %s %q", c.ReplacedBy.Path, c.ReplacedBy.Title)
			}
		}
		result = append(result, line)
	}
	return result
}

// Each release lists what it changed since the release before it, as Code Rules' release notes do, newest first, and
// the version of every rule after it.
func TestReleasesPageListsWhatEachReleaseChanged(t *testing.T) {
	pages := app.Pages{Store: &histories{history: fourReleases}}

	page, err := pages.ReleasesPage(context.Background(), "example", "rules")

	if err != nil {
		t.Fatal(err)
	}
	var numbers []int
	for _, r := range page.Releases {
		numbers = append(numbers, r.Release.Number)
	}
	if !slices.Equal(numbers, []int{4, 3, 2, 1}) || page.Library.Owner != "example" {
		t.Fatalf("releases are %v of %+v, want newest first", numbers, page.Library)
	}
	want := map[int][]string{
		4: {
			`practices/testing/short-lived "Short lived" retired 1.0.0->0.0.0 [] [Drop it.]`,
			`techs/go/name-tests "Name tests" new 0.0.0->1.0.0 [1.0.0]`,
			`techs/go/return-errors "Return errors" minor 1.0.1->1.1.0 [1.1.0]`,
		},
		3: nil,
		2: {
			`practices/testing/check-retry-backoff "Check retry backoff" retired 1.0.0->0.0.0 [] [Merge it.] by practices/testing/verify-retry-limits "Verify retry limits"`,
			`practices/testing/short-lived "Short lived" new 0.0.0->1.0.0 [1.0.0]`,
			`practices/testing/verify-retry-limits "Verify retry limits" major 1.0.0->2.0.0 [2.0.0]`,
			`techs/go/return-errors "Return errors" patch 1.0.0->1.0.1 [1.0.1]`,
		},
		1: {
			`practices/testing/check-retry-backoff "Check retry backoff" new 0.0.0->1.0.0 [1.0.0]`,
			`practices/testing/verify-retry-limits "Verify retry limits" new 0.0.0->1.0.0 [1.0.0]`,
			`techs/go/return-errors "Return errors" new 0.0.0->1.0.0 [1.0.0]`,
		},
	}
	for _, notes := range page.Releases {
		if got := describeChanges(notes.Changes); !slices.Equal(got, want[notes.Release.Number]) {
			t.Errorf("release/%d changes\n%s\nwant\n%s", notes.Release.Number, strings.Join(got, "\n"), strings.Join(want[notes.Release.Number], "\n"))
		}
	}
	wantVersions := []views.RuleVersionRef{
		{Path: "practices/testing/verify-retry-limits", Version: version("2.0.0")},
		{Path: "techs/go/name-tests", Version: version("1.0.0")},
		{Path: "techs/go/return-errors", Version: version("1.1.0")},
	}
	if got := page.Releases[0].Versions; !slices.Equal(got, wantVersions) {
		t.Errorf("release/4's versions are %+v, want %+v", got, wantVersions)
	}
}

// Comparing releases spans every release between them: a rule changed more than once shows its largest change and
// each version, newest first, and a rule added and retired between them doesn't show. The comparison reads the text
// of each changed rule within what a page compares.
func TestReleaseComparisonSpansTheReleasesBetween(t *testing.T) {
	h := &histories{history: fourReleases, texts: map[string]views.ComparedText{
		"techs/go/return-errors": {Old: "old", New: "new"},
	}}
	pages := app.Pages{Store: h}

	comparison, err := pages.ReleaseComparison(context.Background(), "example", "rules", 1, 4)

	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`practices/testing/check-retry-backoff "Check retry backoff" retired 1.0.0->0.0.0 [] [Merge it.] by practices/testing/verify-retry-limits "Verify retry limits"`,
		`practices/testing/verify-retry-limits "Verify retry limits" major 1.0.0->2.0.0 [2.0.0]`,
		`techs/go/name-tests "Name tests" new 0.0.0->1.0.0 [1.0.0]`,
		`techs/go/return-errors "Return errors" minor 1.0.0->1.1.0 [1.1.0 1.0.1]`,
	}
	if got := describeChanges(comparison.Changes); !slices.Equal(got, want) {
		t.Errorf("changes\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if text := comparison.Changes[3].Text; text.Old != "old" || text.New != "new" {
		t.Errorf("return-errors text is %+v", text)
	}
	if comparison.From != 1 || comparison.To != 4 || len(comparison.Releases) != 4 || !slices.Equal(h.maxBytes, []int64{app.MaxComparedBytes}) {
		t.Errorf("compared %d...%d of %d releases, reading at most %v bytes", comparison.From, comparison.To, len(comparison.Releases), h.maxBytes)
	}
}

// Releases compare older first, whichever way round they're asked for, and the same release compares to nothing.
func TestReleaseComparisonPutsTheOlderReleaseFirst(t *testing.T) {
	for name, tc := range map[string]struct {
		from, to int
		want     string
		changes  int
	}{
		"newer first":      {3, 2, "2...3", 0},
		"the same release": {2, 2, "2...2", 0},
		"older first":      {1, 2, "1...2", 4},
	} {
		t.Run(name, func(t *testing.T) {
			h := &histories{history: fourReleases}

			comparison, err := app.Pages{Store: h}.ReleaseComparison(context.Background(), "example", "rules", tc.from, tc.to)

			if err != nil || !slices.Equal(h.compared, []string{tc.want}) || len(comparison.Changes) != tc.changes {
				t.Fatalf("compared %v with %d changes, %v; want %s with %d", h.compared, len(comparison.Changes), err, tc.want, tc.changes)
			}
		})
	}
}

func TestReleaseComparisonRefusesAReleaseTheLibraryDoesntHave(t *testing.T) {
	for name, releases := range map[string][2]int{"release 0": {0, 2}, "past the latest": {1, 5}} {
		t.Run(name, func(t *testing.T) {
			_, err := app.Pages{Store: &histories{history: fourReleases}}.ReleaseComparison(context.Background(), "example", "rules", releases[0], releases[1])

			if !errors.Is(err, app.ErrNotFound) {
				t.Fatalf("got %v, want app.ErrNotFound", err)
			}
		})
	}
}

// Versions compare older first, whichever way round they're asked for, within what a page compares, and the page
// shows the rule's group as canonical or not.
func TestRuleComparisonPutsTheOlderVersionFirst(t *testing.T) {
	h := &histories{}
	pages := app.Pages{Store: h, Groups: canonicalList(t)}

	comparison, err := pages.RuleComparison(context.Background(), "example", "rules", "techs/go/return-errors", version("2.0.0"), version("1.0.0"))

	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(h.compared, []string{"techs/go/return-errors 1.0.0...2.0.0"}) || !slices.Equal(h.maxBytes, []int64{app.MaxComparedBytes}) {
		t.Errorf("compared %v within %v", h.compared, h.maxBytes)
	}
	if comparison.Page.Rule.CanonicalGroup == nil || comparison.Page.Rule.CanonicalGroup.Name != "Go" {
		t.Errorf("the rule's group is %+v", comparison.Page.Rule.CanonicalGroup)
	}
}
