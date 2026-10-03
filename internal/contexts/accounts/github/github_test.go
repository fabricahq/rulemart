package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
)

type fixedSecret string

func (s fixedSecret) Value(context.Context) (string, error) { return string(s), nil }
func (fixedSecret) Forget()                                 {}

type failingSecret struct{}

func (failingSecret) Value(context.Context) (string, error) {
	return "", errors.New("parameter /rulemart/test/github-client-secret not found")
}
func (failingSecret) Forget() {}

// rotatedSecret counts how often GitHub's refusal made the client forget it.
type rotatedSecret struct{ forgotten int }

func (*rotatedSecret) Value(context.Context) (string, error) { return "client-secret", nil }
func (s *rotatedSecret) Forget()                             { s.forgotten++ }

const (
	testCode     = "code-from-github"
	testVerifier = "verifier-the-browser-kept"
	testToken    = "gho_token-github-issued"
	testRedirect = "https://rulemart.example/account/github/callback"
)

// fakeGitHub answers as GitHub's token endpoint and user API do for one authorization: tokenBody and userBody are
// the responses, and requests that don't carry what GitHub needs fail the test.
func fakeGitHub(t *testing.T, tokenBody, userBody string) *Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("the exchange asks for %q", r.Header.Get("Accept"))
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		want := url.Values{
			"client_id": {"client-id"}, "client_secret": {"client-secret"}, "code": {testCode},
			"redirect_uri": {testRedirect}, "code_verifier": {testVerifier},
		}
		if r.PostForm.Encode() != want.Encode() {
			t.Errorf("the exchange sent %v, want %v", r.PostForm, want)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, tokenBody)
	})
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+testToken {
			t.Errorf("the user request authorizes with %q", got)
		}
		if userBody == "" {
			http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, userBody)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	c := New("client-id", fixedSecret("client-secret"))
	c.tokenURL = server.URL + "/login/oauth/access_token"
	c.userURL = server.URL + "/user"
	return c
}

const (
	grantedToken = `{"access_token":"` + testToken + `","token_type":"bearer","scope":""}`
	octocatUser  = `{"login":"octocat","id":583231,"avatar_url":"https://avatars.githubusercontent.com/u/583231?v=4","name":"The Octocat","email":"octocat@github.com"}`
)

// The token comes back with the user, since the session keeps it to read the visitor's organizations and repositories.
func TestIdentifyExchangesTheCodeAndReturnsTheUserItSignsInWithTheToken(t *testing.T) {
	c := fakeGitHub(t, grantedToken, octocatUser)
	got, token, err := c.Identify(context.Background(), testCode, testVerifier, testRedirect)
	if err != nil {
		t.Fatal(err)
	}
	want := domain.Identity{GitHubUserID: 583231, Login: "octocat", AvatarURL: "https://avatars.githubusercontent.com/u/583231?v=4", Name: "The Octocat"}
	if got != want || token != testToken {
		t.Errorf("got %+v with the token %q, want %+v with %q", got, token, want, testToken)
	}
}

// A user who set no name has GitHub's null, which leaves the identity's name empty, so pages show the login.
func TestIdentifyKeepsNoNameForAUserWhoSetNone(t *testing.T) {
	c := fakeGitHub(t, grantedToken, `{"login":"octocat","id":583231,"avatar_url":"","name":null}`)
	got, _, err := c.Identify(context.Background(), testCode, testVerifier, testRedirect)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "" || got.DisplayName() != "octocat" {
		t.Errorf("got the name %q, shown as %q; want none, shown as the login", got.Name, got.DisplayName())
	}
}

func TestIdentifyFailsWithoutLeakingTheCodeVerifierOrToken(t *testing.T) {
	for name, tc := range map[string]struct {
		tokenBody, userBody string
		want                string
	}{
		"GitHub refuses the code": {
			`{"error":"bad_verification_code","error_description":"The code passed is incorrect or expired."}`, octocatUser,
			"GitHub refused the code: bad_verification_code",
		},
		"GitHub refuses with text that isn't a code": {
			`{"error":"<script>` + testCode + `</script>"}`, octocatUser, "an unrecognized error",
		},
		"GitHub returns no token": {`{}`, octocatUser, "GitHub returned no token"},
		"GitHub returns a token too long to keep": {
			`{"access_token":"` + strings.Repeat("t", domain.MaxGitHubTokenLength+1) + `"}`, octocatUser, "GitHub returned no token",
		},
		"the exchange isn't JSON":         {`<html>`, octocatUser, "decode the response"},
		"GitHub refuses the token":        {grantedToken, "", "GitHub answered 401"},
		"the user has no ID":              {grantedToken, `{"login":"octocat"}`, "want a positive user ID"},
		"the user's login isn't GitHub's": {grantedToken, `{"login":"octo/cat","id":1}`, "isn't one GitHub gives"},
		"the user is too large to read": {
			grantedToken, `{"login":"octocat","id":1,"bio":"` + strings.Repeat("x", maxResponseBytes) + `"}`, "larger than",
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := fakeGitHub(t, tc.tokenBody, tc.userBody)
			_, _, err := c.Identify(context.Background(), testCode, testVerifier, testRedirect)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want an error saying %q", err, tc.want)
			}
			for _, secret := range []string{testCode, testVerifier, testToken, "client-secret"} {
				if strings.Contains(err.Error(), secret) {
					t.Errorf("the error %q includes %q", err, secret)
				}
			}
		})
	}
}

// GitHub refusing the code, such as one already used or expired, is the visitor's to retry by signing in again; a
// misconfigured app or an unreachable GitHub isn't.
func TestIdentifyTellsARefusedCodeFromAFailureOfGitHubOrRulemart(t *testing.T) {
	for name, tc := range map[string]struct {
		tokenBody string
		refused   bool
	}{
		"a code GitHub doesn't know":   {`{"error":"bad_verification_code"}`, true},
		"an unusual refusal":           {`{"error":"unverified_user_email"}`, true},
		"the app's wrong secret":       {`{"error":"incorrect_client_credentials"}`, false},
		"the app's wrong callback URL": {`{"error":"redirect_uri_mismatch"}`, false},
		"no token":                     {`{}`, false},
	} {
		t.Run(name, func(t *testing.T) {
			c := fakeGitHub(t, tc.tokenBody, octocatUser)
			_, _, err := c.Identify(context.Background(), testCode, testVerifier, testRedirect)
			if err == nil || errors.Is(err, ErrCodeRefused) != tc.refused {
				t.Errorf("got %v; want refused: %v", err, tc.refused)
			}
		})
	}
	// GitHub failing to answer is never a refusal.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	c := New("client-id", fixedSecret("client-secret"))
	c.tokenURL = server.URL
	if _, _, err := c.Identify(context.Background(), testCode, testVerifier, testRedirect); err == nil || errors.Is(err, ErrCodeRefused) {
		t.Errorf("GitHub answering 503: got %v, want an error that isn't a refusal", err)
	}
}

// A refused client secret may have been rotated, so the next sign-in reads it again.
func TestIdentifyForgetsAClientSecretGitHubRefuses(t *testing.T) {
	c := fakeGitHub(t, `{"error":"incorrect_client_credentials"}`, octocatUser)
	secret := &rotatedSecret{}
	c.secret = secret
	_, _, err := c.Identify(context.Background(), testCode, testVerifier, testRedirect)
	if err == nil || !strings.Contains(err.Error(), "incorrect_client_credentials") || secret.forgotten != 1 {
		t.Errorf("got %v with the secret forgotten %d times, want the refusal and one Forget", err, secret.forgotten)
	}
}

func TestIdentifyFailsWhenItCantReadTheClientSecret(t *testing.T) {
	c := fakeGitHub(t, grantedToken, octocatUser)
	c.secret = failingSecret{}
	if _, _, err := c.Identify(context.Background(), testCode, testVerifier, testRedirect); err == nil || !strings.Contains(err.Error(), "read the client secret") {
		t.Errorf("got %v, want an error reading the client secret", err)
	}
}

// The authorization asks for read:org, and nothing more, so GitHub shows the visitor that Rulemart reads their
// organizations and public information, and it carries the state and the PKCE challenge, with S256 named, so GitHub
// won't accept the code without the verifier.
func TestAuthorizationURLAsksForReadOrgAndCarriesStateAndChallenge(t *testing.T) {
	u, err := url.Parse(New("client-id", fixedSecret("")).AuthorizationURL("the-state", "the-challenge", testRedirect))
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme+"://"+u.Host+u.Path != authorizeURL {
		t.Errorf("the authorization is at %s", u)
	}
	want := url.Values{
		"client_id": {"client-id"}, "redirect_uri": {testRedirect}, "state": {"the-state"},
		"code_challenge": {"the-challenge"}, "code_challenge_method": {"S256"}, "scope": {"read:org"},
	}
	if got := u.Query(); got.Encode() != want.Encode() {
		t.Errorf("the authorization asks %v, want %v", got, want)
	}
}

// RFC 7636's example: https://www.rfc-editor.org/rfc/rfc7636#appendix-B
func TestChallengeIsTheVerifiersS256ChallengeAsRFC7636Computes(t *testing.T) {
	if got := Challenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"); got != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Errorf("got %q", got)
	}
}

// RFC 7636 needs a verifier of 43 to 128 unreserved characters, and each must be unguessable.
func TestNewVerifierAndNewStateAreUnguessableAndURLSafe(t *testing.T) {
	seen := map[string]bool{}
	for range 50 {
		for _, text := range []string{NewVerifier(), NewState()} {
			if len(text) != 43 || strings.Trim(text, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_") != "" {
				t.Fatalf("%q isn't 43 unreserved characters", text)
			}
			if seen[text] {
				t.Fatalf("%q repeated", text)
			}
			seen[text] = true
		}
	}
}
