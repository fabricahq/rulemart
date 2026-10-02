package domain

import (
	"strings"
	"testing"
)

func TestNewIdentityKeepsWhatGitHubReportsForAUser(t *testing.T) {
	got, err := NewIdentity(583231, "octocat", "https://avatars.githubusercontent.com/u/583231?v=4")
	if err != nil {
		t.Fatal(err)
	}
	want := Identity{GitHubUserID: 583231, Login: "octocat", AvatarURL: "https://avatars.githubusercontent.com/u/583231?v=4"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestNewIdentityRefusesAUserGitHubWouldNotReport(t *testing.T) {
	for name, tc := range map[string]struct {
		id    int64
		login string
	}{
		"no ID":                      {0, "octocat"},
		"a negative ID":              {-1, "octocat"},
		"no login":                   {1, ""},
		"a login of 40 characters":   {1, strings.Repeat("a", 40)},
		"a login with a slash":       {1, "octo/cat"},
		"a login with a dot":         {1, "octo.cat"},
		"a login with markup":        {1, "<b>"},
		"a login with a space":       {1, "octo cat"},
		"a login with a non-ASCII a": {1, "octocаt"},
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := NewIdentity(tc.id, tc.login, ""); err == nil {
				t.Errorf("got %+v, want an error", got)
			}
		})
	}
}

// An avatar is shown with <img>, which the pages' content security policy allows only from GitHub's avatar host.
func TestNewIdentityDropsAnAvatarOffGitHubsAvatarHost(t *testing.T) {
	for _, avatar := range []string{
		"",
		"http://avatars.githubusercontent.com/u/1",
		"https://avatars.githubusercontent.com.evil.example/u/1",
		"https://evil.example/https://avatars.githubusercontent.com/u/1",
		`https://avatars.githubusercontent.com/u/1" onerror="alert(1)`,
	} {
		got, err := NewIdentity(1, "octocat", avatar)
		if err != nil {
			t.Fatal(err)
		}
		if got.AvatarURL != "" {
			t.Errorf("NewIdentity kept the avatar %q", avatar)
		}
	}
	// A login of 39 characters, GitHub's longest, is one.
	if _, err := NewIdentity(1, strings.Repeat("a", 39), ""); err != nil {
		t.Errorf("a login of 39 characters: %v", err)
	}
}

// Enterprise Managed Users' logins end with an underscore and their enterprise's short code, such as octocat_acme.
func TestNewIdentityAcceptsAnEnterpriseManagedUsersLogin(t *testing.T) {
	if _, err := NewIdentity(1, "octocat_acme", ""); err != nil {
		t.Error(err)
	}
}

func TestNewSessionTokensAreDistinctAndParseBack(t *testing.T) {
	seen := map[SessionToken]bool{}
	for range 100 {
		token := NewSessionToken()
		if seen[token] {
			t.Fatalf("NewSessionToken repeated %q", token)
		}
		seen[token] = true
		if parsed, ok := ParseSessionToken(string(token)); !ok || parsed != token {
			t.Fatalf("ParseSessionToken(%q) = %q, %v", token, parsed, ok)
		}
		if len(token.Hash()) != 32 {
			t.Fatalf("a token's hash has %d bytes, want 32", len(token.Hash()))
		}
	}
}

func TestParseSessionTokenRefusesWhatNewSessionTokenNeverMakes(t *testing.T) {
	token := string(NewSessionToken())
	for name, text := range map[string]string{
		"empty":                    "",
		"one character short":      token[:len(token)-1],
		"one character long":       token + "A",
		"standard base64 padding":  token + "=",
		"a character outside it":   token[:len(token)-1] + "!",
		"a non-canonical encoding": token[:len(token)-1] + nonCanonicalLast(token),
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := ParseSessionToken(text); ok {
				t.Errorf("ParseSessionToken(%q) accepted it", text)
			}
		})
	}
}

// nonCanonicalLast returns a last character that decodes to the same bytes as token's, with a padding bit set, which a
// strict decoder refuses. 32 bytes leave two padding bits in the last of 43 characters.
func nonCanonicalLast(token string) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	i := strings.IndexByte(alphabet, token[len(token)-1])
	return string(alphabet[i^1])
}
