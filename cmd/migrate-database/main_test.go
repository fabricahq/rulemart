package main

import (
	"context"
	"strings"
	"testing"
)

func env(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

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
	_, err := directConnString(context.Background(), env(map[string]string{"DATABASE_URL_PARAMETER": name}))
	if err == nil {
		t.Fatal("directConnString succeeded with an unreachable SSM endpoint")
	}
	if !strings.Contains(err.Error(), name) {
		t.Errorf("error %q doesn't name the parameter %q", err, name)
	}
}

// DATABASE_URL is used as given, so it must already be direct: migrating through Neon's pooler would lose the lock.
func TestDirectConnStringTakesDatabaseURLOnlyWhenDirect(t *testing.T) {
	const direct = "postgres://app:secret@ep-a.example.neon.tech/db"
	got, err := directConnString(context.Background(), env(map[string]string{"DATABASE_URL": direct}))
	if err != nil || got != direct {
		t.Fatalf("got %q, %v; want %q", got, err, direct)
	}

	const pooled = "postgres://app:secret@ep-a-pooler.example.neon.tech/db"
	got, err = directConnString(context.Background(), env(map[string]string{"DATABASE_URL": pooled}))
	if err == nil {
		t.Fatalf("accepted the pooled DATABASE_URL as %q", got)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Errorf("error %q repeats the connection string", err)
	}
}
