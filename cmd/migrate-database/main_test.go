package main

import (
	"context"
	"strings"
	"testing"
)

// A failed SSM read names the parameter, so the release workflow's log shows which one to check.
func TestDirectConnStringNamesUnreadableParameter(t *testing.T) {
	t.Setenv("AWS_CONFIG_FILE", "/dev/null")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/dev/null")
	t.Setenv("AWS_REGION", "us-west-2")
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_MAX_ATTEMPTS", "1")
	t.Setenv("AWS_ENDPOINT_URL_SSM", "http://127.0.0.1:1")

	const name = "/rulemart/test/database-url"
	_, err := directConnString(context.Background(), "", name)
	if err == nil {
		t.Fatal("directConnString succeeded with an unreachable SSM endpoint")
	}
	if !strings.Contains(err.Error(), name) {
		t.Errorf("error %q doesn't name the parameter %q", err, name)
	}
}
