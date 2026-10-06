package web_test

import (
	"bytes"
	"cmp"
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
	// listed is what AccountStars returns for each account, and uncounted what UncountedStars does.
	listed    map[int64][]views.StarredRule
	uncounted map[int64][]views.UncountedStar
	// err, when set, fails every call.
	err error
}

func newFakeStars() *fakeStars {
	return &fakeStars{
		current: map[string]bool{"example/rules/techs/go/return-errors": true, "example/rules/practices/testing/verify-retry-limits": true},
		starred: map[string]bool{}, listed: map[int64][]views.StarredRule{}, uncounted: map[int64][]views.UncountedStar{},
	}
}

// change stars or unstars the rule for the account, and reports whether a star is the account's first, as
// catalog/app.Stars does: it had none, and now has this one.
func (f *fakeStars) change(accountID int64, library, rulePath string, star bool) (first bool, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return false, f.err
	}
	key := strings.ToLower(library + "/" + rulePath)
	if !f.current[key] {
		return false, fmt.Errorf("star rule=%q: %w", key, app.ErrNotFound)
	}
	account := fmt.Sprintf("%d ", accountID)
	if !star {
		delete(f.starred, account+key)
		return false, nil
	}
	if f.starred[account+key] {
		return false, nil
	}
	first = true
	for k := range f.starred {
		if strings.HasPrefix(k, account) {
			first = false
			break
		}
	}
	f.starred[account+key] = true
	return first, nil
}

func (f *fakeStars) Star(_ context.Context, accountID int64, library, rulePath string) (bool, error) {
	return f.change(accountID, library, rulePath, true)
}

func (f *fakeStars) Unstar(_ context.Context, accountID int64, library, rulePath string) error {
	_, err := f.change(accountID, library, rulePath, false)
	return err
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

func (f *fakeStars) UncountedStars(_ context.Context, accountID int64) ([]views.UncountedStar, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.uncounted[accountID], f.err
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
	withRowStars := func(rows []views.RuleRow) {
		for i, r := range rows {
			switch r.Rule.Path {
			case "techs/go/return-errors":
				rows[i].Rule.Stars = returnErrorsStars
			case "techs/go/close-bodies":
				rows[i].Rule.Stars = 1
			}
		}
	}
	withRowStars(c.groups["techs/go"].Rules.Rows)
	withRowStars(c.results["errors"].Rows)
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
	// retryStarPath is where verify-retry-limits's Star button posts from its own page.
	retryStarPath = "/stars?library=example%2Frules&rule=practices%2Ftesting%2Fverify-retry-limits"
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
		if n.Type != html.ElementNode || attribute(n, "role") != "status" || inTemplate(n) {
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

// noticeToast returns how the page's notice shows where scripts run, as its data-toast says: status or info for a
// toast, or empty for a banner. It fails t unless the page loads toast.js, which every page does, for the toasts
// scripts show too.
func noticeToast(t *testing.T, page string) string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	kind := ""
	for n := range doc.Descendants() {
		if n.Type == html.ElementNode && attribute(n, "role") == "status" && attribute(n, "data-toast") != "" && !inTemplate(n) {
			kind = attribute(n, "data-toast")
		}
	}
	if !strings.Contains(page, "/toast.js") {
		t.Error("the page doesn't load toast.js")
	}
	return kind
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
		want := "/signin?" + url.Values{"return": {back}, "to": {"star"}}.Encode()
		if got := links(t, page, "Star"); !slices.Equal(got, []string{want}) {
			t.Errorf("%s: Star leads to %q, want %q", path, got, want)
		}
		// The star's sign-in dialog's Continue with GitHub leads there too.
		if got := accessibleNames(t, page, want); !slices.Equal(got, []string{"Sign in to star Return errors with context, 1,234 stars", "Continue with GitHub"}) {
			t.Errorf("%s: the star link is named %q", path, got)
		}
		if postsStar(t, page) {
			t.Errorf("%s: a public page has a star form", path)
		}
	}
	assertShows(t, body(t, send(t, site.handler, request{method: http.MethodGet, target: "/signin?return=%2Fexample%2Frules%2Ftechs%2Fgo%2Freturn-errors&to=star"})),
		"Sign in to star rules. You'll come back to this one.")
	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/signin?return=%2F%2Fevil.example&to=star"}))
	assertShows(t, page, "Sign in to star rules.")
	if strings.Contains(page, "come back") {
		t.Error("the sign-in page promises a return it won't make")
	}
}

// With a script, Star first opens the prototype's dialog, "Sign in to star rules", as star.test.mjs tests. Its Continue
// with GitHub follows the same sign-in path as the link, so the page after signing in still asks to star the rule, and
// its Not now and close buttons close it through their forms' dialog method, which returns focus to Star, without a
// script of their own. A signed-in visitor's page has no such dialog.
func TestARulesStarOpensASignInDialogForAVisitorWhoIsntSignedIn(t *testing.T) {
	site := newStarSite(t)
	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: errorsRule}))
	doc := parsePage(t, page)

	want := "/signin?" + url.Values{"return": {errorsRule + "?star=1"}, "to": {"star"}}.Encode()
	opener := find(doc, func(n *html.Node) bool { return n.Data == "a" && hasAttribute(n, "data-star-signin") })
	if opener == nil || attribute(opener, "href") != want {
		t.Fatalf("no Star link opens the dialog with the sign-in path %q", want)
	}
	dialog := find(doc, func(n *html.Node) bool { return n.Data == "dialog" && hasAttribute(n, "data-star-dialog") })
	if dialog == nil {
		t.Fatal("the page has no star sign-in dialog")
	}
	assertShows(t, visibleTextOf(dialog), "Sign in to star rules", "Star this rule if you find it useful.", "Continue with GitHub", "Not now")
	continueLink := find(dialog, func(n *html.Node) bool { return n.Data == "a" && strings.Contains(nodeText(n), "Continue with GitHub") })
	if continueLink == nil || attribute(continueLink, "href") != want {
		t.Errorf("Continue with GitHub doesn't lead to %q", want)
	}
	var closers []string
	for n := range dialog.Descendants() {
		if n.Type == html.ElementNode && n.Data == "button" && n.Parent != nil && n.Parent.Data == "form" && attribute(n.Parent, "method") == "dialog" {
			closers = append(closers, strings.TrimSpace(cmp.Or(attribute(n, "aria-label"), nodeText(n))))
		}
	}
	if !slices.Equal(closers, []string{"Close", "Not now"}) {
		t.Errorf("the dialog's buttons that close it are %q, want Close and Not now", closers)
	}
	if !loadsScript(doc, "star.js") {
		t.Error("the page doesn't load the script that opens the dialog")
	}

	signedIn := parsePage(t, body(t, site.signedInGet(t, errorsRule)))
	if find(signedIn, func(n *html.Node) bool { return n.Data == "dialog" && hasAttribute(n, "data-star-dialog") }) != nil || loadsScript(signedIn, "star.js") {
		t.Error("a signed-in visitor's page has the star sign-in dialog")
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

// A visitor's first star says where their starred rules are on the page it returns to; a later star, and unstarring,
// say nothing, since the button's own Starred or Star says what happened. Each focuses the button, marked as focused
// by the page, which draws its quiet ring, so keyboard and screen reader users land where they were and hear its new
// state. A later view of the page does neither.
func TestOnlyTheFirstStarSaysWhereStarredRulesAreAndEachFocusesTheButton(t *testing.T) {
	site := newStarSite(t)
	focusedAfter := func(what, page, pressed string) {
		t.Helper()
		if button := starButton(t, page); button["aria-pressed"] != pressed || !hasKey(button, "autofocus") || !hasKey(button, "data-autofocused") {
			t.Errorf("after %s, the button: %v, want aria-pressed %s, autofocus, and data-autofocused", what, button, pressed)
		}
	}

	page := body(t, site.follow(t, site.signedInPost(t, starPath)))
	if text, links := pageNotice(t, page); text != "You starred your first rule! 🎉 Find all your starred rules under Starred rules." ||
		!slices.Equal(links, []string{"Starred rules /me?tab=stars"}) {
		t.Errorf("after the first star, the notice says %q and links %q, want Starred rules linked to the list", text, links)
	}
	// Where scripts run, it's an info toast, with a link to follow, and a button that closes it.
	if kind := noticeToast(t, page); kind != "info" || !strings.Contains(page, `aria-label="Dismiss" data-toast-close hidden`) {
		t.Errorf("the first star's notice is a %q toast, want an info toast with a hidden close button", kind)
	}
	focusedAfter("the first star", page, "true")
	page = body(t, site.signedInGet(t, errorsRule))
	if text, _ := pageNotice(t, page); text != "" {
		t.Errorf("a later view of the page says %q", text)
	}
	if button := starButton(t, page); hasKey(button, "autofocus") || hasKey(button, "data-autofocused") {
		t.Errorf("a later view of the page still focuses the button: %v", button)
	}

	page = body(t, site.follow(t, site.signedInPost(t, retryStarPath)))
	if text, _ := pageNotice(t, page); text != "" {
		t.Errorf("a second star says %q, want nothing", text)
	}
	focusedAfter("a second star", page, "true")

	page = body(t, site.follow(t, site.signedInPost(t, unstarPath)))
	if text, _ := pageNotice(t, page); text != "" {
		t.Errorf("unstarring says %q, want nothing", text)
	}
	focusedAfter("unstarring", page, "false")
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
	// It asks the visitor to use the button it highlights, so it stays a banner where scripts run.
	if kind := noticeToast(t, page); kind != "" {
		t.Errorf("the prompt is a %q toast, want a banner", kind)
	}
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

// A rule page's address with the star prompt's parameter redirects once to the same address without it, however the
// parameter's name is encoded and however often it appears, keeping the rest of the query in order, and the redirect
// leads to the page.
func TestTheStarPromptRedirectsOnceToTheAddressWithoutIt(t *testing.T) {
	site := newStarSite(t)

	for target, want := range map[string]string{
		errorsRule + "?%73tar=1":       errorsRule,
		errorsRule + "?star=1&star=2":  errorsRule,
		errorsRule + "?a=1&star=1&b=2": errorsRule + "?a=1&b=2",
	} {
		assertRedirectsToPage(t, site.handler, target, want)
	}
}

// The star prompt's redirect keeps a rule page's path as the visitor's browser spelled it, so a path with an encoded
// letter leads to the page, as it does without the prompt.
func TestTheStarPromptKeepsAnEncodedPath(t *testing.T) {
	site := newStarSite(t)
	encodedRule := "/%65xample/rules/techs/go/return-errors"

	assertRedirectsToPage(t, site.handler, encodedRule+"?tab=versions&star=1", encodedRule+"?tab=versions")
}

// A visitor who isn't signed in, such as one whose session ended in another tab, is sent to sign in and return to
// where they were, and nothing changes.
func TestStarringSignedOutSignsInAndReturnsToThePage(t *testing.T) {
	site := newStarSite(t)

	for target, want := range map[string]string{
		starPath: "/signin?" + url.Values{"return": {errorsRule + "?star=1"}, "to": {"star"}}.Encode(),
		starPath + "&return=%2Fexample%2Frules%2Ftechs%2Fgo%2Freturn-errors%3Ftab%3Dversions": "/signin?" + url.Values{"return": {errorsRule + "?tab=versions&star=1"}, "to": {"star"}}.Encode(),
		unstarPath: "/signin?" + url.Values{"return": {errorsRule}}.Encode(),
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

	for _, back := range []string{"//evil.example", "https://evil.example/", `/\evil.example`, "/signin"} {
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
		library + "?tab=rules": {"Return errors with context HIGH example/rules 1,234 1,234 stars"},
		"/g/techs/go":          {"Return errors with context HIGH example/rules 1,234 1,234 stars", "Close response bodies MEDIUM other/go-rules 1 1 star"},
		"/search?q=errors":     {"Return errors with context HIGH example/rules 1,234 1,234 stars"},
	} {
		page := body(t, send(t, site.handler, request{method: http.MethodGet, target: path}))
		assertShows(t, page, want...)
		if path == library+"?tab=rules" && strings.Contains(visibleText(t, page), "Verify retry limits HIGH example/rules 0") {
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

	signedOut := send(t, site.handler, request{method: http.MethodGet, target: "/me?tab=stars"})
	if want := "/signin?return=%2Fme%3Ftab%3Dstars"; signedOut.StatusCode != http.StatusSeeOther || signedOut.Header.Get("Location") != want {
		t.Fatalf("signed out: answered %d to %q, want a redirect to %q", signedOut.StatusCode, signedOut.Header.Get("Location"), want)
	}
	assertShows(t, body(t, send(t, site.handler, request{method: http.MethodGet, target: "/signin?return=%2Fme%3Ftab%3Dstars"})),
		"Sign in to see your starred rules.")

	resp := site.signedInGet(t, "/me?tab=stars")
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("got %d, cached as %q", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}
	page := body(t, resp)
	assertShows(t, page, "My libraries , 0 Starred rules , 2 Account",
		"Return errors with context HIGH E example/rules · Go 1,234 1,234 stars "+
			"Verify retry limits HIGH You starred practices/testing/check-retry-backoff , which this rule replaced. "+
			"E example/rules · Testing 1 1 star")
	if content, _ := robots(t, page); content != "noindex" {
		t.Errorf("robots %q, want noindex", content)
	}
	if got := links(t, page, "Verify retry limits"); !slices.Equal(got, []string{retryRule}) {
		t.Errorf("the starred rule leads to %q, want its page", got)
	}
}

func TestStarredRulesSayWhenThereAreNone(t *testing.T) {
	site := newStarSite(t)

	page := body(t, site.signedInGet(t, "/me?tab=stars"))
	assertShows(t, page, "You haven't starred any rules yet. Star a rule from its page.")
	if strings.Contains(visibleText(t, page), "most recent first") {
		t.Error("an empty list explains its order")
	}
}

// The account menu doesn't lead to Starred rules, a tab of the dashboard, and the account page says that deleting the
// account removes the visitor's stars. Signing out from Starred rules returns home.
func TestTheAccountPageNamesTheVisitorsStars(t *testing.T) {
	site := newStarSite(t)

	if got := links(t, body(t, site.signedInGet(t, "/")), "Starred rules"); len(got) > 0 {
		t.Errorf("the menu's Starred rules leads to %q", got)
	}
	page := body(t, site.signedInGet(t, "/me?tab=account"))
	assertShows(t, page, "It removes your stars, your listings")
	signedOut := site.signedInPost(t, "/signout?return=%2Fme%3Ftab%3Dstars")
	if signedOut.Header.Get("Location") != "/" {
		t.Errorf("signing out of Starred rules returns to %q, want home", signedOut.Header.Get("Location"))
	}
}

// A failure to read whether the visitor starred a rule fails the page, rather than offering a star they have.
func TestAFailedStarReadFailsThePage(t *testing.T) {
	site := newStarSite(t)
	site.stars.err = errors.New("the database is down")

	for _, path := range []string{errorsRule, "/me?tab=stars"} {
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
	if page := body(t, site.signedInGet(t, "/me?tab=stars")); strings.Contains(page, "Starred rules") || !strings.Contains(page, "Libraries you and your organizations publish") {
		t.Error("without stars, the dashboard offers Starred rules")
	}
}

// A star that counts toward no rule Rulemart shows, which R3 kept but neither counted nor listed, is listed under No
// longer counted, saying why, with Unstar, which removes it and returns to Starred rules.
func TestStarredRulesListStarsThatNoLongerCount(t *testing.T) {
	site := newStarSite(t)
	site.stars.uncounted[octocatID] = []views.UncountedStar{
		{Library: views.LibraryRef{Owner: "example", Name: "rules"}, Vetted: true, Path: "practices/testing/retry-forever",
			Title: "Retry forever", Retired: true, StarredAt: day(1)},
		{Library: views.LibraryRef{Owner: "gone", Name: "rules"}, Path: "techs/go/old", StarredAt: day(1)},
	}

	page := body(t, site.signedInGet(t, "/me?tab=stars"))

	assertShows(t, page, "No longer counted · 2",
		"Retry forever example/rules · practices/testing/retry-forever · Its library retired it without a replacement. Unstar",
		"techs/go/old gone/rules · techs/go/old · Its library is no longer on Rulemart. Unstar")
	want := "/stars/remove?" + url.Values{"library": {"example/rules"}, "return": {"/me?tab=stars"}, "rule": {"practices/testing/retry-forever"}}.Encode()
	if got := formActions(t, page); !slices.Contains(got, want) {
		t.Errorf("the page's forms post to %q, want %q", got, want)
	}
	if got := links(t, page, "techs/go/old"); len(got) != 0 {
		t.Errorf("a rule of a library no longer on Rulemart links %q", got)
	}
}

// inTemplate reports whether n is inside a <template>, which the page doesn't show until a script uses it, such as
// the toast a script shows.
func inTemplate(n *html.Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode && p.Data == "template" {
			return true
		}
	}
	return false
}
