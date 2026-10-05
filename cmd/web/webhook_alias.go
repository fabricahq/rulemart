// Answer only the GitHub App's webhook through the Lambda alias whose Function URL nothing signs.

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

// throughAlias reports whether Lambda invoked the function through alias, by the qualifier of the function ARN it
// invoked: arn:aws:lambda:<region>:<account>:function:<name>, then :<alias> through the alias's Function URL. Lambda
// sets the ARN from the URL the request arrived at, so a caller can't forge it, as it could a header or a path.
func throughAlias(ctx context.Context, alias string) bool {
	invoked, ok := lambdacontext.FromContext(ctx)
	if !ok {
		return false
	}
	parts := strings.Split(invoked.InvokedFunctionArn, ":")
	return len(parts) == 8 && parts[7] == alias
}

// webhookOnly reports whether request is one the webhook alias's URL may pass to the pages: any method on the
// webhook's path, exactly as written, so the pages answer a wrong method there as they would anywhere.
func webhookOnly(request events.APIGatewayV2HTTPRequest) bool {
	return cmp.Or(request.RawPath, request.RequestContext.HTTP.Path) == web.WebhookHref
}

// rejectThroughWebhookAlias logs that the function refused request through the webhook alias, without its path or
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
