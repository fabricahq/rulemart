package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
)

// loggingWorker returns a worker for updater and queue that logs JSON to the returned buffer, as on Lambda.
func loggingWorker(updater updater, queue sender) (*worker, *bytes.Buffer) {
	var out bytes.Buffer
	w := newTestWorker(updater, queue)
	w.log = slog.New(slog.NewJSONHandler(&out, nil))
	return w, &out
}

// logLines returns each JSON line logged to out, by message.
func logLines(t *testing.T, out *bytes.Buffer) map[string][]map[string]any {
	t.Helper()
	lines := map[string][]map[string]any{}
	for _, raw := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var line map[string]any
		if err := json.Unmarshal([]byte(raw), &line); err != nil {
			t.Fatalf("logged %q, which isn't JSON: %v", raw, err)
		}
		message, _ := line["msg"].(string)
		lines[message] = append(lines[message], line)
	}
	return lines
}

// want fails t unless line has each field, with value when it's not nil.
func want(t *testing.T, line map[string]any, fields map[string]any) {
	t.Helper()
	for key, value := range fields {
		got, ok := line[key]
		switch {
		case !ok:
			t.Errorf("%s: no %s field in %v", line["msg"], key, line)
		case value != nil && got != value:
			t.Errorf("%s: %s is %v, want %v", line["msg"], key, got, value)
		}
	}
}

// timedUpdater reports first as ingested, with the time each part took, and second as unchanged.
type timedUpdater struct{}

func (timedUpdater) Update(_ context.Context, library domain.LibraryKey) (app.Update, error) {
	if library != first {
		return app.Update{ListTime: 300 * time.Millisecond}, nil
	}
	repo := domain.Repository{Host: domain.GitHub, ID: "42", Owner: "example", Name: "rules"}
	return app.Update{
		Ingested: true, ListTime: 250 * time.Millisecond, IngestTime: 1500 * time.Millisecond,
		Result: app.Result{Repository: repo, Releases: 3, Rules: 7, Changed: 12},
	}, nil
}

// Logs Insights groups job lines by outcome, library, and duration, so every job line carries them, under the same
// keys, and the batch line counts each outcome.
func TestHandleLogsEachJobAndTheBatch(t *testing.T) {
	w, out := loggingWorker(timedUpdater{}, nil)

	if _, err := w.handle(context.Background(), sqsEvent(
		`{"host":"github","repositoryID":"42"}`, `{"host":"github","repositoryID":"43"}`,
		`{"host":"github","repositoryID":"44"}`)); err != nil {
		t.Fatal(err)
	}

	lines := logLines(t, out)
	if len(lines["ingested library"]) != 1 || len(lines["library unchanged"]) != 1 || len(lines["job failed"]) != 1 ||
		len(lines["batch processed"]) != 1 {
		t.Fatalf("logged %v", lines)
	}
	want(t, lines["ingested library"][0], map[string]any{
		"outcome": "ingested", "host": "github", "repository": "42", "full_name": "example/rules",
		"releases": 3.0, "rules": 7.0, "rows_changed": 12.0, "list_ms": 250.0, "ingest_ms": 1500.0,
		"message_id": "0", "duration_ms": nil,
	})
	want(t, lines["library unchanged"][0], map[string]any{
		"outcome": "unchanged", "host": "github", "repository": "43", "message_id": "1", "duration_ms": nil,
	})
	want(t, lines["job failed"][0], map[string]any{"outcome": "failed", "message_id": "2", "error": nil, "duration_ms": nil})
	want(t, lines["batch processed"][0], map[string]any{
		"jobs": 3.0, "unchanged": 1.0, "ingested": 1.0, "failed": 1.0, "duration_ms": nil,
	})
}

// failingQueue refuses the job for second, and keeps the others.
type failingQueue struct{ memoryQueue }

func (q *failingQueue) Send(ctx context.Context, body string) error {
	if strings.Contains(body, `"43"`) {
		return errors.New("SQS is unavailable")
	}
	return q.memoryQueue.Send(ctx, body)
}

func TestHandleLogsWhatAPollQueued(t *testing.T) {
	w, out := loggingWorker(timedUpdater{}, &failingQueue{})

	_, err := w.handle(context.Background(), json.RawMessage(`{"source":"schedule"}`))

	if err == nil {
		t.Fatal("a poll that couldn't queue a job succeeded")
	}
	lines := logLines(t, out)["poll queued"]
	if len(lines) != 1 {
		t.Fatalf("logged %d poll lines, want 1", len(lines))
	}
	want(t, lines[0], map[string]any{"libraries": 2.0, "listings": 0.0, "queued": 1.0, "queue_failures": 1.0, "duration_ms": nil})
}

// A listing's job line names the listing, and once it's resolved, its library; a refused one says why, and the batch
// line counts refused and skipped jobs.
func TestHandleLogsEachListingsJob(t *testing.T) {
	w, out := loggingWorker(timedUpdater{}, nil)
	w.listings = &fakeListings{checks: map[int64]app.ListingCheck{
		7: {Outcome: app.ListingRefused, Failure: "the repository has no release/<number> tags",
			Listing: domain.Listing{ID: 7, Library: domain.LibraryKey{Host: domain.GitHub, RepositoryID: "50"}}},
		8: {Outcome: app.ListingSkipped},
	}}

	if _, err := w.handle(context.Background(), sqsEvent(`{"listing":7}`, `{"listing":8}`)); err != nil {
		t.Fatal(err)
	}

	lines := logLines(t, out)
	if len(lines["listing refused"]) != 1 || len(lines["listing skipped"]) != 1 {
		t.Fatalf("logged %v", lines)
	}
	want(t, lines["listing refused"][0], map[string]any{
		"outcome": "refused", "listing": 7.0, "host": "github", "repository": "50",
		"failure": "the repository has no release/<number> tags", "message_id": "0", "duration_ms": nil,
	})
	want(t, lines["listing skipped"][0], map[string]any{"outcome": "skipped", "listing": 8.0, "message_id": "1", "duration_ms": nil})
	want(t, lines["batch processed"][0], map[string]any{"jobs": 2.0, "refused": 1.0, "skipped": 1.0, "failed": 0.0})
}
