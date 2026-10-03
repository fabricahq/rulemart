// Sign visitors in with the GitHub OAuth app, and read who they are.

package github

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
)

// Scope is what sign-in asks GitHub for: the organizations a visitor belongs to, including memberships they keep
// private. With it, the token reads what's public, and the visitor's organizations, and nothing private else.
const Scope = "read:org"

// GitHub's OAuth endpoints and API, which a test replaces.
const (
	authorizeURL = "https://github.com/login/oauth/authorize"
	tokenURL     = "https://github.com/login/oauth/access_token"
	userURL      = "https://api.github.com/user"
)

// requestTimeout bounds each request to GitHub, at sign-in and in every read of a visitor's GitHub account, so a
// stalled one fails well within the web function's timeout.
const requestTimeout = 5 * time.Second

// maxResponseBytes bounds what Rulemart reads of each response. GitHub's are under a few KiB.
const maxResponseBytes = 64 << 10

// ErrCodeRefused reports that GitHub refused the code a visitor came back with, such as one already used or
// expired, rather than failing: the visitor can sign in again.
var ErrCodeRefused = errors.New("GitHub refused the code")

// Secret holds the OAuth app's client secret, such as from an SSM parameter. Value may be called on every sign-in;
// Forget drops a value GitHub refused, so the next sign-in reads the secret again, such as after a rotation.
type Secret interface {
	Value(ctx context.Context) (string, error)
	Forget()
}

// Client signs visitors in with one GitHub OAuth app.
type Client struct {
	clientID string
	secret   Secret
	http     *http.Client
	// endpoints are GitHub's, or a test server's.
	authorizeURL, tokenURL, userURL string
}

// New returns a Client for the OAuth app clientID, whose client secret secret returns.
func New(clientID string, secret Secret) *Client {
	return &Client{
		clientID: clientID, secret: secret, http: &http.Client{Timeout: requestTimeout},
		authorizeURL: authorizeURL, tokenURL: tokenURL, userURL: userURL,
	}
}

// AuthorizationURL returns the GitHub page that asks the visitor to authorize Rulemart, and then sends them to
// redirectURI with a code and state. challenge is the PKCE challenge of the verifier the browser keeps. The URL asks
// for Scope.
func (c *Client) AuthorizationURL(state, challenge, redirectURI string) string {
	query := url.Values{
		"client_id":             {c.clientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {Scope},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	return c.authorizeURL + "?" + query.Encode()
}

// Identify exchanges the code GitHub sent to redirectURI, with the PKCE verifier whose challenge AuthorizationURL
// sent, for a token, and returns the user it signs in, with the token, which reads what Scope allows. Errors never
// include the code, the verifier, or the token.
func (c *Client) Identify(ctx context.Context, code, verifier, redirectURI string) (domain.Identity, string, error) {
	token, err := c.exchange(ctx, code, verifier, redirectURI)
	if err != nil {
		return domain.Identity{}, "", fmt.Errorf("sign in with GitHub: exchange the code: %w", err)
	}
	identity, err := c.user(ctx, token)
	if err != nil {
		return domain.Identity{}, "", fmt.Errorf("sign in with GitHub: read the user: %v", err)
	}
	return identity, token, nil
}

// exchange trades code for an access token.
func (c *Client) exchange(ctx context.Context, code, verifier, redirectURI string) (string, error) {
	secret, err := c.secret.Value(ctx)
	if err != nil {
		return "", fmt.Errorf("read the client secret: %v", err)
	}
	form := url.Values{
		"client_id":     {c.clientID},
		"client_secret": {secret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	var response struct {
		AccessToken string `json:"access_token"`
		// GitHub answers a refused exchange with 200 and an error code, such as bad_verification_code.
		Error string `json:"error"`
	}
	if err := c.do(request, &response); err != nil {
		return "", err
	}
	switch {
	case response.Error == "incorrect_client_credentials":
		c.secret.Forget()
		return "", errors.New("GitHub refused the client ID or secret: incorrect_client_credentials")
	case response.Error == "redirect_uri_mismatch":
		// The OAuth app's callback URL isn't the one Rulemart sent: a configuration mistake, not the visitor's.
		return "", errors.New("GitHub refused the callback URL: redirect_uri_mismatch")
	case response.Error != "":
		return "", fmt.Errorf("%w: %s", ErrCodeRefused, safeCode(response.Error))
	case response.AccessToken == "" || len(response.AccessToken) > domain.MaxGitHubTokenLength:
		return "", errors.New("GitHub returned no token Rulemart can keep")
	}
	return response.AccessToken, nil
}

// user returns the user token signs in.
func (c *Client) user(ctx context.Context, token string) (domain.Identity, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.userURL, nil)
	if err != nil {
		return domain.Identity{}, err
	}
	setHeaders(request, token, "")
	var user struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
		// Name is null when the user set none.
		Name *string `json:"name"`
	}
	if err := c.do(request, &user); err != nil {
		return domain.Identity{}, err
	}
	identity, err := domain.NewIdentity(user.ID, user.Login, user.AvatarURL)
	if err != nil || user.Name == nil {
		return identity, err
	}
	return identity.WithName(*user.Name), nil
}

// do sends request and decodes its JSON response into v, failing on any status but 200.
func (c *Client) do(request *http.Request, v any) error {
	response, err := c.http.Do(request)
	if err != nil {
		// The error names the URL, which holds no secret: the code and token travel in the body and headers.
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read the response: %v", err)
	}
	if len(body) > maxResponseBytes {
		return fmt.Errorf("the response is larger than %d bytes", maxResponseBytes)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub answered %s", response.Status)
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(v); err != nil {
		return fmt.Errorf("decode the response: %v", err)
	}
	return nil
}

// safeCode returns an OAuth error code GitHub sent, or a placeholder when it isn't one, so a log line never carries
// arbitrary text from the response.
func safeCode(code string) string {
	if len(code) > 64 || strings.Trim(code, "abcdefghijklmnopqrstuvwxyz_") != "" {
		return "an unrecognized error"
	}
	return code
}

// NewVerifier returns a new PKCE code verifier: 32 random bytes, base64url-encoded, 43 characters.
func NewVerifier() string {
	return randomText()
}

// NewState returns a new OAuth state: random text the callback must return, so it can't complete a sign-in this
// browser didn't start.
func NewState() string {
	return randomText()
}

// Challenge returns verifier's PKCE challenge by the S256 method.
func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// randomText returns 32 random bytes, base64url-encoded.
func randomText() string {
	b := make([]byte, 32)
	// crypto/rand.Read never fails; it crashes the program if the system's generator does.
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
