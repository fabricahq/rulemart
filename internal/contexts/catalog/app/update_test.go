package app_test

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/render"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git/gittest"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres"
	"github.com/fabricahq/rulemart/internal/platform/database/databasetest"
	"github.com/fabricahq/rulemart/internal/platform/postgrestest"
)

// exampleKey is the vetted key of the libraries these tests update: GitHub repository 42, example/rules.
var exampleKey = domain.LibraryKey{Host: domain.GitHub, RepositoryID: "42"}

// updates is a catalog that updates libraries as the worker does, and counts what each update asked of the code
// host and the library's repository.
type updates struct {
	ingester app.Ingester
	pages    app.Pages
	// connString is the catalog database's, as its owner.
	connString string
	// lookups counts repository lookups on the code host, and fetches the release tags fetched.
	lookups, fetches int
}

// newUpdates returns a catalog in a new database where the code host describes repository 42 as lib's, and the
// updates connect as the worker's role, so only what the migrations grant its group lets them write.
func newUpdates(t *testing.T, lib *gittest.Library) *updates {
	t.Helper()
	owner, connString := databasetest.New(t)
	u := &updates{connString: connString}
	u.ingester = app.Ingester{
		Repositories: countedRepositories{repo: lib.Repository(42), lookups: &u.lookups},
		Fetch: func(ctx context.Context, url string, limits domain.FetchLimits) ([]domain.ReleaseSnapshot, error) {
			u.fetches++
			return git.Fetch(ctx, url, limits)
		},
		List:   git.ListReleaseTags,
		Render: render.Rule,
		Store:  postgres.New(databasetest.AsWorkerRole(t, connString)),
		Limits: domain.DefaultLimits,
	}
	u.pages = app.Pages{Store: postgres.New(owner), Vetted: []domain.LibraryKey{exampleKey}}
	return u
}

// update updates the example library, and resets the counts first.
func (u *updates) update(t *testing.T) (app.Update, error) {
	t.Helper()
	u.lookups, u.fetches = 0, 0
	return u.ingester.Update(context.Background(), exampleKey)
}

// countedRepositories describes repository 42 as repo, and counts the lookups.
type countedRepositories struct {
	repo    domain.Repository
	lookups *int
}

func (r countedRepositories) Repository(context.Context, string, string) (domain.Repository, error) {
	*r.lookups++
	return r.repo, nil
}

func (r countedRepositories) RepositoryByID(_ context.Context, id string) (domain.Repository, error) {
	*r.lookups++
	if id != r.repo.ID {
		return domain.Repository{}, os.ErrNotExist
	}
	return r.repo, nil
}

func TestUpdateIngestsALibraryTheCatalogDoesntHaveYet(t *testing.T) {
	lib := firstRelease(t)
	u := newUpdates(t, lib)

	update, err := u.update(t)

	if err != nil {
		t.Fatal(err)
	}
	if !update.Ingested || update.Result.Releases != 1 || update.Result.Rules != 3 {
		t.Fatalf("updated %+v, want an ingestion of 1 release and 3 rules", update)
	}
	if page := read(t, u.pages); page.library.Library.LatestRelease != 1 {
		t.Fatalf("the library page shows release %d, want 1", page.library.Library.LatestRelease)
	}
}

// Every check after the first lists the tags; only a release it hasn't stored makes it look the repository up and
// fetch it.
func TestUpdateIngestsANewRelease(t *testing.T) {
	lib := firstRelease(t)
	u := newUpdates(t, lib)
	if _, err := u.update(t); err != nil {
		t.Fatal(err)
	}
	laterReleases(t, lib)

	update, err := u.update(t)

	if err != nil {
		t.Fatal(err)
	}
	if !update.Ingested || update.Result.Releases != 4 || update.Result.Changed == 0 || u.lookups != 1 || u.fetches != 1 {
		t.Fatalf("updated %+v with %d lookups and %d fetches, want one ingestion of 4 releases", update, u.lookups, u.fetches)
	}
	if page := read(t, u.pages); page.library.Library.LatestRelease != 4 || page.rule.Rule.Version.String() != "1.1.1" {
		t.Fatalf("the pages show release %d and %s %s, want release 4 and 1.1.1", page.library.Library.LatestRelease,
			retryLimits, page.rule.Rule.Version)
	}
}

// A release's record is in its tag, so a tag rewritten on the same commit is a change to ingest.
func TestUpdateIngestsARewrittenReleaseTag(t *testing.T) {
	lib := firstRelease(t)
	u := newUpdates(t, lib)
	if _, err := u.update(t); err != nil {
		t.Fatal(err)
	}
	lib.Retag("release/1", "Library release 1.\n---\n"+strings.ReplaceAll(firstReleaseRecord, "verify-retry-limits: {change: new, summaries: [Add the rule.]}",
		"verify-retry-limits: {change: new, summaries: [Add the rule in a rewritten tag.]}"))

	update, err := u.update(t)

	if err != nil || !update.Ingested {
		t.Fatalf("updated %+v, %v; want an ingestion", update, err)
	}
	if summaries := read(t, u.pages).rule.Versions[0].Summaries; !reflect.DeepEqual(summaries, []string{"Add the rule in a rewritten tag."}) {
		t.Fatalf("the rule page shows %q, want the rewritten tag's summary", summaries)
	}
}

// The check is what keeps a poll every ten minutes cheap: unchanged tags cost one listing, with no lookup on the code
// host and no fetch, and change nothing.
func TestUpdateStopsAtTheCheckWhenTheTagsAreUnchanged(t *testing.T) {
	lib := firstRelease(t)
	laterReleases(t, lib)
	u := newUpdates(t, lib)
	if _, err := u.update(t); err != nil {
		t.Fatal(err)
	}
	before := read(t, u.pages)

	update, err := u.update(t)

	if err != nil {
		t.Fatal(err)
	}
	if update.Ingested || u.lookups != 0 || u.fetches != 0 {
		t.Fatalf("updated %+v with %d lookups and %d fetches, want neither", update, u.lookups, u.fetches)
	}
	if after := read(t, u.pages); !reflect.DeepEqual(after, before) {
		t.Fatal("an update of unchanged tags changed the pages")
	}
}

// A library a release before the check stored has no clone URL or tag IDs, so its first update ingests it again,
// which records them, and the next one stops at the check.
func TestUpdateIngestsALibraryStoredWithoutItsTagsOnce(t *testing.T) {
	lib := firstRelease(t)
	u := newUpdates(t, lib)
	if _, err := u.update(t); err != nil {
		t.Fatal(err)
	}
	postgrestest.Exec(t, u.connString, "UPDATE libraries SET clone_url = NULL")
	postgrestest.Exec(t, u.connString, "UPDATE library_releases SET tag_object_id = NULL")

	first, err := u.update(t)
	if err != nil {
		t.Fatal(err)
	}
	second, err := u.update(t)
	if err != nil {
		t.Fatal(err)
	}

	if !first.Ingested || first.Result.Changed == 0 || second.Ingested {
		t.Fatalf("updated %+v, then %+v; want one ingestion that records the tags, then none", first, second)
	}
}

// An unreachable remote fails the update, so the worker retries it and its alarm reports it, and the catalog keeps
// what it had.
func TestUpdateFailsWithoutWritingWhenTheRemoteIsUnreachable(t *testing.T) {
	lib := firstRelease(t)
	u := newUpdates(t, lib)
	if _, err := u.update(t); err != nil {
		t.Fatal(err)
	}
	before := read(t, u.pages)
	if err := os.RemoveAll(lib.URL()); err != nil {
		t.Fatal(err)
	}

	_, err := u.update(t)

	if err == nil || !strings.Contains(err.Error(), "repository=42") || !strings.Contains(err.Error(), "list the repository's references") {
		t.Fatalf("got error %v, want one naming the library and the failed listing", err)
	}
	if after := read(t, u.pages); !reflect.DeepEqual(after, before) {
		t.Fatal("a failed update changed the pages")
	}
}

// A malformed release fails every update until it's fixed, and leaves the catalog at the last release it ingested.
func TestUpdateKeepsTheLastIngestedReleaseWhileANewOneIsMalformed(t *testing.T) {
	lib := firstRelease(t)
	u := newUpdates(t, lib)
	if _, err := u.update(t); err != nil {
		t.Fatal(err)
	}
	before := read(t, u.pages)
	lib.Rule(retryLimits, "Verify retry limits", "A change in a malformed release.")
	lib.Release(2, "formatVersion: 1\nrelease: [2\n")

	for attempt := 1; attempt <= 2; attempt++ {
		_, err := u.update(t)

		if err == nil || !strings.Contains(err.Error(), "release/2") {
			t.Fatalf("attempt %d: got error %v, want one naming release/2", attempt, err)
		}
		if after := read(t, u.pages); !reflect.DeepEqual(after, before) {
			t.Fatalf("attempt %d: a malformed release changed the pages", attempt)
		}
	}
}

// The worker's role writes through grants a migration gives its group. If one were missing, ingestion must fail and
// say so, and write nothing, rather than store part of a library.
func TestUpdateFailsLoudlyWithoutWritingWhenAGrantIsMissing(t *testing.T) {
	lib := firstRelease(t)
	u := newUpdates(t, lib)
	if _, err := u.update(t); err != nil {
		t.Fatal(err)
	}
	before := read(t, u.pages)
	// Grants belong to this test's database, so revoking one here leaves other tests' databases alone.
	postgrestest.Exec(t, u.connString, "REVOKE INSERT ON rule_versions FROM "+postgrestest.CatalogWriterRole)
	laterReleases(t, lib)

	_, err := u.update(t)

	if err == nil || !strings.Contains(err.Error(), "permission denied for table rule_versions") {
		t.Fatalf("got error %v, want permission denied for rule_versions", err)
	}
	if after := read(t, u.pages); !reflect.DeepEqual(after, before) {
		t.Fatal("an ingestion without its grant changed the pages")
	}
}

// The worker logs how long each part of an update took, so a slow library shows whether listing its tags or ingesting
// it is the slow part. An update that stopped at the check spent no time ingesting, and a library the catalog didn't
// have yet spent none listing.
func TestUpdateReportsHowLongListingAndIngestionTook(t *testing.T) {
	lib := firstRelease(t)
	u := newUpdates(t, lib)

	first, err := u.update(t)
	if err != nil {
		t.Fatal(err)
	}
	unchanged, err := u.update(t)
	if err != nil {
		t.Fatal(err)
	}
	laterReleases(t, lib)
	changed, err := u.update(t)
	if err != nil {
		t.Fatal(err)
	}

	for name, tc := range map[string]struct {
		update           app.Update
		listed, ingested bool
	}{
		"a library the catalog didn't have": {first, false, true},
		"unchanged tags":                    {unchanged, true, false},
		"a new release":                     {changed, true, true},
	} {
		if (tc.update.ListTime > 0) != tc.listed || (tc.update.IngestTime > 0) != tc.ingested {
			t.Errorf("%s: listing took %s and ingestion %s; want listing %v and ingestion %v", name,
				tc.update.ListTime, tc.update.IngestTime, tc.listed, tc.ingested)
		}
	}
}
