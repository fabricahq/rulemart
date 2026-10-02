// Package app holds the operations of accounts: signing a visitor in once GitHub has said who they are, finding who
// a browser's session signs in, signing out, and deleting an account.
package app

import (
	"context"

	"github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/store"
)

// ErrSignedOut reports a session token that signs no one in: one that never did, or whose session has ended or
// expired.
var ErrSignedOut = store.ErrNotFound

// Sessions signs visitors in and out.
type Sessions struct {
	Store store.Store
}

// SignIn signs identity in with a new session, ending the session replacing, the token the browser held before, if
// any, so a token planted in a browser before sign-in never signs anyone in. It returns the account and the session,
// whose token the browser keeps.
func (s Sessions) SignIn(ctx context.Context, identity domain.Identity, replacing domain.SessionToken) (domain.Account, domain.Session, error) {
	token := domain.NewSessionToken()
	var replacingHash []byte
	if replacing != "" {
		replacingHash = replacing.Hash()
	}
	account, session, err := s.Store.SignIn(ctx, identity, token.Hash(), replacingHash)
	if err != nil {
		return domain.Account{}, domain.Session{}, err
	}
	session.Token = token
	return account, session, nil
}

// Account returns the account token signs in, or ErrSignedOut.
func (s Sessions) Account(ctx context.Context, token domain.SessionToken) (domain.Account, error) {
	return s.Store.SessionAccount(ctx, token.Hash())
}

// SignOut ends token's session.
func (s Sessions) SignOut(ctx context.Context, token domain.SessionToken) error {
	return s.Store.EndSession(ctx, token.Hash())
}

// SignOutEverywhere ends every session of the account accountID, in every browser.
func (s Sessions) SignOutEverywhere(ctx context.Context, accountID int64) error {
	return s.Store.EndSessions(ctx, accountID)
}

// DeleteAccount deletes the account accountID and ends its sessions.
func (s Sessions) DeleteAccount(ctx context.Context, accountID int64) error {
	return s.Store.DeleteAccount(ctx, accountID)
}
