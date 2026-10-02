// Sessions: the token a signed-in browser holds, what Rulemart stores of it, and how long it lasts.

package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"time"
)

// SessionLifetime is how long a session lasts after sign-in. A session is never extended: signing in again with
// GitHub takes one click once the visitor has authorized Rulemart, and a fixed lifetime means a page view never
// writes to the database.
const SessionLifetime = 30 * 24 * time.Hour

// MaxSessions is how many sessions one account keeps: signing in on another browser beyond it ends the oldest, so a
// script that signs in over and over can't grow the table without bound.
const MaxSessions = 20

// tokenBytes is the size of a session token's randomness: 256 bits, beyond guessing.
const tokenBytes = 32

// SessionToken is the secret a signed-in browser's session cookie holds. Rulemart stores only its hash.
type SessionToken string

// NewSessionToken returns a new random token.
func NewSessionToken() SessionToken {
	b := make([]byte, tokenBytes)
	// crypto/rand.Read never fails; it crashes the program if the system's generator does.
	_, _ = rand.Read(b)
	return SessionToken(base64.RawURLEncoding.EncodeToString(b))
}

// ParseSessionToken returns text as a token, or false when it can't be one NewSessionToken made, such as a cookie
// someone edited. Checking its shape first keeps a malformed cookie from costing a database read.
func ParseSessionToken(text string) (SessionToken, bool) {
	b, err := base64.RawURLEncoding.Strict().DecodeString(text)
	if err != nil || len(b) != tokenBytes {
		return "", false
	}
	return SessionToken(text), true
}

// Hash returns the SHA-256 of the token, which is what the database stores and looks sessions up by.
func (t SessionToken) Hash() []byte {
	sum := sha256.Sum256([]byte(t))
	return sum[:]
}

// Session is a signed-in browser's session.
type Session struct {
	Token     SessionToken
	ExpiresAt time.Time
}
