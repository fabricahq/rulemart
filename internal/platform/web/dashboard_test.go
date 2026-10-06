package web_test

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	"golang.org/x/net/html"
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
// on Rulemart, a private one, a public one of octo-org's that octocat may only read, and two projects, one private, that
// import example/rules, one behind it.
func octocatsGitHub() accounts.Snapshot {
	behind := []accounts.PinnedRule{{Path: "techs/go/return-errors", Version: coderules.RuleVersion{Major: 1}}}
	current := []accounts.PinnedRule{{Path: "techs/go/return-errors", Version: coderules.RuleVersion{Major: 2}}}
	return accounts.Snapshot{
		ReadAt:        time.Now().Add(-3 * time.Minute),
		Organizations: []string{"octo-org", "example"},
		Libraries: []accounts.PublishableRepository{
			{Repository: accounts.Repository{Owner: "octocat", Name: "new-rules"}, Release: 2, Writable: true},
			{Repository: accounts.Repository{Owner: "example", Name: "rules"}, Release: 3, Writable: true},
			{Repository: accounts.Repository{Owner: "octocat", Name: "team-rules", Private: true}, Release: 1},
			{Repository: accounts.Repository{Owner: "octo-org", Name: "readonly"}, Release: 1},
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
	return newDashboardSiteWith(t, snapshot, c, nil)
}

// newDashboardSiteWith is newDashboardSite with its options adjusted by adjust, if not nil.
func newDashboardSiteWith(t *testing.T, snapshot accounts.Snapshot, c catalog, adjust func(*web.Options)) dashboardSite {
	t.Helper()
	site := dashboardSite{
		accounts: newFakeAccounts(), gitHub: newFakeGitHubAccounts(snapshot), logs: &bytes.Buffer{},
		listings: &fakeListings{byAccount: map[int64][]views.AccountListing{}},
	}
	options := web.Options{
		Log: slog.New(slog.NewJSONHandler(site.logs, nil)), Accounts: site.accounts, GitHub: &fakeGitHub{identity: octocat},
		Listings: site.listings, Stars: newFakeStars(), GitHubAccounts: site.gitHub,
	}
	if adjust != nil {
		adjust(&options)
	}
	handler, err := web.New(c, options)
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

// The dashboard shows the visitor and their organizations, then My libraries: the libraries they and their organizations
// publish, with their stars and New on one listed today, as the prototype's does.
func TestTheDashboardShowsTheVisitorsLibraries(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())

	page := site.get(t, "/me")

	assertShows(t, page,
		"Dashboard The Octocat @octocat · member of octo-org, example",
		"Libraries you and your organizations publish rules ★ 1,235 1,235 stars in all Vetted example/rules · 2 rules",
		"new ★ 0 0 stars in all New Unvetted octo-org/new · 1 rule", "+ Add a library",
		"Public repos only · read from GitHub 3 minutes ago · Refresh · Include private projects")
	if got := links(t, page, "Include private projects"); !slices.Equal(got, []string{"/me/private"}) {
		t.Errorf("Include private projects leads to %q", got)
	}
	if strings.Contains(page, "border-dashed") {
		t.Error("the dashboard still draws the dashed note on private projects")
	}
	if got := rels(t, page, "/octo-org/new"); !slices.Equal(got, []string{"nofollow"}) {
		t.Errorf("the unvetted library links with rel %q", got)
	}
	if content, _ := robots(t, page); content != "noindex" {
		t.Errorf("robots %q, want noindex", content)
	}
}

// My libraries and Projects show their lists directly under the tab bar, with no visible section heading repeating
// the tab: each list's heading is for screen readers only, and the tab bar holds the counts.
func TestTheDashboardsListsHaveNoVisibleSectionHeadings(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
	for target, want := range map[string]string{"/me": "Libraries you and your organizations publish", "/me?tab=projects": "Libraries your projects use"} {
		doc, err := html.Parse(strings.NewReader(site.get(t, target)))
		if err != nil {
			t.Fatal(err)
		}
		var visible, hidden []string
		for n := range doc.Descendants() {
			if n.Type != html.ElementNode || n.Data != "h2" {
				continue
			}
			if strings.Contains(" "+attribute(n, "class")+" ", " sr-only ") {
				hidden = append(hidden, nodeText(n))
			} else {
				visible = append(visible, nodeText(n))
			}
		}
		if len(visible) > 0 || !slices.Equal(hidden, []string{want}) {
			t.Errorf("%s has visible headings %q and screen-reader headings %q, want none and %q", target, visible, hidden, want)
		}
	}
}

// My libraries and Projects each show their rows as a list in one card, and My libraries' + Add a library sits in that
// card, after the list, so the list and its action read as one object; the line on the read of GitHub follows the card.
func TestTheDashboardsListsAreCards(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
	for target, last := range map[string]string{"/me": "+ Add a library", "/me?tab=projects": ""} {
		doc, err := html.Parse(strings.NewReader(site.get(t, target)))
		if err != nil {
			t.Fatal(err)
		}
		name := find(doc, func(n *html.Node) bool {
			return n.Data == "a" && attribute(n, "href") == "/example/rules" && n.Parent != nil && n.Parent.Data == "p"
		})
		if name == nil {
			t.Fatalf("%s lists no example/rules", target)
		}
		item := name
		for item != nil && item.Data != "li" {
			item = item.Parent
		}
		if item == nil || item.Parent.Data != "ul" {
			t.Fatalf("%s: example/rules isn't an item of a list", target)
		}
		card := item.Parent.Parent
		if !strings.Contains(attribute(card, "class"), "rounded-card") || !strings.Contains(attribute(card, "class"), "border") {
			t.Errorf("%s: the list isn't in a card: %q", target, attribute(card, "class"))
		}
		text := nodeText(card)
		if last != "" && !strings.HasSuffix(text, last) {
			t.Errorf("%s: the card ends %q, want %q", target, text[max(0, len(text)-40):], last)
		}
		if strings.Contains(text, "read from GitHub") {
			t.Errorf("%s: the line on the read of GitHub is inside the card", target)
		}
	}
}

// Each row of My libraries and Projects keeps its figure, the stars or the projects, on its title line beside the name,
// with nothing at the row's far edge.
func TestTheDashboardsRowsKeepTheirFiguresBesideTheirNames(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
	for target, want := range map[string]string{"/me": "rules ★ 1,235 1,235 stars in all Vetted", "/me?tab=projects": "rules · 2 projects"} {
		doc, err := html.Parse(strings.NewReader(site.get(t, target)))
		if err != nil {
			t.Fatal(err)
		}
		name := find(doc, func(n *html.Node) bool {
			return n.Data == "a" && attribute(n, "href") == "/example/rules" && n.Parent != nil && n.Parent.Data == "p"
		})
		if name == nil {
			t.Fatalf("%s lists no example/rules", target)
		}
		if got := nodeText(name.Parent); got != want {
			t.Errorf("%s: example/rules' title line reads %q, want %q", target, got, want)
		}
		if row := name.Parent.Parent.Parent; row.FirstChild != row.LastChild && strings.TrimSpace(nodeText(row.LastChild)) != "" && row.LastChild != name.Parent.Parent {
			t.Errorf("%s: example/rules' row holds something beside its text: %q", target, nodeText(row.LastChild))
		}
	}
}

// Projects shows the libraries the visitor's projects use, with the updates waiting for each project, as the
// prototype's My libraries does below the visitor's own libraries.
func TestTheDashboardsProjectsTabShowsTheLibrariesTheVisitorsProjectsUse(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())

	page := site.get(t, "/me?tab=projects")

	assertShows(t, page,
		"Libraries your projects use rules · 2 projects octocat/api · 1 rule update · octocat/billing (private) · up to date",
		"Public repos only · read from GitHub 3 minutes ago · Refresh · Include private projects",
		"Read from each project's .code-rules/generated/provenance.json .")
}

// My libraries and Projects lead with their lists, then what adds to them, and end with one line on the read of GitHub:
// whether it includes private projects, and how fresh it is.
func TestTheDashboardsListsComeBeforeTheirPrompts(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
	for target, order := range map[string][]string{
		"/me":              {"Libraries you and your organizations publish", "+ Add a library", "Public repos only · read from GitHub"},
		"/me?tab=projects": {"Libraries your projects use", "Public repos only · read from GitHub", "provenance.json"},
	} {
		text := visibleText(t, site.get(t, target))
		last := -1
		for _, part := range order {
			at := strings.Index(text, part)
			if at <= last {
				t.Errorf("%s shows %q out of the order %q:\n%s", target, part, order, text)
			}
			last = at
		}
	}
}

// Each of the dashboard's tabs, My libraries, Projects, Starred rules, and Account, shows its own content and none of
// the others', under the tab bar, which counts each tab's rows and marks the tab shown; My libraries and Projects,
// which both show what Rulemart read of GitHub, say how fresh the read is, and their Refresh returns to the same tab.
// A tab the page doesn't know shows My libraries.
func TestEachDashboardTabShowsOnlyItsOwnContent(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
	libraries := []string{"Libraries you and your organizations publish", "+ Add a library"}
	projects := []string{"Libraries your projects use", "provenance.json"}
	stars := []string{"You haven't starred any rules yet."}
	account := []string{"GitHub user ID", "Account created", "Sign out everywhere", "Delete my account"}
	gitHub := []string{"Public repos only", "read from GitHub"}
	for _, tc := range []struct {
		target, tab string
		shows       [][]string
		hides       [][]string
		// refresh is where the tab's Refresh posts, or empty when the tab has none.
		refresh string
	}{
		{"/me", "My libraries", [][]string{gitHub, libraries}, [][]string{projects, stars, account}, "/me/refresh?return=%2Fme"},
		{"/me?tab=projects", "Projects", [][]string{gitHub, projects}, [][]string{libraries, stars, account}, "/me/refresh?return=%2Fme%3Ftab%3Dprojects"},
		{"/me?tab=stars", "Starred rules", [][]string{stars}, [][]string{gitHub, libraries, projects, account}, ""},
		{"/me?tab=account", "Account", [][]string{account}, [][]string{gitHub, libraries, projects, stars}, ""},
		{"/me?tab=nonsense", "My libraries", [][]string{gitHub, libraries}, [][]string{projects, stars, account}, "/me/refresh?return=%2Fme"},
	} {
		page := site.get(t, tc.target)
		text := visibleText(t, page)

		assertShows(t, page, "My libraries , 2 Projects , 1 Starred rules , 0 Account")
		if got := currentTabs(t, page); !slices.Equal(got, []string{tc.tab}) {
			t.Errorf("%s marks %q as the current tab, want %q", tc.target, got, tc.tab)
		}
		for _, shown := range tc.shows {
			for _, want := range shown {
				if !strings.Contains(text, want) {
					t.Errorf("%s doesn't show %q", tc.target, want)
				}
			}
		}
		for _, hidden := range tc.hides {
			for _, unwanted := range hidden {
				if strings.Contains(text, unwanted) {
					t.Errorf("%s shows %q, which another tab holds", tc.target, unwanted)
				}
			}
		}
		refreshes := slices.DeleteFunc(formActions(t, page), func(action string) bool { return !strings.HasPrefix(action, "/me/refresh") })
		if want := []string{tc.refresh}; tc.refresh == "" && len(refreshes) > 0 || tc.refresh != "" && !slices.Equal(refreshes, want) {
			t.Errorf("%s's Refresh posts to %q, want %q", tc.target, refreshes, tc.refresh)
		}
	}
	// The tab is its own heading.
	if got := headings(t, site.get(t, "/me?tab=account"), "h2"); slices.Contains(got, "Account") {
		t.Errorf("the Account tab repeats its name as a heading: %q", got)
	}
}

// Where Rulemart can't read visitors' GitHub accounts, the dashboard has no Projects tab, and its address shows My
// libraries.
func TestTheDashboardHasNoProjectsTabWithoutGitHub(t *testing.T) {
	site := newDashboardSiteWith(t, accounts.Snapshot{}, octocatsCatalog(), func(o *web.Options) { o.GitHubAccounts = nil })

	page := site.get(t, "/me?tab=projects")

	assertShows(t, page, "My libraries , 0 Starred rules , 0 Account", "Libraries you and your organizations publish")
	if text := visibleText(t, page); strings.Contains(text, "Projects") || strings.Contains(text, "Libraries your projects use") {
		t.Errorf("without GitHub, the dashboard offers projects:\n%s", text)
	}
	if got := currentTabs(t, page); !slices.Equal(got, []string{"My libraries"}) {
		t.Errorf("marks %q as the current tab, want My libraries", got)
	}
}

// currentTabs returns the name of each tab a page's tab bar, marked data-tabs, marks current.
func currentTabs(t *testing.T, page string) []string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for n := range doc.Descendants() {
		if n.Type == html.ElementNode && n.Data == "a" && attribute(n, "aria-current") == "page" && n.Parent != nil && hasAttribute(n.Parent, "data-tabs") {
			name, _, _ := strings.Cut(nodeText(n), " ,")
			names = append(names, name)
		}
	}
	return names
}

// My libraries shows the visitor's own listings in place: a listing of a library they or their organizations publish
// joins its row, with Remove, a check under way or failed takes a row of its own, with Try again where it failed, and a
// listing of anyone else's library goes under Listed by you, when there is one. With two groups, each has a heading.
func TestMyLibrariesShowsTheVisitorsListingsInPlace(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
	site.listings.byAccount[1] = []views.AccountListing{
		{ID: 5, Owner: "octocat", Name: "new-rules", State: domain.ListingChecking, ListedAt: time.Now(), RequestedAt: time.Now().Add(-time.Minute)},
		{ID: 4, Owner: "Octocat", Name: "broken", State: domain.ListingFailed, Failure: "The repository has no release/<number> tags",
			ListedAt: day(3), RequestedAt: day(3)},
		{ID: 3, Owner: "stranger", Name: "rules", State: domain.ListingListed, Library: views.LibraryRef{Owner: "stranger", Name: "rules"},
			ListedAt: day(2), RequestedAt: day(2)},
		{ID: 1, Owner: "Example", Name: "Rules", State: domain.ListingVetted, Library: views.LibraryRef{Owner: "example", Name: "rules"},
			ListedAt: day(1), RequestedAt: day(1)},
	}

	page := site.get(t, "/me")

	assertShows(t, page, "My libraries , 5",
		"Published by you and your orgs · 4 rules ★ 1,235 1,235 stars in all Vetted example/rules · 2 rules · Remove "+
			"new ★ 0 0 stars in all New Unvetted octo-org/new · 1 rule "+
			"new-rules Checking octocat/new-rules · Rulemart is checking it on GitHub. You asked 1 minute ago. · Remove "+
			"broken Failed Octocat/broken · The repository has no release/<number> tags · Try again · Remove",
		"+ Add a library Listed by you · 1 rules Unvetted stranger/rules · Listed 2 Sep 2026 · Remove")
	if got := links(t, page, "Remove"); !slices.Equal(got, []string{
		"/me/listings/remove?listing=1", "/me/listings/remove?listing=5", "/me/listings/remove?listing=4", "/me/listings/remove?listing=3",
	}) {
		t.Errorf("the Remove links lead to %q", got)
	}
	// Try again and Remove follow the status they act on, on the row's second line, with nothing at the row's edge.
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	for n := range doc.Descendants() {
		if n.Type != html.ElementNode || !(n.Data == "a" && nodeText(n) == "Remove" || n.Data == "button" && nodeText(n) == "Try again") {
			continue
		}
		line := n.Parent
		for line != nil && (line.Data == "form" || line.Data == "span") {
			line = line.Parent
		}
		if line == nil || !strings.Contains(nodeText(line), " · ") || line.Parent == nil || line.Parent.Parent == nil || line.Parent.Parent.LastChild != line.Parent && strings.TrimSpace(nodeText(line.Parent.Parent.LastChild)) != "" {
			t.Errorf("%s isn't on its row's second line, after the status", nodeText(n))
		}
	}
	if got := headings(t, page, "h2"); !slices.Equal(got, []string{"Published by you and your orgs · 4", "Listed by you · 1"}) {
		t.Errorf("the tab's headings are %q", got)
	}
}

// New marks a library that came to Rulemart within the last day, and only such a library.
func TestTheDashboardMarksALibraryNewOnlyWithinADayOfItsListing(t *testing.T) {
	c := newCatalog()
	c.dashboard = views.Dashboard{Owned: []views.OwnedLibrary{
		{Library: views.LibraryRef{Owner: "octo-org", Name: "fresh"}, Rules: 1, AddedAt: time.Now().Add(-23 * time.Hour)},
		{Library: views.LibraryRef{Owner: "octo-org", Name: "stale"}, Rules: 1, AddedAt: time.Now().Add(-25 * time.Hour)},
	}}
	site := newDashboardSite(t, octocatsGitHub(), c)

	page := site.get(t, "/me")

	assertShows(t, page, "fresh ★ 0 0 stars in all New Unvetted octo-org/fresh", "stale ★ 0 0 stars in all Unvetted octo-org/stale")
}

// A visitor in no organization, with no projects and nothing published, sees each tab say so.
func TestTheDashboardSaysWhenTheVisitorHasNothingYet(t *testing.T) {
	site := newDashboardSite(t, accounts.Snapshot{ReadAt: time.Now()}, newCatalog())

	page := site.get(t, "/me")

	assertShows(t, page, "The Octocat @octocat My libraries , 0 Projects , 0",
		"Libraries you and your organizations publish No library of yours or your organizations' is on Rulemart yet.")
	if strings.Contains(visibleText(t, page), "member of") {
		t.Error("the head names organizations the visitor isn't in")
	}
	assertShows(t, site.get(t, "/me?tab=projects"), "Libraries your projects use None of your projects imports a library that's on Rulemart.")
}

// Once the visitor installed the GitHub App, the line on the read says private projects are included, and leads to
// managing them.
func TestTheDashboardSaysWhenItIncludesPrivateProjects(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
	site.gitHub.installations[1] = []accounts.Installation{{ID: 9, Account: "octocat"}}

	page := site.get(t, "/me")

	assertShows(t, page, "Including private projects from the repos you selected · read from GitHub 3 minutes ago · Refresh · Manage")
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
	assertShows(t, site.get(t, "/me?tab=projects"), "Rulemart couldn't read your repositories on GitHub just now. Showing what it read 3 minutes ago. Try again",
		"octocat/api · 1 rule update")
	if !strings.Contains(site.logs.String(), "GitHub answered 502") {
		t.Errorf("didn't log the failed read: %s", site.logs)
	}

	tokenless := newDashboardSite(t, accounts.Snapshot{}, octocatsCatalog())
	tokenless.gitHub.err = accountsapp.ErrNoGitHubToken
	page := tokenless.get(t, "/me?tab=projects")
	assertShows(t, page, "Sign in again so Rulemart can read your repositories on GitHub. Sign in again", "Rulemart hasn't read your projects yet.")
	if got := links(t, page, "Sign in again"); !slices.Equal(got, []string{"/signin?again=1&return=%2Fme%3Ftab%3Dprojects"}) {
		t.Errorf("Sign in again leads to %q", got)
	}
	again := body(t, send(t, tokenless.handler, request{method: http.MethodGet, target: "/signin?again=1&return=%2Fme", cookies: []*http.Cookie{tokenless.session}}))
	assertShows(t, again, "Sign in again so Rulemart can read your repositories on GitHub.", "Continue with GitHub")

	truncated := octocatsGitHub()
	truncated.Truncated = true
	assertShows(t, newDashboardSite(t, truncated, octocatsCatalog()).get(t, "/me"),
		"Some of your repositories are left out: Rulemart reads the 200 most recently pushed of yours and your first 100 organizations'.")

	broken := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
	broken.gitHub.err = errors.New("connection refused")
	broken.gitHub.kept = map[int64]accounts.Snapshot{}
	if resp := send(t, broken.handler, request{method: http.MethodGet, target: "/me", cookies: []*http.Cookie{broken.session}}); resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("a failure to read the snapshot answered %d, want 503", resp.StatusCode)
	}
}

// While another request reads the visitor's GitHub account for the first time, a page that shows it says Rulemart is
// reading it, rather than that it has nothing, and refreshes itself shortly; once the read is kept, it doesn't.
func TestTheDashboardSaysItsReadingGitHubAndRefreshesWhileAFirstReadIsUnderWay(t *testing.T) {
	site := newDashboardSite(t, accounts.Snapshot{}, octocatsCatalog())
	site.gitHub.err = accountsapp.ErrGitHubReading

	resp := send(t, site.handler, request{method: http.MethodGet, target: "/me?tab=projects", cookies: []*http.Cookie{site.session}})
	page := body(t, resp)

	if resp.StatusCode != http.StatusOK || resp.Header.Get("Refresh") != "3" {
		t.Errorf("answered %d with Refresh %q, want 200 with Refresh 3", resp.StatusCode, resp.Header.Get("Refresh"))
	}
	assertShows(t, page, "Rulemart is reading your repositories on GitHub. This page will update in a moment.", "Rulemart is reading your projects.")
	if strings.Contains(page, "Rulemart hasn't read your projects yet.") {
		t.Error("the page says Rulemart hasn't read the projects, while it's reading them")
	}

	read := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
	if resp := send(t, read.handler, request{method: http.MethodGet, target: "/me", cookies: []*http.Cookie{read.session}}); resp.Header.Get("Refresh") != "" {
		t.Errorf("a page with a kept read refreshes itself, Refresh %q", resp.Header.Get("Refresh"))
	}
}

// Refresh reads GitHub again and returns to the page it came from, one of the visitor's, saying when it read; a session
// without a token GitHub takes is sent to sign in again.
func TestRefreshReadsGitHubAgainAndReturns(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
	site.get(t, "/me")

	for target, want := range map[string]string{
		"/me/refresh": "/me",
		"/me/refresh?return=%2Fme%3Ftab%3Dprojects": "/me?tab=projects",
		"/me/refresh?return=%2Fme%2Fadd":            "/me/add",
		"/me/refresh?return=%2Fcart":                "/cart",
		"/me/refresh?return=%2Ffaq":                 "/me",
	} {
		resp := send(t, site.handler, request{method: http.MethodPost, target: target, cookies: []*http.Cookie{site.session}})
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != want {
			t.Errorf("%s answered %d to %q, want %q", target, resp.StatusCode, resp.Header.Get("Location"), want)
		}
	}
	if site.gitHub.reads != 6 {
		t.Errorf("read GitHub %d times, want once for the page and once a refresh", site.gitHub.reads)
	}
	// A refresh says when Rulemart last read GitHub, which it does at most once a minute, so a second press within the
	// minute isn't met with nothing.
	site.gitHub.snapshot.ReadAt = time.Now().Add(-20 * time.Second)
	refreshed := send(t, site.handler, request{method: http.MethodPost, target: "/me/refresh", cookies: []*http.Cookie{site.session}})
	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/me", cookies: []*http.Cookie{site.session, cookie(refreshed, noticeCookie)}}))
	if got, kind := toastText(t, page), noticeToast(t, page); got != "Read from GitHub less than a minute ago" || kind != "status" {
		t.Errorf("after Refresh, the dashboard toasts %q as a %q toast, want a status toast", got, kind)
	}
	site.gitHub.snapshot.ReadFailed = true
	if failed := send(t, site.handler, request{method: http.MethodPost, target: "/me/refresh", cookies: []*http.Cookie{site.session}}); cookie(failed, noticeCookie) != nil {
		t.Error("a refresh that couldn't read GitHub leaves a notice, beside the page's own failure")
	}
	site.gitHub.err = accountsapp.ErrNoGitHubToken
	resp := send(t, site.handler, request{method: http.MethodPost, target: "/me/refresh", cookies: []*http.Cookie{site.session}})
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/signin?again=1&return=%2Fme" {
		t.Errorf("without a token, Refresh answered %d to %q", resp.StatusCode, resp.Header.Get("Location"))
	}
}

// The account menu names the visitor, then has three entries: the dashboard, adding a library, and Sign out. The
// dashboard's tabs, Starred rules and Account, are only on the dashboard.
func TestTheAccountMenuReadsAsThePrototypes(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
	page := site.get(t, "/faq")
	assertShows(t, page, "The Octocat Signed in as @octocat Dashboard Add a library Sign out")
	for name, want := range map[string][]string{"Dashboard": {"/me"}, "Add a library": {"/me/add"}, "Starred rules": nil, "Account": nil} {
		if got := links(t, page, name); !slices.Equal(got, want) {
			t.Errorf("%s leads to %q, want %q", name, got, want)
		}
	}
	if got := formActions(t, page); !slices.Equal(got, []string{"/signout?return=%2Ffaq"}) {
		t.Errorf("the page's forms post to %q, want only Sign out", got)
	}
}

// The old addresses of the account's pages lead to their places on the dashboard, and the sign-in page's to its new one.
func TestTheAccountsOldAddressesRedirect(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
	for target, want := range map[string]string{
		"/account":               "/me?tab=account",
		"/account/stars":         "/me?tab=stars",
		"/account/listings":      "/me",
		"/list":                  "/me/add",
		"/sign-in?return=%2Ffaq": "/signin?return=%2Ffaq",
	} {
		resp := send(t, site.handler, request{method: http.MethodGet, target: target})
		if resp.StatusCode != http.StatusMovedPermanently || resp.Header.Get("Location") != want {
			t.Errorf("%s answered %d to %q, want %q", target, resp.StatusCode, resp.Header.Get("Location"), want)
		}
	}
}

// The page says what adding does and what a library is, then lists the repositories that publish a library in a card of rows
// in their four kinds: one to add, one on Rulemart, by way of the visitor's organization, a public one the visitor may only
// read, and a private one, both dimmed, that can't be added; a library being added leads to its check.
func TestTheAddPageListsTheVisitorsLibrariesByWhatAddingDoes(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())

	page := site.get(t, "/me/add")

	assertShows(t, page, "Publish Add a library Add a Code Rules library from a GitHub repository you have write access to, "+
		"from the list below or by its URL. A library is a repository with a rule-library.yaml and at least one "+
		"release/<number> tag.",
		"octocat/new-rules on GitHub Public · release/2 Add this library",
		"example/rules on GitHub Public · release/3 · via the example organization ✓ On Rulemart",
		"octocat/team-rules on GitHub Private · release/1 Private libraries can't be published on Rulemart",
		"octo-org/readonly on GitHub Public · release/1 · via the octo-org organization Only someone with write access can add it",
		"octo-org/new on GitHub Public · via the octo-org organization ✓ On Rulemart")
	text := visibleText(t, page)
	for _, gone := range []string{"Only public libraries can be published", "Or add any public library by URL", "Read from GitHub"} {
		if strings.Contains(text, gone) {
			t.Errorf("the page still says %q", gone)
		}
	}
	if strings.Contains(page, "border-dashed") {
		t.Error("the page still draws the dashed note on private repos")
	}
	if got := formActions(t, page); !slices.Equal(got, []string{"/signout", "/me/add?repository=octocat%2Fnew-rules", "/me/refresh?return=%2Fme%2Fadd"}) {
		t.Errorf("the page's forms post to %q", got)
	}
	site.listings.byAccount[1] = []views.AccountListing{{ID: 3, Owner: "octocat", Name: "new-rules", State: domain.ListingChecking}}
	if got := links(t, site.get(t, "/me/add"), "Adding…"); !slices.Equal(got, []string{"/me/add/run?repo=octocat%2Fnew-rules"}) {
		t.Errorf("a library being added leads to %q", got)
	}
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	for name, dimmed := range map[string]bool{"octocat/new-rules": false, "octo-org/readonly": true, "octocat/team-rules": true, "example/rules": false} {
		row := find(doc, func(n *html.Node) bool { return n.Data == "li" && strings.Contains(nodeText(n), name+" on GitHub") })
		if row == nil {
			t.Fatalf("no row for %s", name)
		}
		nameLine := find(row, func(n *html.Node) bool {
			return n.Data == "p" && strings.Contains(attribute(n, "class"), "overflow-wrap")
		})
		if got := strings.Contains(attribute(nameLine, "class"), "text-faint"); got != dimmed {
			t.Errorf("%s's name is dimmed: %v, want %v", name, got, dimmed)
		}
	}
	private := site.get(t, "/me/add?url=octocat%2Fteam-rules")
	assertShows(t, private, "octocat/team-rules is private. Private libraries can't be published on Rulemart.")
	if got := formActions(t, private); slices.Contains(got, "/me/add?repository=octocat%2Fteam-rules") || strings.Contains(private, "Public library on GitHub") {
		t.Errorf("the refusal of a private library offers to add it, with forms posting to %q", got)
	}
}

// A repository's maintainers may add it, so the form that checks an address asks GitHub whether the visitor's token may
// push to the repository it names: one it may is offered to add, one it may only read is refused with why, and offers
// nothing to add, and an address that isn't a repository's is told so without asking GitHub.
func TestTheAddPageOffersARepositoryOnlyToSomeoneWhoMayPushToIt(t *testing.T) {
	const refusal = "Only someone with write access to octo-org/readonly on GitHub can add it to Rulemart."
	addForm := "/me/add?repository=octo-org%2Freadonly"

	t.Run("a repository the token may push to", func(t *testing.T) {
		site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
		page := site.get(t, "/me/add?url=https%3A%2F%2Fgithub.com%2Fsomeone%2Fnew")
		assertShows(t, page, "someone/new on GitHub Public library on GitHub Add this library")
		if !slices.Contains(formActions(t, page), "/me/add?repository=someone%2Fnew") || strings.Contains(visibleText(t, page), "write access to someone/new") {
			t.Errorf("the page doesn't offer to add it: %q", formActions(t, page))
		}
		if !slices.Equal(site.gitHub.maintained, []string{"someone/new"}) {
			t.Errorf("asked GitHub about %q, want someone/new", site.gitHub.maintained)
		}
	})
	t.Run("a repository the token may only read", func(t *testing.T) {
		site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
		page := site.get(t, "/me/add?url=octo-org%2Freadonly")
		assertShows(t, page, refusal)
		if slices.Contains(formActions(t, page), addForm) || strings.Contains(page, "Public library on GitHub") || strings.Contains(page, "The library to add") {
			t.Errorf("the refusal offers to add it, with forms posting to %q", formActions(t, page))
		}
		if !strings.Contains(page, `aria-invalid="true"`) {
			t.Error("the address field doesn't say the address is refused")
		}
	})
	t.Run("an address that isn't a repository's", func(t *testing.T) {
		site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
		assertShows(t, site.get(t, "/me/add?url=not-a-repository"), "Enter a GitHub repository URL, like https://github.com/owner/repo.")
		if len(site.gitHub.maintained) != 0 {
			t.Errorf("asked GitHub about %q for an address that names no repository", site.gitHub.maintained)
		}
	})
	t.Run("GitHub can't be read", func(t *testing.T) {
		site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
		site.gitHub.maintainsErr = errors.New("check write access: GitHub answered 502")
		page := site.get(t, "/me/add?url=octo-org%2Freadonly")
		assertShows(t, page, "Rulemart couldn't read octo-org/readonly from GitHub just now. Try again.")
		if slices.Contains(formActions(t, page), addForm) || strings.Contains(page, "The library to add") {
			t.Errorf("the page offers to add a repository whose access is unknown: %q", formActions(t, page))
		}
		if !strings.Contains(site.logs.String(), "check of write access failed") || !strings.Contains(site.logs.String(), "GitHub answered 502") {
			t.Errorf("didn't log the failed check: %s", site.logs)
		}
	})
	t.Run("the session keeps no token GitHub takes", func(t *testing.T) {
		site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
		site.gitHub.maintainsErr = fmt.Errorf("read the session's token: %w", accountsapp.ErrNoGitHubToken)
		target := "/me/add?url=octo-org%2Freadonly"
		resp := send(t, site.handler, request{method: http.MethodGet, target: target, cookies: []*http.Cookie{site.session}})
		want := "/signin?" + url.Values{"again": {"1"}, "return": {target}}.Encode()
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != want {
			t.Errorf("answered %d to %q, want a redirect to %q", resp.StatusCode, resp.Header.Get("Location"), want)
		}
	})
}

// Adding is refused for a repository the visitor may not push to, whatever the picker offered: asking GitHub comes
// before the listing, so nothing is listed, and a refusal says so with a 403, whatever the repository's spelling.
// GitHub failing to say refuses with a 502 and the same, and a token it refuses asks the visitor to sign in again.
func TestAddingARepositoryTheVisitorMayNotPushToIsRefusedBeforeItIsListed(t *testing.T) {
	post := func(site dashboardSite, repository string) *http.Response {
		return send(t, site.handler, request{method: http.MethodPost, target: "/me/add?repository=" + url.QueryEscape(repository), cookies: []*http.Cookie{site.session}})
	}
	t.Run("a repository the token may only read", func(t *testing.T) {
		site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
		// The refusal names the repository as the visitor spelled it.
		for spelling, name := range map[string]string{
			"octo-org/readonly": "octo-org/readonly", "Octo-Org/ReadOnly": "Octo-Org/ReadOnly", "https://github.com/octo-org/readonly.git": "octo-org/readonly",
		} {
			resp := post(site, spelling)
			if resp.StatusCode != http.StatusForbidden || resp.Header.Get("Cache-Control") != "private, no-store" {
				t.Fatalf("adding %s answered %d, cached as %q, want 403", spelling, resp.StatusCode, resp.Header.Get("Cache-Control"))
			}
			page := body(t, resp)
			assertShows(t, page, "Only someone with write access to "+name+" on GitHub can add it to Rulemart.")
			if slices.Contains(formActions(t, page), "/me/add?repository=octo-org%2Freadonly") {
				t.Errorf("the refusal offers to add it: %q", formActions(t, page))
			}
		}
		if len(site.listings.listed) != 0 {
			t.Errorf("listed %q for a repository the visitor may not push to", site.listings.listed)
		}
	})
	t.Run("a repository the token may push to", func(t *testing.T) {
		site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
		resp := post(site, "octocat/new-rules")
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/me/add/run?repo=octocat%2Fnew-rules" {
			t.Fatalf("answered %d to %q", resp.StatusCode, resp.Header.Get("Location"))
		}
		if !slices.Equal(site.listings.listed, []string{"1 octocat/new-rules"}) || !slices.Equal(site.gitHub.maintained, []string{"octocat/new-rules"}) {
			t.Errorf("listed %q after asking GitHub about %q", site.listings.listed, site.gitHub.maintained)
		}
	})
	t.Run("GitHub can't be read", func(t *testing.T) {
		site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
		site.gitHub.maintainsErr = errors.New("check write access: GitHub answered 502")
		resp := post(site, "octocat/new-rules")
		if resp.StatusCode != http.StatusBadGateway {
			t.Fatalf("answered %d, want 502", resp.StatusCode)
		}
		assertShows(t, body(t, resp), "Rulemart couldn't read octocat/new-rules from GitHub just now. Try again.")
		if len(site.listings.listed) != 0 {
			t.Errorf("listed %q for a repository whose access is unknown", site.listings.listed)
		}
	})
	t.Run("the session keeps no token GitHub takes", func(t *testing.T) {
		site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
		site.gitHub.maintainsErr = fmt.Errorf("read the session's token: %w", accountsapp.ErrNoGitHubToken)
		resp := post(site, "octocat/new-rules")
		want := "/signin?" + url.Values{"again": {"1"}, "return": {"/me/add?url=octocat%2Fnew-rules"}}.Encode()
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != want || len(site.listings.listed) != 0 {
			t.Errorf("answered %d to %q after listing %q, want a redirect to %q", resp.StatusCode, resp.Header.Get("Location"), site.listings.listed, want)
		}
	})
	t.Run("an address that isn't a repository's", func(t *testing.T) {
		site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
		resp := post(site, "not-a-repository")
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("answered %d, want 409", resp.StatusCode)
		}
		assertShows(t, body(t, resp), "Enter a GitHub repository URL, like https://github.com/owner/repo.")
		if len(site.gitHub.maintained) != 0 || len(site.listings.listed) != 0 {
			t.Errorf("asked GitHub about %q and listed %q", site.gitHub.maintained, site.listings.listed)
		}
	})
}

// Each repository's name on the picker leads to it on GitHub, in the same tab as the site's other links to GitHub, and
// is followed by a small arrow, hidden from assistive technology, that says the link leaves the site; the link's
// name says where it leads.
func TestTheAddPagesRowsLinkToTheirRepositoriesOnGitHub(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())

	page := site.get(t, "/me/add")

	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for n := range doc.Descendants() {
		if n.Type != html.ElementNode || n.Data != "a" || !strings.HasPrefix(attribute(n, "href"), "https://github.com/") ||
			!strings.HasSuffix(nodeText(n), " on GitHub") {
			continue
		}
		got[nodeText(n)] = attribute(n, "href")
		if hasAttribute(n, "target") {
			t.Errorf("%q opens another tab", nodeText(n))
		}
		arrows := 0
		for child := range n.Descendants() {
			if child.Type == html.ElementNode && child.Data == "svg" && attribute(child, "aria-hidden") == "true" {
				arrows++
			}
		}
		if arrows != 1 {
			t.Errorf("%q has %d hidden arrows, want 1", nodeText(n), arrows)
		}
	}
	want := map[string]string{
		"octocat/new-rules on GitHub":  "https://github.com/octocat/new-rules",
		"example/rules on GitHub":      "https://github.com/example/rules",
		"octocat/team-rules on GitHub": "https://github.com/octocat/team-rules",
		"octo-org/readonly on GitHub":  "https://github.com/octo-org/readonly",
		"octo-org/new on GitHub":       "https://github.com/octo-org/new",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the rows link to %v, want %v", got, want)
	}
}

// Under its card the page says what Rulemart read of GitHub in one line, as the dashboard does, with Refresh back to the
// page and the way to include private repos, or, once the visitor selected some, to manage them; while Rulemart can't
// show a read, the line says why instead, and the card says it has nothing to list.
func TestTheAddPageEndsItsListWithOneLineOnWhatRulemartRead(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())

	page := site.get(t, "/me/add")

	assertShows(t, page, "Public repos only · read from GitHub 3 minutes ago · Refresh · Include private repos")
	if got := links(t, page, "Include private repos"); !slices.Equal(got, []string{"/me/private"}) {
		t.Errorf("Include private repos leads to %q", got)
	}
	if got := strings.Count(page, "data-github-status"); got != 1 {
		t.Errorf("the page has %d lines on the read of GitHub, want 1", got)
	}

	site.gitHub.installations[1] = []accounts.Installation{{ID: 9, Account: "octocat"}}
	private := site.get(t, "/me/add")
	assertShows(t, private, "Including private repos from the ones you selected · read from GitHub 3 minutes ago · Refresh · Manage")
	if got := links(t, private, "Manage"); !slices.Equal(got, []string{"/me/private"}) {
		t.Errorf("Manage leads to %q", got)
	}

	failed := octocatsGitHub()
	failed.ReadFailed = true
	broken := newDashboardSite(t, failed, octocatsCatalog())
	broken.gitHub.err = fmt.Errorf("read GitHub: %w: GitHub answered 502", accountsapp.ErrGitHubRead)
	broken.gitHub.snapshot = failed
	assertShows(t, broken.get(t, "/me/add"), "octocat/new-rules on GitHub",
		"Rulemart couldn't read your repositories on GitHub just now. Showing what it read 3 minutes ago. Try again")

	tokenless := newDashboardSite(t, accounts.Snapshot{}, octocatsCatalog())
	tokenless.gitHub.err = accountsapp.ErrNoGitHubToken
	signIn := tokenless.get(t, "/me/add")
	assertShows(t, signIn, "Sign in again so Rulemart can read your repositories on GitHub. Sign in again", "Rulemart hasn't read your repositories yet.")
	if got := links(t, signIn, "Sign in again"); !slices.Equal(got, []string{"/signin?again=1&return=%2Fme%2Fadd"}) {
		t.Errorf("Sign in again leads to %q", got)
	}

	reading := newDashboardSite(t, accounts.Snapshot{}, octocatsCatalog())
	reading.gitHub.err = accountsapp.ErrGitHubReading
	assertShows(t, reading.get(t, "/me/add"), "Rulemart is reading your repositories.",
		"Rulemart is reading your repositories on GitHub. This page will update in a moment.")
}

// The page that follows a listing's check shows it running, failed with why and what to do, or done with what Rulemart
// found; a visitor who isn't adding the library is told so.
func TestTheRunPageFollowsTheListingsCheck(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())
	site.listings.byAccount[1] = []views.AccountListing{
		{ID: 3, Owner: "octocat", Name: "new-rules", State: domain.ListingChecking, RequestedAt: time.Now().Add(-10 * time.Second)},
		{ID: 6, Owner: "octocat", Name: "slow", State: domain.ListingChecking, RequestedAt: time.Now().Add(-5 * time.Minute)},
		{ID: 4, Owner: "octocat", Name: "broken", State: domain.ListingFailed, Failure: "The repository has no release/<number> tags."},
		{ID: 5, Owner: "Example", Name: "Rules", State: domain.ListingListed, Library: views.LibraryRef{Owner: "example", Name: "rules"}},
	}

	running := site.get(t, "/me/add/run?repo=octocat%2Fnew-rules")
	assertShows(t, running, "Adding octocat/new-rules", "In progress: Looking for rule-library.yaml in octocat/new-rules",
		"Waiting: Watching for new library releases", "This page follows along.")
	if !strings.Contains(running, "data-polling") || !strings.Contains(running, `<noscript><meta http-equiv="refresh" content="2"></noscript>`) {
		t.Error("a running check's page doesn't follow it")
	}
	if !strings.Contains(running, "<div hidden data-poll-stopped") {
		t.Error("a running check's page has no hidden place for poll.js to say why it stopped following")
	}

	// Past the time a queued check takes, the page says what the listings page says, and stops following the check,
	// which the worker's hourly poll picks up; Refresh status reloads it.
	slow := site.get(t, "/me/add/run?repo=octocat%2Fslow")
	assertShows(t, slow, "In progress: Looking for rule-library.yaml in octocat/slow",
		"This is taking longer than usual Rulemart checks it again within the hour. You asked 5 minutes ago. Refresh status")
	if strings.Contains(slow, "data-polling") || strings.Contains(slow, `http-equiv="refresh"`) {
		t.Error("a slow check's page keeps following it every two seconds")
	}
	if got := links(t, slow, "Refresh status"); !slices.Equal(got, []string{"/me/add/run?repo=octocat%2Fslow"}) {
		t.Errorf("Refresh status leads to %q", got)
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
	// What the check found is in stronger type, which leaves no space before the punctuation that follows it.
	doc, err := html.Parse(strings.NewReader(done))
	if err != nil {
		t.Fatal(err)
	}
	indexed := find(doc, func(n *html.Node) bool { return n.Data == "li" && strings.Contains(visibleTextOf(n), "indexed") })
	if indexed == nil {
		t.Fatal("no step says what was indexed")
	}
	if got := strings.Join(strings.Fields(visibleTextOf(indexed)), " "); strings.Contains(got, " ,") || !strings.Contains(got, "2 rules indexed") {
		t.Errorf("the indexed step reads %q", got)
	}

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

// Skip, public repos only, confirms the choice as the prototype does: it returns to the dashboard, which says, once, in
// a status toast, that Rulemart only looks at public repos. Signed out, it asks to sign in first, and signing in never
// returns to it.
func TestSkippingPrivateProjectsSaysSo(t *testing.T) {
	site := newDashboardSite(t, octocatsGitHub(), octocatsCatalog())

	page := site.get(t, "/me/private")
	if got := formActions(t, page); !slices.Contains(got, "/me/private/skip") {
		t.Errorf("the page's forms post to %q, want Skip's", got)
	}
	if got := links(t, page, "Skip, public repos only"); len(got) != 0 {
		t.Errorf("Skip is a link to %q, which says nothing", got)
	}

	resp := send(t, site.handler, request{method: http.MethodPost, target: "/me/private/skip", cookies: []*http.Cookie{site.session}})
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/me" || len(site.gitHub.installations[1]) != 0 {
		t.Fatalf("skipping answered %d to %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	dashboard := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/me", cookies: []*http.Cookie{site.session, cookie(resp, noticeCookie)}}))
	if got, kind := toastText(t, dashboard), noticeToast(t, dashboard); got != "Okay. Rulemart will only look at your public repos." || kind != "status" {
		t.Errorf("the dashboard toasts %q as a %q toast, want a status toast", got, kind)
	}

	signedOut := send(t, site.handler, request{method: http.MethodPost, target: "/me/private/skip"})
	if want := "/signin?return=%2Fme%2Fprivate"; signedOut.StatusCode != http.StatusSeeOther || signedOut.Header.Get("Location") != want {
		t.Errorf("signed out, skipping answered %d to %q, want %q", signedOut.StatusCode, signedOut.Header.Get("Location"), want)
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

// Checkout says it found none of a signed-in visitor's projects only after a read that found none. Without a read to
// show, it says why instead, as other pages do: the read failed, with Try again back to the cart; Rulemart is reading,
// and the page refreshes itself; or the visitor must sign in again. Each still lets them enter a project.
func TestCheckoutSaysHowItsReadOfGitHubWentWhenItHasNoProjects(t *testing.T) {
	const noneFound = "We didn't find any of your projects using Code Rules"
	for name, tc := range map[string]struct {
		err     error
		failed  bool
		shows   string
		link    string
		refresh string
	}{
		"a failed read": {
			err: fmt.Errorf("read GitHub: %w: GitHub answered 502", accountsapp.ErrGitHubRead), failed: true,
			shows: "Rulemart couldn't read your repositories on GitHub just now.",
		},
		"a read under way": {
			err: accountsapp.ErrGitHubReading, refresh: "3",
			shows: "Rulemart is reading your repositories on GitHub. This page will update in a moment.",
		},
		"a token GitHub refuses": {
			err:   accountsapp.ErrNoGitHubToken,
			shows: "Sign in again so Rulemart can read your repositories on GitHub.", link: "/signin?again=1&return=%2Fcart",
		},
	} {
		t.Run(name, func(t *testing.T) {
			site := newDashboardSite(t, accounts.Snapshot{ReadFailed: tc.failed}, octocatsCatalog())
			site.gitHub.err = tc.err

			resp := send(t, site.handler, request{method: http.MethodGet, target: "/cart", cookies: []*http.Cookie{site.session}})
			page := body(t, resp)

			// The script shows the page's cards, so its markup holds what the visitor sees.
			markup := strings.Join(strings.Fields(page), " ")
			for _, want := range []string{tc.shows, "Enter your project"} {
				if !strings.Contains(markup, want) {
					t.Errorf("the page lacks %q", want)
				}
			}
			if strings.Contains(markup, noneFound) {
				t.Error("the page says Rulemart found no projects, without a read that found none")
			}
			if tc.failed && !strings.Contains(page, `action="/me/refresh?return=%2Fcart"`) {
				t.Error("Try again doesn't read GitHub again and return to the cart")
			}
			if tc.link != "" {
				if got := links(t, page, "Sign in again"); !slices.Equal(got, []string{tc.link}) {
					t.Errorf("Sign in again leads to %q, want %q", got, tc.link)
				}
			}
			if got := resp.Header.Get("Refresh"); got != tc.refresh {
				t.Errorf("Refresh %q, want %q", got, tc.refresh)
			}
		})
	}

	read := newDashboardSite(t, accounts.Snapshot{ReadAt: time.Now().Add(-3 * time.Minute)}, octocatsCatalog())
	if page := strings.Join(strings.Fields(read.get(t, "/cart")), " "); !strings.Contains(page, noneFound) {
		t.Error("after a read that found no projects, the page doesn't say so")
	}
}

// When a read of GitHub fails after an earlier one found projects, checkout still offers them, and says they're what the
// earlier read found, with Try again back to the cart, rather than presenting them as fresh.
func TestCheckoutSaysItsReadFailedWhileItOffersTheProjectsAnEarlierReadFound(t *testing.T) {
	failed := octocatsGitHub()
	failed.ReadFailed = true
	site := newDashboardSite(t, failed, octocatsCatalog())
	site.gitHub.err = fmt.Errorf("read GitHub: %w: GitHub answered 502", accountsapp.ErrGitHubRead)

	// The script shows the page's cards, so its markup holds what the visitor sees.
	page := strings.Join(strings.Fields(site.get(t, "/cart")), " ")

	for _, want := range []string{
		`value="octocat/api" data-cart-project`, "Rulemart couldn't read your repositories on GitHub just now. Showing what it read 3 minutes ago.",
		`action="/me/refresh?return=%2Fcart"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %q", want)
		}
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

// Without a read of GitHub to offer projects from, because it failed, another is under way, or the visitor must sign in
// again, a signed-in visitor's checkout is for the repository they entered, as the page offers, not a failure.
func TestCheckoutWithoutAReadOfGitHubIsForTheRepositoryEntered(t *testing.T) {
	for name, err := range map[string]error{
		"a failed read":          fmt.Errorf("read GitHub: %w: GitHub answered 502", accountsapp.ErrGitHubRead),
		"a read under way":       accountsapp.ErrGitHubReading,
		"a token GitHub refuses": accountsapp.ErrNoGitHubToken,
	} {
		t.Run(name, func(t *testing.T) {
			gitHub := newFakeGitHubAccounts(accounts.Snapshot{})
			gitHub.err = err
			carts := &fakeCarts{}
			site := newAccountsSite(t, func(o *web.Options) { o.GitHubAccounts, o.Carts = gitHub, carts })
			req := httptest.NewRequest(http.MethodPost, "/cart/checkout.json",
				strings.NewReader(`{"cart":["example/rules::techs/go/return-errors"],"repo":"https://github.com/octocat/new-app"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Sec-Fetch-Site", "same-origin")
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: string(site.accounts.signedIn(t, octocat))})
			recorder := httptest.NewRecorder()

			site.handler.ServeHTTP(recorder, req)

			if recorder.Code != http.StatusOK {
				t.Fatalf("checkout answered %d: %s", recorder.Code, recorder.Body)
			}
			if want := (domain.CheckoutTarget{Mode: domain.ProjectUnknown, Repository: "octocat/new-app"}); !reflect.DeepEqual(carts.target, want) {
				t.Errorf("the checkout is for %+v, want %+v", carts.target, want)
			}
		})
	}
}

// A project's provenance file is written by anyone who can push to it, and a checkout for the project copies its
// source names into commands, so a source name holding a newline or shell syntax never reaches them: the parser leaves
// the source out, and the library is added under a name of Rulemart's.
func TestCheckoutNeverCopiesAnUnsafeSourceNameFromAProjectsProvenance(t *testing.T) {
	sources, err := accounts.ParseProvenance([]byte(`{"sources": [
		{"name": "example\nprintf injected\n#", "repository": "https://github.com/example/rules"},
		{"name": "$(touch pwned)", "repository": "https://github.com/example/rules"}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := accounts.Snapshot{ReadAt: time.Now(), Projects: []accounts.Project{
		{Repository: accounts.Repository{Owner: "octocat", Name: "api"}, Sources: sources},
	}}
	carts := &fakeCarts{}
	site := newAccountsSite(t, func(o *web.Options) { o.GitHubAccounts, o.Carts = newFakeGitHubAccounts(snapshot), carts })
	req := httptest.NewRequest(http.MethodPost, "/cart/checkout.json", strings.NewReader(`{"cart":["example/rules::techs/go/return-errors"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: string(site.accounts.signedIn(t, octocat))})
	recorder := httptest.NewRecorder()
	site.handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("checkout answered %d: %s", recorder.Code, recorder.Body)
	}

	checkout := domain.NewCheckout(carts.target, []domain.CheckoutLibrary{{
		Owner: "example", Name: "rules", Vetted: true, Release: 1,
		Rules: []domain.CheckoutRule{{ID: "techs/go/return-errors", Group: domain.CheckoutGroup{ID: "techs/go", Name: "Go"}, Version: "1.0.0"}},
	}})
	var commands []string
	for _, step := range checkout.Commands() {
		commands = append(commands, step.Commands)
	}
	text := strings.Join(commands, "\n") + checkout.Prompt()
	for _, unsafe := range []string{"printf injected", "$(", "already imports"} {
		if strings.Contains(text, unsafe) {
			t.Errorf("the texts hold %q:\n%s", unsafe, text)
		}
	}
	if !strings.Contains(text, "code-rules project add library example \\\n") {
		t.Errorf("the texts don't add the library as example:\n%s", text)
	}
}
