package main

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"

	"github.com/fabricahq/rulemart/internal/hello"
)

func request(method, path string, query map[string]string) events.LambdaFunctionURLRequest {
	req := events.LambdaFunctionURLRequest{RawPath: path, QueryStringParameters: query}
	req.RequestContext.HTTP.Method = method
	return req
}

// These routes answer before reaching SQS or Neon, so they run without AWS.
func TestServe(t *testing.T) {
	tests := []struct {
		name   string
		req    events.LambdaFunctionURLRequest
		status int
		header string
		value  string
	}{
		{"home lists routes", request(http.MethodGet, "/", nil), http.StatusOK, "cache-control", "no-store"},
		{"text is bounded", post("/enqueue", url.Values{"text": {strings.Repeat("x", hello.MaxTextLength+1)}}), http.StatusBadRequest, "cache-control", "no-store"},
		{"cached page is cacheable", request(http.MethodGet, "/cached", nil), http.StatusOK, "cache-control", "public, max-age=60"},
		{"unknown path", request(http.MethodGet, "/nope", nil), http.StatusNotFound, "cache-control", "no-store"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := (&server{}).serve(context.Background(), tt.req)
			if resp.StatusCode != tt.status || resp.Headers[tt.header] != tt.value {
				t.Fatalf("got %d with %s=%q, want %d with %q", resp.StatusCode, tt.header, resp.Headers[tt.header], tt.status, tt.value)
			}
		})
	}
}
