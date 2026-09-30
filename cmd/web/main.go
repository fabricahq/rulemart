// Command web is the walking skeleton's front door. CloudFront invokes it through its Function URL, and the
// EventBridge schedule invokes it directly with {"source": "schedule"}, standing in for the library-release poller.
// Either way it queues a message for the worker; over HTTP it also reads back what the worker stored in Neon.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"

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
	// log receives the details of failures that responses leave out.
	log *slog.Logger
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
		log:      slog.New(slog.NewJSONHandler(os.Stdout, nil)),
	}
	lambda.Start(s.handle)
}

// scheduleSource is the source field of the event the EventBridge schedule sends.
const scheduleSource = "schedule"

// invocation holds the fields handle uses to tell the web function's two kinds of events apart.
type invocation struct {
	// Source is scheduleSource in the schedule's event, and absent from Function URL requests.
	Source string `json:"source"`
	events.LambdaFunctionURLRequest
}

// handle serves Function URL requests, queues a message for the schedule's event, and rejects anything else.
func (s *server) handle(ctx context.Context, raw json.RawMessage) (any, error) {
	var event invocation
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil, fmt.Errorf("decode invocation event: %v", err)
	}
	switch {
	case event.Source == scheduleSource:
		return s.poll(ctx)
	case event.RequestContext.HTTP.Method != "":
		return s.serve(ctx, event.LambdaFunctionURLRequest), nil
	default:
		return nil, errors.New("unrecognized invocation event: neither the schedule's event nor a Function URL request")
	}
}

// poll stands in for the library-release poller: it queues one message for the worker.
func (s *server) poll(ctx context.Context) (any, error) {
	id, err := s.enqueue(ctx, hello.Message{Text: "Hello from the schedule", Source: "schedule", SentAt: time.Now().UTC()})
	if err != nil {
		return nil, err
	}
	return map[string]string{"queued": id}, nil
}

func (s *server) serve(ctx context.Context, req events.LambdaFunctionURLRequest) events.LambdaFunctionURLResponse {
	switch req.RawPath {
	case "/":
		return text(http.StatusOK, "no-store", "Rulemart walking skeleton\n\n"+
			"POST /enqueue   queue a message for the worker; send its text as the form field text\n"+
			"GET /messages   read what the worker stored in Neon\n"+
			"GET /cached     cached by CloudFront for 60 seconds\n")
	case "/enqueue":
		// Queueing is a side effect, so only POST reaches it. GET stays safe for crawlers, prefetchers, and retries.
		if req.RequestContext.HTTP.Method != http.MethodPost {
			resp := text(http.StatusMethodNotAllowed, "no-store", "Use POST\n")
			resp.Headers["allow"] = http.MethodPost
			return resp
		}
		form, err := formValues(req)
		if err != nil {
			return text(http.StatusBadRequest, "no-store", "Send the text as a URL-encoded form field\n")
		}
		m := hello.Message{Text: form.Get("text"), Source: "web", SentAt: time.Now().UTC()}
		if m.Text == "" {
			m.Text = "Hello from the web"
		}
		// Refuse what the worker couldn't store, rather than queueing a message that fails on every delivery.
		if err := m.Validate(); err != nil {
			return text(http.StatusBadRequest, "no-store", err.Error()+"\n")
		}
		id, err := s.enqueue(ctx, m)
		if err != nil {
			return s.fail(ctx, req, http.StatusBadGateway, "The message couldn't be queued. Try again later.", err)
		}
		return jsonBody(http.StatusOK, "no-store", map[string]string{"queued": id, "text": m.Text})
	case "/messages":
		rows, err := s.messages.Latest(ctx, 20)
		if err != nil {
			return s.fail(ctx, req, http.StatusServiceUnavailable, "Messages are unavailable right now. Try again later.", err)
		}
		return jsonBody(http.StatusOK, "no-store", rows)
	case "/cached":
		// CloudFront keeps this for 60 seconds, so repeated requests show the same time and an x-cache: Hit header.
		return text(http.StatusOK, "public, max-age=60", fmt.Sprintf("Rendered at %s\n", time.Now().UTC().Format(time.RFC3339)))
	default:
		return text(http.StatusNotFound, "no-store", "Not found\n")
	}
}

// fail logs err with the request's route and ID, and answers with status and a public message that reveals nothing
// about the failure.
func (s *server) fail(ctx context.Context, req events.LambdaFunctionURLRequest, status int, public string, err error) events.LambdaFunctionURLResponse {
	s.log.ErrorContext(ctx, "request failed", "route", req.RawPath, "method", req.RequestContext.HTTP.Method,
		"requestID", req.RequestContext.RequestID, "status", status, "error", err.Error())
	return text(status, "no-store", public+"\n")
}

// enqueue sends m to the jobs queue and returns its SQS message ID.
func (s *server) enqueue(ctx context.Context, m hello.Message) (string, error) {
	body, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	out, err := s.queue.SendMessage(ctx, &sqs.SendMessageInput{QueueUrl: aws.String(s.queueURL), MessageBody: aws.String(string(body))})
	if err != nil {
		return "", fmt.Errorf("send message source=%q to queue: %v", m.Source, err)
	}
	return aws.ToString(out.MessageId), nil
}

// formValues parses a request's URL-encoded form body, which the Function URL may deliver base64-encoded.
func formValues(req events.LambdaFunctionURLRequest) (url.Values, error) {
	body := req.Body
	if req.IsBase64Encoded {
		decoded, err := base64.StdEncoding.DecodeString(body)
		if err != nil {
			return nil, err
		}
		body = string(decoded)
	}
	return url.ParseQuery(body)
}

func text(status int, cache, body string) events.LambdaFunctionURLResponse {
	return events.LambdaFunctionURLResponse{StatusCode: status, Body: body, Headers: map[string]string{"content-type": "text/plain; charset=utf-8", "cache-control": cache}}
}

func jsonBody(status int, cache string, v any) events.LambdaFunctionURLResponse {
	b, _ := json.MarshalIndent(v, "", "  ")
	return events.LambdaFunctionURLResponse{StatusCode: status, Body: string(b) + "\n", Headers: map[string]string{"content-type": "application/json", "cache-control": cache}}
}
