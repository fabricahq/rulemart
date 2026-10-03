package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strconv"
	"testing"

	"github.com/aws/aws-lambda-go/events"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/render"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git/gittest"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres"
	"github.com/fabricahq/rulemart/internal/platform/database/databasetest"
	"github.com/fabricahq/rulemart/internal/platform/postgrestest"
)

var (
	first  = domain.LibraryKey{Host: domain.GitHub, RepositoryID: "42"}
	second = domain.LibraryKey{Host: domain.GitHub, RepositoryID: "43"}
)

// fakeUpdater records the libraries it's asked to update, and fails those in failing.
type fakeUpdater struct {
	updated []domain.LibraryKey
	failing map[domain.LibraryKey]bool
}

func (u *fakeUpdater) Update(_ context.Context, library domain.LibraryKey) (app.Update, error) {
	u.updated = append(u.updated, library)
	if u.failing[library] {
		return app.Update{}, errors.New("the remote is unreachable")
	}
	return app.Update{}, nil
}

func newTestWorker(updater updater, queue sender) *worker {
	return &worker{
		updater: updater, vetted: []domain.LibraryKey{first, second}, queue: queue,
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// sqsEvent returns an SQS event with one record for each body, whose message IDs are their positions.
func sqsEvent(bodies ...string) json.RawMessage {
	event := events.SQSEvent{}
	for i, body := range bodies {
		event.Records = append(event.Records, events.SQSMessage{MessageId: strconv.Itoa(i), Body: body, EventSource: "aws:sqs"})
	}
	raw, _ := json.Marshal(event)
	return raw
}

// failures returns the message IDs a response to an SQS event reported as failed.
func failures(t *testing.T, out any) []string {
	t.Helper()
	resp, ok := out.(events.SQSEventResponse)
	if !ok {
		t.Fatalf("answered %#v, want an SQS event response", out)
	}
	ids := []string{}
	for _, f := range resp.BatchItemFailures {
		ids = append(ids, f.ItemIdentifier)
	}
	return ids
}

func TestHandleQueuesOneJobPerVettedLibraryOnTheSchedule(t *testing.T) {
	queue := &memoryQueue{}
	updater := &fakeUpdater{}

	if _, err := newTestWorker(updater, queue).handle(context.Background(), json.RawMessage(`{"source":"schedule"}`)); err != nil {
		t.Fatal(err)
	}

	want := []string{`{"host":"github","repositoryID":"42"}`, `{"host":"github","repositoryID":"43"}`}
	if !slices.Equal(queue.bodies, want) {
		t.Fatalf("queued %q, want %q", queue.bodies, want)
	}
	if len(updater.updated) != 0 {
		t.Fatalf("the schedule updated %v itself; the jobs should", updater.updated)
	}
}

func TestHandleUpdatesTheLibraryEachJobNames(t *testing.T) {
	updater := &fakeUpdater{}

	out, err := newTestWorker(updater, nil).handle(context.Background(),
		sqsEvent(`{"host":"github","repositoryID":"42"}`, `{"host":"github","repositoryID":"43"}`))

	if err != nil {
		t.Fatal(err)
	}
	if got := failures(t, out); len(got) != 0 {
		t.Fatalf("reported %q as failed", got)
	}
	if !slices.Equal(updater.updated, []domain.LibraryKey{first, second}) {
		t.Fatalf("updated %v", updater.updated)
	}
}

// A failed update must reach SQS as a failure, so it's retried and then dead-lettered, which the alarm reports.
func TestHandleReportsAFailedUpdateForRetry(t *testing.T) {
	updater := &fakeUpdater{failing: map[domain.LibraryKey]bool{first: true}}

	out, err := newTestWorker(updater, nil).handle(context.Background(),
		sqsEvent(`{"host":"github","repositoryID":"42"}`, `{"host":"github","repositoryID":"43"}`))

	if err != nil {
		t.Fatal(err)
	}
	if got := failures(t, out); !slices.Equal(got, []string{"0"}) {
		t.Fatalf("reported %q as failed, want only message 0", got)
	}
}

// A job names a library by its key and nothing else, and only a vetted one, so a queued message can't make the
// worker fetch a repository nobody vetted.
func TestHandleRefusesAJobItDoesntTrust(t *testing.T) {
	for name, body := range map[string]string{
		"a library that isn't vetted": `{"host":"github","repositoryID":"44"}`,
		"another code host":           `{"host":"gitlab","repositoryID":"42"}`,
		"a URL":                       `{"host":"github","repositoryID":"42","url":"https://example.com/rules.git"}`,
		"no repository ID":            `{"host":"github"}`,
		"not JSON":                    `hello`,
		"two jobs":                    `{"host":"github","repositoryID":"42"}{"host":"github","repositoryID":"43"}`,
		"two jobs apart":              `{"host":"github","repositoryID":"42"} {"host":"github","repositoryID":"43"}`,
		"trailing text":               `{"host":"github","repositoryID":"42"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			updater := &fakeUpdater{}

			out, err := newTestWorker(updater, nil).handle(context.Background(), sqsEvent(body))

			if err != nil {
				t.Fatal(err)
			}
			if got := failures(t, out); !slices.Equal(got, []string{"0"}) {
				t.Fatalf("reported %q as failed, want the job", got)
			}
			if len(updater.updated) != 0 {
				t.Fatalf("updated %v", updater.updated)
			}
		})
	}
}

func TestHandleRejectsEventsItDoesNotRecognize(t *testing.T) {
	for name, event := range map[string]string{
		"empty object":                 `{}`,
		"other source":                 `{"source":"aws.events"}`,
		"a record from another source": `{"Records":[{"messageId":"0","eventSource":"aws:sns","body":"{}"}]}`,
		"a Function URL request":       `{"requestContext":{"http":{"method":"GET"}}}`,
		"not a JSON object":            `[]`,
	} {
		t.Run(name, func(t *testing.T) {
			updater := &fakeUpdater{}
			queue := &memoryQueue{}

			_, err := newTestWorker(updater, queue).handle(context.Background(), json.RawMessage(event))

			if err == nil {
				t.Fatal("accepted an unrecognized event")
			}
			if len(updater.updated) != 0 || len(queue.bodies) != 0 {
				t.Fatalf("an unrecognized event updated %v and queued %q", updater.updated, queue.bodies)
			}
		})
	}
}

// The worker's path from end to end, with a queue in memory in place of SQS: the schedule queues a job, the job
// ingests the library as the worker's role, and the next poll finds nothing to do.
func TestPollIngestsAVettedLibraryThenFindsNothingToDo(t *testing.T) {
	lib := gittest.NewLibrary(t)
	lib.Group("techs/go", "Go")
	lib.Rule("techs/go/return-errors", "Return errors", "Return errors instead of panicking.")
	lib.Release(1, `formatVersion: 1
release: 1
rules: {techs/go/return-errors: 1.0.0}
changes: {techs/go/return-errors: {change: new, summaries: [Add the rule.]}}
`)
	_, connString := databasetest.New(t)
	ingester := app.Ingester{
		Repositories: repositories{lib.Repository(42)}, Fetch: git.Fetch, List: git.ListReleaseTags, Render: render.Renderer{},
		Store: postgres.New(databasetest.AsWorkerRole(t, connString)), Limits: domain.DefaultLimits,
	}
	w := &worker{updater: ingester, vetted: []domain.LibraryKey{first}, log: slog.New(slog.NewTextHandler(io.Discard, nil))}

	for poll, wantIngested := range []bool{true, false} {
		updates, err := w.runOnce(context.Background())

		if err != nil {
			t.Fatalf("poll %d: %v", poll+1, err)
		}
		if len(updates) != 1 || updates[0].Ingested != wantIngested {
			t.Fatalf("poll %d updated %+v, want one update that ingested: %v", poll+1, updates, wantIngested)
		}
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

// fakeListings checks listings as checks says, by ID, and lists toCheck for the poll, recording the IDs it checks.
type fakeListings struct {
	toCheck []int64
	checks  map[int64]app.ListingCheck
	failing map[int64]bool
	checked []int64
}

func (f *fakeListings) CheckListing(_ context.Context, _ []domain.LibraryKey, id int64) (app.ListingCheck, error) {
	f.checked = append(f.checked, id)
	if f.failing[id] {
		return app.ListingCheck{}, errors.New("the database is unreachable")
	}
	return f.checks[id], nil
}

func (f *fakeListings) ListingsToCheck(context.Context, []domain.LibraryKey) ([]int64, error) {
	return f.toCheck, nil
}

func TestHandleQueuesEachListingToCheckAfterTheVettedLibraries(t *testing.T) {
	queue := &memoryQueue{}
	w := newTestWorker(&fakeUpdater{}, queue)
	w.listings = &fakeListings{toCheck: []int64{7, 9}}

	out, err := w.handle(context.Background(), json.RawMessage(`{"source":"schedule"}`))

	if err != nil {
		t.Fatal(err)
	}
	want := []string{`{"host":"github","repositoryID":"42"}`, `{"host":"github","repositoryID":"43"}`, `{"listing":7}`, `{"listing":9}`}
	if !slices.Equal(queue.bodies, want) {
		t.Fatalf("queued %q, want %q", queue.bodies, want)
	}
	if got, ok := out.(map[string]int); !ok || got["queued"] != 4 {
		t.Fatalf("answered %#v, want 4 queued", out)
	}
}

// A listing's check that the repository refused is the lister's to see, so the job succeeds; only a failure that isn't
// the repository's reaches SQS.
func TestHandleChecksEachListingAndReportsOnlyRulemartsFailures(t *testing.T) {
	w := newTestWorker(&fakeUpdater{}, nil)
	listed := &fakeListings{
		checks: map[int64]app.ListingCheck{
			7: {Outcome: app.ListingIngested, Update: app.Update{Ingested: true}},
			8: {Outcome: app.ListingRefused, Failure: "the repository has no release/<number> tags"},
			9: {Outcome: app.ListingSkipped},
		},
		failing: map[int64]bool{10: true},
	}
	w.listings = listed

	out, err := w.handle(context.Background(), sqsEvent(`{"listing":7}`, `{"listing":8}`, `{"listing":9}`, `{"listing":10}`))

	if err != nil {
		t.Fatal(err)
	}
	if got := failures(t, out); !slices.Equal(got, []string{"3"}) {
		t.Fatalf("reported %q as failed, want only message 3", got)
	}
	if !slices.Equal(listed.checked, []int64{7, 8, 9, 10}) {
		t.Fatalf("checked %v", listed.checked)
	}
}

// A worker that checks no listings refuses their jobs rather than dropping them.
func TestHandleRefusesAListingsJobWithoutListings(t *testing.T) {
	out, err := newTestWorker(&fakeUpdater{}, nil).handle(context.Background(), sqsEvent(`{"listing":7}`))

	if err != nil {
		t.Fatal(err)
	}
	if got := failures(t, out); !slices.Equal(got, []string{"0"}) {
		t.Fatalf("reported %q as failed, want the job", got)
	}
}

// The worker's path for a listing, from end to end: the poll queues the listing a visitor added, its job looks the
// repository up and ingests it as the worker's role, and the next poll finds it unchanged.
func TestPollIngestsAListedLibraryThenFindsItUnchanged(t *testing.T) {
	lib := gittest.NewLibrary(t)
	lib.Group("techs/go", "Go")
	lib.Rule("techs/go/return-errors", "Return errors", "Return errors instead of panicking.")
	lib.Release(1, `formatVersion: 1
release: 1
rules: {techs/go/return-errors: 1.0.0}
changes: {techs/go/return-errors: {change: new, summaries: [Add the rule.]}}
`)
	_, connString := databasetest.New(t)
	postgrestest.Exec(t, connString, `WITH lister AS (
		INSERT INTO accounts (github_user_id, github_login, avatar_url) VALUES (1, 'lister', '') RETURNING id
	) INSERT INTO listings (account_id, host, owner, name) SELECT id, 'github', 'example', 'rules' FROM lister`)
	ingester := app.Ingester{
		Repositories: repositories{lib.Repository(44)}, Fetch: git.Fetch, List: git.ListReleaseTags, Render: render.Renderer{},
		Store: postgres.New(databasetest.AsWorkerRole(t, connString)), Limits: domain.DefaultLimits,
	}
	w := &worker{updater: ingester, listings: ingester, vetted: nil, log: slog.New(slog.NewTextHandler(io.Discard, nil))}

	for poll, wantIngested := range []bool{true, false} {
		updates, err := w.runOnce(context.Background())

		if err != nil {
			t.Fatalf("poll %d: %v", poll+1, err)
		}
		if len(updates) != 1 || updates[0].Ingested != wantIngested {
			t.Fatalf("poll %d updated %+v, want one update that ingested: %v", poll+1, updates, wantIngested)
		}
	}
	var checked bool
	postgrestest.QueryRow(t, connString, `SELECT checked_at IS NOT NULL AND failure IS NULL AND host_repository_id = '44' FROM listings`, &checked)
	if !checked {
		t.Fatal("the listing doesn't record a successful check of repository 44")
	}
}
