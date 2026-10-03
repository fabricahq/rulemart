// Package postgres stores accounts and their sessions in Postgres. Its SQL is in queries/, from which sqlc generates
// accountsdb.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/store"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/store/postgres/generated/accountsdb"
	"github.com/fabricahq/rulemart/internal/platform/database"
)

// Store is accounts in Postgres.
type Store struct {
	db *database.DB
}

// New returns a Store that reads and writes through db.
func New(db *database.DB) *Store {
	return &Store{db: db}
}

var _ store.Store = (*Store)(nil)

// SignIn records that identity signed in, in one transaction, as store.Store describes. It's safe to repeat after a
// failed connection, since the transaction either committed nothing or everything.
func (s *Store) SignIn(ctx context.Context, identity domain.Identity, tokenHash, replacing, gitHubToken []byte) (domain.Account, domain.Session, error) {
	var account domain.Account
	var session domain.Session
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			q := accountsdb.New(tx)
			row, err := q.UpsertAccount(ctx, accountsdb.UpsertAccountParams{
				GithubUserID: identity.GitHubUserID, GithubLogin: identity.Login, AvatarUrl: identity.AvatarURL,
				GithubName: identity.Name,
			})
			if err != nil {
				return fmt.Errorf("save account: %v", err)
			}
			account = newAccount(row.ID, row.GithubUserID, row.GithubLogin, row.AvatarUrl, row.GithubName, row.CreatedAt.Time)
			if replacing != nil {
				if _, err := q.DeleteSession(ctx, replacing); err != nil {
					return fmt.Errorf("end the replaced session: %v", err)
				}
			}
			if _, err := q.DeleteExpiredSessions(ctx); err != nil {
				return fmt.Errorf("end expired sessions: %v", err)
			}
			// The next page that shows the visitor's GitHub account reads it again, with this sign-in's token.
			if err := q.DeleteSnapshot(ctx, account.ID); err != nil {
				return fmt.Errorf("discard the GitHub snapshot: %v", err)
			}
			expires, err := q.CreateSession(ctx, accountsdb.CreateSessionParams{
				TokenHash: tokenHash, AccountID: account.ID, LifetimeSeconds: int64(domain.SessionLifetime.Seconds()),
				GithubToken: gitHubToken,
			})
			if err != nil {
				return fmt.Errorf("add session: %v", err)
			}
			session.ExpiresAt = expires.Time
			if _, err := q.DeleteOldestSessions(ctx, accountsdb.DeleteOldestSessionsParams{AccountID: account.ID, Keep: domain.MaxSessions}); err != nil {
				return fmt.Errorf("end the oldest sessions: %v", err)
			}
			return nil
		})
	})
	if err != nil {
		return domain.Account{}, domain.Session{}, fmt.Errorf("sign in githubUserID=%d: %v", identity.GitHubUserID, err)
	}
	return account, session, nil
}

// SessionAccount returns the account signed in with the session whose token hashes to tokenHash, or
// store.ErrNotFound.
func (s *Store) SessionAccount(ctx context.Context, tokenHash []byte) (domain.Account, error) {
	var row accountsdb.GetSessionAccountRow
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		var err error
		row, err = accountsdb.New(pool).GetSessionAccount(ctx, tokenHash)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, store.ErrNotFound
	}
	if err != nil {
		return domain.Account{}, fmt.Errorf("read session: %v", err)
	}
	return newAccount(row.ID, row.GithubUserID, row.GithubLogin, row.AvatarUrl, row.GithubName, row.CreatedAt.Time), nil
}

// newAccount returns the account a row of accounts describes.
func newAccount(id, gitHubUserID int64, login, avatarURL, name string, createdAt time.Time) domain.Account {
	return domain.Account{
		ID:        id,
		Identity:  domain.Identity{GitHubUserID: gitHubUserID, Login: login, AvatarURL: avatarURL, Name: name},
		CreatedAt: createdAt,
	}
}

// SessionGitHubToken returns the sealed GitHub token of the live session whose token hashes to tokenHash, nil when
// it keeps none, or store.ErrNotFound.
func (s *Store) SessionGitHubToken(ctx context.Context, tokenHash []byte) ([]byte, error) {
	var sealed []byte
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		var err error
		sealed, err = accountsdb.New(pool).GetSessionGitHubToken(ctx, tokenHash)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read the session's GitHub token: %v", err)
	}
	return sealed, nil
}

// EndSession ends the session whose token hashes to tokenHash, if there is one.
func (s *Store) EndSession(ctx context.Context, tokenHash []byte) error {
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		_, err := accountsdb.New(pool).DeleteSession(ctx, tokenHash)
		return err
	})
	if err != nil {
		return fmt.Errorf("end session: %v", err)
	}
	return nil
}

// EndSessions ends every session of the account the live session whose token hashes to tokenHash signs in, or
// fails with store.ErrNotFound.
func (s *Store) EndSessions(ctx context.Context, tokenHash []byte) error {
	var ended int64
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		var err error
		ended, err = accountsdb.New(pool).DeleteAccountSessions(ctx, tokenHash)
		return err
	})
	if err != nil {
		return fmt.Errorf("end the account's sessions: %v", err)
	}
	if ended == 0 {
		return store.ErrNotFound
	}
	return nil
}

// DeleteAccount deletes the account the live session whose token hashes to tokenHash signs in, and its sessions
// with it, or fails with store.ErrNotFound.
func (s *Store) DeleteAccount(ctx context.Context, tokenHash []byte) error {
	var deleted int64
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		var err error
		deleted, err = accountsdb.New(pool).DeleteAccount(ctx, tokenHash)
		return err
	})
	if err != nil {
		return fmt.Errorf("delete account: %v", err)
	}
	if deleted == 0 {
		return store.ErrNotFound
	}
	return nil
}
