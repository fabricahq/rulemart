package web_test

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
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

// starButton returns the attributes of the page's star button, or nil when it has none.
func starButton(t *testing.T, page string) map[string]string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	for n := range doc.Descendants() {
		if n.Type == html.ElementNode && n.Data == "button" && attribute(n, "id") == "star" {
			attrs := map[string]string{}
			for _, a := range n.Attr {
				attrs[a.Key] = a.Val
			}
			return attrs
		}
	}
	return nil
}

// The star button says whether it's pressed. Starring or unstarring returns to the page saying what it did, with
// the button focused, so keyboard and screen reader users land where they were, and hear its new state.
func TestStarringSaysWhatItDidAndKeepsFocusOnTheButton(t *testing.T) {
	site := newStarSite(t)

	if button := starButton(t, body(t, site.signedInGet(t, library))); button["aria-pressed"] != "false" {
		t.Fatalf("an unstarred library's button: %v, want aria-pressed false", button)
	}
	page := body(t, site.follow(t, site.signedInPost(t, starPath)))
	assertShows(t, page, "You starred this library. It's on Your stars.")
	if button := starButton(t, page); button["aria-pressed"] != "true" || !hasKey(button, "autofocus") {
		t.Fatalf("after starring, the button: %v, want aria-pressed true and autofocus", button)
	}
	if hasKey(starButton(t, body(t, site.signedInGet(t, library))), "autofocus") {
		t.Error("a later view of the page still focuses the button")
	}
	page = body(t, site.follow(t, site.signedInPost(t, unstarPath)))
	assertShows(t, page, "You unstarred this library.")
	if button := starButton(t, page); button["aria-pressed"] != "false" || !hasKey(button, "autofocus") {
		t.Fatalf("after unstarring, the button: %v, want aria-pressed false and autofocus", button)
	}
}

func hasKey(m map[string]string, key string) bool {
	_, ok := m[key]
	return ok
}

// A visitor who isn't signed in hears that Star leads to signing in.
func TestTheSignedOutStarLinkSaysItSignsIn(t *testing.T) {
	site := newStarSite(t)

	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: library}))
	if got := accessibleNames(t, page, "/sign-in?return=%2Fexample%2Frules%3Fstar"); !slices.Equal(got, []string{"Sign in to star example/rules, 3 stars"}) {
		t.Errorf("the star link is named %q", got)
	}
}

// Signing in from a star link returns to the library with the button focused and highlighted, and a one-time prompt
// to star it, which the next view of the page doesn't repeat. Nothing stars it without a click.
func TestSigningInToStarPromptsOnceToStar(t *testing.T) {
	site := newStarSite(t)

	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: library + "?tab=rules"}))
	want := "/sign-in?" + url.Values{"return": {library + "?tab=rules&star=1"}, "to": {"star"}}.Encode()
	if got := links(t, page, "Star"); !slices.Equal(got, []string{want}) {
		t.Fatalf("Star leads to %q, want %q", got, want)
	}

	back := site.signedInGet(t, library+"?tab=rules&star=1")
	if back.StatusCode != http.StatusSeeOther || back.Header.Get("Location") != library+"?tab=rules" {
		t.Fatalf("returning answered %d to %q, want a redirect to the page without star", back.StatusCode, back.Header.Get("Location"))
	}
	page = body(t, site.follow(t, back))
	assertShows(t, page, "You're signed in. Star example/rules?")
	if button := starButton(t, page); button["aria-pressed"] != "false" || !hasKey(button, "autofocus") || !hasKey(button, "data-prompt") {
		t.Fatalf("the prompted button: %v, want unpressed, focused, and highlighted", button)
	}
	if got := site.stars.all(); len(got) != 0 {
		t.Fatalf("returning starred %q without a click", got)
	}
	if page := body(t, site.signedInGet(t, library+"?tab=rules")); strings.Contains(page, "Star example/rules?") {
		t.Error("the next view of the page prompts again")
	}

	site.signedInPost(t, starPath)
	page = body(t, site.follow(t, site.signedInGet(t, library+"?star=1")))
	assertShows(t, page, "You're signed in. You've starred this library already.")
	if button := starButton(t, page); strings.Contains(page, "Star example/rules?") || hasKey(button, "data-prompt") || hasKey(button, "autofocus") {
		t.Error("the page prompts to star, or focuses the button for, a library the visitor starred already")
	}

	signedOut := send(t, site.handler, request{method: http.MethodGet, target: library + "?star=1"})
	if signedOut.StatusCode != http.StatusMovedPermanently || signedOut.Header.Get("Location") != library {
		t.Errorf("signed out, ?star=1 answered %d to %q, want a redirect to the page", signedOut.StatusCode, signedOut.Header.Get("Location"))
	}
}

// A star whose return a visitor tampered with returns to the library's page; unstarring a library Rulemart doesn't
// have is missing, as starring one is; and the sign-in page promises a return only to a page it will return to.
func TestTamperedStarsFallBackClearly(t *testing.T) {
	site := newStarSite(t)

	for _, back := range []string{"//evil.example", "https://evil.example/", `/\evil.example`, "/sign-in"} {
		resp := site.signedInPost(t, starPath+"&"+url.Values{"return": {back}}.Encode())
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != library {
			t.Errorf("return %q: answered %d to %q, want the library's page", back, resp.StatusCode, resp.Header.Get("Location"))
		}
	}
	for _, target := range []string{"/account/stars/remove?library=nobody%2Fnothing", "/account/stars?library=nobody%2Fnothing"} {
		resp := site.signedInPost(t, target)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: got %d, want 404", target, resp.StatusCode)
		}
		assertShows(t, body(t, resp), "Rulemart has no library named nobody/nothing")
	}
	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/sign-in?return=%2F%2Fevil.example&to=star"}))
	assertShows(t, page, "Sign in to star libraries.")
	if strings.Contains(page, "come back") {
		t.Error("the sign-in page promises a return it won't make")
	}
}

// When a signed-in visitor starred a library, lists show its star filled, saying so to screen readers. Lists never
// show a count of 0; a library's own page always shows its count.
func TestListsShowTheVisitorsOwnStars(t *testing.T) {
	site := newStarSite(t)

	before := body(t, site.signedInGet(t, "/libraries"))
	if strings.Contains(visibleText(t, before), "starred by you") {
		t.Fatal("the list says the visitor starred a library they didn't")
	}
	site.signedInPost(t, starPath)
	for _, path := range []string{"/", "/libraries"} {
		assertShows(t, body(t, site.signedInGet(t, path)), "3 stars, starred by you")
	}
}

// The stars page names what each star is for, and says when each was starred, readably to the minute on hover.
func TestTheStarsPageSaysWhenEachStarWasMade(t *testing.T) {
	site := newStarSite(t)
	starred := time.Now().Add(-5 * time.Minute)
	site.stars.listed[octocatID] = []views.StarredLibrary{
		{Library: views.LibraryCard{Owner: "example", Name: "rules", Rules: 2, Stars: 3}, Vetted: true, StarredAt: starred},
	}

	page := body(t, site.signedInGet(t, "/account/stars"))
	assertShows(t, page, "Starred 5 minutes ago", "others see only how many stars each library has")
	if want := `title="` + starred.UTC().Format("2 Jan 2006, 15:04") + ` UTC"`; !strings.Contains(page, want) {
		t.Errorf("the time's hover text isn't %s", want)
	}
}

// Unstarring on the stars page keeps the library named at the top, with Star again focused; starring it again
// focuses its Unstar. Only the visitor's own unstarring names a library there: an address can't.
func TestTheStarsPageKeepsWhatWasUnstarredWithAWayBack(t *testing.T) {
	site := newStarSite(t)
	site.stars.listed[octocatID] = nil

	resp := site.signedInPost(t, unstarPath+"&return=%2Faccount%2Fstars")
	if resp.Header.Get("Location") != "/account/stars" {
		t.Fatalf("unstarring from the stars page returns to %q, want /account/stars", resp.Header.Get("Location"))
	}
	page := body(t, site.follow(t, resp))
	assertShows(t, page, "You unstarred example/rules.")
	if strings.Contains(visibleText(t, page), "most recent first") {
		t.Error("an empty stars page explains its order")
	}
	assertShows(t, page, "Browse libraries")
	again := buttonNamed(t, page, "Star example/rules again")
	if again == nil || !hasKey(again, "autofocus") {
		t.Fatalf("Star again: %v, want it focused", again)
	}
	if got := formActions(t, page); !slices.Contains(got, starPath+"&return=%2Faccount%2Fstars") {
		t.Errorf("the page's forms post to %q, want Star again", got)
	}
	if again := buttonNamed(t, body(t, site.signedInGet(t, "/account/stars")), "Star example/rules again"); again != nil {
		t.Error("the next view of the stars page still offers Star again")
	}

	delete(site.stars.listed, octocatID)
	resp = site.signedInPost(t, starPath+"&return=%2Faccount%2Fstars")
	page = body(t, site.follow(t, resp))
	if unstar := buttonNamed(t, page, "Unstar example/rules"); unstar == nil || !hasKey(unstar, "autofocus") {
		t.Fatalf("after starring again, Unstar: %v, want it focused", unstar)
	}

	for _, target := range []string{
		"/account/stars?unstarred=evil-site%2Fyour-account-was-compromised-visit-evil.example",
		"/account/stars?unstarred=example%2Frules",
	} {
		if page := body(t, site.signedInGet(t, target)); strings.Contains(page, "You unstarred") {
			t.Errorf("%s says the visitor unstarred something", target)
		}
	}
}

// buttonNamed returns the attributes of the button in page whose aria-label is name, or nil.
func buttonNamed(t *testing.T, page, name string) map[string]string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	for n := range doc.Descendants() {
		if n.Type == html.ElementNode && n.Data == "button" && attribute(n, "aria-label") == name {
			attrs := map[string]string{}
			for _, a := range n.Attr {
				attrs[a.Key] = a.Val
			}
			return attrs
		}
	}
	return nil
}
