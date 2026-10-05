package github

import (
	"cmp"
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/github/githubtest"
)

// GitHub answers 404, or 403, for a repository the token can't see, which a read skips; but a 403 or 429 for a rate
// limit, a 401, or a 5xx fails the read, so a snapshot never takes a refusal for an empty account. A secondary rate
// limit's 403 may carry neither rate limit header, only its message. No error holds the token.
func TestAPITellsWhatATokenCantSeeFromAFailure(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		header map[string]string
		// message is the response's message, or empty for one that repeats the request's Authorization header.
		message string
		found   bool
		wantErr error
		failed  bool
	}{
		"not found":            {status: 404},
		"forbidden":            {status: 403},
		"rate limited":         {status: 403, header: map[string]string{"X-RateLimit-Remaining": "0"}, failed: true},
		"secondary rate limit": {status: 403, header: map[string]string{"Retry-After": "60"}, failed: true},
		"too many requests":    {status: 429, failed: true},
		"a secondary rate limit's message alone": {status: 403, failed: true,
			message: "You have exceeded a secondary rate limit. Please wait a few minutes before you try again."},
		"a primary rate limit's message alone": {status: 403, failed: true, message: "API rate limit exceeded for user ID 1."},
		"an installation's refusal":            {status: 403, message: "Resource not accessible by integration"},
		"a revoked authorization":              {status: 401, wantErr: domain.ErrGitHubTokenRefused, failed: true},
		"GitHub failing":                       {status: 502, failed: true},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for k, v := range tc.header {
					w.Header().Set(k, v)
				}
				message := cmp.Or(tc.message, r.Header.Get("Authorization"))
				http.Error(w, `{"message":"`+message+`"}`, tc.status)
			}))
			defer server.Close()
			_, found, err := NewAPI(server.URL).File(context.Background(), "gho_secret", domain.Repository{Owner: "o", Name: "r"}, "x", 100)
			if (err != nil) != tc.failed || found || (tc.wantErr != nil && !errors.Is(err, tc.wantErr)) {
				t.Fatalf("got found %v, %v; want failed: %v", found, err, tc.failed)
			}
			if err != nil && strings.Contains(err.Error(), "gho_secret") {
				t.Errorf("the error %q holds the token", err)
			}
		})
	}
}

// GitHub gives an app's key as PKCS #1, and tools convert it to PKCS #8; both sign. Anything else is refused without
// repeating it.
func TestParsePrivateKeyTakesPKCS1AndPKCS8(t *testing.T) {
	key := githubtest.NewAppKey()
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"PKCS #1": githubtest.AppKeyPEM(key),
		"PKCS #8": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})),
	} {
		if got, err := ParsePrivateKey([]byte(text)); err != nil || !got.Equal(key) {
			t.Errorf("%s: got %v", name, err)
		}
	}
	for name, text := range map[string]string{"not PEM": "a key", "not a key": "-----BEGIN RSA PRIVATE KEY-----\nAAAA\n-----END RSA PRIVATE KEY-----\n"} {
		if _, err := ParsePrivateKey([]byte(text)); err == nil || strings.Contains(err.Error(), "AAAA") {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

// The app's requests carry a JWT the fake checks as GitHub does; a key GitHub doesn't know gets a refusal that names
// the configuration, not the key.
func TestTheAppSignsItsRequestsWithAJWTFromItsKey(t *testing.T) {
	fake := &githubtest.Fake{
		AppClientID: "Iv1.app", AppKey: githubtest.NewAppKey(),
		Installations: []githubtest.Installation{{ID: 9, Account: "octo-org", AccountID: 3, Organization: true}},
	}
	server := httptest.NewServer(fake.Handler())
	defer server.Close()
	app := NewApp(AppConfig{ClientID: "Iv1.app", PrivateKey: fixedSecret(githubtest.AppKeyPEM(fake.AppKey))}, NewAPI(server.URL))

	got, err := app.Installation(context.Background(), 9)
	if err != nil || got != (domain.InstallationAccount{Login: "octo-org", ID: 3, Organization: true}) {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := app.Installation(context.Background(), 10); !errors.Is(err, domain.ErrNoSuchInstallation) {
		t.Errorf("an unknown installation: got %v", err)
	}
	stranger := NewApp(AppConfig{ClientID: "Iv1.app", PrivateKey: fixedSecret(githubtest.AppKeyPEM(githubtest.NewAppKey()))}, NewAPI(server.URL))
	if _, err := stranger.InstallationToken(context.Background(), 9); err == nil || !strings.Contains(err.Error(), "JWT") {
		t.Errorf("another key: got %v", err)
	}
}

// GitHub answers 404 for an installation it doesn't know, the one answer that says the app was uninstalled; a 403
// refuses to say, so the lookup fails, as it does for a rate limit or GitHub failing, rather than call it gone.
func TestAnInstallationIsGoneOnlyWhenGitHubAnswers404(t *testing.T) {
	for name, tc := range map[string]struct {
		status  int
		header  map[string]string
		message string
		gone    bool
	}{
		"not found":                 {status: 404, gone: true},
		"an installation's refusal": {status: 403, message: "Resource not accessible by integration"},
		"rate limited":              {status: 403, header: map[string]string{"X-RateLimit-Remaining": "0"}},
		"too many requests":         {status: 429},
		"GitHub failing":            {status: 502},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for k, v := range tc.header {
					w.Header().Set(k, v)
				}
				http.Error(w, `{"message":"`+tc.message+`"}`, tc.status)
			}))
			defer server.Close()
			app := NewApp(AppConfig{ClientID: "Iv1.app", PrivateKey: fixedSecret(githubtest.AppKeyPEM(githubtest.NewAppKey()))}, NewAPI(server.URL))

			state, err := app.InstallationState(context.Background(), 9)
			if tc.gone && (err != nil || state != domain.InstallationGone) {
				t.Errorf("the state: got %v, %v, want gone", state, err)
			}
			if !tc.gone && err == nil {
				t.Errorf("the state: got %v, want a failure", state)
			}
			_, err = app.Installation(context.Background(), 9)
			if errors.Is(err, domain.ErrNoSuchInstallation) != tc.gone || err == nil {
				t.Errorf("the account: got %v, want ErrNoSuchInstallation: %t", err, tc.gone)
			}
		})
	}
}

func TestInstallURLIsTheAppsInstallPage(t *testing.T) {
	if got := NewApp(AppConfig{Slug: "rulemart-by-fabrica"}, nil).InstallURL(); got != "https://github.com/apps/rulemart-by-fabrica/installations/new" {
		t.Errorf("got %s", got)
	}
}

// GitHub lists an installation's repositories in an order of its own, with no way to sort them, so the app reads the
// whole list, within a budget of pages, and returns the private ones, the most recently pushed first, however many
// public ones come before them; it says when it left some out, because there were more than the limit, or more pages
// than the budget.
func TestInstallationRepositoriesAreItsPrivateOnesTheMostRecentlyPushedFirst(t *testing.T) {
	pushed := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	install := func(repos []githubtest.Repository) *App {
		t.Helper()
		in := githubtest.Installation{ID: 9, Account: "octo-org", AccountID: 3, Organization: true}
		for _, r := range repos {
			in.Repositories = append(in.Repositories, r.FullName())
		}
		fake := &githubtest.Fake{AppClientID: "Iv1.app", AppKey: githubtest.NewAppKey(), Repositories: repos, Installations: []githubtest.Installation{in}}
		server := httptest.NewServer(fake.Handler())
		t.Cleanup(server.Close)
		return NewApp(AppConfig{ClientID: "Iv1.app", PrivateKey: fixedSecret(githubtest.AppKeyPEM(fake.AppKey))}, NewAPI(server.URL))
	}
	read := func(app *App, limit int) ([]string, bool) {
		t.Helper()
		token, err := app.InstallationToken(context.Background(), 9)
		if err != nil {
			t.Fatal(err)
		}
		repos, more, err := app.InstallationRepositories(context.Background(), token, limit)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, r := range repos {
			names = append(names, r.Name)
		}
		return names, more
	}
	var repos []githubtest.Repository
	for i := range 250 {
		repos = append(repos, githubtest.Repository{Owner: "octo-org", Name: fmt.Sprintf("public-%03d", i), PushedAt: pushed.Add(time.Duration(i) * time.Hour)})
	}
	for i, name := range []string{"middle", "oldest", "newest"} {
		offsets := []time.Duration{-2, -3, -1}
		repos = append(repos, githubtest.Repository{Owner: "octo-org", Name: name, Private: true, PushedAt: pushed.Add(offsets[i] * time.Hour)})
	}
	app := install(repos)

	if got, more := read(app, 5); !slices.Equal(got, []string{"newest", "middle", "oldest"}) || more {
		t.Errorf("got %q, more %v; want the three private ones, newest first, and nothing more", got, more)
	}
	if got, more := read(app, 2); !slices.Equal(got, []string{"newest", "middle"}) || !more {
		t.Errorf("with a limit of 2, got %q, more %v; want the two newest and more", got, more)
	}

	var many []githubtest.Repository
	for i := range maxInstallationPages*perPage + 1 {
		many = append(many, githubtest.Repository{Owner: "octo-org", Name: fmt.Sprintf("r-%04d", i), Private: true, PushedAt: pushed})
	}
	if got, more := read(install(many), maxInstallationPages*perPage+10); len(got) != maxInstallationPages*perPage || !more {
		t.Errorf("past the page budget, got %d repositories, more %v; want %d and more", len(got), more, maxInstallationPages*perPage)
	}
}
