package web_test

import (
	"cmp"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/platform/web"
	"golang.org/x/net/html"
)

// Every page's footer leads to Fabrica, to what Rulemart is, the feedback page, and how Rulemart treats visitors'
// data, in that order, and, by GitHub's mark, to Rulemart's source. It doesn't repeat the header's sections, and the
// about page, not the footer, leads to Code Rules.
func TestEveryPagesFooterLeadsToAboutFeedbackPrivacyAndSource(t *testing.T) {
	handler := newSite(t, unvettedCatalog())
	want := []string{
		"Fabrica https://fabricahq.com",
		"About /about",
		"Feedback /feedback",
		"Privacy /privacy",
		"Source on GitHub https://github.com/fabricahq/rulemart",
	}
	for _, path := range []string{"/", library, unvettedLibrary, "/search?q=errors", "/example/missing", "/about", "/about/vetting", "/privacy", "/faq", "/feedback"} {
		page := get(t, handler, path).Body.String()
		doc, err := html.Parse(strings.NewReader(page[strings.Index(page, "<footer"):]))
		if err != nil {
			t.Fatal(err)
		}
		// Each link is named by its text, or, drawn as an icon, by its label.
		var got []string
		for n := range doc.Descendants() {
			if n.Type == html.ElementNode && n.Data == "a" {
				got = append(got, cmp.Or(attribute(n, "aria-label"), nodeText(n))+" "+attribute(n, "href"))
			}
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s: the footer links %q, want %q", path, got, want)
		}
	}
}

// The about page says, in about one screen, why Rulemart exists, what Code Rules is, and that Rulemart is a visual
// interface over its libraries, and leads to the vetting page for the rest. It has an address of its
// own for search engines.
func TestAboutPageSaysWhatRulemartAndCodeRulesAreAndLeadsToVetting(t *testing.T) {
	handler := newSiteAt(t, newCatalog(), "https://rulemart.example")

	resp := get(t, handler, "/about")

	if resp.Code != http.StatusOK {
		t.Fatalf("answered %d", resp.Code)
	}
	page := resp.Body.String()
	assertShows(t, page, "About", "About Rulemart")
	want := []string{
		"Why. Everyone writes software with agents now, and agents need guidance to write it well. Most teams give " +
			"that guidance through skills, which are coarse: hard to version, hard to share across teams, and " +
			"all-or-nothing when something changes.",
		"Rulemart exists so teams can share what they've learned about writing good code as rules, inside their " +
			"organization or with everyone. Adopt the rules that fit your project, in whatever subset you want. When a " +
			"coding session goes wrong, turn the retro into a rule. The feedback loop gets shorter, within a team and " +
			"across the community, and more code comes out right the first time.",
		"A rule in an agent's context doesn't guarantee it follows every rule to the letter; you still validate the " +
			"work. But an agent told exactly how you want code written is far more likely to write it that way.",
		"What. The mechanism is Code Rules, Fabrica's open-source package manager for engineering rules you give to " +
			"AI. Rules are Markdown files that live in a project or are published in versioned libraries on GitHub; a " +
			"project imports the ones it wants and updates them when it chooses.",
		"Teams needed a way to browse those rules, see what changed, and pick which to adopt. Rulemart is that: a " +
			"visual interface over Code Rules libraries. Collect rules in a cart and check out with a prompt for your " +
			"coding agent, or the commands to run yourself. The libraries it shows by default are vetted by Fabrica; " +
			"read how vetting works.",
		"Rulemart is free and open source on GitHub. We hope you find it useful, we welcome thoughtful " +
			"contributions, and we especially want your feedback.",
	}
	if got := proseParagraphs(t, page); !slices.Equal(got, want) {
		t.Errorf("the body is\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := canonicalLinks(t, page); !slices.Equal(got, []string{"https://rulemart.example/about"}) {
		t.Errorf("names %q as canonical", got)
	}
	for text, want := range map[string]string{
		"Code Rules":        "https://code-rules.fabricahq.com",
		"Fabrica":           "https://fabricahq.com",
		"how vetting works": "/about/vetting",
		"on GitHub":         "https://github.com/fabricahq/rulemart",
		"your feedback":     "/feedback",
	} {
		if got := links(t, page, text); !slices.Contains(got, want) {
			t.Errorf("%s leads to %q, want %s", text, got, want)
		}
	}
}

// proseParagraphs returns the text of each paragraph in a page's prose, its body below the title, with its runs of
// whitespace collapsed to one space.
func proseParagraphs(t *testing.T, body string) []string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var paragraphs []string
	for n := range doc.Descendants() {
		if n.Type == html.ElementNode && slices.Contains(strings.Fields(attribute(n, "class")), "prose") {
			for p := range n.Descendants() {
				if p.Type == html.ElementNode && p.Data == "p" {
					paragraphs = append(paragraphs, strings.Join(strings.Fields(visibleTextOf(p)), " "))
				}
			}
		}
	}
	return paragraphs
}

// The vetting page says what vetting means, as the opt-in every list offers, what an unvetted library is, how a
// library gets vetted, and where to report a problem, and has an address of its own for search engines. Where listing
// isn't available, it names listing a library without leading to the form.
func TestVettingPageExplainsVettingAndHowToGetALibraryVetted(t *testing.T) {
	handler := newSiteAt(t, newCatalog(), "https://rulemart.example")

	resp := get(t, handler, "/about/vetting")

	if resp.Code != http.StatusOK {
		t.Fatalf("answered %d", resp.Code)
	}
	page := resp.Body.String()
	assertShows(t, page, "About", "Library vetting", "What vetting means", "Unvetted libraries", "Get a library vetted",
		"Report a problem", "Include unvetted libraries", unvettedWarning, "List it on Rulemart")
	if got := canonicalLinks(t, page); !slices.Equal(got, []string{"https://rulemart.example/about/vetting"}) {
		t.Errorf("names %q as canonical", got)
	}
	if got := links(t, page, "catalog/vetted.yaml"); !slices.Equal(got, []string{"https://github.com/fabricahq/rulemart/blob/main/catalog/vetted.yaml"}) {
		t.Errorf("vetted.yaml leads to %q", got)
	}
	if got := links(t, page, "unvetted libraries"); !slices.Equal(got, []string{"/unvetted"}) {
		t.Errorf("unvetted libraries leads to %q", got)
	}
	if got := links(t, page, "Ask to vet a library"); len(got) != 1 || !strings.Contains(got[0], "template=ask-to-vet-a-library.yml") {
		t.Errorf("asking to vet leads to %q", got)
	}
	if got := links(t, page, "report a problem"); !slices.Equal(got, []string{"https://github.com/fabricahq/rulemart/issues/new/choose"}) {
		t.Errorf("reporting a problem leads to %q", got)
	}
	if got := links(t, page, "List it on Rulemart"); len(got) != 0 {
		t.Errorf("without listing, List it on Rulemart leads to %q", got)
	}
}

// Where listing is available, the vetting page's steps lead to the listing form.
func TestVettingPageLeadsToListingWhenListingIsAvailable(t *testing.T) {
	site := newListingSite(t)

	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/about/vetting"}))

	if got := links(t, page, "List it on Rulemart"); !slices.Equal(got, []string{"/me/add"}) {
		t.Errorf("List it on Rulemart leads to %q", got)
	}
}

// The privacy page says what Rulemart keeps, for how long, and who else sees it, and it says Rulemart uses no
// analytics until a token turns Cloudflare's on, when it names them.
func TestPrivacyPageSaysWhatRulemartKeepsAndWhetherItCountsVisits(t *testing.T) {
	without := get(t, newSite(t, newCatalog()), "/privacy")
	with := get(t, newSiteWith(t, web.Options{AnalyticsToken: "0123456789abcdef0123456789abcdef"}), "/privacy")

	for _, resp := range []*httptestResponse{{without.Code, without.Body.String()}, {with.Code, with.Body.String()}} {
		if resp.code != http.StatusOK {
			t.Fatalf("answered %d", resp.code)
		}
		assertShows(t, resp.body, "GitHub user ID", "__Host-rulemart-session", "30 days", "IP address", "180 days",
			"Delete your account", "6 hours", "Amazon Web Services", "Neon",
			"A library's page shows the username you last signed in with as who added it, while your listing stands.",
			"the organizations you belong to", "GitHub gives Rulemart a token for your account", "encrypted with a key that only its server holds",
			"Signing out deletes the token along with the session", "whether you sign out of this browser or everywhere",
			"Rulemart is run by Fabrica", "What Rulemart reads from GitHub", "the names of your organizations",
			"at most 200 of them, most recently pushed first", ".code-rules/generated/provenance.json",
			"which rule versions it holds", "Rulemart keeps nothing else about your repositories, and none of their code",
			"No other visitor ever sees any information about your private repositories",
			"It never reads their code", "until you delete your account",
			"what it read of your GitHub account, and its record of the GitHub App's installations",
			"only when you act: when you sign in, press Refresh, or return from installing the app",
			"reads fail until the suspension is lifted", "It never reads in the background",
			"What it did read stays until a later read replaces it")
		// The GitHub App's webhook isn't wired at launch, so nothing discards a snapshot when an installation changes on
		// GitHub, and opening the dashboard shows the snapshot Rulemart keeps without reading GitHub again.
		for _, promise := range []string{"installation on GitHub discards it", "when you open your dashboard"} {
			if strings.Contains(visibleText(t, resp.body), promise) {
				t.Errorf("the page promises %q", promise)
			}
		}
	}
	assertShows(t, without.Body.String(), "Rulemart uses no analytics service.")
	if strings.Contains(visibleText(t, without.Body.String()), "Cloudflare Web Analytics") {
		t.Error("without a token, the page names Cloudflare Web Analytics")
	}
	assertShows(t, with.Body.String(), "Cloudflare Web Analytics", "sets no cookie", "on every page, signed in or not")
}

// httptestResponse is a response's status and body, for tests that check two at once.
type httptestResponse struct {
	code int
	body string
}

// A library's pages, vetted or not, lead to a report about it, with the library filled in, on GitHub.
func TestLibraryPagesLeadToAReportAboutTheLibrary(t *testing.T) {
	handler := newSite(t, unvettedCatalog())

	for path, want := range map[string]string{library: "example/rules", unvettedLibrary: "stranger/rules"} {
		got := links(t, get(t, handler, path).Body.String(), "Report this library")
		if len(got) != 1 {
			t.Errorf("%s: %d report links", path, len(got))
			continue
		}
		u, err := url.Parse(got[0])
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		if u.Host != "github.com" || u.Path != "/fabricahq/rulemart/issues/new" || q.Get("template") != "report-a-library.yml" ||
			q.Get("library") != want || q.Get("title") != "Report: "+want {
			t.Errorf("%s: reports at %s", path, got[0])
		}
	}
}

// The sign-in page says what Rulemart keeps of a GitHub account, and leads to the whole of it.
func TestSignInPageLeadsToPrivacy(t *testing.T) {
	site := newAccountsSite(t, nil)

	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/signin"}))

	if got := links(t, page, "How Rulemart treats your data"); !slices.Equal(got, []string{"/privacy"}) {
		t.Errorf("leads to %q", got)
	}
}
