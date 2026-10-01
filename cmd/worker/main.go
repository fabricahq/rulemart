// Command worker consumes the job queue. No job is defined yet: the library-release poller that will queue
// ingestion jobs arrives in the next slice. Until then it reports every message as failed, so SQS retries it and
// then moves it to the dead-letter queue, where an unexpected message stays visible instead of being dropped.
package main

import (
	"context"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	lambda.Start(handle)
}

// handle reports every record as failed, since no job is defined yet.
func handle(_ context.Context, event events.SQSEvent) (events.SQSEventResponse, error) {
	failed := make([]events.SQSBatchItemFailure, 0, len(event.Records))
	for _, record := range event.Records {
		log.Printf("message %s: no job is defined yet, so the worker leaves it for the dead-letter queue", record.MessageId)
		failed = append(failed, events.SQSBatchItemFailure{ItemIdentifier: record.MessageId})
	}
	return events.SQSEventResponse{BatchItemFailures: failed}, nil
}
