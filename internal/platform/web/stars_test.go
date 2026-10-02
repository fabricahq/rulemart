package web_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/platform/web"
)

// fakeStars keeps stars in memory, as catalog/app.Stars does in Postgres: only a library in vetted can be starred.
type fakeStars struct {
	mu sync.Mutex
	// vetted names the libraries that can be starred, as lowercase owner/name, with how GitHub spells each.
	vetted map[string]views.LibraryRef
	// starred holds each account's stars, as "<account> <lowercase owner/name>".
	starred map[string]bool
	// listed is what AccountStars returns for each account.
	listed map[int64][]views.StarredLibrary
	// err, when set, fails every call.
	err error
}

func newFakeStars() *fakeStars {
	return &fakeStars{
		vetted:  map[string]views.LibraryRef{"example/rules": {Owner: "example", Name: "rules"}},
		starred: map[string]bool{}, listed: map[int64][]views.StarredLibrary{},
	}
}

func (f *fakeStars) Star(_ context.Context, accountID int64, library string) (views.LibraryRef, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return views.LibraryRef{}, f.err
	}
	ref, ok := f.vetted[strings.ToLower(library)]
	if !ok {
		return views.LibraryRef{}, fmt.Errorf("star library=%q: %w", library, app.ErrNotFound)
	}
	f.starred[fmt.Sprintf("%d %s", accountID, strings.ToLower(library))] = true
	return ref, nil
}

func (f *fakeStars) Unstar(_ context.Context, accountID int64, library string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	if !strings.Contains(library, "/") {
		return fmt.Errorf("unstar library=%q: %w", library, app.ErrNotFound)
	}
	delete(f.starred, fmt.Sprintf("%d %s", accountID, strings.ToLower(library)))
	return nil
}

func (f *fakeStars) Starred(_ context.Context, accountID int64, owner, name string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.starred[fmt.Sprintf("%d %s/%s", accountID, strings.ToLower(owner), strings.ToLower(name))], f.err
}

func (f *fakeStars) AccountStars(_ context.Context, accountID int64) ([]views.StarredLibrary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listed[accountID], f.err
}

// stars returns the stars every account holds, sorted.
func (f *fakeStars) all() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var all []string
	for star := range f.starred {
		all = append(all, star)
	}
	slices.Sort(all)
	return all
}

// starSite is the pages of unvettedCatalog, with three stars on example/rules, with sign-in and stars through fakes,
// and a signed-in visitor's session cookie.
type starSite struct {
	listingSite
	stars *fakeStars
}

func newStarSite(t *testing.T) starSite {
	t.Helper()
	return newStarSiteWith(t, func(*web.Options) {})
}

// newStarSiteWith returns a starSite whose options adjust changes.
func newStarSiteWith(t *testing.T, adjust func(*web.Options)) starSite {
	t.Helper()
	c := unvettedCatalog()
	starred := exampleRules
	starred.Stars = 3
	for key, page := range c.pages {
		if page.Library.Vetted {
			page.Library = starred
			c.pages[key] = page
		}
	}
	for key, page := range c.releases {
		if page.Library.Vetted {
			page.Library = starred
			c.releases[key] = page
		}
	}
	for key, comparison := range c.releaseComparisons {
		if comparison.Library.Vetted {
			comparison.Library = starred
			c.releaseComparisons[key] = comparison
		}
	}
	c.libraries[0].Stars = 3
	stars := newFakeStars()
	accounts := newFakeAccounts()
	site := accountsSite{accounts: accounts, gitHub: &fakeGitHub{identity: octocat}, logs: &bytes.Buffer{}}
	options := web.Options{
		Log: slog.New(slog.NewJSONHandler(site.logs, nil)), Accounts: accounts, GitHub: site.gitHub, Stars: stars,
		Listings: &fakeListings{byAccount: map[int64][]views.AccountListing{}},
	}
	adjust(&options)
	handler, err := web.New(c, options)
	if err != nil {
		t.Fatal(err)
	}
	site.handler = handler
	token := accounts.signedIn(t, octocat)
	return starSite{
		listingSite: listingSite{accountsSite: site, session: &http.Cookie{Name: sessionCookie, Value: string(token)}},
		stars:       stars,
	}
}

// octocatID is octocat's account ID in fakeAccounts.
const octocatID = 1

// starPath is where example/rules's Star button posts from its own page, and unstarPath where Starred posts.
const (
	starPath   = "/account/stars?library=example%2Frules"
	unstarPath = "/account/stars/remove?library=example%2Frules"
)

// A visitor who isn't signed in sees a library's stars, and a Star link that signs them in and returns them to the
// same page, on a page that's the same for everyone and cached.
func TestALibrarysPagesOfferAVisitorWhoIsntSignedInToSignInAndStar(t *testing.T) {
	site := newStarSite(t)

	for _, path := range []string{library, library + "?tab=rules", library + "?tab=releases", library + "?tab=releases&from=1&to=3"} {
		resp := send(t, site.handler, request{method: http.MethodGet, target: path})
		if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != "public, max-age=60" {
			t.Fatalf("%s: got %d, cached as %q; want a public page", path, resp.StatusCode, resp.Header.Get("Cache-Control"))
		}
		page := body(t, resp)
		assertShows(t, page, "Star, 3 stars")
		want := "/sign-in?" + url.Values{"return": {path}, "to": {"star"}}.Encode()
		if got := links(t, page, "Star"); !slices.Equal(got, []string{want}) {
			t.Errorf("%s: Star leads to %q, want %q", path, got, want)
		}
		if actions := formActions(t, page); slices.ContainsFunc(actions, func(a string) bool { return strings.HasPrefix(a, "/account/stars") }) {
			t.Errorf("%s: a public page has a star form %q", path, actions)
		}
	}
	assertShows(t, body(t, send(t, site.handler, request{method: http.MethodGet, target: "/sign-in?return=%2Fexample%2Frules&to=star"})),
		"Sign in to star libraries. You'll come back to this one.")
}

// A signed-in visitor's Star button posts to star the library and return to the same page, and once starred, the
// button says so and posts to unstar it. Their pages are never cached.
func TestASignedInVisitorStarsALibraryFromItsPages(t *testing.T) {
	site := newStarSite(t)

	resp := site.signedInGet(t, library+"?tab=rules")
	if resp.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("a signed-in visitor's page is cached as %q", resp.Header.Get("Cache-Control"))
	}
	page := body(t, resp)
	assertShows(t, page, "Star, 3 stars")
	formAction := starPath + "&" + url.Values{"return": {library + "?tab=rules"}}.Encode()
	if got := formActions(t, page); !slices.Contains(got, formAction) {
		t.Fatalf("the page's forms post to %q, want %q", got, formAction)
	}

	starred := site.signedInPost(t, formAction)
	if starred.StatusCode != http.StatusSeeOther || starred.Header.Get("Location") != library+"?tab=rules" ||
		starred.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("starring answered %d to %q, cached as %q", starred.StatusCode, starred.Header.Get("Location"),
			starred.Header.Get("Cache-Control"))
	}
	if got := site.stars.all(); !slices.Equal(got, []string{"1 example/rules"}) {
		t.Fatalf("stars %q, want octocat's on example/rules", got)
	}

	page = body(t, site.signedInGet(t, library))
	assertShows(t, page, "Starred, 3 stars. Unstar")
	if got := formActions(t, page); !slices.Contains(got, unstarPath) || slices.Contains(got, starPath) {
		t.Fatalf("a starred library's forms post to %q, want %q and not %q", got, unstarPath, starPath)
	}
	unstarred := site.signedInPost(t, unstarPath)
	if unstarred.StatusCode != http.StatusSeeOther || unstarred.Header.Get("Location") != library {
		t.Fatalf("unstarring answered %d to %q, want a redirect to the library", unstarred.StatusCode, unstarred.Header.Get("Location"))
	}
	if got := site.stars.all(); len(got) != 0 {
		t.Fatalf("stars %q after unstarring", got)
	}
}

// Starring or unstarring twice, as a double click or a reload might, lands where once does.
func TestStarringAndUnstarringTwiceIsLikeOnce(t *testing.T) {
	site := newStarSite(t)

	for range 2 {
		if resp := site.signedInPost(t, starPath); resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != library {
			t.Fatalf("starring answered %d to %q", resp.StatusCode, resp.Header.Get("Location"))
		}
	}
	if got := site.stars.all(); !slices.Equal(got, []string{"1 example/rules"}) {
		t.Fatalf("stars %q", got)
	}
	for range 2 {
		if resp := site.signedInPost(t, unstarPath); resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("unstarring answered %d", resp.StatusCode)
		}
	}
	if got := site.stars.all(); len(got) != 0 {
		t.Fatalf("stars %q", got)
	}
}

// A visitor who isn't signed in, such as one whose session ended in another tab, is sent to sign in and return to
// where they were, and nothing changes.
func TestStarringSignedOutSignsInAndReturnsToThePage(t *testing.T) {
	site := newStarSite(t)

	for target, back := range map[string]string{
		starPath: library,
		starPath + "&return=%2Fexample%2Frules%3Ftab%3Drules": library + "?tab=rules",
		unstarPath + "&return=%2Faccount%2Fstars":             "/account/stars",
	} {
		resp := send(t, site.handler, request{method: http.MethodPost, target: target})
		want := "/sign-in?" + url.Values{"return": {back}, "to": {"star"}}.Encode()
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != want {
			t.Errorf("%s: answered %d to %q, want a redirect to %q", target, resp.StatusCode, resp.Header.Get("Location"), want)
		}
	}
	if got := site.stars.all(); len(got) != 0 {
		t.Fatalf("stars %q for a visitor who isn't signed in", got)
	}
}

// A star returns only to a path on this site.
func TestStarringReturnsOnlyToPathsOnThisSite(t *testing.T) {
	site := newStarSite(t)

	for _, back := range []string{"//evil.example", "https://evil.example/", `/\evil.example`} {
		resp := site.signedInPost(t, starPath+"&"+url.Values{"return": {back}}.Encode())
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/" {
			t.Errorf("return %q: answered %d to %q, want a redirect home", back, resp.StatusCode, resp.Header.Get("Location"))
		}
	}
}

// Another site can't star or unstar for a visitor.
func TestAnotherSiteCantStarForAVisitor(t *testing.T) {
	site := newStarSite(t)

	for _, header := range []http.Header{
		{"Sec-Fetch-Site": {"cross-site"}},
		{"Sec-Fetch-Site": {"same-site"}},
		{"Sec-Fetch-Site": nil, "Origin": {"https://evil.example"}},
	} {
		for _, target := range []string{starPath, unstarPath} {
			resp := send(t, site.handler, request{method: http.MethodPost, target: target, cookies: []*http.Cookie{site.session}, header: header})
			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("%s with %v: got %d, want 403", target, header, resp.StatusCode)
			}
		}
	}
	if got := site.stars.all(); len(got) != 0 {
		t.Fatalf("stars %q from another site", got)
	}
}

// Only a vetted library can be starred: an unvetted library's pages show no stars, and starring one, or a library
// Rulemart doesn't have, is missing.
func TestOnlyAVettedLibraryCanBeStarredFromItsPage(t *testing.T) {
	site := newStarSite(t)

	for _, get := range []func(*testing.T, string) *http.Response{
		site.signedInGet,
		func(t *testing.T, path string) *http.Response {
			return send(t, site.handler, request{method: http.MethodGet, target: path})
		},
	} {
		page := body(t, get(t, unvettedLibrary))
		if strings.Contains(visibleText(t, page), "Star") {
			t.Error("an unvetted library's page offers a star")
		}
	}
	for _, target := range []string{"/account/stars?library=stranger%2Frules", "/account/stars?library=nobody%2Fnothing", "/account/stars"} {
		resp := site.signedInPost(t, target)
		if resp.StatusCode != http.StatusNotFound || resp.Header.Get("Cache-Control") != "private, no-store" {
			t.Errorf("%s: got %d, cached as %q; want a private 404", target, resp.StatusCode, resp.Header.Get("Cache-Control"))
		}
	}
	if got := site.stars.all(); len(got) != 0 {
		t.Fatalf("stars %q", got)
	}
}

// The libraries list each library's stars, when it has any.
func TestTheLibrariesListShowsStars(t *testing.T) {
	site := newStarSite(t)

	for _, path := range []string{"/", "/libraries"} {
		assertShows(t, body(t, send(t, site.handler, request{method: http.MethodGet, target: path})), "3 stars")
	}
	if page := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/unvetted"})); strings.Contains(visibleText(t, page), "star") {
		t.Error("the unvetted libraries show stars")
	}
}

// A signed-in visitor's stars page lists the libraries they starred, each with Unstar, and says which no longer
// shows as vetted. Others are asked to sign in and come back.
func TestTheStarsPageListsTheVisitorsStars(t *testing.T) {
	site := newStarSite(t)
	site.stars.listed[octocatID] = []views.StarredLibrary{
		{Library: views.LibraryCard{Owner: "example", Name: "rules", Description: "Example rules for tests.", Rules: 2, Stars: 3}, Vetted: true, StarredAt: day(3)},
		{Library: views.LibraryCard{Owner: "stranger", Name: "rules", Rules: 2, Stars: 1}, Listed: true, StarredAt: day(2)},
		{Library: views.LibraryCard{Owner: "gone", Name: "rules", Rules: 1, Stars: 1}, StarredAt: day(1)},
	}

	signedOut := send(t, site.handler, request{method: http.MethodGet, target: "/account/stars"})
	if want := "/sign-in?return=%2Faccount%2Fstars"; signedOut.StatusCode != http.StatusSeeOther || signedOut.Header.Get("Location") != want {
		t.Fatalf("signed out: answered %d to %q, want a redirect to %q", signedOut.StatusCode, signedOut.Header.Get("Location"), want)
	}
	assertShows(t, body(t, send(t, site.handler, request{method: http.MethodGet, target: "/sign-in?return=%2Faccount%2Fstars"})),
		"Sign in to see your stars.")

	resp := site.signedInGet(t, "/account/stars")
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("got %d, cached as %q", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}
	page := body(t, resp)
	assertShows(t, page, "Your stars", "rules Example rules for tests. example/rules · Starred 3 Sep 2026",
		"rules Unvetted stranger/rules · Starred 2 Sep 2026", "rules No longer on Rulemart gone/rules · Starred 1 Sep 2026")
	if content, _ := robots(t, page); content != "noindex" {
		t.Errorf("robots %q, want noindex", content)
	}
	if got := rels(t, page, library); !slices.Equal(got, []string{""}) {
		t.Errorf("the vetted library's link has rel %q", got)
	}
	if got := rels(t, page, unvettedLibrary); !slices.Equal(got, []string{"nofollow"}) {
		t.Errorf("the unvetted library's link has rel %q, want nofollow", got)
	}
	if got := rels(t, page, "/gone/rules"); len(got) != 0 {
		t.Errorf("the page links a library Rulemart no longer shows")
	}
	back := "&return=%2Faccount%2Fstars"
	want := []string{unstarPath + back, "/account/stars/remove?library=stranger%2Frules" + back, "/account/stars/remove?library=gone%2Frules" + back}
	if got := formActions(t, page); !slices.Equal(got, append([]string{"/sign-out"}, want...)) {
		t.Errorf("the page's forms post to %q, want sign-out and %q", got, want)
	}
}

func TestTheStarsPageSaysWhenThereAreNone(t *testing.T) {
	site := newStarSite(t)

	assertShows(t, body(t, site.signedInGet(t, "/account/stars")), "You haven't starred a library.")
}

// The account menu leads to the visitor's stars, and the account page says Rulemart keeps them, and that deleting the
// account removes them.
func TestTheAccountMenuAndPageNameTheVisitorsStars(t *testing.T) {
	site := newStarSite(t)

	if got := links(t, body(t, site.signedInGet(t, "/")), "Your stars"); !slices.Equal(got, []string{"/account/stars"}) {
		t.Errorf("the menu's Your stars leads to %q", got)
	}
	assertShows(t, body(t, site.signedInGet(t, "/account")), "which libraries you star", "It removes your stars and your listings")
	signedOut := site.signedInPost(t, "/sign-out?return=%2Faccount%2Fstars")
	if signedOut.Header.Get("Location") != "/" {
		t.Errorf("signing out of the stars page returns to %q, want home", signedOut.Header.Get("Location"))
	}
}

// A failure to read whether the visitor starred a library fails the page, rather than offering a star they have.
func TestAFailedStarReadFailsThePage(t *testing.T) {
	site := newStarSite(t)
	site.stars.err = errors.New("the database is down")

	if resp := site.signedInGet(t, library); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503", resp.StatusCode)
	}
	if resp := send(t, site.handler, request{method: http.MethodGet, target: library}); resp.StatusCode != http.StatusOK {
		t.Fatalf("a visitor who isn't signed in got %d; their page reads no stars of theirs", resp.StatusCode)
	}
}

// Where no one can sign in, pages still count stars, with nothing to click.
func TestWithoutSignInPagesOnlyCountStars(t *testing.T) {
	site := newStarSiteWith(t, func(o *web.Options) { o.Accounts, o.GitHub = nil, nil })

	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: library}))
	assertShows(t, page, "3 stars")
	if got := links(t, page, "Star"); len(got) != 0 {
		t.Errorf("Star leads to %q where no one can sign in", got)
	}
}

// Without stars, pages show none, and their addresses are missing.
func TestWithoutStarsPagesShowNone(t *testing.T) {
	site := newStarSiteWith(t, func(o *web.Options) { o.Stars = nil })

	if page := body(t, site.signedInGet(t, library)); strings.Contains(visibleText(t, page), "Star") {
		t.Error("the library's page offers a star")
	}
	if resp := site.signedInGet(t, "/account/stars"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("the stars page answered %d, want 404", resp.StatusCode)
	}
}
