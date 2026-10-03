package domain

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

const testToken = "gho_16C7e42F292c6912E7710c838347Ae178B4a"

// A sealed token opens only with its key and its session's hash, and only unchanged, so a copy of the sessions table, a
// sealed token moved to another session, or one edited in place reveals nothing.
func TestASealedTokenOpensOnlyWithItsKeyAndSession(t *testing.T) {
	key := NewTokenKey()
	session, other := NewSessionToken().Hash(), NewSessionToken().Hash()
	sealed, err := key.Seal(testToken, session)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sealed), testToken) {
		t.Fatal("the sealed token holds its text")
	}
	if got, err := key.Open(sealed, session); err != nil || got != testToken {
		t.Fatalf("opened %q, %v; want %q", got, err, testToken)
	}
	tampered := append([]byte(nil), sealed...)
	tampered[len(tampered)-1] ^= 1
	for name, open := range map[string]func() (string, error){
		"another session": func() (string, error) { return key.Open(sealed, other) },
		"another key":     func() (string, error) { return NewTokenKey().Open(sealed, session) },
		"a changed byte":  func() (string, error) { return key.Open(tampered, session) },
		"too short":       func() (string, error) { return key.Open(sealed[:20], session) },
		"nothing sealed":  func() (string, error) { return key.Open(nil, session) },
	} {
		if got, err := open(); !errors.Is(err, ErrTokenUnreadable) {
			t.Errorf("%s: opened %q, %v; want ErrTokenUnreadable", name, got, err)
		}
	}
}

// Each seal draws a new nonce, so sealing the same token twice gives two ciphertexts, and GCM's nonce is never reused.
func TestSealingATokenTwiceGivesDifferentCiphertexts(t *testing.T) {
	key, session := NewTokenKey(), NewSessionToken().Hash()
	a, _ := key.Seal(testToken, session)
	b, _ := key.Seal(testToken, session)
	if string(a) == string(b) {
		t.Fatal("sealed the same bytes twice")
	}
}

func TestSealRefusesATokenItCantKeep(t *testing.T) {
	key, session := NewTokenKey(), NewSessionToken().Hash()
	for name, token := range map[string]string{"empty": "", "too long": strings.Repeat("t", MaxGitHubTokenLength+1)} {
		if _, err := key.Seal(token, session); err == nil {
			t.Errorf("%s: sealed it", name)
		}
	}
	// The longest token it keeps fits the sessions table's limit of 1,024 bytes.
	sealed, err := key.Seal(strings.Repeat("t", MaxGitHubTokenLength), session)
	if err != nil || len(sealed) > 1024 {
		t.Errorf("sealed the longest token into %d bytes, %v; want at most 1,024", len(sealed), err)
	}
}

// The SSM parameter holds 32 random bytes in standard base64, as openssl writes them, with the newline it may keep.
func TestParseTokenKeyTakesThirtyTwoBytesOfBase64(t *testing.T) {
	raw := strings.Repeat("k", 32)
	text := base64.StdEncoding.EncodeToString([]byte(raw))
	key, err := ParseTokenKey(text + "\n")
	if err != nil {
		t.Fatal(err)
	}
	session := NewSessionToken().Hash()
	sealed, _ := key.Seal(testToken, session)
	again, _ := ParseTokenKey(text)
	if got, err := again.Open(sealed, session); err != nil || got != testToken {
		t.Errorf("the same key text didn't open the token: %q, %v", got, err)
	}
	for name, bad := range map[string]string{
		"16 bytes":     base64.StdEncoding.EncodeToString([]byte(raw[:16])),
		"33 bytes":     base64.StdEncoding.EncodeToString([]byte(raw + "k")),
		"not base64":   "not a key",
		"URL encoding": base64.URLEncoding.EncodeToString([]byte(strings.Repeat("\xff", 32))),
		"empty":        "",
	} {
		_, err := ParseTokenKey(bad)
		if err == nil {
			t.Errorf("%s: parsed it", name)
			continue
		}
		if bad != "" && strings.Contains(err.Error(), bad) {
			t.Errorf("%s: the error %q repeats the key text", name, err)
		}
	}
}

// A profile's name is the visitor's to write, so the header shows it only once trimmed, cut to GitHub's limit, and
// without characters that would change how the rest of the page reads, such as a right-to-left override.
func TestWithNameKeepsAPrintableNameWithinGitHubsLimit(t *testing.T) {
	base := Identity{GitHubUserID: 1, Login: "octocat"}
	for name, tc := range map[string]struct{ in, want string }{
		"plain":                {"Mona Lisa Octocat", "Mona Lisa Octocat"},
		"padded":               {"  Mona \n", "Mona"},
		"with an override":     {"Mona‮Lisa", "MonaLisa"},
		"with a control":       {"Mona\x07", "Mona"},
		"non-Latin":            {"山田 太郎", "山田 太郎"},
		"blank":                {" \t", ""},
		"longer than GitHub's": {strings.Repeat("é", MaxNameLength+5), strings.Repeat("é", MaxNameLength)},
	} {
		if got := base.WithName(tc.in).Name; got != tc.want {
			t.Errorf("%s: got %q, want %q", name, got, tc.want)
		}
	}
	if got := base.DisplayName(); got != "octocat" {
		t.Errorf("without a name, the display name is %q, want the login", got)
	}
	if got := base.WithName("Mona").DisplayName(); got != "Mona" {
		t.Errorf("with a name, the display name is %q, want it", got)
	}
}
