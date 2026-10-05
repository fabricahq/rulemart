// Package githubapp acts as a GitHub App through GitHub's REST API: it signs the app's requests with a JWT made from
// its private key, finds the app's installations, by ID or by the account they're on, and mints installation tokens,
// which act for an installation for an hour. A TokenSource keeps an installation's token, and mints the next one
// before it expires. It owns no product concept.
package githubapp

import (
	"bytes"
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
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// APIURL is GitHub's REST API.
const APIURL = "https://api.github.com"

// ErrNoSuchInstallation reports that GitHub has no installation of the app where a request looked, or refuses the
// installation a token, as it does a suspended one.
var ErrNoSuchInstallation = errors.New("no such installation of the GitHub App")

// requestTimeout bounds each request to GitHub when Config gives no client.
const requestTimeout = 10 * time.Second

// maxResponseBytes bounds what the app reads of each response. GitHub's are under a few KiB.
const maxResponseBytes = 64 << 10

// Secret holds the app's private key, such as an SSM parameter's. Forget drops a value GitHub refused, so the next
// request reads it again, such as after a rotation.
type Secret interface {
	Value(ctx context.Context) (string, error)
	Forget()
}

// Config names a GitHub App and where to reach GitHub.
type Config struct {
	// Issuer names the app in its JWTs: its client ID, as GitHub advises, or its numeric ID, which GitHub also takes.
	Issuer string
	// PrivateKey holds the app's private key, in PEM, as GitHub gives it.
	PrivateKey Secret
	// BaseURL is GitHub's REST API, without a trailing slash: APIURL, or a test's server. Empty is APIURL.
	BaseURL string
	// HTTP sends the requests; nil is a client whose requests time out after requestTimeout.
	HTTP *http.Client
}

// App acts as one GitHub App. Its methods are safe for concurrent use.
type App struct {
	config Config
	// now returns the time a JWT is issued at, and a TokenSource's clock, which a test fixes.
	now func() time.Time
}

// New returns the app config describes.
func New(config Config) *App {
	config.BaseURL = strings.TrimSuffix(cmp.Or(config.BaseURL, APIURL), "/")
	if config.HTTP == nil {
		config.HTTP = &http.Client{Timeout: requestTimeout}
	}
	return &App{config: config, now: time.Now}
}

// Installation is one installation of the app.
type Installation struct {
	ID      int64
	Account Account
	// Suspended is true while the account's owner has suspended the app there: GitHub still describes the installation,
	// but refuses it a token.
	Suspended bool
}

// Account is the GitHub account an installation is on.
type Account struct {
	Login string
	ID    int64
	// Organization is true for an organization's account, and false for a user's.
	Organization bool
}

// installationJSON is an installation as GitHub describes it.
type installationJSON struct {
	ID      int64 `json:"id"`
	Account struct {
		Login string `json:"login"`
		ID    int64  `json:"id"`
		Type  string `json:"type"`
	} `json:"account"`
	// SuspendedAt is when the account's owner suspended the app there, or nil while they haven't.
	SuspendedAt *time.Time `json:"suspended_at"`
}

func (in installationJSON) installation() Installation {
	account := in.Account
	return Installation{
		ID:        in.ID,
		Account:   Account{Login: account.Login, ID: account.ID, Organization: account.Type == "Organization"},
		Suspended: in.SuspendedAt != nil,
	}
}

// Installation returns the installation id, or fails with ErrNoSuchInstallation when GitHub answers 404: a 403, which
// refuses to say, fails without it, so a caller never takes an installation GitHub merely wouldn't describe for gone.
func (a *App) Installation(ctx context.Context, id int64) (Installation, error) {
	var found installationJSON
	ok, err := a.asApp(ctx, http.MethodGet, "/app/installations/"+strconv.FormatInt(id, 10), &found)
	if err == nil && !ok {
		err = ErrNoSuchInstallation
	}
	if err != nil {
		return Installation{}, fmt.Errorf("read installation id=%d: %w", id, err)
	}
	return found.installation(), nil
}

// AccountInstallation returns the app's installation on the account login, an organization's or a user's, or fails
// with ErrNoSuchInstallation when GitHub answers 404 for both.
func (a *App) AccountInstallation(ctx context.Context, login string) (Installation, error) {
	var found installationJSON
	ok, err := a.asApp(ctx, http.MethodGet, "/orgs/"+url.PathEscape(login)+"/installation", &found)
	if err == nil && !ok {
		// GitHub keeps a user's installation at another address, and answers 404 for an organization that isn't one.
		ok, err = a.asApp(ctx, http.MethodGet, "/users/"+url.PathEscape(login)+"/installation", &found)
	}
	if err == nil && !ok {
		err = ErrNoSuchInstallation
	}
	if err != nil {
		return Installation{}, fmt.Errorf("find the installation on account=%q: %w", login, err)
	}
	return found.installation(), nil
}

// Token is an installation's token.
type Token struct {
	Value     string
	ExpiresAt time.Time
}

// InstallationToken mints a token that acts for installation id until it expires, an hour from now, or fails with
// ErrNoSuchInstallation, which GitHub answers with 404 for an installation it doesn't have and 403 for one it has
// suspended.
func (a *App) InstallationToken(ctx context.Context, id int64) (Token, error) {
	var token struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	ok, err := a.asApp(ctx, http.MethodPost, "/app/installations/"+strconv.FormatInt(id, 10)+"/access_tokens", &token)
	switch {
	case errors.Is(err, errForbidden), err == nil && !ok:
		err = ErrNoSuchInstallation
	case err != nil:
	case token.Token == "" || token.ExpiresAt.IsZero():
		err = errors.New("GitHub returned no token, or no expiry")
	}
	if err != nil {
		return Token{}, fmt.Errorf("get a token for installation id=%d: %w", id, err)
	}
	return Token{Value: token.Token, ExpiresAt: token.ExpiresAt}, nil
}

// errForbidden reports a 403 that isn't a rate limit's, which GitHub answers for what the app may not do, such as
// minting a token for a suspended installation. Only a caller can tell what it means, so asApp fails with it.
var errForbidden = errors.New("GitHub answered 403 Forbidden")

// asApp sends a request to path, signed as the app, and decodes its JSON into v. It returns false for 404. It fails
// with errForbidden for a 403 that isn't a rate limit's, for a rate limit, and when GitHub refuses the app's JWT,
// after forgetting the key, which may have been rotated. Errors never include the key or the JWT.
func (a *App) asApp(ctx context.Context, method, path string, v any) (bool, error) {
	jwt, err := a.jwt(ctx)
	if err != nil {
		return false, err
	}
	request, err := http.NewRequestWithContext(ctx, method, a.config.BaseURL+path, nil)
	if err != nil {
		return false, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "Rulemart")
	request.Header.Set("Authorization", "Bearer "+jwt)
	response, err := a.config.HTTP.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode == http.StatusUnauthorized:
		a.config.PrivateKey.Forget()
		return false, errors.New("GitHub refused the app's JWT: check the app's ID and private key")
	case RateLimited(response):
		return false, fmt.Errorf("GitHub answered %s: rate limited", response.Status)
	case response.StatusCode == http.StatusNotFound:
		return false, nil
	case response.StatusCode == http.StatusForbidden:
		return false, errForbidden
	case response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated:
		return false, fmt.Errorf("GitHub answered %s", response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	switch {
	case err != nil:
		return false, fmt.Errorf("read the response: %v", err)
	case len(body) > maxResponseBytes:
		return false, fmt.Errorf("the response is larger than %d bytes", maxResponseBytes)
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(v); err != nil {
		return false, fmt.Errorf("decode the response: %v", err)
	}
	return true, nil
}

// maxRateLimitMessageBytes bounds what RateLimited reads of a 403's body: GitHub's error messages are under a KiB.
const maxRateLimitMessageBytes = 4 << 10

// RateLimited reports whether GitHub's response says the caller made too many requests, rather than that it can't see
// what it asked for: a 429, or a 403 with its primary rate limit's remaining count of 0, a secondary limit's
// Retry-After, or a message about a rate limit, which a secondary limit may send without either header. It reads a
// 403's body, and takes one it can't read for a rate limit's, so a caller fails rather than skip what it couldn't tell.
func RateLimited(response *http.Response) bool {
	switch {
	case response.StatusCode == http.StatusTooManyRequests:
		return true
	case response.StatusCode != http.StatusForbidden:
		return false
	case response.Header.Get("X-RateLimit-Remaining") == "0" || response.Header.Get("Retry-After") != "":
		return true
	}
	message, err := io.ReadAll(io.LimitReader(response.Body, maxRateLimitMessageBytes))
	return err != nil || strings.Contains(strings.ToLower(string(message)), "rate limit")
}

// jwt returns a JSON Web Token that signs the app's requests for the next ten minutes, as GitHub asks: issued a minute
// in the past, against clock drift, by the app's issuer, signed with RS256.
func (a *App) jwt(ctx context.Context) (string, error) {
	key, err := a.privateKey(ctx)
	if err != nil {
		return "", err
	}
	now := a.now()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, err := json.Marshal(map[string]any{
		"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(9 * time.Minute).Unix(), "iss": a.config.Issuer,
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

// privateKey returns the app's RSA private key. Errors never include the key.
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

// ParsePrivateKey returns the RSA private key pemText holds, as PKCS #1 PEM, which GitHub gives, or PKCS #8 PEM, once
// converted. Errors never include the key.
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
