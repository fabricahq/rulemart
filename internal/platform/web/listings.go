// List a library, remove or retry one's listings, which My libraries shows, and browse the unvetted libraries.

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
	// unvettedHref lists the unvetted libraries. /libraries/unvetted could hide a library: GitHub has an account
	// named libraries.
	unvettedHref = "/unvetted"
	// unvettedOptInLabel labels the checkbox that adds unvetted libraries to a list, which the about and FAQ pages
	// name too.
	unvettedOptInLabel = "Include unvetted libraries"
	// unvettedWarningText heads every page of an unvetted library and the dialog that confirms adding its rules, and
	// the about page quotes it.
	unvettedWarningText = "This library has not been vetted. Be sure to review these rules carefully."
	// listingsHref was the signed-in visitor's listings, which My libraries holds now, and redirects there.
	// removeListingHref and retryListingHref act on the listing their listing parameter names, with POST; GET of
	// removeListingHref asks first.
	listingsHref      = dashboardHref + "/listings"
	removeListingHref = listingsHref + "/remove"
	retryListingHref  = listingsHref + "/retry"
	// legacyListingsHref is the listings page's old address, which redirects.
	legacyListingsHref = accountHref + "/listings"
)

// checkingLonger is how long after a listing is added or retried its check is taking longer than usual: once queued,
// a check takes seconds, so after a few minutes it waits for the hourly poll.
const checkingLonger = 3 * time.Minute

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
	s.render(w, r, http.StatusOK, unvettedPage(s.chrome, newLibraryCards(libraries, false), s.listingAvailable()))
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

// legacyListings redirects the listings page's addresses, now and before, to My libraries, which holds the visitor's
// listings.
func (s *server) legacyListings(w http.ResponseWriter, r *http.Request) {
	redirect(w, r, dashboardHref)
}

// removeListingPage asks the signed-in visitor to confirm removing their listing that the listing parameter names,
// saying what removing it does, or sends anyone else to sign in first.
func (s *server) removeListingPage(w http.ResponseWriter, r *http.Request) {
	account, ok := s.signedIn(w, r, dashboardHref)
	if !ok {
		return
	}
	listings, err := s.Listings.AccountListings(r.Context(), account.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, l := range newListingViews(listings) {
		if l.id == r.URL.Query().Get("listing") {
			s.renderPrivate(w, r, http.StatusOK, removeListingPage(s.chrome, l))
			return
		}
	}
	s.noSuchListing(w, r)
}

// noSuchListing answers a request about a listing the visitor doesn't have.
func (s *server) noSuchListing(w http.ResponseWriter, r *http.Request) {
	s.renderPrivate(w, r, http.StatusNotFound, listingMessagePage(s.chrome, "Not found",
		"You have no such listing. It may have been removed already."))
}

// removeListing removes the signed-in visitor's listing that the listing parameter names, and returns to My
// libraries, saying so.
func (s *server) removeListing(w http.ResponseWriter, r *http.Request) {
	notice := "listing-removed"
	s.changeListing(w, r, &notice, func(ctx context.Context, accountID, id int64) error {
		// The notice says what removing did, which depends on the listing's state.
		listings, err := s.Listings.AccountListings(ctx, accountID)
		if err != nil {
			return err
		}
		for _, l := range listings {
			if l.ID == id {
				notice = removedNotices[l.State]
			}
		}
		return s.Listings.Remove(ctx, accountID, id)
	})
}

// removedNotices are what My libraries says once a listing in each state is removed.
var removedNotices = map[domain.ListingState]string{
	domain.ListingChecking: "listing-removed-checking",
	domain.ListingListed:   "listing-removed-listed",
	domain.ListingVetted:   "listing-removed-vetted",
	domain.ListingFailed:   "listing-removed",
}

// retryListing asks the worker to check the signed-in visitor's failed listing that the listing parameter names
// again, and returns to My libraries, where it's being checked, or to the run page the return parameter names, which
// follows the check.
func (s *server) retryListing(w http.ResponseWriter, r *http.Request) {
	notice := "listing-retried"
	s.changeListing(w, r, &notice, func(ctx context.Context, accountID, id int64) error {
		err := s.Listings.Retry(ctx, accountID, id)
		if errors.Is(err, app.ErrListingNotFailed) {
			// From a page left open while the listing was checked.
			notice = "listing-not-failed"
			return nil
		}
		if errors.Is(err, app.ErrNotQueued) {
			s.Log.WarnContext(ctx, "listing not queued", "route", s.route(r), "requestID", s.requestID(r),
				"listingID", id, "error", err.Error())
			return nil
		}
		return err
	})
}

// listingReturn returns where a change to a listing returns to: the run page target names, as returnPath checks it,
// or else My libraries.
func listingReturn(target string) string {
	back := returnPath(target)
	if u, err := url.Parse(back); err == nil && u.Path == runHref {
		return back
	}
	return dashboardHref
}

// changeListing applies change to the signed-in visitor's listing that the listing parameter names, and returns to My
// libraries, or the run page listingReturn allows, with the notice notices names by *notice, which change may replace.
// A visitor who isn't signed in is sent to sign in and return to My libraries, and a listing they don't have is
// missing.
func (s *server) changeListing(w http.ResponseWriter, r *http.Request, notice *string, change func(ctx context.Context, accountID, id int64) error) {
	v := visitorOf(r.Context())
	if v.account == nil {
		if hasCookie(r, sessionCookie) {
			clearCookie(w, sessionCookie)
		}
		seeOther(w, r, s.absolute(signInPageHref(dashboardHref)))
		return
	}
	id, err := strconv.ParseInt(r.URL.Query().Get("listing"), 10, 64)
	if err == nil {
		err = change(r.Context(), v.account.ID, id)
	}
	var numErr *strconv.NumError
	if errors.As(err, &numErr) || errors.Is(err, app.ErrNotFound) {
		s.noSuchListing(w, r)
		return
	}
	if errors.Is(err, app.ErrListingTooOften) || errors.Is(err, app.ErrListingsBusy) {
		s.renderPrivate(w, r, http.StatusTooManyRequests, listingMessagePage(s.chrome, "Try again later", tooOften(err)))
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	setNotice(w, *notice)
	seeOther(w, r, listingReturn(r.URL.Query().Get("return")))
}

// listingView is a listing of the visitor's on the page that confirms removing it.
type listingView struct {
	id string
	// fullName is the repository as its lister gave it.
	fullName string
	state    domain.ListingState
	// remove is where the page's button posts.
	remove string
}

// newListingViews describes listings.
func newListingViews(listings []views.AccountListing) []listingView {
	rows := make([]listingView, len(listings))
	for i, l := range listings {
		id := strconv.FormatInt(l.ID, 10)
		rows[i] = listingView{
			id: id, fullName: l.Owner + "/" + l.Name, state: l.State,
			remove: removeListingHref + "?" + url.Values{"listing": {id}}.Encode(),
		}
	}
	return rows
}

// moment says when t was, as of now: how many minutes ago within the hour, and otherwise its time, and its date
// unless it was today, in UTC.
func moment(t, now time.Time) string {
	switch ago := now.Sub(t); {
	case ago < time.Minute:
		return "less than a minute ago"
	case ago < time.Hour:
		return plural(int(ago/time.Minute), "minute", "minutes") + " ago"
	case t.UTC().Format(time.DateOnly) == now.UTC().Format(time.DateOnly):
		return "at " + t.UTC().Format("15:04") + " UTC"
	}
	return "on " + date(t) + " at " + t.UTC().Format("15:04") + " UTC"
}
