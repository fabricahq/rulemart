// The signed-in visitor's dashboard: the libraries they and their organizations publish, the projects that use
// Rulemart's libraries and the updates waiting for them, their starred rules, and their account.

package web

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	accountsapp "github.com/fabricahq/rulemart/internal/contexts/accounts/app"
	accounts "github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// starsTab is the dashboard's tab parameter for Starred rules; without it, the dashboard shows My libraries.
const starsTab = "stars"

// newWithin is how long after a library comes to Rulemart the dashboard marks it New.
const newWithin = 24 * time.Hour

// dashboard shows the signed-in visitor's dashboard, on the tab the tab parameter names, or sends anyone else to sign
// in first.
func (s *server) dashboard(w http.ResponseWriter, r *http.Request) {
	tab := r.URL.Query().Get("tab")
	back := dashboardHref
	if tab == starsTab {
		back = starredHref
	}
	account, ok := s.signedIn(w, r, back)
	if !ok {
		return
	}
	gitHub, ok := s.gitHubView(w, r, account, back)
	if !ok {
		return
	}
	view := dashboardView{account: newAccountView(account), gitHub: gitHub, stars: tab == starsTab && s.starsAvailable(), starsAvailable: s.starsAvailable()}
	if s.Stars != nil {
		starred, err := s.Stars.AccountStars(r.Context(), account.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		view.starCount = len(starred)
		if view.stars {
			uncounted, err := s.Stars.UncountedStars(r.Context(), account.ID)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			view.starred, view.uncounted = newStarredViews(starred), newUncountedViews(uncounted, starredHref)
		}
	}
	if !view.stars {
		libraries, err := s.catalog.Dashboard(r.Context(), gitHub.snapshot.Owners(account.Login), gitHub.snapshot.LibraryNames())
		if err != nil {
			s.fail(w, r, err)
			return
		}
		view.owned, view.used = newOwnedViews(libraries.Owned, time.Now()), newUsedViews(libraries.Imported, gitHub.snapshot)
	}
	s.renderPrivate(w, r, http.StatusOK, dashboardPage(s.chrome, view))
}

// refresh reads the signed-in visitor's GitHub account again, and returns to the return parameter, one of the pages
// that show it, the dashboard by default, which says how the read went.
func (s *server) refresh(w http.ResponseWriter, r *http.Request) {
	back := returnPath(r.URL.Query().Get("return"))
	if path, _, _ := strings.Cut(back, "?"); !signedInPage(path) && path != cartHref {
		back = dashboardHref
	}
	v := visitorOf(r.Context())
	if v.account == nil {
		seeOther(w, r, s.absolute(signInPageHref(back)))
		return
	}
	_, err := s.GitHubAccounts.Refresh(r.Context(), *v.account, v.token)
	switch {
	case errors.Is(err, accountsapp.ErrGitHubRead):
		s.logFailure(r, err)
	case errors.Is(err, accountsapp.ErrNoGitHubToken):
		seeOther(w, r, s.absolute(signInAgainHref(back)))
		return
	case err != nil:
		s.fail(w, r, err)
		return
	}
	seeOther(w, r, back)
}

// gitHubView is what a page that shows the visitor's GitHub account knows of it.
type gitHubView struct {
	// available is false when Rulemart can't read visitors' GitHub accounts, so pages leave them out.
	available bool
	// snapshot is what Rulemart last read, and readAt says when, or is empty when it never read it.
	snapshot accounts.Snapshot
	readAt   string
	// failed is true when the latest read failed.
	failed bool
	// signInAgain is the sign-in page for signing in again, when the session keeps no token GitHub takes, or empty.
	signInAgain string
	// refresh is where the Refresh button posts.
	refresh string
	// private is true when the visitor installed the GitHub App, and privateAvailable when there's an app to install.
	private, privateAvailable bool
	installations             []accounts.Installation
}

// gitHubView returns what the page back knows of the signed-in account's GitHub account, reading it when Rulemart has
// none. It answers the request itself with a failure, and returns false, when a read fails for a reason a page
// can't show.
func (s *server) gitHubView(w http.ResponseWriter, r *http.Request, account accounts.Account, back string) (gitHubView, bool) {
	if s.GitHubAccounts == nil {
		return gitHubView{}, true
	}
	view := gitHubView{available: true, privateAvailable: s.privateAvailable(), refresh: refreshHref + returnQuery(back)}
	snapshot, err := s.GitHubAccounts.Snapshot(r.Context(), account, visitorOf(r.Context()).token)
	switch {
	case errors.Is(err, accountsapp.ErrNoGitHubToken):
		view.signInAgain = s.absolute(signInAgainHref(back))
	case errors.Is(err, accountsapp.ErrGitHubRead):
		s.logFailure(r, err)
	case err != nil:
		s.fail(w, r, err)
		return gitHubView{}, false
	}
	view.snapshot, view.failed = snapshot, snapshot.ReadFailed
	if !snapshot.ReadAt.IsZero() {
		view.readAt = moment(snapshot.ReadAt, time.Now())
	}
	installations, err := s.GitHubAccounts.Installations(r.Context(), account.ID)
	if err != nil {
		s.fail(w, r, err)
		return gitHubView{}, false
	}
	view.installations, view.private = installations, len(installations) > 0
	return view, true
}

// dashboardView is what the dashboard shows.
type dashboardView struct {
	account accountView
	gitHub  gitHubView
	// stars is true on the Starred rules tab, and false on My libraries. starsAvailable is false when no one can star.
	stars, starsAvailable bool
	// starCount counts the rules the visitor's stars count toward, which the tab shows.
	starCount int
	// owned and used are My libraries' sections.
	owned []ownedView
	used  []usedView
	// starred are the rules the visitor's stars count toward, and uncounted their stars that count toward none.
	starred   []ruleRowView
	uncounted []uncountedView
}

// orgs says which organizations the visitor belongs to, as the dashboard's head does, or is empty for none.
func (d dashboardView) orgs() string {
	return strings.Join(d.gitHub.snapshot.Organizations, ", ")
}

// ownedView is a library the visitor or one of their organizations publishes.
type ownedView struct {
	href, owner, name string
	vetted, isNew     bool
	rules, stars      int
}

func newOwnedViews(owned []views.OwnedLibrary, now time.Time) []ownedView {
	rows := make([]ownedView, len(owned))
	for i, o := range owned {
		rows[i] = ownedView{
			href: libraryHref(o.Library.Owner, o.Library.Name), owner: o.Library.Owner, name: o.Library.Name,
			vetted: o.Vetted, isNew: now.Sub(o.AddedAt) < newWithin, rules: o.Rules, stars: o.Stars,
		}
	}
	return rows
}

// usedView is a library the visitor's projects import, with each project that does.
type usedView struct {
	href, owner, name string
	vetted            bool
	projects          []projectUseView
}

// projectUseView is a project that imports a library, and how many of its rules have updates waiting.
type projectUseView struct {
	repository string
	private    bool
	updates    int
}

// newUsedViews returns each library of imported with the projects of snapshot that import it, in the order snapshot
// lists them, and how many updates wait for each: rules with a newer version, and rules retired.
func newUsedViews(imported []views.ImportedLibrary, snapshot accounts.Snapshot) []usedView {
	var rows []usedView
	for _, lib := range imported {
		row := usedView{href: libraryHref(lib.Library.Owner, lib.Library.Name), owner: lib.Library.Owner, name: lib.Library.Name, vetted: lib.Vetted}
		for _, p := range snapshot.Projects {
			for _, source := range p.Sources {
				if !strings.EqualFold(source.Library, lib.Library.FullName()) {
					continue
				}
				pinned := make([]domain.PinnedVersion, len(source.Rules))
				for i, r := range source.Rules {
					pinned[i] = domain.PinnedVersion{Path: r.Path, Version: r.Version}
				}
				row.projects = append(row.projects, projectUseView{repository: p.FullName(), private: p.Private, updates: lib.Rules.Updates(pinned)})
			}
		}
		if len(row.projects) > 0 {
			rows = append(rows, row)
		}
	}
	return rows
}

// uncountedView is a star of the visitor's that counts toward no current rule of a vetted library, and the form that
// removes it.
type uncountedView struct {
	title, library, path, why, unstar string
	// href is the rule's page, or empty when its library isn't on Rulemart now.
	href string
}

// newUncountedViews describes stars, whose Unstar returns to back.
func newUncountedViews(stars []views.UncountedStar, back string) []uncountedView {
	rows := make([]uncountedView, len(stars))
	for i, s := range stars {
		row := uncountedView{
			title: titleOrID(s.Title, s.Path), library: s.Library.FullName(), path: s.Path,
			unstar: unstarHref + "?" + url.Values{"library": {s.Library.FullName()}, "rule": {s.Path}, "return": {back}}.Encode(),
		}
		if s.Vetted || s.Listed {
			row.href = ruleHref(libraryHref(s.Library.Owner, s.Library.Name), s.Path)
		}
		switch {
		case !s.Vetted && !s.Listed:
			row.why = "Its library is no longer on Rulemart."
		case !s.Vetted:
			row.why = "Its library isn't vetted now, so its rules show no stars."
		case s.ReplacedBy == "":
			row.why = "Its library retired it without a replacement."
		default:
			row.why = "Its library retired it, and its replacements end at a rule that isn't current."
		}
		rows[i] = row
	}
	return rows
}
