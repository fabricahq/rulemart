package web_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/fabricahq/rulemart/internal/platform/web"
)

// newSiteWith returns the pages' handler, reading from newBrowsingCatalog, with options and a log that discards.
func newSiteWith(t *testing.T, options web.Options) http.Handler {
	t.Helper()
	options.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	handler, err := web.New(newBrowsingCatalog(), options)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

// policy returns the sources each directive of resp's content security policy allows, by directive.
func policy(resp *httptest.ResponseRecorder) map[string][]string {
	directives := map[string][]string{}
	for directive := range strings.SplitSeq(resp.Header().Get("Content-Security-Policy"), ";") {
		if fields := strings.Fields(directive); len(fields) > 0 {
			directives[fields[0]] = fields[1:]
		}
	}
	return directives
}

// Every response, a page, a static file, a missing page, or a redirect, carries the headers that keep browsers on
// HTTPS, off other sites' frames, and away from features Rulemart never uses.
func TestEveryResponseCarriesTheSecurityHeaders(t *testing.T) {
	handler := newSiteWith(t, web.Options{})
	stylesheet := regexp.MustCompile(`href="(/_static/[0-9a-f]+/generated/app\.css)"`).FindStringSubmatch(get(t, handler, "/").Body.String())
	if stylesheet == nil {
		t.Fatal("the page links no stylesheet")
	}
	want := map[string]string{
		"Strict-Transport-Security":  "max-age=31536000; includeSubDomains",
		"X-Content-Type-Options":     "nosniff",
		"Referrer-Policy":            "strict-origin-when-cross-origin",
		"Cross-Origin-Opener-Policy": "same-origin",
	}

	for _, path := range []string{"/", library, stylesheet[1], "/example/missing", "/browse/techs/"} {
		resp := get(t, handler, path)
		for name, value := range want {
			if got := resp.Header().Get(name); got != value {
				t.Errorf("%s: %s is %q, want %q", path, name, got, value)
			}
		}
		if got := policy(resp)["frame-ancestors"]; !slices.Equal(got, []string{"'none'"}) {
			t.Errorf("%s: frame-ancestors allows %q, want only 'none'", path, got)
		}
		permissions := resp.Header().Get("Permissions-Policy")
		for _, feature := range []string{"camera=()", "microphone=()", "geolocation=()", "payment=()", "browsing-topics=()"} {
			if !strings.Contains(permissions, feature) {
				t.Errorf("%s: Permissions-Policy %q doesn't deny %s", path, permissions, feature)
			}
		}
	}
}

// Without a token, pages load no analytics, and the policy allows no other site's script, and connections only to
// Rulemart, which the cart's checkout makes.
func TestPagesLoadNoAnalyticsWithoutAToken(t *testing.T) {
	resp := get(t, newSiteWith(t, web.Options{}), "/")

	if strings.Contains(resp.Body.String(), "cloudflareinsights") {
		t.Error("the page mentions Cloudflare's analytics without a token")
	}
	directives := policy(resp)
	if got := directives["script-src"]; !slices.Equal(got, []string{"'self'"}) {
		t.Errorf("script-src allows %q, want only 'self'", got)
	}
	if got := directives["connect-src"]; !slices.Equal(got, []string{"'self'"}) {
		t.Errorf("connect-src allows %q, want only 'self'", got)
	}
}

// With a token, every page, the sign-in page included, loads Cloudflare's beacon with it, and the policy allows that
// script and its reports, and nothing else of Cloudflare's.
func TestPagesLoadCloudflareAnalyticsWithAToken(t *testing.T) {
	const token = "0123456789abcdef0123456789abcdef"
	handler := newSiteWith(t, web.Options{AnalyticsToken: token})

	for _, path := range []string{"/", library, "/example/missing"} {
		resp := get(t, handler, path)
		doc, err := html.Parse(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		beacon := find(doc, func(n *html.Node) bool {
			return n.Data == "script" && attribute(n, "src") == "https://static.cloudflareinsights.com/beacon.min.js"
		})
		if beacon == nil {
			t.Errorf("%s: no Cloudflare beacon", path)
			continue
		}
		if got := attribute(beacon, "data-cf-beacon"); got != `{"token":"`+token+`"}` {
			t.Errorf("%s: the beacon's data is %q", path, got)
		}
		if !hasAttribute(beacon, "defer") {
			t.Errorf("%s: the beacon blocks the page: no defer", path)
		}
		directives := policy(resp)
		if got := directives["script-src"]; !slices.Equal(got, []string{"'self'", "https://static.cloudflareinsights.com"}) {
			t.Errorf("%s: script-src allows %q", path, got)
		}
		if got := directives["connect-src"]; !slices.Equal(got, []string{"'self'", "https://cloudflareinsights.com"}) {
			t.Errorf("%s: connect-src allows %q", path, got)
		}
	}
}

// Signed-in pages load the beacon too, since their addresses, such as /me and /cart, carry nothing about the visitor,
// and the cart's contents never appear in one.
func TestSignedInPagesLoadCloudflareAnalyticsWithAToken(t *testing.T) {
	const token = "0123456789abcdef0123456789abcdef"
	site := newDashboardSiteWith(t, octocatsGitHub(), octocatsCatalog(), func(o *web.Options) { o.AnalyticsToken = token })

	for _, path := range []string{"/me", "/me?tab=stars", "/me/add", "/cart"} {
		doc, err := html.Parse(strings.NewReader(site.get(t, path)))
		if err != nil {
			t.Fatal(err)
		}
		beacon := find(doc, func(n *html.Node) bool {
			return n.Data == "script" && attribute(n, "src") == "https://static.cloudflareinsights.com/beacon.min.js"
		})
		if beacon == nil {
			t.Errorf("%s: no Cloudflare beacon", path)
			continue
		}
		if got := attribute(beacon, "data-cf-beacon"); got != `{"token":"`+token+`"}` {
			t.Errorf("%s: the beacon's data is %q", path, got)
		}
	}
}

// The sign-in page's policy adds GitHub's authorization page to form-action, and keeps the analytics sources.
func TestSignInPolicyKeepsAnalyticsSources(t *testing.T) {
	site := newAccountsSite(t, func(o *web.Options) { o.AnalyticsToken = "0123456789abcdef0123456789abcdef" })

	resp := send(t, site.handler, request{method: http.MethodGet, target: "/signin"})

	recorder := httptest.NewRecorder()
	recorder.Header()["Content-Security-Policy"] = resp.Header.Values("Content-Security-Policy")
	directives := policy(recorder)
	if got := directives["form-action"]; !slices.Equal(got, []string{"'self'", "https://github.com/login/oauth/authorize"}) {
		t.Errorf("form-action allows %q", got)
	}
	if got := directives["connect-src"]; !slices.Equal(got, []string{"'self'", "https://cloudflareinsights.com"}) {
		t.Errorf("connect-src allows %q", got)
	}
}

// A token that couldn't be Cloudflare's stops the site at start, rather than writing it into every page.
func TestNewRefusesAnAnalyticsTokenThatIsntOne(t *testing.T) {
	for _, token := range []string{`abc"}`, "abc def", "<script>", strings.Repeat("a", 129)} {
		_, err := web.New(newBrowsingCatalog(), web.Options{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), AnalyticsToken: token})
		if err == nil {
			t.Errorf("New accepted the analytics token %q", token)
		}
	}
}

// hasAttribute reports whether n has the attribute key, with any value.
func hasAttribute(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}
