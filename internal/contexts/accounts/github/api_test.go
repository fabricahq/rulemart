package github

import (
	"cmp"
	"context"
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
	"github.com/fabricahq/rulemart/internal/lib/githubapp/githubapptest"
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

// The app's requests carry a JWT the fake checks as GitHub does; a key GitHub doesn't know gets a refusal that names
// the configuration, not the key.
func TestTheAppSignsItsRequestsWithAJWTFromItsKey(t *testing.T) {
	fake := &githubtest.Fake{
		AppClientID: "Iv1.app", AppKey: githubapptest.NewKey(),
		Installations: []githubtest.Installation{{ID: 9, Account: "octo-org", AccountID: 3, Organization: true}},
	}
	server := httptest.NewServer(fake.Handler())
	defer server.Close()
	app := NewApp(AppConfig{ClientID: "Iv1.app", PrivateKey: fixedSecret(githubapptest.KeyPEM(fake.AppKey))}, NewAPI(server.URL))

	got, err := app.Installation(context.Background(), 9)
	if err != nil || got != (domain.InstallationAccount{Login: "octo-org", ID: 3, Organization: true}) {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := app.Installation(context.Background(), 10); !errors.Is(err, domain.ErrNoSuchInstallation) {
		t.Errorf("an unknown installation: got %v", err)
	}
	stranger := NewApp(AppConfig{ClientID: "Iv1.app", PrivateKey: fixedSecret(githubapptest.KeyPEM(githubapptest.NewKey()))}, NewAPI(server.URL))
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
			app := NewApp(AppConfig{ClientID: "Iv1.app", PrivateKey: fixedSecret(githubapptest.KeyPEM(githubapptest.NewKey()))}, NewAPI(server.URL))

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
		fake := &githubtest.Fake{AppClientID: "Iv1.app", AppKey: githubapptest.NewKey(), Repositories: repos, Installations: []githubtest.Installation{in}}
		server := httptest.NewServer(fake.Handler())
		t.Cleanup(server.Close)
		return NewApp(AppConfig{ClientID: "Iv1.app", PrivateKey: fixedSecret(githubapptest.KeyPEM(fake.AppKey))}, NewAPI(server.URL))
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

// A repository read by name, and the repositories of a list, say whether the token's user may push to them: GitHub's
// permissions object, where push, maintain, or admin is enough, and one without any, as for a token that reads nothing
// of the user's, says no.
func TestAPIReadsWhetherTheTokenMayPushToARepository(t *testing.T) {
	for name, tc := range map[string]struct {
		permissions string
		writable    bool
	}{
		"push":         {`{"pull":true,"push":true}`, true},
		"maintain":     {`{"pull":true,"maintain":true}`, true},
		"admin":        {`{"pull":true,"admin":true}`, true},
		"only pull":    {`{"pull":true,"push":false,"admin":false}`, false},
		"no object":    {``, false},
		"empty object": {`{}`, false},
	} {
		t.Run(name, func(t *testing.T) {
			described := `{"name":"rules","private":false,"owner":{"login":"octo-org"}`
			if tc.permissions != "" {
				described += `,"permissions":` + tc.permissions
			}
			described += `}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/repos/octo-org/rules" {
					fmt.Fprint(w, described)
					return
				}
				fmt.Fprint(w, "["+described+"]")
			}))
			defer server.Close()
			api := NewAPI(server.URL)

			repo, found, err := api.Repository(context.Background(), "gho_x", domain.Repository{Owner: "octo-org", Name: "rules"})
			if err != nil || !found || repo.Writable != tc.writable || repo.FullName() != "octo-org/rules" {
				t.Errorf("Repository: got %+v, found %v, %v; want writable %v", repo, found, err, tc.writable)
			}
			listed, _, err := api.Repositories(context.Background(), "gho_x", "octo-org", true, 10)
			if err != nil || len(listed) != 1 || listed[0].Writable != tc.writable {
				t.Errorf("Repositories: got %+v, %v; want writable %v", listed, err, tc.writable)
			}
		})
	}
}

// A repository GitHub answers 404 or 403 for is not found rather than a failure, and a token it refuses fails as such.
func TestAPIRepositoryIsNotFoundWhereGitHubAnswers404Or403(t *testing.T) {
	for status, want := range map[int]error{404: nil, 403: nil, 401: domain.ErrGitHubTokenRefused} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"message":"no"}`, status)
		}))
		_, found, err := NewAPI(server.URL).Repository(context.Background(), "gho_x", domain.Repository{Owner: "o", Name: "r"})
		server.Close()
		if found || !errors.Is(err, want) || (want == nil && err != nil) {
			t.Errorf("%d: got found %v, %v; want %v", status, found, err, want)
		}
	}
}
