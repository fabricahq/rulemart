package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/github"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/github/githubtest"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/store/postgres"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
	"github.com/fabricahq/rulemart/internal/platform/database/databasetest"
	"github.com/fabricahq/rulemart/internal/platform/postgrestest"
)

const (
	monaToken     = "gho_mona"
	monaID        = 7
	webhookSecret = "webhook-secret"
	appID         = 42
)

var mona = domain.Identity{GitHubUserID: monaID, Login: "mona"}

// staticSecret is a secret that never changes, as a value given directly.
type staticSecret string

func (s staticSecret) Value(context.Context) (string, error) { return string(s), nil }
func (staticSecret) Forget()                                 {}

// gitHubSite is GitHubAccounts against a fake GitHub, with accounts stored as the web function's role stores them, and
// mona signed in with her token.
type gitHubSite struct {
	accounts   GitHubAccounts
	fake       *githubtest.Fake
	account    domain.Account
	session    domain.SessionToken
	connString string
	now        time.Time
}

// newGitHubSite serves fake, and returns GitHubAccounts reading it, with the GitHub App when app is true.
func newGitHubSite(t *testing.T, fake *githubtest.Fake, app bool) *gitHubSite {
	t.Helper()
	server := httptest.NewServer(fake.Handler())
	t.Cleanup(server.Close)
	_, connString := databasetest.New(t)
	store := postgres.New(databasetest.AsWebRole(t, connString))
	sessions := Sessions{Store: store, TokenKeys: FixedTokenKey{domain.NewTokenKey()}}
	site := &gitHubSite{fake: fake, connString: connString, now: time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)}
	api := github.NewAPI(server.URL)
	site.accounts = GitHubAccounts{Store: store, Sessions: sessions, GitHub: api, Now: func() time.Time { return site.now }}
	if app {
		fake.AppClientID, fake.AppKey = "Iv1.app", githubtest.NewAppKey()
		site.accounts.App = github.NewApp(github.AppConfig{
			ID: appID, ClientID: fake.AppClientID, Slug: "rulemart-by-fabrica",
			PrivateKey: staticSecret(githubtest.AppKeyPEM(fake.AppKey)), WebhookSecret: staticSecret(webhookSecret),
		}, api)
	}
	account, session, err := sessions.SignIn(context.Background(), mona, monaToken, "")
	if err != nil {
		t.Fatal(err)
	}
	site.account, site.session = account, session.Token
	return site
}

func (s *gitHubSite) snapshot(t *testing.T) domain.Snapshot {
	t.Helper()
	snapshot, err := s.accounts.Snapshot(context.Background(), s.account, s.session)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

var pushed = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

// monasGitHub is a GitHub where mona belongs to octo-org and owns a published library, a project, a manifest without
// a release, and a repository with neither; octo-org owns a library too, and a private repository of mona's is a
// project only the app reads.
func monasGitHub() *githubtest.Fake {
	manifest := map[string]string{"rule-library.yaml": "schemaVersion: 1\n"}
	provenance := githubtest.Provenance(githubtest.ProvenanceSource{
		Name: "fabrica", Repository: "https://github.com/fabricahq/public-rules.git", Release: 2, Groups: []string{"techs/go"},
		Rules: map[string]string{"techs/go/return-errors": "1.2.0"},
	})
	return &githubtest.Fake{
		Users: []githubtest.User{{Token: monaToken, ID: monaID, Login: "mona", Organizations: []githubtest.Membership{{Organization: "octo-org", Role: "member"}}}},
		Repositories: []githubtest.Repository{
			{Owner: "mona", Name: "rules", PushedAt: pushed, Files: manifest, Tags: []string{"release/1", "release/12", "release/3", "v1.0"}},
			{Owner: "mona", Name: "api", PushedAt: pushed.Add(-time.Hour), Files: map[string]string{domain.ProvenancePath: provenance}},
			{Owner: "mona", Name: "draft-rules", PushedAt: pushed.Add(-2 * time.Hour), Files: manifest},
			{Owner: "mona", Name: "broken", PushedAt: pushed.Add(-3 * time.Hour), Files: map[string]string{domain.ProvenancePath: "{not json"}},
			{Owner: "mona", Name: "notes", PushedAt: pushed.Add(-4 * time.Hour), Files: map[string]string{"README.md": "notes"}},
			{Owner: "mona", Name: "empty", PushedAt: pushed.Add(-5 * time.Hour)},
			{Owner: "octo-org", Name: "Org-Rules", PushedAt: pushed.Add(-6 * time.Hour), Files: manifest, Tags: []string{"release/2"}},
			{Owner: "mona", Name: "billing", Private: true, PushedAt: pushed.Add(-7 * time.Hour), Files: map[string]string{domain.ProvenancePath: provenance}},
		},
	}
}

// A read finds the visitor's organizations, the repositories of theirs and their organizations' that hold a
// rule-library.yaml and a release tag, with the latest release, and the projects whose provenance names their sources,
// and leaves out a manifest without a release, a provenance file Rulemart can't parse, and private repositories its
// token can't see.
func TestSnapshotReadsTheVisitorsLibrariesAndProjects(t *testing.T) {
	site := newGitHubSite(t, monasGitHub(), false)

	got := site.snapshot(t)

	if !slices.Equal(got.Organizations, []string{"octo-org"}) || !got.ReadAt.Equal(site.now) || got.Truncated || got.ReadFailed {
		t.Errorf("read %+v", got)
	}
	wantLibraries := []domain.PublishableRepository{
		{Repository: domain.Repository{Owner: "mona", Name: "rules"}, Release: 12},
		{Repository: domain.Repository{Owner: "octo-org", Name: "Org-Rules"}, Release: 2},
	}
	if !slices.Equal(got.Libraries, wantLibraries) {
		t.Errorf("libraries %+v, want %+v", got.Libraries, wantLibraries)
	}
	if len(got.Projects) != 1 || got.Projects[0].FullName() != "mona/api" {
		t.Fatalf("projects %+v, want mona/api", got.Projects)
	}
	source := got.Projects[0].Sources[0]
	want := domain.PinnedRule{Path: "techs/go/return-errors", Version: coderules.RuleVersion{Major: 1, Minor: 2}}
	if source.Library != "fabricahq/public-rules" || source.Release != 2 || len(source.Rules) != 1 || source.Rules[0] != want {
		t.Errorf("the project's source is %+v", source)
	}
}

// The snapshot is kept, so pages after the first read nothing from GitHub, and a refresh within a minute keeps it
// too; a minute later, a refresh reads again.
func TestASnapshotIsReadOnceAndRefreshedAtMostOnceAMinute(t *testing.T) {
	ctx := context.Background()
	site := newGitHubSite(t, monasGitHub(), false)
	site.snapshot(t)
	reads := func() int { return site.fake.Requests("GET /user/orgs") }

	site.snapshot(t)
	if _, err := site.accounts.Refresh(ctx, site.account, site.session); err != nil {
		t.Fatal(err)
	}
	if reads() != 1 {
		t.Fatalf("read GitHub %d times, want once", reads())
	}

	site.now = site.now.Add(domain.RefreshInterval)
	refreshed, err := site.accounts.Refresh(ctx, site.account, site.session)
	if err != nil {
		t.Fatal(err)
	}
	if reads() != 2 || !refreshed.ReadAt.Equal(site.now) {
		t.Errorf("after a minute, read GitHub %d times, as of %v", reads(), refreshed.ReadAt)
	}
}

// A visitor whose organizations hold more repositories than a read looks into gets the most recently pushed ones
// read, and the snapshot says it stopped there.
func TestAReadLooksIntoAtMostTheMostRecentlyPushedRepositories(t *testing.T) {
	fake := monasGitHub()
	for i := range domain.MaxRepositories + 50 {
		fake.Repositories = append(fake.Repositories, githubtest.Repository{
			Owner: "octo-org", Name: fmt.Sprintf("service-%03d", i), PushedAt: pushed.AddDate(0, 0, -1-i),
			Files: map[string]string{"README.md": "service"},
		})
	}
	// The least recently pushed of all, which the read leaves out.
	fake.Repositories = append(fake.Repositories, githubtest.Repository{
		Owner: "octo-org", Name: "old-rules", PushedAt: pushed.AddDate(-5, 0, 0), Files: map[string]string{"rule-library.yaml": "x"}, Tags: []string{"release/1"},
	})
	site := newGitHubSite(t, fake, false)

	got := site.snapshot(t)

	// Each repository read costs a listing of its root, and mona/api and mona/broken a provenance file too.
	if inspected := site.fake.Requests("GET /repos/{owner}/{repo}/contents/{path...}") - 2; inspected != domain.MaxRepositories {
		t.Errorf("looked into %d repositories, want %d", inspected, domain.MaxRepositories)
	}
	if !got.Truncated || slices.ContainsFunc(got.Libraries, func(l domain.PublishableRepository) bool { return l.Name == "old-rules" }) {
		t.Errorf("truncated %v, libraries %+v", got.Truncated, got.Libraries)
	}
	if !slices.ContainsFunc(got.Libraries, func(l domain.PublishableRepository) bool { return l.Name == "rules" }) {
		t.Error("the most recently pushed library is missing")
	}
}

// When GitHub fails, the snapshot says so, keeps what the last read found, and waits a minute before the next read, so
// a broken GitHub isn't asked again on every page.
func TestAFailedReadKeepsTheLastSnapshotAndSaysSo(t *testing.T) {
	ctx := context.Background()
	fake := monasGitHub()
	site := newGitHubSite(t, fake, false)
	first := site.snapshot(t)

	fake.Fail = func(path string) bool { return strings.Contains(path, "/contents/") }
	site.now = site.now.Add(domain.RefreshInterval)
	got, err := site.accounts.Refresh(ctx, site.account, site.session)

	if !errors.Is(err, ErrGitHubRead) {
		t.Fatalf("got %v, want ErrGitHubRead", err)
	}
	if !got.ReadFailed || !got.ReadAt.Equal(first.ReadAt) || len(got.Libraries) != len(first.Libraries) {
		t.Errorf("after a failed read, the snapshot is %+v", got)
	}
	again, err := site.accounts.Snapshot(ctx, site.account, site.session)
	if err != nil || !again.ReadFailed {
		t.Errorf("the kept snapshot is %+v, %v", again, err)
	}
	if _, err := site.accounts.Refresh(ctx, site.account, site.session); err != nil {
		t.Errorf("a refresh within the minute after the failure: %v", err)
	}
}

// A visitor who revoked Rulemart on GitHub, or whose session keeps no token, is asked to sign in again, and nothing is
// kept for them.
func TestAReadWithATokenGitHubRefusesNeedsASignIn(t *testing.T) {
	fake := monasGitHub()
	site := newGitHubSite(t, fake, false)
	fake.Users[0].Token = "gho_rotated"

	if _, err := site.accounts.Snapshot(context.Background(), site.account, site.session); !errors.Is(err, ErrNoGitHubToken) {
		t.Fatalf("got %v, want ErrNoGitHubToken", err)
	}
	var kept int
	postgrestest.QueryRow(t, site.connString, "SELECT count(*) FROM github_snapshots", &kept)
	if kept != 0 {
		t.Errorf("kept %d snapshots", kept)
	}
}

// Installing the app on the visitor's own account reads their private repositories with its token, and the snapshot
// shows them as private; forgetting it reads public repositories only again.
func TestInstallingTheAppOnTheVisitorsAccountReadsTheirPrivateRepositories(t *testing.T) {
	ctx := context.Background()
	fake := monasGitHub()
	fake.Installations = []githubtest.Installation{{ID: 5, Account: "mona", AccountID: monaID, Repositories: []string{"mona/billing", "mona/rules"}}}
	site := newGitHubSite(t, fake, true)
	if len(site.snapshot(t).Projects) != 1 {
		t.Fatal("read a private project before installing")
	}

	got, err := site.accounts.Install(ctx, site.account, site.session, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Projects) != 2 || got.Projects[0].FullName() != "mona/api" || !got.Projects[1].Private {
		t.Errorf("projects %+v, want mona/api and the private mona/billing", got.Projects)
	}
	installations, err := site.accounts.Installations(ctx, site.account.ID)
	if err != nil || len(installations) != 1 || installations[0] != (domain.Installation{ID: 5, Account: "mona"}) {
		t.Errorf("installations %+v, %v", installations, err)
	}

	if err := site.accounts.ForgetInstallations(ctx, site.account.ID); err != nil {
		t.Fatal(err)
	}
	if got := site.snapshot(t); len(got.Projects) != 1 {
		t.Errorf("after forgetting the app, projects %+v", got.Projects)
	}
}

// Only an installation on the visitor's own account, or an organization they own, is theirs to read through: one on
// another user, on an organization they're only a member of, or one GitHub doesn't know, is refused and not recorded.
func TestInstallRefusesAnInstallationTheVisitorDoesntOwn(t *testing.T) {
	fake := monasGitHub()
	fake.Installations = []githubtest.Installation{
		{ID: 1, Account: "hubot", AccountID: 99},
		{ID: 2, Account: "octo-org", AccountID: 100, Organization: true, Repositories: []string{"octo-org/Org-Rules"}},
	}
	for name, id := range map[string]int64{"another user's": 1, "a member's organization": 2, "an unknown one": 3} {
		t.Run(name, func(t *testing.T) {
			site := newGitHubSite(t, fake, true)
			if _, err := site.accounts.Install(context.Background(), site.account, site.session, id); !errors.Is(err, ErrNotYourInstallation) {
				t.Fatalf("got %v, want ErrNotYourInstallation", err)
			}
			if installations, _ := site.accounts.Installations(context.Background(), site.account.ID); len(installations) != 0 {
				t.Errorf("recorded %+v", installations)
			}
		})
	}
	t.Run("an owned organization's", func(t *testing.T) {
		owner := monasGitHub()
		owner.Users[0].Organizations[0].Role = "admin"
		owner.Installations = fake.Installations
		site := newGitHubSite(t, owner, true)
		if _, err := site.accounts.Install(context.Background(), site.account, site.session, 2); err != nil {
			t.Fatal(err)
		}
	})
}

// An installation uninstalled on GitHub since the visitor installed it reads nothing: a read forgets it and goes on.
func TestAReadForgetsAnInstallationGitHubNoLongerKnows(t *testing.T) {
	ctx := context.Background()
	fake := monasGitHub()
	fake.Installations = []githubtest.Installation{{ID: 5, Account: "mona", AccountID: monaID, Repositories: []string{"mona/billing"}}}
	site := newGitHubSite(t, fake, true)
	if _, err := site.accounts.Install(ctx, site.account, site.session, 5); err != nil {
		t.Fatal(err)
	}
	fake.Installations = nil
	site.now = site.now.Add(domain.RefreshInterval)

	got, err := site.accounts.Refresh(ctx, site.account, site.session)

	if err != nil || len(got.Projects) != 1 {
		t.Fatalf("got %+v, %v", got.Projects, err)
	}
	if installations, _ := site.accounts.Installations(ctx, site.account.ID); len(installations) != 0 {
		t.Errorf("still reads through %+v", installations)
	}
}

// signature returns body's X-Hub-Signature-256 with secret.
func signature(secret, body string) string {
	mac := hmacSHA256(secret, body)
	return "sha256=" + mac
}

// GitHub's webhook says when the app is uninstalled, which forgets it for every account and discards their snapshots,
// or when the repositories it reads change, which discards them, so the next page reads GitHub again. A delivery
// GitHub didn't sign, or for another app, changes nothing.
func TestWebhookDeliveriesForgetRemovedInstallationsAndDiscardChangedSnapshots(t *testing.T) {
	ctx := context.Background()
	deliver := func(site *gitHubSite, event, body, sig string) error {
		return site.accounts.Deliver(ctx, event, []byte(body), sig)
	}
	count := func(site *gitHubSite, table string) int {
		var n int
		postgrestest.QueryRow(t, site.connString, "SELECT count(*) FROM "+table, &n)
		return n
	}
	installed := func(t *testing.T) *gitHubSite {
		fake := monasGitHub()
		fake.Installations = []githubtest.Installation{{ID: 5, Account: "mona", AccountID: monaID, Repositories: []string{"mona/billing"}}}
		site := newGitHubSite(t, fake, true)
		if _, err := site.accounts.Install(ctx, site.account, site.session, 5); err != nil {
			t.Fatal(err)
		}
		return site
	}
	removed := `{"action":"deleted","installation":{"id":5,"app_id":42}}`

	t.Run("removed", func(t *testing.T) {
		site := installed(t)
		if err := deliver(site, "installation", removed, signature(webhookSecret, removed)); err != nil {
			t.Fatal(err)
		}
		if count(site, "github_installations") != 0 || count(site, "github_snapshots") != 0 {
			t.Error("the installation or the snapshot outlived its removal")
		}
	})
	t.Run("repositories changed", func(t *testing.T) {
		site := installed(t)
		body := `{"action":"removed","installation":{"id":5,"app_id":42},"repositories_removed":[{"full_name":"mona/billing"}]}`
		if err := deliver(site, "installation_repositories", body, signature(webhookSecret, body)); err != nil {
			t.Fatal(err)
		}
		if count(site, "github_installations") != 1 || count(site, "github_snapshots") != 0 {
			t.Error("a change kept the snapshot, or forgot the installation")
		}
	})
	for name, tc := range map[string]struct {
		event, body, signature string
		want                   error
	}{
		"unsigned":         {"installation", removed, "", domain.ErrBadSignature},
		"signed otherwise": {"installation", removed, signature("another-secret", removed), domain.ErrBadSignature},
		"altered":          {"installation", removed, signature(webhookSecret, strings.Replace(removed, "5", "6", 1)), domain.ErrBadSignature},
		"another app's": {
			"installation", `{"action":"deleted","installation":{"id":5,"app_id":7}}`,
			signature(webhookSecret, `{"action":"deleted","installation":{"id":5,"app_id":7}}`), domain.ErrIgnoredEvent,
		},
		"a new installation": {
			"installation", `{"action":"created","installation":{"id":5,"app_id":42}}`,
			signature(webhookSecret, `{"action":"created","installation":{"id":5,"app_id":42}}`), domain.ErrIgnoredEvent,
		},
	} {
		t.Run(name, func(t *testing.T) {
			site := installed(t)
			if err := deliver(site, tc.event, tc.body, tc.signature); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if count(site, "github_installations") != 1 || count(site, "github_snapshots") != 1 {
				t.Error("an ignored delivery changed what Rulemart keeps")
			}
		})
	}
}

// Deleting an account deletes its snapshot and its installations with it.
func TestDeletingAnAccountDeletesItsGitHubSnapshotAndInstallations(t *testing.T) {
	fake := monasGitHub()
	fake.Installations = []githubtest.Installation{{ID: 5, Account: "mona", AccountID: monaID, Repositories: []string{"mona/billing"}}}
	site := newGitHubSite(t, fake, true)
	if _, err := site.accounts.Install(context.Background(), site.account, site.session, 5); err != nil {
		t.Fatal(err)
	}
	if err := site.accounts.Sessions.DeleteAccount(context.Background(), site.session); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"github_snapshots", "github_installations"} {
		var n int
		postgrestest.QueryRow(t, site.connString, "SELECT count(*) FROM "+table, &n)
		if n != 0 {
			t.Errorf("%s holds %d rows", table, n)
		}
	}
}

// Without the app, a visitor can't install it.
func TestInstallWithoutTheAppFails(t *testing.T) {
	site := newGitHubSite(t, monasGitHub(), false)
	if _, err := site.accounts.Install(context.Background(), site.account, site.session, 1); !errors.Is(err, ErrNoApp) {
		t.Errorf("got %v, want ErrNoApp", err)
	}
	if err := site.accounts.Deliver(context.Background(), "installation", nil, ""); !errors.Is(err, ErrNoApp) {
		t.Errorf("a delivery: got %v, want ErrNoApp", err)
	}
}

func hmacSHA256(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

// An organization's owner who installed the app there stops reading its private repositories once they're no longer
// an owner, demoted or gone from the organization: the next read checks their role with their own token, forgets the
// installation for their account, and keeps none of what it read.
func TestAFormerOrganizationOwnerNoLongerReadsItsPrivateRepositories(t *testing.T) {
	for name, revoke := range map[string]func(*githubtest.User){
		"demoted": func(u *githubtest.User) { u.Organizations[0].Role = "member" },
		"left":    func(u *githubtest.User) { u.Organizations = nil },
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			fake := monasGitHub()
			fake.Users[0].Organizations[0].Role = "admin"
			fake.Repositories = append(fake.Repositories, githubtest.Repository{
				Owner: "octo-org", Name: "secret", Private: true, PushedAt: pushed.Add(-8 * time.Hour),
				Files: map[string]string{domain.ProvenancePath: githubtest.Provenance(githubtest.ProvenanceSource{Name: "fabrica", Repository: "https://github.com/fabricahq/public-rules.git"})},
			})
			fake.Installations = []githubtest.Installation{{ID: 2, Account: "octo-org", AccountID: 100, Organization: true, Repositories: []string{"octo-org/secret"}}}
			site := newGitHubSite(t, fake, true)
			got, err := site.accounts.Install(ctx, site.account, site.session, 2)
			if err != nil || !slices.ContainsFunc(got.Projects, func(p domain.Project) bool { return p.FullName() == "octo-org/secret" }) {
				t.Fatalf("installing read %+v, %v", got.Projects, err)
			}

			revoke(&fake.Users[0])
			site.now = site.now.Add(domain.RefreshInterval)
			got, err = site.accounts.Refresh(ctx, site.account, site.session)

			if err != nil {
				t.Fatal(err)
			}
			for _, snapshot := range []domain.Snapshot{got, site.snapshot(t)} {
				for _, p := range snapshot.Projects {
					if p.Private {
						t.Errorf("still shows the private %s", p.FullName())
					}
				}
			}
			if installations, _ := site.accounts.Installations(ctx, site.account.ID); len(installations) != 0 {
				t.Errorf("still reads through %+v", installations)
			}
		})
	}
}

// Access removed while a read is under way, by the visitor's "Remove access" or GitHub's webhook, is never undone by
// that read: whether it succeeds or fails, neither what it returns nor what it keeps names a private repository.
func TestAccessRemovedDuringAReadStaysRemoved(t *testing.T) {
	for name, tc := range map[string]struct {
		remove func(*gitHubSite) error
		fail   bool
	}{
		"Remove access during a read that succeeds": {
			remove: func(s *gitHubSite) error { return s.accounts.ForgetInstallations(context.Background(), s.account.ID) },
		},
		"the webhook's removal during a read that fails": {
			remove: func(s *gitHubSite) error { return s.accounts.Store.InstallationRemoved(context.Background(), 5) },
			fail:   true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			fake := monasGitHub()
			fake.Installations = []githubtest.Installation{{ID: 5, Account: "mona", AccountID: monaID, Repositories: []string{"mona/billing"}}}
			site := newGitHubSite(t, fake, true)
			if _, err := site.accounts.Install(ctx, site.account, site.session, 5); err != nil {
				t.Fatal(err)
			}
			var once sync.Once
			fake.Fail = func(path string) bool {
				if !strings.Contains(path, "/repos/mona/billing/contents") {
					return false
				}
				once.Do(func() {
					if err := tc.remove(site); err != nil {
						t.Error(err)
					}
				})
				return tc.fail
			}
			site.now = site.now.Add(domain.RefreshInterval)

			got, _ := site.accounts.Refresh(ctx, site.account, site.session)

			kept, _, err := site.accounts.Store.Snapshot(ctx, site.account.ID)
			if err != nil {
				t.Fatal(err)
			}
			for what, snapshot := range map[string]domain.Snapshot{"returned": got, "kept": kept} {
				for _, p := range snapshot.Projects {
					if p.Private {
						t.Errorf("the %s snapshot shows the private %s", what, p.FullName())
					}
				}
			}
		})
	}
}

// Requests that arrive together, such as a dashboard open in two tabs, read GitHub once between them: the refresh
// limit holds however many ask at once, for a first read and for a refresh.
func TestSimultaneousRequestsReadGitHubOnce(t *testing.T) {
	ctx := context.Background()
	site := newGitHubSite(t, monasGitHub(), false)
	reads := func() int { return site.fake.Requests("GET /user/orgs") }
	together := func(read func() error) {
		t.Helper()
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				if err := read(); err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()
	}

	together(func() error { _, err := site.accounts.Snapshot(ctx, site.account, site.session); return err })
	if reads() != 1 {
		t.Errorf("a first read by 8 requests at once read GitHub %d times, want once", reads())
	}
	site.now = site.now.Add(domain.RefreshInterval)
	together(func() error { _, err := site.accounts.Refresh(ctx, site.account, site.session); return err })
	if reads() != 2 {
		t.Errorf("8 refreshes at once read GitHub %d times in all, want twice", reads())
	}
}

// GitHub's return from installing the app, repeated, such as by reloading it, records the installation once, reads
// GitHub at most once a minute, and keeps the snapshot the first return read.
func TestARepeatedInstallationCallbackKeepsItsSnapshotAndTheRefreshLimit(t *testing.T) {
	ctx := context.Background()
	fake := monasGitHub()
	fake.Installations = []githubtest.Installation{{ID: 5, Account: "mona", AccountID: monaID, Repositories: []string{"mona/billing"}}}
	site := newGitHubSite(t, fake, true)
	if _, err := site.accounts.Install(ctx, site.account, site.session, 5); err != nil {
		t.Fatal(err)
	}
	reads := site.fake.Requests("GET /user/orgs")

	for range 3 {
		got, err := site.accounts.Install(ctx, site.account, site.session, 5)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Projects) != 2 {
			t.Errorf("a repeated return shows projects %+v, want mona/api and mona/billing", got.Projects)
		}
	}

	if again := site.fake.Requests("GET /user/orgs"); again != reads {
		t.Errorf("repeated returns read GitHub %d more times, want none within the minute", again-reads)
	}
	if kept := site.snapshot(t); len(kept.Projects) != 2 {
		t.Errorf("after repeated returns, the kept projects are %+v", kept.Projects)
	}
}

// A visitor with no organizations and more repositories than a read looks into is told the read stopped short, though
// one owner's listing never returns more than the read takes; exactly as many is complete.
func TestAReadOfOneOwnerSaysWhenItLeftRepositoriesOut(t *testing.T) {
	for _, tc := range []struct {
		repositories int
		truncated    bool
	}{
		{domain.MaxRepositories - 1, false},
		{domain.MaxRepositories, false},
		{domain.MaxRepositories + 1, true},
	} {
		t.Run(fmt.Sprint(tc.repositories), func(t *testing.T) {
			fake := &githubtest.Fake{Users: []githubtest.User{{Token: monaToken, ID: monaID, Login: "mona"}}}
			for i := range tc.repositories {
				fake.Repositories = append(fake.Repositories, githubtest.Repository{
					Owner: "mona", Name: fmt.Sprintf("repo-%03d", i), PushedAt: pushed.Add(-time.Duration(i) * time.Hour),
				})
			}
			site := newGitHubSite(t, fake, false)

			if got := site.snapshot(t); got.Truncated != tc.truncated {
				t.Errorf("truncated %v, want %v", got.Truncated, tc.truncated)
			}
		})
	}
}
