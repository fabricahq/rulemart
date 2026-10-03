package app_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
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

// listingKey is the key of the repository these tests list: GitHub repository 42, example/rules.
var listingKey = domain.LibraryKey{Host: domain.GitHub, RepositoryID: "42"}

// listing is a catalog where an account lists example/rules as the web function does, and the worker checks it.
type listing struct {
	listings app.Listings
	ingester app.Ingester
	pages    app.Pages
	queue    *recordingQueue
	// hosted describes the code host's repositories: example/rules, unless a test changes it.
	hosted *hostedRepositories
	// account is the account that lists, and connString the database's, as its owner.
	account    int64
	connString string
}

// recordingQueue keeps what's sent to it, or fails with err.
type recordingQueue struct {
	bodies []string
	err    error
}

func (q *recordingQueue) Send(_ context.Context, body string) error {
	if q.err != nil {
		return q.err
	}
	q.bodies = append(q.bodies, body)
	return nil
}

// hostedRepositories describes the code host's repositories: every name and ID as repo, unless err is set, and
// counts the lookups.
type hostedRepositories struct {
	repo    domain.Repository
	err     error
	lookups int
}

func (h *hostedRepositories) Repository(context.Context, string, string) (domain.Repository, error) {
	h.lookups++
	return h.repo, h.err
}

func (h *hostedRepositories) RepositoryByID(context.Context, string) (domain.Repository, error) {
	h.lookups++
	return h.repo, h.err
}

// newListing returns a catalog in a new database with one account, where the code host describes repository 42 as
// lib's. Listings connect as the web function's role, and checks as the worker's.
func newListing(t *testing.T, lib *gittest.Library) *listing {
	t.Helper()
	_, connString := databasetest.New(t)
	web := postgres.New(databasetest.AsWebRole(t, connString))
	l := &listing{queue: &recordingQueue{}, hosted: &hostedRepositories{repo: lib.Repository(42)}, connString: connString}
	l.listings = app.Listings{Store: web, Queue: l.queue}
	l.ingester = app.Ingester{
		Repositories: l.hosted, Fetch: git.Fetch, List: git.ListReleaseTags, Render: render.Renderer{},
		Store: postgres.New(databasetest.AsWorkerRole(t, connString)), Limits: domain.DefaultLimits,
	}
	l.pages = app.Pages{Store: web}
	postgrestest.QueryRow(t, connString,
		"INSERT INTO accounts (github_user_id, github_login, avatar_url) VALUES (1, 'lister', '') RETURNING id", &l.account)
	return l
}

// list lists example/rules, and fails t if it can't.
func (l *listing) list(t *testing.T) int64 {
	t.Helper()
	id, err := l.listings.List(context.Background(), l.account, "https://github.com/example/rules")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// check checks the listing id as the worker does, with nothing vetted.
func (l *listing) check(t *testing.T, id int64) (app.ListingCheck, error) {
	t.Helper()
	return l.ingester.CheckListing(context.Background(), nil, id)
}

// state returns the account's only listing.
func (l *listing) only(t *testing.T) (state domain.ListingState, failure string) {
	t.Helper()
	listings, err := l.listings.AccountListings(context.Background(), l.account)
	if err != nil || len(listings) != 1 {
		t.Fatalf("got %+v, %v; want one listing", listings, err)
	}
	return listings[0].State, listings[0].Failure
}

func TestListingQueuesTheListingsCheck(t *testing.T) {
	l := newListing(t, firstRelease(t))

	id := l.list(t)

	if want := []string{fmt.Sprintf(`{"listing":%d}`, id)}; !slices.Equal(l.queue.bodies, want) {
		t.Fatalf("queued %q, want %q", l.queue.bodies, want)
	}
	if state, _ := l.only(t); state != domain.ListingChecking {
		t.Fatalf("the listing is %s, want checking", state)
	}
}

// A listing whose check couldn't be queued stays listed, for the hourly poll to check.
func TestListingKeepsAListingWhoseCheckWasntQueued(t *testing.T) {
	l := newListing(t, firstRelease(t))
	l.queue.err = errors.New("SQS is down")

	id, err := l.listings.List(context.Background(), l.account, "example/rules")

	if !errors.Is(err, app.ErrNotQueued) || id == 0 {
		t.Fatalf("got %d, %v; want the listing's ID and app.ErrNotQueued", id, err)
	}
	if checks, err := l.ingester.ListingsToCheck(context.Background(), nil); err != nil || !slices.Equal(checks, []int64{id}) {
		t.Fatalf("the poll checks %v, %v; want %d", checks, err, id)
	}
}

func TestListingRefusesTextThatNamesNoRepository(t *testing.T) {
	l := newListing(t, firstRelease(t))

	for _, text := range []string{"", "example", "https://gitlab.com/example/rules"} {
		if _, err := l.listings.Check(context.Background(), l.account, text); !errors.Is(err, app.ErrInvalidRepository) {
			t.Errorf("check %q: got %v, want app.ErrInvalidRepository", text, err)
		}
		if _, err := l.listings.List(context.Background(), l.account, text); !errors.Is(err, app.ErrInvalidRepository) {
			t.Errorf("list %q: got %v, want app.ErrInvalidRepository", text, err)
		}
	}
	if repo, err := l.listings.Check(context.Background(), l.account, " github.com/example/rules.git "); err != nil || repo.FullName() != "example/rules" {
		t.Errorf("got %+v, %v; want example/rules", repo, err)
	}
}

// The worker looks a listing's repository up by name once, ingests it, and its pages show it unvetted; the next check
// only lists its tags.
func TestCheckListingIngestsTheLibraryAsUnvetted(t *testing.T) {
	l := newListing(t, firstRelease(t))
	id := l.list(t)

	check, err := l.check(t, id)

	if err != nil || check.Outcome != app.ListingIngested || check.Listing.Library != listingKey || check.Update.Result.Rules != 3 {
		t.Fatalf("got %+v, %v; want an ingestion of repository 42", check, err)
	}
	if state, failure := l.only(t); state != domain.ListingListed || failure != "" {
		t.Fatalf("the listing is %s, %q; want listed", state, failure)
	}
	page, err := l.pages.LibraryPage(context.Background(), "example", "rules")
	if err != nil || page.Library.Vetted {
		t.Fatalf("got %+v, %v; want example/rules, unvetted", page.Library, err)
	}
	l.hosted.lookups = 0
	if check, err := l.check(t, id); err != nil || check.Outcome != app.ListingUnchanged || l.hosted.lookups != 0 {
		t.Fatalf("checking again: got %+v, %v, with %d lookups; want unchanged, without a lookup", check, err, l.hosted.lookups)
	}
}

// A failure the repository caused is the lister's to see, recorded on the listing, and the check succeeds, so the
// queue doesn't retry it.
func TestCheckListingRecordsWhatTheRepositoryGotWrong(t *testing.T) {
	for name, test := range map[string]struct {
		prepare func(*testing.T, *listing)
		want    string
		// polled is true when the hourly poll checks the listing again, since GitHub has its repository and fetching
		// may have failed for a moment.
		polled bool
	}{
		"a missing or private repository": {
			func(_ *testing.T, l *listing) {
				l.hosted.err = fmt.Errorf("look up repository=%q on GitHub: GitHub has %w", "example/rules", domain.ErrNoPublicRepository)
			},
			"GitHub has no public repository by this name.", false,
		},
		"a repository without releases": {
			func(t *testing.T, l *listing) { l.hosted.repo = gittest.NewLibrary(t).Repository(42) },
			"The repository has no release/<number> tags", true,
		},
		"a repository another listing names": {
			func(t *testing.T, l *listing) {
				postgrestest.Exec(t, l.connString, `WITH other AS (INSERT INTO accounts (github_user_id, github_login, avatar_url) VALUES (2, 'other', '') RETURNING id)
					INSERT INTO listings (account_id, host, owner, name, host_repository_id) SELECT id, 'github', 'old', 'name', '42' FROM other`)
			},
			"Another listing names this repository, which GitHub calls example/rules now.", false,
		},
		"an invalid release": {
			func(t *testing.T, l *listing) {
				lib := gittest.NewLibrary(t)
				lib.Release(1, "formatVersion: 1\nrelease: 2\n")
				l.hosted.repo = lib.Repository(42)
			},
			"release/1: invalid release record", true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			l := newListing(t, firstRelease(t))
			id := l.list(t)
			test.prepare(t, l)

			check, err := l.check(t, id)

			if err != nil || check.Outcome != app.ListingRefused || !strings.Contains(check.Failure, test.want) {
				t.Fatalf("got %+v, %v; want refused for %q", check, err, test.want)
			}
			if state, failure := l.only(t); state != domain.ListingFailed || failure != check.Failure {
				t.Fatalf("the listing is %s, %q; want failed with %q", state, failure, check.Failure)
			}
			if checks, err := l.ingester.ListingsToCheck(context.Background(), nil); err != nil || slices.Contains(checks, id) != test.polled {
				t.Fatalf("the poll checks %v, %v; want %d among them: %v", checks, err, id, test.polled)
			}
		})
	}
}

// A failure that isn't the repository's, such as GitHub's API refusing a request, fails the check for the queue to
// retry, and leaves the listing as it was.
func TestCheckListingFailsWhenTheFailureIsntTheRepositorys(t *testing.T) {
	l := newListing(t, firstRelease(t))
	id := l.list(t)
	l.hosted.err = errors.New("GitHub answered 403 Forbidden")

	if _, err := l.check(t, id); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("got %v, want the API's failure", err)
	}
	if state, failure := l.only(t); state != domain.ListingChecking || failure != "" {
		t.Fatalf("the listing is %s, %q; want still checking", state, failure)
	}
}

// A listed library whose new release fails keeps showing the last release it ingested, and its lister sees why.
func TestCheckListingKeepsTheLastReleaseWhenALaterOneFails(t *testing.T) {
	lib := firstRelease(t)
	l := newListing(t, lib)
	id := l.list(t)
	if _, err := l.check(t, id); err != nil {
		t.Fatal(err)
	}
	lib.Release(2, "formatVersion: 1\nrelease: 3\n")

	check, err := l.check(t, id)

	if err != nil || check.Outcome != app.ListingRefused {
		t.Fatalf("got %+v, %v; want refused", check, err)
	}
	if state, failure := l.only(t); state != domain.ListingListed || failure == "" {
		t.Fatalf("the listing is %s, %q; want listed, with the failure", state, failure)
	}
	if page, err := l.pages.LibraryPage(context.Background(), "example", "rules"); err != nil || page.Library.LatestRelease != 1 {
		t.Fatalf("got %+v, %v; want release 1", page.Library, err)
	}
	if checks, err := l.ingester.ListingsToCheck(context.Background(), nil); err != nil || !slices.Equal(checks, []int64{id}) {
		t.Fatalf("the poll checks %v, %v; want %d, every hour", checks, err, id)
	}
}

// A listing that's gone, or names a vetted library, needs no check: the vetted library's own job updates it.
func TestCheckListingSkipsAListingThatsGoneOrVetted(t *testing.T) {
	l := newListing(t, firstRelease(t))
	id := l.list(t)

	check, err := l.ingester.CheckListing(context.Background(), []domain.LibraryKey{listingKey}, id)
	if err != nil || check.Outcome != app.ListingSkipped || check.Update.Ingested {
		t.Fatalf("a vetted library: got %+v, %v; want skipped", check, err)
	}
	if listings, _ := l.listings.AccountListings(context.Background(), l.account); listings[0].CheckedAt.IsZero() {
		t.Fatalf("the check that resolved it recorded nothing: %+v", listings[0])
	}
	if err := l.listings.Remove(context.Background(), l.account, id); err != nil {
		t.Fatal(err)
	}
	if check, err := l.check(t, id); err != nil || check.Outcome != app.ListingSkipped {
		t.Fatalf("a removed listing: got %+v, %v; want skipped", check, err)
	}
}

// Trying a failed listing again clears its failure and queues its check.
func TestRetryQueuesAFailedListingsCheck(t *testing.T) {
	l := newListing(t, firstRelease(t))
	id := l.list(t)
	l.hosted.err = fmt.Errorf("GitHub has %w", domain.ErrNoPublicRepository)
	if _, err := l.check(t, id); err != nil {
		t.Fatal(err)
	}
	l.queue.bodies, l.hosted.err = nil, nil

	if err := l.listings.Retry(context.Background(), l.account, id); err != nil {
		t.Fatal(err)
	}

	if state, failure := l.only(t); state != domain.ListingChecking || failure != "" {
		t.Fatalf("the listing is %s, %q; want checking", state, failure)
	}
	if want := []string{fmt.Sprintf(`{"listing":%d}`, id)}; !slices.Equal(l.queue.bodies, want) {
		t.Fatalf("queued %q, want %q", l.queue.bodies, want)
	}
	if check, err := l.check(t, id); err != nil || check.Outcome != app.ListingIngested {
		t.Fatalf("got %+v, %v; want an ingestion", check, err)
	}
}
