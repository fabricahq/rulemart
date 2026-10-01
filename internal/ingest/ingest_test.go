package ingest_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fabricahq/rulemart/internal/ingest"
	"github.com/fabricahq/rulemart/internal/ingest/ingesttest"
)

const (
	retryLimits  = "practices/testing/verify-retry-limits"
	retryBackoff = "practices/testing/check-retry-backoff"
	returnErrors = "techs/go/return-errors"
)

// firstRelease publishes three rules in two groups.
func firstRelease(t *testing.T) *ingesttest.Library {
	t.Helper()
	lib := ingesttest.NewLibrary(t)
	lib.Group("practices/testing", "Testing")
	lib.Group("techs/go", "Go")
	lib.Rule(retryLimits, "Verify retry limits", "Every retry loop stops after a fixed number of attempts.")
	lib.Rule(retryBackoff, "Check retry backoff", "Retries wait longer after each attempt.")
	lib.Rule(returnErrors, "Return errors", "Return errors instead of panicking.")
	lib.Release(1, `formatVersion: 1
release: 1
rules:
  practices/testing/check-retry-backoff: 1.0.0
  practices/testing/verify-retry-limits: 1.0.0
  techs/go/return-errors: 1.0.0
changes:
  practices/testing/check-retry-backoff: {change: new, summaries: [Add the rule.]}
  practices/testing/verify-retry-limits: {change: new, summaries: [Add the rule.]}
  techs/go/return-errors: {change: new, summaries: [Add the rule.]}
`)
	return lib
}

// laterReleases adds three releases to firstRelease's library: a minor change, then a patch and a major change and
// a retirement, then a release that only edits a rule's file without a change, which must not show.
func laterReleases(t *testing.T, lib *ingesttest.Library) {
	t.Helper()
	lib.Rule(retryLimits, "Verify retry limits", "Every retry loop stops after a fixed number of attempts, including timeouts.")
	lib.Release(2, `formatVersion: 1
release: 2
rules:
  practices/testing/check-retry-backoff: 1.0.0
  practices/testing/verify-retry-limits: 1.1.0
  techs/go/return-errors: 1.0.0
changes:
  practices/testing/verify-retry-limits: {change: minor, from: 1.0.0, summaries: [Cover timeouts.]}
`)
	lib.Rule(retryLimits, "Verify retry limits", "Every retry loop stops after a fixed number of attempts, including timeouts, and says so.")
	lib.Rule(returnErrors, "Return errors with context", "Wrap every returned error with the operation that failed.")
	lib.Remove(retryBackoff + ".md")
	lib.Release(3, `formatVersion: 1
release: 3
rules:
  practices/testing/verify-retry-limits: 1.1.1
  techs/go/return-errors: 2.0.0
changes:
  practices/testing/verify-retry-limits: {change: patch, from: 1.1.0, summaries: [Fix a typo.]}
  techs/go/return-errors:
    change: major
    from: 1.0.0
    summaries: [Require context on every returned error., Add an example.]
retired:
  practices/testing/check-retry-backoff:
    lastVersion: 1.0.0
    replacedBy: practices/testing/verify-retry-limits
    summaries: [Merge into verify-retry-limits.]
`)
	lib.Rule(returnErrors, "Unreleased title", "An edit no library release published.")
	lib.Release(4, `formatVersion: 1
release: 4
rules:
  practices/testing/verify-retry-limits: 1.1.1
  techs/go/return-errors: 2.0.0
`)
}

func TestIngestStoresTheFirstRelease(t *testing.T) {
	store, connString := newStore(t)
	lib := firstRelease(t)

	result, err := ingest.Ingest(context.Background(), store, lib.Repository(42))
	if err != nil {
		t.Fatal(err)
	}

	if result.Releases != 1 || result.Rules != 3 {
		t.Fatalf("ingested %+v, want 1 release and 3 rules", result)
	}
	var owner, name, license string
	query(t, connString, `SELECT owner, name, license_expression FROM libraries WHERE host = 'github' AND host_repository_id = '42'`, &owner, &name, &license)
	if owner != "example" || name != "rules" || license != "MIT" {
		t.Fatalf("library is %s/%s under %s, want example/rules under MIT", owner, name, license)
	}
	var taggedAt time.Time
	query(t, connString, `SELECT tagged_at FROM library_releases WHERE number = 1`, &taggedAt)
	if !taggedAt.Equal(ingesttest.FirstTagged) {
		t.Fatalf("release/1 tagged at %s, want %s", taggedAt, ingesttest.FirstTagged)
	}
	if got := groups(t, connString); !slices.Equal(got, []string{"practices/testing Testing", "techs/go Go"}) {
		t.Fatalf("groups are %q", got)
	}
	if got := versions(t, connString, retryLimits); !slices.Equal(got, []string{"1.0.0 release/1 new [Add the rule.]"}) {
		t.Fatalf("versions are %q", got)
	}
	title, html := currentContent(t, connString, retryLimits)
	if title != "Verify retry limits" || !strings.Contains(html, "fixed number of attempts") {
		t.Fatalf("current content is %q: %s", title, html)
	}
	if strings.Contains(html, "<h2") {
		t.Fatalf("content repeats the title as a heading: %s", html)
	}
}

func TestIngestRecordsEachChangeLevelAndRetirement(t *testing.T) {
	store, connString := newStore(t)
	lib := firstRelease(t)
	laterReleases(t, lib)

	result, err := ingest.Ingest(context.Background(), store, lib.Repository(42))
	if err != nil {
		t.Fatal(err)
	}

	if result.Releases != 4 || result.Rules != 2 {
		t.Fatalf("ingested %+v, want 4 releases and 2 current rules", result)
	}
	for id, want := range map[string][]string{
		retryLimits: {
			"1.0.0 release/1 new [Add the rule.]",
			"1.1.0 release/2 minor [Cover timeouts.]",
			"1.1.1 release/3 patch [Fix a typo.]",
		},
		returnErrors: {
			"1.0.0 release/1 new [Add the rule.]",
			"2.0.0 release/3 major [Require context on every returned error. Add an example.]",
		},
		retryBackoff: {"1.0.0 release/1 new [Add the rule.]"},
	} {
		if got := versions(t, connString, id); !slices.Equal(got, want) {
			t.Errorf("%s versions are %q, want %q", id, got, want)
		}
	}
	var retiredIn int
	var replacedBy string
	query(t, connString, `SELECT rel.number, r.replaced_by FROM rules r JOIN library_releases rel ON rel.id = r.retired_in_release_id WHERE r.path = '`+retryBackoff+`'`, &retiredIn, &replacedBy)
	if retiredIn != 3 || replacedBy != retryLimits {
		t.Fatalf("%s retired in release/%d, replaced by %q", retryBackoff, retiredIn, replacedBy)
	}
	var withContent int
	query(t, connString, `SELECT count(*) FROM rule_versions v JOIN rules r ON r.id = v.rule_id WHERE r.path = '`+retryBackoff+`' AND v.html IS NOT NULL`, &withContent)
	if withContent != 0 {
		t.Fatal("the retired rule kept content")
	}
	if title, html := currentContent(t, connString, retryLimits); !strings.Contains(html, "and says so") {
		t.Fatalf("%s shows %q from an earlier version: %s", retryLimits, title, html)
	}
}

// A rule's current version is the file at the release that published it. A later edit no release recorded
// must not show.
func TestIngestReadsAnUnchangedRuleFromTheReleaseThatPublishedIt(t *testing.T) {
	store, connString := newStore(t)
	lib := firstRelease(t)
	laterReleases(t, lib)

	if _, err := ingest.Ingest(context.Background(), store, lib.Repository(42)); err != nil {
		t.Fatal(err)
	}

	title, html := currentContent(t, connString, returnErrors)
	if title != "Return errors with context" || !strings.Contains(html, "operation that failed") {
		t.Fatalf("%s shows %q: %s, want release/3's version", returnErrors, title, html)
	}
}

// Rows keep their ids while they exist, so anything that refers to a release, group, rule, or version keeps
// pointing at it after every ingestion.
func TestIngestKeepsTheIDsOfRowsThatSurvive(t *testing.T) {
	store, connString := newStore(t)
	lib := firstRelease(t)
	if _, err := ingest.Ingest(context.Background(), store, lib.Repository(42)); err != nil {
		t.Fatal(err)
	}
	before := ids(t, connString)
	lib.Rule(retryLimits, "Verify retry limits", "Every retry loop stops after a fixed number of attempts, timeouts included.")
	lib.Rule("techs/go/close-what-you-open", "Close what you open", "Close every resource you open.")
	lib.Remove(retryBackoff + ".md")
	lib.Release(2, `formatVersion: 1
release: 2
rules:
  practices/testing/verify-retry-limits: 1.1.0
  techs/go/close-what-you-open: 1.0.0
  techs/go/return-errors: 1.0.0
changes:
  practices/testing/verify-retry-limits: {change: minor, from: 1.0.0, summaries: [Count timeouts.]}
  techs/go/close-what-you-open: {change: new, summaries: [Add the rule.]}
retired:
  practices/testing/check-retry-backoff: {lastVersion: 1.0.0, summaries: [Merge it.]}
`)

	if _, err := ingest.Ingest(context.Background(), store, lib.Repository(42)); err != nil {
		t.Fatal(err)
	}

	after := ids(t, connString)
	for key, id := range before {
		if after[key] != id {
			t.Errorf("%s had id %s, and now %q", key, id, after[key])
		}
	}
	for _, added := range []string{"release 2", "rule techs/go/close-what-you-open", "version practices/testing/verify-retry-limits 1.1.0"} {
		if after[added] == "" {
			t.Errorf("no %s", added)
		}
	}
}

// A group stays while any rule belongs to it, retired or not, so every rule's group exists. One whose rules are all
// retired keeps the metadata of the last release that had it.
func TestIngestKeepsTheGroupOfRetiredRules(t *testing.T) {
	store, connString := newStore(t)
	lib := firstRelease(t)
	lib.Remove(returnErrors + ".md")
	lib.Remove("techs/go/_group.yaml")
	lib.Release(2, `formatVersion: 1
release: 2
rules:
  practices/testing/check-retry-backoff: 1.0.0
  practices/testing/verify-retry-limits: 1.0.0
retired:
  techs/go/return-errors: {lastVersion: 1.0.0, summaries: [Retire it.]}
`)

	if _, err := ingest.Ingest(context.Background(), store, lib.Repository(42)); err != nil {
		t.Fatal(err)
	}

	if got := groups(t, connString); !slices.Equal(got, []string{"practices/testing Testing", "techs/go Go"}) {
		t.Fatalf("groups are %q", got)
	}
	var group string
	query(t, connString, `SELECT g.path FROM rules r JOIN library_groups g ON g.id = r.group_id WHERE r.path = '`+returnErrors+`'`, &group)
	if group != "techs/go" {
		t.Fatalf("the retired rule is in group %q", group)
	}
}

func TestIngestChangesNothingWhenRunAgain(t *testing.T) {
	store, connString := newStore(t)
	lib := firstRelease(t)
	laterReleases(t, lib)
	if _, err := ingest.Ingest(context.Background(), store, lib.Repository(42)); err != nil {
		t.Fatal(err)
	}
	before := catalog(t, connString)

	result, err := ingest.Ingest(context.Background(), store, lib.Repository(42))
	if err != nil {
		t.Fatal(err)
	}

	if result.Changed != 0 {
		t.Fatalf("the second ingestion changed %d rows", result.Changed)
	}
	if after := catalog(t, connString); after != before {
		t.Fatalf("the catalog changed:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestIngestRejectsAMalformedRecordWithoutWriting(t *testing.T) {
	for name, record := range map[string]string{
		"invalid YAML": "formatVersion: 1\nrelease: [2\n",
		"change from the wrong version": `formatVersion: 1
release: 2
rules:
  practices/testing/check-retry-backoff: 1.0.0
  practices/testing/verify-retry-limits: 1.2.1
  techs/go/return-errors: 1.0.0
changes:
  practices/testing/verify-retry-limits: {change: patch, from: 1.2.0, summaries: [Fix a typo.]}
`,
		"rule dropped without retiring it": `formatVersion: 1
release: 2
rules:
  practices/testing/verify-retry-limits: 1.0.0
  techs/go/return-errors: 1.0.0
`,
	} {
		t.Run(name, func(t *testing.T) {
			store, connString := newStore(t)
			lib := firstRelease(t)
			if _, err := ingest.Ingest(context.Background(), store, lib.Repository(42)); err != nil {
				t.Fatal(err)
			}
			before := catalog(t, connString)
			lib.Rule(retryLimits, "Verify retry limits", "A change in a broken release.")
			lib.Release(2, record)

			_, err := ingest.Ingest(context.Background(), store, lib.Repository(42))

			if err == nil || !strings.Contains(err.Error(), "release/2") {
				t.Fatalf("got error %v, want one naming release/2", err)
			}
			if after := catalog(t, connString); after != before {
				t.Fatalf("a failed ingestion changed the catalog:\nbefore:\n%s\nafter:\n%s", before, after)
			}
		})
	}
}

// The library page links to the license file, so a release whose manifest names a missing one is refused rather
// than stored as a broken link.
func TestIngestRejectsAMissingLicenseFileWithoutWriting(t *testing.T) {
	store, connString := newStore(t)
	lib := firstRelease(t)
	if _, err := ingest.Ingest(context.Background(), store, lib.Repository(42)); err != nil {
		t.Fatal(err)
	}
	before := catalog(t, connString)
	lib.Remove("LICENSE")
	lib.Release(2, `formatVersion: 1
release: 2
rules:
  practices/testing/check-retry-backoff: 1.0.0
  practices/testing/verify-retry-limits: 1.0.0
  techs/go/return-errors: 1.0.0
`)

	_, err := ingest.Ingest(context.Background(), store, lib.Repository(42))

	if err == nil || !strings.Contains(err.Error(), "release/2") || !strings.Contains(err.Error(), "LICENSE") {
		t.Fatalf("got error %v, want one naming release/2 and LICENSE", err)
	}
	if after := catalog(t, connString); after != before {
		t.Fatalf("a refused release changed the catalog:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// hugeObject is larger, inflated, than ingestion holds in memory for one object. It's zeros, so it compresses to
// almost nothing and only its inflated size can stop it.
var hugeObject = strings.Repeat("\x00", 40<<20)

func TestIngestRejectsAnOversizedObjectWithoutWriting(t *testing.T) {
	store, connString := newStore(t)
	lib := firstRelease(t)
	if _, err := ingest.Ingest(context.Background(), store, lib.Repository(42)); err != nil {
		t.Fatal(err)
	}
	before := catalog(t, connString)
	lib.Write("assets/huge.bin", hugeObject)
	lib.Release(2, `formatVersion: 1
release: 2
rules:
  practices/testing/check-retry-backoff: 1.0.0
  practices/testing/verify-retry-limits: 1.0.0
  techs/go/return-errors: 1.0.0
`)

	_, err := ingest.Ingest(context.Background(), store, lib.Repository(42))

	if err == nil || !strings.Contains(err.Error(), "bytes") {
		t.Fatalf("got error %v, want one about the object's size", err)
	}
	if after := catalog(t, connString); after != before {
		t.Fatalf("a refused fetch changed the catalog:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// Ingestion reads only the trees at tagged commits, so history it doesn't read, even an oversized object, isn't
// fetched.
func TestIngestFetchesOnlyTheTaggedCommits(t *testing.T) {
	store, _ := newStore(t)
	lib := ingesttest.NewLibrary(t)
	lib.Write("assets/huge.bin", hugeObject)
	lib.Commit("Add a large file")
	lib.Remove("assets/huge.bin")
	lib.Commit("Remove the large file")
	lib.Group("techs/go", "Go")
	lib.Rule(returnErrors, "Return errors", "Return errors instead of panicking.")
	lib.Release(1, `formatVersion: 1
release: 1
rules:
  techs/go/return-errors: 1.0.0
changes:
  techs/go/return-errors: {change: new, summaries: [Add the rule.]}
`)

	result, err := ingest.Ingest(context.Background(), store, lib.Repository(42))

	if err != nil {
		t.Fatal(err)
	}
	if result.Releases != 1 || result.Rules != 1 {
		t.Fatalf("ingested %+v, want 1 release and 1 rule", result)
	}
}

func TestIngestRejectsALibraryWithoutReleases(t *testing.T) {
	store, connString := newStore(t)
	lib := ingesttest.NewLibrary(t)
	lib.Commit("Start the library")

	_, err := ingest.Ingest(context.Background(), store, lib.Repository(42))

	if err == nil {
		t.Fatal("ingested a library without release tags")
	}
	if got := catalog(t, connString); got != "" {
		t.Fatalf("stored rows for a library without releases:\n%s", got)
	}
}

// A library whose tags were rewritten, as when a test library is reset, keeps only what its current tags publish.
func TestIngestRemovesWhatRewrittenTagsNoLongerPublish(t *testing.T) {
	store, connString := newStore(t)
	lib := firstRelease(t)
	laterReleases(t, lib)
	if _, err := ingest.Ingest(context.Background(), store, lib.Repository(42)); err != nil {
		t.Fatal(err)
	}
	fresh := ingesttest.NewLibrary(t)
	fresh.Group("techs/go", "Go")
	fresh.Rule(returnErrors, "Return errors", "Return errors instead of panicking.")
	fresh.Release(1, `formatVersion: 1
release: 1
rules:
  techs/go/return-errors: 1.0.0
changes:
  techs/go/return-errors: {change: new, summaries: [Add the rule.]}
`)

	if _, err := ingest.Ingest(context.Background(), store, fresh.Repository(42)); err != nil {
		t.Fatal(err)
	}

	var releases, rules int
	query(t, connString, `SELECT (SELECT count(*) FROM library_releases), (SELECT count(*) FROM rules)`, &releases, &rules)
	if releases != 1 || rules != 1 {
		t.Fatalf("kept %d releases and %d rules, want 1 of each", releases, rules)
	}
	if got := versions(t, connString, returnErrors); !slices.Equal(got, []string{"1.0.0 release/1 new [Add the rule.]"}) {
		t.Fatalf("versions are %q", got)
	}
	if got := groups(t, connString); !slices.Equal(got, []string{"techs/go Go"}) {
		t.Fatalf("groups are %q", got)
	}
}

func newStore(t *testing.T) (*ingest.Store, string) {
	t.Helper()
	db, connString := ingesttest.NewDatabase(t)
	return ingest.NewStore(db), connString
}

// query runs a query returning one row and scans it into dest.
func query(t *testing.T, connString, sql string, dest ...any) {
	t.Helper()
	conn := connect(t, connString)
	if err := conn.QueryRow(context.Background(), sql).Scan(dest...); err != nil {
		t.Fatalf("run %q: %v", sql, err)
	}
}

// lines runs a query returning one text column and returns its values.
func lines(t *testing.T, connString, sql string, args ...any) []string {
	t.Helper()
	conn := connect(t, connString)
	rows, err := conn.Query(context.Background(), sql, args...)
	if err != nil {
		t.Fatalf("run %q: %v", sql, err)
	}
	values, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("run %q: %v", sql, err)
	}
	return values
}

// versions describes each of a rule's versions, oldest first, as "<version> release/<n> <change> [<summaries>]".
func versions(t *testing.T, connString, ruleID string) []string {
	t.Helper()
	return lines(t, connString, `SELECT format('%s.%s.%s release/%s %s [%s]', v.major, v.minor, v.patch, rel.number, v.change,
		array_to_string(v.summaries, ' ')) FROM rule_versions v JOIN rules r ON r.id = v.rule_id
		JOIN library_releases rel ON rel.id = v.release_id WHERE r.path = $1 ORDER BY rel.number`, ruleID)
}

// groups describes each stored group as "<id> <name>".
func groups(t *testing.T, connString string) []string {
	t.Helper()
	return lines(t, connString, `SELECT path || ' ' || name FROM library_groups ORDER BY path`)
}

// currentContent returns the title and HTML of a rule's version with content.
func currentContent(t *testing.T, connString, ruleID string) (title, html string) {
	t.Helper()
	query(t, connString, `SELECT v.title, v.html FROM rule_versions v JOIN rules r ON r.id = v.rule_id WHERE r.path = '`+ruleID+`' AND v.html IS NOT NULL`, &title, &html)
	return title, html
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

// catalog returns every catalog row as text, in a stable order, with each row's transaction ID, so it changes
// when any row is written, even with the same values.
func catalog(t *testing.T, connString string) string {
	t.Helper()
	var all []string
	for _, table := range []string{"libraries", "library_releases", "library_groups", "rules", "rule_versions"} {
		all = append(all, lines(t, connString, `SELECT t.xmin::text || ' ' || t::text FROM `+table+` t ORDER BY t::text`)...)
	}
	return strings.Join(all, "\n")
}

func connect(t *testing.T, connString string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), connString)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	return conn
}
