package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/lib/githubapp/githubapptest"
)

// appGitHub serves a fake GitHub for the app 123, installed on fabricahq as installation 9 and on octo-org as 10,
// and returns its URL and the app's private key, in PEM.
func appGitHub(t *testing.T) (string, string) {
	t.Helper()
	fake := &githubapptest.Fake{
		Issuer: "123", Key: githubapptest.NewKey(),
		Installations: []githubapptest.Installation{
			{ID: 9, Account: "fabricahq", Organization: true},
			{ID: 10, Account: "octo-org", Organization: true},
		},
	}
	server := httptest.NewServer(fake.Handler())
	t.Cleanup(server.Close)
	return server.URL, githubapptest.KeyPEM(fake.Key)
}

// With the app's variables, the worker's lookups carry its installation's tokens, and the personal token, which the
// launch configuration may still set, is ignored; without them, the personal token, if any, as before.
func TestTheWorkerAuthenticatesAsTheAppWhenItsVariablesAreSet(t *testing.T) {
	const key = "the app's key"
	for name, tc := range map[string]struct {
		// env holds the variables, with key standing for the app's private key.
		env   map[string]string
		token string
		attrs []any
	}{
		"the app, on fabricahq by default": {
			env:   map[string]string{"GITHUB_APP_ID": "123", "GITHUB_APP_PRIVATE_KEY": key, "GITHUB_TOKEN": "github_pat_x"},
			token: githubapptest.Token(9, 1),
			attrs: []any{"github_auth", "app", "github_app_id", "123", "installation_account", "fabricahq"},
		},
		"the app, on the account named": {
			env:   map[string]string{"GITHUB_APP_ID": "123", "GITHUB_APP_PRIVATE_KEY": key, "GITHUB_APP_INSTALLATION_ACCOUNT": "octo-org"},
			token: githubapptest.Token(10, 1),
			attrs: []any{"github_auth", "app", "github_app_id", "123", "installation_account", "octo-org"},
		},
		"a personal token": {
			env:   map[string]string{"GITHUB_TOKEN": "github_pat_x"},
			token: "github_pat_x",
			attrs: []any{"github_auth", "token"},
		},
		"nothing": {attrs: []any{"github_auth", "none"}},
	} {
		t.Run(name, func(t *testing.T) {
			apiURL, appKey := appGitHub(t)
			if tc.env["GITHUB_APP_PRIVATE_KEY"] == key {
				tc.env["GITHUB_APP_PRIVATE_KEY"] = appKey
			}
			auth, err := newGitHubAuth(context.Background(), env(tc.env), apiURL, http.DefaultClient)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(auth.attrs, tc.attrs) {
				t.Errorf("logs %v, want %v", auth.attrs, tc.attrs)
			}
			if tc.token == "" {
				if auth.token != nil {
					t.Errorf("got a token, want none")
				}
				return
			}
			if got, err := auth.token.Value(context.Background()); err != nil || got != tc.token {
				t.Errorf("got the token %q, %v; want %q", got, err, tc.token)
			}
		})
	}
}

// Half of the app's variables, or a value that can't name the app or an account, stops the worker at start, naming
// the variable and never the key.
func TestTheWorkerRefusesHalfOfTheAppsVariables(t *testing.T) {
	const key = "-----BEGIN RSA PRIVATE KEY-----\nsecret\n-----END RSA PRIVATE KEY-----\n"
	for name, tc := range map[string]struct {
		env  map[string]string
		want string
	}{
		"the ID alone":              {map[string]string{"GITHUB_APP_ID": "123"}, "together"},
		"the key alone":             {map[string]string{"GITHUB_APP_PRIVATE_KEY": key}, "together"},
		"the key's parameter alone": {map[string]string{"GITHUB_APP_PRIVATE_KEY_PARAMETER": "/rulemart/prod/github-app-key"}, "together"},
		"the account alone":         {map[string]string{"GITHUB_APP_INSTALLATION_ACCOUNT": "fabricahq", "GITHUB_TOKEN": "github_pat_x"}, "together"},
		"an ID that isn't a number": {map[string]string{"GITHUB_APP_ID": "rulemart", "GITHUB_APP_PRIVATE_KEY": key}, "GITHUB_APP_ID"},
		"an ID of zero":             {map[string]string{"GITHUB_APP_ID": "0", "GITHUB_APP_PRIVATE_KEY": key}, "GITHUB_APP_ID"},
		"an account with a slash":   {map[string]string{"GITHUB_APP_ID": "123", "GITHUB_APP_PRIVATE_KEY": key, "GITHUB_APP_INSTALLATION_ACCOUNT": "fabricahq/rulemart"}, "GITHUB_APP_INSTALLATION_ACCOUNT"},
		"the key and its parameter": {map[string]string{"GITHUB_APP_ID": "123", "GITHUB_APP_PRIVATE_KEY": key, "GITHUB_APP_PRIVATE_KEY_PARAMETER": "/rulemart/prod/github-app-key"}, "not both"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := newGitHubAuth(context.Background(), env(tc.env), "http://127.0.0.1:1", http.DefaultClient)
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "secret") {
				t.Errorf("got %v, want an error with %q and without the key", err, tc.want)
			}
		})
	}
}

// env returns a getenv that reads vars.
func env(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

// The worker says once at start, after it's ready, which authentication its GitHub lookups use, and never logs the
// key or a token. The runtime API here is unreachable, so the worker stops right after it starts.
func TestStartupLogsTheWorkersGitHubAuthenticationWithoutSecrets(t *testing.T) {
	key := githubapptest.KeyPEM(githubapptest.NewKey())
	lines, _ := runMain(t, "AWS_LAMBDA_RUNTIME_API=127.0.0.1:1",
		"QUEUE_URL=https://sqs.us-west-2.amazonaws.com/123456789012/jobs", "AWS_REGION=us-west-2",
		"AWS_CONFIG_FILE=/dev/null", "AWS_SHARED_CREDENTIALS_FILE=/dev/null",
		"DATABASE_URL=postgres://rulemart@db.example/rulemart", "DATABASE_URL_PARAMETER=",
		"GITHUB_APP_ID=123", "GITHUB_APP_PRIVATE_KEY="+key, "GITHUB_APP_PRIVATE_KEY_PARAMETER=",
		"GITHUB_APP_INSTALLATION_ACCOUNT=", "GITHUB_TOKEN=github_pat_secret", "GITHUB_TOKEN_PARAMETER=")

	var auth []map[string]any
	for _, line := range lines {
		if line["msg"] == "github authentication" {
			auth = append(auth, line)
		}
		for field, value := range line {
			if s, ok := value.(string); ok && (strings.Contains(s, "PRIVATE KEY") || strings.Contains(s, "github_pat_secret")) {
				t.Fatalf("%s logged a secret: %v", field, line)
			}
		}
	}
	if len(lines) < 2 || lines[0]["msg"] != "ready" || len(auth) != 1 {
		t.Fatalf("logged %v, want ready, then one line for GitHub's authentication", lines)
	}
	if auth[0]["github_auth"] != "app" || auth[0]["github_app_id"] != "123" || auth[0]["installation_account"] != "fabricahq" {
		t.Errorf("logged %v, want the app 123 on fabricahq", auth[0])
	}
}

// Half of the app's variables stops the worker before it's ready.
func TestStartupRefusesHalfOfTheAppsVariables(t *testing.T) {
	lines, code := runMain(t, "DATABASE_URL=postgres://rulemart@db.example/rulemart", "DATABASE_URL_PARAMETER=",
		"GITHUB_APP_ID=123", "GITHUB_APP_PRIVATE_KEY=", "GITHUB_APP_PRIVATE_KEY_PARAMETER=")

	if code != 1 || len(lines) != 1 || lines[0]["msg"] != "startup failed" || !strings.Contains(lines[0]["error"].(string), "GITHUB_APP_ID") {
		t.Fatalf("exited %d and logged %v, want a failed start naming the app's variables", code, lines)
	}
}
