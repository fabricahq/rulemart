// Command web serves Rulemart's pages. On Lambda, CloudFront reaches it through its Function URL. Run anywhere else,
// it serves HTTP at ADDR, 127.0.0.1:8080 by default.
//
// Set DATABASE_URL to a connection string, or DATABASE_URL_PARAMETER to the SSM parameter holding one, as on Lambda.
// LOG_LEVEL and RULEMART_RELEASE configure its logs, as internal/platform/logging describes. RULEMART_BASE_URL, such
// as https://rulemart.fabricahq.com, is the public origin each page names as its canonical address; unset, pages name
// none.
package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/fabricahq/rulemart/internal/platform/logging"
	"github.com/fabricahq/rulemart/internal/platform/web"
)

func main() {
	start := time.Now()
	logger, err := logging.New(os.Stdout, os.Getenv)
	if err != nil {
		exit(logger, err)
	}
	// The log package then writes JSON lines through logger too.
	slog.SetDefault(logger)
	schemaVersion, err := migrate.RequiredVersion()
	if err != nil {
		exit(logger, err)
	}
	handler, err := newHandler(context.Background(), logger, schemaVersion)
	if err != nil {
		exit(logger, err)
	}
	logging.Ready(logger, schemaVersion, time.Since(start))
	if os.Getenv("AWS_LAMBDA_RUNTIME_API") != "" {
		lambda.Start(newFunction(handler).handle)
		return
	}
	addr := cmp.Or(os.Getenv("ADDR"), "127.0.0.1:8080")
	server := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	logger.Info("serving Rulemart", "url", "http://"+addr)
	err = server.ListenAndServe()
	logger.Error("server stopped", "error", err.Error())
	os.Exit(1)
}

// exit reports that the function couldn't start, and stops it.
func exit(logger *slog.Logger, err error) {
	logging.StartupFailed(logger, err)
	os.Exit(1)
}

// newHandler returns the pages' handler, reading the catalog from the database the environment names, which must
// be at schemaVersion. It connects on the first request, so a misconfigured database fails requests rather than
// the function's start.
func newHandler(ctx context.Context, logger *slog.Logger, schemaVersion int64) (http.Handler, error) {
	baseURL, err := web.ParseBaseURL(os.Getenv("RULEMART_BASE_URL"))
	if err != nil {
		return nil, fmt.Errorf("read RULEMART_BASE_URL: %v", err)
	}
	source, err := database.SourceFromEnv(ctx, os.Getenv)
	if err != nil {
		return nil, err
	}
	vetted, err := catalog.Vetted()
	if err != nil {
		return nil, err
	}
	groups, err := catalog.CanonicalGroups()
	if err != nil {
		return nil, err
	}
	pages := app.Pages{Store: postgres.New(source.Open(schemaVersion)), Vetted: vetted, Groups: groups}
	return web.New(pages, web.Options{Log: logger, RequestID: lambdaRequestID, BaseURL: baseURL})
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
}

func newFunction(handler http.Handler) *function {
	return &function{adapter: httpadapter.NewV2(handler)}
}

// invocation holds the field handle uses to recognize a Function URL request.
type invocation struct {
	RequestContext struct {
		HTTP struct {
			Method string `json:"method"`
		} `json:"http"`
	} `json:"requestContext"`
}

// handle serves Function URL requests, and rejects anything else. The schedule invokes the worker function, not this
// one.
func (f *function) handle(ctx context.Context, raw json.RawMessage) (any, error) {
	var event invocation
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil, fmt.Errorf("decode invocation event: %v", err)
	}
	if event.RequestContext.HTTP.Method == "" {
		return nil, errors.New("unrecognized invocation event: not a Function URL request")
	}
	// A Function URL request has the same shape as an API Gateway HTTP API request with payload format 2.0.
	var request events.APIGatewayV2HTTPRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("decode Function URL request: %v", err)
	}
	if !convertible(request) {
		return rejectUnconvertible(ctx, request), nil
	}
	return f.adapter.ProxyWithContext(ctx, request)
}
