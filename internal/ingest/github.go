// Identify a library's GitHub repository: parse its URL, and look it up in GitHub's REST API.

package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// githubName matches a GitHub owner or repository name.
var githubName = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// ParseRepositoryURL returns the owner and name in a GitHub repository URL, such as
// https://github.com/fabricahq/code-rules-test-library. It accepts a trailing slash or .git suffix.
func ParseRepositoryURL(raw string) (owner, name string, err error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.RawQuery != "" || u.Fragment != "" {
		return "", "", fmt.Errorf("repository URL %q: expected https://github.com/<owner>/<repository>", raw)
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), "/"), ".git"), "/")
	if len(parts) != 2 || !githubName.MatchString(parts[0]) || !githubName.MatchString(parts[1]) {
		return "", "", fmt.Errorf("repository URL %q: expected https://github.com/<owner>/<repository>", raw)
	}
	return parts[0], parts[1], nil
}

// GitHub looks up repositories in GitHub's REST API.
type GitHub struct {
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
func (g GitHub) Repository(ctx context.Context, owner, name string) (Repository, error) {
	endpoint := g.BaseURL + "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Repository{}, fmt.Errorf("look up repository=%q on GitHub: %v", owner+"/"+name, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}
	found, err := g.get(req)
	if err != nil {
		return Repository{}, fmt.Errorf("look up repository=%q on GitHub: %v", owner+"/"+name, err)
	}
	repo := Repository{ID: found.ID, Owner: found.Owner.Login, Name: found.Name, CloneURL: found.CloneURL}
	if found.Description != nil {
		repo.Description = *found.Description
	}
	if strings.HasPrefix(found.Owner.AvatarURL, avatarHost) {
		repo.OwnerAvatarURL = found.Owner.AvatarURL
	}
	return repo, nil
}

// get sends req and decodes the public repository it returns.
func (g GitHub) get(req *http.Request) (githubRepository, error) {
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
