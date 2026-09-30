package main

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func post(path string, form url.Values) events.LambdaFunctionURLRequest {
	req := events.LambdaFunctionURLRequest{
		RawPath: path,
		Headers: map[string]string{"content-type": "application/x-www-form-urlencoded"},
		Body:    form.Encode(),
	}
	req.RequestContext.HTTP.Method = http.MethodPost
	return req
}

// GET used to queue a message, so crawlers, prefetchers, and retried GETs created work. Only POST queues now.
func TestEnqueueQueuesOnlyForPost(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			q := &fakeQueue{}
			req := request(method, "/enqueue", map[string]string{"text": "hi"})

			resp := (&server{queue: q}).serve(context.Background(), req)

			if resp.StatusCode != http.StatusMethodNotAllowed || resp.Headers["allow"] != http.MethodPost {
				t.Fatalf("got %d with allow=%q, want 405 with allow=POST", resp.StatusCode, resp.Headers["allow"])
			}
			if sent := q.messages(t); len(sent) != 0 {
				t.Fatalf("%s queued %d messages, want none", method, len(sent))
			}
		})
	}
}

func TestEnqueueQueuesTheFormTextForPost(t *testing.T) {
	for name, encode := range map[string]func(events.LambdaFunctionURLRequest) events.LambdaFunctionURLRequest{
		"plain body": func(req events.LambdaFunctionURLRequest) events.LambdaFunctionURLRequest { return req },
		"base64 body": func(req events.LambdaFunctionURLRequest) events.LambdaFunctionURLRequest {
			req.Body, req.IsBase64Encoded = base64.StdEncoding.EncodeToString([]byte(req.Body)), true
			return req
		},
	} {
		t.Run(name, func(t *testing.T) {
			q := &fakeQueue{}

			resp := (&server{queue: q}).serve(context.Background(), encode(post("/enqueue", url.Values{"text": {"hi there"}})))

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("got %d: %s", resp.StatusCode, resp.Body)
			}
			if sent := q.messages(t); len(sent) != 1 || sent[0].Text != "hi there" || sent[0].Source != "web" {
				t.Fatalf("queued %+v, want one web message with the form's text", sent)
			}
		})
	}
}
