package main

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/platform/web"
)

// Sign-in with GitHub needs both the OAuth app's client ID and its secret; half of them is a mistake to report at
// start, not a sign-in that fails for every visitor.
func TestNewGitHubNeedsTheClientIDAndSecretTogether(t *testing.T) {
	for name, tc := range map[string]struct {
		env     map[string]string
		want    bool
		wantErr string
	}{
		"neither":                    {nil, false, ""},
		"an ID and a secret":         {map[string]string{"GITHUB_CLIENT_ID": "id", "GITHUB_CLIENT_SECRET": "secret"}, true, ""},
		"an ID and a parameter":      {map[string]string{"GITHUB_CLIENT_ID": "id", "GITHUB_CLIENT_SECRET_PARAMETER": "/rulemart/x", "AWS_REGION": "us-west-2"}, true, ""},
		"an ID alone":                {map[string]string{"GITHUB_CLIENT_ID": "id"}, false, "set GITHUB_CLIENT_SECRET"},
		"a secret alone":             {map[string]string{"GITHUB_CLIENT_SECRET": "secret"}, false, "set GITHUB_CLIENT_ID"},
		"a secret and its parameter": {map[string]string{"GITHUB_CLIENT_ID": "id", "GITHUB_CLIENT_SECRET": "secret", "GITHUB_CLIENT_SECRET_PARAMETER": "/rulemart/x"}, false, "not both"},
	} {
		t.Run(name, func(t *testing.T) {
			client, err := newGitHub(context.Background(), func(name string) string { return tc.env[name] })
			if (client != nil) != tc.want || (err == nil) != (tc.wantErr == "") || (err != nil && !strings.Contains(err.Error(), tc.wantErr)) {
				t.Errorf("got %v, %v; want a client: %v, an error saying %q", client, err, tc.want, tc.wantErr)
			}
			if err != nil && strings.Contains(err.Error(), "secret\"") {
				t.Errorf("the error %q holds the secret", err)
			}
		})
	}
}

// On Lambda, GitHub sends visitors back to the public origin, so GitHub sign-in without one can't work.
func TestStartupOnLambdaRefusesGitHubSignInWithoutABaseURL(t *testing.T) {
	if web.DevSignIn {
		t.Skip("a build with the dev sign-in refuses Lambda first")
	}
	lines, code := runMain(t, "DATABASE_URL=postgres://localhost/rulemart", "AWS_LAMBDA_RUNTIME_API=127.0.0.1:1",
		"GITHUB_CLIENT_ID=id", "GITHUB_CLIENT_SECRET=secret", "TOKEN_KEY="+base64.StdEncoding.EncodeToString(make([]byte, 32)),
		"RULEMART_BASE_URL=")

	if code != 1 || len(lines) != 1 || lines[0]["msg"] != "startup failed" || !strings.Contains(lines[0]["error"].(string), "RULEMART_BASE_URL") {
		t.Fatalf("exited %d, logging %v", code, lines)
	}
}

// GitHub sign-in keeps each visitor's token, sealed, so it needs the key, and a key given directly that isn't 32 bytes
// of base64 is a mistake to report at start, not a sign-in that fails for every visitor. The error never repeats the
// key.
func TestNewTokenKeysNeedsAValidKeyForGitHubSignIn(t *testing.T) {
	valid := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	for name, tc := range map[string]struct {
		env          map[string]string
		gitHubSignIn bool
		want         bool
		wantErr      string
	}{
		"no sign-in and no key":    {nil, false, false, ""},
		"sign-in without a key":    {nil, true, false, "set TOKEN_KEY or TOKEN_KEY_PARAMETER"},
		"sign-in with a key":       {map[string]string{"TOKEN_KEY": valid}, true, true, ""},
		"sign-in with a parameter": {map[string]string{"TOKEN_KEY_PARAMETER": "/rulemart/x", "AWS_REGION": "us-west-2"}, true, true, ""},
		"a key that's too short":   {map[string]string{"TOKEN_KEY": "c2hvcnQ="}, true, false, "32 random bytes"},
		"a key and its parameter":  {map[string]string{"TOKEN_KEY": valid, "TOKEN_KEY_PARAMETER": "/rulemart/x"}, true, false, "not both"},
	} {
		t.Run(name, func(t *testing.T) {
			keys, err := newTokenKeys(context.Background(), func(name string) string { return tc.env[name] }, tc.gitHubSignIn)
			if (keys != nil) != tc.want || (err == nil) != (tc.wantErr == "") || (err != nil && !strings.Contains(err.Error(), tc.wantErr)) {
				t.Errorf("got %v, %v; want keys: %v, an error saying %q", keys, err, tc.want, tc.wantErr)
			}
			if err != nil && tc.env["TOKEN_KEY"] != "" && strings.Contains(err.Error(), tc.env["TOKEN_KEY"]) {
				t.Errorf("the error %q holds the key", err)
			}
		})
	}
}
