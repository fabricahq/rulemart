// Package queue sends messages to the worker's SQS jobs queue, which the worker function and the web function share:
// the worker's poll queues each job, and the web function queues a new listing's check.
package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// SQS sends messages to an SQS queue.
type SQS struct {
	client *sqs.Client
	url    string
}

// New returns the queue at url, sending with the ambient AWS credentials.
func New(ctx context.Context, url string) (*SQS, error) {
	if url == "" {
		return nil, errors.New("set QUEUE_URL to the jobs queue's URL")
	}
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration to send to QUEUE_URL: %v", err)
	}
	return &SQS{client: sqs.NewFromConfig(cfg), url: url}, nil
}

// Send sends body as one message.
func (q *SQS) Send(ctx context.Context, body string) error {
	_, err := q.client.SendMessage(ctx, &sqs.SendMessageInput{QueueUrl: aws.String(q.url), MessageBody: aws.String(body)})
	return err
}
