package web_test

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	accountsapp "github.com/fabricahq/rulemart/internal/contexts/accounts/app"
	accounts "github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
	"github.com/fabricahq/rulemart/internal/platform/web"
)

// dashboardSite is the pages with sign-in, listings, stars, and visitors' GitHub accounts through fakes, octocat signed
// in.
type dashboardSite struct {
	handler  http.Handler
	accounts *fakeAccounts
	gitHub   *fakeGitHubAccounts
	listings *fakeListings
	session  *http.Cookie
	logs     *bytes.Buffer
}

// octocatsGitHub is what Rulemart reads of octocat's GitHub account: a member of octo-org, with a library to add, one
// on Rulemart, a private one, and two projects, one private, that import example/rules, one behind it.
func octocatsGitHub() accounts.Snapshot {
	behind := []accounts.PinnedRule{{Path: "techs/go/return-errors", Version: coderules.RuleVersion{Major: 1}}}
	current := []accounts.PinnedRule{{Path: "techs/go/return-errors", Version: coderules.RuleVersion{Major: 2}}}
	return accounts.Snapshot{
		ReadAt:        time.Now().Add(-3 * time.Minute),
		Organizations: []string{"octo-org", "example"},
		Libraries: []accounts.PublishableRepository{
			{Repository: accounts.Repository{Owner: "octocat", Name: "new-rules"}, Release: 2},
			{Repository: accounts.Repository{Owner: "example", Name: "rules"}, Release: 3},
			{Repository: accounts.Repository{Owner: "octocat", Name: "team-rules", Private: true}, Release: 1},
		},
		Projects: []accounts.Project{
			{Repository: accounts.Repository{Owner: "octocat", Name: "api"}, Sources: []accounts.Source{{Name: "example", Library: "example/rules", Rules: behind}}},
			{Repository: accounts.Repository{Owner: "octocat", Name: "billing", Private: true}, Sources: []accounts.Source{{Name: "ex", Library: "example/rules", Rules: current}}},
		},
	}
}

// octocatsCatalog is newCatalog with what the dashboard reads: example/rules, which octo-org publishes, and
// octo-org/new, listed today; and example/rules' rules as they stand.
func octocatsCatalog() catalog {
	c := newCatalog()
	c.dashboard = views.Dashboard{
		Owned: []views.OwnedLibrary{
			{Library: views.LibraryRef{Owner: "example", Name: "rules"}, Vetted: true, Rules: 2, Stars: 1235, AddedAt: day(1)},
			{Library: views.LibraryRef{Owner: "octo-org", Name: "new"}, Rules: 1, AddedAt: time.Now().Add(-time.Hour)},
		},
		Imported: []views.ImportedLibrary{{
			Library: views.LibraryRef{Owner: "example", Name: "rules"}, Vetted: true,
			Rules: domain.RuleStates{Current: map[string]coderules.RuleVersion{"techs/go/return-errors": {Major: 2}}, Retired: map[string]bool{}},
		}},
	}
	return c
}

func newDashboardSite(t *testing.T, snapshot accounts.Snapshot, c catalog) dashboardSite {
	t.Helper()
	site := dashboardSite{
		accounts: newFakeAccounts(), gitHub: newFakeGitHubAccounts(snapshot), logs: &bytes.Buffer{},
		listings: &fakeListings{byAccount: map[int64][]views.AccountListing{}},
	}
	handler, err := web.New(c, web.Options{
		Log: slog.New(slog.NewJSONHandler(site.logs, nil)), Accounts: site.accounts, GitHub: &fakeGitHub{identity: octocat},
		Listings: site.listings, Stars: newFakeStars(), GitHubAccounts: site.gitHub,
	})
	if err != nil {
		t.Fatal(err)
	}
	site.handler = handler
	site.session = &http.Cookie{Name: sessionCookie, Value: string(site.accounts.signedIn(t, octocat.WithName("The Octocat")))}
	return site
}

func (s dashboardSite) get(t *testing.T, target string) string {
	t.Helper()
	resp := send(t, s.handler, request{method: http.MethodGet, target: target, cookies: []*http.Cookie{s.session}})
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("%s answered %d, cached as %q", target, resp.StatusCode, resp.Header.Get("Cache-Control"))
	}
	return body(t, resp)
}

// The dashboard shows the visitor, their organizations, the libraries they and their organizations publish, with
// their stars and New on one listed today, and the libraries their projects use, with the updates waiting for each
// project, as the prototype's does.
func TestTheDashboardShowsTheVisitorsLibrariesAndProjects(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())

	page := site.get(t, "/me")

	assertShows(t, page,
		"Dashboard The Octocat @octocat · member of octo-org, example", "My libraries Starred rules , 0",
		"Showing public repos only. Include private projects", "Read from GitHub 3 minutes ago. Refresh",
		"Published by you and your orgs 2 rules example/rules · 2 rules ★ 1,235 1,235 stars in all",
		"new New Unvetted octo-org/new · 1 rule ★ 0 0 stars in all", "+ Add a library",
		"Used in your projects 1 rules octocat/api · 1 rule update · octocat/billing (private) · up to date 2 projects",
		"Read from each project's .code-rules/generated/provenance.json .",
		"Account GitHub user ID 583231", "Sign out everywhere", "Delete my account")
	if got := rels(t, page, "/octo-org/new"); !slices.Equal(got, []string{"nofollow"}) {
		t.Errorf("the unvetted library links with rel %q", got)
	}
	if content, _ := robots(t, page); content != "noindex" {
		t.Errorf("robots %q, want noindex", content)
	}
	if got := formActions(t, page); !slices.Contains(got, "/me/refresh?return=%2Fme") {
		t.Errorf("the page's forms post to %q, want Refresh's", got)
	}
}

// A visitor in no organization, with no projects and nothing published, sees each section say so.
func TestTheDashboardSaysWhenTheVisitorHasNothingYet(t *testing.T) {
	site := newDashboardSite(t, accounts.Snapshot{ReadAt: time.Now()}, newCatalog())

	page := site.get(t, "/me")

	assertShows(t, page, "The Octocat @octocat My libraries",
		"Published by you and your orgs 0 No library of yours or your organizations' is on Rulemart yet.",
		"Used in your projects 0 None of your projects imports a library that's on Rulemart.")
	if strings.Contains(visibleText(t, page), "member of") {
		t.Error("the head names organizations the visitor isn't in")
	}
}

// Once the visitor installed the GitHub App, the note says private projects are included, and leads to managing them.
func TestTheDashboardSaysWhenItIncludesPrivateProjects(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
	site.gitHub.installations[1] = []accounts.Installation{{ID: 9, Account: "octocat"}}

	page := site.get(t, "/me")

	assertShows(t, page, "Including private projects from the repos you selected. Private projects are only visible to you. Manage")
	if got := links(t, page, "Manage"); !slices.Equal(got, []string{"/me/private"}) {
		t.Errorf("Manage leads to %q", got)
	}
}

// When the read of GitHub fails, the dashboard shows what an earlier read found, says the read failed, and offers to
// try again; a session without a token GitHub takes is asked to sign in again; a read too large says it stopped.
func TestTheDashboardSaysHowItsReadOfGitHubWent(t *testing.T) {
	failed := octocatsGitHub()
	failed.ReadFailed = true
	site := newDashboardSite(t, failed, octocatsCatalog())
	site.gitHub.err = fmt.Errorf("read GitHub: %w: GitHub answered 502", accountsapp.ErrGitHubRead)
	site.gitHub.snapshot = failed
	assertShows(t, site.get(t, "/me"), "Rulemart couldn't read your repositories on GitHub just now. Showing what it read 3 minutes ago. Try again",
		"octocat/api · 1 rule update")
	if !strings.Contains(site.logs.String(), "GitHub answered 502") {
		t.Errorf("didn't log the failed read: %s", site.logs)
	}

	tokenless := newDashboardSite(t, accounts.Snapshot{}, octocatsCatalog())
	tokenless.gitHub.err = accountsapp.ErrNoGitHubToken
	page := tokenless.get(t, "/me")
	assertShows(t, page, "Sign in again so Rulemart can read your repositories on GitHub. Sign in again", "Rulemart hasn't read your projects yet.")
	if got := links(t, page, "Sign in again"); !slices.Equal(got, []string{"/signin?again=1&return=%2Fme"}) {
		t.Errorf("Sign in again leads to %q", got)
	}
	again := body(t, send(t, tokenless.handler, request{method: http.MethodGet, target: "/signin?again=1&return=%2Fme", cookies: []*http.Cookie{tokenless.session}}))
	assertShows(t, again, "Sign in again so Rulemart can read your repositories on GitHub.", "Continue with GitHub")

	truncated := octocatsGitHub()
	truncated.Truncated = true
	assertShows(t, newDashboardSite(t, truncated, octocatsCatalog()).get(t, "/me"),
		"Rulemart reads your 200 most recently pushed repositories, and yours and your organizations' hold more.")

	broken := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
	broken.gitHub.err = errors.New("connection refused")
	broken.gitHub.kept = map[int64]accounts.Snapshot{}
	if resp := send(t, broken.handler, request{method: http.MethodGet, target: "/me", cookies: []*http.Cookie{broken.session}}); resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("a failure to read the snapshot answered %d, want 503", resp.StatusCode)
	}
}

// Refresh reads GitHub again and returns to the page it came from, one of the visitor's; a session without a token
// GitHub takes is sent to sign in again.
func TestRefreshReadsGitHubAgainAndReturns(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
	site.get(t, "/me")

	for target, want := range map[string]string{
		"/me/refresh":                    "/me",
		"/me/refresh?return=%2Fme%2Fadd": "/me/add",
		"/me/refresh?return=%2Fcart":     "/cart",
		"/me/refresh?return=%2Ffaq":      "/me",
	} {
		resp := send(t, site.handler, request{method: http.MethodPost, target: target, cookies: []*http.Cookie{site.session}})
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != want {
			t.Errorf("%s answered %d to %q, want %q", target, resp.StatusCode, resp.Header.Get("Location"), want)
		}
	}
	if site.gitHub.reads != 5 {
		t.Errorf("read GitHub %d times, want once for the page and once a refresh", site.gitHub.reads)
	}
	site.gitHub.err = accountsapp.ErrNoGitHubToken
	resp := send(t, site.handler, request{method: http.MethodPost, target: "/me/refresh", cookies: []*http.Cookie{site.session}})
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/signin?again=1&return=%2Fme" {
		t.Errorf("without a token, Refresh answered %d to %q", resp.StatusCode, resp.Header.Get("Location"))
	}
}

// The account menu names the visitor, then leads to the dashboard, adding a library, and Starred rules, and signs out,
// as the prototype's does.
func TestTheAccountMenuReadsAsThePrototypes(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
	page := site.get(t, "/faq")
	assertShows(t, page, "The Octocat Signed in as @octocat Dashboard Add a library Starred rules Sign out")
	for name, want := range map[string]string{"Dashboard": "/me", "Add a library": "/me/add", "Starred rules": "/me?tab=stars"} {
		if got := links(t, page, name); !slices.Equal(got, []string{want}) {
			t.Errorf("%s leads to %q, want %q", name, got, want)
		}
	}
}

// The old addresses of the account's pages lead to their places on the dashboard, and the sign-in page's to its new one.
func TestTheAccountsOldAddressesRedirect(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
	for target, want := range map[string]string{
		"/account":               "/me",
		"/account/stars":         "/me?tab=stars",
		"/account/listings":      "/me/listings",
		"/list":                  "/me/add",
		"/sign-in?return=%2Ffaq": "/signin?return=%2Ffaq",
	} {
		resp := send(t, site.handler, request{method: http.MethodGet, target: target})
		if resp.StatusCode != http.StatusMovedPermanently || resp.Header.Get("Location") != want {
			t.Errorf("%s answered %d to %q, want %q", target, resp.StatusCode, resp.Header.Get("Location"), want)
		}
	}
}

// The picker lists the repositories that publish a library in their three kinds: one to add, one on Rulemart, by way of
// the visitor's organization, and a private one, dimmed, that can't be added; a library being added leads to its check.
func TestTheAddPageListsTheVisitorsLibrariesByWhatAddingDoes(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())

	page := site.get(t, "/me/add")

	assertShows(t, page, "Publish Add a library", "Your libraries on GitHub",
		"octocat/new-rules Public · release/2 Add this library",
		"example/rules Public · via the example organization ✓ On Rulemart",
		"octocat/team-rules Private · release/1 Private libraries can't be published on Rulemart",
		"octo-org/new Public · via the octo-org organization ✓ On Rulemart",
		"Only public libraries can be published on Rulemart. Rulemart can only see your public repos right now. Include private repos")
	if got := formActions(t, page); !slices.Equal(got, []string{"/signout", "/me/add?repository=octocat%2Fnew-rules", "/me/refresh?return=%2Fme%2Fadd"}) {
		t.Errorf("the page's forms post to %q", got)
	}

	site.listings.byAccount[1] = []views.AccountListing{{ID: 3, Owner: "octocat", Name: "new-rules", State: domain.ListingChecking}}
	if got := links(t, site.get(t, "/me/add"), "Adding…"); !slices.Equal(got, []string{"/me/add/run?repo=octocat%2Fnew-rules"}) {
		t.Errorf("a library being added leads to %q", got)
	}
	private := site.get(t, "/me/add?url=octocat%2Fteam-rules")
	assertShows(t, private, "octocat/team-rules is private. Private libraries can't be published on Rulemart.")
}

// The page that follows a listing's check shows it running, failed with why and what to do, or done with what Rulemart
// found; a visitor who isn't adding the library is told so.
func TestTheRunPageFollowsTheListingsCheck(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
	site.listings.byAccount[1] = []views.AccountListing{
		{ID: 3, Owner: "octocat", Name: "new-rules", State: domain.ListingChecking},
		{ID: 4, Owner: "octocat", Name: "broken", State: domain.ListingFailed, Failure: "The repository has no release/<number> tags."},
		{ID: 5, Owner: "Example", Name: "Rules", State: domain.ListingListed, Library: views.LibraryRef{Owner: "example", Name: "rules"}},
	}

	running := site.get(t, "/me/add/run?repo=octocat%2Fnew-rules")
	assertShows(t, running, "Adding octocat/new-rules", "In progress: Looking for rule-library.yaml in octocat/new-rules",
		"Waiting: Watching for new library releases", "This page follows along.")
	if !strings.Contains(running, "data-polling") || !strings.Contains(running, `<noscript><meta http-equiv="refresh" content="2"></noscript>`) {
		t.Error("a running check's page doesn't follow it")
	}

	failed := site.get(t, "/me/add/run?repo=octocat%2Fbroken")
	assertShows(t, failed, "Failed: Looking for rule-library.yaml in octocat/broken The repository has no release/<number> tags.",
		"Rulemart couldn't add octocat/broken.", "Try again", "Remove", "Back to Dashboard")
	retry := "/me/listings/retry?listing=4&return=%2Fme%2Fadd%2Frun%3Frepo%3Doctocat%252Fbroken"
	if strings.Contains(failed, "data-polling") || !slices.Contains(formActions(t, failed), retry) {
		t.Errorf("a failed check's page follows it, or offers no Try again that returns to it: %q", formActions(t, failed))
	}
	// Try again stays on the page, which then follows the new check.
	again := send(t, site.handler, request{method: http.MethodPost, target: retry, cookies: []*http.Cookie{site.session}})
	if again.StatusCode != http.StatusSeeOther || again.Header.Get("Location") != "/me/add/run?repo=octocat%2Fbroken" {
		t.Errorf("Try again answered %d to %q, want the run page", again.StatusCode, again.Header.Get("Location"))
	}

	done := site.get(t, "/me/add/run?repo=example%2Frules")
	assertShows(t, done, "Done: Found rule-library.yaml in example/rules",
		"Done: Read library release release/3 · 2 rules at their published versions", "example/rules is live on Rulemart.")

	for _, target := range []string{"/me/add/run?repo=someone%2Felse", "/me/add/run"} {
		resp := send(t, site.handler, request{method: http.MethodGet, target: target, cookies: []*http.Cookie{site.session}})
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s answered %d, want 404", target, resp.StatusCode)
		}
	}
}

// The private projects page shows the prototype's permissions and promises, then Continue to GitHub, which leads to
// the app's install page; once installed, Done, and Remove access, which forgets the installation and returns to the
// dashboard saying so, in a status toast, as the prototype's disconnect does.
func TestThePrivateProjectsPageOffersTheAppOrItsRemoval(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())

	page := site.get(t, "/me/private")
	assertShows(t, page, "Optional Include your private projects?", "Contents: read-only", "Metadata: read-only",
		"We never", "Worth knowing", "Continue to GitHub", "Skip, public repos only", "You can change this anytime.")
	if got := links(t, page, "Continue to GitHub"); !slices.Equal(got, []string{"https://github.com/apps/rulemart-by-fabrica/installations/new"}) {
		t.Errorf("Continue to GitHub leads to %q", got)
	}

	site.gitHub.installations[1] = []accounts.Installation{{ID: 9, Account: "octocat"}}
	page = site.get(t, "/me/private")
	assertShows(t, page, "Done Remove access to private repos", "in its settings on GitHub: octocat")
	// The sentence's period follows the last installation's link, with no space before it.
	if !strings.Contains(page, ">octocat</a>.") {
		t.Error("the installations' list doesn't end with a period right after the last link")
	}
	if got := links(t, page, "octocat"); !slices.Contains(got, "https://github.com/settings/installations/9") {
		t.Errorf("the installation's settings are at %q", got)
	}
	resp := send(t, site.handler, request{method: http.MethodPost, target: "/me/github/remove", cookies: []*http.Cookie{site.session}})
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/me" || len(site.gitHub.installations[1]) != 0 {
		t.Fatalf("removing answered %d to %q, leaving %v", resp.StatusCode, resp.Header.Get("Location"), site.gitHub.installations[1])
	}
	dashboard := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/me", cookies: []*http.Cookie{site.session, cookie(resp, noticeCookie)}}))
	if got, kind := toastText(t, dashboard), noticeToast(t, dashboard); got != "Private repo access removed" || kind != "status" {
		t.Errorf("the dashboard toasts %q as a %q toast, want a status toast", got, kind)
	}
}

// Checkout's Where it goes offers a signed-in visitor their projects, each saying what it uses, private ones marked,
// and a way to use a project that doesn't use Code Rules yet; and the checkout writes for the project chosen.
func TestCheckoutOffersTheVisitorsProjects(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())

	// The script shows the page's cards, so its markup holds what the visitor sees.
	page := strings.Join(strings.Fields(site.get(t, "/cart")), " ")
	for _, want := range []string{
		`value="octocat/api" data-cart-project`, "octocat/api</b>", "Uses example/rules", `value="octocat/billing" data-cart-project`,
		">Private</span>", "Or use a project that doesn", "data-cart-new-box hidden", "A new project",
		"The prompt sets up Code Rules there first.",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	if strings.Contains(page, "Pick from your projects") {
		t.Error("a signed-in visitor with projects is asked to sign in")
	}
}

// A signed-in visitor's checkout is for one of their projects, the first unless they chose another, whose sources the
// texts add to; or for a new project, which the repository field names; a forged project name falls back to the first.
func TestCheckoutWritesForTheVisitorsProject(t *testing.T) {
	gitHub := newFakeGitHubAccounts(octocatsGitHub())
	carts := &fakeCarts{}
	site := newAccountsSite(t, func(o *web.Options) { o.GitHubAccounts, o.Carts = gitHub, carts })
	session := &http.Cookie{Name: sessionCookie, Value: string(site.accounts.signedIn(t, octocat))}
	checkout := func(project, repo string) domain.CheckoutTarget {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/cart/checkout.json",
			strings.NewReader(fmt.Sprintf(`{"cart":["example/rules::techs/go/return-errors"],"project":%q,"repo":%q}`, project, repo)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.AddCookie(session)
		recorder := httptest.NewRecorder()
		site.handler.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("checkout answered %d: %s", recorder.Code, recorder.Body)
		}
		return carts.target
	}
	for name, tc := range map[string]struct {
		project, repo string
		want          domain.CheckoutTarget
	}{
		"the first":    {"", "", domain.CheckoutTarget{Mode: domain.ProjectKnown, Repository: "octocat/api", Sources: map[string]string{"example/rules": "example"}}},
		"another":      {"OCTOCAT/billing", "", domain.CheckoutTarget{Mode: domain.ProjectKnown, Repository: "octocat/billing", Sources: map[string]string{"example/rules": "ex"}}},
		"a forged one": {"hubot/secret", "", domain.CheckoutTarget{Mode: domain.ProjectKnown, Repository: "octocat/api", Sources: map[string]string{"example/rules": "example"}}},
		"a new one":    {"new", "https://github.com/octocat/fresh", domain.CheckoutTarget{Mode: domain.ProjectNew, Repository: "octocat/fresh"}},
	} {
		if got := checkout(tc.project, tc.repo); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: the checkout is for %+v, want %+v", name, got, tc.want)
		}
	}
}
