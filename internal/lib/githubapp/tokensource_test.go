package githubapp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fabricahq/rulemart/internal/lib/githubapp/githubapptest"
)

// clock is a test's clock, which the app and the fake share.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// newTokenSource returns a TokenSource for the app's installation on octo-org, the fake it reads, and the clock both
// keep, which the test advances.
func newTokenSource(t *testing.T) (*TokenSource, *githubapptest.Fake, *clock) {
	t.Helper()
	fake, app := newFake(t)
	c := &clock{now: time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)}
	fake.Now, app.now = c.Now, c.Now
	return app.TokenSource("octo-org"), fake, c
}

// wantToken checks that the source gives the token want.
func wantToken(t *testing.T, source *TokenSource, want string) {
	t.Helper()
	if got, err := source.Value(context.Background()); err != nil || got != want {
		t.Fatalf("got %q, %v; want %q", got, err, want)
	}
}

// A token lasts an hour, so the source finds the installation and mints a token once, keeps it, and mints the next
// once five minutes of the hour are left.
func TestTokenSourceKeepsATokenUntilFiveMinutesBeforeItExpires(t *testing.T) {
	source, fake, c := newTokenSource(t)

	wantToken(t, source, githubapptest.Token(9, 1))
	c.advance(55*time.Minute - time.Second)
	wantToken(t, source, githubapptest.Token(9, 1))
	if fake.Mints() != 1 || fake.Lookups() != 1 {
		t.Fatalf("minted %d tokens after %d lookups, want one of each", fake.Mints(), fake.Lookups())
	}

	c.advance(time.Second)
	wantToken(t, source, githubapptest.Token(9, 2))
	if fake.Mints() != 2 || fake.Lookups() != 1 {
		t.Errorf("minted %d tokens after %d lookups, want two after one", fake.Mints(), fake.Lookups())
	}
}

// Jobs that need a token at once, such as the first ones after a cold start, wait for one mint and share its token.
func TestConcurrentCallersShareOneMint(t *testing.T) {
	source, fake, _ := newTokenSource(t)

	var wg sync.WaitGroup
	tokens := make([]string, 20)
	errs := make([]error, len(tokens))
	for i := range tokens {
		wg.Go(func() { tokens[i], errs[i] = source.Value(context.Background()) })
	}
	wg.Wait()

	for i, token := range tokens {
		if errs[i] != nil || token != githubapptest.Token(9, 1) {
			t.Errorf("caller %d got %q, %v", i, token, errs[i])
		}
	}
	if fake.Mints() != 1 || fake.Lookups() != 1 {
		t.Errorf("minted %d tokens after %d lookups, want one of each", fake.Mints(), fake.Lookups())
	}
}

// When GitHub fails to mint the next token, the source keeps the one it has, while more than a minute of it is left,
// and fails after that until GitHub mints again.
func TestAFailedMintKeepsTheOldTokenUntilAMinuteBeforeItExpires(t *testing.T) {
	source, fake, c := newTokenSource(t)
	wantToken(t, source, githubapptest.Token(9, 1))

	fake.FailMints(true)
	c.advance(56 * time.Minute)
	wantToken(t, source, githubapptest.Token(9, 1))
	c.advance(3*time.Minute - time.Second)
	wantToken(t, source, githubapptest.Token(9, 1))
	c.advance(time.Second)
	if got, err := source.Value(context.Background()); err == nil {
		t.Fatalf("a minute before the token expires, got %q, want the failed mint's error", got)
	}

	fake.FailMints(false)
	wantToken(t, source, githubapptest.Token(9, 2))
}

// A token GitHub refused, such as one revoked, is forgotten, and the next call mints another.
func TestForgetDropsTheToken(t *testing.T) {
	source, fake, _ := newTokenSource(t)
	wantToken(t, source, githubapptest.Token(9, 1))

	source.Forget()
	wantToken(t, source, githubapptest.Token(9, 2))
	if fake.Lookups() != 1 {
		t.Errorf("looked the installation up %d times, want once", fake.Lookups())
	}
}

// Reinstalling the app gives the account a new installation, which the source finds once the old one is gone.
func TestTokenSourceFindsAReinstalledInstallation(t *testing.T) {
	source, fake, c := newTokenSource(t)
	wantToken(t, source, githubapptest.Token(9, 1))

	fake.SetInstallations([]githubapptest.Installation{{ID: 20, Account: "octo-org", AccountID: 3, Organization: true}})
	c.advance(time.Hour)
	if _, err := source.Value(context.Background()); !errors.Is(err, ErrNoSuchInstallation) {
		t.Fatalf("with the old installation gone, got %v, want ErrNoSuchInstallation", err)
	}
	wantToken(t, source, githubapptest.Token(20, 2))
}

// An account without the app, or whose installation is suspended, gets no token.
func TestTokenSourceFailsWithoutAnInstallationToUse(t *testing.T) {
	_, app := newFake(t)
	for _, account := range []string{"elsewhere", "paused-org"} {
		if got, err := app.TokenSource(account).Value(context.Background()); !errors.Is(err, ErrNoSuchInstallation) || got != "" {
			t.Errorf("%s: got %q, %v; want ErrNoSuchInstallation", account, got, err)
		}
	}
}
