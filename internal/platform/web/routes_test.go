package web

import (
	"regexp"
	"strings"
	"testing"
)

// gitHubLogin matches a segment a GitHub login could spell: letters, digits, and hyphens. No owner's page could be
// at /robots.txt, /sitemap.xml, or /favicon.ico, since no login holds a dot.
var gitHubLogin = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

// A route of one literal segment, such as /faq, takes that address from the owner whose login it spells, so
// reservedOwner must reserve the segment, which moves the owner's page to /o/{login}. Otherwise a new one-segment
// route would silently hide an owner's page, and every link to it.
func TestEveryOneSegmentRouteIsReservedFromOwners(t *testing.T) {
	s, err := newServer(nil, Options{
		Accounts: struct{ Accounts }{}, Listings: struct{ Listings }{}, Stars: struct{ Stars }{}, Cart: struct{ Cart }{},
	})
	if err != nil {
		t.Fatal(err)
	}
	s.handler()

	checked := 0
	for pattern := range s.routes {
		path := pattern
		if _, withoutMethod, ok := strings.Cut(pattern, " "); ok {
			path = withoutMethod
		}
		segment := strings.TrimPrefix(path, "/")
		if strings.ContainsAny(segment, "/{") || !gitHubLogin.MatchString(segment) {
			continue
		}
		checked++
		if !reservedOwner(segment) {
			t.Errorf("the route %q hides the page of the owner %q: reserve it next to siteSections", pattern, segment)
		}
	}
	if checked == 0 {
		t.Fatal("found no one-segment route to check")
	}
}
