package main

import (
	"context"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

// Until a job is defined, no message may be acknowledged and silently dropped.
func TestHandleLeavesEveryMessageForTheDeadLetterQueue(t *testing.T) {
	batch := events.SQSEvent{Records: []events.SQSMessage{{MessageId: "a", Body: "{}"}, {MessageId: "b", Body: "hello"}}}

	resp, err := handle(context.Background(), batch)

	if err != nil {
		t.Fatal(err)
	}
	if len(resp.BatchItemFailures) != 2 || resp.BatchItemFailures[0].ItemIdentifier != "a" || resp.BatchItemFailures[1].ItemIdentifier != "b" {
		t.Fatalf("reported %+v, want both messages as failures", resp.BatchItemFailures)
	}
}
