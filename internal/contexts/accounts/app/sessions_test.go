package app

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/store/postgres"
	"github.com/fabricahq/rulemart/internal/platform/database/databasetest"
	"github.com/fabricahq/rulemart/internal/platform/postgrestest"
)

func newSessions(t *testing.T) Sessions {
	t.Helper()
	_, connString := databasetest.New(t)
	return Sessions{Store: postgres.New(databasetest.AsWebRole(t, connString))}
}

var octocat = domain.Identity{GitHubUserID: 583231, Login: "octocat"}

// The browser's token is the only way back to a session: the store keeps its hash, and a sign-in that replaces it
// ends it.
func TestSignInGivesATokenThatSignsTheAccountInUntilSignOut(t *testing.T) {
	ctx := context.Background()
	s := newSessions(t)
	account, session, err := s.SignIn(ctx, octocat, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := domain.ParseSessionToken(string(session.Token)); !ok || session.ExpiresAt.IsZero() {
		t.Fatalf("signed in with the session %+v", session)
	}
	if got, err := s.Account(ctx, session.Token); err != nil || got != account {
		t.Errorf("the token signs in %+v, %v; want %+v", got, err, account)
	}

	_, replacement, err := s.SignIn(ctx, octocat, "", session.Token)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.Token == session.Token {
		t.Fatal("signing in again kept the token")
	}
	if _, err := s.Account(ctx, session.Token); !errors.Is(err, ErrSignedOut) {
		t.Errorf("the replaced token: got %v, want ErrSignedOut", err)
	}

	if err := s.SignOut(ctx, replacement.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Account(ctx, replacement.Token); !errors.Is(err, ErrSignedOut) {
		t.Errorf("after sign-out: got %v, want ErrSignedOut", err)
	}
}

func TestSignOutEverywhereAndDeleteAccountEndEverySession(t *testing.T) {
	ctx := context.Background()
	s := newSessions(t)
	for name, end := range map[string]func(domain.SessionToken) error{
		"sign out everywhere": func(token domain.SessionToken) error { return s.SignOutEverywhere(ctx, token) },
		"delete the account":  func(token domain.SessionToken) error { return s.DeleteAccount(ctx, token) },
	} {
		t.Run(name, func(t *testing.T) {
			_, first, err := s.SignIn(ctx, octocat, "", "")
			if err != nil {
				t.Fatal(err)
			}
			_, second, err := s.SignIn(ctx, octocat, "", "")
			if err != nil {
				t.Fatal(err)
			}
			if err := end(first.Token); err != nil {
				t.Fatal(err)
			}
			for _, session := range []domain.Session{first, second} {
				if _, err := s.Account(ctx, session.Token); !errors.Is(err, ErrSignedOut) {
					t.Errorf("got %v, want ErrSignedOut", err)
				}
			}
		})
	}
}

const gitHubToken = "gho_16C7e42F292c6912E7710c838347Ae178B4a"

// The session keeps the visitor's GitHub token sealed: the database holds none of its text, the session opens it, and
// signing out deletes it, so no later request, even one with the old cookie, gets it back.
func TestTheSessionKeepsTheGitHubTokenSealedUntilSignOut(t *testing.T) {
	ctx := context.Background()
	_, connString := databasetest.New(t)
	s := Sessions{Store: postgres.New(databasetest.AsWebRole(t, connString)), TokenKeys: FixedTokenKey{domain.NewTokenKey()}}
	_, session, err := s.SignIn(ctx, octocat, gitHubToken, "")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.GitHubToken(ctx, session.Token); err != nil || got != gitHubToken {
		t.Fatalf("the session's token is %q, %v; want %q", got, err, gitHubToken)
	}
	var stored []byte
	postgrestest.QueryRow(t, connString, "SELECT github_token FROM sessions", &stored)
	if len(stored) == 0 || bytes.Contains(stored, []byte(gitHubToken)) || bytes.Contains(stored, []byte("gho_")) {
		t.Fatalf("the sessions table holds %q", stored)
	}

	if err := s.SignOut(ctx, session.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GitHubToken(ctx, session.Token); !errors.Is(err, ErrSignedOut) {
		t.Errorf("after sign-out: got %v, want ErrSignedOut", err)
	}
	var sessions int
	postgrestest.QueryRow(t, connString, "SELECT count(*) FROM sessions WHERE github_token IS NOT NULL", &sessions)
	if sessions != 0 {
		t.Errorf("%d sealed tokens outlived signing out", sessions)
	}
}

// Signing out everywhere and deleting the account delete every session's token, the other browsers' too.
func TestSignOutEverywhereAndDeleteAccountDeleteEverySessionsGitHubToken(t *testing.T) {
	ctx := context.Background()
	for name, end := range map[string]func(Sessions, domain.SessionToken) error{
		"sign out everywhere": func(s Sessions, token domain.SessionToken) error { return s.SignOutEverywhere(ctx, token) },
		"delete the account":  func(s Sessions, token domain.SessionToken) error { return s.DeleteAccount(ctx, token) },
	} {
		t.Run(name, func(t *testing.T) {
			_, connString := databasetest.New(t)
			s := Sessions{Store: postgres.New(databasetest.AsWebRole(t, connString)), TokenKeys: FixedTokenKey{domain.NewTokenKey()}}
			_, first, err := s.SignIn(ctx, octocat, gitHubToken, "")
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := s.SignIn(ctx, octocat, gitHubToken, ""); err != nil {
				t.Fatal(err)
			}
			if err := end(s, first.Token); err != nil {
				t.Fatal(err)
			}
			var tokens int
			postgrestest.QueryRow(t, connString, "SELECT count(*) FROM sessions WHERE github_token IS NOT NULL", &tokens)
			if tokens != 0 {
				t.Errorf("%d sealed tokens remain", tokens)
			}
		})
	}
}

// A session without a token, such as a test user's, and one whose token a rotated key can't open, both say there's no
// token to use, so the visitor is asked to sign in again rather than shown a failure.
func TestASessionWithoutAnOpenableGitHubTokenHasNone(t *testing.T) {
	ctx := context.Background()
	_, connString := databasetest.New(t)
	store := postgres.New(databasetest.AsWebRole(t, connString))
	s := Sessions{Store: store, TokenKeys: FixedTokenKey{domain.NewTokenKey()}}

	_, without, err := s.SignIn(ctx, octocat, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GitHubToken(ctx, without.Token); !errors.Is(err, ErrNoGitHubToken) {
		t.Errorf("a session signed in without a token: got %v, want ErrNoGitHubToken", err)
	}

	_, sealed, err := s.SignIn(ctx, octocat, gitHubToken, "")
	if err != nil {
		t.Fatal(err)
	}
	rotated := Sessions{Store: store, TokenKeys: FixedTokenKey{domain.NewTokenKey()}}
	if _, err := rotated.GitHubToken(ctx, sealed.Token); !errors.Is(err, ErrNoGitHubToken) {
		t.Errorf("a token sealed under the key before a rotation: got %v, want ErrNoGitHubToken", err)
	}
}

// Without a key, a session can't keep a token, so signing in with one fails rather than keeping it in the clear.
func TestSignInWithAGitHubTokenAndNoKeyFails(t *testing.T) {
	s := newSessions(t)
	if _, _, err := s.SignIn(context.Background(), octocat, gitHubToken, ""); err == nil {
		t.Fatal("signed in, keeping a token without a key to seal it")
	}
}
