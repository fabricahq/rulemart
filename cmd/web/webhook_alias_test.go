package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambdacontext"

	accountsapp "github.com/fabricahq/rulemart/internal/contexts/accounts/app"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/github"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/platform/secret"
	"github.com/fabricahq/rulemart/internal/platform/web"
)

const (
	// functionARN is the web function's ARN, as Lambda passes it for a request through the function's own URL, which
	// CloudFront signs.
	functionARN = "arn:aws:lambda:us-east-1:123456789012:function:rulemart-web"
	// aliasARN is the ARN Lambda passes for a request through the webhook alias's URL, which nothing signs.
	aliasARN = functionARN + ":webhook"
	// webhookSecret is the secret the test site's GitHub App signs its webhook's deliveries with.
	webhookSecret = "webhook-secret"
	// ping is a delivery GitHub sends when the webhook is set up, which changes nothing.
	ping = `{"zen":"Keep it logically awesome.","hook_id":1,"installation":{"id":7,"app_id":1}}`
)

// call is a Function URL request: its method, path, headers, cookies, and body.
type call struct {
	method, path string
	headers      map[string]string
	cookies      []string
	body         string
}

// invoke returns c as the Function URL delivers it.
func invoke(t *testing.T, c call) json.RawMessage {
	t.Helper()
	var event map[string]any
	if err := json.Unmarshal([]byte(functionURLRequest), &event); err != nil {
		t.Fatal(err)
	}
	event["rawPath"], event["rawQueryString"], event["body"] = c.path, "", c.body
	headers := event["headers"].(map[string]any)
	for name, value := range c.headers {
		headers[strings.ToLower(name)] = value
	}
	if c.cookies != nil {
		event["cookies"] = c.cookies
	}
	event["requestContext"].(map[string]any)["http"].(map[string]any)["method"] = c.method
	event["requestContext"].(map[string]any)["http"].(map[string]any)["path"] = c.path
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// invokedAt returns the context Lambda gives a request through arn.
func invokedAt(arn string) context.Context {
	return lambdacontext.NewContext(context.Background(), &lambdacontext.LambdaContext{InvokedFunctionArn: arn})
}

// newSiteFunction returns the function with the real site: sign-in and the GitHub App, whose webhook takes deliveries
// signed with webhookSecret. A delivery that changes nothing, such as a ping, needs nothing else.
func newSiteFunction(t *testing.T) *function {
	t.Helper()
	gitHubApp := github.NewApp(github.AppConfig{
		ID: 1, ClientID: "Iv1.abc", Slug: "rulemart-by-fabrica", PrivateKey: secret.FromValue("pem"), WebhookSecret: secret.FromValue(webhookSecret),
	}, nil)
	site, err := web.New(app.Pages{}, web.Options{
		Log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		Accounts:       accountsapp.Sessions{},
		GitHubAccounts: accountsapp.GitHubAccounts{App: gitHubApp},
		GitHubWebhook:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return newFunction(site, site.Webhook())
}

// sign returns body's X-Hub-Signature-256 with webhookSecret.
func sign(body string) string {
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// answer returns what f answers c with, invoked at ctx.
func answer(t *testing.T, f *function, ctx context.Context, c call) events.APIGatewayV2HTTPResponse {
	t.Helper()
	out, err := f.handle(ctx, invoke(t, c))
	if err != nil {
		t.Fatal(err)
	}
	return out.(events.APIGatewayV2HTTPResponse)
}

// refusedPlainly reports whether resp is a plain 404 that sets no cookie and can't be cached.
func refusedPlainly(resp events.APIGatewayV2HTTPResponse) bool {
	return resp.StatusCode == http.StatusNotFound && len(resp.Cookies) == 0 && resp.Headers["Cache-Control"] == "no-store" &&
		strings.HasPrefix(resp.Headers["Content-Type"], "text/plain")
}

// The webhook alias's URL takes requests from anyone, so through it the function answers only GitHub's deliveries,
// with the webhook alone: a cross-site browser's headers and cookies change nothing, its signature decides, and every
// other request, a GET on the webhook's path too, gets a plain 404 before any page could set a cookie or render.
func TestThroughTheWebhookAliasOnlyDeliveriesReachTheWebhook(t *testing.T) {
	f := newSiteFunction(t)
	browser := map[string]string{"Origin": "https://attacker.example", "Sec-Fetch-Site": "cross-site"}
	stale := []string{"__Host-rulemart-session=stale", "__Host-rulemart-notice=unknown"}
	delivery := func(signature string) map[string]string {
		headers := map[string]string{"X-GitHub-Delivery": "delivery-1", "X-GitHub-Event": "ping", "X-Hub-Signature-256": signature}
		for name, value := range browser {
			headers[name] = value
		}
		return headers
	}
	for name, tc := range map[string]struct {
		call call
		want int
	}{
		"a signed delivery":     {call{http.MethodPost, web.WebhookHref, delivery(sign(ping)), stale, ping}, http.StatusNoContent},
		"an unsigned delivery":  {call{http.MethodPost, web.WebhookHref, delivery("sha256=00"), stale, ping}, http.StatusUnauthorized},
		"a GET on its path":     {call{http.MethodGet, web.WebhookHref, browser, stale, ""}, http.StatusNotFound},
		"the home page":         {call{http.MethodGet, "/", browser, stale, ""}, http.StatusNotFound},
		"the privacy page":      {call{http.MethodGet, "/privacy", browser, stale, ""}, http.StatusNotFound},
		"signing in":            {call{http.MethodPost, "/account/signin", browser, stale, ""}, http.StatusNotFound},
		"a trailing slash":      {call{http.MethodPost, web.WebhookHref + "/", delivery(sign(ping)), stale, ping}, http.StatusNotFound},
		"an escaped spelling":   {call{http.MethodPost, "/account/github/%77ebhook", delivery(sign(ping)), stale, ping}, http.StatusNotFound},
		"a static file":         {call{http.MethodGet, "/static/app.css", browser, stale, ""}, http.StatusNotFound},
		"the webhook's sibling": {call{http.MethodPost, "/account/github/webhooks", delivery(sign(ping)), stale, ping}, http.StatusNotFound},
	} {
		t.Run(name, func(t *testing.T) {
			resp := answer(t, f, invokedAt(aliasARN), tc.call)

			if resp.StatusCode != tc.want || len(resp.Cookies) != 0 || strings.Contains(resp.Headers["Content-Type"], "html") {
				t.Errorf("answered %d, %q, setting %q; want %d, setting nothing", resp.StatusCode, resp.Headers["Content-Type"], resp.Cookies, tc.want)
			}
			if tc.want == http.StatusNotFound && !refusedPlainly(resp) {
				t.Errorf("answered %+v, want a plain 404 that sets no cookie and can't be cached", resp)
			}
		})
	}
}

// Only an invocation Lambda identifies as the site's reaches the pages: through the function's own URL, which only
// CloudFront can call, unqualified, or qualified by $LATEST or a version. Any other qualifier is an alias, which can
// have a URL nothing signs, so it gets only the webhook whatever GITHUB_APP_WEBHOOK_ALIAS says, and so does an
// invocation without Lambda's context, which names nothing.
func TestOnlyTheSitesInvocationsReachThePages(t *testing.T) {
	f := newSiteFunction(t)
	for name, tc := range map[string]struct {
		ctx     context.Context
		reaches bool
	}{
		"the function's own URL":          {invokedAt(functionARN), true},
		"$LATEST":                         {invokedAt(functionARN + ":$LATEST"), true},
		"a version":                       {invokedAt(functionARN + ":42"), true},
		"the webhook alias":               {invokedAt(aliasARN), false},
		"an alias the variable misspells": {invokedAt(functionARN + ":webhooks"), false},
		"another alias":                   {invokedAt(functionARN + ":live"), false},
		"an ARN that isn't a function's":  {invokedAt("arn:aws:lambda:us-east-1:123456789012"), false},
		"no Lambda context":               {context.Background(), false},
	} {
		t.Run(name, func(t *testing.T) {
			resp := answer(t, f, tc.ctx, call{method: http.MethodGet, path: "/privacy"})

			if reached := resp.StatusCode == http.StatusOK && strings.Contains(resp.Body, "Privacy"); reached != tc.reaches {
				t.Fatalf("reached the privacy page: %v, want %v; answered %d", reached, tc.reaches, resp.StatusCode)
			}
			if !tc.reaches && !refusedPlainly(resp) {
				t.Errorf("answered %+v, want a plain 404 that sets no cookie and can't be cached", resp)
			}
		})
	}
}

// GITHUB_APP_WEBHOOK_ALIAS names a Lambda alias, which is never all digits, since that's a version, nor $LATEST, so a
// name Lambda can't give an alias is a mistake to report at start.
func TestTheWebhookAliasMustBeAnAliasName(t *testing.T) {
	for text, valid := range map[string]bool{
		"":           true,
		"webhook":    true,
		"web_Hook-2": true,
		"$LATEST":    false,
		"42":         false,
		"web:hook":   false,
		" webhook":   false,
	} {
		t.Run(text, func(t *testing.T) {
			got, err := newWebhookAlias(func(name string) string {
				if name == "GITHUB_APP_WEBHOOK_ALIAS" {
					return text
				}
				return ""
			})
			if valid && (err != nil || got != text) {
				t.Errorf("got %q, %v, want %q", got, err, text)
			}
			if !valid && err == nil {
				t.Errorf("accepted %q", text)
			}
		})
	}
}
