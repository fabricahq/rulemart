package web_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"

	accounts "github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/platform/web"
)

const (
	// unvettedLibrary is the library unvettedCatalog lists without vetting it.
	unvettedLibrary = "/stranger/rules"
	unvettedWarning = "This library has not been vetted. Be sure to review these rules carefully."
)

// strangerRules is exampleRules, listed as stranger/rules and not vetted.
var strangerRules = func() views.Library {
	lib := exampleRules
	lib.Owner, lib.Vetted = "stranger", false
	return lib
}()

// unvettedCatalog returns historyCatalog with every page of example/rules also as stranger/rules, a library listed
// but not vetted, which the unvetted libraries list.
func unvettedCatalog() catalog {
	c := historyCatalog()
	move := func(key string) string { return "stranger/rules" + strings.TrimPrefix(key, "example/rules") }
	for key, page := range maps.Clone(c.pages) {
		page.Library = strangerRules
		c.pages[move(key)] = page
	}
	for key, page := range maps.Clone(c.rules) {
		page.Library = strangerRules
		c.rules[move(key)] = page
	}
	for key, page := range maps.Clone(c.releases) {
		page.Library = strangerRules
		c.releases[move(key)] = page
	}
	for key, comparison := range maps.Clone(c.releaseComparisons) {
		comparison.Library = strangerRules
		c.releaseComparisons[move(key)] = comparison
	}
	for key, comparison := range maps.Clone(c.ruleComparisons) {
		comparison.Page.Library = strangerRules
		c.ruleComparisons[move(key)] = comparison
	}
	c.unvetted = []views.LibraryCard{{Owner: "stranger", Name: "rules", Description: "Stranger's rules.", Rules: 2}}
	return c
}

// unvettedPages are every kind of page about the unvetted library, with the status each answers.
var unvettedPages = map[string]int{
	unvettedLibrary:                                                              http.StatusOK,
	unvettedLibrary + "?tab=rules":                                               http.StatusOK,
	unvettedLibrary + "?tab=releases":                                            http.StatusOK,
	unvettedLibrary + "?tab=releases&from=1&to=3":                                http.StatusOK,
	unvettedLibrary + "?tab=releases&from=1&to=9":                                http.StatusNotFound,
	unvettedLibrary + "/techs/go/return-errors":                                  http.StatusOK,
	unvettedLibrary + "/techs/go/return-errors?tab=versions":                     http.StatusOK,
	unvettedLibrary + "/techs/go/return-errors?tab=versions&from=1.0.0&to=2.0.0": http.StatusOK,
	unvettedLibrary + "/techs/go/return-errors?tab=versions&from=1.0.0&to=9.0.0": http.StatusNotFound,
}

// robots returns the content of a page's robots meta tag, and whether it has a canonical link.
func robots(t *testing.T, body string) (content string, canonical bool) {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for n := range doc.Descendants() {
		if n.Type != html.ElementNode {
			continue
		}
		attrs := map[string]string{}
		for _, a := range n.Attr {
			attrs[a.Key] = a.Val
		}
		switch {
		case n.Data == "meta" && attrs["name"] == "robots":
			content = attrs["content"]
		case n.Data == "link" && attrs["rel"] == "canonical":
			canonical = true
		}
	}
	return content, canonical
}

// rels returns the rel attribute of each link in body to href.
func rels(t *testing.T, body, href string) []string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for n := range doc.Descendants() {
		if n.Type != html.ElementNode || n.Data != "a" {
			continue
		}
		var target, rel string
		for _, a := range n.Attr {
			switch a.Key {
			case "href":
				target = a.Val
			case "rel":
				rel = a.Val
			}
		}
		if target == href {
			found = append(found, rel)
		}
	}
	return found
}

// Every page about an unvetted library warns that it isn't vetted, asks search engines neither to index it nor to
// follow its links, and names no canonical address, even with a base URL.
func TestEveryUnvettedPageWarnsAndKeepsSearchEnginesAway(t *testing.T) {
	handler := newSiteAt(t, unvettedCatalog(), "https://rulemart.example")

	for path, status := range unvettedPages {
		t.Run(path, func(t *testing.T) {
			resp := get(t, handler, path)

			if resp.Code != status {
				t.Fatalf("got %d, want %d", resp.Code, status)
			}
			page := resp.Body.String()
			assertShows(t, page, unvettedWarning)
			if content, canonical := robots(t, page); content != "noindex, nofollow" || canonical {
				t.Errorf("robots %q, canonical %v; want noindex, nofollow and no canonical address", content, canonical)
			}
		})
	}
}

// The same pages for a vetted library don't warn, and those search engines index name their canonical address.
func TestVettedPagesDontWarn(t *testing.T) {
	handler := newSiteAt(t, unvettedCatalog(), "https://rulemart.example")

	for path, status := range unvettedPages {
		path = library + strings.TrimPrefix(path, unvettedLibrary)
		resp := get(t, handler, path)
		if resp.Code != status {
			t.Fatalf("%s: got %d, want %d", path, resp.Code, status)
		}
		if strings.Contains(resp.Body.String(), "not been vetted") {
			t.Errorf("%s warns that the library isn't vetted", path)
		}
		if content, _ := robots(t, resp.Body.String()); strings.Contains(content, "nofollow") {
			t.Errorf("%s: robots %q", path, content)
		}
	}
	if _, canonical := robots(t, get(t, handler, library).Body.String()); !canonical {
		t.Error("a vetted library's page names no canonical address")
	}
}

// The unvetted libraries are reached from the bottom of the libraries page, which links them with nofollow, and their
// page lists them under the warning, each linked with nofollow.
func TestUnvettedLibrariesAreReachedFromTheLibrariesPage(t *testing.T) {
	handler := newSite(t, unvettedCatalog())

	libraries := get(t, handler, "/libraries").Body.String()
	if got := rels(t, libraries, "/unvetted"); !slices.Equal(got, []string{"nofollow"}) {
		t.Fatalf("the libraries page links the unvetted libraries with rel %q, want one nofollow link", got)
	}
	if strings.Contains(libraries, "stranger") {
		t.Fatal("the libraries page shows the unvetted library")
	}
	resp := get(t, handler, "/unvetted")
	if resp.Code != http.StatusOK {
		t.Fatalf("got %d", resp.Code)
	}
	page := resp.Body.String()
	assertShows(t, page, "These libraries have not been vetted. Be sure to review their rules carefully.", "rules Stranger's rules. stranger/rules · 2 rules ›")
	if got := rels(t, page, unvettedLibrary); !slices.Equal(got, []string{"nofollow"}) {
		t.Errorf("the unvetted page links the library with rel %q, want nofollow", got)
	}
	if content, canonical := robots(t, page); content != "noindex, nofollow" || canonical {
		t.Errorf("robots %q, canonical %v; want noindex, nofollow and no canonical address", content, canonical)
	}
	if strings.Contains(page, `href="/list"`) {
		t.Error("the unvetted page offers listing on a site where no one can sign in")
	}
	if got := get(t, handler, "/UNVETTED"); got.Code != http.StatusMovedPermanently || got.Header().Get("Location") != "/unvetted" {
		t.Errorf("/UNVETTED answered %d to %q, want a redirect to /unvetted", got.Code, got.Header().Get("Location"))
	}
}

// With no unvetted libraries, the page says so, without warning about libraries it doesn't show.
func TestUnvettedPageSaysWhenThereAreNone(t *testing.T) {
	page := get(t, newSite(t, newCatalog()), "/unvetted").Body.String()

	assertShows(t, page, "No unvetted libraries yet.")
	if strings.Contains(page, "have not been vetted") {
		t.Error("the empty page warns about libraries it doesn't show")
	}
}

// Links an unvetted library's author wrote in a rule carry nofollow and ugc; a vetted library's don't.
func TestLinksInAnUnvettedRuleAreMarkedAsItsAuthors(t *testing.T) {
	c := unvettedCatalog()
	for key, page := range c.rules {
		page.Rule.HTML = `<p>See <a href="https://example.com/guide">the guide</a>.</p>`
		c.rules[key] = page
	}
	handler := newSite(t, c)

	if got := rels(t, get(t, handler, unvettedLibrary+"/techs/go/return-errors").Body.String(), "https://example.com/guide"); !slices.Equal(got, []string{"nofollow ugc"}) {
		t.Errorf("the unvetted rule's link has rel %q, want nofollow ugc", got)
	}
	if got := rels(t, get(t, handler, library+"/techs/go/return-errors").Body.String(), "https://example.com/guide"); !slices.Equal(got, []string{""}) {
		t.Errorf("the vetted rule's link has rel %q, want none", got)
	}
}

// A refusal marks the field invalid, describes it with the refusal, and takes the focus, so it's read first, with the
// caret at the end of what the visitor typed, which caret.js puts there.
func TestAddPageFocusesTheFieldARefusalDescribes(t *testing.T) {
	site := newListingSite(t)

	page := body(t, site.signedInGet(t, "/me/add?url=example"))

	for _, want := range []string{`aria-invalid="true"`, `aria-describedby="library-url-problem"`, "autofocus", `role="alert"`, "/caret.js"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page has no %s", want)
		}
	}
	if page := body(t, site.signedInGet(t, "/me/add?url=example%2Fnew")); strings.Contains(page, `aria-invalid="true"`) || strings.Contains(page, " autofocus") || strings.Contains(page, "/caret.js") {
		t.Error("the page marks a repository it accepted invalid")
	}
}

// fakeListings keeps listings in memory, as catalog/app.Listings does in Postgres, and refuses as told.
type fakeListings struct {
	mu sync.Mutex
	// byAccount holds each account's listings, newest first.
	byAccount map[int64][]views.AccountListing
	// refusal, when set, refuses every repository to check or list.
	refusal error
	// notQueued makes List and Retry report that their check wasn't queued.
	notQueued bool
	// retryRefusal, when set, refuses every retry.
	retryRefusal error
	// listed, removed, and retried record what each account asked, as "<account> <repository>" and listing IDs.
	listed           []string
	removed, retried []int64
}

func (f *fakeListings) Check(_ context.Context, _ int64, text string) (app.Repository, error) {
	owner, name, err := domain.ParseGitHubRepository(text)
	if err != nil {
		return app.Repository{}, err
	}
	return app.Repository{Owner: owner, Name: name}, f.refusal
}

func (f *fakeListings) List(ctx context.Context, accountID int64, text string) (int64, error) {
	repo, err := f.Check(ctx, accountID, text)
	if err != nil {
		return 0, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listed = append(f.listed, fmt.Sprintf("%d %s", accountID, repo.FullName()))
	if f.notQueued {
		return 7, fmt.Errorf("queue check of listing id=7: %w: SQS is down", app.ErrNotQueued)
	}
	return 7, nil
}

func (f *fakeListings) AccountListings(_ context.Context, accountID int64) ([]views.AccountListing, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.byAccount[accountID], nil
}

func (f *fakeListings) Remove(_ context.Context, accountID, id int64) error {
	return f.change(accountID, id, &f.removed, func(views.AccountListing) bool { return true })
}

func (f *fakeListings) Retry(_ context.Context, accountID, id int64) error {
	if f.retryRefusal != nil {
		return f.retryRefusal
	}
	err := f.change(accountID, id, &f.retried, func(l views.AccountListing) bool { return l.Failure != "" })
	if errors.Is(err, app.ErrNotFound) && f.has(accountID, id) {
		return fmt.Errorf("retry listing id=%d: %w", id, app.ErrListingNotFailed)
	}
	if err == nil && f.notQueued {
		return fmt.Errorf("%w: SQS is down", app.ErrNotQueued)
	}
	return err
}

// has reports whether the account has the listing id.
func (f *fakeListings) has(accountID, id int64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.ContainsFunc(f.byAccount[accountID], func(l views.AccountListing) bool { return l.ID == id })
}

// change records id in changed when the account has that listing and ok says it may change, and fails with
// app.ErrNotFound otherwise.
func (f *fakeListings) change(accountID, id int64, changed *[]int64, ok func(views.AccountListing) bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, l := range f.byAccount[accountID] {
		if l.ID == id && ok(l) {
			*changed = append(*changed, id)
			return nil
		}
	}
	return fmt.Errorf("change listing id=%d: %w", id, app.ErrNotFound)
}

// listingSite is the pages with sign-in and listings through fakes, and a signed-in visitor's session cookie.
type listingSite struct {
	accountsSite
	listings *fakeListings
	// session signs in as octocat, account 1.
	session *http.Cookie
}

func newListingSite(t *testing.T) listingSite {
	t.Helper()
	listings := &fakeListings{byAccount: map[int64][]views.AccountListing{}}
	site := newAccountsSite(t, func(o *web.Options) { o.Listings = listings })
	token := site.accounts.signedIn(t, octocat)
	return listingSite{accountsSite: site, listings: listings, session: &http.Cookie{Name: sessionCookie, Value: string(token)}}
}

// signedInGet requests target as the signed-in visitor.
func (s listingSite) signedInGet(t *testing.T, target string) *http.Response {
	t.Helper()
	return send(t, s.handler, request{method: http.MethodGet, target: target, cookies: []*http.Cookie{s.session}})
}

// signedInPost posts to target as the signed-in visitor, with an empty body.
func (s listingSite) signedInPost(t *testing.T, target string) *http.Response {
	t.Helper()
	return send(t, s.handler, request{method: http.MethodPost, target: target, cookies: []*http.Cookie{s.session}})
}

// A visitor who isn't signed in is asked to sign in, and comes back to the page with what they typed; the listing
// form's old address leads there too.
func TestAddPageAsksAVisitorToSignInFirst(t *testing.T) {
	site := newListingSite(t)

	for target, back := range map[string]string{
		"/me/add":                   "/me/add",
		"/me/add?url=example%2Fnew": "/me/add?url=example%2Fnew",
	} {
		resp := send(t, site.handler, request{method: http.MethodGet, target: target})
		want := "/signin?" + url.Values{"return": {back}}.Encode()
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != want {
			t.Errorf("%s answered %d to %q, want %q", target, resp.StatusCode, resp.Header.Get("Location"), want)
		}
	}
	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/signin?return=%2Fme%2Fadd"}))
	assertShows(t, page, "Sign in to add a library.")
	resp := send(t, site.handler, request{method: http.MethodPost, target: "/me/add?repository=example%2Fnew"})
	want := "/signin?" + url.Values{"return": {"/me/add?url=example%2Fnew"}}.Encode()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != want {
		t.Fatalf("adding signed out answered %d to %q, want a redirect to %q", resp.StatusCode, resp.Header.Get("Location"), want)
	}
	if len(site.listings.listed) != 0 {
		t.Fatalf("listed %q for a visitor who isn't signed in", site.listings.listed)
	}
	legacy := send(t, site.handler, request{method: http.MethodGet, target: "/list?repository=example%2Fnew"})
	if legacy.StatusCode != http.StatusMovedPermanently || legacy.Header.Get("Location") != "/me/add?url=example%2Fnew" {
		t.Errorf("the old listing form answered %d to %q", legacy.StatusCode, legacy.Header.Get("Location"))
	}
}

// The form checks the address the visitor wrote, then offers to add the repository it names, with it in Add this
// library's action, since a POST can't have a body.
func TestAddPageConfirmsARepositoryBeforeAddingIt(t *testing.T) {
	site := newListingSite(t)

	page := body(t, site.signedInGet(t, "/me/add"))
	if !strings.Contains(page, `name="url"`) || slices.Contains(formActions(t, page), "/me/add?repository=example%2Fnew") {
		t.Fatal("the form doesn't ask for an address, or offers a repository before it's given")
	}
	assertShows(t, page, "Add a library", "Add a library by URL Anyone can add a public library. Its page shows that you added it.")
	resp := site.signedInGet(t, "/me/add?url="+url.QueryEscape("https://github.com/example/new.git"))
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("got %d, cached as %q", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}
	page = body(t, resp)
	assertShows(t, page, "example/new on GitHub Public library on GitHub Add this library")
	if got := formActions(t, page); !slices.Contains(got, "/me/add?repository=example%2Fnew") {
		t.Fatalf("the page's forms post to %q, want /me/add?repository=example%%2Fnew", got)
	}
	if content, _ := robots(t, page); content != "noindex" {
		t.Errorf("robots %q, want noindex", content)
	}
}

// formActions returns the action of each form in body that posts.
func formActions(t *testing.T, body string) []string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var actions []string
	for n := range doc.Descendants() {
		if n.Type != html.ElementNode || n.Data != "form" {
			continue
		}
		var method, action string
		for _, a := range n.Attr {
			switch a.Key {
			case "method":
				method = a.Val
			case "action":
				action = a.Val
			}
		}
		if method == "post" {
			actions = append(actions, action)
		}
	}
	return actions
}

// Each reason a repository can't be added says what to do, in the prototype's words, and links the library already
// there, with nofollow when it isn't vetted, or the visitor's own check of it; Add this library shows only for a
// repository that may be added.
func TestAddPageSaysWhyARepositoryCantBeAdded(t *testing.T) {
	for name, test := range map[string]struct {
		repository string
		refusal    error
		want       string
		href       string
		rel        []string
	}{
		"not a repository": {"example", nil, "Enter a GitHub repository URL, like https://github.com/owner/repo.", "", nil},
		"added by the visitor": {"stranger/rules", &app.ListingConflict{Own: true, Library: views.LibraryRef{Owner: "stranger", Name: "rules"}},
			"You added stranger/rules already. See how it went", "/me/add/run?repo=stranger%2Frules", []string{""}},
		"added by the visitor, not checked yet": {"someone/new", &app.ListingConflict{Own: true},
			"You added someone/new already. See how it went", "/me/add/run?repo=someone%2Fnew", []string{""}},
		"added by someone else a moment ago": {"someone/new", &app.ListingConflict{Checking: true, RequestedAt: time.Now()},
			"Someone added someone/new a moment ago, and Rulemart is checking it.", "", nil},
		"added by someone else, checked for long": {"someone/new", &app.ListingConflict{Checking: true, RequestedAt: time.Now().Add(-10 * time.Minute)},
			"Someone added someone/new, and Rulemart's check of it is taking longer than usual. Rulemart checks it again within the hour.", "", nil},
		"at the account's limit": {"someone/new", app.ErrAccountListingLimit,
			"You have 5 libraries Rulemart hasn't vetted, as many as an account may add. Remove one, such as one that failed, to add another. My libraries",
			"", nil},
		"full":      {"someone/new", app.ErrListingsFull, "Rulemart isn't taking new libraries right now.", "", nil},
		"too often": {"someone/new", app.ErrListingTooOften, "You've listed or retried 20 times in the last day", "", nil},
		"busy":      {"someone/new", app.ErrListingsBusy, "Rulemart is checking many listings right now. Try again in an hour.", "", nil},
	} {
		t.Run(name, func(t *testing.T) {
			site := newListingSite(t)
			site.listings.refusal = test.refusal

			page := body(t, site.signedInGet(t, "/me/add?url="+url.QueryEscape(test.repository)))

			assertShows(t, page, test.want)
			if slices.Contains(formActions(t, page), "/me/add?repository="+url.QueryEscape(test.repository)) {
				t.Error("the page offers to add a repository it refused")
			}
			if test.href != "" {
				if got := rels(t, page, test.href); !slices.Equal(got, test.rel) {
					t.Errorf("links %s with rel %q, want %q", test.href, got, test.rel)
				}
			}

			resp := site.signedInPost(t, "/me/add?repository="+url.QueryEscape(test.repository))
			if resp.StatusCode != http.StatusConflict || len(site.listings.listed) != 0 {
				t.Fatalf("adding answered %d and listed %q, want 409 and nothing listed", resp.StatusCode, site.listings.listed)
			}
			assertShows(t, body(t, resp), test.want)
		})
	}
}

// A repository Rulemart has already leads to its library's page, which says so in a status toast, as the prototype's
// form does.
func TestAddingALibraryRulemartHasLeadsToItsPage(t *testing.T) {
	for name, conflict := range map[string]*app.ListingConflict{
		"vetted": {Vetted: true, Library: views.LibraryRef{Owner: "example", Name: "rules"}},
		"listed": {Library: views.LibraryRef{Owner: "example", Name: "rules"}},
	} {
		t.Run(name, func(t *testing.T) {
			site := newListingSite(t)
			site.listings.refusal = conflict

			for _, resp := range []*http.Response{site.signedInGet(t, "/me/add?url=Example%2FRules"), site.signedInPost(t, "/me/add?repository=Example%2FRules")} {
				if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/example/rules" {
					t.Fatalf("answered %d to %q, want the library's page", resp.StatusCode, resp.Header.Get("Location"))
				}
				page := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/example/rules", cookies: []*http.Cookie{cookie(resp, noticeCookie)}}))
				if got, kind := toastText(t, page), noticeToast(t, page); got != "example/rules is already on Rulemart" || kind != "status" {
					t.Errorf("the library's page toasts %q as a %q toast, want a status toast", got, kind)
				}
			}
			if len(site.listings.listed) != 0 {
				t.Errorf("listed %q", site.listings.listed)
			}
		})
	}
}

// Adding lists the repository for the signed-in visitor and follows its check.
func TestAddingListsTheRepositoryAndFollowsItsCheck(t *testing.T) {
	site := newListingSite(t)

	resp := site.signedInPost(t, "/me/add?repository=example%2Fnew")

	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/me/add/run?repo=example%2Fnew" {
		t.Fatalf("answered %d to %q, want a redirect to the check", resp.StatusCode, resp.Header.Get("Location"))
	}
	if !slices.Equal(site.listings.listed, []string{"1 example/new"}) {
		t.Fatalf("listed %q", site.listings.listed)
	}
	if !strings.Contains(site.logs.String(), `"msg":"listed library"`) || strings.Contains(site.logs.String(), "example/new") {
		t.Errorf("logged %s; want listed library, without the repository", site.logs)
	}
}

// A listing whose check couldn't be queued still stands, for the hourly poll, and the failure is logged.
func TestAddingStandsWhenItsCheckWasntQueued(t *testing.T) {
	site := newListingSite(t)
	site.listings.notQueued = true

	resp := site.signedInPost(t, "/me/add?repository=example%2Fnew")

	if resp.StatusCode != http.StatusSeeOther || len(site.listings.listed) != 1 {
		t.Fatalf("answered %d and listed %q", resp.StatusCode, site.listings.listed)
	}
	if !strings.Contains(site.logs.String(), `"msg":"listing not queued"`) {
		t.Errorf("logged %s; want listing not queued", site.logs)
	}
}

// Another site can't list a library as the visitor.
func TestListingRefusesARequestAnotherSiteStarted(t *testing.T) {
	site := newListingSite(t)

	resp := send(t, site.handler, request{method: http.MethodPost, target: "/me/add?repository=example%2Fnew",
		cookies: []*http.Cookie{site.session}, header: http.Header{"Sec-Fetch-Site": {"cross-site"}}})

	if resp.StatusCode != http.StatusForbidden || len(site.listings.listed) != 0 {
		t.Fatalf("answered %d and listed %q, want 403 and nothing listed", resp.StatusCode, site.listings.listed)
	}
}

// listingsFor gives octocat's account, 1, one listing in each state.
func listingsFor(site listingSite, requested time.Time) {
	site.listings.byAccount[1] = []views.AccountListing{
		{ID: 4, Owner: "someone", Name: "new", State: domain.ListingChecking, ListedAt: requested, RequestedAt: requested},
		{ID: 3, Owner: "someone", Name: "broken", RepositoryID: "99", State: domain.ListingFailed, Failure: "The repository has no release/<number> tags",
			ListedAt: day(3), RequestedAt: day(3), CheckedAt: day(3)},
		{ID: 2, Owner: "stranger", Name: "rules", RepositoryID: "23", State: domain.ListingListed,
			Library: views.LibraryRef{Owner: "stranger", Name: "rules"}, ListedAt: day(2), RequestedAt: day(2), CheckedAt: day(3)},
		{ID: 1, Owner: "example", Name: "rules", RepositoryID: "7", State: domain.ListingVetted,
			Library: views.LibraryRef{Owner: "example", Name: "rules"}, ListedAt: day(1), RequestedAt: day(1), CheckedAt: day(1)},
	}
}

// My libraries lists the libraries the visitor listed for others under Listed by you, each with its state: being
// checked, saying when it was asked for, failed, saying why, listed and unvetted, or vetted. A listed library links
// with nofollow and a vetted one without; a failed one offers Try again, and each one Remove, which asks first. The
// page doesn't reload itself: a reload would move a reader's focus and place.
func TestMyLibrariesShowsEachListingsState(t *testing.T) {
	site := newListingSite(t)
	listingsFor(site, time.Now().Add(-time.Minute))

	resp := site.signedInGet(t, "/me")

	if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("got %d, cached as %q", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}
	page := body(t, resp)
	assertShows(t, page, "My libraries , 4", "Listed by you · 4",
		"new Checking someone/new · Rulemart is checking it on GitHub. You asked 1 minute ago.",
		"broken Failed someone/broken · The repository has no release/<number> tags · Try again · Remove",
		"rules Unvetted stranger/rules · Listed 2 Sep 2026 · Remove",
		"rules Vetted example/rules · Listed 1 Sep 2026 · Remove")
	if strings.Contains(page, `http-equiv="refresh"`) {
		t.Error("the page reloads itself")
	}
	if strings.Contains(page, "GitHub repository 99") {
		t.Error("the page shows a raw repository ID")
	}
	if got := rels(t, page, "/stranger/rules"); !slices.Equal(got, []string{"nofollow"}) {
		t.Errorf("links the listed library with rel %q, want nofollow", got)
	}
	if got := rels(t, page, "/example/rules"); !slices.Equal(got, []string{""}) {
		t.Errorf("links the vetted library with rel %q, want none", got)
	}
	for _, repo := range []string{"someone/new", "someone/broken"} {
		if got := links(t, page, strings.Split(repo, "/")[1]); !slices.Contains(got, "/me/add/run?repo="+url.QueryEscape(repo)) {
			t.Errorf("%s leads to %q, want its check", repo, got)
		}
	}
	if actions := formActions(t, page); !slices.Equal(actions, []string{"/signout", "/me/listings/retry?listing=3"}) {
		t.Errorf("the page's forms post to %q, want signing out, and only a retry of the failed listing", actions)
	}
	if got := links(t, page, "Remove"); !slices.Equal(got, []string{
		"/me/listings/remove?listing=4", "/me/listings/remove?listing=3", "/me/listings/remove?listing=2", "/me/listings/remove?listing=1",
	}) {
		t.Errorf("the Remove links lead to %q, want each listing's confirmation", got)
	}
	// Try again posts from the row and returns to My libraries, which then shows the check.
	retried := site.signedInPost(t, "/me/listings/retry?listing=3")
	if retried.StatusCode != http.StatusSeeOther || retried.Header.Get("Location") != "/me" || !slices.Equal(site.listings.retried, []int64{3}) {
		t.Errorf("Try again answered %d to %q, retrying %v", retried.StatusCode, retried.Header.Get("Location"), site.listings.retried)
	}
}

// A listing checked for a few minutes is taking longer than usual: its row says the hourly poll checks it.
func TestMyLibrariesSaysWhenACheckIsTakingLonger(t *testing.T) {
	site := newListingSite(t)
	listingsFor(site, time.Now().Add(-5*time.Minute))

	assertShows(t, body(t, site.signedInGet(t, "/me")),
		"new Checking someone/new · Taking longer than usual: Rulemart checks it again within the hour. You asked 5 minutes ago.")
}

// At the account's limit, the add page leads to My libraries, where the visitor removes a listing.
func TestAddPageLeadsToMyLibrariesAtTheListingLimit(t *testing.T) {
	site := newListingSite(t)
	site.listings.refusal = app.ErrAccountListingLimit

	page := body(t, site.signedInGet(t, "/me/add?url=someone%2Fnew"))

	if got := links(t, page, "My libraries"); !slices.Equal(got, []string{"/me"}) {
		t.Errorf("My libraries leads to %q", got)
	}
}

// Without listings for others, My libraries has no Listed by you.
func TestMyLibrariesHasNoListedByYouWithoutListings(t *testing.T) {
	site := newListingSite(t)

	if text := visibleText(t, body(t, site.signedInGet(t, "/me"))); strings.Contains(text, "Listed by you") {
		t.Errorf("My libraries offers Listed by you without listings:\n%s", text)
	}
}

// The listings page's addresses, now and before, lead to My libraries, which holds the listings, and no page links
// them.
func TestTheListingsPageRedirectsToMyLibraries(t *testing.T) {
	site := newListingSite(t)
	listingsFor(site, time.Now())

	for _, target := range []string{"/me/listings", "/account/listings"} {
		for _, cookies := range [][]*http.Cookie{nil, {site.session}} {
			resp := send(t, site.handler, request{method: http.MethodGet, target: target, cookies: cookies})
			if resp.StatusCode != http.StatusMovedPermanently || resp.Header.Get("Location") != "/me" {
				t.Errorf("%s answered %d to %q, want 301 to /me", target, resp.StatusCode, resp.Header.Get("Location"))
			}
		}
	}
	for _, target := range []string{"/me", "/me?tab=account", "/me/add", "/me/add/run?repo=someone%2Fbroken", "/me/listings/remove?listing=3", "/libraries"} {
		doc, err := html.Parse(strings.NewReader(body(t, site.signedInGet(t, target))))
		if err != nil {
			t.Fatal(err)
		}
		if a := find(doc, func(n *html.Node) bool { return n.Data == "a" && attribute(n, "href") == "/me/listings" }); a != nil {
			t.Errorf("%s links the listings page: %q", target, nodeText(a))
		}
	}
}

// Removing a listing asks first, saying what removing does for its state, then removes it with an empty POST.
func TestRemovingAListingAsksFirstAndSaysWhatItDoes(t *testing.T) {
	site := newListingSite(t)
	listingsFor(site, time.Now())

	for id, want := range map[string]string{
		"2": "Its library leaves Rulemart: its pages stop showing, and links to them stop working.",
		"1": "Rulemart has vetted its library, so the library stays on Rulemart. Only your listing goes.",
		"3": "Rulemart forgets it, and it stops taking one of your places.",
		"4": "Rulemart stops checking it, so its library won't show on Rulemart",
	} {
		resp := site.signedInGet(t, "/me/listings/remove?listing="+id)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("listing %s: got %d", id, resp.StatusCode)
		}
		page := body(t, resp)
		assertShows(t, page, "Remove this listing?", want)
		if got := formActions(t, page); !slices.Contains(got, "/me/listings/remove?listing="+id) {
			t.Errorf("listing %s: the page's forms post to %q", id, got)
		}
		if got := links(t, page, "Keep it"); !slices.Equal(got, []string{"/me"}) {
			t.Errorf("listing %s: Keep it leads to %q, want My libraries", id, got)
		}
	}
	if len(site.listings.removed) != 0 {
		t.Fatalf("asking removed %v", site.listings.removed)
	}
	for _, target := range []string{"/me/listings/remove?listing=99", "/me/listings/remove?listing=x"} {
		if resp := site.signedInGet(t, target); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s answered %d, want 404", target, resp.StatusCode)
		}
	}
}

// Removing and trying again act on the visitor's own listing, and return to My libraries, saying so; trying a
// listing again that isn't failing, from a page left open, says so too.
func TestRemovingAndRetryingActOnTheVisitorsOwnListing(t *testing.T) {
	site := newListingSite(t)
	listingsFor(site, time.Now())

	for _, test := range []struct{ target, notice string }{
		{"/me/listings/remove?listing=2", "listing-removed-listed"},
		{"/me/listings/remove?listing=1", "listing-removed-vetted"},
		{"/me/listings/remove?listing=4", "listing-removed-checking"},
		{"/me/listings/remove?listing=3", "listing-removed"},
		{"/me/listings/retry?listing=3", "listing-retried"},
		{"/me/listings/retry?listing=4", "listing-not-failed"},
	} {
		resp := site.signedInPost(t, test.target)
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/me" {
			t.Fatalf("%s answered %d to %q", test.target, resp.StatusCode, resp.Header.Get("Location"))
		}
		if got := cookie(resp, noticeCookie); got == nil || got.Value != test.notice {
			t.Errorf("%s set the notice %v, want %s", test.target, got, test.notice)
		}
	}
	// Only the run page is a page to return to other than My libraries.
	for _, back := range []string{"/browse/techs", "https://evil.example/me/add/run", "/me/add/run/x"} {
		resp := site.signedInPost(t, "/me/listings/retry?"+url.Values{"listing": {"4"}, "return": {back}}.Encode())
		if got := resp.Header.Get("Location"); got != "/me" {
			t.Errorf("retrying with the return %q returned to %q, want /me", back, got)
		}
	}
	if !slices.Equal(site.listings.removed, []int64{2, 1, 4, 3}) || !slices.Equal(site.listings.retried, []int64{3}) {
		t.Fatalf("removed %v and retried %v", site.listings.removed, site.listings.retried)
	}
	for _, target := range []string{
		"/me/listings/remove?listing=99", "/me/listings/remove?listing=x", "/me/listings/remove",
	} {
		resp := site.signedInPost(t, target)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s answered %d, want 404", target, resp.StatusCode)
		}
		assertShows(t, body(t, resp), "You have no such listing. It may have been removed already. My libraries")
	}
	for _, target := range []string{"/me/listings/remove?listing=3", "/me/listings/retry?listing=3"} {
		signedOut := send(t, site.handler, request{method: http.MethodPost, target: target})
		want := "/signin?" + url.Values{"return": {"/me"}}.Encode()
		if signedOut.StatusCode != http.StatusSeeOther || signedOut.Header.Get("Location") != want {
			t.Errorf("%s signed out answered %d to %q, want sign-in", target, signedOut.StatusCode, signedOut.Header.Get("Location"))
		}
	}
	if len(site.listings.removed) != 4 || len(site.listings.retried) != 1 {
		t.Fatalf("signed out, removed %v and retried %v", site.listings.removed, site.listings.retried)
	}
}

// The account menu leads to adding a library, and the libraries page to adding one, only where listing is available.
func TestListingIsOfferedOnlyWhereItsAvailable(t *testing.T) {
	site := newListingSite(t)
	page := body(t, site.signedInGet(t, "/libraries"))
	if got := links(t, page, "Add a library"); !slices.Equal(got, []string{"/me/add"}) {
		t.Errorf("the menu links adding a library at %q", got)
	}
	if got := links(t, page, "list a public library"); !slices.Equal(got, []string{"/me/add"}) {
		t.Errorf("the libraries page links listing at %q", got)
	}

	without := newAccountsSite(t, nil)
	token := without.accounts.signedIn(t, accounts.Identity{GitHubUserID: 2, Login: "hubot"})
	page = body(t, send(t, without.handler, request{method: http.MethodGet, target: "/libraries",
		cookies: []*http.Cookie{{Name: sessionCookie, Value: string(token)}}}))
	if strings.Contains(page, "Add a library") || len(links(t, page, "list a public library")) > 0 {
		t.Error("a site without listings offers them")
	}
	if resp := send(t, without.handler, request{method: http.MethodGet, target: "/me/add"}); resp.StatusCode != http.StatusNotFound {
		t.Errorf("/me/add answered %d without listings, want 404", resp.StatusCode)
	}
}

// A failure to read listings fails the page, as any read does.
func TestAddPageFailsWhenListingsCantBeRead(t *testing.T) {
	site := newListingSite(t)
	site.listings.refusal = errors.New("connection refused")

	if resp := site.signedInGet(t, "/me/add?url=example%2Fnew"); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503", resp.StatusCode)
	}
}

// newSiteAt returns the pages' handler, reading from c, at the base URL base.
func newSiteAt(t *testing.T, c web.Catalog, base string) http.Handler {
	t.Helper()
	baseURL, err := web.ParseBaseURL(base)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := web.New(c, web.Options{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), BaseURL: baseURL})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

// A visitor who asked for too many checks is told when to try again, rather than retrying.
func TestRetryingSaysWhenAVisitorAskedTooOften(t *testing.T) {
	site := newListingSite(t)
	listingsFor(site, time.Now())
	site.listings.retryRefusal = fmt.Errorf("retry listing id=3: %w", app.ErrListingTooOften)

	resp := site.signedInPost(t, "/me/listings/retry?listing=3")

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("answered %d, want 429", resp.StatusCode)
	}
	assertShows(t, body(t, resp), "Try again later", "Try again tomorrow.")
}
