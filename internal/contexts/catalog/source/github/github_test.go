package github_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/github"
	"github.com/fabricahq/rulemart/internal/platform/secret"
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

// A missing or private repository is one GitHub has no public repository for, which a listing's lister can act on;
// any other refusal, such as a rate limit, is GitHub's or Rulemart's.
func TestGitHubRepositoryRejectsRepositoriesRulemartCantList(t *testing.T) {
	for name, tc := range map[string]struct {
		status       int
		body         string
		noPublicRepo bool
	}{
		"missing":      {http.StatusNotFound, `{"message": "Not Found"}`, true},
		"private":      {http.StatusOK, `{"id": 1, "name": "r", "private": true, "clone_url": "https://github.com/o/r.git", "owner": {"login": "o"}}`, true},
		"elsewhere":    {http.StatusOK, `{"id": 1, "name": "r", "clone_url": "https://example.com/o/r.git", "owner": {"login": "o"}}`, false},
		"rate limited": {http.StatusForbidden, `{"message": "API rate limit exceeded"}`, false},
		"failing":      {http.StatusBadGateway, `{}`, false},
	} {
		t.Run(name, func(t *testing.T) {
			server := serve(t, tc.status, tc.body)

			_, err := github.Client{Client: server.Client(), BaseURL: server.URL}.Repository(context.Background(), "o", "r")

			if err == nil {
				t.Fatal("accepted the repository")
			}
			if got := errors.Is(err, domain.ErrNoPublicRepository); got != tc.noPublicRepo {
				t.Fatalf("%v: errors.Is(err, domain.ErrNoPublicRepository) = %v, want %v", err, got, tc.noPublicRepo)
			}
		})
	}
}

// A token authenticates each request, and one GitHub refuses is read again on the next.
func TestGitHubAuthenticatesWithTheTokenAndForgetsARefusedOne(t *testing.T) {
	var authorizations []string
	status := http.StatusUnauthorized
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorizations = append(authorizations, r.Header.Get("Authorization"))
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"id": 1, "name": "r", "clone_url": "https://github.com/o/r.git", "owner": {"login": "o"}}`))
	}))
	t.Cleanup(server.Close)
	parameter := &rotatingParameter{values: []string{"old-token", "new-token"}}
	client := github.Client{Client: server.Client(), BaseURL: server.URL, Token: secret.FromParameter(parameter, "/token")}

	_, refused := client.Repository(context.Background(), "o", "r")
	status = http.StatusOK
	_, err := client.Repository(context.Background(), "o", "r")

	if refused == nil || strings.Contains(refused.Error(), "old-token") || err != nil {
		t.Fatalf("got %v, then %v", refused, err)
	}
	if want := []string{"Bearer old-token", "Bearer new-token"}; !slices.Equal(authorizations, want) {
		t.Fatalf("sent %q, want %q", authorizations, want)
	}
}

// rotatingParameter is an SSM parameter whose value changes to the next of values each time it's read.
type rotatingParameter struct{ values []string }

func (p *rotatingParameter) GetParameter(context.Context, *ssm.GetParameterInput, ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	value := p.values[0]
	p.values = p.values[1:]
	return &ssm.GetParameterOutput{Parameter: &types.Parameter{Value: aws.String(value)}}, nil
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

// Vetting names a repository by its ID, which survives renames, so the worker looks it up by ID and gets its current
// name.
func TestGitHubRepositoryByIDLooksTheRepositoryUpByItsID(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"id": 1234, "name": "renamed", "private": false, "description": "Rules.",
			"clone_url": "https://github.com/example/renamed.git", "owner": {"login": "example"}}`))
	}))
	t.Cleanup(server.Close)

	repo, err := github.Client{Client: server.Client(), BaseURL: server.URL}.RepositoryByID(context.Background(), "1234")

	if err != nil {
		t.Fatal(err)
	}
	if path != "/repositories/1234" || repo.ID != "1234" || repo.FullName() != "example/renamed" || repo.CloneURL != "https://github.com/example/renamed.git" {
		t.Fatalf("requested %s and got %+v", path, repo)
	}
}

func TestGitHubRepositoryByIDRejectsAnotherRepositoryOrAnInvalidID(t *testing.T) {
	server := serve(t, http.StatusOK, `{"id": 99, "name": "r", "clone_url": "https://github.com/o/r.git", "owner": {"login": "o"}}`)
	client := github.Client{Client: server.Client(), BaseURL: server.URL}
	for _, id := range []string{"1234", "", "0", "12a", "../repos/o/r"} {
		t.Run(id, func(t *testing.T) {
			if repo, err := client.RepositoryByID(context.Background(), id); err == nil {
				t.Fatalf("accepted %+v for ID %q", repo, id)
			}
		})
	}
}
