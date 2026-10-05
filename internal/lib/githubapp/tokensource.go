// Keep an installation's token, and mint the next one before it expires.

package githubapp

import (
	"context"
	"errors"
	"sync"
	"time"
)

// refreshBefore is how long before a token expires the source mints the next one, so requests never carry a token
// about to expire, and a failed mint leaves time to try again.
const refreshBefore = 5 * time.Minute

// minimumLife is the least a token must have left for the source to keep giving it after a failed mint, so a request
// that carries it finishes before it expires.
const minimumLife = time.Minute

// TokenSource gives the tokens of the app's installation on one account, as requests that act for it need them. It
// finds the installation on first use, mints a token, and keeps it until refreshBefore of its hour is left, then mints
// the next. Its methods are safe for concurrent use: callers that need a new token at once wait for one mint.
type TokenSource struct {
	app *App
	// account is the login of the account the installation is on.
	account string

	mu sync.Mutex
	// installation is the installation's ID, or 0 until the source finds it, and after GitHub said it has none.
	installation int64
	// token is the last token minted, or the zero Token before the first, and after Forget.
	token Token
}

// TokenSource returns a source of tokens for the app's installation on the account login, an organization's or a
// user's. It reads GitHub on first use.
func (a *App) TokenSource(login string) *TokenSource {
	return &TokenSource{app: a, account: login}
}

// Value returns a token for the installation: the one the source keeps, while more than refreshBefore of it is left,
// or a new one. When minting fails, it returns the token it keeps while more than minimumLife of it is left, and fails
// after that, with ErrNoSuchInstallation when GitHub has no installation on the account, or refuses it a token.
func (s *TokenSource) Value(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.app.now()
	if s.token.Value != "" && now.Before(s.token.ExpiresAt.Add(-refreshBefore)) {
		return s.token.Value, nil
	}
	token, err := s.mint(ctx)
	switch {
	case err == nil:
		s.token = token
		return token.Value, nil
	case s.token.Value != "" && now.Before(s.token.ExpiresAt.Add(-minimumLife)):
		return s.token.Value, nil
	}
	return "", err
}

// Forget drops the token the source keeps, such as one GitHub refused, so the next Value mints another.
func (s *TokenSource) Forget() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = Token{}
}

// mint finds the installation, unless the source knows it, and mints a token for it. When GitHub has no such
// installation, as after the app was reinstalled, it forgets the installation, so the next mint finds it again.
func (s *TokenSource) mint(ctx context.Context) (Token, error) {
	if s.installation == 0 {
		installation, err := s.app.AccountInstallation(ctx, s.account)
		if err != nil {
			return Token{}, err
		}
		s.installation = installation.ID
	}
	token, err := s.app.InstallationToken(ctx, s.installation)
	if errors.Is(err, ErrNoSuchInstallation) {
		s.installation = 0
	}
	return token, err
}
