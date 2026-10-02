// Command worker keeps the catalog's vetted libraries current with their release tags.
//
// On Lambda, the EventBridge schedule invokes it with {"source": "schedule"}, and it sends one job for each library
// catalog/vetted.yaml lists to the jobs queue at QUEUE_URL. The queue's SQS trigger then invokes it with each job,
// and it updates that library: it lists the library's release tags, and ingests the library when they aren't the
// ones the catalog stored. It reports a job that fails as failed, so SQS retries it and then moves it to the
// dead-letter queue, whose alarm reports it.
//
// It logs JSON lines: one per job, with its outcome and duration, one per SQS batch, and one per poll, as
// _internal/slices/2-automatic-updates.md describes.
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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/fabricahq/rulemart/catalog"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/render"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/github"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres"
	"github.com/fabricahq/rulemart/internal/platform/database"
	"github.com/fabricahq/rulemart/internal/platform/database/migrate"
	"github.com/fabricahq/rulemart/internal/platform/logging"
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
		queue, err := newSQSQueue(ctx, os.Getenv("QUEUE_URL"))
		if err != nil {
			exit(logger, err)
		}
		w, err := newWorker(ctx, logger, queue, schemaVersion)
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

// sender sends one message to the jobs queue.
type sender interface {
	Send(ctx context.Context, body string) error
}

// worker answers the worker function's invocations.
type worker struct {
	updater updater
	// vetted are the libraries the schedule queues jobs for, and the only ones a job may name.
	vetted []domain.LibraryKey
	// queue receives the schedule's jobs; runOnce replaces it with a queue in memory.
	queue sender
	log   *slog.Logger
}

// newWorker returns a worker that updates the vetted libraries in the database the environment names, which must be
// at schemaVersion, and sends jobs to queue. It connects on the first job, so the schedule's invocations never touch
// the database.
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
		Render:       render.Rule,
		Store:        postgres.New(source.Open(schemaVersion)),
		Limits:       domain.DefaultLimits,
	}
	return &worker{updater: ingester, vetted: vetted, queue: queue, log: logger}, nil
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
		return map[string]int{"queued": len(w.vetted)}, w.poll(ctx)
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

// job asks the worker to update one vetted library. It names the library only by its key, so a queued message
// can't point the worker at a repository nobody vetted.
type job struct {
	Host         string `json:"host"`
	RepositoryID string `json:"repositoryID"`
}

// poll sends one job for each vetted library to the queue, and logs what it queued. It tries every library, and
// fails when any send failed.
func (w *worker) poll(ctx context.Context) error {
	started := time.Now()
	var failed []error
	for _, library := range w.vetted {
		body, err := json.Marshal(job{Host: library.Host, RepositoryID: library.RepositoryID})
		if err == nil {
			err = w.queue.Send(ctx, string(body))
		}
		if err != nil {
			failed = append(failed, fmt.Errorf("queue update host=%s repository=%s: %v", library.Host, library.RepositoryID, err))
		}
	}
	w.log.InfoContext(ctx, "poll queued", "libraries", len(w.vetted), "queued", len(w.vetted)-len(failed),
		"queue_failures", len(failed), "duration_ms", milliseconds(time.Since(started)))
	return errors.Join(failed...)
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
)

// jobResult is what one job of a batch did.
type jobResult struct {
	messageID string
	update    app.Update
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
		outcomeIngested, counts[outcomeIngested], outcomeFailed, counts[outcomeFailed],
		"duration_ms", milliseconds(time.Since(started)))
	return results
}

// runJob updates the vetted library message names, and logs the job's outcome and how long it took. Every job line
// carries outcome, message_id, and duration_ms, and host and repository once the job names a library.
func (w *worker) runJob(ctx context.Context, message events.SQSMessage) jobResult {
	started := time.Now()
	result := jobResult{messageID: message.MessageId}
	library, err := w.parseJob(message.Body)
	if err == nil {
		result.update, err = w.updater.Update(ctx, library)
	}
	result.err = err
	attrs := []any{"outcome", outcome(result), "message_id", message.MessageId}
	if library != (domain.LibraryKey{}) {
		attrs = append(attrs, "host", library.Host, "repository", library.RepositoryID)
	}
	update := result.update
	switch {
	case err != nil:
		attrs = append(attrs, "error", err.Error(), "duration_ms", milliseconds(time.Since(started)))
		w.log.ErrorContext(ctx, "job failed", attrs...)
	case update.Ingested:
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
	case result.update.Ingested:
		return outcomeIngested
	}
	return outcomeUnchanged
}

// milliseconds returns d in whole milliseconds, as the log's *_ms fields hold it.
func milliseconds(d time.Duration) int64 { return d.Milliseconds() }

// parseJob returns the library a job's body names. It refuses a body that isn't exactly one job with known fields, or
// that names a library w doesn't vet.
func (w *worker) parseJob(body string) (domain.LibraryKey, error) {
	decoder := json.NewDecoder(bytes.NewReader([]byte(body)))
	decoder.DisallowUnknownFields()
	var j job
	if err := decoder.Decode(&j); err != nil {
		return domain.LibraryKey{}, fmt.Errorf("decode job: %v", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return domain.LibraryKey{}, errors.New("decode job: expected one JSON object and nothing after it")
	}
	library := domain.LibraryKey{Host: j.Host, RepositoryID: j.RepositoryID}
	if !slices.Contains(w.vetted, library) {
		return domain.LibraryKey{}, fmt.Errorf("job for host=%q repository=%q: the library isn't vetted in this release", j.Host, j.RepositoryID)
	}
	return library, nil
}

// runOnce polls once without SQS: it sends the schedule's jobs to a queue in memory, then runs them as one batch, as
// consume does, and returns what each update did. It fails when any job failed.
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

// sqsQueue sends jobs to an SQS queue.
type sqsQueue struct {
	client *sqs.Client
	url    string
}

// newSQSQueue returns the queue at url, sending with the ambient AWS credentials.
func newSQSQueue(ctx context.Context, url string) (*sqsQueue, error) {
	if url == "" {
		return nil, errors.New("set QUEUE_URL to the jobs queue's URL")
	}
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration to send to QUEUE_URL: %v", err)
	}
	return &sqsQueue{client: sqs.NewFromConfig(cfg), url: url}, nil
}

func (q *sqsQueue) Send(ctx context.Context, body string) error {
	_, err := q.client.SendMessage(ctx, &sqs.SendMessageInput{QueueUrl: aws.String(q.url), MessageBody: aws.String(body)})
	return err
}
