// Add a library: the picker over the visitor's and their organizations' repositories that publish one, the form that
// adds a library by its address, once GitHub says the visitor may push to it, and the page that follows the listing's
// check.

package web

import (
	"cmp"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	accountsapp "github.com/fabricahq/rulemart/internal/contexts/accounts/app"
	accounts "github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

const (
	// listHref is the page that adds a library, which checks the address its url parameter names. POST to it, with the
	// repository parameter, lists the repository, and leads to runHref.
	listHref = dashboardHref + "/add"
	// runHref follows the check of the listing of the repository its repo parameter names.
	runHref = listHref + "/run"
	// legacyListHref is the listing form's old address, which redirects.
	legacyListHref = "/list"
)

// addPage shows the add-a-library page: the visitor's and their organizations' repositories that publish a library,
// each with what adding it would do, and the form that adds a public library by its address, with what checking the
// address its url parameter names found, including whether GitHub says the visitor may push to it. A visitor who isn't
// signed in is asked to sign in first.
func (s *server) addPage(w http.ResponseWriter, r *http.Request) {
	account, ok := s.signedIn(w, r, returnPath(r.URL.RequestURI()))
	if !ok {
		return
	}
	view, ok := s.addView(w, r, account)
	if !ok {
		return
	}
	if text := r.URL.Query().Get("url"); r.URL.Query().Has("url") {
		view.address = text
		repo, err := s.Listings.Check(r.Context(), account.ID, text)
		if !s.explainRefusal(w, r, &view, repo, err) {
			return
		}
		if err == nil && view.problem == "" {
			access, ok := s.writeAccess(w, r, account, repo, returnPath(r.URL.RequestURI()))
			if !ok {
				return
			}
			// A refusal, such as of a private library or one the visitor can't push to, offers nothing to add.
			if access != mayAdd {
				view.problem = access.problem(repo)
			} else {
				view.confirm = &pickView{fullName: repo.FullName(), detail: "Public library on GitHub", action: addAction(repo.FullName())}
			}
		}
	}
	s.renderPrivate(w, r, http.StatusOK, addPage(s.chrome, view))
}

// legacyList redirects the listing form's old address to the add-a-library page, its repository parameter to the
// page's url.
func (s *server) legacyList(w http.ResponseWriter, r *http.Request) {
	target := listHref
	if r.URL.Query().Has("repository") {
		target += "?" + url.Values{"url": {r.URL.Query().Get("repository")}}.Encode()
	}
	redirect(w, r, target)
}

// addView returns what the add-a-library page shows of the visitor's repositories, or answers the request with a
// failure and returns false.
func (s *server) addView(w http.ResponseWriter, r *http.Request, account accounts.Account) (addView, bool) {
	gitHub, ok := s.gitHubView(w, r, account, listHref)
	if !ok {
		return addView{}, false
	}
	libraries, err := s.catalog.Dashboard(r.Context(), gitHub.snapshot.Owners(account.Login), nil)
	if err != nil {
		s.fail(w, r, err)
		return addView{}, false
	}
	listings, err := s.Listings.AccountListings(r.Context(), account.ID)
	if err != nil {
		s.fail(w, r, err)
		return addView{}, false
	}
	return addView{gitHub: gitHub, picks: newPickViews(gitHub.snapshot, libraries.Owned, listings, account.Login)}, true
}

// createListing lists the repository the repository parameter names for the signed-in visitor, and follows its check,
// once GitHub says the visitor may push to it: the picker only offers what they may add, so this is the gate. A refusal
// shows the add-a-library page saying why. A visitor who isn't signed in is sent to sign in and return to the page, with
// the repository in its form.
func (s *server) createListing(w http.ResponseWriter, r *http.Request) {
	text := r.URL.Query().Get("repository")
	v := visitorOf(r.Context())
	back := listHref + "?" + url.Values{"url": {text}}.Encode()
	if v.account == nil {
		seeOther(w, r, s.absolute(signInPageHref(back)))
		return
	}
	// The listing's own checks come before the one that asks GitHub, so an address that isn't a repository's, or one
	// Rulemart already has, is told so without a request to GitHub.
	repo, err := s.Listings.Check(r.Context(), v.account.ID, text)
	if err == nil {
		access, ok := s.writeAccess(w, r, *v.account, repo, back)
		if !ok {
			return
		}
		if access != mayAdd {
			view, ok := s.addView(w, r, *v.account)
			if !ok {
				return
			}
			view.address, view.problem = text, access.problem(repo)
			s.renderPrivate(w, r, access.status(), addPage(s.chrome, view))
			return
		}
	}
	var id int64
	if err == nil {
		id, err = s.Listings.List(r.Context(), v.account.ID, text)
	}
	if errors.Is(err, app.ErrNotQueued) {
		// The listing stands, and the hourly poll checks it.
		s.Log.WarnContext(r.Context(), "listing not queued", "route", s.route(r), "requestID", s.requestID(r),
			"listingID", id, "error", err.Error())
		err = nil
	}
	if err != nil {
		view, ok := s.addView(w, r, *v.account)
		if !ok {
			return
		}
		view.address = text
		if s.explainRefusal(w, r, &view, app.Repository{}, err) {
			s.renderPrivate(w, r, http.StatusConflict, addPage(s.chrome, view))
		}
		return
	}
	s.Log.InfoContext(r.Context(), "listed library", "route", s.route(r), "requestID", s.requestID(r),
		"accountID", v.account.ID, "listingID", id)
	owner, name, _ := domain.ParseGitHubRepository(text)
	seeOther(w, r, runPageHref(owner+"/"+name))
}

// writeAccess is what GitHub says of whether the visitor may add a repository.
type writeAccess int

const (
	// mayAdd is that the visitor's token may push to the repository.
	mayAdd writeAccess = iota
	// notMaintainer is that it may not, or can't see the repository.
	notMaintainer
	// gitHubUnreadable is that GitHub couldn't say.
	gitHubUnreadable
)

// problem says why the visitor can't add repo, as the add-a-library page says it.
func (a writeAccess) problem(repo app.Repository) string {
	if a == gitHubUnreadable {
		return "Rulemart couldn't read " + repo.FullName() + " from GitHub just now. Try again."
	}
	return "Only someone with write access to " + repo.FullName() + " on GitHub can add it to Rulemart."
}

// status is the HTTP status of the page that refuses a request to add a repository the visitor may not.
func (a writeAccess) status() int {
	if a == gitHubUnreadable {
		return http.StatusBadGateway
	}
	return http.StatusForbidden
}

// writeAccess asks GitHub whether the signed-in visitor, whose session r carries, may push to repo, so may add it. When
// the session keeps no token GitHub takes it sends the visitor to sign in again and return to back, and returns false.
func (s *server) writeAccess(w http.ResponseWriter, r *http.Request, account accounts.Account, repo app.Repository, back string) (writeAccess, bool) {
	maintains, err := s.GitHubAccounts.Maintains(r.Context(), visitorOf(r.Context()).token, accounts.Repository{Owner: repo.Owner, Name: repo.Name})
	switch {
	case errors.Is(err, accountsapp.ErrNoGitHubToken):
		s.signInAgain(w, r, back)
		return mayAdd, false
	case err != nil:
		s.Log.WarnContext(r.Context(), "check of write access failed", "route", s.route(r), "requestID", s.requestID(r),
			"accountID", account.ID, "repository", repo.FullName(), "error", err.Error())
		return gitHubUnreadable, true
	case !maintains:
		return notMaintainer, true
	}
	return mayAdd, true
}

// explainRefusal sets view's problem to why the visitor can't add repo, by err, in the prototype's words, and reports
// whether the page can go on. It answers the request itself, and returns false, with the page of a library Rulemart
// has already, or with a failure when err isn't a refusal.
func (s *server) explainRefusal(w http.ResponseWriter, r *http.Request, view *addView, repo app.Repository, err error) bool {
	var conflict *app.ListingConflict
	name := repo.FullName()
	if repo.Owner == "" {
		name = cmp.Or(strings.TrimSpace(view.address), "This repository")
	}
	switch {
	case err == nil:
		if private := view.privatePick(repo.FullName()); private {
			view.problem = repo.FullName() + " is private. Private libraries can't be published on Rulemart."
			return true
		}
	case errors.Is(err, app.ErrInvalidRepository):
		view.problem = "Enter a GitHub repository URL, like https://github.com/owner/repo."
	case errors.As(err, &conflict) && conflict.Own:
		view.problem = "You added " + listedName(conflict.Library, name) + " already."
		view.problemLink, view.problemLinkText = runPageHref(listedName(conflict.Library, name)), "See how it went"
	case errors.As(err, &conflict) && conflict.Checking && time.Since(conflict.RequestedAt) > checkingLonger:
		view.problem = "Someone added " + name + ", and Rulemart's check of it is taking longer than usual. Rulemart checks it again within the hour."
	case errors.As(err, &conflict) && conflict.Checking:
		view.problem = "Someone added " + name + " a moment ago, and Rulemart is checking it."
	case errors.As(err, &conflict) && conflict.Library.Owner != "":
		// As the prototype's form does: the library's page, which says Rulemart has it.
		setNotice(w, alreadyListedKey)
		seeOther(w, r, libraryHref(conflict.Library.Owner, conflict.Library.Name))
		return false
	case errors.As(err, &conflict):
		view.problem = name + " is already on Rulemart."
	case errors.Is(err, app.ErrAccountListingLimit):
		view.problem = "You have " + strconv.Itoa(domain.MaxAccountListings) + " libraries Rulemart hasn't vetted, as many as an account may add. Remove one, such as one that failed, to add another."
		view.problemLink, view.problemLinkText = dashboardHref, "My libraries"
	case errors.Is(err, app.ErrListingsFull):
		view.problem = "Rulemart isn't taking new libraries right now. Try again later."
	case errors.Is(err, app.ErrListingTooOften), errors.Is(err, app.ErrListingsBusy):
		view.problem = tooOften(err)
	default:
		s.fail(w, r, err)
		return false
	}
	return true
}

// listedName names the library a conflict found, or else fallback.
func listedName(lib views.LibraryRef, fallback string) string {
	if lib.Owner == "" {
		return fallback
	}
	return lib.FullName()
}

// addAction returns where a form posts to add the repository fullName.
func addAction(fullName string) string {
	return listHref + "?" + url.Values{"repository": {fullName}}.Encode()
}

// addView is what the add-a-library page shows.
type addView struct {
	gitHub gitHubView
	// picks are the visitor's and their organizations' repositories that publish a library.
	picks []pickView
	// address is what the visitor wrote in the form, which it shows again.
	address string
	// confirm is the repository the form's address names, which the visitor may add, or nil.
	confirm *pickView
	// problem says why the visitor can't add the address, or is empty; problemLink leads somewhere that explains,
	// labeled problemLinkText.
	problem, problemLink, problemLinkText string
}

// privatePick reports whether fullName is one of the picks that are private.
func (v addView) privatePick(fullName string) bool {
	for _, p := range v.picks {
		if p.private && strings.EqualFold(p.fullName, fullName) {
			return true
		}
	}
	return false
}

// pickState is what adding a repository on the picker would do.
type pickState int

const (
	// pickAdd is a public repository Rulemart doesn't have, which the visitor can add.
	pickAdd pickState = iota
	// pickOnRulemart is one Rulemart has, vetted or listed.
	pickOnRulemart
	// pickAdding is one whose listing the visitor made, which Rulemart is checking, or whose check failed.
	pickAdding
	// pickReadOnly is a public repository the visitor may read but not push to, so can't add.
	pickReadOnly
	// pickPrivate is a private repository, which Rulemart can't publish.
	pickPrivate
)

// The reasons a dimmed row of the picker gives for why its repository can't be added.
const (
	privateNote  = "Private libraries can't be published on Rulemart"
	readOnlyNote = "Only someone with write access can add it"
)

// pickOrder orders the picker's rows by state.
var pickOrder = map[pickState]int{pickAdd: 0, pickAdding: 1, pickOnRulemart: 2, pickReadOnly: 3, pickPrivate: 4}

// pickView is a repository on the picker.
type pickView struct {
	fullName, detail string
	state            pickState
	private          bool
	// note is why the visitor can't add it, for a dimmed row, or is empty.
	note string
	// href is the library's page, or the page that follows its listing's check; action is where Add this library posts.
	href, action string
	// vetted is false for a library on Rulemart that isn't vetted, whose page link carries nofollow.
	vetted bool
	// failed is true for a listing whose check failed.
	failed bool
}

// newPickViews returns the picker's rows: each library snapshot found, by what adding it would do, which for a public one
// the visitor may only read is nothing, then each library
// owned holds that snapshot didn't find, such as one too old for the read, as on Rulemart. login is the visitor's, whose
// own libraries name no organization.
func newPickViews(snapshot accounts.Snapshot, owned []views.OwnedLibrary, listings []views.AccountListing, login string) []pickView {
	onRulemart := map[string]views.OwnedLibrary{}
	for _, o := range owned {
		onRulemart[strings.ToLower(o.Library.FullName())] = o
	}
	adding := map[string]domain.ListingState{}
	for _, l := range listings {
		if l.State == domain.ListingChecking || l.State == domain.ListingFailed {
			adding[strings.ToLower(l.Owner+"/"+l.Name)] = l.State
		}
	}
	// detail says whether a repository is public, which library release it published, if known, and, for an
	// organization's, which organization it's via.
	detail := func(visibility string, release int, owner string) string {
		text := visibility
		if release > 0 {
			text += " · " + domain.ReleaseTag(release)
		}
		if !strings.EqualFold(owner, login) {
			text += " · via the " + owner + " organization"
		}
		return text
	}
	var picks []pickView
	seen := map[string]bool{}
	for _, lib := range snapshot.Libraries {
		key := strings.ToLower(lib.FullName())
		seen[key] = true
		pick := pickView{fullName: lib.FullName(), detail: detail("Public", lib.Release, lib.Owner), private: lib.Private}
		o, on := onRulemart[key]
		state, isAdding := adding[key]
		switch {
		case lib.Private:
			pick.state, pick.detail, pick.note = pickPrivate, detail("Private", lib.Release, lib.Owner), privateNote
		case on:
			pick.state, pick.href, pick.vetted = pickOnRulemart, libraryHref(o.Library.Owner, o.Library.Name), o.Vetted
		case isAdding:
			pick.state, pick.href, pick.failed = pickAdding, runPageHref(lib.FullName()), state == domain.ListingFailed
		case !lib.Writable:
			pick.state, pick.note = pickReadOnly, readOnlyNote
		default:
			pick.action = addAction(lib.FullName())
		}
		picks = append(picks, pick)
	}
	// As the prototype orders them: what the visitor can add first, then what Rulemart is adding, has, and can't take.
	slices.SortStableFunc(picks, func(a, b pickView) int { return cmp.Compare(pickOrder[a.state], pickOrder[b.state]) })
	for _, o := range owned {
		if !seen[strings.ToLower(o.Library.FullName())] {
			picks = append(picks, pickView{
				fullName: o.Library.FullName(), detail: detail("Public", 0, o.Library.Owner), state: pickOnRulemart,
				href: libraryHref(o.Library.Owner, o.Library.Name), vetted: o.Vetted,
			})
		}
	}
	return picks
}

// runPage follows the check of the signed-in visitor's listing of the repository the repo parameter names: a checklist
// that ticks once the check finds the library, saying what it found, or shows why it failed, with Try again and Remove.
// While Rulemart is checking, the page follows the check, with poll.js or by reloading itself, until the check takes
// longer than usual, as the listings page says, when the next is up to an hour away.
func (s *server) runPage(w http.ResponseWriter, r *http.Request) {
	account, ok := s.signedIn(w, r, returnPath(r.URL.RequestURI()))
	if !ok {
		return
	}
	owner, name, err := domain.ParseGitHubRepository(r.URL.Query().Get("repo"))
	if err != nil {
		s.notAdding(w, r)
		return
	}
	listings, err := s.Listings.AccountListings(r.Context(), account.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	view := runView{fullName: owner + "/" + name, here: runPageHref(owner + "/" + name)}
	listing, listed := findListing(listings, owner, name)
	if listed {
		view.fullName, view.state, view.failure = listing.Owner+"/"+listing.Name, listing.State, listing.Failure
		query := "?" + url.Values{"listing": {strconv.FormatInt(listing.ID, 10)}}.Encode()
		view.retry = retryListingHref + "?" + url.Values{
			"listing": {strconv.FormatInt(listing.ID, 10)}, "return": {runPageHref(view.fullName)},
		}.Encode()
		view.remove = removeListingHref + query
		if listing.State == domain.ListingChecking {
			now := time.Now()
			view.longer, view.requested = now.Sub(listing.RequestedAt) > checkingLonger, moment(listing.RequestedAt, now)
		}
		if listing.Library.Owner != "" {
			owner, name = listing.Library.Owner, listing.Library.Name
		}
	}
	if !listed || listing.State == domain.ListingListed || listing.State == domain.ListingVetted {
		page, err := s.catalog.LibraryPage(r.Context(), owner, name)
		switch {
		case errors.Is(err, app.ErrNotFound) && !listed:
			s.notAdding(w, r)
			return
		case err != nil && !errors.Is(err, app.ErrNotFound):
			s.fail(w, r, err)
			return
		case err == nil:
			view.state, view.library, view.fullName = domain.ListingListed, &page.Library, page.Library.FullName()
		}
	}
	s.renderPrivate(w, r, http.StatusOK, runPage(s.chrome, view))
}

// notAdding answers a request to follow a listing the visitor didn't make.
func (s *server) notAdding(w http.ResponseWriter, r *http.Request) {
	s.renderPrivate(w, r, http.StatusNotFound, messagePage(s.chrome, "Not found", "You aren't adding that library. Add one from your dashboard."))
}

// findListing returns the listing of owner/name among listings, by the name its lister gave or the name GitHub gives it
// now, without regard to case.
func findListing(listings []views.AccountListing, owner, name string) (views.AccountListing, bool) {
	want := strings.ToLower(owner + "/" + name)
	for _, l := range listings {
		if strings.ToLower(l.Owner+"/"+l.Name) == want || strings.ToLower(l.Library.FullName()) == want {
			return l, true
		}
	}
	return views.AccountListing{}, false
}

// runView is what the page that follows a listing's check shows.
type runView struct {
	fullName string
	state    domain.ListingState
	// failure says why the check failed, for a failed listing.
	failure string
	// library is the library once Rulemart has it, or nil.
	library *views.Library
	// retry and remove are where Try again posts and where Remove leads, for the visitor's listing.
	retry, remove string
	// longer is true for a check taking longer than checkingLonger, which waits for the worker's hourly poll, and
	// requested says when the visitor last asked for it, and here is the page's own address.
	longer          bool
	requested, here string
}

// done reports whether the library is live on Rulemart.
func (v runView) done() bool { return v.library != nil }

// following reports whether the page follows the check every two seconds: while it runs, and isn't taking longer than
// usual, when the next check is up to an hour away.
func (v runView) following() bool { return !v.done() && v.state != domain.ListingFailed && !v.longer }

// runStep is one line of the checklist, which ticks once it's done, spins while it's running, and says why it failed.
type runStep struct {
	parts                 []runPart
	done, running, failed bool
}

// runPart is a run of a step's text, in stronger type when it names what the check found, as the prototype's does.
type runPart struct {
	text   string
	strong bool
}

// runPartsText writes parts' text, each run in stronger type in a b element. It writes them itself, since templ puts a
// space after an element that ends a line, which would stand before the punctuation that follows a run in stronger
// type.
func runPartsText(parts []runPart) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		for _, part := range parts {
			text := templ.EscapeString(part.text)
			if part.strong {
				text = `<b class="font-semibold">` + text + `</b>`
			}
			if _, err := io.WriteString(w, text); err != nil {
				return err
			}
		}
		return nil
	})
}

// runText and runStrong return a step's text as one part, in ordinary and in stronger type.
func runText(text string) runPart   { return runPart{text: text} }
func runStrong(text string) runPart { return runPart{text: text, strong: true} }

// steps returns the checklist, as the prototype words it: what the check found, once it found it.
func (v runView) steps() []runStep {
	steps := []runStep{
		{parts: []runPart{runText("Looking for "), runStrong("rule-library.yaml"), runText(" in " + v.fullName)}},
		{parts: []runPart{runText("Read the latest library release · its rules at their published versions")}},
		{parts: []runPart{runText("Groups and rules indexed")}},
		{parts: []runPart{runText("Watching for new library releases")}},
	}
	switch {
	case v.library != nil:
		lib := v.library
		rules := plural(lib.Rules, "rule", "rules")
		steps[0].parts = []runPart{runText("Found "), runStrong("rule-library.yaml"), runText(" in " + v.fullName)}
		steps[1].parts = []runPart{runText("Read library release "), runStrong(domain.ReleaseTag(lib.LatestRelease)), runText(" · " + rules + " at their published versions")}
		steps[2].parts = []runPart{runStrong(plural(lib.Groups, "group", "groups")), runText(", "), runStrong(rules), runText(" indexed")}
		if lib.LicenseExpression != "" {
			steps[2].parts = append(steps[2].parts, runText(" · license "), runStrong(lib.LicenseExpression))
		}
		for i := range steps {
			steps[i].done = true
		}
	case v.state == domain.ListingFailed:
		steps[0].failed = true
	default:
		steps[0].running = true
	}
	return steps
}

// runPageHref is the page that follows the check of the visitor's listing of repository, owner/name.
func runPageHref(repository string) string {
	return runHref + "?" + url.Values{"repo": {repository}}.Encode()
}
