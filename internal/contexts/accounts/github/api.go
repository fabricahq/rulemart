// Read a visitor's GitHub account through GitHub's REST API: their organizations, the repositories of theirs and their
// organizations', what each repository's root holds, its release tags, and a file in it.

package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
// in GitHub's order, including memberships the user keeps private, which read:org reveals, and whether the user
// belongs to more.
func (a *API) Organizations(ctx context.Context, token string) ([]string, bool, error) {
	var orgs []struct {
		Login string `json:"login"`
	}
	if _, err := a.get(ctx, token, "/user/orgs?per_page="+strconv.Itoa(domain.MaxOrganizations), "", maxListBytes, &orgs); err != nil {
		return nil, false, fmt.Errorf("list the user's organizations: %w", err)
	}
	logins := make([]string, 0, len(orgs))
	for _, o := range orgs {
		logins = append(logins, o.Login)
	}
	if len(orgs) < domain.MaxOrganizations {
		return logins, false, nil
	}
	// A full page leaves the user's next organization, if any, to a page of one past it.
	var next []json.RawMessage
	path := "/user/orgs?per_page=1&page=" + strconv.Itoa(domain.MaxOrganizations+1)
	if _, err := a.get(ctx, token, path, "", maxListBytes, &next); err != nil {
		return nil, false, fmt.Errorf("list the user's organizations past the first %d: %w", domain.MaxOrganizations, err)
	}
	return logins, len(next) > 0, nil
}

// Repositories returns the public repositories owner owns, an organization's when organization is true and a user's
// otherwise, the most recently pushed first, at most limit of them, and whether owner has more.
func (a *API) Repositories(ctx context.Context, token, owner string, organization bool, limit int) ([]domain.GitHubRepository, bool, error) {
	path := "/users/" + url.PathEscape(owner) + "/repos?type=owner&sort=pushed&direction=desc&"
	if organization {
		path = "/orgs/" + url.PathEscape(owner) + "/repos?type=public&sort=pushed&direction=desc&"
	}
	// One page past limit's tells whether owner has more.
	repositories, more, err := a.repositories(ctx, token, path, "", limit, limit/perPage+1, keepAll)
	if err != nil {
		return nil, false, fmt.Errorf("list repositories owner=%q: %w", owner, err)
	}
	return repositories, more, nil
}

// repositories returns the repositories the list at path holds that keep returns true for, in the list's order, page
// by page, at most limit of them, reading at most maxPages pages. more reports that it left some out: the list held
// more that keep took than limit, or pages remained past maxPages. path ends with its query's ? or &. field names the field of each page's object that
// holds them, or is empty for a page that's a list itself.
func (a *API) repositories(ctx context.Context, token, path, field string, limit, maxPages int, keep func(domain.GitHubRepository) bool) (found []domain.GitHubRepository, more bool, err error) {
	for page := 1; ; page++ {
		if page > maxPages {
			return found, true, nil
		}
		var repos []repositoryJSON
		url := path + "per_page=" + strconv.Itoa(perPage) + "&page=" + strconv.Itoa(page)
		if field == "" {
			_, err = a.get(ctx, token, url, "", maxListBytes, &repos)
		} else {
			var object map[string]json.RawMessage
			if _, err = a.get(ctx, token, url, "", maxListBytes, &object); err == nil {
				err = json.Unmarshal(object[field], &repos)
			}
		}
		if err != nil {
			return nil, false, err
		}
		for _, r := range repos {
			if repo, ok := r.repository(); ok && keep(repo) {
				if len(found) == limit {
					return found, true, nil
				}
				found = append(found, repo)
			}
		}
		if len(repos) < perPage {
			return found, false, nil
		}
	}
}

// keepAll keeps every repository a list holds.
func keepAll(domain.GitHubRepository) bool { return true }

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
// fails for a rate limit, so a read never takes a throttled repository for one it can't see, and with
// domain.ErrGitHubTokenRefused for 401. Errors never include the token or the response's text.
func (a *API) get(ctx context.Context, token, path, accept string, maxBytes int, v any) (bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+path, nil)
	if err != nil {
		return false, err
	}
	setHeaders(request, token, accept)
	found, err := a.do(request, maxBytes, v)
	if errors.Is(err, errForbidden) {
		return false, nil
	}
	return found, err
}

// errForbidden reports a 403 that isn't a rate limit's: GitHub refusing to say, which a read through a token takes for
// not found, since GitHub answers it for what the token can't see, but an app's lookup of an installation doesn't,
// since only 404 says GitHub doesn't know the installation.
var errForbidden = errors.New("GitHub answered 403 Forbidden")

// do sends request, and decodes its response as get describes, except that it fails with errForbidden for a 403 that
// isn't a rate limit's.
func (a *API) do(request *http.Request, maxBytes int, v any) (bool, error) {
	response, err := a.http.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode == http.StatusUnauthorized:
		return false, domain.ErrGitHubTokenRefused
	case rateLimited(response):
		return false, fmt.Errorf("GitHub answered %s: rate limited", response.Status)
	case response.StatusCode == http.StatusNotFound:
		return false, nil
	case response.StatusCode == http.StatusForbidden:
		return false, errForbidden
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

// maxRateLimitMessageBytes bounds what rateLimited reads of a 403's body: GitHub's error messages are under a KiB.
const maxRateLimitMessageBytes = 4 << 10

// rateLimited reports whether GitHub's response says the token made too many requests, rather than that it can't see
// what it asked for: a 429, or a 403 with its primary rate limit's remaining count of 0, a secondary limit's
// Retry-After, or a message about a rate limit, which a secondary limit may send without either header. It reads a
// 403's body, and takes one it can't read for a rate limit's, so a read fails rather than skip what it couldn't tell.
func rateLimited(response *http.Response) bool {
	switch {
	case response.StatusCode == http.StatusTooManyRequests:
		return true
	case response.StatusCode != http.StatusForbidden:
		return false
	case response.Header.Get("X-RateLimit-Remaining") == "0" || response.Header.Get("Retry-After") != "":
		return true
	}
	message, err := io.ReadAll(io.LimitReader(response.Body, maxRateLimitMessageBytes))
	return err != nil || strings.Contains(strings.ToLower(string(message)), "rate limit")
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
