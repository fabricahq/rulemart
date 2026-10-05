// Tell the site's invocations from any Lambda alias's, such as the one whose Function URL nothing signs, which only the
// GitHub App's webhook answers.

package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/aws/aws-lambda-go/lambdacontext"
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
