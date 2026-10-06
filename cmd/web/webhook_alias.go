// Tell the site's invocations from any Lambda alias's, such as the one whose Function URL nothing signs, which only the
// GitHub App's webhook answers.

package main

import (
	"context"
	"fmt"
	"regexp"

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
// sets from the URL the request arrived at, so a caller can't forge it, as it could a header or a path: the function's
// ARN unqualified, through its own URL, which only CloudFront can call, or qualified by $LATEST or a version when
// invoked so. Any other qualifier is an alias, which may have a URL that nothing signs, such as the webhook alias's,
// so it isn't the site's, whatever GITHUB_APP_WEBHOOK_ALIAS names; nor is an ARN invokedARN doesn't match, or an
// invocation without Lambda's context, which names nothing.
func throughSite(ctx context.Context) bool {
	invoked, ok := lambdacontext.FromContext(ctx)
	if !ok {
		return false
	}
	match := invokedARN.FindStringSubmatch(invoked.InvokedFunctionArn)
	if match == nil {
		return false
	}
	qualifier := match[1]
	return qualifier == "" || qualifier == "$LATEST" || allDigits.MatchString(qualifier)
}

// invokedARN matches the function ARN Lambda invokes, arn:<partition>:lambda:<region>:<account>:function:<name>, with
// a 12-digit account and a name Lambda can give a function, then :<qualifier> when invoked with one: $LATEST, or a
// version or an alias's name, which aliasName matches. Its group is the qualifier, empty without one.
var invokedARN = regexp.MustCompile(
	`^arn:aws(?:-[a-z]+)*:lambda:[a-z]{2}(?:-[a-z]+)+-[0-9]+:[0-9]{12}:function:[A-Za-z0-9_-]{1,64}(?::(\$LATEST|[A-Za-z0-9_-]{1,128}))?$`,
)
