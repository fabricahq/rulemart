// Command web serves Rulemart's pages. On Lambda, CloudFront reaches it through its Function URL, and the
// EventBridge schedule invokes it with {"source": "schedule"}, which it acknowledges and ignores until the
// library-release poller arrives in the next slice. Run anywhere else, it serves HTTP at ADDR, 127.0.0.1:8080 by
// default.
//
// Set DATABASE_URL to a connection string, or DATABASE_URL_PARAMETER to the SSM parameter holding one, as on Lambda.
package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/core"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"github.com/fabricahq/rulemart/catalog"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres"
	"github.com/fabricahq/rulemart/internal/platform/database"
	"github.com/fabricahq/rulemart/internal/platform/database/migrate"
	"github.com/fabricahq/rulemart/internal/platform/web"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	handler, err := newHandler(context.Background(), logger)
	if err != nil {
		log.Fatal(err)
	}
	if os.Getenv("AWS_LAMBDA_RUNTIME_API") != "" {
		lambda.Start(newFunction(handler, logger).handle)
		return
	}
	addr := cmp.Or(os.Getenv("ADDR"), "127.0.0.1:8080")
	server := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	logger.Info("serving Rulemart", "url", "http://"+addr)
	log.Fatal(server.ListenAndServe())
}

// newHandler returns the pages' handler, reading the catalog from the database the environment names. It
// connects on the first request, so a misconfigured database fails requests rather than the function's start.
func newHandler(ctx context.Context, logger *slog.Logger) (http.Handler, error) {
	source, err := database.SourceFromEnv(ctx, os.Getenv)
	if err != nil {
		return nil, err
	}
	schemaVersion, err := migrate.RequiredVersion()
	if err != nil {
		return nil, err
	}
	vetted, err := catalog.Vetted()
	if err != nil {
		return nil, err
	}
	pages := app.Pages{Store: postgres.New(source.Open(schemaVersion)), Vetted: vetted}
	return web.New(pages, web.Options{Log: logger, RequestID: lambdaRequestID})
}

// lambdaRequestID returns the Lambda request ID of a request the Function URL delivered, or "" for another.
func lambdaRequestID(r *http.Request) string {
	context, ok := core.GetAPIGatewayV2ContextFromContext(r.Context())
	if !ok {
		return ""
	}
	return context.RequestID
}

// function answers the web function's Lambda invocations.
type function struct {
	// adapter turns Function URL requests into requests for the pages' handler.
	adapter *httpadapter.HandlerAdapterV2
	log     *slog.Logger
}

func newFunction(handler http.Handler, logger *slog.Logger) *function {
	return &function{adapter: httpadapter.NewV2(handler), log: logger}
}

// scheduleSource is the source field of the event the EventBridge schedule sends.
const scheduleSource = "schedule"

// invocation holds the fields handle uses to tell the web function's two kinds of events apart.
type invocation struct {
	// Source is scheduleSource in the schedule's event, and absent from Function URL requests.
	Source         string `json:"source"`
	RequestContext struct {
		HTTP struct {
			Method string `json:"method"`
		} `json:"http"`
	} `json:"requestContext"`
}

// handle serves Function URL requests, acknowledges the schedule's event, and rejects anything else.
func (f *function) handle(ctx context.Context, raw json.RawMessage) (any, error) {
	var event invocation
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil, fmt.Errorf("decode invocation event: %v", err)
	}
	switch {
	case event.Source == scheduleSource:
		f.log.InfoContext(ctx, "ignored the scheduled invocation: the library-release poller isn't built yet")
		return map[string]string{"status": "ignored"}, nil
	case event.RequestContext.HTTP.Method != "":
		// A Function URL request has the same shape as an API Gateway HTTP API request with payload format 2.0.
		var request events.APIGatewayV2HTTPRequest
		if err := json.Unmarshal(raw, &request); err != nil {
			return nil, fmt.Errorf("decode Function URL request: %v", err)
		}
		return f.adapter.ProxyWithContext(ctx, request)
	default:
		return nil, errors.New("unrecognized invocation event: neither the schedule's event nor a Function URL request")
	}
}
