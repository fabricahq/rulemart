package main

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/fabricahq/rulemart/internal/database"
	"github.com/fabricahq/rulemart/internal/hello"
	"github.com/fabricahq/rulemart/internal/migrate"
	"github.com/fabricahq/rulemart/internal/testdb"
)

// newWorker returns a worker storing messages in a fresh, migrated database, and that database's store.
func newWorker(t *testing.T) (*worker, *hello.Store) {
	t.Helper()
	connString := testdb.New(t)
	if err := migrate.Up(context.Background(), connString); err != nil {
		t.Fatal(err)
	}
	version, err := migrate.RequiredVersion()
	if err != nil {
		t.Fatal(err)
	}
	db := database.New(testdb.Parameter(connString), "/test/database-url", version)
	t.Cleanup(db.Close)
	store := hello.NewStore(db)
	return &worker{messages: store}, store
}

func record(t *testing.T, id string, m hello.Message) events.SQSMessage {
	t.Helper()
	body, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return events.SQSMessage{MessageId: id, Body: string(body)}
}

func failedIDs(resp events.SQSEventResponse) []string {
	ids := []string{}
	for _, f := range resp.BatchItemFailures {
		ids = append(ids, f.ItemIdentifier)
	}
	return ids
}

func TestHandleStoresEachMessageOnceWhenSQSDeliversItAgain(t *testing.T) {
	ctx := context.Background()
	w, store := newWorker(t)
	sent := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	delivery := events.SQSEvent{Records: []events.SQSMessage{record(t, "msg-1", hello.Message{Text: "hi", Source: "web", SentAt: sent})}}

	for range 2 {
		resp, err := w.handle(ctx, delivery)
		if err != nil || len(resp.BatchItemFailures) != 0 {
			t.Fatalf("delivery failed: resp=%+v err=%v", resp, err)
		}
	}

	rows, err := store.Latest(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("stored %d rows for one message delivered twice, want 1", len(rows))
	}
	if r := rows[0]; r.MessageID != "msg-1" || r.Text != "hi" || r.Source != "web" || !r.SentAt.Equal(sent) {
		t.Fatalf("stored %+v, want the delivered message's ID, text, source, and sent time", r)
	}
}

func TestHandleReportsOnlyTheRecordsItCannotStore(t *testing.T) {
	ctx := context.Background()
	w, store := newWorker(t)
	sent := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	batch := events.SQSEvent{Records: []events.SQSMessage{
		{MessageId: "not-json", Body: "{"},
		record(t, "no-text", hello.Message{Source: "web", SentAt: sent}),
		record(t, "nul-text", hello.Message{Text: "hello\x00world", Source: "web", SentAt: sent}),
		record(t, "valid", hello.Message{Text: "after the bad ones", Source: "web", SentAt: sent}),
	}}

	resp, err := w.handle(ctx, batch)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := failedIDs(resp), []string{"not-json", "no-text", "nul-text"}; !slices.Equal(got, want) {
		t.Fatalf("reported failures %v, want exactly %v", got, want)
	}
	rows, err := store.Latest(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].MessageID != "valid" {
		t.Fatalf("stored %+v, want only the valid record after the failures", rows)
	}
}
