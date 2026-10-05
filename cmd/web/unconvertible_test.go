package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

// captureStdout returns what fn writes to standard output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = stdout }()
	fn()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if _, err := io.Copy(&out, reader); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// The Lambda adapter prints the raw path and query string of a request it can't turn into an http.Request, outside
// the pages' logging, so the function must answer such a request itself, without printing either.
func TestHandleRejectsRequestsTheAdapterCantConvertWithoutLoggingThem(t *testing.T) {
	for name, edit := range map[string]func(map[string]any){
		"an invalid escape in the path": func(event map[string]any) {
			event["rawPath"] = "/alice@example.test/private%GG"
			event["rawQueryString"] = "token=query-s3cr3t"
		},
		// The adapter builds the URL on GO_API_HOST when it's set; goAPIHost isn't part of the event.
		"an invalid GO_API_HOST": func(event map[string]any) {
			event["rawQueryString"] = "token=query-s3cr3t"
			event["goAPIHost"] = "https://invalid%host"
		},
		"a body that isn't base64": func(event map[string]any) {
			event["rawQueryString"] = "token=query-s3cr3t"
			event["isBase64Encoded"] = true
			event["body"] = "not base64!"
		},
	} {
		t.Run(name, func(t *testing.T) {
			var event map[string]any
			if err := json.Unmarshal([]byte(functionURLRequest), &event); err != nil {
				t.Fatal(err)
			}
			edit(event)
			raw, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			pages := &recorder{}
			var out any
			if host, ok := event["goAPIHost"].(string); ok {
				t.Setenv("GO_API_HOST", host)
			}

			printed := captureStdout(t, func() {
				out, err = newFunction(pages, "").handle(context.Background(), raw)
			})

			if err != nil {
				t.Fatal(err)
			}
			resp, ok := out.(events.APIGatewayV2HTTPResponse)
			if !ok || resp.StatusCode != http.StatusBadRequest || resp.Headers["Cache-Control"] != "no-store" {
				t.Fatalf("answered %+v, want an uncacheable 400", out)
			}
			if pages.got != nil {
				t.Fatal("the request reached the pages")
			}
			for _, leak := range []string{"s3cr3t", "alice", "private"} {
				if strings.Contains(printed, leak) {
					t.Fatalf("printed %q", printed)
				}
			}
		})
	}
}
