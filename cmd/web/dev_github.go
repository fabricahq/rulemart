//go:build rulemartdev

// A local build without GitHub sign-in reads a fake GitHub, so its test users have a dashboard: githubtest's DevFake,
// served in this process, with a GitHub App of its own, and a key for sessions' tokens when none is set.

package main

import (
	"cmp"
	"net/http/httptest"
	"os"

	accountsapp "github.com/fabricahq/rulemart/internal/contexts/accounts/app"
	accounts "github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/github"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/github/githubtest"
	"github.com/fabricahq/rulemart/internal/platform/secret"
)

// devGitHub serves DevFake, whose app's install page returns to this server's setup URL, and returns what reads it,
// with keys, or a new random key when keys is nil. Tokens sealed with a random key open only until the server stops.
func devGitHub(keys accountsapp.TokenKeys) (*github.API, *github.App, accountsapp.TokenKeys) {
	addr := cmp.Or(os.Getenv("ADDR"), "127.0.0.1:8080")
	fake := githubtest.DevFake("http://" + addr + "/me/github/installed")
	server := httptest.NewServer(fake.Handler())
	api := github.NewAPI(server.URL)
	app := github.NewApp(github.AppConfig{
		ID: 1, ClientID: fake.AppClientID, Slug: fake.AppSlug, WebURL: server.URL,
		PrivateKey: secret.FromValue(githubtest.AppKeyPEM(fake.AppKey)), WebhookSecret: secret.FromValue("dev-webhook-secret"),
	}, api)
	if keys == nil {
		keys = accountsapp.FixedTokenKey{Key: accounts.NewTokenKey()}
	}
	return api, app, keys
}
