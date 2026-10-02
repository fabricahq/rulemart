// List a library, see one's listings, remove or retry them, and browse the unvetted libraries.

package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// Listings lists libraries for signed-in visitors. catalog/app.Listings implements it.
type Listings interface {
	// Check returns the repository text names, and whether the account may list it: it fails with
	// app.ErrInvalidRepository, an *app.ListingConflict, app.ErrAccountListingLimit, app.ErrListingsFull,
	// app.ErrListingTooOften, or app.ErrListingsBusy.
	Check(ctx context.Context, accountID int64, text string) (app.Repository, error)
	// List lists the repository text names after the same checks, and queues its check. It returns the listing's ID,
	// with an error wrapping app.ErrNotQueued when only queueing failed.
	List(ctx context.Context, accountID int64, text string) (int64, error)
	AccountListings(ctx context.Context, accountID int64) ([]views.AccountListing, error)
	// Remove and Retry fail with app.ErrNotFound when the account has no such listing, or, for Retry, its last check
	// didn't fail; Retry fails as Check does when the visitor asked for checks too often.
	Remove(ctx context.Context, accountID, id int64) error
	Retry(ctx context.Context, accountID, id int64) error
}

const (
	// listHref is the listing form, which takes the repository in its repository parameter. POST to it lists the
	// repository. One segment can't hide a library's page.
	listHref = "/list"
	// unvettedHref lists the unvetted libraries. /libraries/unvetted could hide a library: GitHub has an account
	// named libraries.
	unvettedHref = "/unvetted"
	// listingsHref is the signed-in visitor's listings, and removeListingHref and retryListingHref act on the one its
	// listing parameter names, with POST.
	listingsHref      = accountHref + "/listings"
	removeListingHref = listingsHref + "/remove"
	retryListingHref  = listingsHref + "/retry"
)

// checkingRefresh is how long after a listing is added or retried its listings page reloads itself while the worker
// checks it, which takes seconds once the check is queued. After that, the check waits for the hourly poll.
const checkingRefresh = 10 * time.Minute

// listingAvailable reports whether visitors can list libraries: sign-in is available, and so are listings.
func (s *server) listingAvailable() bool {
	return s.Listings != nil && s.signInAvailable()
}

// unvetted lists the libraries listings name that aren't vetted, under the warning. Like every unvetted page, it asks
// search engines neither to index it nor to follow its links.
func (s *server) unvetted(w http.ResponseWriter, r *http.Request) {
	libraries, err := s.catalog.UnvettedLibraries(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, unvettedPage(s.chrome, newLibraryCards(libraries, true), s.listingAvailable()))
}

// listPage shows the listing form, and with a repository in the repository parameter, whether the visitor may list it
// and, if they may, a button that lists it. A visitor who isn't signed in is asked to sign in first.
func (s *server) listPage(w http.ResponseWriter, r *http.Request) {
	text := r.URL.Query().Get("repository")
	view := listView{repository: text}
	v := visitorOf(r.Context())
	switch {
	case v.account == nil:
		view.signIn = s.absolute(signInPageHref(returnPath(r.URL.RequestURI())))
	case r.URL.Query().Has("repository"):
		repo, err := s.Listings.Check(r.Context(), v.account.ID, text)
		if !s.explainRefusal(w, r, &view, err) {
			return
		}
		if err == nil {
			view.confirm = repo.FullName()
			view.action = listHref + "?" + url.Values{"repository": {repo.FullName()}}.Encode()
		}
	}
	s.render(w, r, http.StatusOK, listPage(s.chrome, view))
}

// createListing lists the repository the repository parameter names for the signed-in visitor, and shows their
// listings, where the new one is being checked. A visitor who isn't signed in is sent to sign in and return to the
// form.
func (s *server) createListing(w http.ResponseWriter, r *http.Request) {
	text := r.URL.Query().Get("repository")
	v := visitorOf(r.Context())
	if v.account == nil {
		seeOther(w, r, s.absolute(signInPageHref(listHref+"?"+url.Values{"repository": {text}}.Encode())))
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
		view := listView{repository: text}
		if s.explainRefusal(w, r, &view, err) {
			s.renderPrivate(w, r, http.StatusConflict, listPage(s.chrome, view))
		}
		return
	}
	s.Log.InfoContext(r.Context(), "listed library", "route", s.route(r), "requestID", s.requestID(r),
		"accountID", v.account.ID, "listingID", id)
	setNotice(w, "listed")
	seeOther(w, r, listingsHref)
}

// explainRefusal sets view's problem to why the visitor can't list the repository, by err, and reports whether the
// page can go on. It answers the request itself with a failure, and returns false, when err isn't a refusal.
func (s *server) explainRefusal(w http.ResponseWriter, r *http.Request, view *listView, err error) bool {
	var conflict *app.ListingConflict
	switch {
	case err == nil:
	case errors.Is(err, app.ErrInvalidRepository):
		view.problem = "Give a GitHub repository as owner/name, or its address, such as https://github.com/owner/name."
	case errors.As(err, &conflict) && conflict.Vetted:
		view.problem = "Rulemart has vetted this library already."
		view.existing = existingLibrary(conflict.Library, false)
	case errors.As(err, &conflict):
		view.problem = "This repository is listed already."
		view.existing = existingLibrary(conflict.Library, true)
	case errors.Is(err, app.ErrAccountListingLimit):
		view.problem = "You have " + strconv.Itoa(domain.MaxAccountListings) +
			" unvetted listings, as many as an account may. Remove one to list another."
		view.atLimit = true
	case errors.Is(err, app.ErrListingsFull):
		view.problem = "Rulemart isn't taking new listings right now. Try again later."
	case errors.Is(err, app.ErrListingTooOften), errors.Is(err, app.ErrListingsBusy):
		view.problem = tooOften(err)
	default:
		s.fail(w, r, err)
		return false
	}
	return true
}

// tooOften says why a visitor can't ask for another check now, by err, app.ErrListingTooOften or
// app.ErrListingsBusy.
func tooOften(err error) string {
	if errors.Is(err, app.ErrListingTooOften) {
		return "You've listed or retried " + strconv.Itoa(domain.MaxAccountListingRequests) +
			" times in the last day, as often as an account may. Try again tomorrow."
	}
	return "Rulemart is checking many listings right now. Try again in an hour."
}

// existingLibrary returns the library already in the catalog under the name a visitor tried to list, or nil when the
// catalog doesn't store it yet.
func existingLibrary(lib views.LibraryRef, unvetted bool) *libraryCard {
	if lib.Owner == "" {
		return nil
	}
	return &libraryCard{href: libraryHref(lib.Owner, lib.Name), owner: lib.Owner, name: lib.Name, unvetted: unvetted}
}

// listingsPage shows the signed-in visitor's listings, or sends anyone else to sign in first. While a listing added
// or retried in the last checkingRefresh is being checked, it reloads itself.
func (s *server) listingsPage(w http.ResponseWriter, r *http.Request) {
	account, ok := s.signedIn(w, r, listingsHref)
	if !ok {
		return
	}
	listings, err := s.Listings.AccountListings(r.Context(), account.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.renderPrivate(w, r, http.StatusOK, listingsPage(s.chrome, newListingsView(listings, time.Now())))
}

// removeListing removes the signed-in visitor's listing that the listing parameter names, and returns to their
// listings, saying so.
func (s *server) removeListing(w http.ResponseWriter, r *http.Request) {
	s.changeListing(w, r, "listing-removed", s.Listings.Remove)
}

// retryListing asks the worker to check the signed-in visitor's failed listing that the listing parameter names
// again, and returns to their listings, where it's being checked.
func (s *server) retryListing(w http.ResponseWriter, r *http.Request) {
	s.changeListing(w, r, "listing-retried", func(ctx context.Context, accountID, id int64) error {
		err := s.Listings.Retry(ctx, accountID, id)
		if errors.Is(err, app.ErrNotQueued) {
			s.Log.WarnContext(ctx, "listing not queued", "route", s.route(r), "requestID", s.requestID(r),
				"listingID", id, "error", err.Error())
			return nil
		}
		return err
	})
}

// changeListing applies change to the signed-in visitor's listing that the listing parameter names, and returns to
// their listings with the notice notices names by notice. A visitor whose session has ended goes home told so, and a
// listing they don't have is missing.
func (s *server) changeListing(w http.ResponseWriter, r *http.Request, notice string, change func(ctx context.Context, accountID, id int64) error) {
	v := visitorOf(r.Context())
	if v.account == nil {
		s.signedOutAlready(w, r)
		return
	}
	id, err := strconv.ParseInt(r.URL.Query().Get("listing"), 10, 64)
	if err == nil {
		err = change(r.Context(), v.account.ID, id)
	}
	var numErr *strconv.NumError
	if errors.As(err, &numErr) || errors.Is(err, app.ErrNotFound) {
		s.renderPrivate(w, r, http.StatusNotFound, messagePage(s.chrome, "Not found", "You have no such listing."))
		return
	}
	if errors.Is(err, app.ErrListingTooOften) || errors.Is(err, app.ErrListingsBusy) {
		s.renderPrivate(w, r, http.StatusTooManyRequests, messagePage(s.chrome, "Try again later", tooOften(err)))
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	setNotice(w, notice)
	seeOther(w, r, listingsHref)
}

// listView is what the listing form shows.
type listView struct {
	// signIn is where to sign in and return to the form, for a visitor who isn't signed in, or empty.
	signIn string
	// repository is what the visitor typed, which the form shows again.
	repository string
	// confirm is the repository the visitor may list, as owner/name, and action where its List button posts, or
	// empty before a repository passes its checks.
	confirm, action string
	// problem says why the visitor can't list the repository, or is empty.
	problem string
	// existing is the library already in the catalog under that name, which the problem links, or nil.
	existing *libraryCard
	// atLimit is true when the visitor holds as many unvetted listings as they may.
	atLimit bool
}

// listingsView is what the listings page shows.
type listingsView struct {
	listings []listingView
	// unvetted counts the visitor's listings that aren't vetted, which their limit counts.
	unvetted int
	// refresh is true while a listing added or retried recently is being checked.
	refresh bool
}

// listingView is one listing on the listings page.
type listingView struct {
	// fullName is the repository as its lister gave it, and repositoryURL its GitHub page.
	fullName, repositoryURL string
	state                   domain.ListingState
	// library is the library the listing names while the catalog stores it, or nil.
	library *libraryCard
	// repositoryID is GitHub's ID for the repository, which vetting names, or empty until the worker finds it.
	repositoryID string
	failure      string
	listed       string
	// waiting is true for a listing checked for longer than checkingRefresh, whose check waits for the hourly poll.
	waiting bool
	// remove and retry are where its buttons post; retry is empty unless its last check failed.
	remove, retry string
}

// newListingsView describes listings as of now.
func newListingsView(listings []views.AccountListing, now time.Time) listingsView {
	var v listingsView
	for _, l := range listings {
		query := "?" + url.Values{"listing": {strconv.FormatInt(l.ID, 10)}}.Encode()
		item := listingView{
			fullName: l.Owner + "/" + l.Name, repositoryURL: domain.RepositoryURL(l.Owner + "/" + l.Name), state: l.State,
			repositoryID: l.RepositoryID, failure: l.Failure, listed: date(l.ListedAt), remove: removeListingHref + query,
		}
		if l.Library.Owner != "" {
			item.library = &libraryCard{
				href: libraryHref(l.Library.Owner, l.Library.Name), owner: l.Library.Owner, name: l.Library.Name,
				avatar: l.Library.OwnerAvatarURL, unvetted: l.State != domain.ListingVetted,
			}
		}
		if l.Failure != "" {
			item.retry = retryListingHref + query
		}
		if l.State == domain.ListingChecking {
			item.waiting = now.Sub(l.RequestedAt) > checkingRefresh
			v.refresh = v.refresh || !item.waiting
		}
		if l.State != domain.ListingVetted {
			v.unvetted++
		}
		v.listings = append(v.listings, item)
	}
	return v
}
