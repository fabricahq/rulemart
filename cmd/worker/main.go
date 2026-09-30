// Command worker consumes the job queue and stores each message in Neon, once per SQS message ID. Invalid or failed
// messages are reported one by one, so SQS retries only those and moves them to the dead-letter queue after repeated
// failures.
package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fabricahq/rulemart/internal/hello"
)

var db *pgxpool.Pool

func main() {
	var err error
	if db, err = hello.OpenDB(context.Background()); err != nil {
		log.Fatal(err)
	}
	lambda.Start(handle)
}

func handle(ctx context.Context, ev events.SQSEvent) (events.SQSEventResponse, error) {
	var failed []events.SQSBatchItemFailure
	for _, rec := range ev.Records {
		var m hello.Message
		err := json.Unmarshal([]byte(rec.Body), &m)
		if err == nil {
			err = m.Validate()
		}
		if err == nil {
			err = hello.Insert(ctx, db, rec.MessageId, m)
		}
		if err != nil {
			log.Printf("message %s: %v", rec.MessageId, err)
			failed = append(failed, events.SQSBatchItemFailure{ItemIdentifier: rec.MessageId})
			continue
		}
		log.Printf("stored message %s from %s", rec.MessageId, m.Source)
	}
	return events.SQSEventResponse{BatchItemFailures: failed}, nil
}
