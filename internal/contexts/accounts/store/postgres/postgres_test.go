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
	account, session, err := s.SignIn(context.Background(), identity, token.Hash(), replacingHash, nil)
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
	first, _ := signIn(t, s, octocat, "")
	second, _ := signIn(t, s, octocat, "")
	hubot, _ := signIn(t, s, domain.Identity{GitHubUserID: 2, Login: "hubot"}, "")
	if err := s.EndSessions(context.Background(), first.Hash()); err != nil {
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
	if err := s.DeleteAccount(context.Background(), token.Hash()); err != nil {
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

// Signing out everywhere and deleting an account act only for a session that's still live when they write: a request
// whose session another browser ended a moment earlier, such as by signing out everywhere, changes nothing, even if
// the visitor has signed in again since.
func TestAnEndedSessionCanNeitherSignOutEverywhereNorDeleteTheAccount(t *testing.T) {
	s, owner := newStore(t)
	ctx := context.Background()
	ended, account := signIn(t, s, octocat, "")
	if err := s.EndSession(ctx, ended.Hash()); err != nil {
		t.Fatal(err)
	}
	again, _ := signIn(t, s, octocat, "")
	expired, _ := signIn(t, s, octocat, "")
	postgrestest.Exec(t, owner, `UPDATE sessions SET created_at = now() - interval '31 days', expires_at = now() - interval '1 day'
		WHERE token_hash = $1`, expired.Hash())

	for _, token := range []domain.SessionToken{ended, expired, domain.NewSessionToken()} {
		if err := s.EndSessions(ctx, token.Hash()); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("signing out everywhere with a dead session: got %v, want ErrNotFound", err)
		}
		if err := s.DeleteAccount(ctx, token.Hash()); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("deleting with a dead session: got %v, want ErrNotFound", err)
		}
	}
	if got, found := sessionAccount(t, s, again); !found || got.ID != account.ID {
		t.Error("a dead session's request ended the visitor's new session or deleted the account")
	}
}

// An Enterprise Managed User's login has an underscore, which the accounts table accepts as GitHub does.
func TestSignInStoresAnEnterpriseManagedUsersLogin(t *testing.T) {
	s, _ := newStore(t)
	_, account := signIn(t, s, domain.Identity{GitHubUserID: 3, Login: "octocat_acme"}, "")
	if account.Login != "octocat_acme" {
		t.Errorf("stored %+v", account)
	}
}

// The header shows the name a visitor's GitHub profile shows, so each sign-in keeps the name GitHub reports then, and
// clears it when they removed it.
func TestSignInKeepsTheNameGitHubReportsAtEachSignIn(t *testing.T) {
	s, _ := newStore(t)
	token, _ := signIn(t, s, octocat.WithName("The Octocat"), "")
	if account, _ := sessionAccount(t, s, token); account.Name != "The Octocat" {
		t.Errorf("the account's name is %q", account.Name)
	}
	token, _ = signIn(t, s, octocat, token)
	if account, _ := sessionAccount(t, s, token); account.Name != "" {
		t.Errorf("after the name was removed on GitHub, the account's name is %q", account.Name)
	}
}

// The session's sealed token comes back as stored, a session without one has none, and once the session ends, so does
// its token.
func TestSessionGitHubTokenReturnsTheSealedTokenWhileTheSessionLasts(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	token := domain.NewSessionToken()
	sealed := []byte("a sealed token of at least twenty-nine bytes")
	if _, _, err := s.SignIn(ctx, octocat, token.Hash(), nil, sealed); err != nil {
		t.Fatal(err)
	}
	if got, err := s.SessionGitHubToken(ctx, token.Hash()); err != nil || string(got) != string(sealed) {
		t.Fatalf("got %q, %v", got, err)
	}
	without, _ := signIn(t, s, octocat, "")
	if got, err := s.SessionGitHubToken(ctx, without.Hash()); err != nil || got != nil {
		t.Errorf("a session without a token: got %q, %v; want nil", got, err)
	}
	if err := s.EndSession(ctx, token.Hash()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionGitHubToken(ctx, token.Hash()); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("after sign-out: got %v, want ErrNotFound", err)
	}
}

// Each sign-in discards what Rulemart read of the account's GitHub account, so the next page reads it again with the
// new session's token.
func TestSignInDiscardsTheAccountsGitHubSnapshot(t *testing.T) {
	ctx := context.Background()
	s, connString := newStore(t)
	_, account := signIn(t, s, octocat, "")
	if err := s.SaveSnapshot(ctx, account.ID, domain.Snapshot{Organizations: []string{"octo-org"}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	signIn(t, s, octocat, "")
	var kept int
	postgrestest.QueryRow(t, connString, "SELECT count(*) FROM github_snapshots", &kept)
	if kept != 0 {
		t.Errorf("%d snapshots outlived signing in again", kept)
	}
}
