//go:build !rulemartdev

// Release builds have no dev sign-in: dev_sign_in.go, which a rulemartdev build compiles instead, holds it.

package web

import "net/http"

// DevSignIn reports whether this build lets visitors sign in as a test user. Only a rulemartdev build does.
const DevSignIn = false

// registerDevSignIn adds no route: a release build has no dev sign-in.
func (s *server) registerDevSignIn(func(pattern string, handler http.HandlerFunc)) {}

// testUserViews returns no test users: a release build has none.
func testUserViews(string) []testUserView { return nil }

// isTestUser reports that no one is a test user: a release build has none.
func isTestUser(int64) bool { return false }
