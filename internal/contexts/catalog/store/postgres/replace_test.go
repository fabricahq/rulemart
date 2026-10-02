package postgres_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
	"github.com/fabricahq/rulemart/internal/platform/database/databasetest"
)

// release returns library release number, tagged on day number.
func release(number int) domain.Release {
	return domain.Release{Number: number, CommitID: fmt.Sprintf("%040d", number), TaggedAt: day(number)}
}

// added returns version 1.0.0 of a rule, new in release number.
func added(number int) domain.Version {
	return domain.Version{Number: v(1, 0, 0), Release: number, Change: coderules.ChangeNew, Summaries: []string{"Add the rule."}}
}

// goRules returns a library with the techs/go group and its rules, each published in releases.
func goRules(releases int, rules ...domain.Rule) domain.Library {
	lib := domain.Library{
		Repository: exampleRules.Repository,
		Groups:     []domain.Group{{Path: "techs/go", Name: "Go", Description: "Go rules.", WhenToRead: "When writing Go."}},
		Rules:      rules,
	}
	for n := 1; n <= releases; n++ {
		lib.Releases = append(lib.Releases, release(n))
	}
	return lib
}

// current returns the current rule at path in techs/go, with versions.
func current(path string, versions ...domain.Version) domain.Rule {
	return withContent(domain.Rule{Path: path, Group: "techs/go", Versions: versions}, path)
}

// newStore returns a Store on a new, migrated database, and the database's connection string.
func newStore(t *testing.T) (*postgres.Store, string) {
	t.Helper()
	db, connString := databasetest.New(t)
	return postgres.New(db), connString
}

// replace replaces lib in s, failing t on error, and returns how many rows changed.
func replace(t *testing.T, s *postgres.Store, lib domain.Library) int64 {
	t.Helper()
	changed, err := s.ReplaceLibrary(context.Background(), lib)
	if err != nil {
		t.Fatal(err)
	}
	return changed
}

// Rows keep their ids while they exist, so anything that refers to a release, group, rule, or version keeps
// pointing at it after every ingestion. Pages show no ids, so this reads them from the tables.
func TestReplaceLibraryKeepsTheIDsOfRowsThatSurvive(t *testing.T) {
	s, connString := newStore(t)
	replace(t, s, exampleRules)
	before := ids(t, connString)
	next := exampleRules
	next.Releases = append(slices.Clone(exampleRules.Releases), release(4))
	next.Rules = slices.Clone(exampleRules.Rules)
	next.Rules[2].Versions = append(slices.Clone(next.Rules[2].Versions),
		domain.Version{Number: v(1, 2, 0), Release: 4, Change: coderules.ChangeMinor, Summaries: []string{"Count retries."}})
	next.Rules = append(next.Rules, withContent(domain.Rule{Path: "techs/go/close-what-you-open", Group: "techs/go",
		Versions: []domain.Version{added(4)}}, "Close what you open"))

	replace(t, s, next)

	after := ids(t, connString)
	assertIDsSurvive(t, before, after)
	if len(after) < len(before) {
		t.Errorf("rows went missing: before %v, after %v", before, after)
	}
	for _, key := range []string{"release 4", "rule techs/go/close-what-you-open", "version practices/testing/verify-retry-limits 1.2.0"} {
		if after[key] == "" {
			t.Errorf("no %s", key)
		}
	}
}

// A group stays while any rule belongs to it, retired or not, so every rule's group exists.
func TestReplaceLibraryKeepsTheGroupOfRetiredRules(t *testing.T) {
	s, connString := newStore(t)
	replace(t, s, goRules(1, current("techs/go/return-errors", added(1))))
	before := ids(t, connString)
	retired := domain.Rule{Path: "techs/go/return-errors", Group: "techs/go", Versions: []domain.Version{added(1)},
		RetiredIn: 2, RetirementSummaries: []string{"Retire it."}}

	replace(t, s, goRules(2, retired))

	after := ids(t, connString)
	assertIDsSurvive(t, before, after)
	if after["group techs/go"] == "" || after["rule techs/go/return-errors"] == "" {
		t.Fatalf("the retired rule or its group is gone: %v", after)
	}
}

// A release-notes view will need what each release did: the summaries of each retirement, and whether the release
// also updated shared files. No page reads them yet, so this reads them from the tables.
func TestReplaceLibraryStoresRetirementSummariesAndSharedFileUpdates(t *testing.T) {
	s, connString := newStore(t)
	lib := exampleRules
	lib.Releases = slices.Clone(exampleRules.Releases)
	lib.Releases[1].UpdatesSharedFiles = true

	replace(t, s, lib)

	got := lines(t, connString, `SELECT path || ' ' || coalesce(array_to_string(retirement_summaries, ' | '), '-') FROM rules ORDER BY path`)
	want := []string{
		"practices/legacy/old-habit Drop it.",
		"practices/testing/check-retry-backoff Merge it.",
		"practices/testing/verify-retry-limits -",
		"techs/go/return-errors -",
	}
	if !slices.Equal(got, want) {
		t.Errorf("retirement summaries are %q, want %q", got, want)
	}
	got = lines(t, connString, `SELECT 'release/' || number || ' ' || updates_shared_files FROM library_releases ORDER BY number`)
	if want := []string{"release/1 false", "release/2 true", "release/3 false"}; !slices.Equal(got, want) {
		t.Errorf("shared file updates are %q, want %q", got, want)
	}
}

// A retired rule keeps the release that retired it and the rule that replaced it, for its history. No page reads
// them yet, so this reads them from the tables.
func TestReplaceLibraryStoresEachRetirement(t *testing.T) {
	s, connString := newStore(t)
	lib := exampleRules
	lib.Rules = slices.Clone(exampleRules.Rules)
	// check-retry-backoff is retired in release 2, before old-habit, and replaced by verify-retry-limits.
	lib.Rules[1].RetiredIn = 2

	replace(t, s, lib)

	got := lines(t, connString, `SELECT r.path || ' retired in release/' || rel.number || ', replaced by ' || coalesce(r.replaced_by, '-')
		FROM rules r JOIN library_releases rel ON rel.id = r.retired_in_release_id ORDER BY r.path`)
	want := []string{
		"practices/legacy/old-habit retired in release/3, replaced by -",
		"practices/testing/check-retry-backoff retired in release/2, replaced by practices/testing/verify-retry-limits",
	}
	if !slices.Equal(got, want) {
		t.Errorf("retirements are %q, want %q", got, want)
	}
}

// Replacing a library with itself writes no row at all, which the rows' transaction IDs show.
func TestReplaceLibraryChangesNothingWhenRepeated(t *testing.T) {
	s, connString := newStore(t)
	replace(t, s, exampleRules)
	before := dump(t, connString)

	changed := replace(t, s, exampleRules)

	if changed != 0 {
		t.Fatalf("the second replacement changed %d rows", changed)
	}
	if after := dump(t, connString); after != before {
		t.Fatalf("the catalog changed:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// Every version keeps the file its release published, so pages can compare versions and name a retired rule, but
// only a current rule's current version has the HTML its page shows.
func TestReplaceLibraryStoresEveryVersionsContentAndOnlyTheCurrentHTML(t *testing.T) {
	s, connString := newStore(t)

	replace(t, s, exampleRules)

	got := lines(t, connString, `SELECT r.path || ' ' || v.major || '.' || v.minor || '.' || v.patch || ' ' || v.title || ': ' ||
		v.markdown || coalesce(' ' || v.html, '')
		FROM rule_versions v JOIN rules r ON r.id = v.rule_id ORDER BY r.path, v.major, v.minor`)
	want := []string{
		"practices/legacy/old-habit 1.0.0 Old habit: ---\ntitle: Old habit\n---\n\nOld habit, version 1.0.0.\n",
		"practices/testing/check-retry-backoff 1.0.0 Check retry backoff: ---\ntitle: Check retry backoff\n---\n\nCheck retry backoff, version 1.0.0.\n",
		"practices/testing/verify-retry-limits 1.0.0 Verify retry limits: ---\ntitle: Verify retry limits\n---\n\nVerify retry limits, version 1.0.0.\n",
		"practices/testing/verify-retry-limits 1.1.0 Verify retry limits: ---\ntitle: Verify retry limits\n---\n\nVerify retry limits, version 1.1.0.\n <p>Verify retry limits.</p>\n",
		"techs/go/return-errors 1.0.0 Return errors: ---\ntitle: Return errors\n---\n\nReturn errors, version 1.0.0.\n",
		"techs/go/return-errors 2.0.0 Return errors: ---\ntitle: Return errors\n---\n\nReturn errors, version 2.0.0.\n <p>Return errors.</p>\n",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("stored versions\n%q\nwant\n%q", got, want)
	}
}

// A replacement that fails writes nothing: the library's rows are replaced in one transaction.
func TestReplaceLibraryWritesNothingWhenItFails(t *testing.T) {
	s, connString := newStore(t)
	replace(t, s, exampleRules)
	before := dump(t, connString)
	broken := goRules(4, current("techs/go/return-errors", added(1)), withContent(domain.Rule{
		Path: "practices/missing/rule", Group: "practices/missing", Versions: []domain.Version{added(4)},
	}, "Missing"))

	_, err := s.ReplaceLibrary(context.Background(), broken)

	if err == nil || !strings.Contains(err.Error(), "repository=7") {
		t.Fatalf("got error %v, want one naming the repository", err)
	}
	if after := dump(t, connString); after != before {
		t.Fatalf("a failed replacement changed the catalog:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// A library whose tags were rewritten, as when a test library is reset, keeps only what its current tags publish,
// and what they still publish keeps its id.
func TestReplaceLibraryRemovesWhatALibraryNoLongerPublishes(t *testing.T) {
	s, connString := newStore(t)
	replace(t, s, exampleRules)
	before := ids(t, connString)

	replace(t, s, goRules(1, current("techs/go/return-errors", added(1))))

	page, err := s.LibraryPage(context.Background(), vetted, "example", "rules")
	if err != nil {
		t.Fatal(err)
	}
	if page.Library.LatestRelease != 1 || len(page.Groups) != 1 || len(page.Rules) != 1 || page.Rules[0].Path != "techs/go/return-errors" {
		t.Fatalf("the library page is %+v", page)
	}
	if got := versions(t, s, "techs/go/return-errors"); !slices.Equal(got, []string{"1.0.0 release/1 new"}) {
		t.Fatalf("versions are %q", got)
	}
	after := ids(t, connString)
	assertIDsSurvive(t, before, after)
	if len(after) != 5 {
		t.Errorf("kept %v, want the library, release 1, the group, the rule, and its version", after)
	}
}

// Rewritten tags can move a version to another release, such as when a reset library publishes in release 1 a rule
// it first published in release 2. The version is the same row, so it keeps its id and moves with the tags.
func TestReplaceLibraryKeepsTheIDOfAVersionThatMovesToAnotherRelease(t *testing.T) {
	s, connString := newStore(t)
	replace(t, s, goRules(2, current("techs/go/close-what-you-open", added(2)), current("techs/go/return-errors", added(1))))
	before := ids(t, connString)

	replace(t, s, goRules(1, current("techs/go/close-what-you-open", added(1)), current("techs/go/return-errors", added(1))))

	assertIDsSurvive(t, before, ids(t, connString))
	if got := versions(t, s, "techs/go/close-what-you-open"); !slices.Equal(got, []string{"1.0.0 release/1 new"}) {
		t.Fatalf("versions are %q", got)
	}
}

// Moving a rule's versions each one release later passes through a moment where two of them share a release, which
// the database must allow within the replacement.
func TestReplaceLibraryMovesSeveralVersionsOfARuleToLaterReleases(t *testing.T) {
	s, connString := newStore(t)
	fix := func(number int) domain.Version {
		return domain.Version{Number: v(1, 0, 1), Release: number, Change: coderules.ChangePatch, Summaries: []string{"Fix a typo."}}
	}
	replace(t, s, goRules(2, current("techs/go/return-errors", added(1), fix(2))))
	before := ids(t, connString)

	replace(t, s, goRules(3, current("techs/go/close-what-you-open", added(1)), current("techs/go/return-errors", added(2), fix(3))))

	assertIDsSurvive(t, before, ids(t, connString))
	if got, want := versions(t, s, "techs/go/return-errors"), []string{"1.0.1 release/3 patch", "1.0.0 release/2 new"}; !slices.Equal(got, want) {
		t.Fatalf("versions are %q, want %q", got, want)
	}
}

// versions describes each version of the current rule at path in exampleRules' library, newest first, as its page
// lists them: "<version> release/<n> <change>".
func versions(t *testing.T, s *postgres.Store, path string) []string {
	t.Helper()
	page, err := s.RulePage(context.Background(), vetted, "example", "rules", path)
	if err != nil {
		t.Fatal(err)
	}
	var result []string
	for _, version := range page.Versions {
		result = append(result, version.Version.String()+" "+domain.ReleaseTag(version.Release)+" "+string(version.Change))
	}
	return result
}

// assertIDsSurvive fails t unless every row in both before and after, by its natural key, kept its id.
func assertIDsSurvive(t *testing.T, before, after map[string]string) {
	t.Helper()
	for key, id := range before {
		if now, ok := after[key]; ok && now != id {
			t.Errorf("%s had id %s, and now %s", key, id, now)
		}
	}
}

// ids returns the id of every catalog row, keyed by what the row is, such as "rule techs/go/return-errors".
func ids(t *testing.T, connString string) map[string]string {
	t.Helper()
	result := map[string]string{}
	for _, row := range lines(t, connString, `
		SELECT 'library ' || host || ' ' || host_repository_id || '=' || id FROM libraries
		UNION ALL SELECT 'release ' || number || '=' || id FROM library_releases
		UNION ALL SELECT 'group ' || path || '=' || id FROM library_groups
		UNION ALL SELECT 'rule ' || path || '=' || id FROM rules
		UNION ALL SELECT 'version ' || r.path || ' ' || v.major || '.' || v.minor || '.' || v.patch || '=' || v.id
			FROM rule_versions v JOIN rules r ON r.id = v.rule_id`) {
		key, id, _ := strings.Cut(row, "=")
		result[key] = id
	}
	return result
}

// dump returns every catalog row as text, in a stable order, with each row's transaction ID, so it changes when
// any row is written, even with the same values.
func dump(t *testing.T, connString string) string {
	t.Helper()
	var all []string
	for _, table := range []string{"libraries", "library_releases", "library_groups", "rules", "rule_versions"} {
		all = append(all, lines(t, connString, `SELECT t.xmin::text || ' ' || t::text FROM `+table+` t ORDER BY t::text`)...)
	}
	return strings.Join(all, "\n")
}

// lines runs a query returning one text column and returns its values.
func lines(t *testing.T, connString, sql string) []string {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, sql)
	if err != nil {
		t.Fatalf("run %q: %v", sql, err)
	}
	values, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("run %q: %v", sql, err)
	}
	return values
}
