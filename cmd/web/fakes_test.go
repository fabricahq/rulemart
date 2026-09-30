package main

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/fabricahq/rulemart/internal/hello"
)

// fakeQueue stands in for SQS, recording each message sent, or failing every send with err.
type fakeQueue struct {
	mu   sync.Mutex
	sent []hello.Message
	err  error
}

func (q *fakeQueue) SendMessage(_ context.Context, in *sqs.SendMessageInput, _ ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	if q.err != nil {
		return nil, q.err
	}
	var m hello.Message
	if err := json.Unmarshal([]byte(aws.ToString(in.MessageBody)), &m); err != nil {
		return nil, err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.sent = append(q.sent, m)
	return &sqs.SendMessageOutput{MessageId: aws.String("queued-message-id")}, nil
}

func (q *fakeQueue) messages(t *testing.T) []hello.Message {
	t.Helper()
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]hello.Message(nil), q.sent...)
}
