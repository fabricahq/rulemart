package github

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/github/githubtest"
)

// GitHub answers 404, or 403, for a repository the token can't see, which a read skips; but a 403 for a rate limit, a
// 401, or a 5xx fails the read, so a snapshot never takes a refusal for an empty account. No error holds the token.
func TestAPITellsWhatATokenCantSeeFromAFailure(t *testing.T) {
	for name, tc := range map[string]struct {
		status  int
		header  map[string]string
		found   bool
		wantErr error
		failed  bool
	}{
		"not found":               {status: 404},
		"forbidden":               {status: 403},
		"rate limited":            {status: 403, header: map[string]string{"X-RateLimit-Remaining": "0"}, failed: true},
		"secondary rate limit":    {status: 403, header: map[string]string{"Retry-After": "60"}, failed: true},
		"too many requests":       {status: 429, failed: true},
		"a revoked authorization": {status: 401, wantErr: domain.ErrGitHubTokenRefused, failed: true},
		"GitHub failing":          {status: 502, failed: true},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for k, v := range tc.header {
					w.Header().Set(k, v)
				}
				http.Error(w, `{"message":"`+r.Header.Get("Authorization")+`"}`, tc.status)
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

func TestInstallURLIsTheAppsInstallPage(t *testing.T) {
	if got := NewApp(AppConfig{Slug: "rulemart-by-fabrica"}, nil).InstallURL(); got != "https://github.com/apps/rulemart-by-fabrica/installations/new" {
		t.Errorf("got %s", got)
	}
}
