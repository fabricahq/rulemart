package main

import (
	"context"
	"encoding/json"
	"testing"
)

func TestHandleQueuesAMessageForTheScheduledEvent(t *testing.T) {
	q := &fakeQueue{}

	if _, err := (&server{queue: q}).handle(context.Background(), json.RawMessage(`{"source":"schedule"}`)); err != nil {
		t.Fatal(err)
	}

	if sent := q.messages(t); len(sent) != 1 || sent[0].Source != "schedule" {
		t.Fatalf("queued %+v, want one scheduled message", sent)
	}
}

// Any invocation without an HTTP method used to count as the schedule and queue a message. Only the schedule's
// explicit event does now.
func TestHandleRejectsEventsItDoesNotRecognize(t *testing.T) {
	for name, event := range map[string]string{
		"empty object":      `{}`,
		"other source":      `{"source":"aws.events"}`,
		"unrelated payload": `{"Records":[]}`,
		"not a JSON object": `[]`,
	} {
		t.Run(name, func(t *testing.T) {
			q := &fakeQueue{}

			_, err := (&server{queue: q}).handle(context.Background(), json.RawMessage(event))

			if err == nil {
				t.Fatal("accepted an unrecognized event")
			}
			if sent := q.messages(t); len(sent) != 0 {
				t.Fatalf("queued %d messages for an unrecognized event, want none", len(sent))
			}
		})
	}
}
