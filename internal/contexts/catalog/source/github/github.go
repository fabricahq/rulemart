// Package github looks up a library's repository in GitHub's REST API.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
)

// Client looks up repositories in GitHub's REST API.
type Client struct {
	Client *http.Client
	// BaseURL is the API's root: https://api.github.com, unless a test serves its own.
	BaseURL string
	// Token authenticates requests when it's set, which raises GitHub's rate limit.
	Token string
}

// githubRepository is the part of GitHub's repository resource ingestion uses.
type githubRepository struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Owner struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"owner"`
	// Description is null when the repository has none.
	Description *string `json:"description"`
	Private     bool    `json:"private"`
	CloneURL    string  `json:"clone_url"`
}

// avatarHost is where GitHub serves avatars; pages load avatars only from there.
const avatarHost = "https://avatars.githubusercontent.com/"

// Repository returns the public repository owner/name as GitHub describes it now, with its current spelling after
// a rename or transfer.
func (g Client) Repository(ctx context.Context, owner, name string) (domain.Repository, error) {
	found, err := g.get(ctx, "/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(name))
	if err != nil {
		return domain.Repository{}, fmt.Errorf("look up repository=%q on GitHub: %v", owner+"/"+name, err)
	}
	return repository(found), nil
}

// RepositoryByID returns the public repository whose GitHub repository ID is id, as GitHub describes it now. GitHub
// serves it at /repositories/<id>, where it redirects a renamed repository's old /repos/ URL.
func (g Client) RepositoryByID(ctx context.Context, id string) (domain.Repository, error) {
	number, err := strconv.ParseInt(id, 10, 64)
	if err != nil || number <= 0 || strconv.FormatInt(number, 10) != id {
		return domain.Repository{}, fmt.Errorf("look up repository id=%q on GitHub: expected GitHub's numeric repository ID", id)
	}
	found, err := g.get(ctx, "/repositories/"+id)
	if err == nil && found.ID != number {
		err = fmt.Errorf("GitHub answered with repository %d", found.ID)
	}
	if err != nil {
		return domain.Repository{}, fmt.Errorf("look up repository id=%q on GitHub: %v", id, err)
	}
	return repository(found), nil
}

// repository returns found as the catalog describes a repository.
func repository(found githubRepository) domain.Repository {
	repo := domain.Repository{
		Host: domain.GitHub, ID: strconv.FormatInt(found.ID, 10), Owner: found.Owner.Login, Name: found.Name,
		CloneURL: found.CloneURL,
	}
	if found.Description != nil {
		repo.Description = *found.Description
	}
	if strings.HasPrefix(found.Owner.AvatarURL, avatarHost) {
		repo.OwnerAvatarURL = found.Owner.AvatarURL
	}
	return repo
}

// get requests the repository resource at path, below BaseURL, and decodes the public repository it returns.
func (g Client) get(ctx context.Context, path string) (githubRepository, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.BaseURL+path, nil)
	if err != nil {
		return githubRepository{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}
	resp, err := g.Client.Do(req)
	if err != nil {
		return githubRepository{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return githubRepository{}, fmt.Errorf("read response: %v", err)
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return githubRepository{}, errors.New("GitHub has no such public repository")
	case resp.StatusCode != http.StatusOK:
		return githubRepository{}, fmt.Errorf("GitHub answered %s", resp.Status)
	}
	var found githubRepository
	if err := json.Unmarshal(body, &found); err != nil {
		return githubRepository{}, fmt.Errorf("decode response: %v", err)
	}
	switch {
	case found.Private:
		return githubRepository{}, errors.New("the repository is private; Rulemart lists public libraries only")
	case found.ID <= 0 || found.Owner.Login == "" || found.Name == "":
		return githubRepository{}, errors.New("GitHub's response has no repository ID, owner, or name")
	case !strings.HasPrefix(found.CloneURL, "https://github.com/"):
		return githubRepository{}, fmt.Errorf("GitHub's clone URL %q isn't on https://github.com", found.CloneURL)
	}
	return found, nil
}
