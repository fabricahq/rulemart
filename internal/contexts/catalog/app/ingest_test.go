package app_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/render"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git/gittest"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/platform/database/databasetest"
)

const (
	retryLimits  = "practices/testing/verify-retry-limits"
	retryBackoff = "practices/testing/check-retry-backoff"
	returnErrors = "techs/go/return-errors"
)

// firstRelease publishes three rules in two groups.
func firstRelease(t *testing.T) *gittest.Library {
	t.Helper()
	lib := gittest.NewLibrary(t)
	lib.Group("practices/testing", "Testing")
	lib.Group("techs/go", "Go")
	lib.Rule(retryLimits, "Verify retry limits", "Every retry loop stops after a fixed number of attempts.")
	lib.Rule(retryBackoff, "Check retry backoff", "Retries wait longer after each attempt.")
	lib.Rule(returnErrors, "Return errors", "Return errors instead of panicking.")
	lib.Release(1, firstReleaseRecord)
	return lib
}

// firstReleaseRecord is the record of firstRelease's release/1.
const firstReleaseRecord = `formatVersion: 1
release: 1
rules:
  practices/testing/check-retry-backoff: 1.0.0
  practices/testing/verify-retry-limits: 1.0.0
  techs/go/return-errors: 1.0.0
changes:
  practices/testing/check-retry-backoff: {change: new, summaries: [Add the rule.]}
  practices/testing/verify-retry-limits: {change: new, summaries: [Add the rule.]}
  techs/go/return-errors: {change: new, summaries: [Add the rule.]}
libraryFiles: [LICENSE, practices/testing/_group.yaml, rule-library.yaml, techs/go/_group.yaml]
`

// laterReleases adds three releases to firstRelease's library: a minor change, then a patch and a major change and
// a retirement, then a release that only edits a rule's file without a change, which must not show.
func laterReleases(t *testing.T, lib *gittest.Library) {
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

// Ingestion stores what the releases publish, as the pages read it: each change level, a retirement, and each
// version's content at the release that published it, not a later edit no release recorded.
func TestIngestStoresWhatTheReleasesPublish(t *testing.T) {
	ingester, pages := newCatalog(t)
	lib := firstRelease(t)
	laterReleases(t, lib)

	result, err := ingest(ingester, lib.Repository(42))

	if err != nil {
		t.Fatal(err)
	}
	if result.Releases != 4 || result.Rules != 2 || result.Repository != lib.Repository(42) {
		t.Fatalf("ingested %+v, want 4 releases and 2 current rules", result)
	}
	page, err := pages.LibraryPage(context.Background(), "example", "rules")
	if err != nil {
		t.Fatal(err)
	}
	if page.Library.LatestRelease != 4 || page.Library.LicenseExpression != "MIT" || len(page.Groups) != 2 {
		t.Errorf("the library page is %+v", page)
	}
	var rules []string
	for _, r := range page.Rules {
		rules = append(rules, r.Path+" "+r.Version.String()+" "+r.Title)
	}
	if want := []string{retryLimits + " 1.1.1 Verify retry limits", returnErrors + " 2.0.0 Return errors with context"}; !slices.Equal(rules, want) {
		t.Errorf("current rules are %q, want %q", rules, want)
	}
	rule, err := pages.RulePage(context.Background(), "example", "rules", returnErrors)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rule.Rule.HTML, "operation that failed") || strings.Contains(rule.Rule.HTML, "<h2") || rule.Rule.Release != 3 {
		t.Errorf("%s shows release/%d's %s", returnErrors, rule.Rule.Release, rule.Rule.HTML)
	}
	var versions []string
	for _, v := range rule.Versions {
		versions = append(versions, v.Version.String()+" "+domain.ReleaseTag(v.Release)+" "+string(v.Change)+" "+strings.Join(v.Summaries, " | "))
	}
	if want := []string{"2.0.0 release/3 major Require context on every returned error. | Add an example.", "1.0.0 release/1 new Add the rule."}; !slices.Equal(versions, want) {
		t.Errorf("versions are %q, want %q", versions, want)
	}
	retired, err := pages.RulePage(context.Background(), "example", "rules", retryBackoff)
	if err != nil || retired.Rule.Title != "Check retry backoff" || retired.Rule.Retirement == nil || retired.Rule.Retirement.Release != 3 {
		t.Errorf("the retired rule %s is %+v, %v; want its last title, retired in release/3", retryBackoff, retired.Rule, err)
	}
	comparison, err := pages.RuleComparison(context.Background(), "example", "rules", returnErrors, rule.Versions[1].Version, rule.Versions[0].Version)
	if err != nil {
		t.Fatal(err)
	}
	if text := comparison.Text; !strings.Contains(text.Old, "Return errors instead of panicking.") || !strings.Contains(text.New, "operation that failed") {
		t.Errorf("%s 1.0.0...2.0.0 compares %+v, want release/1's file and release/3's", returnErrors, text)
	}
}

func TestIngestChangesNothingWhenRunAgain(t *testing.T) {
	ingester, _ := newCatalog(t)
	lib := firstRelease(t)
	laterReleases(t, lib)
	if _, err := ingest(ingester, lib.Repository(42)); err != nil {
		t.Fatal(err)
	}

	result, err := ingest(ingester, lib.Repository(42))

	if err != nil || result.Changed != 0 {
		t.Fatalf("the second ingestion changed %d rows, %v", result.Changed, err)
	}
}

// hugeObject is larger, inflated, than ingestion holds in memory for one object. It's zeros, so it compresses to
// almost nothing and only its inflated size can stop it.
var hugeObject = strings.Repeat("\x00", 40<<20)

// A refused library writes nothing, whichever step refuses it: fetching the tags, or assembling what they publish.
func TestIngestWritesNothingWhenItRefusesALibrary(t *testing.T) {
	for name, release := range map[string]func(lib *gittest.Library){
		"a record that isn't YAML": func(lib *gittest.Library) { lib.Release(2, "formatVersion: 1\nrelease: [2\n") },
		"an object too large to fetch": func(lib *gittest.Library) {
			lib.Write("assets/huge.bin", hugeObject)
			lib.Release(2, "formatVersion: 1\nrelease: 2\nrules:\n  practices/testing/check-retry-backoff: 1.0.0\n"+
				"  practices/testing/verify-retry-limits: 1.0.0\n  techs/go/return-errors: 1.0.0\n")
		},
		"a rule dropped without retiring it": func(lib *gittest.Library) {
			lib.Release(2, "formatVersion: 1\nrelease: 2\nrules:\n  practices/testing/verify-retry-limits: 1.0.0\n"+
				"  techs/go/return-errors: 1.0.0\n")
		},
	} {
		t.Run(name, func(t *testing.T) {
			ingester, pages := newCatalog(t)
			lib := firstRelease(t)
			if _, err := ingest(ingester, lib.Repository(42)); err != nil {
				t.Fatal(err)
			}
			before := read(t, pages)
			lib.Rule(retryLimits, "Verify retry limits", "A change in a refused release.")
			release(lib)

			_, err := ingest(ingester, lib.Repository(42))

			if err == nil {
				t.Fatal("ingested a library that should be refused")
			}
			if after := read(t, pages); !reflect.DeepEqual(after, before) {
				t.Fatalf("a refused ingestion changed the pages:\nbefore: %+v\nafter:  %+v", before, after)
			}
		})
	}
}

func TestIngestRejectsAURLThatIsntAGitHubRepository(t *testing.T) {
	ingester := app.Ingester{Repositories: repositories{}, Fetch: git.Fetch, Limits: domain.DefaultLimits}

	_, err := ingester.Ingest(context.Background(), "https://gitlab.com/example/rules")

	if err == nil || !strings.Contains(err.Error(), "expected https://github.com/<owner>/<repository>") {
		t.Fatalf("got error %v, want one asking for a GitHub repository URL", err)
	}
}

// repositories describes every repository as repo, as GitHub would describe it.
type repositories struct{ repo domain.Repository }

func (r repositories) Repository(context.Context, string, string) (domain.Repository, error) {
	return r.repo, nil
}

func (r repositories) RepositoryByID(context.Context, string) (domain.Repository, error) {
	return r.repo, nil
}

// newCatalog returns an Ingester that fetches with go-git and writes to a new database, and the Pages that read it,
// where example/rules with GitHub repository ID 42 is vetted.
func newCatalog(t *testing.T) (app.Ingester, app.Pages) {
	t.Helper()
	db, _ := databasetest.New(t)
	store := postgres.New(db)
	ingester := app.Ingester{Fetch: git.Fetch, Render: render.Rule, Store: store, Limits: domain.DefaultLimits}
	return ingester, app.Pages{Store: store, Vetted: []domain.LibraryKey{{Host: domain.GitHub, RepositoryID: "42"}}}
}

// ingest ingests the library in repo with ingester.
func ingest(ingester app.Ingester, repo domain.Repository) (app.Result, error) {
	ingester.Repositories = repositories{repo}
	return ingester.Ingest(context.Background(), "https://github.com/"+repo.FullName())
}

// page is what the pages read of firstRelease's library.
type page struct {
	library views.LibraryPage
	rule    views.RulePage
}

// read returns what the pages read of firstRelease's library.
func read(t *testing.T, pages app.Pages) page {
	t.Helper()
	library, err := pages.LibraryPage(context.Background(), "example", "rules")
	if err != nil {
		t.Fatal(err)
	}
	rule, err := pages.RulePage(context.Background(), "example", "rules", retryLimits)
	if err != nil {
		t.Fatal(err)
	}
	return page{library, rule}
}

// Rendering is what an ingestion spends its time on, and a job has a deadline, so ingestion must stop rendering when
// its context ends rather than run on until Lambda stops the function.
func TestIngestStopsRenderingWhenItsContextEnds(t *testing.T) {
	ingester, pages := newCatalog(t)
	lib := firstRelease(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rendered := 0
	ingester.Render = func(body string, page domain.RulePage, allowance int64) (string, int64, error) {
		rendered++
		cancel()
		return render.Rule(body, page, allowance)
	}
	ingester.Repositories = repositories{lib.Repository(42)}

	_, err := ingester.IngestRepository(ctx, lib.Repository(42))

	if !errors.Is(err, context.Canceled) && (err == nil || !strings.Contains(err.Error(), context.Canceled.Error())) {
		t.Fatalf("got error %v, want the context's", err)
	}
	if rendered != 1 {
		t.Fatalf("rendered %d rules after the context ended, want to stop after the first", rendered)
	}
	if _, err := pages.LibraryPage(context.Background(), "example", "rules"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a stopped ingestion stored the library: %v", err)
	}
}
