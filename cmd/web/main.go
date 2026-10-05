// Command web serves Rulemart's pages. On Lambda, CloudFront reaches it through its Function URL. Run anywhere else,
// it serves HTTP at ADDR, 127.0.0.1:8080 by default.
//
// Set DATABASE_URL to a connection string, or DATABASE_URL_PARAMETER to the SSM parameter holding one, as on Lambda.
// LOG_LEVEL and RULEMART_RELEASE configure its logs, as internal/platform/logging describes. RULEMART_BASE_URL, such
// as https://rulemart.fabricahq.com, is the public origin each page names as its canonical address, and where
// sign-in happens; unset, pages name none, except in a build with the rulemartdev tag, whose pages name the loopback
// address it serves at.
//
// GITHUB_CLIENT_ID names the GitHub OAuth app visitors sign in with, and GITHUB_CLIENT_SECRET holds its client
// secret, or GITHUB_CLIENT_SECRET_PARAMETER names the SSM parameter holding it, as on Lambda. Each session keeps the
// visitor's GitHub token sealed with the key TOKEN_KEY holds, 32 random bytes in base64, or TOKEN_KEY_PARAMETER names
// the SSM parameter holding it; GitHub sign-in needs it. Unset, visitors can't sign in with GitHub, and pages offer no
// sign-in, except in a build with the rulemartdev tag, which offers test users instead; such a build refuses to start
// on Lambda.
//
// With GitHub sign-in, the dashboard reads each visitor's organizations and repositories with their token. The GitHub
// App "Rulemart by Fabrica" reads the private repositories visitors install it on: GITHUB_APP_ID, GITHUB_APP_CLIENT_ID,
// and GITHUB_APP_SLUG name it, GITHUB_APP_PRIVATE_KEY holds its private key, in PEM, or GITHUB_APP_PRIVATE_KEY_PARAMETER
// names the SSM parameter holding it, and GITHUB_APP_WEBHOOK_SECRET or GITHUB_APP_WEBHOOK_SECRET_PARAMETER holds its
// webhook's secret. Set all of them or none; unset, the dashboard reads public repositories only. A build with the
// rulemartdev tag and no GitHub sign-in reads a fake GitHub in memory instead, with an app of its own.
//
// GITHUB_APP_WEBHOOK_ALIAS names the Lambda alias whose Function URL takes the app's webhook deliveries from anyone,
// since GitHub can't sign them as CloudFront's origin access control asks, and the privacy page then says what a
// delivery discards; on Lambda, the app without it warns at start. Whatever it names, through any alias, and without
// Lambda's context, the function answers only a POST to the webhook, /account/github/webhook, whose own signature check
// authenticates GitHub, and a plain 404 to everything else, before any page could set a cookie or render. Only the
// function's own URL, unqualified or through $LATEST or a version, reaches the pages.
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
	"net/url"
	"os"
	"regexp"
	"strconv"
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
	webhookAlias, err := newWebhookAlias(os.Getenv)
	if err != nil {
		exit(logger, err)
	}
	site, err := newSite(context.Background(), logger, schemaVersion, webhookAlias)
	if err != nil {
		exit(logger, err)
	}
	logging.Ready(logger, schemaVersion, time.Since(start))
	if os.Getenv("AWS_LAMBDA_RUNTIME_API") != "" {
		lambda.Start(newFunction(site, site.Webhook()).handle)
		return
	}
	addr := listenAddr(os.Getenv)
	server := &http.Server{Addr: addr, Handler: site, ReadHeaderTimeout: 10 * time.Second}
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

// newSite returns the site, reading the catalog from the database the environment names, which must be at
// schemaVersion. It connects on the first request, so a misconfigured database fails requests rather than
// the function's start. webhookAlias names the alias GitHub's webhook deliveries arrive through, or is empty without
// one.
func newSite(ctx context.Context, logger *slog.Logger, schemaVersion int64, webhookAlias string) (*web.Site, error) {
	baseURL, err := newBaseURL(os.Getenv)
	if err != nil {
		return nil, err
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
	reader, gitHubApp, err := newGitHubReaders(ctx, os.Getenv, gitHub != nil)
	if err != nil {
		return nil, err
	}
	if gitHub == nil && web.DevSignIn {
		reader, gitHubApp, keys = devGitHub(keys)
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
	accountsStore := accountspostgres.New(db)
	sessions := accountsapp.Sessions{Store: accountsStore, TokenKeys: keys}
	options := web.Options{
		Log: logger, RequestID: lambdaRequestID, BaseURL: baseURL,
		Accounts: sessions,
		Listings: listings,
		Stars:    app.Stars{Store: catalogStore, Vetted: vetted, Groups: groups},
		Carts:    app.Carts{Store: catalogStore, Vetted: vetted, Groups: groups},
		// Not a secret: Cloudflare's beacon sends it from every page.
		AnalyticsToken: os.Getenv("CLOUDFLARE_WEB_ANALYTICS_TOKEN"),
	}
	if gitHub != nil {
		options.GitHub = gitHub
	}
	if reader != nil {
		accounts := accountsapp.GitHubAccounts{Store: accountsStore, Sessions: sessions, GitHub: reader}
		if gitHubApp != nil {
			accounts.App = gitHubApp
			options.GitHubWebhook = webhookAlias != ""
			if onLambda && webhookAlias == "" {
				logger.Warn("GITHUB_APP_WEBHOOK_ALIAS is unset, so the privacy page doesn't say what the GitHub App's webhook " +
					"discards: name the alias whose Function URL takes its deliveries, once there is one")
			}
		}
		options.GitHubAccounts = accounts
	}
	return web.New(pages, options)
}

// listenAddr returns the address a server run anywhere but on Lambda listens at: ADDR, or 127.0.0.1:8080.
func listenAddr(getenv func(string) string) string {
	return cmp.Or(getenv("ADDR"), "127.0.0.1:8080")
}

// newBaseURL returns the public origin RULEMART_BASE_URL names. Unset, a build with the rulemartdev tag names the
// address it listens at, when that's a loopback one, such as http://127.0.0.1:8080, so a local build serves the sitemap
// and robots.txt names it, as in production.
func newBaseURL(getenv func(string) string) (*url.URL, error) {
	text := getenv("RULEMART_BASE_URL")
	if text == "" && web.DevSignIn {
		if local, err := web.ParseBaseURL("http://" + listenAddr(getenv)); err == nil {
			return local, nil
		}
	}
	baseURL, err := web.ParseBaseURL(text)
	if err != nil {
		return nil, fmt.Errorf("read RULEMART_BASE_URL: %v", err)
	}
	return baseURL, nil
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

// newGitHubReaders returns what reads visitors' GitHub accounts with their tokens, with GitHub sign-in, when
// gitHubSignIn is true, and the GitHub App that GITHUB_APP_ID, GITHUB_APP_CLIENT_ID, GITHUB_APP_SLUG, and the secrets
// GITHUB_APP_PRIVATE_KEY and GITHUB_APP_WEBHOOK_SECRET name, or nil when none of them is set. Half of the app's
// variables, or an app without GitHub sign-in, is a mistake to report at start. getenv reads a variable, such as
// os.Getenv.
func newGitHubReaders(ctx context.Context, getenv func(string) string, gitHubSignIn bool) (*github.API, *github.App, error) {
	key, err := secret.FromEnv(ctx, getenv, "GITHUB_APP_PRIVATE_KEY")
	if err != nil {
		return nil, nil, err
	}
	webhookSecret, err := secret.FromEnv(ctx, getenv, "GITHUB_APP_WEBHOOK_SECRET")
	if err != nil {
		return nil, nil, err
	}
	id, slug, clientID := getenv("GITHUB_APP_ID"), getenv("GITHUB_APP_SLUG"), getenv("GITHUB_APP_CLIENT_ID")
	set := 0
	for _, present := range []bool{id != "", slug != "", clientID != "", key != nil, webhookSecret != nil} {
		if present {
			set++
		}
	}
	var api *github.API
	if gitHubSignIn {
		api = github.NewAPI(github.APIURL)
	}
	switch {
	case set == 0:
		return api, nil, nil
	case set < 5:
		return nil, nil, errors.New("set GITHUB_APP_ID, GITHUB_APP_CLIENT_ID, GITHUB_APP_SLUG, GITHUB_APP_PRIVATE_KEY or its _PARAMETER, and GITHUB_APP_WEBHOOK_SECRET or its _PARAMETER together, or none of them")
	case !gitHubSignIn:
		return nil, nil, errors.New("set GITHUB_CLIENT_ID for the GitHub App: visitors sign in with GitHub before installing it")
	}
	appID, err := strconv.ParseInt(id, 10, 64)
	if err != nil || appID <= 0 {
		return nil, nil, fmt.Errorf("read GITHUB_APP_ID: want the app's numeric ID, got %q", id)
	}
	if !validSlug.MatchString(slug) {
		return nil, nil, fmt.Errorf("read GITHUB_APP_SLUG: want the app's name in its URLs, such as rulemart-by-fabrica, got %q", slug)
	}
	app := github.NewApp(github.AppConfig{ID: appID, ClientID: clientID, Slug: slug, PrivateKey: key, WebhookSecret: webhookSecret}, api)
	return api, app, nil
}

// validSlug matches a GitHub App's slug: lowercase letters, digits, and hyphens.
var validSlug = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

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

// RereadTokenKey reads the secret's key again, as accountsapp.TokenKeys asks, and keeps it for TokenKey.
func (k tokenKeys) RereadTokenKey(ctx context.Context) (accounts.TokenKey, error) {
	k.secret.Forget()
	return k.TokenKey(ctx)
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
	// site turns Function URL requests into requests for the site, and webhook into requests for the GitHub App's
	// webhook alone, which answers nothing else.
	site, webhook *httpadapter.HandlerAdapterV2
}

func newFunction(site, webhook http.Handler) *function {
	return &function{site: httpadapter.NewV2(site), webhook: httpadapter.NewV2(webhook)}
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
// one. Unless Lambda invoked it as the site, as throughSite says, such as through the webhook alias, only the webhook
// answers, which takes nothing but GitHub's deliveries, before any page could set a cookie or render.
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
	if !throughSite(ctx) {
		return f.webhook.ProxyWithContext(ctx, request)
	}
	return f.site.ProxyWithContext(ctx, request)
}
