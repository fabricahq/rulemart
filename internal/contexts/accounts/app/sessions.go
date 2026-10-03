// Package app holds the operations of accounts: signing a visitor in once GitHub has said who they are, finding who
// a browser's session signs in, signing out, and deleting an account.
package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/store"
)

// ErrSignedOut reports a session token that signs no one in: one that never did, or whose session has ended or
// expired.
var ErrSignedOut = store.ErrNotFound

// ErrNoGitHubToken reports a session that keeps no GitHub token Rulemart can use: it signed in without one, such as a
// local build's test user, or its token was sealed under a key Rulemart no longer has. Signing in again gives it one.
var ErrNoGitHubToken = errors.New("the session keeps no GitHub token Rulemart can open")

// TokenKeys gives the key that seals sessions' GitHub tokens, such as from an SSM parameter.
type TokenKeys interface {
	TokenKey(ctx context.Context) (domain.TokenKey, error)
}

// FixedTokenKey gives one key that never changes, such as a local build's random one.
type FixedTokenKey struct{ Key domain.TokenKey }

// TokenKey returns k's key.
func (k FixedTokenKey) TokenKey(context.Context) (domain.TokenKey, error) { return k.Key, nil }

// Sessions signs visitors in and out.
type Sessions struct {
	Store store.Store
	// TokenKeys seals the GitHub token each session keeps. Nil keeps none, which only a server without GitHub sign-in
	// may do.
	TokenKeys TokenKeys
}

// SignIn signs identity in with a new session, ending the session replacing, the token the browser held before, if
// any, so a token planted in a browser before sign-in never signs anyone in. The session keeps gitHubToken, the
// visitor's OAuth token, sealed for it alone, or none when it's empty. It returns the account and the session, whose
// token the browser keeps.
func (s Sessions) SignIn(ctx context.Context, identity domain.Identity, gitHubToken string, replacing domain.SessionToken) (domain.Account, domain.Session, error) {
	token := domain.NewSessionToken()
	var replacingHash []byte
	if replacing != "" {
		replacingHash = replacing.Hash()
	}
	sealed, err := s.seal(ctx, gitHubToken, token.Hash())
	if err != nil {
		return domain.Account{}, domain.Session{}, fmt.Errorf("sign in githubUserID=%d: %v", identity.GitHubUserID, err)
	}
	account, session, err := s.Store.SignIn(ctx, identity, token.Hash(), replacingHash, sealed)
	if err != nil {
		return domain.Account{}, domain.Session{}, err
	}
	session.Token = token
	return account, session, nil
}

// seal returns gitHubToken sealed for the session whose token hashes to sessionHash, or nil for no token.
func (s Sessions) seal(ctx context.Context, gitHubToken string, sessionHash []byte) ([]byte, error) {
	if gitHubToken == "" {
		return nil, nil
	}
	if s.TokenKeys == nil {
		return nil, errors.New("keep the GitHub token: no key seals it")
	}
	key, err := s.TokenKeys.TokenKey(ctx)
	if err != nil {
		return nil, fmt.Errorf("keep the GitHub token: %v", err)
	}
	return key.Seal(gitHubToken, sessionHash)
}

// GitHubToken returns the GitHub token token's session keeps, or fails with ErrNoGitHubToken when it keeps none
// Rulemart can open, or ErrSignedOut when the session has ended.
func (s Sessions) GitHubToken(ctx context.Context, token domain.SessionToken) (string, error) {
	sealed, err := s.Store.SessionGitHubToken(ctx, token.Hash())
	if err != nil {
		return "", err
	}
	if sealed == nil || s.TokenKeys == nil {
		return "", ErrNoGitHubToken
	}
	key, err := s.TokenKeys.TokenKey(ctx)
	if err != nil {
		return "", fmt.Errorf("open the session's GitHub token: %v", err)
	}
	gitHubToken, err := key.Open(sealed, token.Hash())
	if errors.Is(err, domain.ErrTokenUnreadable) {
		return "", ErrNoGitHubToken
	}
	return gitHubToken, err
}

// Account returns the account token signs in, or ErrSignedOut.
func (s Sessions) Account(ctx context.Context, token domain.SessionToken) (domain.Account, error) {
	return s.Store.SessionAccount(ctx, token.Hash())
}

// SignOut ends token's session.
func (s Sessions) SignOut(ctx context.Context, token domain.SessionToken) error {
	return s.Store.EndSession(ctx, token.Hash())
}

// SignOutEverywhere ends every session, in every browser, of the account token signs in, or fails with ErrSignedOut
// when token's session has ended by then.
func (s Sessions) SignOutEverywhere(ctx context.Context, token domain.SessionToken) error {
	return s.Store.EndSessions(ctx, token.Hash())
}

// DeleteAccount deletes the account token signs in, which ends its sessions, or fails with ErrSignedOut when
// token's session has ended by then.
func (s Sessions) DeleteAccount(ctx context.Context, token domain.SessionToken) error {
	return s.Store.DeleteAccount(ctx, token.Hash())
}
