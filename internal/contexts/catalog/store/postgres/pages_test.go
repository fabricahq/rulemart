package postgres_test

import (
	"context"
	"errors"
	"reflect"
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
	"github.com/fabricahq/rulemart/internal/platform/postgrestest"
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

// withContent returns r with each version's content, titled title, its newest version's HTML, and while r is current,
// its reading guidance's.
func withContent(r domain.Rule, title string) domain.Rule {
	r.Versions = slices.Clone(r.Versions)
	for i, version := range r.Versions {
		r.Versions[i].Content = content(title, version.Number)
	}
	r.HTML = "<p>" + title + ".</p>\n"
	if r.IsCurrent() {
		r.WhenToReadHTML = "<p>When changing <code>" + title + "</code>.</p>\n"
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

// rule returns a current rule at path, in the group its path names, with one version and the given content.
func rule(path, title, whenToRead, body string) domain.Rule {
	c := content(title, v(1, 0, 0))
	c.WhenToRead = whenToRead
	c.Markdown = "---\ntitle: " + title + "\n---\n\n" + body + "\n"
	group := path[:strings.LastIndex(path, "/")]
	return domain.Rule{Path: path, Group: group, HTML: "<p>" + title + ".</p>\n", WhenToReadHTML: "<p>" + whenToRead + "</p>\n", Versions: []domain.Version{
		{Number: v(1, 0, 0), Release: 1, Change: coderules.ChangeNew, Summaries: []string{"Add the rule."}, Content: c},
	}}
}

// newLibrary returns a library of one release, holding groups and rules, at GitHub repository id, owner/name.
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

func TestHomePageListsOnlyVettedLibrariesAndTheirGroups(t *testing.T) {
	reader := newCatalog(t)

	got, groups, err := reader.HomePage(context.Background(), vetted)

	if err != nil {
		t.Fatal(err)
	}
	want := []views.LibraryCard{{Owner: "example", Name: "rules", Description: "Example rules.", OwnerAvatarURL: exampleRules.Repository.OwnerAvatarURL, Rules: 2, Vetted: true}}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	ref := views.LibraryRef{Owner: "example", Name: "rules", OwnerAvatarURL: exampleRules.Repository.OwnerAvatarURL}
	wantGroups := []views.LibraryGroup{{Path: "practices/testing", Library: ref, Vetted: true, Rules: 1}, {Path: "techs/go", Library: ref, Vetted: true, Rules: 1}}
	if !slices.Equal(groups, wantGroups) {
		t.Fatalf("got groups %+v, want %+v", groups, wantGroups)
	}
}

func TestGroupsListEachVettedLibrarysGroupsWithCurrentRules(t *testing.T) {
	reader := newLibraries(t)

	got, err := reader.Groups(context.Background(), vettedBoth, false)

	if err != nil {
		t.Fatal(err)
	}
	acmeRef := views.LibraryRef{Owner: "acme", Name: "backend", OwnerAvatarURL: acme.Repository.OwnerAvatarURL}
	betaRef := views.LibraryRef{Owner: "Beta", Name: "rules", OwnerAvatarURL: beta.Repository.OwnerAvatarURL}
	want := []views.LibraryGroup{
		{Path: "practices/testing", Library: acmeRef, Vetted: true, Rules: 2},
		{Path: "techs/go", Library: acmeRef, Vetted: true, Rules: 1},
		{Path: "techs/go", Library: betaRef, Vetted: true, Rules: 1},
		{Path: "techs/golang", Library: acmeRef, Vetted: true, Rules: 1},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// The libraries index lists the vetted libraries as the home page does, and no other.
func TestLibrariesListsOnlyVettedLibraries(t *testing.T) {
	reader := newCatalog(t)

	got, err := reader.Libraries(context.Background(), vetted, false)

	if err != nil {
		t.Fatal(err)
	}
	want := []views.LibraryCard{{Owner: "example", Name: "rules", Description: "Example rules.", OwnerAvatarURL: exampleRules.Repository.OwnerAvatarURL, Rules: 2, Vetted: true}}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// An owner's page lists the vetted libraries they own, matched without regard to case and spelled as the host
// spells them, and none of an owner whose only library is unvetted.
func TestOwnerLibrariesListsOnlyTheOwnersVettedLibraries(t *testing.T) {
	reader := newCatalog(t)

	for login, want := range map[string][]views.LibraryCard{
		"EXAMPLE":  {{Owner: "example", Name: "rules", Description: "Example rules.", OwnerAvatarURL: exampleRules.Repository.OwnerAvatarURL, Rules: 2, Vetted: true}},
		"stranger": nil,
		"nobody":   nil,
	} {
		got, err := reader.OwnerLibraries(context.Background(), vetted, login)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s: got %+v, want %+v", login, got, want)
		}
	}
}

// A rule's address may spell its ID in any case, as a library's may spell its owner and name; the page names the ID
// as the library spells it, so the site can redirect to it.
func TestRulePageMatchesTheRuleIDWithoutRegardToCase(t *testing.T) {
	reader := newCatalog(t)

	page, err := reader.RulePage(context.Background(), vetted, "example", "rules", "Techs/Go/Return-Errors")

	if err != nil || page.Rule.Path != "techs/go/return-errors" {
		t.Fatalf("got %q, %v; want techs/go/return-errors", page.Rule.Path, err)
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
		Vetted: true, Owner: "example", Name: "rules", Description: "Example rules.", OwnerAvatarURL: exampleRules.Repository.OwnerAvatarURL,
		LicenseExpression: "MIT", LicenseFile: "LICENSE", LatestRelease: 3, LatestTaggedAt: day(3), Groups: 2, Rules: 2,
	}
	if !page.Library.LatestTaggedAt.Equal(day(3)) {
		t.Errorf("latest release tagged at %s, want %s", page.Library.LatestTaggedAt, day(3))
	}
	// Without a listing, the library came to Rulemart when it was first ingested, a moment ago.
	if since := time.Since(page.Library.AddedAt); since < 0 || since > time.Minute {
		t.Errorf("the library came to Rulemart at %s, want when it was ingested", page.Library.AddedAt)
	}
	page.Library.LatestTaggedAt, page.Library.AddedAt = day(3), time.Time{}
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
		{Path: "practices/legacy/old-habit", Group: "practices/legacy", Title: "Old habit", Impact: "HIGH", LastVersion: v(1, 0, 0), RetiredIn: 3},
		{Path: "practices/testing/check-retry-backoff", Group: "practices/testing", Title: "Check retry backoff", Impact: "HIGH",
			LastVersion: v(1, 0, 0), RetiredIn: 3, ReplacedBy: "practices/testing/verify-retry-limits"},
	}
	if !reflect.DeepEqual(page.Retired, wantRetired) {
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
		WhenToRead: "When changing Return errors.", WhenToReadHTML: "<p>When changing <code>Return errors</code>.</p>\n",
		HTML: "<p>Return errors.</p>\n", Tags: []string{}, Version: v(2, 0, 0), Release: 3, PublishedAt: day(3),
	}
	if !reflect.DeepEqual(r, want) || !page.Rule.PublishedAt.Equal(day(3)) {
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
		"an unlisted, unvetted library's history": func() error {
			_, err := reader.LibraryHistory(ctx, vetted, "stranger", "unvetted-rules")
			return err
		},
		"an unlisted, unvetted library's releases to compare": func() error {
			_, _, err := reader.ReleaseComparison(ctx, vetted, "stranger", "unvetted-rules", pickChanged(1, 3), 1<<20)
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

// A current rule's page reads its tags, and the assets it lists, its own first, each in path order; an asset's page
// reads one of them with the rule's page, or the first rule's that lists it; and only a kept file's bytes are read.
func TestRulePageReadsTagsAndAssets(t *testing.T) {
	s, connString := newStore(t)
	lib := goRules(1, current("techs/go/return-errors", added(1)), current("techs/go/close-what-you-open", added(1)))
	lib.Rules[0].Versions[0].Content.Tags = []string{"errors", "wrapping"}
	lib.Rules[0].Assets = []string{"techs/go/assets/return-errors/z.svg", "assets/glossary.md", "assets/a.md"}
	lib.Rules[1].Assets = []string{"assets/glossary.md"}
	lib.Assets = []domain.Asset{
		{Path: "assets/a.md", Release: 1, Size: 9000, MediaType: "text/markdown; charset=utf-8"},
		{Path: "assets/glossary.md", Release: 1, Size: 6, MediaType: "text/markdown; charset=utf-8", Content: []byte("Terms."), HTML: "<p>Terms.</p>"},
		{Path: "techs/go/assets/return-errors/z.svg", Release: 1, Size: 6, MediaType: "image/svg+xml", Content: []byte("<svg/>")},
	}
	replace(t, s, lib)
	reader := postgres.New(databasetest.AsWebRole(t, connString))
	ctx := context.Background()

	page, err := reader.RulePage(ctx, vetted, "example", "rules", "techs/go/return-errors")

	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"errors", "wrapping"}; !slices.Equal(page.Rule.Tags, want) {
		t.Errorf("tags are %q, want %q", page.Rule.Tags, want)
	}
	wantAssets := []views.Asset{
		{Path: "techs/go/assets/return-errors/z.svg", Size: 6, MediaType: "image/svg+xml", Release: 1, Kept: true},
		{Path: "assets/a.md", Size: 9000, MediaType: "text/markdown; charset=utf-8", Release: 1},
		{Path: "assets/glossary.md", Size: 6, MediaType: "text/markdown; charset=utf-8", Release: 1, Kept: true},
	}
	if !reflect.DeepEqual(page.Assets, wantAssets) {
		t.Errorf("assets are %+v, want %+v", page.Assets, wantAssets)
	}

	shared, err := reader.AssetPage(ctx, vetted, "example", "rules", "", "assets/glossary.md")
	if err != nil || shared.Page.Rule.Path != "techs/go/close-what-you-open" || shared.HTML != "<p>Terms.</p>" {
		t.Errorf("the shared asset's page is %+v, %v; want it with close-what-you-open, the first rule that lists it", shared, err)
	}
	if _, err := reader.AssetPage(ctx, vetted, "example", "rules", "techs/go/close-what-you-open", "assets/a.md"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("an asset the rule doesn't list: got %v, want ErrNotFound", err)
	}
	image, err := reader.AssetContent(ctx, vetted, "example", "rules", "techs/go/assets/return-errors/z.svg")
	if err != nil || image.MediaType != "image/svg+xml" || string(image.Content) != "<svg/>" {
		t.Errorf("the image is %+v, %v", image, err)
	}
	if _, err := reader.AssetContent(ctx, vetted, "example", "rules", "assets/a.md"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("a file kept without bytes: got %v, want ErrNotFound", err)
	}
}

// A listed library names who listed it, by the login the listing's account signed in with last, and came to Rulemart
// when it was listed, vetted since or not; one vetted without a listing names no one.
func TestLibraryPageNamesWhoListedTheLibrary(t *testing.T) {
	db, connString := databasetest.New(t)
	if _, err := postgres.New(db).ReplaceLibrary(context.Background(), unvetted()); err != nil {
		t.Fatal(err)
	}
	listedAt := time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC)
	postgrestest.Exec(t, connString, `WITH account AS (
		INSERT INTO accounts (github_user_id, github_login, avatar_url) VALUES (99, 'Lister', '') RETURNING id
	) INSERT INTO listings (account_id, host, owner, name, host_repository_id, created_at)
	SELECT id, 'github', 'stranger', 'unvetted-rules', '8', $1 FROM account`, listedAt)
	reader := postgres.New(databasetest.AsWebRole(t, connString))

	for name, vettedNow := range map[string][]domain.LibraryKey{"unvetted": nil, "vetted since": {{Host: domain.GitHub, RepositoryID: "8"}}} {
		page, err := reader.LibraryPage(context.Background(), vettedNow, "stranger", "unvetted-rules")
		if err != nil {
			t.Fatal(err)
		}
		if page.Library.AddedBy != "Lister" || !page.Library.AddedAt.Equal(listedAt) {
			t.Errorf("%s: added by %q at %s, want Lister at %s", name, page.Library.AddedBy, page.Library.AddedAt, listedAt)
		}
	}
}
