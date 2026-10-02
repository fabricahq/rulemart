// The headers every response carries: the content security policy, with Cloudflare's analytics when a token turns
// them on, HTTPS only, and no frames, sniffing, or browser features Rulemart doesn't use.

package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// gitHubAuthorization is GitHub's OAuth authorization page, where the sign-in form's POST redirects.
const gitHubAuthorization = "https://github.com/login/oauth/authorize"

// Cloudflare Web Analytics' beacon: the script pages load, and where it reports page views. Manual setup, for a site
// Cloudflare doesn't proxy, as Rulemart, needs both in the content security policy.
const (
	analyticsScript   = "https://static.cloudflareinsights.com/beacon.min.js"
	analyticsOrigin   = "https://static.cloudflareinsights.com"
	analyticsReceiver = "https://cloudflareinsights.com"
)

// analyticsToken is the shape of a Cloudflare Web Analytics site token: letters and digits, as the dashboard shows
// it, or a hyphen or underscore, so a token can't end the attribute or the JSON it sits in.
var analyticsToken = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// policies are the content security policies a site sends: one for every page, and one for the sign-in page, whose
// GitHub form posts here to be redirected to GitHub's authorization page. Browsers check a form's redirects against
// form-action too, so only that page allows GitHub's.
type policies struct {
	page, signIn string
}

// newPolicies returns the policies for a site, with Cloudflare's analytics when analytics is true. Each allows only
// Rulemart's own files, images from GitHub's avatar and raw file hosts, which rules and library owners use, and
// forms that submit to Rulemart, such as search. Rule content comes from repositories Rulemart doesn't control, so
// nothing else may load or run, and no page may be framed.
func newPolicies(analytics bool) policies {
	build := func(formAction string) string {
		scripts, connect := "'self'", ""
		if analytics {
			scripts += " " + analyticsOrigin
			connect = "connect-src " + analyticsReceiver + "; "
		}
		return "default-src 'none'; script-src " + scripts + "; style-src 'self'; font-src 'self'; " +
			"img-src 'self' https://avatars.githubusercontent.com https://raw.githubusercontent.com; " + connect +
			"base-uri 'none'; form-action " + formAction + "; frame-ancestors 'none'"
	}
	return policies{page: build("'self'"), signIn: build("'self' " + gitHubAuthorization)}
}

// analyticsBeacon returns the data-cf-beacon attribute that names token to Cloudflare's beacon, or empty when token is
// empty, which leaves analytics off.
func analyticsBeacon(token string) (string, error) {
	if token == "" {
		return "", nil
	}
	if !analyticsToken.MatchString(token) {
		return "", fmt.Errorf("want a Cloudflare Web Analytics site token of up to 128 letters, digits, hyphens, or underscores, not %q", token)
	}
	data, err := json.Marshal(struct {
		Token string `json:"token"`
	}{token})
	if err != nil {
		return "", fmt.Errorf("encode the analytics beacon's data: %v", err)
	}
	return string(data), nil
}

// strictTransportSecurity keeps browsers on HTTPS for a year once they've visited, on Rulemart's domain and any name
// under it. CloudFront already redirects HTTP to HTTPS; this spares the redirect, and the unencrypted request before
// it. It leaves out preload, which would commit the domain for good, while the domain may still change. Browsers ignore
// it over plain HTTP, as locally.
const strictTransportSecurity = "max-age=31536000; includeSubDomains"

// permissionsPolicy turns off the browser features a page could ask for that Rulemart never uses, so neither its pages
// nor a library's content can, and opts out of interest-based advertising's topics.
var permissionsPolicy = strings.Join([]string{
	"accelerometer=()", "browsing-topics=()", "camera=()", "display-capture=()", "geolocation=()", "gyroscope=()",
	"magnetometer=()", "microphone=()", "midi=()", "payment=()", "publickey-credentials-get=()", "usb=()",
}, ", ")

// withSecurityHeaders adds the headers every response carries, with the content security policy for every page.
func withSecurityHeaders(policy string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("Content-Security-Policy", policy)
		header.Set("Strict-Transport-Security", strictTransportSecurity)
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		header.Set("Permissions-Policy", permissionsPolicy)
		// Pages open no other window, so none can reach Rulemart's through window.opener, nor Rulemart another's.
		header.Set("Cross-Origin-Opener-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}
