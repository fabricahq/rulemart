// Package store is the persistence contract of accounts: what signing in and out needs from storage. store/postgres
// implements it.
package store

import (
	"context"
	"errors"

	"github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
)

// Store keeps accounts and their sessions. It finds a session by its token's hash, never the token.
type Store interface {
	// SignIn records that identity signed in, in one transaction: it adds the identity's account, or updates its login
	// and avatar; ends the session whose token hashes to replacing, if there is one, since the browser that held it
	// is signing in again; ends every expired session; and adds a session for the account whose token hashes to
	// tokenHash, lasting domain.SessionLifetime, keeping the account's newest domain.MaxSessions. It returns the
	// account and when the new session expires.
	SignIn(ctx context.Context, identity domain.Identity, tokenHash, replacing []byte) (domain.Account, domain.Session, error)
	// SessionAccount returns the account signed in with the session whose token hashes to tokenHash, or ErrNotFound
	// when there's no such session or it has expired.
	SessionAccount(ctx context.Context, tokenHash []byte) (domain.Account, error)
	// EndSession ends the session whose token hashes to tokenHash. Ending one that doesn't exist does nothing.
	EndSession(ctx context.Context, tokenHash []byte) error
	// EndSessions ends every session of the account accountID.
	EndSessions(ctx context.Context, accountID int64) error
	// DeleteAccount deletes the account accountID and ends its sessions. Deleting one that doesn't exist does
	// nothing.
	DeleteAccount(ctx context.Context, accountID int64) error
}

// ErrNotFound reports a session that doesn't exist or has expired.
var ErrNotFound = errors.New("not found")
