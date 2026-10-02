package app

import (
	"context"
	"errors"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/store/postgres"
	"github.com/fabricahq/rulemart/internal/platform/database/databasetest"
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
	account, session, err := s.SignIn(ctx, octocat, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := domain.ParseSessionToken(string(session.Token)); !ok || session.ExpiresAt.IsZero() {
		t.Fatalf("signed in with the session %+v", session)
	}
	if got, err := s.Account(ctx, session.Token); err != nil || got != account {
		t.Errorf("the token signs in %+v, %v; want %+v", got, err, account)
	}

	_, replacement, err := s.SignIn(ctx, octocat, session.Token)
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
	for name, end := range map[string]func(accountID int64) error{
		"sign out everywhere": func(id int64) error { return s.SignOutEverywhere(ctx, id) },
		"delete the account":  func(id int64) error { return s.DeleteAccount(ctx, id) },
	} {
		t.Run(name, func(t *testing.T) {
			account, first, err := s.SignIn(ctx, octocat, "")
			if err != nil {
				t.Fatal(err)
			}
			_, second, err := s.SignIn(ctx, octocat, "")
			if err != nil {
				t.Fatal(err)
			}
			if err := end(account.ID); err != nil {
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
