package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/ssm"

	"github.com/fabricahq/rulemart/internal/database"
	"github.com/fabricahq/rulemart/internal/hello"
)

// internalDetail is the kind of dependency error that used to reach public responses: it names an IAM role, a
// parameter path, and the AWS operation.
const internalDetail = "operation error SSM: GetParameter, AccessDeniedException: User: arn:aws:sts::123456789012:assumed-role/rulemart-web is not authorized to perform ssm:GetParameter on /rulemart/skeleton/database-url"

// failingParameter stands in for an SSM parameter that can't be read.
type failingParameter struct{}

func (failingParameter) GetParameter(context.Context, *ssm.GetParameterInput, ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	return nil, errors.New(internalDetail)
}

func assertNoInternalDetail(t *testing.T, body string) {
	t.Helper()
	for _, leak := range []string{"arn:", "/rulemart/", "AccessDenied", "GetParameter"} {
		if strings.Contains(body, leak) {
			t.Fatalf("response body %q exposes %q", body, leak)
		}
	}
}

// Dependency errors used to be returned to public clients, exposing IAM roles, parameter paths, and AWS operations.
// They're now logged with the route and request ID, and clients get a fixed message.
func TestServeLogsDependencyErrorsAndKeepsThemOutOfResponses(t *testing.T) {
	for name, tc := range map[string]struct {
		server *server
		req    events.LambdaFunctionURLRequest
		status int
	}{
		"messages when Neon can't be reached": {
			server: &server{messages: hello.NewStore(database.New(failingParameter{}, "/rulemart/skeleton/database-url", 1))},
			req:    request(http.MethodGet, "/messages", nil),
			status: http.StatusServiceUnavailable,
		},
		"enqueue when SQS fails": {
			server: &server{queue: &fakeQueue{err: errors.New(internalDetail)}},
			req:    post("/enqueue", url.Values{"text": {"hi"}}),
			status: http.StatusBadGateway,
		},
	} {
		t.Run(name, func(t *testing.T) {
			var logs bytes.Buffer
			tc.server.log = slog.New(slog.NewJSONHandler(&logs, nil))
			tc.req.RequestContext.RequestID = "request-123"

			resp := tc.server.serve(context.Background(), tc.req)

			if resp.StatusCode != tc.status {
				t.Fatalf("got %d, want %d", resp.StatusCode, tc.status)
			}
			assertNoInternalDetail(t, resp.Body)
			for _, want := range []string{internalDetail, `"route":"` + tc.req.RawPath + `"`, `"requestID":"request-123"`} {
				if !strings.Contains(logs.String(), want) {
					t.Fatalf("logs %s don't include %s", logs.String(), want)
				}
			}
		})
	}
}
