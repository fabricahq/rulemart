package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambdacontext"
)

const (
	// functionARN is the web function's ARN, as Lambda passes it for a request through the function's own URL, which
	// CloudFront signs.
	functionARN = "arn:aws:lambda:us-east-1:123456789012:function:rulemart-web"
	// aliasARN is the ARN Lambda passes for a request through the webhook alias's URL, which nothing signs.
	aliasARN = functionARN + ":webhook"
)

// invokeAt returns a Function URL request for method and path, invoked at arn, and the context Lambda would give it.
func invokeAt(t *testing.T, arn, method, path string) (context.Context, json.RawMessage) {
	t.Helper()
	var event map[string]any
	if err := json.Unmarshal([]byte(functionURLRequest), &event); err != nil {
		t.Fatal(err)
	}
	event["rawPath"], event["rawQueryString"] = path, ""
	event["requestContext"].(map[string]any)["http"].(map[string]any)["method"] = method
	event["requestContext"].(map[string]any)["http"].(map[string]any)["path"] = path
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	return lambdacontext.NewContext(context.Background(), &lambdacontext.LambdaContext{InvokedFunctionArn: arn}), raw
}

// sessionSetter is a pages handler that sets a session cookie on every response, as signing in does, so a test can tell
// whether a request reached the pages at all.
type sessionSetter struct{ reached bool }

func (h *sessionSetter) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	h.reached = true
	http.SetCookie(w, &http.Cookie{Name: "__Host-rulemart-session", Value: "v", Path: "/", Secure: true, HttpOnly: true})
	w.WriteHeader(http.StatusNoContent)
}

// The webhook alias's URL takes requests from anyone, so through it the function answers only the webhook, and answers
// 404 for every other path before the pages could set a cookie or render anything.
func TestThroughTheWebhookAliasOnlyTheWebhookReachesThePages(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		reaches      bool
	}{
		{http.MethodPost, "/account/github/webhook", true},
		// The pages answer other methods on the webhook's path as they would anywhere.
		{http.MethodGet, "/account/github/webhook", true},
		{http.MethodGet, "/", false},
		{http.MethodGet, "/me", false},
		{http.MethodPost, "/account/signin", false},
		{http.MethodPost, "/account/github/webhook/", false},
		{http.MethodPost, "/account/github/webhooks", false},
		{http.MethodPost, "/account/github/%77ebhook", false},
		{http.MethodGet, "/static/app.css", false},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			pages := &sessionSetter{}
			ctx, raw := invokeAt(t, aliasARN, tc.method, tc.path)

			out, err := newFunction(pages, "webhook").handle(ctx, raw)

			if err != nil {
				t.Fatal(err)
			}
			resp := out.(events.APIGatewayV2HTTPResponse)
			if tc.reaches {
				if !pages.reached || resp.StatusCode != http.StatusNoContent {
					t.Errorf("answered %d without reaching the pages", resp.StatusCode)
				}
				return
			}
			if pages.reached {
				t.Fatal("reached the pages")
			}
			if resp.StatusCode != http.StatusNotFound || len(resp.Cookies) != 0 || resp.Headers["Cache-Control"] != "no-store" {
				t.Errorf("answered %+v, want a 404 that sets no cookie and can't be cached", resp)
			}
		})
	}
}

// Through the function's own URL, which only CloudFront can call, and through any qualifier that isn't the alias,
// every path reaches the pages; with no alias configured, nothing is restricted, whatever the qualifier.
func TestOutsideTheWebhookAliasEveryPathReachesThePages(t *testing.T) {
	for name, tc := range map[string]struct{ alias, arn string }{
		"the function's own URL":          {"webhook", functionARN},
		"another alias":                   {"webhook", functionARN + ":live"},
		"an alias whose name contains it": {"webhook", functionARN + ":webhooks"},
		"no alias configured":             {"", aliasARN},
	} {
		t.Run(name, func(t *testing.T) {
			for _, path := range []string{"/", "/me", "/account/github/webhook"} {
				pages := &sessionSetter{}
				ctx, raw := invokeAt(t, tc.arn, http.MethodGet, path)

				out, err := newFunction(pages, tc.alias).handle(ctx, raw)

				if err != nil {
					t.Fatal(err)
				}
				if resp := out.(events.APIGatewayV2HTTPResponse); !pages.reached || resp.StatusCode != http.StatusNoContent {
					t.Errorf("%s answered %d without reaching the pages", path, resp.StatusCode)
				}
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
