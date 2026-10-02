package web_test

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/platform/web"
)

// Every page's footer leads to the techs, the practices, the libraries, and the FAQ, which a phone's header hides,
// and to what Rulemart is, how it treats visitors' data, Code Rules, Rulemart's source, and the feedback page.
func TestEveryPagesFooterLeadsToTheSectionsAboutPrivacySourceAndFeedback(t *testing.T) {
	handler := newSite(t, unvettedCatalog())
	want := map[string]string{
		"Techs":            "/browse/techs",
		"Practices":        "/browse/practices",
		"Libraries":        "/libraries",
		"FAQ":              "/faq",
		"About Rulemart":   "/about",
		"Privacy":          "/privacy",
		"About Code Rules": "https://code-rules.fabricahq.com",
		"Source on GitHub": "https://github.com/fabricahq/rulemart",
		"Give us feedback": "/feedback",
	}
	for _, path := range []string{"/", library, unvettedLibrary, "/search?q=errors", "/example/missing", "/about", "/privacy", "/faq", "/feedback"} {
		page := get(t, handler, path).Body.String()
		footer := page[strings.Index(page, "<footer"):]
		for text, href := range want {
			if got := links(t, footer, text); !slices.Contains(got, href) {
				t.Errorf("%s: the footer's %q leads to %q, want %s", path, text, got, href)
			}
		}
	}
}

// The about page says what Rulemart is, what vetting means, and how a library gets vetted, and has an address of its
// own for search engines.
func TestAboutPageExplainsVettingAndHowToGetALibraryVetted(t *testing.T) {
	handler := newSiteAt(t, newCatalog(), "https://rulemart.example")

	resp := get(t, handler, "/about")

	if resp.Code != http.StatusOK {
		t.Fatalf("answered %d", resp.Code)
	}
	page := resp.Body.String()
	assertShows(t, page, "About Rulemart", "What vetting means", "Unvetted libraries", "Get a library vetted",
		"Report a problem")
	if got := canonicalLinks(t, page); !slices.Equal(got, []string{"https://rulemart.example/about"}) {
		t.Errorf("names %q as canonical", got)
	}
	if got := links(t, page, "catalog/vetted.yaml"); !slices.Equal(got, []string{"https://github.com/fabricahq/rulemart/blob/main/catalog/vetted.yaml"}) {
		t.Errorf("vetted.yaml leads to %q", got)
	}
	if got := links(t, page, "Ask to vet a library"); len(got) != 1 || !strings.Contains(got[0], "template=ask-to-vet-a-library.yml") {
		t.Errorf("asking to vet leads to %q", got)
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
			"Delete your account", "6 hours", "Amazon Web Services", "Neon")
	}
	assertShows(t, without.Body.String(), "Rulemart uses no analytics service.")
	if strings.Contains(visibleText(t, without.Body.String()), "Cloudflare Web Analytics") {
		t.Error("without a token, the page names Cloudflare Web Analytics")
	}
	assertShows(t, with.Body.String(), "Cloudflare Web Analytics", "sets no cookie")
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

	page := body(t, send(t, site.handler, request{method: http.MethodGet, target: "/sign-in"}))

	if got := links(t, page, "How Rulemart treats your data"); !slices.Equal(got, []string{"/privacy"}) {
		t.Errorf("leads to %q", got)
	}
}
