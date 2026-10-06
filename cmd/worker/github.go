// Choose how the worker authenticates its GitHub lookups: as the GitHub App, with a personal access token, or not at
// all.

package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/github"
	"github.com/fabricahq/rulemart/internal/lib/githubapp"
	"github.com/fabricahq/rulemart/internal/platform/secret"
)

// defaultInstallationAccount is the account whose installation of the GitHub App the worker uses unless
// GITHUB_APP_INSTALLATION_ACCOUNT names another.
const defaultInstallationAccount = "fabricahq"

// gitHubAuth is how the worker authenticates its GitHub lookups.
type gitHubAuth struct {
	// token gives each lookup's token, or is nil when the worker looks repositories up without one.
	token github.Token
	// attrs say which authentication the worker uses, for its log, and never hold a secret.
	attrs []any
}

// newGitHubAuth returns the worker's GitHub authentication, from the variables getenv reads, such as os.Getenv:
//
//   - With GITHUB_APP_ID and GITHUB_APP_PRIVATE_KEY, or GITHUB_APP_PRIVATE_KEY_PARAMETER, the GitHub App's
//     installation tokens, for its installation on the account GITHUB_APP_INSTALLATION_ACCOUNT names, fabricahq unless
//     set. The app reads GitHub's API at apiURL through client. It ignores GITHUB_TOKEN and its _PARAMETER.
//   - Otherwise, the token GITHUB_TOKEN, or GITHUB_TOKEN_PARAMETER, holds.
//   - Otherwise, none.
//
// Half of the app's variables is a mistake to report at start.
func newGitHubAuth(ctx context.Context, getenv func(string) string, apiURL string, client *http.Client) (gitHubAuth, error) {
	key, err := secret.FromEnv(ctx, getenv, "GITHUB_APP_PRIVATE_KEY")
	if err != nil {
		return gitHubAuth{}, err
	}
	id, account := getenv("GITHUB_APP_ID"), getenv("GITHUB_APP_INSTALLATION_ACCOUNT")
	switch {
	case id == "" && key == nil && account == "":
		return personalToken(ctx, getenv)
	case id == "" || key == nil:
		return gitHubAuth{}, errors.New("set GITHUB_APP_ID and GITHUB_APP_PRIVATE_KEY or its _PARAMETER together, with GITHUB_APP_INSTALLATION_ACCOUNT optionally, or none of them")
	}
	if appID, err := strconv.ParseInt(id, 10, 64); err != nil || appID <= 0 {
		return gitHubAuth{}, fmt.Errorf("read GITHUB_APP_ID: want the app's numeric ID, got %q", id)
	}
	account = cmp.Or(account, defaultInstallationAccount)
	if !validLogin.MatchString(account) {
		return gitHubAuth{}, fmt.Errorf("read GITHUB_APP_INSTALLATION_ACCOUNT: want a GitHub account's login, such as fabricahq, got %q", account)
	}
	app := githubapp.New(githubapp.Config{Issuer: id, PrivateKey: key, BaseURL: apiURL, HTTP: client})
	return gitHubAuth{
		token: app.TokenSource(account),
		attrs: []any{"github_auth", "app", "github_app_id", id, "installation_account", account},
	}, nil
}

// personalToken returns the authentication of the token GITHUB_TOKEN or GITHUB_TOKEN_PARAMETER holds, or none.
func personalToken(ctx context.Context, getenv func(string) string) (gitHubAuth, error) {
	token, err := secret.FromEnv(ctx, getenv, "GITHUB_TOKEN")
	switch {
	case err != nil:
		return gitHubAuth{}, err
	case token == nil:
		// A nil *secret.Secret in the interface would be a token that panics, so none stays a nil interface.
		return gitHubAuth{attrs: []any{"github_auth", "none"}}, nil
	}
	return gitHubAuth{token: token, attrs: []any{"github_auth", "token"}}, nil
}

// validLogin matches a GitHub account's login: letters, digits, and single hyphens between them, at most 39.
var validLogin = regexp.MustCompile(`^[A-Za-z0-9](?:-?[A-Za-z0-9]){0,38}$`)
