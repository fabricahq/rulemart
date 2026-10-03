// Package githubtest is a fake of the parts of GitHub's REST API that accounts read: users and their organizations,
// repositories with files and tags, and installations of one GitHub App, which checks the app's JWTs. Tests serve it
// with httptest, and a local build with the rulemartdev tag serves DevFake, so the dashboard works without GitHub.
package githubtest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Fake is a fake GitHub. Set its fields before serving it; Handler serves it.
type Fake struct {
	Users         []User
	Repositories  []Repository
	Installations []Installation
	// AppClientID is the GitHub App's client ID, which its JWTs must name as their issuer, and AppKey the key they're
	// signed with. AppSlug names the app in its install page's address.
	AppClientID string
	AppKey      *rsa.PrivateKey
	AppSlug     string
	// InstalledURL is where the app's install page sends the visitor back, with installation_id and setup_action, as
	// GitHub sends them to the app's setup URL. InstallAs is the installation it names.
	InstalledURL string
	InstallAs    int64
	// Fail, when it returns true for a request's path, answers it with 502, as GitHub failing does.
	Fail func(path string) bool

	mu sync.Mutex
	// requests counts the requests each route answered, by its pattern.
	requests map[string]int
}

// User is a GitHub user, who signs in with Token.
type User struct {
	Token string
	ID    int64
	Login string
	// Organizations are the user's memberships.
	Organizations []Membership
}

// Membership is a user's membership of an organization: Role is admin for an owner, and member otherwise.
type Membership struct {
	Organization, Role string
}

// Repository is a repository on GitHub.
type Repository struct {
	Owner, Name string
	Private     bool
	PushedAt    time.Time
	// Files are its default branch's files, by path.
	Files map[string]string
	// Tags are its tags' names, such as release/1.
	Tags []string
}

// FullName returns the repository as owner/name.
func (r Repository) FullName() string { return r.Owner + "/" + r.Name }

// Installation is an installation of the GitHub App on Account, a user's or an organization's, which reads
// Repositories, by full name.
type Installation struct {
	ID           int64
	Account      string
	AccountID    int64
	Organization bool
	Repositories []string
}

// installationToken is the token Fake gives installation id.
func installationToken(id int64) string { return "ghs_fake_" + strconv.FormatInt(id, 10) }

// NewAppKey returns a new RSA key for a fake GitHub App.
func NewAppKey() *rsa.PrivateKey {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return key
}

// AppKeyPEM returns key in PKCS #1 PEM, as GitHub gives an app's private key.
func AppKeyPEM(key *rsa.PrivateKey) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}

// Requests returns how many requests the route pattern answered, such as "GET /repos/{owner}/{repo}/contents/".
func (f *Fake) Requests(pattern string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[pattern]
}

// Handler serves f as GitHub's REST API.
func (f *Fake) Handler() http.Handler {
	mux := http.NewServeMux()
	route := func(pattern string, handler http.HandlerFunc) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			f.mu.Lock()
			if f.requests == nil {
				f.requests = map[string]int{}
			}
			f.requests[pattern]++
			f.mu.Unlock()
			if f.Fail != nil && f.Fail(r.URL.Path) {
				http.Error(w, `{"message":"Server Error"}`, http.StatusBadGateway)
				return
			}
			handler(w, r)
		})
	}
	route("GET /user/orgs", f.organizations)
	route("GET /user/memberships/orgs/{org}", f.membership)
	route("GET /users/{owner}/repos", f.ownerRepositories(false))
	route("GET /orgs/{owner}/repos", f.ownerRepositories(true))
	route("GET /repos/{owner}/{repo}/contents/{path...}", f.contents)
	route("GET /repos/{owner}/{repo}/git/matching-refs/tags/release/", f.releaseTags)
	route("GET /app/installations/{id}", f.installation)
	route("POST /app/installations/{id}/access_tokens", f.accessToken)
	route("GET /installation/repositories", f.installationRepositories)
	route("GET /apps/{slug}/installations/new", f.installPage)
	return mux
}

// user returns the user r's token signs in, or answers 401 and returns false.
func (f *Fake) user(w http.ResponseWriter, r *http.Request) (User, bool) {
	token := bearer(r)
	for _, u := range f.Users {
		if u.Token != "" && u.Token == token {
			return u, true
		}
	}
	unauthorized(w)
	return User{}, false
}

func (f *Fake) organizations(w http.ResponseWriter, r *http.Request) {
	u, ok := f.user(w, r)
	if !ok {
		return
	}
	orgs := []map[string]string{}
	for _, m := range u.Organizations {
		orgs = append(orgs, map[string]string{"login": m.Organization})
	}
	writeJSON(w, orgs)
}

func (f *Fake) membership(w http.ResponseWriter, r *http.Request) {
	u, ok := f.user(w, r)
	if !ok {
		return
	}
	for _, m := range u.Organizations {
		if strings.EqualFold(m.Organization, r.PathValue("org")) {
			writeJSON(w, map[string]string{"state": "active", "role": m.Role})
			return
		}
	}
	notFound(w)
}

// ownerRepositories lists an owner's public repositories, an organization's when organization is true, the most
// recently pushed first, a page at a time.
func (f *Fake) ownerRepositories(organization bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := f.user(w, r); !ok {
			return
		}
		if f.isOrganization(r.PathValue("owner")) != organization {
			notFound(w)
			return
		}
		var repos []Repository
		for _, repo := range f.Repositories {
			if strings.EqualFold(repo.Owner, r.PathValue("owner")) && !repo.Private {
				repos = append(repos, repo)
			}
		}
		writeJSON(w, page(sortedByPush(repos), r.URL.Query()))
	}
}

// isOrganization reports whether login is an organization's: one a user is a member of, or an installation is on.
func (f *Fake) isOrganization(login string) bool {
	for _, u := range f.Users {
		for _, m := range u.Organizations {
			if strings.EqualFold(m.Organization, login) {
				return true
			}
		}
	}
	for _, in := range f.Installations {
		if in.Organization && strings.EqualFold(in.Account, login) {
			return true
		}
	}
	return false
}

// repository returns the repository r names, when r's token may read it: a public one with any user's token, and a
// private one with the token of an installation that reads it. It answers 404 otherwise, as GitHub does.
func (f *Fake) repository(w http.ResponseWriter, r *http.Request) (Repository, bool) {
	token := bearer(r)
	for _, repo := range f.Repositories {
		if !strings.EqualFold(repo.FullName(), r.PathValue("owner")+"/"+r.PathValue("repo")) {
			continue
		}
		if !repo.Private && slices.ContainsFunc(f.Users, func(u User) bool { return u.Token == token }) {
			return repo, true
		}
		for _, in := range f.Installations {
			if token == installationToken(in.ID) && slices.ContainsFunc(in.Repositories, func(name string) bool { return strings.EqualFold(name, repo.FullName()) }) {
				return repo, true
			}
		}
	}
	notFound(w)
	return Repository{}, false
}

// contents lists the root of a repository's default branch, or returns one of its files, raw.
func (f *Fake) contents(w http.ResponseWriter, r *http.Request) {
	repo, ok := f.repository(w, r)
	if !ok {
		return
	}
	path := r.PathValue("path")
	if path == "" {
		entries := []map[string]string{}
		seen := map[string]bool{}
		for file := range repo.Files {
			name, _, isDir := strings.Cut(file, "/")
			if seen[name] {
				continue
			}
			seen[name] = true
			kind := "file"
			if isDir {
				kind = "dir"
			}
			entries = append(entries, map[string]string{"name": name, "path": name, "type": kind})
		}
		if len(entries) == 0 {
			http.Error(w, `{"message":"This repository is empty."}`, http.StatusNotFound)
			return
		}
		slices.SortFunc(entries, func(a, b map[string]string) int { return strings.Compare(a["name"], b["name"]) })
		writeJSON(w, entries)
		return
	}
	content, ok := repo.Files[path]
	if !ok {
		notFound(w)
		return
	}
	if r.Header.Get("Accept") != "application/vnd.github.raw+json" {
		http.Error(w, `{"message":"ask for the raw file"}`, http.StatusNotAcceptable)
		return
	}
	_, _ = w.Write([]byte(content))
}

func (f *Fake) releaseTags(w http.ResponseWriter, r *http.Request) {
	repo, ok := f.repository(w, r)
	if !ok {
		return
	}
	refs := []map[string]string{}
	for _, tag := range repo.Tags {
		if strings.HasPrefix(tag, "release/") {
			refs = append(refs, map[string]string{"ref": "refs/tags/" + tag})
		}
	}
	writeJSON(w, refs)
}

// findInstallation returns the installation r's id names, after checking r is signed with the app's JWT, or answers
// and returns false.
func (f *Fake) findInstallation(w http.ResponseWriter, r *http.Request) (Installation, bool) {
	if err := f.checkJWT(bearer(r)); err != nil {
		http.Error(w, `{"message":"`+err.Error()+`"}`, http.StatusUnauthorized)
		return Installation{}, false
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	for _, in := range f.Installations {
		if in.ID == id {
			return in, true
		}
	}
	notFound(w)
	return Installation{}, false
}

func (f *Fake) installation(w http.ResponseWriter, r *http.Request) {
	in, ok := f.findInstallation(w, r)
	if !ok {
		return
	}
	kind := "User"
	if in.Organization {
		kind = "Organization"
	}
	writeJSON(w, map[string]any{"id": in.ID, "account": map[string]any{"login": in.Account, "id": in.AccountID, "type": kind}})
}

func (f *Fake) accessToken(w http.ResponseWriter, r *http.Request) {
	in, ok := f.findInstallation(w, r)
	if !ok {
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]string{"token": installationToken(in.ID)})
}

func (f *Fake) installationRepositories(w http.ResponseWriter, r *http.Request) {
	token := bearer(r)
	for _, in := range f.Installations {
		if token != installationToken(in.ID) {
			continue
		}
		var repos []Repository
		for _, repo := range f.Repositories {
			if slices.ContainsFunc(in.Repositories, func(name string) bool { return strings.EqualFold(name, repo.FullName()) }) {
				repos = append(repos, repo)
			}
		}
		paged := page(sortedByPush(repos), r.URL.Query())
		writeJSON(w, map[string]any{"total_count": len(repos), "repositories": paged})
		return
	}
	unauthorized(w)
}

// installPage stands in for the app's install page on GitHub: it installs the app as InstallAs at once, and sends the
// visitor back to InstalledURL.
func (f *Fake) installPage(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("slug") != f.AppSlug || f.InstalledURL == "" {
		notFound(w)
		return
	}
	query := url.Values{"installation_id": {strconv.FormatInt(f.InstallAs, 10)}, "setup_action": {"install"}}
	http.Redirect(w, r, f.InstalledURL+"?"+query.Encode(), http.StatusFound)
}

// checkJWT reports why token isn't a JWT the app signed, as GitHub checks one: RS256, signed with AppKey, issued by
// AppClientID, and not expired.
func (f *Fake) checkJWT(token string) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || f.AppKey == nil {
		return fmt.Errorf("a JWT could not be decoded")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(&f.AppKey.PublicKey, crypto.SHA256, digest[:], signature); err != nil {
		return fmt.Errorf("a JWT signature does not match")
	}
	claims, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return err
	}
	var c struct {
		Iss string `json:"iss"`
		Exp int64  `json:"exp"`
		Iat int64  `json:"iat"`
	}
	if err := json.Unmarshal(claims, &c); err != nil {
		return err
	}
	now := time.Now().Unix()
	if c.Iss != f.AppClientID || c.Exp <= now || c.Iat > now || c.Exp-c.Iat > 600 {
		return fmt.Errorf("the JWT's claims aren't the app's")
	}
	return nil
}

// repositoryJSON describes repo as GitHub's lists do.
func repositoryJSON(repo Repository) map[string]any {
	return map[string]any{
		"name": repo.Name, "full_name": repo.FullName(), "private": repo.Private,
		"pushed_at": repo.PushedAt.UTC().Format(time.RFC3339), "owner": map[string]string{"login": repo.Owner},
	}
}

func sortedByPush(repos []Repository) []Repository {
	sorted := slices.Clone(repos)
	slices.SortStableFunc(sorted, func(a, b Repository) int { return b.PushedAt.Compare(a.PushedAt) })
	return sorted
}

// page returns the page of repos query's page and per_page parameters name, as GitHub pages lists, 30 a page by
// default.
func page(repos []Repository, query url.Values) []map[string]any {
	size, err := strconv.Atoi(query.Get("per_page"))
	if err != nil || size <= 0 || size > 100 {
		size = 30
	}
	n, err := strconv.Atoi(query.Get("page"))
	if err != nil || n < 1 {
		n = 1
	}
	out := []map[string]any{}
	for i := (n - 1) * size; i < min(n*size, len(repos)); i++ {
		out = append(out, repositoryJSON(repos[i]))
	}
	return out
}

func bearer(r *http.Request) string {
	token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return token
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func notFound(w http.ResponseWriter) {
	http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
}

func unauthorized(w http.ResponseWriter) {
	http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
}

// ProvenanceSource is a source a fake project's provenance file names, with the versions of the rules it holds by ID.
type ProvenanceSource struct {
	Name, Repository string
	Release          int
	Groups           []string
	Rules            map[string]string
}

// Provenance returns a .code-rules/generated/provenance.json naming sources, in the shape Code Rules writes.
func Provenance(sources ...ProvenanceSource) string {
	type origin struct {
		Source  string  `json:"source"`
		File    string  `json:"file"`
		Version *string `json:"version"`
		Release *int    `json:"release"`
	}
	type rule struct {
		ID     string `json:"id"`
		Group  string `json:"group"`
		Origin origin `json:"origin"`
	}
	type source struct {
		Name       string   `json:"name"`
		Repository string   `json:"repository"`
		Release    int      `json:"release,omitempty"`
		Groups     []string `json:"groups"`
	}
	file := struct {
		GeneratedNotice string   `json:"generatedNotice"`
		ToolVersion     string   `json:"toolVersion"`
		Sources         []source `json:"sources"`
		Rules           []rule   `json:"rules"`
	}{GeneratedNotice: "Generated by Code Rules.", ToolVersion: "0.3.0", Sources: []source{}, Rules: []rule{}}
	for _, s := range sources {
		file.Sources = append(file.Sources, source{Name: s.Name, Repository: s.Repository, Release: s.Release, Groups: s.Groups})
		for id, version := range s.Rules {
			group := id[:strings.LastIndex(id, "/")]
			release := s.Release
			file.Rules = append(file.Rules, rule{ID: s.Name + ":" + id, Group: group, Origin: origin{Source: s.Name, File: id + ".md", Version: &version, Release: &release}})
		}
	}
	slices.SortFunc(file.Rules, func(a, b rule) int { return strings.Compare(a.ID, b.ID) })
	data, _ := json.MarshalIndent(file, "", "  ")
	return string(data)
}
