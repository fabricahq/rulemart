// Act as the GitHub App "Rulemart by Fabrica", which reads the private repositories visitors install it on: it signs
// its own requests with a JWT made from its private key, reads an installation's account, and gets an installation's
// token, which reads the repositories the visitor chose.

package github

import (
	"cmp"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
)

// AppConfig names the GitHub App and holds what it signs and checks with.
type AppConfig struct {
	// ID is GitHub's numeric ID for the app, which webhook deliveries name. ClientID is its client ID, which its JWTs
	// name as their issuer, as GitHub advises. Slug is the app's name in its URLs, such as rulemart-by-fabrica.
	ID       int64
	ClientID string
	Slug     string
	// WebURL is GitHub's site, where the app's install page is: https://github.com, or a fake's, when empty the former.
	WebURL string
	// PrivateKey holds the app's private key, in PEM, as GitHub gives it. WebhookSecret holds the secret its webhook's
	// deliveries are signed with.
	PrivateKey, WebhookSecret Secret
}

// App acts as one GitHub App, through api.
type App struct {
	config AppConfig
	api    *API
	// now returns the time a JWT is issued at, which a test fixes.
	now func() time.Time
}

// NewApp returns the app config describes, which reads GitHub through api.
func NewApp(config AppConfig, api *API) *App {
	return &App{config: config, api: api, now: time.Now}
}

// InstallURL returns the GitHub page where a visitor installs the app on the repositories they choose.
func (a *App) InstallURL() string {
	return cmp.Or(a.config.WebURL, "https://github.com") + "/apps/" + a.config.Slug + "/installations/new"
}

// Installation returns the GitHub account installation id is on, or fails with domain.ErrNoSuchInstallation.
func (a *App) Installation(ctx context.Context, id int64) (domain.InstallationAccount, error) {
	var installation struct {
		Account struct {
			Login string `json:"login"`
			ID    int64  `json:"id"`
			Type  string `json:"type"`
		} `json:"account"`
	}
	found, err := a.asApp(ctx, http.MethodGet, "/app/installations/"+strconv.FormatInt(id, 10), &installation)
	if err == nil && !found {
		err = domain.ErrNoSuchInstallation
	}
	if err != nil {
		return domain.InstallationAccount{}, fmt.Errorf("read installation id=%d: %w", id, err)
	}
	account := installation.Account
	return domain.InstallationAccount{Login: account.Login, ID: account.ID, Organization: account.Type == "Organization"}, nil
}

// InstallationToken returns a token that reads the repositories installation id may, for an hour, or fails with
// domain.ErrNoSuchInstallation.
func (a *App) InstallationToken(ctx context.Context, id int64) (string, error) {
	var token struct {
		Token string `json:"token"`
	}
	found, err := a.asApp(ctx, http.MethodPost, "/app/installations/"+strconv.FormatInt(id, 10)+"/access_tokens", &token)
	switch {
	case err == nil && !found:
		err = domain.ErrNoSuchInstallation
	case err == nil && token.Token == "":
		err = errors.New("GitHub returned no token")
	}
	if err != nil {
		return "", fmt.Errorf("get a token for installation id=%d: %w", id, err)
	}
	return token.Token, nil
}

// InstallationRepositories returns the repositories an installation's token reads, the most recently pushed first, at
// most limit of them.
func (a *App) InstallationRepositories(ctx context.Context, token string, limit int) ([]domain.GitHubRepository, error) {
	repositories, err := a.api.repositories(ctx, token, "/installation/repositories?sort=pushed", "repositories", limit)
	if err != nil {
		return nil, fmt.Errorf("list the installation's repositories: %w", err)
	}
	return repositories, nil
}

// asApp sends a request to path, signed as the app, and decodes its JSON into v, as API.get does.
func (a *App) asApp(ctx context.Context, method, path string, v any) (bool, error) {
	jwt, err := a.jwt(ctx)
	if err != nil {
		return false, err
	}
	request, err := http.NewRequestWithContext(ctx, method, a.api.baseURL+path, nil)
	if err != nil {
		return false, err
	}
	setHeaders(request, jwt, "")
	found, err := a.api.do(request, maxResponseBytes, v)
	if errors.Is(err, domain.ErrGitHubTokenRefused) {
		// GitHub refused the JWT: the key may have been rotated, so the next request reads it again.
		a.config.PrivateKey.Forget()
		err = errors.New("GitHub refused the app's JWT: check the app's client ID and private key")
	}
	return found, err
}

// jwt returns a JSON Web Token that signs the app's requests for the next ten minutes, as GitHub asks: issued a minute
// in the past, against clock drift, by the app's client ID, signed with RS256.
func (a *App) jwt(ctx context.Context) (string, error) {
	key, err := a.privateKey(ctx)
	if err != nil {
		return "", err
	}
	now := a.now()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, err := json.Marshal(map[string]any{
		"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(9 * time.Minute).Unix(), "iss": a.config.ClientID,
	})
	if err != nil {
		return "", err
	}
	unsigned := header + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign the app's JWT: %v", err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

// privateKey returns the app's RSA private key, which GitHub gives as PKCS #1 PEM, or PKCS #8 once converted. Errors
// never include the key.
func (a *App) privateKey(ctx context.Context) (*rsa.PrivateKey, error) {
	text, err := a.config.PrivateKey.Value(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the app's private key: %v", err)
	}
	key, err := ParsePrivateKey([]byte(text))
	if err != nil {
		a.config.PrivateKey.Forget()
		return nil, err
	}
	return key, nil
}

// ParsePrivateKey returns the RSA private key pemText holds, as PKCS #1 or PKCS #8 PEM. Errors never include the key.
func ParsePrivateKey(pemText []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemText)
	if block == nil {
		return nil, errors.New("parse the app's private key: it isn't PEM")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("parse the app's private key: it isn't an RSA key in PKCS #1 or PKCS #8")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("parse the app's private key: it isn't an RSA key")
	}
	return key, nil
}
