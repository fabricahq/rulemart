package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/github"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/github/githubtest"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/store"
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
	accountStore := postgres.New(databasetest.AsWebRole(t, connString))
	sessions := Sessions{Store: accountStore, TokenKeys: FixedTokenKey{domain.NewTokenKey()}}
	site := &gitHubSite{fake: fake, connString: connString, now: time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)}
	api := github.NewAPI(server.URL)
	site.accounts = GitHubAccounts{Store: accountStore, Sessions: sessions, GitHub: api, Now: func() time.Time { return site.now }}
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

// GitHub's webhook says when the app is uninstalled, which, once GitHub confirms the installation is gone, forgets it
// for every account and discards their snapshots, or when the repositories it reads change, which discards them, so
// the next page reads GitHub again. A delivery GitHub didn't sign, or for another app, changes nothing.
func TestWebhookDeliveriesForgetRemovedInstallationsAndDiscardChangedSnapshots(t *testing.T) {
	ctx := context.Background()
	deliver := func(site *gitHubSite, event, body, sig string) error {
		return site.accounts.Deliver(ctx, domain.Delivery{ID: "72d3162e-cc78-11e3-81ab-4c9367dc0958", Event: event, Body: []byte(body), Signature: sig})
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
		site.fake.Installations = nil
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
	// GitHub always sends a delivery's ID, which Rulemart records to recognize the delivery sent again, but checks the
	// signature first, so a delivery GitHub didn't sign learns nothing more, and the ID next, so a signed one without
	// it is refused even when Rulemart would do nothing for it, such as a ping.
	created := `{"action":"created","installation":{"id":5,"app_id":42}}`
	ping := `{"zen":"Keep it logically awesome.","hook_id":1,"installation":{"id":5,"app_id":42}}`
	for name, tc := range map[string]struct {
		id, event, body, signature string
		want                       error
	}{
		"signed without an ID":            {"", "installation", removed, signature(webhookSecret, removed), domain.ErrNoDeliveryID},
		"signed with an ID too long":      {strings.Repeat("a", 101), "installation", removed, signature(webhookSecret, removed), domain.ErrNoDeliveryID},
		"signed with a space in its ID":   {"72d3162e cc78", "installation", removed, signature(webhookSecret, removed), domain.ErrNoDeliveryID},
		"a signed ping without an ID":     {"", "ping", ping, signature(webhookSecret, ping), domain.ErrNoDeliveryID},
		"an ignored action without an ID": {"", "installation", created, signature(webhookSecret, created), domain.ErrNoDeliveryID},
		"unsigned without an ID":          {"", "installation", removed, "", domain.ErrBadSignature},
		"an unsigned ping without an ID":  {"", "ping", ping, signature("another-secret", ping), domain.ErrBadSignature},
	} {
		t.Run(name, func(t *testing.T) {
			site := installed(t)
			err := site.accounts.Deliver(ctx, domain.Delivery{ID: tc.id, Event: tc.event, Body: []byte(tc.body), Signature: tc.signature})
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if count(site, "github_installations") != 1 || count(site, "github_snapshots") != 1 {
				t.Error("a delivery without an ID changed what Rulemart keeps")
			}
		})
	}
}

// suspendable returns mona's site with the app installed on her account, and functions that deliver a signed
// installation event with the action and ID given, and that report whether her installation is suspended. GitHub's
// installation, site.fake.Installations[0], stays as it is: a test suspends it there as its owner would.
func suspendable(t *testing.T) (site *gitHubSite, deliver func(id, action string) error, suspended func() bool) {
	t.Helper()
	ctx := context.Background()
	fake := monasGitHub()
	fake.Installations = []githubtest.Installation{{ID: 5, Account: "mona", AccountID: monaID, Repositories: []string{"mona/billing"}}}
	site = newGitHubSite(t, fake, true)
	if _, err := site.accounts.Install(ctx, site.account, site.session, 5); err != nil {
		t.Fatal(err)
	}
	deliver = func(id, action string) error {
		body := `{"action":"` + action + `","installation":{"id":5,"app_id":42}}`
		return site.accounts.Deliver(ctx, domain.Delivery{ID: id, Event: "installation", Body: []byte(body), Signature: signature(webhookSecret, body)})
	}
	suspended = func() bool {
		t.Helper()
		installations, err := site.accounts.Installations(ctx, site.account.ID)
		if err != nil || len(installations) != 1 {
			t.Fatalf("installations %+v, %v, want installation 5", installations, err)
		}
		return installations[0].Suspended
	}
	return site, deliver, suspended
}

// A delivery Rulemart acted on changes nothing when it arrives again: by its ID, as GitHub redelivers it, or by its
// body, since only the body is signed, so a copy of an old suspension can't suspend the app again under a new ID.
func TestARepeatedDeliveryChangesNothing(t *testing.T) {
	ctx := context.Background()
	site, deliver, suspended := suspendable(t)
	site.fake.Installations[0].Suspended = true
	if err := deliver("delivery-1", "suspend"); err != nil || !suspended() {
		t.Fatalf("a first suspension: %v, suspended %t", err, suspended())
	}
	site.fake.Installations[0].Suspended = false
	if err := deliver("delivery-2", "unsuspend"); err != nil || suspended() {
		t.Fatalf("a new delivery, unsuspending: %v, suspended %t", err, suspended())
	}

	if err := deliver("delivery-1", "suspend"); !errors.Is(err, domain.ErrRepeatedDelivery) {
		t.Errorf("the suspension again: got %v, want ErrRepeatedDelivery", err)
	}
	if err := deliver("delivery-3", "suspend"); !errors.Is(err, domain.ErrRepeatedDelivery) {
		t.Errorf("the suspension's body under a new ID: got %v, want ErrRepeatedDelivery", err)
	}
	other := `{"action":"suspend","installation":{"id":5,"app_id":42},"sender":{"login":"mona"}}`
	err := site.accounts.Deliver(ctx, domain.Delivery{ID: "delivery-2", Event: "installation", Body: []byte(other), Signature: signature(webhookSecret, other)})
	if !errors.Is(err, domain.ErrRepeatedDelivery) {
		t.Errorf("another body under a recorded ID: got %v, want ErrRepeatedDelivery", err)
	}
	if suspended() {
		t.Error("a repeated delivery suspended the installation again")
	}
}

// Rulemart applies what GitHub says of the installation when a delivery arrives, not what the delivery says happened,
// so a copy of an old suspension can't undo the unsuspension that followed it, even once Rulemart has forgotten the
// delivery after domain.DeliveryMemory.
func TestAnOldSuspensionCantUndoANewerUnsuspension(t *testing.T) {
	site, deliver, suspended := suspendable(t)
	start := site.now
	site.fake.Installations[0].Suspended = true
	if err := deliver("delivery-1", "suspend"); err != nil || !suspended() {
		t.Fatalf("the suspension: %v, suspended %t", err, suspended())
	}
	site.fake.Installations[0].Suspended = false
	if err := deliver("delivery-2", "unsuspend"); err != nil || suspended() {
		t.Fatalf("the unsuspension: %v, suspended %t", err, suspended())
	}

	site.now = start.Add(domain.DeliveryMemory)
	if err := deliver("delivery-1", "suspend"); !errors.Is(err, domain.ErrRepeatedDelivery) || suspended() {
		t.Fatalf("exactly DeliveryMemory later: got %v, suspended %t, want ErrRepeatedDelivery", err, suspended())
	}
	site.now = start.Add(domain.DeliveryMemory + time.Second)
	if err := deliver("delivery-1", "suspend"); err != nil || suspended() {
		t.Fatalf("after DeliveryMemory: got %v, suspended %t, want it acted on and the installation still unsuspended", err, suspended())
	}
	var remembered int
	postgrestest.QueryRow(t, site.connString, "SELECT count(*) FROM github_deliveries", &remembered)
	if remembered != 1 {
		t.Errorf("remembers %d deliveries, want only the latest", remembered)
	}
}

// A delivery only says which installation changed: Rulemart applies what GitHub says of it now, whatever the delivery
// says happened, and discards the snapshots of the accounts that read through it, so their next page reads GitHub.
func TestADeliveryAppliesWhatGitHubSaysOfTheInstallationNow(t *testing.T) {
	for name, tc := range map[string]struct {
		event, action     string
		gone, isSuspended bool
		wantKept          bool
		wantSuspended     bool
	}{
		"a suspension of an active installation":    {"installation", "suspend", false, false, true, false},
		"an unsuspension of a suspended one":        {"installation", "unsuspend", false, true, true, true},
		"a removal of an active one":                {"installation", "deleted", false, false, true, false},
		"a change of repositories of a removed one": {"installation_repositories", "added", true, false, false, false},
		"a suspension of a removed one":             {"installation", "suspend", true, false, false, false},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			site, _, _ := suspendable(t)
			site.fake.Installations[0].Suspended = tc.isSuspended
			if tc.gone {
				site.fake.Installations = nil
			}
			body := `{"action":"` + tc.action + `","installation":{"id":5,"app_id":42}}`

			err := site.accounts.Deliver(ctx, domain.Delivery{ID: "delivery-1", Event: tc.event, Body: []byte(body), Signature: signature(webhookSecret, body)})

			if err != nil {
				t.Fatal(err)
			}
			installations, err := site.accounts.Installations(ctx, site.account.ID)
			if err != nil || (len(installations) == 1) != tc.wantKept || (tc.wantKept && installations[0].Suspended != tc.wantSuspended) {
				t.Errorf("installations %+v, %v; want kept: %t, suspended: %t", installations, err, tc.wantKept, tc.wantSuspended)
			}
			var snapshots int
			postgrestest.QueryRow(t, site.connString, "SELECT count(*) FROM github_snapshots", &snapshots)
			if snapshots != 0 {
				t.Error("kept the snapshot")
			}
		})
	}
}

// Copies of one delivery that arrive together act once between them, and ask GitHub once: the account's GitHub
// generation, which each application advances, advances once.
func TestSimultaneousCopiesOfADeliveryActOnce(t *testing.T) {
	site, deliver, suspended := suspendable(t)
	site.fake.Installations[0].Suspended = true
	generation := func() int64 {
		t.Helper()
		var generation int64
		postgrestest.QueryRow(t, site.connString, fmt.Sprintf("SELECT github_generation FROM accounts WHERE id = %d", site.account.ID), &generation)
		return generation
	}
	before, readsBefore := generation(), site.fake.Requests(installationRoute)
	errs := make([]error, 8)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Go(func() { errs[i] = deliver("delivery-1", "suspend") })
	}
	wg.Wait()

	applied := 0
	for _, err := range errs {
		switch {
		case err == nil:
			applied++
		case !errors.Is(err, domain.ErrRepeatedDelivery):
			t.Errorf("a copy failed: %v", err)
		}
	}
	if applied != 1 || !suspended() {
		t.Errorf("%d copies acted, suspended %t, want one", applied, suspended())
	}
	if after := generation(); after != before+1 {
		t.Errorf("the generation advanced from %d to %d, want once", before, after)
	}
	if reads := site.fake.Requests(installationRoute) - readsBefore; reads != 1 {
		t.Errorf("the copies asked GitHub about the installation %d times, want once", reads)
	}
}

// GitHub answers 404 for an installation it doesn't know, but 403 when it refuses to say, which doesn't mean the app
// was uninstalled: a delivery whose read GitHub refuses fails, keeping the installation and its snapshot and recording
// nothing, so GitHub's redelivery of it is acted on once GitHub answers.
func TestAnInstallationGitHubRefusesToDescribeIsntTakenForRemoved(t *testing.T) {
	site, deliver, suspended := suspendable(t)
	snapshots := func() int {
		var n int
		postgrestest.QueryRow(t, site.connString, "SELECT count(*) FROM github_snapshots", &n)
		return n
	}
	site.fake.Fail = func(path string) bool { return strings.HasPrefix(path, "/app/installations/") }
	site.fake.FailStatus = http.StatusForbidden
	site.fake.Installations[0].Suspended = true

	if err := deliver("delivery-1", "deleted"); err == nil {
		t.Fatal("a delivery whose read GitHub refused succeeded")
	}
	if suspended() || snapshots() != 1 {
		t.Fatalf("a refused read changed the installation or discarded the snapshot")
	}

	site.fake.Fail = nil
	if err := deliver("delivery-1", "deleted"); err != nil || !suspended() {
		t.Fatalf("GitHub's redelivery once GitHub answers: got %v, suspended %t, want it acted on", err, suspended())
	}
	site.fake.Installations = nil
	if err := deliver("delivery-2", "suspend"); err != nil {
		t.Fatalf("a delivery for an installation GitHub doesn't know: %v", err)
	}
	if installations, err := site.accounts.Installations(context.Background(), site.account.ID); err != nil || len(installations) != 0 {
		t.Errorf("installations %+v, %v, want the one GitHub doesn't know forgotten", installations, err)
	}
}

// installationRoute is the fake GitHub's route that says what GitHub says of an installation now.
const installationRoute = "GET /app/installations/{id}"

// A delivery Rulemart already acted on is answered as a repeat without asking GitHub, even while GitHub fails, so a
// copy sent again costs GitHub nothing; but one whose read of GitHub failed is recorded nowhere, so GitHub's
// redelivery of it is acted on, not taken for a repeat.
func TestARepeatedDeliveryDoesntAskGitHub(t *testing.T) {
	site, deliver, suspended := suspendable(t)
	site.fake.Installations[0].Suspended = true
	if err := deliver("delivery-1", "suspend"); err != nil {
		t.Fatal(err)
	}
	reads := site.fake.Requests(installationRoute)
	site.fake.Fail = func(path string) bool { return strings.HasPrefix(path, "/app/installations/") }

	if err := deliver("delivery-1", "suspend"); !errors.Is(err, domain.ErrRepeatedDelivery) {
		t.Errorf("the delivery again while GitHub fails: got %v, want ErrRepeatedDelivery", err)
	}
	if got := site.fake.Requests(installationRoute); got != reads {
		t.Errorf("the repeat asked GitHub about the installation %d times, want none", got-reads)
	}

	site.fake.Installations[0].Suspended = false
	if err := deliver("delivery-2", "unsuspend"); err == nil || errors.Is(err, domain.ErrRepeatedDelivery) || !suspended() {
		t.Fatalf("a new delivery while GitHub fails: got %v, suspended %t, want it to fail and change nothing", err, suspended())
	}
	site.fake.Fail = nil
	if err := deliver("delivery-2", "unsuspend"); err != nil || suspended() {
		t.Errorf("GitHub's redelivery once GitHub answers: got %v, suspended %t, want it acted on", err, suspended())
	}
}

// Deliveries for one installation act one at a time, each asking GitHub once the one before it committed, so a read
// GitHub answers slowly, from before a newer change, can't commit after the newer delivery's and undo it.
func TestASlowReadOfAnInstallationCantUndoANewerDeliverys(t *testing.T) {
	site, deliver, suspended := suspendable(t)
	held, release := make(chan struct{}), make(chan struct{})
	var holding atomic.Bool
	site.fake.Answering = func(string) {
		if holding.CompareAndSwap(false, true) {
			close(held)
			<-release
		}
	}
	site.fake.Installations[0].Suspended = true
	older := make(chan error, 1)
	go func() { older <- deliver("delivery-1", "suspend") }()
	<-held

	site.fake.Installations[0].Suspended = false
	newerDone := make(chan struct{})
	var newer error
	go func() {
		defer close(newerDone)
		newer = deliver("delivery-2", "unsuspend")
	}()
	// The newer delivery either acts while the older one's read is held, or waits for the older one to commit.
	waitFor(t, "the newer delivery to act or wait for the older one", func() bool {
		select {
		case <-newerDone:
			return true
		default:
			var waiting int
			postgrestest.QueryRow(t, site.connString, `SELECT count(*) FROM pg_stat_activity
				WHERE datname = current_database() AND wait_event_type = 'Lock'`, &waiting)
			return waiting > 0
		}
	})
	close(release)

	if err := <-older; err != nil {
		t.Errorf("the older delivery: %v", err)
	}
	<-newerDone
	if newer != nil {
		t.Errorf("the newer delivery: %v", newer)
	}
	if suspended() {
		t.Error("the older delivery's read of a suspension outlasted the newer unsuspension")
	}
}

// waitFor waits until done reports true, failing the test after ten seconds.
func waitFor(t *testing.T, what string, done func() bool) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); !done(); {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Suspending the app on GitHub hides the private repositories it reads, without forgetting the installation, and
// unsuspending it shows them again, each at once, though GitHub refuses a suspended installation a token.
func TestASuspendedInstallationsRepositoriesDisappearAndReturnWhenUnsuspended(t *testing.T) {
	ctx := context.Background()
	fake := monasGitHub()
	fake.Installations = []githubtest.Installation{{ID: 5, Account: "mona", AccountID: monaID, Repositories: []string{"mona/billing"}}}
	site := newGitHubSite(t, fake, true)
	if _, err := site.accounts.Install(ctx, site.account, site.session, 5); err != nil {
		t.Fatal(err)
	}
	deliver := func(action string, suspended bool) {
		t.Helper()
		fake.Installations[0].Suspended = suspended
		body := `{"action":"` + action + `","installation":{"id":5,"app_id":42}}`
		delivery := domain.Delivery{ID: action, Event: "installation", Body: []byte(body), Signature: signature(webhookSecret, body)}
		if err := site.accounts.Deliver(ctx, delivery); err != nil {
			t.Fatalf("deliver %s: %v", action, err)
		}
	}
	projects := func() []string {
		t.Helper()
		var names []string
		for _, p := range site.snapshot(t).Projects {
			names = append(names, p.FullName())
		}
		return names
	}

	deliver("suspend", true)
	if got := projects(); !slices.Equal(got, []string{"mona/api"}) {
		t.Errorf("while suspended, projects %v, want mona/api only", got)
	}
	if installations, err := site.accounts.Installations(ctx, site.account.ID); err != nil || len(installations) != 1 || !installations[0].Suspended {
		t.Errorf("while suspended, installations %+v, %v, want installation 5, suspended", installations, err)
	}

	deliver("unsuspend", false)
	if got := projects(); !slices.Equal(got, []string{"mona/api", "mona/billing"}) {
		t.Errorf("after unsuspending, projects %v, want mona/api and mona/billing", got)
	}
	if installations, err := site.accounts.Installations(ctx, site.account.ID); err != nil || len(installations) != 1 || installations[0].Suspended {
		t.Errorf("after unsuspending, installations %+v, %v, want installation 5, not suspended", installations, err)
	}
}

// A page that arrives while another request is reading the visitor's GitHub account for the first time, so there's no
// snapshot to show yet, is told a read is under way, and reads nothing itself; one that arrives once a snapshot is
// kept shows it.
func TestASnapshotRequestedWhileTheFirstReadIsUnderWaySaysSo(t *testing.T) {
	ctx := context.Background()
	site := newGitHubSite(t, monasGitHub(), false)
	// Another request claims the first read and hasn't kept anything yet.
	claim, err := site.accounts.Store.ClaimRead(ctx, site.account.ID, site.now, domain.RefreshInterval)
	if err != nil || !claim.Claimed {
		t.Fatalf("claim %+v, %v", claim, err)
	}

	got, err := site.accounts.Snapshot(ctx, site.account, site.session)

	if !errors.Is(err, ErrGitHubReading) || got.ReadFailed || !got.ReadAt.IsZero() {
		t.Errorf("got %+v, %v, want an empty snapshot and ErrGitHubReading", got, err)
	}
	if reads := site.fake.Requests("GET /user/orgs"); reads != 0 {
		t.Errorf("read GitHub %d times while another request was reading it", reads)
	}

	if saved, err := site.accounts.Store.SaveSnapshot(ctx, site.account.ID, claim.Generation, domain.Snapshot{ReadAt: site.now}); err != nil || !saved {
		t.Fatalf("saved %v, %v", saved, err)
	}
	if got, err := site.accounts.Snapshot(ctx, site.account, site.session); err != nil || !got.ReadAt.Equal(site.now) {
		t.Errorf("once kept, got %+v, %v", got, err)
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
	if err := site.accounts.Deliver(context.Background(), domain.Delivery{ID: "delivery-1", Event: "installation"}); !errors.Is(err, ErrNoApp) {
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

// An installation belongs to the GitHub account it's on, whatever that account is called now: an organization renamed
// since the visitor installed the app, whose old name another organization they own took, is checked by its new name,
// where they're only a member, so the installation is forgotten.
func TestAnInstallationOnARenamedOrganizationIsCheckedByItsCurrentName(t *testing.T) {
	ctx := context.Background()
	fake := monasGitHub()
	fake.Users[0].Organizations[0].Role = "admin"
	fake.Repositories = append(fake.Repositories, githubtest.Repository{
		Owner: "octo-org", Name: "secret", Private: true, PushedAt: pushed.Add(-8 * time.Hour),
		Files: map[string]string{domain.ProvenancePath: githubtest.Provenance(githubtest.ProvenanceSource{Name: "fabrica", Repository: "https://github.com/fabricahq/public-rules.git"})},
	})
	fake.Installations = []githubtest.Installation{{ID: 2, Account: "octo-org", AccountID: 100, Organization: true, Repositories: []string{"octo-org/secret"}}}
	site := newGitHubSite(t, fake, true)
	if _, err := site.accounts.Install(ctx, site.account, site.session, 2); err != nil {
		t.Fatal(err)
	}

	fake.Installations[0].Account = "octo-renamed"
	fake.Users[0].Organizations = []githubtest.Membership{{Organization: "octo-renamed", Role: "member"}, {Organization: "octo-org", Role: "admin"}}
	site.now = site.now.Add(domain.RefreshInterval)
	got, err := site.accounts.Refresh(ctx, site.account, site.session)

	if err != nil {
		t.Fatal(err)
	}
	for _, p := range got.Projects {
		if p.Private {
			t.Errorf("still shows the private %s", p.FullName())
		}
	}
	if installations, _ := site.accounts.Installations(ctx, site.account.ID); len(installations) != 0 {
		t.Errorf("still reads through %+v", installations)
	}
}

// An installation on the visitor's own account stays theirs when they rename it: it's still on their GitHub user.
func TestAnInstallationOnTheVisitorsRenamedAccountStaysTheirs(t *testing.T) {
	ctx := context.Background()
	fake := monasGitHub()
	fake.Installations = []githubtest.Installation{{ID: 5, Account: "mona", AccountID: monaID, Repositories: []string{"mona/billing"}}}
	site := newGitHubSite(t, fake, true)
	if _, err := site.accounts.Install(ctx, site.account, site.session, 5); err != nil {
		t.Fatal(err)
	}

	fake.Users[0].Login, fake.Installations[0].Account, site.account.Login = "mona-renamed", "mona-renamed", "mona-renamed"
	site.now = site.now.Add(domain.RefreshInterval)
	got, err := site.accounts.Refresh(ctx, site.account, site.session)

	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(got.Projects, func(p domain.Project) bool { return p.Private && p.FullName() == "mona/billing" }) {
		t.Errorf("projects %+v, want the private mona/billing", got.Projects)
	}
	if installations, _ := site.accounts.Installations(ctx, site.account.ID); len(installations) != 1 {
		t.Errorf("reads through %+v, want installation 5", installations)
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

// hookedStore is a store that runs beforeClaim, when set, as each read claims its attempt, and afterRejectedSave after
// a save it rejected because access changed, so a test can act at exactly those moments.
type hookedStore struct {
	store.Store
	beforeClaim       func()
	afterRejectedSave func()
}

func (s hookedStore) SaveSnapshot(ctx context.Context, accountID, generation int64, snapshot domain.Snapshot) (bool, error) {
	saved, err := s.Store.SaveSnapshot(ctx, accountID, generation, snapshot)
	if err == nil && !saved && s.afterRejectedSave != nil {
		s.afterRejectedSave()
	}
	return saved, err
}

func (s hookedStore) ClaimRead(ctx context.Context, accountID int64, now time.Time, interval time.Duration) (store.ReadClaim, error) {
	if s.beforeClaim != nil {
		s.beforeClaim()
	}
	return s.Store.ClaimRead(ctx, accountID, now, interval)
}

// Access removed just before a refresh claims its read, after the snapshot it would fall back on was kept, is never
// undone when GitHub then fails: the failed read keeps nothing private, and returns nothing private.
func TestAccessRemovedAsAFailingRefreshBeginsStaysRemoved(t *testing.T) {
	ctx := context.Background()
	fake := monasGitHub()
	fake.Installations = []githubtest.Installation{{ID: 5, Account: "mona", AccountID: monaID, Repositories: []string{"mona/billing"}}}
	site := newGitHubSite(t, fake, true)
	if got, err := site.accounts.Install(ctx, site.account, site.session, 5); err != nil || len(got.Projects) != 2 {
		t.Fatalf("installing read %+v, %v", got.Projects, err)
	}
	var once sync.Once
	site.accounts.Store = hookedStore{Store: site.accounts.Store, beforeClaim: func() {
		once.Do(func() {
			if err := site.accounts.ForgetInstallations(ctx, site.account.ID); err != nil {
				t.Error(err)
			}
		})
	}}
	fake.Fail = func(path string) bool { return path == "/user/orgs" }
	site.now = site.now.Add(domain.RefreshInterval)

	got, err := site.accounts.Refresh(ctx, site.account, site.session)

	if !errors.Is(err, ErrGitHubRead) {
		t.Errorf("got %v, want ErrGitHubRead", err)
	}
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
}

// Requests that arrive together, such as a dashboard open in two tabs, read GitHub once between them: the refresh
// limit holds however many ask at once, for a first read and for a refresh. A request that arrives while the first read
// is under way is told so.
func TestSimultaneousRequestsReadGitHubOnce(t *testing.T) {
	ctx := context.Background()
	site := newGitHubSite(t, monasGitHub(), false)
	reads := func() int { return site.fake.Requests("GET /user/orgs") }
	together := func(read func() error) {
		t.Helper()
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				if err := read(); err != nil && !errors.Is(err, ErrGitHubReading) {
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

// A read whose access changed while it was under way reads again only under a claim of its own, as any read does: a
// request that arrives between its attempts reads GitHub in its place, and once the retry has read, the refresh limit
// holds again for a request right after it.
func TestARetryAfterAccessChangedClaimsItsRead(t *testing.T) {
	for name, compete := range map[string]bool{
		"a request between attempts reads in the retry's place": true,
		"the retry holds the refresh limit":                     false,
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			fake := monasGitHub()
			site := newGitHubSite(t, fake, false)
			site.snapshot(t)
			reads := func() int { return fake.Requests("GET /user/orgs") }
			before := reads()
			var discard, between sync.Once
			fake.Fail = func(path string) bool {
				if strings.Contains(path, "/contents/") {
					discard.Do(func() {
						if err := site.accounts.ForgetInstallations(ctx, site.account.ID); err != nil {
							t.Error(err)
						}
					})
				}
				return false
			}
			refresh := func() domain.Snapshot {
				got, err := site.accounts.Refresh(ctx, site.account, site.session)
				if err != nil {
					t.Error(err)
				}
				return got
			}
			hooked := hookedStore{Store: site.accounts.Store}
			if compete {
				hooked.afterRejectedSave = func() { between.Do(func() { refresh() }) }
			}
			site.accounts.Store = hooked
			site.now = site.now.Add(domain.RefreshInterval)

			got := refresh()
			refresh()

			if n := reads() - before; n != 2 {
				t.Errorf("read GitHub %d times, want twice: the first attempt, then one read after access changed", n)
			}
			if len(got.Libraries) == 0 {
				t.Errorf("the refresh returned %+v, want what was read after access changed", got)
			}
		})
	}
}

// GitHub's return from installing the app, repeated after the visitor changed which repositories it reads, reads GitHub
// again at once, within the minute since the last read, and shows the new selection; the installation is recorded
// once.
func TestARepeatedInstallationCallbackShowsTheNewSelectionAtOnce(t *testing.T) {
	ctx := context.Background()
	fake := monasGitHub()
	fake.Repositories = append(fake.Repositories, githubtest.Repository{
		Owner: "mona", Name: "ledger", Private: true, PushedAt: pushed.Add(-8 * time.Hour),
		Files: map[string]string{"rule-library.yaml": "schemaVersion: 1\n"}, Tags: []string{"release/1"},
	})
	fake.Installations = []githubtest.Installation{{ID: 5, Account: "mona", AccountID: monaID, Repositories: []string{"mona/billing"}}}
	site := newGitHubSite(t, fake, true)
	if _, err := site.accounts.Install(ctx, site.account, site.session, 5); err != nil {
		t.Fatal(err)
	}

	fake.Installations[0].Repositories = append(fake.Installations[0].Repositories, "mona/ledger")
	got, err := site.accounts.Install(ctx, site.account, site.session, 5)

	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(got.Libraries, func(l domain.PublishableRepository) bool { return l.FullName() == "mona/ledger" }) {
		t.Errorf("the repeated return shows libraries %+v, want the newly chosen mona/ledger among them", got.Libraries)
	}
	if kept := site.snapshot(t); !slices.ContainsFunc(kept.Libraries, func(l domain.PublishableRepository) bool { return l.FullName() == "mona/ledger" }) {
		t.Errorf("the kept libraries are %+v, want mona/ledger among them", kept.Libraries)
	}
	if installations, _ := site.accounts.Installations(ctx, site.account.ID); len(installations) != 1 {
		t.Errorf("installations %+v, want installation 5 once", installations)
	}
}

// A repeated return from installing the app whose read of GitHub fails keeps the snapshot the first return read, and
// says the read failed, rather than showing nothing.
func TestARepeatedInstallationCallbackWhoseReadFailsKeepsTheSnapshot(t *testing.T) {
	ctx := context.Background()
	fake := monasGitHub()
	fake.Installations = []githubtest.Installation{{ID: 5, Account: "mona", AccountID: monaID, Repositories: []string{"mona/billing"}}}
	site := newGitHubSite(t, fake, true)
	if _, err := site.accounts.Install(ctx, site.account, site.session, 5); err != nil {
		t.Fatal(err)
	}

	fake.Fail = func(path string) bool { return path == "/user/orgs" }
	got, err := site.accounts.Install(ctx, site.account, site.session, 5)

	if !errors.Is(err, ErrGitHubRead) {
		t.Fatalf("got %v, want ErrGitHubRead", err)
	}
	if !got.ReadFailed || len(got.Projects) != 2 {
		t.Errorf("the failed return shows %+v, want mona/api and mona/billing, saying the read failed", got)
	}
	if kept := site.snapshot(t); !kept.ReadFailed || len(kept.Projects) != 2 {
		t.Errorf("after the failed return, the kept snapshot is %+v", kept)
	}
}

// A visitor in more organizations than a read lists repositories of is told the read left repositories out; exactly as
// many is complete.
func TestAReadSaysWhenItLeftOrganizationsOut(t *testing.T) {
	for _, tc := range []struct {
		organizations int
		truncated     bool
	}{
		{domain.MaxOrganizations, false},
		{domain.MaxOrganizations + 1, true},
	} {
		t.Run(fmt.Sprint(tc.organizations), func(t *testing.T) {
			user := githubtest.User{Token: monaToken, ID: monaID, Login: "mona"}
			for i := range tc.organizations {
				user.Organizations = append(user.Organizations, githubtest.Membership{Organization: fmt.Sprintf("org-%03d", i), Role: "member"})
			}
			site := newGitHubSite(t, &githubtest.Fake{Users: []githubtest.User{user}}, false)

			got := site.snapshot(t)

			if got.Truncated != tc.truncated || len(got.Organizations) != domain.MaxOrganizations {
				t.Errorf("truncated %v with %d organizations, want %v with %d", got.Truncated, len(got.Organizations), tc.truncated, domain.MaxOrganizations)
			}
		})
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
