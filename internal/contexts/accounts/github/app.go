// Act as the GitHub App "Rulemart by Fabrica", which reads the private repositories visitors install it on: through
// internal/lib/githubapp, it reads an installation's account, and gets an installation's token, which reads the
// repositories the visitor chose.

package github

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/lib/githubapp"
)

// AppConfig names the GitHub App and holds what it signs and checks with.
type AppConfig struct {
	// ID is GitHub's numeric ID for the app, which webhook deliveries name. ClientID is its client ID, which its JWTs
	// name as their issuer, as GitHub advises. Slug is the app's name in its URLs, such as rulemart-by-fabrica.
	ID       int64
	ClientID string
	Slug     string
	// WebURL is GitHub's site, where the app's install page is: https://github.com, or a fake's, when empty the former.
	WebURL string
	// PrivateKey holds the app's private key, in PEM, as GitHub gives it. WebhookSecret holds the secret its webhook's
	// deliveries are signed with.
	PrivateKey, WebhookSecret Secret
}

// App acts as one GitHub App, through api.
type App struct {
	config AppConfig
	api    *API
	// app signs the app's own requests, which api's base URL and client send.
	app *githubapp.App
}

// NewApp returns the app config describes, which reads GitHub through api.
func NewApp(config AppConfig, api *API) *App {
	a := &App{config: config, api: api}
	if api != nil {
		a.app = githubapp.New(githubapp.Config{Issuer: config.ClientID, PrivateKey: config.PrivateKey, BaseURL: api.baseURL, HTTP: api.http})
	}
	return a
}

// InstallURL returns the GitHub page where a visitor installs the app on the repositories they choose.
func (a *App) InstallURL() string {
	return cmp.Or(a.config.WebURL, "https://github.com") + "/apps/" + a.config.Slug + "/installations/new"
}

// Installation returns the GitHub account installation id is on, or fails with domain.ErrNoSuchInstallation.
func (a *App) Installation(ctx context.Context, id int64) (domain.InstallationAccount, error) {
	installation, err := a.app.Installation(ctx, id)
	if err != nil {
		return domain.InstallationAccount{}, installationError(err)
	}
	account := installation.Account
	return domain.InstallationAccount{Login: account.Login, ID: account.ID, Organization: account.Organization}, nil
}

// InstallationState returns what GitHub says of installation id now: gone, suspended, or active. Only a 404 says
// it's gone; any other failure is an error.
func (a *App) InstallationState(ctx context.Context, id int64) (domain.InstallationState, error) {
	installation, err := a.app.Installation(ctx, id)
	switch {
	case errors.Is(err, githubapp.ErrNoSuchInstallation):
		return domain.InstallationGone, nil
	case err != nil:
		return 0, fmt.Errorf("read the state of installation id=%d: %w", id, err)
	case installation.Suspended:
		return domain.InstallationSuspended, nil
	}
	return domain.InstallationActive, nil
}

// InstallationToken returns a token that reads the repositories installation id may, for an hour, or fails with
// domain.ErrNoSuchInstallation.
func (a *App) InstallationToken(ctx context.Context, id int64) (string, error) {
	token, err := a.app.InstallationToken(ctx, id)
	if err != nil {
		return "", installationError(err)
	}
	return token.Value, nil
}

// installationError returns err, a failure of the app's request, as accounts report it: with
// domain.ErrNoSuchInstallation for githubapp.ErrNoSuchInstallation.
func installationError(err error) error {
	if errors.Is(err, githubapp.ErrNoSuchInstallation) {
		return fmt.Errorf("%v: %w", err, domain.ErrNoSuchInstallation)
	}
	return err
}

// maxInstallationPages bounds the pages of an installation's repositories a read lists: GitHub lists them in an order of
// its own, which can't be sorted, so a read lists them all, up to this budget, to find the most recently pushed.
const maxInstallationPages = 10

// InstallationRepositories returns the private repositories an installation's token reads, the most recently pushed
// first, at most limit of them; the visitor's own token reads the public ones. more reports that it left some out:
// there were more than limit, or more than maxInstallationPages pages to list.
func (a *App) InstallationRepositories(ctx context.Context, token string, limit int) (repos []domain.GitHubRepository, more bool, err error) {
	private := func(r domain.GitHubRepository) bool { return r.Private }
	repos, more, err = a.api.repositories(ctx, token, "/installation/repositories?", "repositories", maxInstallationPages*perPage, maxInstallationPages, private)
	if err != nil {
		return nil, false, fmt.Errorf("list the installation's repositories: %w", err)
	}
	slices.SortStableFunc(repos, func(a, b domain.GitHubRepository) int { return b.PushedAt.Compare(a.PushedAt) })
	if len(repos) > limit {
		repos, more = repos[:limit], true
	}
	return repos, more, nil
}
