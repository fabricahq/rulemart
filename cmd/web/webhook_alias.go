// Answer only the GitHub App's webhook through any Lambda alias, such as the one whose Function URL nothing signs.

package main

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambdacontext"

	"github.com/fabricahq/rulemart/internal/platform/web"
)

// newWebhookAlias returns the Lambda alias GITHUB_APP_WEBHOOK_ALIAS names, or empty when it's unset. getenv reads a
// variable, such as os.Getenv.
func newWebhookAlias(getenv func(string) string) (string, error) {
	name := getenv("GITHUB_APP_WEBHOOK_ALIAS")
	if name != "" && (!aliasName.MatchString(name) || allDigits.MatchString(name)) {
		return "", fmt.Errorf("read GITHUB_APP_WEBHOOK_ALIAS: want a Lambda alias's name, such as webhook, got %q", name)
	}
	return name, nil
}

// aliasName matches what Lambda accepts as an alias's name, except all digits, which allDigits matches: Lambda reads
// those as a version.
var (
	aliasName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	allDigits = regexp.MustCompile(`^[0-9]+$`)
)

// throughSite reports whether Lambda invoked the function as the site, by the function ARN it invoked, which Lambda
// sets from the URL the request arrived at, so a caller can't forge it, as it could a header or a path:
// arn:aws:lambda:<region>:<account>:function:<name>, through the function's own URL, which only CloudFront can call,
// then :$LATEST or :<version> when invoked so. Any other qualifier is an alias, which may have a URL that nothing signs,
// such as the webhook alias's, so it isn't the site's, whatever GITHUB_APP_WEBHOOK_ALIAS names; nor is an invocation
// without Lambda's context, which names nothing.
func throughSite(ctx context.Context) bool {
	invoked, ok := lambdacontext.FromContext(ctx)
	if !ok {
		return false
	}
	parts := strings.Split(invoked.InvokedFunctionArn, ":")
	switch {
	case len(parts) < 7 || parts[0] != "arn" || parts[2] != "lambda" || parts[5] != "function" || parts[6] == "":
		return false
	case len(parts) == 7:
		return true
	}
	return len(parts) == 8 && (parts[7] == "$LATEST" || allDigits.MatchString(parts[7]))
}

// webhookOnly reports whether request is one the webhook alias's URL may pass to the pages: any method on the
// webhook's path, exactly as written, so the pages answer a wrong method there as they would anywhere.
func webhookOnly(request events.APIGatewayV2HTTPRequest) bool {
	return cmp.Or(request.RawPath, request.RequestContext.HTTP.Path) == web.WebhookHref
}

// rejectThroughWebhookAlias logs that the function refused request through an invocation that isn't the site's, without its path or
// query string, which anyone can choose, and returns the response for it: a 404 that can't be cached and sets nothing.
func rejectThroughWebhookAlias(ctx context.Context, request events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	slog.InfoContext(ctx, "request", "route", "webhook alias", "method", request.RequestContext.HTTP.Method,
		"status", http.StatusNotFound, "requestID", request.RequestContext.RequestID)
	return events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusNotFound,
		Headers:    map[string]string{"Content-Type": "text/plain; charset=utf-8", "Cache-Control": "no-store"},
		Body:       "Not found\n",
	}
}
