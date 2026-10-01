package github_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/github"
)

func TestGitHubRepositoryReturnsTheRepositoryAsGitHubSpellsIt(t *testing.T) {
	server := serve(t, http.StatusOK, `{"id": 1234, "name": "Code-Rules", "private": false, "description": null,
		"clone_url": "https://github.com/FabricaHQ/Code-Rules.git",
		"owner": {"login": "FabricaHQ", "avatar_url": "https://avatars.githubusercontent.com/u/9?v=4"}}`)

	repo, err := github.Client{Client: server.Client(), BaseURL: server.URL}.Repository(context.Background(), "fabricahq", "code-rules")

	if err != nil {
		t.Fatal(err)
	}
	want := domain.Repository{Host: "github", ID: "1234", Owner: "FabricaHQ", Name: "Code-Rules",
		OwnerAvatarURL: "https://avatars.githubusercontent.com/u/9?v=4", CloneURL: "https://github.com/FabricaHQ/Code-Rules.git"}
	if repo != want {
		t.Fatalf("got %+v, want %+v", repo, want)
	}
}

func TestGitHubRepositoryRejectsRepositoriesRulemartCantList(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
	}{
		"missing":   {http.StatusNotFound, `{"message": "Not Found"}`},
		"private":   {http.StatusOK, `{"id": 1, "name": "r", "private": true, "clone_url": "https://github.com/o/r.git", "owner": {"login": "o"}}`},
		"elsewhere": {http.StatusOK, `{"id": 1, "name": "r", "clone_url": "https://example.com/o/r.git", "owner": {"login": "o"}}`},
	} {
		t.Run(name, func(t *testing.T) {
			server := serve(t, tc.status, tc.body)

			_, err := github.Client{Client: server.Client(), BaseURL: server.URL}.Repository(context.Background(), "o", "r")

			if err == nil {
				t.Fatal("accepted the repository")
			}
		})
	}
}

// serve serves body with status for any request.
func serve(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}
