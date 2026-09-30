// Command web is the walking skeleton's front door. CloudFront invokes it through its Function URL, and the
// EventBridge schedule invokes it directly, standing in for the library-release poller. Either way it queues a
// message for the worker; over HTTP it also reads back what the worker stored in Neon.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/ssm"

	"github.com/fabricahq/rulemart/internal/database"
	"github.com/fabricahq/rulemart/internal/hello"
	"github.com/fabricahq/rulemart/internal/migrate"
)

// queue is the part of the SQS client the web function uses.
type queue interface {
	SendMessage(context.Context, *sqs.SendMessageInput, ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
}

// server answers the web function's invocations.
type server struct {
	queue queue
	// queueURL identifies the jobs queue the worker consumes.
	queueURL string
	// messages connects to Neon on first use, so routes other than /messages work before Neon is configured.
	messages *hello.Store
}

func main() {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	schemaVersion, err := migrate.RequiredVersion()
	if err != nil {
		log.Fatal(err)
	}
	s := &server{
		queue:    sqs.NewFromConfig(cfg),
		queueURL: os.Getenv("QUEUE_URL"),
		messages: hello.NewStore(database.New(ssm.NewFromConfig(cfg), os.Getenv("DATABASE_URL_PARAMETER"), schemaVersion)),
	}
	lambda.Start(s.handle)
}

// handle tells a Function URL request from a scheduled invocation by the request's HTTP method.
func (s *server) handle(ctx context.Context, raw json.RawMessage) (any, error) {
	var req events.LambdaFunctionURLRequest
	if err := json.Unmarshal(raw, &req); err == nil && req.RequestContext.HTTP.Method != "" {
		return s.serve(ctx, req), nil
	}
	id, err := s.enqueue(ctx, "Hello from the schedule", "schedule")
	if err != nil {
		return nil, err
	}
	return map[string]string{"queued": id}, nil
}

func (s *server) serve(ctx context.Context, req events.LambdaFunctionURLRequest) events.LambdaFunctionURLResponse {
	switch req.RawPath {
	case "/":
		return text(http.StatusOK, "no-store", "Rulemart walking skeleton\n\n"+
			"GET /enqueue?text=hi  queue a message for the worker\n"+
			"GET /messages         read what the worker stored in Neon\n"+
			"GET /cached           cached by CloudFront for 60 seconds\n")
	case "/enqueue":
		// Queueing is a side effect, so HEAD probes and other methods don't reach it.
		if req.RequestContext.HTTP.Method != http.MethodGet {
			resp := text(http.StatusMethodNotAllowed, "no-store", "Use GET\n")
			resp.Headers["allow"] = http.MethodGet
			return resp
		}
		msg := req.QueryStringParameters["text"]
		if msg == "" {
			msg = "Hello from the web"
		}
		if utf8.RuneCountInString(msg) > hello.MaxTextLength {
			return text(http.StatusBadRequest, "no-store", fmt.Sprintf("text must be at most %d characters\n", hello.MaxTextLength))
		}
		id, err := s.enqueue(ctx, msg, "web")
		if err != nil {
			return text(http.StatusBadGateway, "no-store", err.Error())
		}
		return jsonBody(http.StatusOK, "no-store", map[string]string{"queued": id, "text": msg})
	case "/messages":
		rows, err := s.messages.Latest(ctx, 20)
		if err != nil {
			return text(http.StatusBadGateway, "no-store", err.Error())
		}
		return jsonBody(http.StatusOK, "no-store", rows)
	case "/cached":
		// CloudFront keeps this for 60 seconds, so repeated requests show the same time and an x-cache: Hit header.
		return text(http.StatusOK, "public, max-age=60", fmt.Sprintf("Rendered at %s\n", time.Now().UTC().Format(time.RFC3339)))
	default:
		return text(http.StatusNotFound, "no-store", "Not found\n")
	}
}

func (s *server) enqueue(ctx context.Context, msg, source string) (string, error) {
	body, err := json.Marshal(hello.Message{Text: msg, Source: source, SentAt: time.Now().UTC()})
	if err != nil {
		return "", err
	}
	out, err := s.queue.SendMessage(ctx, &sqs.SendMessageInput{QueueUrl: aws.String(s.queueURL), MessageBody: aws.String(string(body))})
	if err != nil {
		return "", fmt.Errorf("send to queue: %w", err)
	}
	return aws.ToString(out.MessageId), nil
}

func text(status int, cache, body string) events.LambdaFunctionURLResponse {
	return events.LambdaFunctionURLResponse{StatusCode: status, Body: body, Headers: map[string]string{"content-type": "text/plain; charset=utf-8", "cache-control": cache}}
}

func jsonBody(status int, cache string, v any) events.LambdaFunctionURLResponse {
	b, _ := json.MarshalIndent(v, "", "  ")
	return events.LambdaFunctionURLResponse{StatusCode: status, Body: string(b) + "\n", Headers: map[string]string{"content-type": "application/json", "cache-control": cache}}
}
