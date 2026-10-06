package githubapp

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fabricahq/rulemart/internal/lib/githubapp/githubapptest"
)

// secret is a private key a test gives directly, counting how often it was forgotten.
type secret struct {
	value     string
	forgotten int
}

func (s *secret) Value(context.Context) (string, error) { return s.value, nil }
func (s *secret) Forget()                               { s.forgotten++ }

// newFake serves a fake GitHub for the app "Iv1.app", installed on the organization octo-org and the user octocat, and
// returns it with an App that signs with its key.
func newFake(t *testing.T) (*githubapptest.Fake, *App) {
	t.Helper()
	fake := &githubapptest.Fake{
		Issuer: "Iv1.app", Key: githubapptest.NewKey(),
		Installations: []githubapptest.Installation{
			{ID: 9, Account: "octo-org", AccountID: 3, Organization: true},
			{ID: 11, Account: "octocat", AccountID: 4},
			{ID: 12, Account: "paused-org", AccountID: 5, Organization: true, Suspended: true},
		},
	}
	server := httptest.NewServer(fake.Handler())
	t.Cleanup(server.Close)
	return fake, New(Config{Issuer: "Iv1.app", PrivateKey: &secret{value: githubapptest.KeyPEM(fake.Key)}, BaseURL: server.URL})
}

// GitHub gives an app's key as PKCS #1, and tools convert it to PKCS #8; both sign. Anything else is refused without
// repeating it.
func TestParsePrivateKeyTakesPKCS1AndPKCS8(t *testing.T) {
	key := githubapptest.NewKey()
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"PKCS #1": githubapptest.KeyPEM(key),
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

// The app's requests carry a JWT the fake checks as GitHub does, and an installation reads as the account it's on.
func TestInstallationReadsTheAccountItsOn(t *testing.T) {
	_, app := newFake(t)

	got, err := app.Installation(context.Background(), 9)
	if err != nil || got != (Installation{ID: 9, Account: Account{Login: "octo-org", ID: 3, Organization: true}}) {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := app.Installation(context.Background(), 10); !errors.Is(err, ErrNoSuchInstallation) {
		t.Errorf("an unknown installation: got %v, want ErrNoSuchInstallation", err)
	}
}

// GitHub keeps an organization's installation and a user's at different addresses, and a login names one or the other.
func TestAccountInstallationFindsAnOrganizationsOrAUsers(t *testing.T) {
	_, app := newFake(t)
	for login, want := range map[string]int64{"octo-org": 9, "octocat": 11, "OCTO-ORG": 9} {
		if got, err := app.AccountInstallation(context.Background(), login); err != nil || got.ID != want {
			t.Errorf("%s: got %+v, %v; want installation %d", login, got, err, want)
		}
	}
	if _, err := app.AccountInstallation(context.Background(), "elsewhere"); !errors.Is(err, ErrNoSuchInstallation) || !strings.Contains(err.Error(), `"elsewhere"`) {
		t.Errorf("an account without the app: got %v, want ErrNoSuchInstallation naming it", err)
	}
}

// An installation's token lasts the hour GitHub says it does; a suspended installation gets none.
func TestInstallationTokenReturnsTheTokenAndWhenItExpires(t *testing.T) {
	_, app := newFake(t)
	before := time.Now().Truncate(time.Second)

	got, err := app.InstallationToken(context.Background(), 9)
	if err != nil || got.Value != githubapptest.Token(9, 1) {
		t.Fatalf("got %+v, %v", got, err)
	}
	if expires := got.ExpiresAt.Sub(before); expires < githubapptest.TokenLifetime || expires > githubapptest.TokenLifetime+time.Minute {
		t.Errorf("the token expires in %s, want about an hour", expires)
	}
	for _, id := range []int64{10, 12} {
		if _, err := app.InstallationToken(context.Background(), id); !errors.Is(err, ErrNoSuchInstallation) {
			t.Errorf("installation %d: got %v, want ErrNoSuchInstallation", id, err)
		}
	}
}

// A key GitHub doesn't know gets a refusal that names the configuration, not the key, and the key is read again next
// time, in case it was rotated.
func TestARefusedJWTForgetsTheKey(t *testing.T) {
	fake, _ := newFake(t)
	server := httptest.NewServer(fake.Handler())
	defer server.Close()
	key := &secret{value: githubapptest.KeyPEM(githubapptest.NewKey())}
	stranger := New(Config{Issuer: "Iv1.app", PrivateKey: key, BaseURL: server.URL})

	_, err := stranger.InstallationToken(context.Background(), 9)
	if err == nil || !strings.Contains(err.Error(), "JWT") || strings.Contains(err.Error(), "PRIVATE KEY") {
		t.Errorf("got %v, want a refusal of the JWT", err)
	}
	if key.forgotten != 1 {
		t.Errorf("forgot the key %d times, want once", key.forgotten)
	}
}

// A rate limit, or GitHub failing, is a failure, never an installation GitHub doesn't have.
func TestAThrottledOrFailedRequestIsntAMissingInstallation(t *testing.T) {
	for name, status := range map[string]int{"rate limited": http.StatusTooManyRequests, "GitHub failing": http.StatusBadGateway} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, `{"message":"no"}`, status)
		}))
		app := New(Config{Issuer: "Iv1.app", PrivateKey: &secret{value: githubapptest.KeyPEM(githubapptest.NewKey())}, BaseURL: server.URL})
		if _, err := app.InstallationToken(context.Background(), 9); err == nil || errors.Is(err, ErrNoSuchInstallation) {
			t.Errorf("%s: got %v, want a failure", name, err)
		}
		server.Close()
	}
}
