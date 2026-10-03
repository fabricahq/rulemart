// Read a visitor's GitHub account through GitHub's REST API: their organizations, the repositories of theirs and their
// organizations', what each repository's root holds, its release tags, and a file in it.

package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
)

// APIURL is GitHub's REST API.
const APIURL = "https://api.github.com"

// perPage is how many items Rulemart asks for in each page of a list: GitHub's most.
const perPage = 100

// maxListBytes bounds what Rulemart reads of a page of a list. A page of 100 repositories is about 600 KiB.
const maxListBytes = 4 << 20

// ErrTokenRefused reports a token GitHub refused, such as one whose user revoked Rulemart's authorization: signing in
// again gives a new one.
var ErrTokenRefused = domain.ErrGitHubTokenRefused

// API reads GitHub's REST API, at APIURL or a test's server, with whichever token each call names.
type API struct {
	baseURL string
	http    *http.Client
}

// NewAPI returns an API at baseURL, such as APIURL, without a trailing slash.
func NewAPI(baseURL string) *API {
	return &API{baseURL: strings.TrimSuffix(baseURL, "/"), http: &http.Client{Timeout: requestTimeout}}
}

// Organizations returns the logins of the organizations the token's user belongs to, at most domain.MaxOrganizations,
// in GitHub's order, including memberships the user keeps private, which read:org reveals.
func (a *API) Organizations(ctx context.Context, token string) ([]string, error) {
	var orgs []struct {
		Login string `json:"login"`
	}
	if _, err := a.get(ctx, token, "/user/orgs?per_page="+strconv.Itoa(domain.MaxOrganizations), "", maxListBytes, &orgs); err != nil {
		return nil, fmt.Errorf("list the user's organizations: %w", err)
	}
	logins := make([]string, 0, len(orgs))
	for _, o := range orgs {
		logins = append(logins, o.Login)
	}
	return logins, nil
}

// Repositories returns the public repositories owner owns, an organization's when organization is true and a user's
// otherwise, the most recently pushed first, at most limit of them.
func (a *API) Repositories(ctx context.Context, token, owner string, organization bool, limit int) ([]domain.GitHubRepository, error) {
	path := "/users/" + url.PathEscape(owner) + "/repos?type=owner&sort=pushed&direction=desc"
	if organization {
		path = "/orgs/" + url.PathEscape(owner) + "/repos?type=public&sort=pushed&direction=desc"
	}
	repositories, err := a.repositories(ctx, token, path, "", limit)
	if err != nil {
		return nil, fmt.Errorf("list repositories owner=%q: %w", owner, err)
	}
	return repositories, nil
}

// repositories returns the repositories the list at path holds, page by page, at most limit of them. field names the
// field of each page's object that holds them, or is empty for a page that's a list itself.
func (a *API) repositories(ctx context.Context, token, path, field string, limit int) ([]domain.GitHubRepository, error) {
	var found []domain.GitHubRepository
	for page := 1; len(found) < limit; page++ {
		var repos []repositoryJSON
		var err error
		if field == "" {
			_, err = a.get(ctx, token, path+"&per_page="+strconv.Itoa(perPage)+"&page="+strconv.Itoa(page), "", maxListBytes, &repos)
		} else {
			var object map[string]json.RawMessage
			if _, err = a.get(ctx, token, path+"&per_page="+strconv.Itoa(perPage)+"&page="+strconv.Itoa(page), "", maxListBytes, &object); err == nil {
				err = json.Unmarshal(object[field], &repos)
			}
		}
		if err != nil {
			return nil, err
		}
		for _, r := range repos {
			if repo, ok := r.repository(); ok && len(found) < limit {
				found = append(found, repo)
			}
		}
		if len(repos) < perPage {
			break
		}
	}
	return found, nil
}

// repositoryJSON is a repository as GitHub's lists describe it.
type repositoryJSON struct {
	Name     string    `json:"name"`
	Private  bool      `json:"private"`
	PushedAt time.Time `json:"pushed_at"`
	Owner    struct {
		Login string `json:"login"`
	} `json:"owner"`
}

// repository returns r as Rulemart reads it, or false when it isn't a repository GitHub would describe.
func (r repositoryJSON) repository() (domain.GitHubRepository, bool) {
	if r.Name == "" || r.Owner.Login == "" || strings.ContainsAny(r.Name+r.Owner.Login, "/?#") {
		return domain.GitHubRepository{}, false
	}
	return domain.GitHubRepository{
		Repository: domain.Repository{Owner: r.Owner.Login, Name: r.Name, Private: r.Private}, PushedAt: r.PushedAt,
	}, true
}

// RootEntries returns what the root of repo's default branch holds that a read looks for: nothing, for an empty
// repository or one the token can't see.
func (a *API) RootEntries(ctx context.Context, token string, repo domain.Repository) (domain.RootEntries, error) {
	var entries []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	found, err := a.get(ctx, token, repositoryPath(repo)+"/contents/", "", maxListBytes, &entries)
	if err != nil || !found {
		if err != nil {
			err = fmt.Errorf("list the root of repository=%q: %w", repo.FullName(), err)
		}
		return domain.RootEntries{}, err
	}
	var root domain.RootEntries
	for _, e := range entries {
		root.Manifest = root.Manifest || (e.Name == "rule-library.yaml" && e.Type == "file")
		root.CodeRules = root.CodeRules || (e.Name == ".code-rules" && e.Type == "dir")
	}
	return root, nil
}

// ReleaseTags returns the names of repo's tags that start with release/, such as release/4, or none for a repository
// without any.
func (a *API) ReleaseTags(ctx context.Context, token string, repo domain.Repository) ([]string, error) {
	var refs []struct {
		Ref string `json:"ref"`
	}
	if _, err := a.get(ctx, token, repositoryPath(repo)+"/git/matching-refs/tags/release/", "", maxListBytes, &refs); err != nil {
		return nil, fmt.Errorf("list the release tags of repository=%q: %w", repo.FullName(), err)
	}
	tags := make([]string, 0, len(refs))
	for _, r := range refs {
		tags = append(tags, strings.TrimPrefix(r.Ref, "refs/tags/"))
	}
	return tags, nil
}

// File returns the file at path in repo's default branch, at most maxBytes of it, or found false when there's none.
func (a *API) File(ctx context.Context, token string, repo domain.Repository, path string, maxBytes int) ([]byte, bool, error) {
	var data []byte
	found, err := a.get(ctx, token, repositoryPath(repo)+"/contents/"+escapePath(path), "application/vnd.github.raw+json", maxBytes, &data)
	if err != nil {
		return nil, false, fmt.Errorf("read file=%q in repository=%q: %w", path, repo.FullName(), err)
	}
	return data, found, nil
}

// OrganizationRole returns the token's user's role in the organization org, admin for an owner and member otherwise,
// or empty when they aren't a member.
func (a *API) OrganizationRole(ctx context.Context, token, org string) (string, error) {
	var membership struct {
		State string `json:"state"`
		Role  string `json:"role"`
	}
	found, err := a.get(ctx, token, "/user/memberships/orgs/"+url.PathEscape(org), "", maxResponseBytes, &membership)
	if err != nil {
		return "", fmt.Errorf("read the user's membership org=%q: %w", org, err)
	}
	if !found || membership.State != "active" {
		return "", nil
	}
	return membership.Role, nil
}

// get reads path from the API with token, at most maxBytes of it, and decodes its JSON into v, or, when v is a
// *[]byte, keeps its bytes. accept is the media type to ask for, or empty for GitHub's JSON. It returns found false for
// 404, and 403 that isn't a rate limit's, both of which GitHub answers for a repository or file the token can't see; it
// fails with ErrTokenRefused for 401. Errors never include the token or the response's text.
func (a *API) get(ctx context.Context, token, path, accept string, maxBytes int, v any) (bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+path, nil)
	if err != nil {
		return false, err
	}
	setHeaders(request, token, accept)
	return a.do(request, maxBytes, v)
}

// do sends request, and decodes its response as get describes.
func (a *API) do(request *http.Request, maxBytes int, v any) (bool, error) {
	response, err := a.http.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode == http.StatusUnauthorized:
		return false, ErrTokenRefused
	case response.StatusCode == http.StatusNotFound,
		response.StatusCode == http.StatusForbidden && !rateLimited(response):
		return false, nil
	case response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated:
		return false, fmt.Errorf("GitHub answered %s", response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(maxBytes)+1))
	if err != nil {
		return false, fmt.Errorf("read the response: %v", err)
	}
	if len(body) > maxBytes {
		return false, fmt.Errorf("the response is larger than %d bytes", maxBytes)
	}
	if data, ok := v.(*[]byte); ok {
		*data = body
		return true, nil
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(v); err != nil {
		return false, fmt.Errorf("decode the response: %v", err)
	}
	return true, nil
}

// rateLimited reports whether GitHub's 403 says the token made too many requests, as its primary rate limit's
// remaining count of 0 or a secondary limit's Retry-After does, rather than that it can't see what it asked for.
func rateLimited(response *http.Response) bool {
	return response.Header.Get("X-RateLimit-Remaining") == "0" || response.Header.Get("Retry-After") != ""
}

// setHeaders asks GitHub for accept, or its JSON, from API version 2022-11-28, authorized by token, if any.
func setHeaders(request *http.Request, token, accept string) {
	if accept == "" {
		accept = "application/vnd.github+json"
	}
	request.Header.Set("Accept", accept)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "Rulemart")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
}

// repositoryPath returns the API's path of repo.
func repositoryPath(repo domain.Repository) string {
	return "/repos/" + url.PathEscape(repo.Owner) + "/" + url.PathEscape(repo.Name)
}

// escapePath escapes each segment of path, a file's path in a repository.
func escapePath(path string) string {
	segments := strings.Split(path, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	return strings.Join(segments, "/")
}
