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
	checked := 0
	for pattern, path := range everyRoute(t) {
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

// everyRoute returns the path of every route of a server with every option, by its pattern.
func everyRoute(t *testing.T) map[string]string {
	t.Helper()
	s, err := newServer(nil, Options{
		Accounts: struct{ Accounts }{}, Listings: struct{ Listings }{}, Stars: struct{ Stars }{}, Cart: struct{ Cart }{},
	})
	if err != nil {
		t.Fatal(err)
	}
	s.handler()
	paths := map[string]string{}
	for pattern := range s.routes {
		_, path, ok := strings.Cut(pattern, " ")
		if !ok {
			path = pattern
		}
		paths[pattern] = path
	}
	return paths
}

// A route under a segment a GitHub login could spell, such as /browse/techs, takes the address of a library's page
// when it has two segments, and of a rule's when it could have five or more, since a rule's ID has at least three.
// Each library page the site's routes take must be one libraryPageTaken names, which siteSections documents, so the
// sitemap leaves it out; no route may take a rule's page. Otherwise a new route would silently hide a library's page.
func TestEveryRouteUnderALoginTakesOnlyTheLibraryPagesItDocuments(t *testing.T) {
	checked := 0
	for pattern, path := range everyRoute(t) {
		segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
		if len(segments) < 2 || !gitHubLogin.MatchString(segments[0]) {
			continue
		}
		checked++
		last := segments[len(segments)-1]
		if len(segments) >= 5 || (strings.HasPrefix(last, "{") && strings.HasSuffix(last, "...}")) {
			t.Errorf("the route %q hides the pages of rules of libraries owned by %q", pattern, segments[0])
		}
		if len(segments) != 2 {
			continue
		}
		name := segments[1]
		if strings.HasPrefix(name, "{") {
			// A wildcard takes the page of every library the owner names, such as one named rules.
			name = "rules"
		}
		if !libraryPageTaken(segments[0], name) {
			t.Errorf("the route %q hides the page of the library %s/%s: name it in libraryPageTaken and siteSections", pattern, segments[0], name)
		}
	}
	if checked == 0 {
		t.Fatal("found no route under a login to check")
	}
}
