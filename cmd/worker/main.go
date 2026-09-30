// Command worker consumes the job queue and stores each message in Neon, once per SQS message ID. Invalid or failed
// messages are reported one by one, so SQS retries only those and moves them to the dead-letter queue after repeated
// failures.
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"

	"github.com/fabricahq/rulemart/internal/database"
	"github.com/fabricahq/rulemart/internal/hello"
	"github.com/fabricahq/rulemart/internal/migrate"
)

// The store connects on the first message, so a message that can't be stored fails and retries instead of the
// function.
var messages *hello.Store

func main() {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	schemaVersion, err := migrate.RequiredVersion()
	if err != nil {
		log.Fatal(err)
	}
	messages = hello.NewStore(database.New(ssm.NewFromConfig(cfg), os.Getenv("DATABASE_URL_PARAMETER"), schemaVersion))
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
			err = messages.Insert(ctx, rec.MessageId, m)
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
