// Package store is the persistence contract of accounts: what signing in and out needs from storage. store/postgres
// implements it.
package store

import (
	"context"
	"errors"
	"time"

	"github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
)

// Store keeps accounts and their sessions. It finds a session by its token's hash, never the token.
type Store interface {
	// SignIn records that identity signed in, in one transaction: it adds the identity's account, or updates its login,
	// avatar, and name; ends the session whose token hashes to replacing, if there is one, since the browser that held
	// it is signing in again; ends every expired session; discards the account's GitHub snapshot, so the next page that
	// shows it reads GitHub again with the new session's token; and adds a session for the account whose token hashes
	// to tokenHash, lasting domain.SessionLifetime, keeping gitHubToken, the session's sealed GitHub token, or none
	// when it's nil, and keeping the account's newest domain.MaxSessions. It returns the account and when the new
	// session expires.
	SignIn(ctx context.Context, identity domain.Identity, tokenHash, replacing, gitHubToken []byte) (domain.Account, domain.Session, error)
	// SessionAccount returns the account signed in with the session whose token hashes to tokenHash, or ErrNotFound
	// when there's no such session or it has expired.
	SessionAccount(ctx context.Context, tokenHash []byte) (domain.Account, error)
	// SessionGitHubToken returns the sealed GitHub token of the session whose token hashes to tokenHash, or nil when it
	// keeps none, or fails with ErrNotFound when there's no such session or it has expired.
	SessionGitHubToken(ctx context.Context, tokenHash []byte) ([]byte, error)
	// EndSession ends the session whose token hashes to tokenHash, and deletes its GitHub token with it. Ending one that
	// doesn't exist does nothing.
	EndSession(ctx context.Context, tokenHash []byte) error
	// EndSessions ends every session of the account signed in with the session whose token hashes to tokenHash, or
	// fails with ErrNotFound when that session has ended or expired. It reads the account and ends its sessions in one
	// statement, so only a session that's live when the write happens can end them.
	EndSessions(ctx context.Context, tokenHash []byte) error
	// DeleteAccount deletes the account signed in with the session whose token hashes to tokenHash, which ends its
	// sessions, or fails with ErrNotFound when that session has ended or expired, in one statement as EndSessions.
	DeleteAccount(ctx context.Context, tokenHash []byte) error

	// Snapshot returns the account's GitHub snapshot, or found false when it has none.
	Snapshot(ctx context.Context, accountID int64) (snapshot domain.Snapshot, found bool, err error)
	// ClaimRead claims a read of the account's GitHub account beginning at now, unless one began within interval before
	// now and its snapshot hasn't been discarded since, so of requests that arrive together, one reads GitHub. In the
	// same transaction it returns the account's GitHub generation and snapshot as of the claim, which no discard can
	// come between.
	ClaimRead(ctx context.Context, accountID int64, now time.Time, interval time.Duration) (ReadClaim, error)
	// GitHubGeneration returns how many times the account's snapshot has been discarded, such as when its access to
	// private repositories changed. A read notes it before it reads GitHub, and saves what it found with it.
	GitHubGeneration(ctx context.Context, accountID int64) (int64, error)
	// SaveSnapshot keeps snapshot as the account's, replacing the one it had, unless the account's GitHub generation is
	// no longer generation: its snapshot was discarded since the read began, and the read may hold what the account can
	// no longer see. It returns whether it kept the snapshot.
	SaveSnapshot(ctx context.Context, accountID, generation int64, snapshot domain.Snapshot) (bool, error)
	// Installations returns the installations of the GitHub App the account reads private repositories through, in
	// the order it added them.
	Installations(ctx context.Context, accountID int64) ([]domain.Installation, error)
	// AddInstallation records that the account reads private repositories through installation, and discards its
	// snapshot, in one transaction. Adding one it has changes nothing, and keeps the snapshot.
	AddInstallation(ctx context.Context, accountID int64, installation domain.Installation) error
	// RemoveInstallations forgets every installation the account reads through, and discards its snapshot, in one
	// transaction.
	RemoveInstallations(ctx context.Context, accountID int64) error
	// RemoveInstallation forgets that the account reads through the installation id, and discards its snapshot, in one
	// transaction. Other accounts that read through it keep it.
	RemoveInstallation(ctx context.Context, accountID, id int64) error
	// InstallationRemoved forgets the installation id for every account, and discards their snapshots, in one
	// transaction.
	InstallationRemoved(ctx context.Context, id int64) error
	// InstallationChanged discards the snapshots of the accounts that read through the installation id.
	InstallationChanged(ctx context.Context, id int64) error
}

// ReadClaim is what ClaimRead found as it claimed a read, or declined to.
type ReadClaim struct {
	// Claimed is whether the read is the caller's to make: false when another read began within the interval, which
	// may still be under way.
	Claimed bool
	// Generation is the account's GitHub generation as the read was claimed, which the read saves what it finds with;
	// zero when it wasn't claimed.
	Generation int64
	// Snapshot is the account's snapshot as of the claim, empty when it has none: what a failed read keeps, saying
	// so, and what a request that didn't claim the read shows.
	Snapshot domain.Snapshot
}

// ErrNotFound reports a session that doesn't exist or has expired.
var ErrNotFound = errors.New("not found")
