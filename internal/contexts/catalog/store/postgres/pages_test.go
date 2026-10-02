package postgres_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
	"github.com/fabricahq/rulemart/internal/platform/database/databasetest"
)

// day returns noon UTC on day n of September 2026.
func day(n int) time.Time { return time.Date(2026, 9, n, 12, 0, 0, 0, time.UTC) }

func v(major, minor, patch int) coderules.RuleVersion {
	return coderules.RuleVersion{Major: major, Minor: minor, Patch: patch}
}

// content returns what version of a rule titled title published: a file whose body names the version.
func content(title string, version coderules.RuleVersion) domain.Content {
	return domain.Content{
		Title: title, Impact: "HIGH", ImpactDescription: "Prevents mistakes.", WhenToRead: "When changing " + title + ".",
		Markdown: "---\ntitle: " + title + "\n---\n\n" + title + ", version " + version.String() + ".\n",
	}
}

// withContent returns r with each version's content, titled title, and while r is current, its current version's
// HTML.
func withContent(r domain.Rule, title string) domain.Rule {
	r.Versions = slices.Clone(r.Versions)
	for i, version := range r.Versions {
		r.Versions[i].Content = content(title, version.Number)
	}
	if r.IsCurrent() {
		r.HTML = "<p>" + title + ".</p>\n"
	}
	return r
}

// exampleRules is a library of three releases. Release 2 changed verify-retry-limits; release 3 changed
// return-errors, a major change, and retired check-retry-backoff and old-habit, whose group practices/legacy has
// no current rule left.
var exampleRules = domain.Library{
	Repository: domain.Repository{
		Host: domain.GitHub, ID: "7", Owner: "example", Name: "rules", Description: "Example rules.",
		OwnerAvatarURL: "https://avatars.githubusercontent.com/u/1?v=4",
	},
	LicenseExpression: "MIT", LicenseFile: "LICENSE",
	Releases: []domain.Release{
		{Number: 1, CommitID: "1111111111111111111111111111111111111111", TaggedAt: day(1)},
		{Number: 2, CommitID: "2222222222222222222222222222222222222222", TaggedAt: day(2)},
		{Number: 3, CommitID: "3333333333333333333333333333333333333333", TaggedAt: day(3)},
	},
	Groups: []domain.Group{
		{Path: "practices/legacy", Name: "Legacy", Description: "Legacy rules.", WhenToRead: "Never."},
		{Path: "practices/testing", Name: "Testing", Description: "Testing rules.", WhenToRead: "When testing."},
		{Path: "techs/go", Name: "Go", Description: "Go rules.", WhenToRead: "When writing Go."},
	},
	Rules: []domain.Rule{
		withContent(domain.Rule{Path: "practices/legacy/old-habit", Group: "practices/legacy", RetiredIn: 3, RetirementSummaries: []string{"Drop it."},
			Versions: []domain.Version{{Number: v(1, 0, 0), Release: 1, Change: coderules.ChangeNew, Summaries: []string{"Add the rule."}}}}, "Old habit"),
		withContent(domain.Rule{Path: "practices/testing/check-retry-backoff", Group: "practices/testing", RetiredIn: 3,
			ReplacedBy: "practices/testing/verify-retry-limits", RetirementSummaries: []string{"Merge it."},
			Versions: []domain.Version{{Number: v(1, 0, 0), Release: 1, Change: coderules.ChangeNew, Summaries: []string{"Add the rule."}}}}, "Check retry backoff"),
		withContent(domain.Rule{Path: "practices/testing/verify-retry-limits", Group: "practices/testing",
			Versions: []domain.Version{
				{Number: v(1, 0, 0), Release: 1, Change: coderules.ChangeNew, Summaries: []string{"Add the rule."}},
				{Number: v(1, 1, 0), Release: 2, Change: coderules.ChangeMinor, Summaries: []string{"Count timeouts."}},
			}}, "Verify retry limits"),
		withContent(domain.Rule{Path: "techs/go/return-errors", Group: "techs/go",
			Versions: []domain.Version{
				{Number: v(1, 0, 0), Release: 1, Change: coderules.ChangeNew, Summaries: []string{"Add the rule."}},
				{Number: v(2, 0, 0), Release: 3, Change: coderules.ChangeMajor, Summaries: []string{"Require context.", "Add an example."}},
			}}, "Return errors"),
	},
}

// unvetted returns a library that isn't vetted: exampleRules under another repository.
func unvetted() domain.Library {
	lib := exampleRules
	lib.Repository.ID, lib.Repository.Owner, lib.Repository.Name = "8", "stranger", "unvetted-rules"
	return lib
}

var vetted = []domain.LibraryKey{{Host: domain.GitHub, RepositoryID: "7"}}

// newCatalog stores exampleRules and an unvetted library in a new database, and returns a Store that reads it as
// the web function's role, so a table the migrations don't grant it fails these tests.
func newCatalog(t *testing.T) *postgres.Store {
	t.Helper()
	db, connString := databasetest.New(t)
	writer := postgres.New(db)
	for _, lib := range []domain.Library{exampleRules, unvetted()} {
		if _, err := writer.ReplaceLibrary(context.Background(), lib); err != nil {
			t.Fatal(err)
		}
	}
	return postgres.New(databasetest.AsWebRole(t, connString))
}

func TestHomePageListsOnlyVettedLibrariesAndTheirGroups(t *testing.T) {
	reader := newCatalog(t)

	got, groups, err := reader.HomePage(context.Background(), vetted)

	if err != nil {
		t.Fatal(err)
	}
	want := []views.LibraryCard{{Owner: "example", Name: "rules", Description: "Example rules.", OwnerAvatarURL: exampleRules.Repository.OwnerAvatarURL, Rules: 2}}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	ref := views.LibraryRef{Owner: "example", Name: "rules", OwnerAvatarURL: exampleRules.Repository.OwnerAvatarURL}
	wantGroups := []views.LibraryGroup{{Path: "practices/testing", Library: ref, Rules: 1}, {Path: "techs/go", Library: ref, Rules: 1}}
	if !slices.Equal(groups, wantGroups) {
		t.Fatalf("got groups %+v, want %+v", groups, wantGroups)
	}
}

// A library page lists only the groups that hold current rules, though the catalog keeps a group whose rules are
// all retired, its current rules, and apart from them, its retired rules.
func TestLibraryPageListsCurrentRulesAndTheirGroups(t *testing.T) {
	reader := newCatalog(t)

	// Owner and name match without regard to case, and the page spells them as the host does.
	page, err := reader.LibraryPage(context.Background(), vetted, "Example", "RULES")

	if err != nil {
		t.Fatal(err)
	}
	wantLibrary := views.Library{
		Owner: "example", Name: "rules", Description: "Example rules.", OwnerAvatarURL: exampleRules.Repository.OwnerAvatarURL,
		LicenseExpression: "MIT", LicenseFile: "LICENSE", LatestRelease: 3, LatestTaggedAt: day(3), Groups: 2, Rules: 2,
	}
	if !page.Library.LatestTaggedAt.Equal(day(3)) {
		t.Errorf("latest release tagged at %s, want %s", page.Library.LatestTaggedAt, day(3))
	}
	page.Library.LatestTaggedAt = day(3)
	if page.Library != wantLibrary {
		t.Errorf("library is %+v, want %+v", page.Library, wantLibrary)
	}
	wantGroups := []views.Group{
		{Path: "practices/testing", Description: "Testing rules.", WhenToRead: "When testing.", Rules: 1},
		{Path: "techs/go", Description: "Go rules.", WhenToRead: "When writing Go.", Rules: 1},
	}
	if !slices.Equal(page.Groups, wantGroups) {
		t.Errorf("groups are %+v, want %+v", page.Groups, wantGroups)
	}
	wantRules := []views.RuleCard{
		{Path: "practices/testing/verify-retry-limits", Group: "practices/testing", Title: "Verify retry limits", Impact: "HIGH", Version: v(1, 1, 0)},
		{Path: "techs/go/return-errors", Group: "techs/go", Title: "Return errors", Impact: "HIGH", Version: v(2, 0, 0)},
	}
	if !slices.Equal(page.Rules, wantRules) {
		t.Errorf("rules are %+v, want %+v", page.Rules, wantRules)
	}
	wantRetired := []views.RetiredRuleCard{
		{Path: "practices/legacy/old-habit", Title: "Old habit", LastVersion: v(1, 0, 0), RetiredIn: 3},
		{Path: "practices/testing/check-retry-backoff", Title: "Check retry backoff", LastVersion: v(1, 0, 0), RetiredIn: 3,
			ReplacedBy: "practices/testing/verify-retry-limits"},
	}
	if !slices.Equal(page.Retired, wantRetired) {
		t.Errorf("retired rules are %+v, want %+v", page.Retired, wantRetired)
	}
}

func TestRulePageReadsTheCurrentVersionAndEveryVersionNewestFirst(t *testing.T) {
	reader := newCatalog(t)

	page, err := reader.RulePage(context.Background(), vetted, "example", "rules", "techs/go/return-errors")

	if err != nil {
		t.Fatal(err)
	}
	if page.Library.Owner != "example" || page.Library.LatestRelease != 3 {
		t.Errorf("library is %+v", page.Library)
	}
	r := page.Rule
	r.PublishedAt = day(3)
	want := views.Rule{
		Path: "techs/go/return-errors", Group: "techs/go", Title: "Return errors", Impact: "HIGH",
		WhenToRead: "When changing Return errors.", HTML: "<p>Return errors.</p>\n", Version: v(2, 0, 0), Release: 3, PublishedAt: day(3),
	}
	if r != want || !page.Rule.PublishedAt.Equal(day(3)) {
		t.Errorf("rule is %+v, want %+v", page.Rule, want)
	}
	var versions []string
	for _, version := range page.Versions {
		versions = append(versions, version.Version.String()+" "+domain.ReleaseTag(version.Release)+" "+string(version.Change)+" "+
			version.PublishedAt.UTC().Format(time.DateOnly)+" "+strings.Join(version.Summaries, " | "))
	}
	if want := []string{"2.0.0 release/3 major 2026-09-03 Require context. | Add an example.", "1.0.0 release/1 new 2026-09-01 Add the rule."}; !slices.Equal(versions, want) {
		t.Errorf("versions are %q, want %q", versions, want)
	}
}

func TestReadsDontFindWhatPagesDontShow(t *testing.T) {
	reader := newCatalog(t)
	ctx := context.Background()
	for name, read := range map[string]func() error{
		"an unknown library":  func() error { _, err := reader.LibraryPage(ctx, vetted, "example", "missing"); return err },
		"an unvetted library": func() error { _, err := reader.LibraryPage(ctx, vetted, "stranger", "unvetted-rules"); return err },
		"an unvetted library's rule": func() error {
			_, err := reader.RulePage(ctx, vetted, "stranger", "unvetted-rules", "techs/go/return-errors")
			return err
		},
		"an unknown rule": func() error {
			_, err := reader.RulePage(ctx, vetted, "example", "rules", "techs/go/missing")
			return err
		},
		"an unvetted library's history": func() error {
			_, err := reader.LibraryHistory(ctx, vetted, "stranger", "unvetted-rules")
			return err
		},
		"an unvetted library's releases to compare": func() error {
			_, _, err := reader.ReleaseComparison(ctx, vetted, "stranger", "unvetted-rules", 1, 3, 1<<20)
			return err
		},
		"a version the rule doesn't have": func() error {
			_, err := reader.RuleComparison(ctx, vetted, "example", "rules", "techs/go/return-errors", v(1, 0, 0), v(1, 1, 0), 1<<20)
			return err
		},
		"a group": func() error { _, err := reader.RulePage(ctx, vetted, "example", "rules", "techs/go"); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := read(); !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("got %v, want store.ErrNotFound", err)
			}
		})
	}
}
