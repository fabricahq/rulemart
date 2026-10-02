package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/store"
	"github.com/fabricahq/rulemart/internal/platform/database/databasetest"
	"github.com/fabricahq/rulemart/internal/platform/postgrestest"
)

// newStore returns a Store on a new migrated database, connected as the web function's role, so a missing grant fails
// the test, and the database's connection string as its owner, to arrange what the web role can't.
func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	_, connString := databasetest.New(t)
	return New(databasetest.AsWebRole(t, connString)), connString
}

var octocat = domain.Identity{GitHubUserID: 583231, Login: "octocat", AvatarURL: "https://avatars.githubusercontent.com/u/583231?v=4"}

// signIn signs identity in with a new token, ending replacing's session, and returns the token.
func signIn(t *testing.T, s *Store, identity domain.Identity, replacing domain.SessionToken) (domain.SessionToken, domain.Account) {
	t.Helper()
	token := domain.NewSessionToken()
	var replacingHash []byte
	if replacing != "" {
		replacingHash = replacing.Hash()
	}
	account, session, err := s.SignIn(context.Background(), identity, token.Hash(), replacingHash)
	if err != nil {
		t.Fatal(err)
	}
	if lifetime := time.Until(session.ExpiresAt); lifetime < domain.SessionLifetime-time.Minute || lifetime > domain.SessionLifetime {
		t.Errorf("the session expires in %v, want %v", lifetime, domain.SessionLifetime)
	}
	return token, account
}

// sessionAccount returns the account signed in with token, or false when there's none.
func sessionAccount(t *testing.T, s *Store, token domain.SessionToken) (domain.Account, bool) {
	t.Helper()
	account, err := s.SessionAccount(context.Background(), token.Hash())
	if errors.Is(err, store.ErrNotFound) {
		return domain.Account{}, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return account, true
}

func TestSignInCreatesAnAccountAndASessionThatFindsIt(t *testing.T) {
	s, _ := newStore(t)
	token, account := signIn(t, s, octocat, "")
	if account.ID == 0 || account.Identity != octocat || account.CreatedAt.IsZero() {
		t.Errorf("signed in %+v", account)
	}
	got, found := sessionAccount(t, s, token)
	if !found || got != account {
		t.Errorf("the session finds %+v, %v; want %+v", got, found, account)
	}
	if _, found := sessionAccount(t, s, domain.NewSessionToken()); found {
		t.Error("a token that was never signed in finds an account")
	}
}

// GitHub's user ID identifies an account, so a renamed user keeps theirs, with the new login and avatar.
func TestSignInAgainAfterARenameKeepsTheAccountAndUpdatesItsLogin(t *testing.T) {
	s, _ := newStore(t)
	first, before := signIn(t, s, octocat, "")
	renamed := domain.Identity{GitHubUserID: octocat.GitHubUserID, Login: "monalisa", AvatarURL: ""}
	second, after := signIn(t, s, renamed, "")
	if after.ID != before.ID || after.Identity != renamed || !after.CreatedAt.Equal(before.CreatedAt) {
		t.Errorf("signed in as %+v after %+v; want the same account with the new login", after, before)
	}
	// Both browsers stay signed in, and both see the new login.
	for _, token := range []domain.SessionToken{first, second} {
		if got, found := sessionAccount(t, s, token); !found || got.Login != "monalisa" {
			t.Errorf("a session finds %+v, %v", got, found)
		}
	}
	// Another user who takes the old login gets an account of their own.
	_, other := signIn(t, s, domain.Identity{GitHubUserID: 1, Login: "octocat"}, "")
	if other.ID == before.ID {
		t.Error("a user with a renamed user's old login got their account")
	}
}

// A browser that signs in again gets a new token, and its old one stops working, so a token fixed before sign-in is
// worth nothing after it.
func TestSignInEndsTheSessionItReplaces(t *testing.T) {
	s, _ := newStore(t)
	old, _ := signIn(t, s, octocat, "")
	other, _ := signIn(t, s, octocat, "")
	replacement, _ := signIn(t, s, octocat, old)
	if _, found := sessionAccount(t, s, old); found {
		t.Error("the replaced session still finds the account")
	}
	for _, token := range []domain.SessionToken{other, replacement} {
		if _, found := sessionAccount(t, s, token); !found {
			t.Error("a session the sign-in didn't replace ended")
		}
	}
}

func TestAnExpiredSessionFindsNoAccountAndTheNextSignInDeletesIt(t *testing.T) {
	s, owner := newStore(t)
	expired, _ := signIn(t, s, octocat, "")
	postgrestest.Exec(t, owner, `UPDATE sessions SET created_at = now() - interval '31 days', expires_at = now() - interval '1 day'
		WHERE token_hash = $1`, expired.Hash())
	if _, found := sessionAccount(t, s, expired); found {
		t.Error("an expired session finds its account")
	}
	signIn(t, s, domain.Identity{GitHubUserID: 2, Login: "hubot"}, "")
	var left int
	postgrestest.QueryRow(t, owner, "SELECT count(*) FROM sessions WHERE expires_at <= now()", &left)
	if left != 0 {
		t.Errorf("%d expired sessions are left after a sign-in", left)
	}
}

func TestSignInKeepsAnAccountsNewestSessionsUpToTheLimit(t *testing.T) {
	s, owner := newStore(t)
	tokens := make([]domain.SessionToken, domain.MaxSessions+1)
	for i := range tokens {
		tokens[i], _ = signIn(t, s, octocat, "")
	}
	if _, found := sessionAccount(t, s, tokens[0]); found {
		t.Error("the oldest session survived a sign-in past the limit")
	}
	for _, token := range tokens[1:] {
		if _, found := sessionAccount(t, s, token); !found {
			t.Fatal("one of the newest sessions ended")
		}
	}
	// Another account's sessions don't count toward it.
	hubot, _ := signIn(t, s, domain.Identity{GitHubUserID: 2, Login: "hubot"}, "")
	signIn(t, s, octocat, "")
	if _, found := sessionAccount(t, s, hubot); !found {
		t.Error("another account's sign-in ended hubot's only session")
	}
	var count int
	postgrestest.QueryRow(t, owner, "SELECT count(*) FROM sessions", &count)
	if count != domain.MaxSessions+1 {
		t.Errorf("%d sessions are stored, want %d for octocat and 1 for hubot", count, domain.MaxSessions+1)
	}
}

func TestEndSessionSignsOutOnlyThatBrowser(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	ended, _ := signIn(t, s, octocat, "")
	kept, _ := signIn(t, s, octocat, "")
	if err := s.EndSession(ctx, ended.Hash()); err != nil {
		t.Fatal(err)
	}
	if _, found := sessionAccount(t, s, ended); found {
		t.Error("the ended session still finds its account")
	}
	if _, found := sessionAccount(t, s, kept); !found {
		t.Error("ending one session ended another")
	}
	// Ending it again, or a session that never existed, does nothing.
	if err := s.EndSession(ctx, ended.Hash()); err != nil {
		t.Error(err)
	}
}

func TestEndSessionsSignsOutEveryBrowserOfOneAccount(t *testing.T) {
	s, _ := newStore(t)
	first, account := signIn(t, s, octocat, "")
	second, _ := signIn(t, s, octocat, "")
	hubot, _ := signIn(t, s, domain.Identity{GitHubUserID: 2, Login: "hubot"}, "")
	if err := s.EndSessions(context.Background(), account.ID); err != nil {
		t.Fatal(err)
	}
	for _, token := range []domain.SessionToken{first, second} {
		if _, found := sessionAccount(t, s, token); found {
			t.Error("a session of the account survived")
		}
	}
	if _, found := sessionAccount(t, s, hubot); !found {
		t.Error("another account's session ended")
	}
}

func TestDeleteAccountRemovesItAndItsSessions(t *testing.T) {
	s, owner := newStore(t)
	token, account := signIn(t, s, octocat, "")
	signIn(t, s, domain.Identity{GitHubUserID: 2, Login: "hubot"}, "")
	if err := s.DeleteAccount(context.Background(), account.ID); err != nil {
		t.Fatal(err)
	}
	if _, found := sessionAccount(t, s, token); found {
		t.Error("the deleted account's session still finds it")
	}
	var accounts, sessions int
	postgrestest.QueryRow(t, owner, "SELECT (SELECT count(*) FROM accounts WHERE github_user_id = 583231), (SELECT count(*) FROM sessions)", &accounts, &sessions)
	if accounts != 0 || sessions != 1 {
		t.Errorf("%d accounts for octocat and %d sessions are left, want 0 and hubot's 1", accounts, sessions)
	}
	// Signing in again makes a new account.
	_, again := signIn(t, s, octocat, "")
	if again.ID == account.ID {
		t.Error("signing in after deletion brought back the deleted account's ID")
	}
}
