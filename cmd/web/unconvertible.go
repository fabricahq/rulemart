// Answer Function URL requests that the Lambda adapter can't convert, which it would print with their path and query.

package main

import (
	"cmp"
	"context"
	"encoding/base64"
	"log/slog"
	"net/http"
	"strings"

	"github.com/aws/aws-lambda-go/events"
)

// convertible reports whether the adapter can turn request into an http.Request, checking what its conversion can
// fail on: a body marked base64 that isn't, and a URL that doesn't parse, such as a path with an invalid escape. The
// adapter prints a request it can't convert, path and query string included, straight to standard output.
func convertible(request events.APIGatewayV2HTTPRequest) bool {
	if request.IsBase64Encoded {
		if _, err := base64.StdEncoding.DecodeString(request.Body); err != nil {
			return false
		}
	}
	path := cmp.Or(request.RawPath, request.RequestContext.HTTP.Path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	target := "https://" + request.RequestContext.DomainName + path
	if request.RawQueryString != "" {
		target += "?" + request.RawQueryString
	}
	_, err := http.NewRequest(strings.ToUpper(request.RequestContext.HTTP.Method), target, nil)
	return err == nil
}

// rejectUnconvertible logs that the function refused request, without its path or query string, and returns the
// response for it: a 400 that can't be cached.
func rejectUnconvertible(ctx context.Context, request events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	slog.WarnContext(ctx, "request", "route", "unconvertible", "method", request.RequestContext.HTTP.Method,
		"status", http.StatusBadRequest, "requestID", request.RequestContext.RequestID)
	return events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusBadRequest,
		Headers:    map[string]string{"Content-Type": "text/plain; charset=utf-8", "Cache-Control": "no-store"},
		Body:       "Bad request\n",
	}
}
