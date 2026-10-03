// GitHub tokens: the OAuth token a session keeps so the dashboard can read the visitor's GitHub account again, sealed
// so that reading the database alone reveals none.

package domain

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// tokenKeyBytes is the size of a token key: AES-256's.
const tokenKeyBytes = 32

// MaxGitHubTokenLength bounds a GitHub token a session keeps, well past GitHub's, whose tokens are 40 to 255
// characters, so a sealed token fits the sessions table's limit.
const MaxGitHubTokenLength = 900

// ErrTokenUnreadable reports a sealed token the key can't open: one sealed under another key, such as before a rotation,
// for another session, or changed since.
var ErrTokenUnreadable = errors.New("the sealed GitHub token can't be opened with this key")

// TokenKey seals and opens GitHub tokens with AES-256-GCM. The zero TokenKey can't be used; ParseTokenKey and
// NewTokenKey make one.
type TokenKey struct {
	aead cipher.AEAD
}

// ParseTokenKey returns the key text holds: 32 bytes in standard base64, as `openssl rand -base64 32` writes them.
// Errors never include the text.
func ParseTokenKey(text string) (TokenKey, error) {
	raw, err := base64.StdEncoding.Strict().DecodeString(strings.TrimSpace(text))
	if err != nil || len(raw) != tokenKeyBytes {
		return TokenKey{}, fmt.Errorf("parse token key: want %d random bytes in standard base64, such as `openssl rand -base64 32` writes", tokenKeyBytes)
	}
	return newTokenKey(raw), nil
}

// NewTokenKey returns a new random key, such as for a local build, whose sealed tokens no other process can open.
func NewTokenKey() TokenKey {
	raw := make([]byte, tokenKeyBytes)
	// crypto/rand.Read never fails; it crashes the program if the system's generator does.
	_, _ = rand.Read(raw)
	return newTokenKey(raw)
}

func newTokenKey(raw []byte) TokenKey {
	block, err := aes.NewCipher(raw)
	if err != nil {
		// aes.NewCipher fails only for a key that isn't 16, 24, or 32 bytes long.
		panic(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}
	return TokenKey{aead: aead}
}

// Seal returns token sealed for the session whose token hashes to sessionHash: a random nonce, then the ciphertext and
// its tag. It opens only with this key and that hash, so a sealed token copied to another session's row is useless. It
// fails for an empty token, or one longer than MaxGitHubTokenLength.
func (k TokenKey) Seal(token string, sessionHash []byte) ([]byte, error) {
	if token == "" || len(token) > MaxGitHubTokenLength {
		return nil, fmt.Errorf("seal GitHub token: want 1 to %d bytes, got %d", MaxGitHubTokenLength, len(token))
	}
	nonce := make([]byte, k.aead.NonceSize(), k.aead.NonceSize()+len(token)+k.aead.Overhead())
	_, _ = rand.Read(nonce)
	return k.aead.Seal(nonce, nonce, []byte(token), sessionHash), nil
}

// Open returns the token Seal sealed for the session whose token hashes to sessionHash, or fails with
// ErrTokenUnreadable.
func (k TokenKey) Open(sealed, sessionHash []byte) (string, error) {
	size := k.aead.NonceSize()
	if len(sealed) < size+k.aead.Overhead() {
		return "", ErrTokenUnreadable
	}
	token, err := k.aead.Open(nil, sealed[:size], sealed[size:], sessionHash)
	if err != nil {
		return "", ErrTokenUnreadable
	}
	return string(token), nil
}
