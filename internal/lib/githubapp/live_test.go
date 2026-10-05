package githubapp

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// liveKey is the private key the live test is given directly.
type liveKey string

func (k liveKey) Value(context.Context) (string, error) { return string(k), nil }
func (liveKey) Forget()                                 {}

// The worker reads public repositories that aren't part of the app's installation, such as a listed library's, with
// the installation's token, so this checks, against real GitHub, that the token reads a public repository outside the
// installation, its contents, tags, and releases, at the authenticated rate limit rather than the 60 requests an hour
// GitHub allows without one. It runs only when RULEMART_GITHUB_APP_ID names the app, and
// RULEMART_GITHUB_APP_PRIVATE_KEY holds its key, in PEM, or RULEMART_GITHUB_APP_PRIVATE_KEY_FILE names a file holding
// it. RULEMART_GITHUB_APP_INSTALLATION_ACCOUNT names the account whose installation it uses, fabricahq unless set, and
// RULEMART_GITHUB_PUBLIC_REPOSITORY the repository it reads, cli/cli unless set, which must be public and outside the
// installation. Run it with -v to see GitHub's answers.
func TestLiveAnInstallationTokenReadsPublicRepositoriesOutsideTheInstallation(t *testing.T) {
	id, key := os.Getenv("RULEMART_GITHUB_APP_ID"), os.Getenv("RULEMART_GITHUB_APP_PRIVATE_KEY")
	if file := os.Getenv("RULEMART_GITHUB_APP_PRIVATE_KEY_FILE"); file != "" && key == "" {
		text, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read RULEMART_GITHUB_APP_PRIVATE_KEY_FILE: %v", err)
		}
		key = string(text)
	}
	if id == "" || key == "" {
		t.Skip("set RULEMART_GITHUB_APP_ID, and RULEMART_GITHUB_APP_PRIVATE_KEY or RULEMART_GITHUB_APP_PRIVATE_KEY_FILE, to check the GitHub App against real GitHub")
	}
	account := cmp.Or(os.Getenv("RULEMART_GITHUB_APP_INSTALLATION_ACCOUNT"), "fabricahq")
	repository := cmp.Or(os.Getenv("RULEMART_GITHUB_PUBLIC_REPOSITORY"), "cli/cli")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	app := New(Config{Issuer: id, PrivateKey: liveKey(key)})

	installation, err := app.AccountInstallation(ctx, account)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("installation %d on %s (organization: %v)", installation.ID, installation.Account.Login, installation.Account.Organization)
	token, err := app.InstallationToken(ctx, installation.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("minted a token that expires at %s", token.ExpiresAt.Format(time.RFC3339))

	if installed := installationRepositories(ctx, t, token.Value); installed[strings.ToLower(repository)] {
		t.Fatalf("%s is part of the installation; set RULEMART_GITHUB_PUBLIC_REPOSITORY to a public repository outside it", repository)
	} else {
		t.Logf("the installation reads %d repositories, and %s isn't one of them", len(installed), repository)
	}

	var found struct {
		ID      int64 `json:"id"`
		Private bool  `json:"private"`
	}
	read(ctx, t, token.Value, "/repos/"+repository, &found)
	if found.Private || found.ID <= 0 {
		t.Fatalf("%s: got %+v, want a public repository", repository, found)
	}
	for _, path := range []string{
		"/repositories/" + strconv.FormatInt(found.ID, 10),
		"/repos/" + repository + "/contents/",
		"/repos/" + repository + "/tags?per_page=5",
		"/repos/" + repository + "/git/matching-refs/tags/",
		"/repos/" + repository + "/releases?per_page=5",
	} {
		var body json.RawMessage
		read(ctx, t, token.Value, path, &body)
	}
}

// installationRepositories returns the full names, in lower case, of the repositories the installation's token reads.
func installationRepositories(ctx context.Context, t *testing.T, token string) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	for page := 1; page <= 20; page++ {
		var list struct {
			Repositories []struct {
				FullName string `json:"full_name"`
			} `json:"repositories"`
		}
		read(ctx, t, token, "/installation/repositories?per_page=100&page="+strconv.Itoa(page), &list)
		for _, r := range list.Repositories {
			names[strings.ToLower(r.FullName)] = true
		}
		if len(list.Repositories) < 100 {
			return names
		}
	}
	t.Fatal("the installation reads more than 2,000 repositories")
	return nil
}

// read gets path from GitHub's API with token, logs GitHub's answer and its rate limit, and decodes its JSON into v.
// It fails the test unless GitHub answers 200 within its authenticated rate limit for the core API, at least 5,000
// requests an hour.
func read(ctx context.Context, t *testing.T, token, path string, v any) {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, APIURL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "Rulemart")
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		t.Fatalf("GET %s: read the response: %v", path, err)
	}
	h := response.Header
	answer := fmt.Sprintf("GET %s: %s, rate limit %s %s, %s remaining", path, response.Status,
		h.Get("X-RateLimit-Resource"), h.Get("X-RateLimit-Limit"), h.Get("X-RateLimit-Remaining"))
	t.Log(answer)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("%s: %.500s", answer, body)
	}
	if limit, _ := strconv.Atoi(h.Get("X-RateLimit-Limit")); limit < 5000 || h.Get("X-RateLimit-Resource") != "core" {
		t.Errorf("%s: want the authenticated core rate limit, at least 5000", answer)
	}
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("GET %s: decode the response: %v", path, err)
	}
}
