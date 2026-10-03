//go:build rulemartdev

// Sign in as a test user, without GitHub, in a local build. Only a build with the rulemartdev tag compiles this file,
// as make web-dev does; release builds never set the tag, so their web function has no such route, and
// cmd/web's tests check the release build's binary for it.

package web

import (
	"net/http"
	"net/url"

	"github.com/fabricahq/rulemart/internal/contexts/accounts/github/githubtest"
)

// DevSignIn reports whether this build lets visitors sign in as a test user. Only a rulemartdev build does.
const DevSignIn = true

// devSignInPattern is the dev sign-in's route: POST, with the test user's login in the as parameter and where to
// return in the return parameter. GitHub has no account named account, so it can't hide a library's page.
const devSignInPattern = "POST " + accountHref + "/dev-sign-in"

// testUsers are who a local build can sign in as: the users its fake GitHub knows.
var testUsers = githubtest.DevUsers()

// isTestUser reports whether the GitHub user ID gitHubUserID is a test user's.
func isTestUser(gitHubUserID int64) bool {
	for _, user := range testUsers {
		if user.GitHubUserID == gitHubUserID {
			return true
		}
	}
	return false
}

// registerDevSignIn adds the dev sign-in's route through handle.
func (s *server) registerDevSignIn(handle func(pattern string, handler http.HandlerFunc)) {
	handle(devSignInPattern, s.devSignIn)
}

// devSignIn signs the visitor in as the test user the as parameter names, as GitHub's callback signs in a GitHub
// user, with the token the local build's fake GitHub knows them by, and returns them to the return parameter.
func (s *server) devSignIn(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	for _, user := range testUsers {
		if user.Login == query.Get("as") {
			s.signIn(w, r, user, githubtest.DevToken(user.Login), signInReturn(query.Get("return")))
			return
		}
	}
	s.notFound(w, r)
}

// testUserViews returns a form for signing in as each test user, returning to back.
func testUserViews(back string) []testUserView {
	views := make([]testUserView, len(testUsers))
	for i, user := range testUsers {
		views[i] = testUserView{login: user.Login, action: accountHref + "/dev-sign-in?" + url.Values{"as": {user.Login}, "return": {back}}.Encode()}
	}
	return views
}
