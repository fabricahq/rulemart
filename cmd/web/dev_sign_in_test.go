//go:build rulemartdev

package main

import (
	"strings"
	"testing"
)

// A build with the dev sign-in lets anyone sign in as a test user, so it refuses to serve on Lambda.
func TestStartupOnLambdaRefusesABuildWithTheDevSignIn(t *testing.T) {
	lines, code := runMain(t, "DATABASE_URL=postgres://localhost/rulemart", "AWS_LAMBDA_RUNTIME_API=127.0.0.1:1")

	if code != 1 || len(lines) != 1 || lines[0]["msg"] != "startup failed" || !strings.Contains(lines[0]["error"].(string), "rulemartdev") {
		t.Fatalf("exited %d, logging %v", code, lines)
	}
}

// A local build without RULEMART_BASE_URL names the loopback address it serves at, so robots.txt names the sitemap
// and the sitemap answers, as in production; an address another machine could reach gets none.
func TestLocalBuildNamesItsLoopbackAddressAsBaseURL(t *testing.T) {
	for addr, want := range map[string]string{
		"":               "http://127.0.0.1:8080",
		"localhost:9000": "http://localhost:9000",
		"[::1]:8080":     "http://[::1]:8080",
		"0.0.0.0:8080":   "",
		":8080":          "",
	} {
		t.Run(addr, func(t *testing.T) {
			got, err := newBaseURL(env(map[string]string{"ADDR": addr}))

			if err != nil || (got == nil) != (want == "") || (got != nil && got.String() != want) {
				t.Fatalf("got %v, %v, want %q", got, err, want)
			}
		})
	}
}
