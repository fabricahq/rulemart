// Add a library: the picker over the visitor's and their organizations' repositories that publish one, the form that
// adds any public library by its address, and the page that follows the listing's check.

package web

import (
	"cmp"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

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
// each with what adding it would do, and the form that adds any public library by its address, with what checking the
// address its url parameter names found. A visitor who isn't signed in is asked to sign in first.
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
		if err == nil {
			view.confirm = &pickView{fullName: repo.FullName(), detail: "Public library on GitHub", action: addAction(repo.FullName())}
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

// createListing lists the repository the repository parameter names for the signed-in visitor, and follows its check.
// A refusal shows the add-a-library page saying why. A visitor who isn't signed in is sent to sign in and return to the
// page, with the repository in its form.
func (s *server) createListing(w http.ResponseWriter, r *http.Request) {
	text := r.URL.Query().Get("repository")
	v := visitorOf(r.Context())
	if v.account == nil {
		seeOther(w, r, s.absolute(signInPageHref(listHref+"?"+url.Values{"url": {text}}.Encode())))
		return
	}
	id, err := s.Listings.List(r.Context(), v.account.ID, text)
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
	seeOther(w, r, runHref+"?"+url.Values{"repo": {owner + "/" + name}}.Encode())
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
		view.problemLink, view.problemLinkText = runHref+"?"+url.Values{"repo": {listedName(conflict.Library, name)}}.Encode(), "See how it went"
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
		view.problemLink, view.problemLinkText = listingsHref, "Your listings"
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
	// pickPrivate is a private repository, which Rulemart can't publish.
	pickPrivate
)

// pickOrder orders the picker's rows by state.
var pickOrder = map[pickState]int{pickAdd: 0, pickAdding: 1, pickOnRulemart: 2, pickPrivate: 3}

// pickView is a repository on the picker.
type pickView struct {
	fullName, detail string
	state            pickState
	private          bool
	// href is the library's page, or the page that follows its listing's check; action is where Add this library posts.
	href, action string
	// vetted is false for a library on Rulemart that isn't vetted, whose page link carries nofollow.
	vetted bool
	// failed is true for a listing whose check failed.
	failed bool
}

// newPickViews returns the picker's rows: each library snapshot found, by what adding it would do, then each library
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
	via := func(owner string) string {
		if strings.EqualFold(owner, login) {
			return "Public"
		}
		return "Public · via the " + owner + " organization"
	}
	var picks []pickView
	seen := map[string]bool{}
	for _, lib := range snapshot.Libraries {
		key := strings.ToLower(lib.FullName())
		seen[key] = true
		pick := pickView{fullName: lib.FullName(), detail: "Public · " + domain.ReleaseTag(lib.Release), private: lib.Private}
		o, on := onRulemart[key]
		state, isAdding := adding[key]
		switch {
		case lib.Private:
			pick.state, pick.detail = pickPrivate, "Private · "+domain.ReleaseTag(lib.Release)
		case on:
			pick.state, pick.href, pick.vetted, pick.detail = pickOnRulemart, libraryHref(o.Library.Owner, o.Library.Name), o.Vetted, via(lib.Owner)
		case isAdding:
			pick.state, pick.href, pick.failed = pickAdding, runHref+"?"+url.Values{"repo": {lib.FullName()}}.Encode(), state == domain.ListingFailed
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
				fullName: o.Library.FullName(), detail: via(o.Library.Owner), state: pickOnRulemart,
				href: libraryHref(o.Library.Owner, o.Library.Name), vetted: o.Vetted,
			})
		}
	}
	return picks
}

// runPage follows the check of the signed-in visitor's listing of the repository the repo parameter names: a checklist
// that ticks once the check finds the library, saying what it found, or shows why it failed, with Try again and Remove.
// While Rulemart is checking, the page follows the check, with poll.js or by reloading itself.
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
	view := runView{fullName: owner + "/" + name}
	listing, listed := findListing(listings, owner, name)
	if listed {
		view.fullName, view.state, view.failure = listing.Owner+"/"+listing.Name, listing.State, listing.Failure
		query := "?" + url.Values{"listing": {strconv.FormatInt(listing.ID, 10)}}.Encode()
		view.retry = retryListingHref + "?" + url.Values{
			"listing": {strconv.FormatInt(listing.ID, 10)}, "return": {runHref + "?" + url.Values{"repo": {view.fullName}}.Encode()},
		}.Encode()
		view.remove = removeListingHref + query
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
}

// done reports whether the library is live on Rulemart.
func (v runView) done() bool { return v.library != nil }

// runStep is one line of the checklist, which ticks once it's done, spins while it's running, and says why it failed.
type runStep struct {
	text                  string
	done, running, failed bool
}

// steps returns the checklist, as the prototype words it: what the check found, once it found it.
func (v runView) steps() []runStep {
	steps := []runStep{
		{text: "Looking for rule-library.yaml in " + v.fullName},
		{text: "Read the latest library release · its rules at their published versions"},
		{text: "Groups and rules indexed"},
		{text: "Watching for new library releases"},
	}
	switch {
	case v.library != nil:
		lib := v.library
		steps[0].text = "Found rule-library.yaml in " + v.fullName
		steps[1].text = "Read library release " + domain.ReleaseTag(lib.LatestRelease) + " · " + plural(lib.Rules, "rule", "rules") + " at their published versions"
		steps[2].text = plural(lib.Groups, "group", "groups") + ", " + plural(lib.Rules, "rule", "rules") + " indexed"
		if lib.LicenseExpression != "" {
			steps[2].text += " · license " + lib.LicenseExpression
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
