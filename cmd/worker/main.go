// Command worker keeps the catalog's vetted and listed libraries current with their release tags, and checks new
// listings.
//
// On Lambda, the EventBridge schedule invokes it with {"source": "schedule"}, and it sends one job for each library
// catalog/vetted.yaml lists, and one for each listing to check, to the jobs queue at QUEUE_URL. The web function
// queues a new listing's job too. The queue's SQS trigger then invokes it with each job. For a vetted library, it
// lists the library's release tags, and ingests the library when they aren't the ones the catalog stored. For a
// listing, it looks the repository up the first time, then does the same, and records how the check went for the
// listing's lister. It reports a job that fails as failed, so SQS retries it and then moves it to the dead-letter
// queue, whose alarm reports it; a listing that fails because of its repository isn't a failed job.
//
// It logs JSON lines: one per job, with its outcome and duration, one per SQS batch, and one per poll, as
// _internal/decisions.md describes.
//
// Run anywhere else, it polls once: it sends the jobs to a queue in memory, handles each of them as on Lambda, and
// exits, failing when a job failed.
//
// Set DATABASE_URL to a connection string, or DATABASE_URL_PARAMETER to the SSM parameter holding one, as on Lambda.
// GITHUB_TOKEN, or GITHUB_TOKEN_PARAMETER naming the SSM parameter that holds it, authenticates GitHub lookups when
// set. LOG_LEVEL and RULEMART_RELEASE configure its logs, as
// internal/platform/logging describes.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/fabricahq/rulemart/catalog"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/jobs"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/render"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/github"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres"
	"github.com/fabricahq/rulemart/internal/platform/database"
	"github.com/fabricahq/rulemart/internal/platform/database/migrate"
	"github.com/fabricahq/rulemart/internal/platform/logging"
	"github.com/fabricahq/rulemart/internal/platform/queue"
	"github.com/fabricahq/rulemart/internal/platform/secret"
)

func main() {
	start := time.Now()
	logger, err := logging.New(os.Stdout, os.Getenv)
	if err != nil {
		exit(logger, err)
	}
	// The log package then writes JSON lines through logger too.
	slog.SetDefault(logger)
	schemaVersion, err := migrate.RequiredVersion()
	if err != nil {
		exit(logger, err)
	}
	ctx := context.Background()
	if os.Getenv("AWS_LAMBDA_RUNTIME_API") != "" {
		jobsQueue, err := queue.New(ctx, os.Getenv("QUEUE_URL"))
		if err != nil {
			exit(logger, err)
		}
		w, err := newWorker(ctx, logger, jobsQueue, schemaVersion)
		if err != nil {
			exit(logger, err)
		}
		logging.Ready(logger, schemaVersion, time.Since(start))
		lambda.Start(w.handle)
		return
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	w, err := newWorker(ctx, logger, nil, schemaVersion)
	if err != nil {
		exit(logger, err)
	}
	logging.Ready(logger, schemaVersion, time.Since(start))
	if _, err := w.runOnce(ctx); err != nil {
		logger.Error("poll failed", "error", err.Error())
		os.Exit(1)
	}
}

// exit reports that the function couldn't start, and stops it.
func exit(logger *slog.Logger, err error) {
	logging.StartupFailed(logger, err)
	os.Exit(1)
}

// updater updates one library, as app.Ingester does.
type updater interface {
	Update(ctx context.Context, library domain.LibraryKey) (app.Update, error)
}

// listings checks listings, as app.Ingester does.
type listings interface {
	CheckListing(ctx context.Context, vetted []domain.LibraryKey, id int64) (app.ListingCheck, error)
	ListingsToCheck(ctx context.Context, vetted []domain.LibraryKey) ([]int64, error)
}

// sender sends one message to the jobs queue.
type sender interface {
	Send(ctx context.Context, body string) error
}

// worker answers the worker function's invocations.
type worker struct {
	updater updater
	// listings checks listings; nil leaves them out of polls, and refuses their jobs.
	listings listings
	// vetted are the libraries the schedule queues jobs for, and the only ones a library's job may name.
	vetted []domain.LibraryKey
	// queue receives the schedule's jobs; runOnce replaces it with a queue in memory.
	queue sender
	log   *slog.Logger
}

// newWorker returns a worker that updates the vetted libraries and checks listings in the database the environment
// names, which must be at schemaVersion, and sends jobs to queue. It connects on the first poll or job.
func newWorker(ctx context.Context, logger *slog.Logger, queue sender, schemaVersion int64) (*worker, error) {
	vetted, err := catalog.Vetted()
	if err != nil {
		return nil, err
	}
	source, err := database.SourceFromEnv(ctx, os.Getenv)
	if err != nil {
		return nil, err
	}
	token, err := secret.FromEnv(ctx, os.Getenv, "GITHUB_TOKEN")
	if err != nil {
		return nil, err
	}
	ingester := app.Ingester{
		Repositories: github.Client{Client: &http.Client{Timeout: 30 * time.Second}, BaseURL: "https://api.github.com", Token: token},
		Fetch:        git.Fetch,
		List:         git.ListReleaseTags,
		Renderer:     render.Renderer{},
		Store:        postgres.New(source.Open(schemaVersion)),
		Limits:       domain.DefaultLimits,
	}
	return &worker{updater: ingester, listings: ingester, vetted: vetted, queue: queue, log: logger}, nil
}

// scheduleSource is the source field of the event the EventBridge schedule sends.
const scheduleSource = "schedule"

// invocation holds the fields handle uses to tell the worker's two kinds of events apart.
type invocation struct {
	// Source is scheduleSource in the schedule's event, and absent from SQS events.
	Source  string   `json:"source"`
	Records []record `json:"Records"`
}

// record is the field of an event's record that names the service that sent it.
type record struct {
	EventSource string `json:"eventSource"`
}

// fromSQS reports whether the event is a batch of SQS messages.
func (e invocation) fromSQS() bool {
	return len(e.Records) > 0 && !slices.ContainsFunc(e.Records, func(r record) bool { return r.EventSource != "aws:sqs" })
}

// handle queues a job for each vetted library on the schedule's event, runs the jobs in an SQS event, and rejects
// anything else.
func (w *worker) handle(ctx context.Context, raw json.RawMessage) (any, error) {
	var event invocation
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil, fmt.Errorf("decode invocation event: %v", err)
	}
	switch {
	case event.Source == scheduleSource:
		queued, err := w.poll(ctx)
		return map[string]int{"queued": queued}, err
	case event.fromSQS():
		var batch events.SQSEvent
		if err := json.Unmarshal(raw, &batch); err != nil {
			return nil, fmt.Errorf("decode SQS event: %v", err)
		}
		return w.consume(ctx, batch), nil
	default:
		return nil, errors.New("unrecognized invocation event: neither the schedule's event nor an SQS event")
	}
}

// poll sends one job for each vetted library and each listing to check to the queue, logs what it queued, and returns
// how many it queued. It tries every job, and fails when any send failed, or it couldn't read the listings.
func (w *worker) poll(ctx context.Context) (int, error) {
	started := time.Now()
	var failed []error
	bodies := make([]string, 0, len(w.vetted))
	for _, library := range w.vetted {
		bodies = append(bodies, jobs.Update(library))
	}
	var listed []int64
	if w.listings != nil {
		var err error
		if listed, err = w.listings.ListingsToCheck(ctx, w.vetted); err != nil {
			failed = append(failed, err)
		}
	}
	for _, id := range listed {
		bodies = append(bodies, jobs.CheckListing(id))
	}
	queueFailures := 0
	for _, body := range bodies {
		if err := w.queue.Send(ctx, body); err != nil {
			queueFailures++
			failed = append(failed, fmt.Errorf("queue job %s: %v", body, err))
		}
	}
	w.log.InfoContext(ctx, "poll queued", "libraries", len(w.vetted), "listings", len(listed),
		"queued", len(bodies)-queueFailures, "queue_failures", queueFailures, "duration_ms", milliseconds(time.Since(started)))
	return len(bodies) - queueFailures, errors.Join(failed...)
}

// jobMargin is how long before the function's deadline a job stops, so its transaction rolls back and its failure is
// reported while the function still runs.
const jobMargin = 10 * time.Second

// consume runs each job in batch, and reports the ones that failed, so SQS retries them.
func (w *worker) consume(ctx context.Context, batch events.SQSEvent) events.SQSEventResponse {
	resp := events.SQSEventResponse{BatchItemFailures: []events.SQSBatchItemFailure{}}
	if deadline, ok := ctx.Deadline(); ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, deadline.Add(-jobMargin))
		defer cancel()
	}
	for _, done := range w.runBatch(ctx, batch.Records) {
		if done.err != nil {
			resp.BatchItemFailures = append(resp.BatchItemFailures, events.SQSBatchItemFailure{ItemIdentifier: done.messageID})
		}
	}
	return resp
}

// Outcomes of a job, as the worker logs them under the outcome key.
const (
	outcomeUnchanged = "unchanged"
	outcomeIngested  = "ingested"
	outcomeFailed    = "failed"
	// outcomeRefused is a listing's check that failed because of its repository, which it recorded for its lister.
	outcomeRefused = "refused"
	// outcomeSkipped is a listing's check that had nothing to do: the listing is gone, or its library is vetted.
	outcomeSkipped = "skipped"
)

// jobResult is what one job of a batch did.
type jobResult struct {
	messageID string
	update    app.Update
	// listing is what a listing's job did, or nil for a library's job.
	listing *app.ListingCheck
	// err is why the job failed; nil when it succeeded.
	err error
}

// runBatch runs each job in messages, in order, and logs a line for each job and one for the batch.
func (w *worker) runBatch(ctx context.Context, messages []events.SQSMessage) []jobResult {
	started := time.Now()
	results := make([]jobResult, 0, len(messages))
	counts := map[string]int{}
	for _, message := range messages {
		result := w.runJob(ctx, message)
		results = append(results, result)
		counts[outcome(result)]++
	}
	w.log.InfoContext(ctx, "batch processed", "jobs", len(messages), outcomeUnchanged, counts[outcomeUnchanged],
		outcomeIngested, counts[outcomeIngested], outcomeFailed, counts[outcomeFailed], outcomeRefused,
		counts[outcomeRefused], outcomeSkipped, counts[outcomeSkipped], "duration_ms", milliseconds(time.Since(started)))
	return results
}

// runJob runs the job message holds: it updates a vetted library, or checks a listing, and logs the job's outcome and
// how long it took. Every job line carries outcome, message_id, and duration_ms, host and repository once the job
// names a library, and listing for a listing's job.
func (w *worker) runJob(ctx context.Context, message events.SQSMessage) jobResult {
	started := time.Now()
	result := jobResult{messageID: message.MessageId}
	job, err := w.parseJob(message.Body)
	library := job.Library
	switch {
	case err != nil:
	case job.Listing != 0:
		var check app.ListingCheck
		check, err = w.listings.CheckListing(ctx, w.vetted, job.Listing)
		result.listing, result.update, library = &check, check.Update, check.Listing.Library
	default:
		result.update, err = w.updater.Update(ctx, library)
	}
	result.err = err
	attrs := []any{"outcome", outcome(result), "message_id", message.MessageId}
	if job.Listing != 0 {
		attrs = append(attrs, "listing", job.Listing)
	}
	if library.RepositoryID != "" {
		attrs = append(attrs, "host", library.Host, "repository", library.RepositoryID)
	}
	update := result.update
	switch outcome(result) {
	case outcomeFailed:
		attrs = append(attrs, "error", err.Error(), "duration_ms", milliseconds(time.Since(started)))
		w.log.ErrorContext(ctx, "job failed", attrs...)
	case outcomeRefused:
		attrs = append(attrs, "failure", result.listing.Failure, "duration_ms", milliseconds(time.Since(started)))
		w.log.InfoContext(ctx, "listing refused", attrs...)
	case outcomeSkipped:
		attrs = append(attrs, "duration_ms", milliseconds(time.Since(started)))
		w.log.InfoContext(ctx, "listing skipped", attrs...)
	case outcomeIngested:
		attrs = append(attrs, "full_name", update.Result.Repository.FullName(), "releases", update.Result.Releases,
			"rules", update.Result.Rules, "rows_changed", update.Result.Changed, "list_ms", milliseconds(update.ListTime),
			"ingest_ms", milliseconds(update.IngestTime), "duration_ms", milliseconds(time.Since(started)))
		w.log.InfoContext(ctx, "ingested library", attrs...)
	default:
		attrs = append(attrs, "list_ms", milliseconds(update.ListTime), "duration_ms", milliseconds(time.Since(started)))
		w.log.InfoContext(ctx, "library unchanged", attrs...)
	}
	return result
}

// outcome names what a job did.
func outcome(result jobResult) string {
	switch {
	case result.err != nil:
		return outcomeFailed
	case result.listing != nil && result.listing.Outcome == app.ListingRefused:
		return outcomeRefused
	case result.listing != nil && result.listing.Outcome == app.ListingSkipped:
		return outcomeSkipped
	case result.update.Ingested:
		return outcomeIngested
	}
	return outcomeUnchanged
}

// milliseconds returns d in whole milliseconds, as the log's *_ms fields hold it.
func milliseconds(d time.Duration) int64 { return d.Milliseconds() }

// parseJob returns the job a message's body holds, as jobs.Parse reads it. It refuses a library w doesn't vet, and a
// listing when w checks none.
func (w *worker) parseJob(body string) (jobs.Job, error) {
	job, err := jobs.Parse(body)
	switch {
	case err != nil:
		return jobs.Job{}, err
	case job.Listing != 0 && w.listings == nil:
		return jobs.Job{}, fmt.Errorf("job for listing=%d: this worker checks no listings", job.Listing)
	case job.Listing == 0 && !slices.Contains(w.vetted, job.Library):
		return jobs.Job{}, fmt.Errorf("job for host=%q repository=%q: the library isn't vetted in this release", job.Library.Host, job.Library.RepositoryID)
	}
	return job, nil
}

// runOnce polls once without SQS: it sends the schedule's jobs to a queue in memory, then runs them as one batch, as
// consume does, and returns what each update did, a listing's included. It fails when any job failed.
func (w *worker) runOnce(ctx context.Context) ([]app.Update, error) {
	queue := &memoryQueue{}
	w.queue = queue
	if _, err := w.handle(ctx, json.RawMessage(`{"source":"`+scheduleSource+`"}`)); err != nil {
		return nil, err
	}
	messages := make([]events.SQSMessage, len(queue.bodies))
	for i, body := range queue.bodies {
		messages[i] = events.SQSMessage{MessageId: "local-" + strconv.Itoa(i), Body: body, EventSource: "aws:sqs"}
	}
	var updates []app.Update
	var failed []error
	for _, result := range w.runBatch(ctx, messages) {
		if result.err != nil {
			failed = append(failed, result.err)
			continue
		}
		updates = append(updates, result.update)
	}
	return updates, errors.Join(failed...)
}

// memoryQueue keeps the bodies sent to it, in order.
type memoryQueue struct {
	bodies []string
}

func (q *memoryQueue) Send(_ context.Context, body string) error {
	q.bodies = append(q.bodies, body)
	return nil
}
