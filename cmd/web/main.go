// Command web serves Rulemart's pages. On Lambda, CloudFront reaches it through its Function URL. Run anywhere else,
// it serves HTTP at ADDR, 127.0.0.1:8080 by default.
//
// Set DATABASE_URL to a connection string, or DATABASE_URL_PARAMETER to the SSM parameter holding one, as on Lambda.
// LOG_LEVEL and RULEMART_RELEASE configure its logs, as internal/platform/logging describes. RULEMART_BASE_URL, such
// as https://rulemart.fabricahq.com, is the public origin each page names as its canonical address, and where
// sign-in happens; unset, pages name none.
//
// GITHUB_CLIENT_ID names the GitHub OAuth app visitors sign in with, and GITHUB_CLIENT_SECRET holds its client
// secret, or GITHUB_CLIENT_SECRET_PARAMETER names the SSM parameter holding it, as on Lambda. Each session keeps the
// visitor's GitHub token sealed with the key TOKEN_KEY holds, 32 random bytes in base64, or TOKEN_KEY_PARAMETER names
// the SSM parameter holding it; GitHub sign-in needs it. Unset, visitors can't sign in with GitHub, and pages offer no
// sign-in, except in a build with the rulemartdev tag, which offers test users instead; such a build refuses to start
// on Lambda.
//
// Signed-in visitors can list libraries. QUEUE_URL names the worker's jobs queue, where each new listing's check is
// sent at once; unset, as locally, listings wait for the worker's next poll, such as make worker.
//
// CLOUDFLARE_WEB_ANALYTICS_TOKEN is the site token of a Cloudflare Web Analytics site, which turns on its beacon in
// every page; unset, pages load no analytics. The token is public: every page shows it.
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
	accountsapp "github.com/fabricahq/rulemart/internal/contexts/accounts/app"
	accounts "github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/github"
	accountspostgres "github.com/fabricahq/rulemart/internal/contexts/accounts/store/postgres"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres"
	"github.com/fabricahq/rulemart/internal/platform/database"
	"github.com/fabricahq/rulemart/internal/platform/database/migrate"
	"github.com/fabricahq/rulemart/internal/platform/logging"
	"github.com/fabricahq/rulemart/internal/platform/queue"
	"github.com/fabricahq/rulemart/internal/platform/secret"
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
	onLambda := os.Getenv("AWS_LAMBDA_RUNTIME_API") != ""
	if web.DevSignIn && onLambda {
		return nil, errors.New("this build has the dev sign-in, which only local builds may have: build without the rulemartdev tag")
	}
	gitHub, err := newGitHub(ctx, os.Getenv)
	if err != nil {
		return nil, err
	}
	keys, err := newTokenKeys(ctx, os.Getenv, gitHub != nil)
	if err != nil {
		return nil, err
	}
	if gitHub != nil && baseURL == nil && onLambda {
		return nil, errors.New("set RULEMART_BASE_URL for GitHub sign-in: GitHub sends visitors back to it")
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
	db := source.Open(schemaVersion)
	catalogStore := postgres.New(db)
	pages := app.Pages{Store: catalogStore, Vetted: vetted, Groups: groups}
	listings := app.Listings{Store: catalogStore, Vetted: vetted}
	if url := os.Getenv("QUEUE_URL"); url != "" {
		jobsQueue, err := queue.New(ctx, url)
		if err != nil {
			return nil, err
		}
		listings.Queue = jobsQueue
	}
	options := web.Options{
		Log: logger, RequestID: lambdaRequestID, BaseURL: baseURL,
		Accounts: accountsapp.Sessions{Store: accountspostgres.New(db), TokenKeys: keys},
		Listings: listings,
		Stars:    app.Stars{Store: catalogStore, Vetted: vetted, Groups: groups},
		Carts:    app.Carts{Store: catalogStore, Vetted: vetted, Groups: groups},
		// Not a secret: Cloudflare's beacon sends it from every page.
		AnalyticsToken: os.Getenv("CLOUDFLARE_WEB_ANALYTICS_TOKEN"),
	}
	if gitHub != nil {
		options.GitHub = gitHub
	}
	return web.New(pages, options)
}

// newGitHub returns the GitHub OAuth app that GITHUB_CLIENT_ID names, with the client secret GITHUB_CLIENT_SECRET or
// GITHUB_CLIENT_SECRET_PARAMETER holds, or nil when none is named. getenv reads a variable, such as os.Getenv.
func newGitHub(ctx context.Context, getenv func(string) string) (*github.Client, error) {
	clientID := getenv("GITHUB_CLIENT_ID")
	clientSecret, err := secret.FromEnv(ctx, getenv, "GITHUB_CLIENT_SECRET")
	switch {
	case err != nil:
		return nil, err
	case clientID == "" && clientSecret == nil:
		return nil, nil
	case clientID == "":
		return nil, errors.New("set GITHUB_CLIENT_ID with GITHUB_CLIENT_SECRET or GITHUB_CLIENT_SECRET_PARAMETER")
	case clientSecret == nil:
		return nil, errors.New("set GITHUB_CLIENT_SECRET or GITHUB_CLIENT_SECRET_PARAMETER with GITHUB_CLIENT_ID")
	}
	return github.New(clientID, clientSecret), nil
}

// newTokenKeys returns the key that seals sessions' GitHub tokens, which TOKEN_KEY or TOKEN_KEY_PARAMETER holds, or nil
// when neither is set. GitHub sign-in, when gitHubSignIn is true, needs it, since every session then keeps a token. A
// key given directly is checked now; one in a parameter, when first used. getenv reads a variable, such as os.Getenv.
func newTokenKeys(ctx context.Context, getenv func(string) string, gitHubSignIn bool) (accountsapp.TokenKeys, error) {
	key, err := secret.FromEnv(ctx, getenv, "TOKEN_KEY")
	switch {
	case err != nil:
		return nil, err
	case key == nil && gitHubSignIn:
		return nil, errors.New("set TOKEN_KEY or TOKEN_KEY_PARAMETER with GITHUB_CLIENT_ID: sessions keep the visitor's GitHub token sealed with it")
	case key == nil:
		return nil, nil
	}
	keys := tokenKeys{secret: key}
	if getenv("TOKEN_KEY") != "" {
		if _, err := keys.TokenKey(ctx); err != nil {
			return nil, fmt.Errorf("read TOKEN_KEY: %v", err)
		}
	}
	return keys, nil
}

// tokenKeys reads the key that seals sessions' GitHub tokens from a secret, as accountsapp.TokenKeys asks.
type tokenKeys struct {
	secret *secret.Secret
}

// TokenKey returns the key the secret holds, reading it again next time when it holds none, such as while an operator
// replaces a parameter's value.
func (k tokenKeys) TokenKey(ctx context.Context) (accounts.TokenKey, error) {
	text, err := k.secret.Value(ctx)
	if err != nil {
		return accounts.TokenKey{}, err
	}
	key, err := accounts.ParseTokenKey(text)
	if err != nil {
		k.secret.Forget()
		return accounts.TokenKey{}, err
	}
	return key, nil
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
