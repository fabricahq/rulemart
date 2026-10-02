package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

// functionURLRequest is a GET request as a Lambda Function URL delivers it.
const functionURLRequest = `{
  "version": "2.0",
  "rawPath": "/fabricahq/code-rules-test-library/practices/testing/verify-retry-limits",
  "rawQueryString": "tab=versions",
  "headers": {"accept": "text/html", "host": "abc.lambda-url.us-east-1.on.aws"},
  "requestContext": {
    "domainName": "abc.lambda-url.us-east-1.on.aws",
    "requestId": "request-123",
    "http": {"method": "GET", "path": "/fabricahq/code-rules-test-library/practices/testing/verify-retry-limits", "sourceIp": "203.0.113.1"}
  },
  "isBase64Encoded": false
}`

// recorder is a pages handler that records the request it got and answers with body.
type recorder struct {
	got  *http.Request
	body []byte
}

func (h *recorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.got = r
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(h.body)
}

func TestHandleServesFunctionURLRequestsWithThePages(t *testing.T) {
	pages := &recorder{body: []byte("<h1>Verify retry limits</h1>")}

	out, err := newFunction(pages).handle(context.Background(), json.RawMessage(functionURLRequest))

	if err != nil {
		t.Fatal(err)
	}
	if pages.got == nil || pages.got.URL.Path != "/fabricahq/code-rules-test-library/practices/testing/verify-retry-limits" ||
		pages.got.URL.Query().Get("tab") != "versions" || lambdaRequestID(pages.got) != "request-123" {
		t.Fatalf("the pages got %+v", pages.got)
	}
	resp := out.(events.APIGatewayV2HTTPResponse)
	if resp.StatusCode != http.StatusOK || resp.Body != string(pages.body) || resp.Headers["Cache-Control"] != "public, max-age=60" {
		t.Fatalf("answered %+v", resp)
	}
}

// Fonts aren't text, so the Function URL needs them base64-encoded.
func TestHandleEncodesBinaryResponses(t *testing.T) {
	font := []byte{0x77, 0x4f, 0x46, 0x32, 0xff, 0xfe, 0x00}

	out, err := newFunction(&recorder{body: font}).handle(context.Background(), json.RawMessage(functionURLRequest))

	if err != nil {
		t.Fatal(err)
	}
	resp := out.(events.APIGatewayV2HTTPResponse)
	decoded, err := base64.StdEncoding.DecodeString(resp.Body)
	if !resp.IsBase64Encoded || err != nil || !bytes.Equal(decoded, font) {
		t.Fatalf("answered %+v", resp)
	}
}

func TestHandleRejectsEventsItDoesNotRecognize(t *testing.T) {
	for name, event := range map[string]string{
		"empty object": `{}`,
		"the schedule's event, which the worker handles": `{"source":"schedule"}`,
		"other source":      `{"source":"aws.events"}`,
		"unrelated payload": `{"Records":[]}`,
		"not a JSON object": `[]`,
	} {
		t.Run(name, func(t *testing.T) {
			pages := &recorder{}

			_, err := newFunction(pages).handle(context.Background(), json.RawMessage(event))

			if err == nil {
				t.Fatal("accepted an unrecognized event")
			}
			if pages.got != nil {
				t.Fatal("an unrecognized event reached the pages")
			}
		})
	}
}
