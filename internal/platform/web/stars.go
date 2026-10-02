// Star and unstar vetted libraries from their pages, and see one's stars.

package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// Stars stars libraries for signed-in visitors. catalog/app.Stars implements it.
type Stars interface {
	// Star stars the vetted library library names, as owner/name, for the account, or fails with app.ErrNotFound
	// when there's no such vetted library. Starring a library twice keeps one star.
	Star(ctx context.Context, accountID int64, library string) (views.LibraryRef, error)
	// Unstar removes the account's star from the library library names, vetted or not, if it has one, or fails with
	// app.ErrNotFound when library isn't owner/name.
	Unstar(ctx context.Context, accountID int64, library string) error
	Starred(ctx context.Context, accountID int64, owner, name string) (bool, error)
	// AccountStars returns the libraries the account starred, most recently starred first.
	AccountStars(ctx context.Context, accountID int64) ([]views.StarredLibrary, error)
}

const (
	// starsHref is the signed-in visitor's stars. POST to it stars the library its library parameter names, as
	// owner/name, and starsHref's remove unstars it; both return to their return parameter, or the library's page.
	starsHref  = accountHref + "/stars"
	unstarHref = starsHref + "/remove"
)

// starPurpose is the sign-in page's to parameter for a visitor who signs in to star a library.
const starPurpose = "star"

// starSignIn returns where a visitor who isn't signed in goes to sign in and star a library, returning to back.
func (s *server) starSignIn(back string) string {
	return s.absolute(signInHref + "?" + url.Values{"return": {back}, "to": {starPurpose}}.Encode())
}

// starsAvailable reports whether visitors can star libraries: sign-in is available, and so are stars.
func (s *server) starsAvailable() bool {
	return s.Stars != nil && s.signInAvailable()
}

// libraryView describes lib for the page r asks for, with the star control a vetted library's pages show: for a
// signed-in visitor, whether they starred it, which it reads.
func (s *server) libraryView(r *http.Request, lib views.Library) (libraryView, error) {
	view := newLibraryView(lib)
	if !lib.Vetted || s.Stars == nil {
		return view, nil
	}
	v := visitorOf(r.Context())
	view.star = starView{shown: true, count: lib.Stars}
	if v.account == nil {
		if v.signIn != "" {
			view.star.signIn = s.starSignIn(v.here)
		}
		return view, nil
	}
	starred, err := s.Stars.Starred(r.Context(), v.account.ID, lib.Owner, lib.Name)
	if err != nil {
		return libraryView{}, err
	}
	view.star.starred = starred
	view.star.action = starAction(starsHref, lib.FullName(), v.here, view.href)
	if starred {
		view.star.action = starAction(unstarHref, lib.FullName(), v.here, view.href)
	}
	return view, nil
}

// starAction returns where a form posts to star or unstar the library fullName, whose page is libraryPage, from the
// page here: path, starsHref or unstarHref, with the library, and here to return to unless it's the library's page,
// where a star returns anyway.
func starAction(path, fullName, here, libraryPage string) string {
	query := url.Values{"library": {fullName}}
	if here != libraryPage {
		query.Set("return", here)
	}
	return path + "?" + query.Encode()
}

// starLibrary stars the library the library parameter names for the signed-in visitor, and returns to the return
// parameter, or the library's page.
func (s *server) starLibrary(w http.ResponseWriter, r *http.Request) {
	s.changeStar(w, r, func(ctx context.Context, accountID int64, library string) error {
		_, err := s.Stars.Star(ctx, accountID, library)
		return err
	})
}

// unstarLibrary removes the signed-in visitor's star from the library the library parameter names, and returns as
// starLibrary does.
func (s *server) unstarLibrary(w http.ResponseWriter, r *http.Request) {
	s.changeStar(w, r, s.Stars.Unstar)
}

// changeStar applies change to the signed-in visitor's star on the library the library parameter names, and returns
// to the return parameter, or the library's page. A visitor who isn't signed in, such as one whose session ended in
// another tab, is sent to sign in and return there, and changes nothing; a library that can't be starred is missing.
func (s *server) changeStar(w http.ResponseWriter, r *http.Request, change func(ctx context.Context, accountID int64, library string) error) {
	query := r.URL.Query()
	library := query.Get("library")
	back := starReturn(query)
	v := visitorOf(r.Context())
	if v.account == nil {
		seeOther(w, r, s.starSignIn(back))
		return
	}
	err := change(r.Context(), v.account.ID, library)
	if errors.Is(err, app.ErrNotFound) {
		s.renderPrivate(w, r, http.StatusNotFound, messagePage(s.chrome, "Not found", "Rulemart stars only vetted libraries, and has none by that name."))
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	seeOther(w, r, back)
}

// starReturn returns where a star or unstar returns, by its query: the return parameter, as a path on this site, or
// else the page of the library the library parameter names, or home.
func starReturn(query url.Values) string {
	if query.Has("return") {
		return returnPath(query.Get("return"))
	}
	owner, name, ok := strings.Cut(query.Get("library"), "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return "/"
	}
	return returnPath(libraryHref(owner, name))
}

// starsPage shows the signed-in visitor's stars, or sends anyone else to sign in first.
func (s *server) starsPage(w http.ResponseWriter, r *http.Request) {
	account, ok := s.signedIn(w, r, starsHref)
	if !ok {
		return
	}
	stars, err := s.Stars.AccountStars(r.Context(), account.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.renderPrivate(w, r, http.StatusOK, starsPage(s.chrome, newStarredViews(stars)))
}

// starView is a library's star control: its stars, and a way to star or unstar it.
type starView struct {
	// shown is false when the page shows no stars: for a library that isn't vetted, or without stars.
	shown bool
	count int
	// starred is true when the signed-in visitor starred the library, and action is where the button posts to star
	// or unstar it. For a visitor who isn't signed in, signIn leads to sign in and return; with neither, as when no one
	// can sign in, the control shows the count alone.
	starred        bool
	action, signIn string
}

// starredView is one of the visitor's stars on their stars page.
type starredView struct {
	library libraryCard
	// unvetted is true for a library the release no longer vets that a listing names, whose link carries nofollow,
	// and gone for one neither vetted nor listed, which has no page, so library.href is empty.
	unvetted, gone bool
	starred        string
	// unstar is where its Unstar button posts.
	unstar string
}

// newStarredViews describes stars on the stars page, whose Unstar buttons return to it.
func newStarredViews(stars []views.StarredLibrary) []starredView {
	starred := make([]starredView, len(stars))
	for i, star := range stars {
		card := newLibraryCards([]views.LibraryCard{star.Library}, !star.Vetted)[0]
		gone := !star.Vetted && !star.Listed
		if gone {
			card.href = ""
		}
		starred[i] = starredView{
			library: card, unvetted: !star.Vetted && star.Listed, gone: gone, starred: date(star.StarredAt),
			unstar: starAction(unstarHref, star.Library.Owner+"/"+star.Library.Name, starsHref, ""),
		}
	}
	return starred
}
