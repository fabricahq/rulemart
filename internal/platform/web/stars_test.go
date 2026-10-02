package web_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
	"github.com/fabricahq/rulemart/internal/platform/web"
)

// fakeStars keeps stars in memory, as catalog/app.Stars does in Postgres: only a rule in current can be starred.
type fakeStars struct {
	mu sync.Mutex
	// current names the rules that can be starred, as lowercase owner/name/rule ID.
	current map[string]bool
	// starred holds each account's stars, as "<account> <lowercase owner/name/rule ID>".
	starred map[string]bool
	// listed is what AccountStars returns for each account.
	listed map[int64][]views.StarredRule
	// err, when set, fails every call.
	err error
}

func newFakeStars() *fakeStars {
	return &fakeStars{
		current: map[string]bool{"example/rules/techs/go/return-errors": true, "example/rules/practices/testing/verify-retry-limits": true},
		starred: map[string]bool{}, listed: map[int64][]views.StarredRule{},
	}
}

func (f *fakeStars) change(accountID int64, library, rulePath string, star bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	key := strings.ToLower(library + "/" + rulePath)
	if !f.current[key] {
		return fmt.Errorf("star rule=%q: %w", key, app.ErrNotFound)
	}
	if star {
		f.starred[fmt.Sprintf("%d %s", accountID, key)] = true
	} else {
		delete(f.starred, fmt.Sprintf("%d %s", accountID, key))
	}
	return nil
}

func (f *fakeStars) Star(_ context.Context, accountID int64, library, rulePath string) error {
	return f.change(accountID, library, rulePath, true)
}

func (f *fakeStars) Unstar(_ context.Context, accountID int64, library, rulePath string) error {
	return f.change(accountID, library, rulePath, false)
}

func (f *fakeStars) Starred(_ context.Context, accountID int64, owner, name, rulePath string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.starred[fmt.Sprintf("%d %s", accountID, strings.ToLower(owner+"/"+name+"/"+rulePath))], f.err
}

func (f *fakeStars) AccountStars(_ context.Context, accountID int64) ([]views.StarredRule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listed[accountID], f.err
}

// all returns the stars every account holds, sorted.
func (f *fakeStars) all() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Sorted(maps.Keys(f.starred))
}

// starSite is the pages of unvettedCatalog, with a group's page and search results, and 1,234 stars on
// example/rules's return-errors, with sign-in and stars through fakes, and a signed-in visitor's session cookie.
type starSite struct {
	listingSite
	stars *fakeStars
}

func newStarSite(t *testing.T) starSite {
	t.Helper()
	return newStarSiteWith(t, func(*web.Options) {})
}

// returnErrorsStars is how many stars the catalog counts on example/rules's return-errors.
const returnErrorsStars = 1234

// newStarCatalog returns unvettedCatalog with the browsing catalog's group page and search results, the rule
// check-retry-backoff, retired by release 3, and returnErrorsStars on return-errors, wherever the catalog lists it.
func newStarCatalog() catalog {
	c := unvettedCatalog()
	browsing := newBrowsingCatalog()
	c.groups, c.results = browsing.groups, browsing.results
	withStars := func(cards []views.RuleCard) {
		for i, card := range cards {
			if card.Path == "techs/go/return-errors" {
				cards[i].Stars = returnErrorsStars
			}
		}
	}
	for key, page := range c.rules {
		if page.Library.Vetted && page.Rule.Path == "techs/go/return-errors" {
			page.Rule.Stars = returnErrorsStars
			c.rules[key] = page
		}
	}
	for key, comparison := range c.ruleComparisons {
		if comparison.Page.Library.Vetted && comparison.Page.Rule.Path == "techs/go/return-errors" {
			comparison.Page.Rule.Stars = returnErrorsStars
			c.ruleComparisons[key] = comparison
		}
	}
	withStars(c.pages["example/rules"].Rules)
	for _, lib := range c.groups["techs/go"].Libraries {
		if lib.Library.Owner == "example" {
			withStars(lib.Rules)
		} else {
			lib.Rules[0].Stars = 1
		}
	}
	for i, r := range c.results["errors"].Results {
		if r.Library.Owner == "example" {
			c.results["errors"].Results[i].Rule.Stars = returnErrorsStars
		}
	}
	retired := c.rules["example/rules/"+retryRuleID]
	retired.Rule.Path, retired.Rule.Title = "practices/testing/check-retry-backoff", "Check retry backoff"
	retired.Rule.Retirement = &views.Retirement{Release: 3, RetiredAt: day(3), Summaries: []string{"Merge it."}}
	c.rules["example/rules/practices/testing/check-retry-backoff"] = retired
	return c
}

// newStarSiteWith returns a starSite whose options adjust changes.
func newStarSiteWith(t *testing.T, adjust func(*web.Options)) starSite {
	t.Helper()
	stars := newFakeStars()
	accounts := newFakeAccounts()
	site := accountsSite{accounts: accounts, gitHub: &fakeGitHub{identity: octocat}, logs: &bytes.Buffer{}}
	options := web.Options{
		Log: slog.New(slog.NewJSONHandler(site.logs, nil)), Accounts: accounts, GitHub: site.gitHub, Stars: stars,
		Listings: &fakeListings{byAccount: map[int64][]views.AccountListing{}},
	}
	adjust(&options)
	handler, err := web.New(newStarCatalog(), options)
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

const (
	// retryRuleID is the ID of example/rules's verify-retry-limits, which has no stars.
	retryRuleID = "practices/testing/verify-retry-limits"
	// retiredRule is the page of example/rules's check-retry-backoff, which release 3 retired.
	retiredRule = library + "/practices/testing/check-retry-backoff"
	// starPath is where return-errors's Star button posts from its own page, and unstarPath where Starred posts.
	starPath   = "/stars?library=example%2Frules&rule=techs%2Fgo%2Freturn-errors"
	unstarPath = "/stars/remove?library=example%2Frules&rule=techs%2Fgo%2Freturn-errors"
)

// follow requests resp's redirect as the signed-in visitor would, with the notice it set, if any.
func (s starSite) follow(t *testing.T, resp *http.Response) *http.Response {
	t.Helper()
	cookies := []*http.Cookie{s.session}
	if notice := cookie(resp, noticeCookie); notice != nil && notice.MaxAge > 0 {
		cookies = append(cookies, notice)
	}
	return send(t, s.handler, request{method: http.MethodGet, target: resp.Header.Get("Location"), cookies: cookies})
}

// pageNotice returns the text of the page's notice, its text nodes joined as they are, and each link in it as its
// text, a space, and its href.
func pageNotice(t *testing.T, page string) (text string, links []string) {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	for n := range doc.Descendants() {
		if n.Type != html.ElementNode || attribute(n, "role") != "status" {
			continue
		}
		text = accessibleName(n)
		for a := range n.Descendants() {
			if a.Type == html.ElementNode && a.Data == "a" {
				links = append(links, nodeText(a)+" "+attribute(a, "href"))
			}
		}
	}
	return text, links
}

// starButton returns the attributes of the page's star button, or nil when it has none.
func starButton(t *testing.T, page string) map[string]string {
	t.Helper()
	n := starButtonNode(t, page)
	if n == nil {
		return nil
	}
	attrs := map[string]string{}
	for _, a := range n.Attr {
		attrs[a.Key] = a.Val
	}
	return attrs
}

// starButtonName returns the name a screen reader gives the page's star button, or "" when it has none.
func starButtonName(t *testing.T, page string) string {
	t.Helper()
	if n := starButtonNode(t, page); n != nil {
		return accessibleName(n)
	}
	return ""
}

// starButtonNode returns the page's star button, or nil when it has none.
func starButtonNode(t *testing.T, page string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	for n := range doc.Descendants() {
		if n.Type == html.ElementNode && n.Data == "button" && attribute(n, "id") == "star" {
			return n
		}
	}
	return nil
}

func hasKey(m map[string]string, key string) bool {
	_, ok := m[key]
	return ok
}

// postsStar reports whether any form on page posts to star or unstar a rule.
func postsStar(t *testing.T, page string) bool {
	t.Helper()
	return slices.ContainsFunc(formActions(t, page), func(a string) bool { return strings.HasPrefix(a, "/stars") })
}

// A visitor who isn't signed in sees a rule's stars, and a Star link that signs them in and returns them to the same
// page, on every tab and comparison, on a page that's the same for everyone and cached. They hear that it signs in.
func TestARulesPagesOfferAVisitorWhoIsntSignedInToSignInAndStar(t *testing.T) {
	site := newStarSite(t)

	for _, path := range []string{errorsRule, errorsRule + "?tab=versions", errorsRule + "?tab=versions&from=1.0.0&to=2.0.0"} {
		resp := send(t, site.handler, request{method: http.MethodGet, target: path})
		if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != "public, max-age=0, s-maxage=60" {
			t.Fatalf("%s: got %d, cached as %q; want a public page", path, resp.StatusCode, resp.Header.Get("Cache-Control"))
		}
		page := body(t, resp)
		back := path + "?star=1"
		if strings.Contains(path, "?") {
			back = path + "&star=1"
		}
		want := "/sign-in?" + url.Values{"return": {back}, "to": {"star"}}.Encode()
		if got := links(t, page, "Star"); !slices.Equal(got, []string{want}) {
			t.Errorf("%s: Star leads to %q, want %q", path, got, want)
		}
		if got := accessibleNames(t, page, want); !slices.Equal(got, []string{"Sign in to star Return errors with context, 1,234 stars"}) {
			t.Errorf("%s: the star link is named %q", path, got)
		}
		if postsStar(t, page) {
			t.Errorf("%s: a public page has a star form", path)
		}
	}
	assertShows(t, body(t, send(t, site.handler, request{method: http.MethodGet, target: "/sign-in?return=%2Fexample%2Frules%2Ftechs%2Fgo%2Freturn-errors&to=star"})),
		"Sign in to star rules. You'll come back to this one.")
	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/sign-in?return=%2F%2Fevil.example&to=star"}))
	assertShows(t, page, "Sign in to star rules.")
	if strings.Contains(page, "come back") {
		t.Error("the sign-in page promises a return it won't make")
	}
}

// A signed-in visitor's Star button posts to star the rule and return to the same page, and once starred, the
// button says so and posts to unstar it. Their pages are never cached.
func TestASignedInVisitorStarsARuleFromItsPages(t *testing.T) {
	site := newStarSite(t)

	resp := site.signedInGet(t, errorsRule+"?tab=versions")
	if resp.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("a signed-in visitor's page is cached as %q", resp.Header.Get("Cache-Control"))
	}
	page := body(t, resp)
	formAction := "/stars?" + url.Values{
		"library": {"example/rules"}, "rule": {"techs/go/return-errors"}, "return": {errorsRule + "?tab=versions"},
	}.Encode()
	if got := formActions(t, page); !slices.Contains(got, formAction) {
		t.Fatalf("the page's forms post to %q, want %q", got, formAction)
	}
	if got := starButtonName(t, page); got != "Star, 1,234 stars: Return errors with context" {
		t.Errorf("the star button is named %q, want its visible label first, then its stars and the rule", got)
	}
	if button := starButton(t, page); button["aria-pressed"] != "false" {
		t.Fatalf("an unstarred rule's button: %v, want aria-pressed false", button)
	}
	assertShows(t, page, "Star 1,234")

	starred := site.signedInPost(t, formAction)
	if starred.StatusCode != http.StatusSeeOther || starred.Header.Get("Location") != errorsRule+"?tab=versions" ||
		starred.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("starring answered %d to %q, cached as %q", starred.StatusCode, starred.Header.Get("Location"),
			starred.Header.Get("Cache-Control"))
	}
	if got := site.stars.all(); !slices.Equal(got, []string{"1 example/rules/techs/go/return-errors"}) {
		t.Fatalf("stars %q, want octocat's on return-errors", got)
	}

	page = body(t, site.signedInGet(t, errorsRule))
	assertShows(t, page, "Starred 1,234")
	if got := formActions(t, page); !slices.Contains(got, unstarPath) || slices.Contains(got, starPath) {
		t.Fatalf("a starred rule's forms post to %q, want %q and not %q", got, unstarPath, starPath)
	}
	if button := starButton(t, page); button["aria-pressed"] != "true" {
		t.Errorf("a starred rule's button: %v, want aria-pressed true", button)
	}
	if got := starButtonName(t, page); got != "Starred, 1,234 stars: Return errors with context" {
		t.Errorf("a starred rule's button is named %q, want its visible label first", got)
	}
	unstarred := site.signedInPost(t, unstarPath)
	if unstarred.StatusCode != http.StatusSeeOther || unstarred.Header.Get("Location") != errorsRule {
		t.Fatalf("unstarring answered %d to %q, want a redirect to the rule", unstarred.StatusCode, unstarred.Header.Get("Location"))
	}
	if got := site.stars.all(); len(got) != 0 {
		t.Fatalf("stars %q after unstarring", got)
	}
}

// Starring or unstarring twice, as a double click or a reload might, lands where once does.
func TestStarringAndUnstarringTwiceIsLikeOnce(t *testing.T) {
	site := newStarSite(t)

	for range 2 {
		if resp := site.signedInPost(t, starPath); resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != errorsRule {
			t.Fatalf("starring answered %d to %q", resp.StatusCode, resp.Header.Get("Location"))
		}
	}
	if got := site.stars.all(); !slices.Equal(got, []string{"1 example/rules/techs/go/return-errors"}) {
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

// The star button says what starring or unstarring did on the page it returns to, with the button focused, so
// keyboard and screen reader users land where they were, and hear its new state. A later view doesn't repeat it.
func TestStarringSaysWhatItDidAndKeepsFocusOnTheButton(t *testing.T) {
	site := newStarSite(t)

	page := body(t, site.follow(t, site.signedInPost(t, starPath)))
	if text, links := pageNotice(t, page); text != "You starred this rule. It's on your Starred rules." ||
		!slices.Equal(links, []string{"Starred rules /account/stars"}) {
		t.Errorf("the notice says %q and links %q, want Starred rules linked to the list", text, links)
	}
	if button := starButton(t, page); button["aria-pressed"] != "true" || !hasKey(button, "autofocus") {
		t.Fatalf("after starring, the button: %v, want aria-pressed true and autofocus", button)
	}
	if hasKey(starButton(t, body(t, site.signedInGet(t, errorsRule))), "autofocus") {
		t.Error("a later view of the page still focuses the button")
	}
	page = body(t, site.follow(t, site.signedInPost(t, unstarPath)))
	assertShows(t, page, "You unstarred this rule.")
	if button := starButton(t, page); button["aria-pressed"] != "false" || !hasKey(button, "autofocus") {
		t.Fatalf("after unstarring, the button: %v, want aria-pressed false and autofocus", button)
	}
}

// Signing in from a star link returns to the rule with the button focused and highlighted, and a one-time prompt to
// star it, which the next view of the page doesn't repeat. Nothing stars it without a click.
func TestSigningInToStarPromptsOnceToStar(t *testing.T) {
	site := newStarSite(t)

	back := site.signedInGet(t, errorsRule+"?tab=versions&star=1")
	if back.StatusCode != http.StatusSeeOther || back.Header.Get("Location") != errorsRule+"?tab=versions" {
		t.Fatalf("returning answered %d to %q, want a redirect to the page without star", back.StatusCode, back.Header.Get("Location"))
	}
	page := body(t, site.follow(t, back))
	assertShows(t, page, "You're signed in. Star Return errors with context?")
	if button := starButton(t, page); button["aria-pressed"] != "false" || !hasKey(button, "autofocus") || !hasKey(button, "data-prompt") {
		t.Fatalf("the prompted button: %v, want unpressed, focused, and highlighted", button)
	}
	if got := site.stars.all(); len(got) != 0 {
		t.Fatalf("returning starred %q without a click", got)
	}
	if page := body(t, site.signedInGet(t, errorsRule+"?tab=versions")); strings.Contains(page, "Star Return errors with context?") {
		t.Error("the next view of the page prompts again")
	}

	site.signedInPost(t, starPath)
	page = body(t, site.follow(t, site.signedInGet(t, errorsRule+"?star=1")))
	assertShows(t, page, "You're signed in. You've starred this rule already.")
	if button := starButton(t, page); hasKey(button, "data-prompt") || hasKey(button, "autofocus") {
		t.Error("the page prompts to star, or focuses the button for, a rule the visitor starred already")
	}

	signedOut := send(t, site.handler, request{method: http.MethodGet, target: errorsRule + "?star=1"})
	if signedOut.StatusCode != http.StatusMovedPermanently || signedOut.Header.Get("Location") != errorsRule {
		t.Errorf("signed out, ?star=1 answered %d to %q, want a redirect to the page", signedOut.StatusCode, signedOut.Header.Get("Location"))
	}
}

// A visitor who isn't signed in, such as one whose session ended in another tab, is sent to sign in and return to
// where they were, and nothing changes.
func TestStarringSignedOutSignsInAndReturnsToThePage(t *testing.T) {
	site := newStarSite(t)

	for target, want := range map[string]string{
		starPath: "/sign-in?" + url.Values{"return": {errorsRule + "?star=1"}, "to": {"star"}}.Encode(),
		starPath + "&return=%2Fexample%2Frules%2Ftechs%2Fgo%2Freturn-errors%3Ftab%3Dversions": "/sign-in?" + url.Values{"return": {errorsRule + "?tab=versions&star=1"}, "to": {"star"}}.Encode(),
		unstarPath: "/sign-in?" + url.Values{"return": {errorsRule}}.Encode(),
	} {
		resp := send(t, site.handler, request{method: http.MethodPost, target: target})
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != want {
			t.Errorf("%s: answered %d to %q, want a redirect to %q", target, resp.StatusCode, resp.Header.Get("Location"), want)
		}
	}
	if got := site.stars.all(); len(got) != 0 {
		t.Fatalf("stars %q for a visitor who isn't signed in", got)
	}
}

// A star whose return a visitor tampered with returns to the rule's page.
func TestATamperedStarReturnFallsBackToTheRule(t *testing.T) {
	site := newStarSite(t)

	for _, back := range []string{"//evil.example", "https://evil.example/", `/\evil.example`, "/sign-in"} {
		resp := site.signedInPost(t, starPath+"&"+url.Values{"return": {back}}.Encode())
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != errorsRule {
			t.Errorf("return %q: answered %d to %q, want the rule's page", back, resp.StatusCode, resp.Header.Get("Location"))
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

// Only a current rule of a vetted library offers a star: a retired rule's page and an unvetted library's rule pages
// show none, and starring or unstarring one, or a rule Rulemart doesn't have, is missing.
func TestOnlyACurrentRuleOfAVettedLibraryOffersAStar(t *testing.T) {
	site := newStarSite(t)

	for _, get := range []func(*testing.T, string) *http.Response{
		site.signedInGet,
		func(t *testing.T, path string) *http.Response {
			return send(t, site.handler, request{method: http.MethodGet, target: path})
		},
	} {
		for _, path := range []string{retiredRule, unvettedLibrary + "/techs/go/return-errors"} {
			if page := body(t, get(t, path)); starButton(t, page) != nil || strings.Contains(page, "to=star") || postsStar(t, page) {
				t.Errorf("%s offers a star", path)
			}
		}
	}
	for _, target := range []string{
		"/stars?library=stranger%2Frules&rule=techs%2Fgo%2Freturn-errors",
		"/stars?library=example%2Frules&rule=practices%2Ftesting%2Fcheck-retry-backoff",
		"/stars/remove?library=example%2Frules&rule=practices%2Ftesting%2Fcheck-retry-backoff",
		"/stars?library=nobody%2Fnothing&rule=techs%2Fgo%2Freturn-errors",
		"/stars?library=example%2Frules",
		"/stars",
	} {
		resp := site.signedInPost(t, target)
		if resp.StatusCode != http.StatusNotFound || resp.Header.Get("Cache-Control") != "private, no-store" {
			t.Errorf("%s: got %d, cached as %q; want a private 404", target, resp.StatusCode, resp.Header.Get("Cache-Control"))
		}
		assertShows(t, body(t, resp), "Rulemart has no rule here that can be starred.")
	}
	if got := site.stars.all(); len(got) != 0 {
		t.Fatalf("stars %q", got)
	}
}

// Every list of rules shows each rule's stars when it has any, with commas, and in words to screen readers; one with
// none shows nothing. Libraries show no stars of their own, and offer none.
func TestRuleListsShowEachRulesStars(t *testing.T) {
	site := newStarSite(t)

	for path, want := range map[string][]string{
		library + "?tab=rules": {"Return errors with context HIGH 2.0.0 techs/go/return-errors 1,234 1,234 stars"},
		"/g/techs/go":          {"techs/go/return-errors 1,234 1,234 stars", "techs/go/close-bodies 1 1 star"},
		"/search?q=errors":     {"Go 2.0.0 1,234 1,234 stars"},
	} {
		page := body(t, send(t, site.handler, request{method: http.MethodGet, target: path}))
		assertShows(t, page, want...)
		if path == library+"?tab=rules" && strings.Contains(visibleText(t, page), "practices/testing/verify-retry-limits 0") {
			t.Errorf("%s shows a count of 0", path)
		}
	}
	for _, path := range []string{library, library + "?tab=rules", library + "?tab=releases", "/", "/libraries"} {
		for _, get := range []func(*testing.T, string) *http.Response{site.signedInGet, func(t *testing.T, path string) *http.Response {
			return send(t, site.handler, request{method: http.MethodGet, target: path})
		}} {
			page := body(t, get(t, path))
			if strings.Contains(page, "to=star") || postsStar(t, page) || starButton(t, page) != nil {
				t.Errorf("%s offers to star the library", path)
			}
		}
	}
}

// A signed-in visitor's Starred rules list the rules their stars count toward, newest first, as rule rows, saying
// which retired rule they starred in a rule's place. Others are asked to sign in and come back.
func TestStarredRulesListTheVisitorsRules(t *testing.T) {
	site := newStarSite(t)
	site.stars.listed[octocatID] = []views.StarredRule{
		{Library: views.LibraryRef{Owner: "example", Name: "rules"}, Rule: views.RuleCard{Path: "techs/go/return-errors", Group: "techs/go",
			Title: "Return errors with context", Impact: "HIGH", Version: coderules.RuleVersion{Major: 2}, Stars: returnErrorsStars},
			CanonicalGroup: goGroup, StarredAt: day(3)},
		{Library: views.LibraryRef{Owner: "example", Name: "rules"}, Rule: views.RuleCard{Path: retryRuleID, Group: "practices/testing",
			Title: "Verify retry limits", Impact: "HIGH", Version: coderules.RuleVersion{Major: 1, Minor: 1}, Stars: 1},
			CanonicalGroup: testingGroup, StarredAs: "practices/testing/check-retry-backoff", StarredAt: day(2)},
	}

	signedOut := send(t, site.handler, request{method: http.MethodGet, target: "/account/stars"})
	if want := "/sign-in?return=%2Faccount%2Fstars"; signedOut.StatusCode != http.StatusSeeOther || signedOut.Header.Get("Location") != want {
		t.Fatalf("signed out: answered %d to %q, want a redirect to %q", signedOut.StatusCode, signedOut.Header.Get("Location"), want)
	}
	assertShows(t, body(t, send(t, site.handler, request{method: http.MethodGet, target: "/sign-in?return=%2Faccount%2Fstars"})),
		"Sign in to see your starred rules.")

	resp := site.signedInGet(t, "/account/stars")
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("got %d, cached as %q", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}
	page := body(t, resp)
	assertShows(t, page, "Starred rules Rules you starred, most recent first.",
		"Return errors with context HIGH E example/rules:techs/go/return-errors Go 2.0.0 1,234 1,234 stars "+
			"Verify retry limits HIGH You starred it as practices/testing/check-retry-backoff , which it replaced. "+
			"E example/rules:practices/testing/verify-retry-limits Testing 1.1.0 1 1 star")
	if content, _ := robots(t, page); content != "noindex" {
		t.Errorf("robots %q, want noindex", content)
	}
	if got := links(t, page, "Verify retry limits"); !slices.Equal(got, []string{retryRule}) {
		t.Errorf("the starred rule leads to %q, want its page", got)
	}
}

func TestStarredRulesSayWhenThereAreNone(t *testing.T) {
	site := newStarSite(t)

	page := body(t, site.signedInGet(t, "/account/stars"))
	assertShows(t, page, "You haven't starred any rules yet. Star a rule from its page.")
	if strings.Contains(visibleText(t, page), "most recent first") {
		t.Error("an empty list explains its order")
	}
}

// The account menu leads to Starred rules, and the account page says Rulemart keeps which rules the visitor
// starred, and that deleting the account removes them. Signing out from Starred rules returns home.
func TestTheAccountMenuAndPageNameTheVisitorsStars(t *testing.T) {
	site := newStarSite(t)

	if got := links(t, body(t, site.signedInGet(t, "/")), "Starred rules"); !slices.Equal(got, []string{"/account/stars"}) {
		t.Errorf("the menu's Starred rules leads to %q", got)
	}
	page := body(t, site.signedInGet(t, "/account"))
	assertShows(t, page, "it keeps which rules you starred, and when", "It removes your stars, your cart, and your listings")
	if text := visibleText(t, page); strings.Count(text, " also ") > 1 {
		t.Errorf("the account page says also more than once: %s", text)
	}
	signedOut := site.signedInPost(t, "/sign-out?return=%2Faccount%2Fstars")
	if signedOut.Header.Get("Location") != "/" {
		t.Errorf("signing out of Starred rules returns to %q, want home", signedOut.Header.Get("Location"))
	}
}

// A failure to read whether the visitor starred a rule fails the page, rather than offering a star they have.
func TestAFailedStarReadFailsThePage(t *testing.T) {
	site := newStarSite(t)
	site.stars.err = errors.New("the database is down")

	for _, path := range []string{errorsRule, "/account/stars"} {
		if resp := site.signedInGet(t, path); resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("%s: got %d, want 503", path, resp.StatusCode)
		}
	}
	if resp := send(t, site.handler, request{method: http.MethodGet, target: errorsRule}); resp.StatusCode != http.StatusOK {
		t.Fatalf("a visitor who isn't signed in got %d; their page reads no stars of theirs", resp.StatusCode)
	}
}

// Where no one can sign in, or no one can star, a rule's page shows its stars with nothing to click, and Starred
// rules are missing without stars.
func TestWithoutStarringPagesOnlyCountStars(t *testing.T) {
	for name, adjust := range map[string]func(*web.Options){
		"without sign-in": func(o *web.Options) { o.Accounts, o.GitHub = nil, nil },
		"without stars":   func(o *web.Options) { o.Stars = nil },
	} {
		t.Run(name, func(t *testing.T) {
			site := newStarSiteWith(t, adjust)

			page := body(t, send(t, site.handler, request{method: http.MethodGet, target: errorsRule}))
			assertShows(t, page, "1,234 1,234 stars")
			if got := links(t, page, "Star"); len(got) != 0 || starButton(t, page) != nil {
				t.Errorf("Star leads to %q where no one can star", got)
			}
			if page := body(t, send(t, site.handler, request{method: http.MethodGet, target: retryRule})); strings.Contains(visibleText(t, page), "star") {
				t.Error("a rule without stars shows a count")
			}
		})
	}
	site := newStarSiteWith(t, func(o *web.Options) { o.Stars = nil })
	if resp := site.signedInGet(t, "/account/stars"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("Starred rules answered %d without stars, want 404", resp.StatusCode)
	}
}
