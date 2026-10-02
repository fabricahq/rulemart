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
