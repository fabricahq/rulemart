// Command worker keeps the catalog's vetted libraries current with their release tags.
//
// On Lambda, the EventBridge schedule invokes it with {"source": "schedule"}, and it sends one job for each library
// catalog/vetted.yaml lists to the jobs queue at QUEUE_URL. The queue's SQS trigger then invokes it with each job,
// and it updates that library: it lists the library's release tags, and ingests the library when they aren't the
// ones the catalog stored. It reports a job that fails as failed, so SQS retries it and then moves it to the
// dead-letter queue, whose alarm reports it.
//
// Run anywhere else, it polls once: it sends the jobs to a queue in memory, handles each of them as on Lambda, and
// exits, failing when a job failed.
//
// Set DATABASE_URL to a connection string, or DATABASE_URL_PARAMETER to the SSM parameter holding one, as on Lambda.
// GITHUB_TOKEN, when set, authenticates GitHub lookups.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"slices"
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
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx := context.Background()
	if os.Getenv("AWS_LAMBDA_RUNTIME_API") != "" {
		queue, err := newSQSQueue(ctx, os.Getenv("QUEUE_URL"))
		if err != nil {
			log.Fatal(err)
		}
		w, err := newWorker(ctx, logger, queue)
		if err != nil {
			log.Fatal(err)
		}
		lambda.Start(w.handle)
		return
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	w, err := newWorker(ctx, logger, nil)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := w.runOnce(ctx); err != nil {
		log.Fatal(err)
	}
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

// newWorker returns a worker that updates the vetted libraries in the database the environment names, and sends
// jobs to queue. It connects on the first job, so the schedule's invocations never touch the database.
func newWorker(ctx context.Context, logger *slog.Logger, queue sender) (*worker, error) {
	vetted, err := catalog.Vetted()
	if err != nil {
		return nil, err
	}
	source, err := database.SourceFromEnv(ctx, os.Getenv)
	if err != nil {
		return nil, err
	}
	schemaVersion, err := migrate.RequiredVersion()
	if err != nil {
		return nil, err
	}
	ingester := app.Ingester{
		Repositories: github.Client{Client: &http.Client{Timeout: 30 * time.Second}, BaseURL: "https://api.github.com", Token: os.Getenv("GITHUB_TOKEN")},
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

// poll sends one job for each vetted library to the queue. It tries every library, and fails when any send failed.
func (w *worker) poll(ctx context.Context) error {
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
	for _, message := range batch.Records {
		if _, err := w.process(ctx, message.Body); err != nil {
			w.log.ErrorContext(ctx, "job failed", "message", message.MessageId, "error", err.Error())
			resp.BatchItemFailures = append(resp.BatchItemFailures, events.SQSBatchItemFailure{ItemIdentifier: message.MessageId})
		}
	}
	return resp
}

// process runs one job: it updates the vetted library body names, and logs what the update did.
func (w *worker) process(ctx context.Context, body string) (app.Update, error) {
	library, err := w.parseJob(body)
	if err != nil {
		return app.Update{}, err
	}
	update, err := w.updater.Update(ctx, library)
	if err != nil {
		return app.Update{}, err
	}
	if !update.Ingested {
		w.log.InfoContext(ctx, "library unchanged", "host", library.Host, "repository", library.RepositoryID)
		return update, nil
	}
	result := update.Result
	w.log.InfoContext(ctx, "ingested library", "host", library.Host, "repository", library.RepositoryID,
		"name", result.Repository.FullName(), "releases", result.Releases, "rules", result.Rules, "changed", result.Changed)
	return update, nil
}

// parseJob returns the library a job's body names. It refuses a body that isn't exactly one job with known fields, or
// that names a library w doesn't vet.
func (w *worker) parseJob(body string) (domain.LibraryKey, error) {
	decoder := json.NewDecoder(bytes.NewReader([]byte(body)))
	decoder.DisallowUnknownFields()
	var j job
	if err := decoder.Decode(&j); err != nil {
		return domain.LibraryKey{}, fmt.Errorf("decode job: %v", err)
	}
	if decoder.More() {
		return domain.LibraryKey{}, errors.New("decode job: expected one JSON object")
	}
	library := domain.LibraryKey{Host: j.Host, RepositoryID: j.RepositoryID}
	if !slices.Contains(w.vetted, library) {
		return domain.LibraryKey{}, fmt.Errorf("job for host=%q repository=%q: the library isn't vetted in this release", j.Host, j.RepositoryID)
	}
	return library, nil
}

// runOnce polls once without SQS: it sends the schedule's jobs to a queue in memory, then runs each of them as consume
// does, and returns what each update did. It fails when any job failed.
func (w *worker) runOnce(ctx context.Context) ([]app.Update, error) {
	queue := &memoryQueue{}
	w.queue = queue
	if _, err := w.handle(ctx, json.RawMessage(`{"source":"`+scheduleSource+`"}`)); err != nil {
		return nil, err
	}
	var updates []app.Update
	var failed []error
	for _, body := range queue.bodies {
		update, err := w.process(ctx, body)
		if err != nil {
			failed = append(failed, err)
			continue
		}
		updates = append(updates, update)
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
