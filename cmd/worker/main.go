// Command worker consumes the job queue. No job is defined yet: the library-release poller that will queue
// ingestion jobs arrives in the next slice. Until then it reports every message as failed, so SQS retries it and
// then moves it to the dead-letter queue, where an unexpected message stays visible instead of being dropped.
//
// LOG_LEVEL and RULEMART_RELEASE configure its logs, as internal/platform/logging describes.
package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/fabricahq/rulemart/internal/platform/database/migrate"
	"github.com/fabricahq/rulemart/internal/platform/logging"
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
	logging.Ready(logger, schemaVersion, time.Since(start))
	lambda.Start(handle)
}

// exit reports that the function couldn't start, and stops it.
func exit(logger *slog.Logger, err error) {
	logging.StartupFailed(logger, err)
	os.Exit(1)
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
