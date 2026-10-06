// Package githubapptest is a fake of the parts of GitHub's REST API that act as a GitHub App: it checks the app's JWTs,
// finds its installations by ID or by account, and mints installation tokens that expire TokenLifetime after it mints
// them. Tests serve it with httptest.
package githubapptest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TokenLifetime is how long a token the fake mints lasts, as GitHub's do.
const TokenLifetime = time.Hour

// Fake is a fake GitHub for one GitHub App. Set its exported fields before serving it, and change its installations
// with SetInstallations while it serves; Handler serves it.
type Fake struct {
	// Issuer is what the app's JWTs must name as their issuer: its client ID or its numeric ID. Key is what they must
	// be signed with.
	Issuer string
	Key    *rsa.PrivateKey
	// Installations are the app's installations; read them under mu once the fake serves.
	Installations []Installation
	// Now is the fake's clock, which checks JWTs and dates tokens; nil is time.Now.
	Now func() time.Time

	mu sync.Mutex
	// failMints answers each request for a token with 502 while it's true.
	failMints bool
	// onMint, unless nil, runs as the fake answers each request for a token, before it answers.
	onMint func()
	// mints counts the tokens the fake minted, and lookups the requests for an account's installation.
	mints, lookups int
}

// Installation is an installation of the app on Account, a user's or an organization's.
type Installation struct {
	ID           int64
	Account      string
	AccountID    int64
	Organization bool
	// Suspended is true while the account's owner has suspended the app there: GitHub still describes the installation
	// but refuses it a token.
	Suspended bool
}

// NewKey returns a new RSA key for a fake app.
func NewKey() *rsa.PrivateKey {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return key
}

// KeyPEM returns key in PKCS #1 PEM, as GitHub gives an app's private key.
func KeyPEM(key *rsa.PrivateKey) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}

// Token returns the token the fake mints as its nth for installation id, counting from 1.
func Token(id int64, n int) string {
	return "ghs_fake_" + strconv.FormatInt(id, 10) + "_" + strconv.Itoa(n)
}

// FailMints makes the fake answer each request for a token with 502 while fail is true, as GitHub failing does.
func (f *Fake) FailMints(fail bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failMints = fail
}

// OnMint makes the fake call fn as it answers each request for a token, before it answers, such as to advance a test's
// clock as though GitHub were slow to answer. A nil fn stops it.
func (f *Fake) OnMint(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onMint = fn
}

// SetInstallations replaces the app's installations, such as when it's reinstalled, while the fake serves.
func (f *Fake) SetInstallations(installations []Installation) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Installations = installations
}

// installations returns the app's installations.
func (f *Fake) installations() []Installation {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Installations
}

// Mints returns how many tokens the fake has minted.
func (f *Fake) Mints() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.mints
}

// Lookups returns how many requests for an account's installation the fake has answered.
func (f *Fake) Lookups() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lookups
}

// Handler serves f as GitHub's REST API.
func (f *Fake) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /app/installations/{id}", f.installation)
	mux.HandleFunc("POST /app/installations/{id}/access_tokens", f.accessToken)
	mux.HandleFunc("GET /orgs/{account}/installation", f.accountInstallation(true))
	mux.HandleFunc("GET /users/{account}/installation", f.accountInstallation(false))
	return mux
}

func (f *Fake) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now()
}

func (f *Fake) installation(w http.ResponseWriter, r *http.Request) {
	if in, ok := f.findInstallation(w, r); ok {
		writeInstallation(w, in)
	}
}

// accountInstallation answers the request for the app's installation on an account: an organization's when
// organization is true, and a user's otherwise.
func (f *Fake) accountInstallation(organization bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !f.signedByApp(w, r) {
			return
		}
		f.mu.Lock()
		f.lookups++
		f.mu.Unlock()
		for _, in := range f.installations() {
			if strings.EqualFold(in.Account, r.PathValue("account")) && in.Organization == organization {
				writeInstallation(w, in)
				return
			}
		}
		notFound(w)
	}
}

func (f *Fake) accessToken(w http.ResponseWriter, r *http.Request) {
	in, ok := f.findInstallation(w, r)
	if !ok {
		return
	}
	f.mu.Lock()
	onMint := f.onMint
	f.mu.Unlock()
	if onMint != nil {
		onMint()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case f.failMints:
		http.Error(w, `{"message":"Server Error"}`, http.StatusBadGateway)
		return
	case in.Suspended:
		http.Error(w, `{"message":"This installation has been suspended"}`, http.StatusForbidden)
		return
	}
	f.mints++
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"token": Token(in.ID, f.mints), "expires_at": f.now().Add(TokenLifetime).UTC().Format(time.RFC3339),
	})
}

// findInstallation returns the installation the request's path names, once its JWT is the app's, or answers 401 or
// 404 and returns false.
func (f *Fake) findInstallation(w http.ResponseWriter, r *http.Request) (Installation, bool) {
	if !f.signedByApp(w, r) {
		return Installation{}, false
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	for _, in := range f.installations() {
		if err == nil && in.ID == id {
			return in, true
		}
	}
	notFound(w)
	return Installation{}, false
}

// signedByApp reports whether the request carries a JWT the app signed, or answers 401 and returns false.
func (f *Fake) signedByApp(w http.ResponseWriter, r *http.Request) bool {
	if err := CheckJWT(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), f.Issuer, f.Key, f.now()); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": err.Error()})
		return false
	}
	return true
}

// CheckJWT reports why token isn't a JWT the app issuer names signed with key at now, as GitHub checks one: signed
// with RS256 by key, issued by issuer, already valid, unexpired, and lasting at most ten minutes.
func CheckJWT(token, issuer string, key *rsa.PrivateKey, now time.Time) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || key == nil {
		return fmt.Errorf("a JWT could not be decoded")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], signature); err != nil {
		return fmt.Errorf("a JWT signature does not match")
	}
	claims, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return err
	}
	var c struct {
		Iss string `json:"iss"`
		Exp int64  `json:"exp"`
		Iat int64  `json:"iat"`
	}
	if err := json.Unmarshal(claims, &c); err != nil {
		return err
	}
	if c.Iss != issuer || c.Exp <= now.Unix() || c.Iat > now.Unix() || c.Exp-c.Iat > 600 {
		return fmt.Errorf("the JWT's claims aren't the app's")
	}
	return nil
}

func writeInstallation(w http.ResponseWriter, in Installation) {
	kind := "User"
	if in.Organization {
		kind = "Organization"
	}
	// GitHub says when the account's owner suspended the app there, and null while they haven't.
	var suspendedAt any
	if in.Suspended {
		suspendedAt = "2026-09-01T00:00:00Z"
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": in.ID, "account": map[string]any{"login": in.Account, "id": in.AccountID, "type": kind}, "suspended_at": suspendedAt,
	})
}

func notFound(w http.ResponseWriter) {
	http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
}
