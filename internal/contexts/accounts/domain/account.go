// Package domain holds the language and rules of accounts: who a visitor is once they sign in with GitHub, and the
// sessions that keep them signed in. It reads no network or database itself.
package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Identity is a GitHub user, as GitHub describes them when they sign in. NewIdentity makes one from what GitHub
// reports.
type Identity struct {
	// GitHubUserID is GitHub's numeric ID for the user, which identifies them across renames.
	GitHubUserID int64
	// Login is the user's GitHub login at sign-in, which they can change.
	Login string
	// AvatarURL is the user's avatar on GitHub's avatar host, or empty to show the login's initial.
	AvatarURL string
}

// githubLogin matches what GitHub allows in a login: letters, digits, and hyphens, at most 39 of them. Older logins
// may have hyphens where new ones can't, so it doesn't check where they fall.
var githubLogin = regexp.MustCompile(`^[A-Za-z0-9-]{1,39}$`)

// githubAvatarHost is the only host an avatar may be on: the pages' content security policy allows images from it.
const githubAvatarHost = "https://avatars.githubusercontent.com/"

// NewIdentity returns the GitHub user id, named login, with the avatar at avatarURL. It refuses an ID that isn't
// positive or a login GitHub wouldn't give, and drops an avatar that isn't on GitHub's avatar host, so the account
// shows its initial instead.
func NewIdentity(id int64, login, avatarURL string) (Identity, error) {
	if id <= 0 {
		return Identity{}, fmt.Errorf("check GitHub user login=%q: want a positive user ID, got %d", login, id)
	}
	if !githubLogin.MatchString(login) {
		return Identity{}, fmt.Errorf("check GitHub user id=%d: login %q isn't one GitHub gives", id, login)
	}
	if !strings.HasPrefix(avatarURL, githubAvatarHost) || strings.ContainsAny(avatarURL, " \"'<>\\") {
		avatarURL = ""
	}
	return Identity{GitHubUserID: id, Login: login, AvatarURL: avatarURL}, nil
}

// Account is a visitor who has signed in, as Rulemart stores them.
type Account struct {
	// ID is Rulemart's own ID for the account, which other tables reference.
	ID int64
	Identity
	// CreatedAt is when the account first signed in.
	CreatedAt time.Time
}

// ProfileURL returns the account's GitHub profile, by the login it last signed in with.
func (a Account) ProfileURL() string { return "https://github.com/" + a.Login }
