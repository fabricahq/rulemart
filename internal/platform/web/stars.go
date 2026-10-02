// Star and unstar vetted libraries from their pages, and see one's stars.

package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// Stars stars libraries for signed-in visitors. catalog/app.Stars implements it.
type Stars interface {
	// Star stars the vetted library library names, as owner/name, for the account, or fails with app.ErrNotFound
	// when there's no such vetted library. Starring a library twice keeps one star.
	Star(ctx context.Context, accountID int64, library string) (views.LibraryRef, error)
	// Unstar removes the account's star from the library library names, vetted or not, if it has one, or fails with
	// app.ErrNotFound when Rulemart has no library by that name.
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
	// starPurpose is the sign-in page's to parameter for a visitor who signs in to star a library.
	starPurpose = "star"
	// starPromptParam marks a library page's address that a visitor returns to after signing in to star it. The page
	// takes it off, and prompts them, once, to star the library.
	starPromptParam = "star"
	// starPromptKey is the notice that prompts a visitor who signed in to star a library.
	starPromptKey = "star-prompt"
	// starredHereKey and unstarredHereKey are the subjectNotices that the visitor starred or unstarred a library on
	// their stars page, which focuses its Unstar, or names it with Star again, focused.
	starredHereKey   = "starred-here"
	unstarredHereKey = "unstarred-here"
)

// starExplanation says what a star is for, beside the star button, and on the pages that list stars.
const starExplanation = "A star keeps a library on Your stars, and tells others it's worth a look: they see only how many stars it has."

// starSignIn returns where a visitor who isn't signed in goes to sign in and star the library whose page is back,
// returning to it with starPromptParam.
func (s *server) starSignIn(back string) string {
	if strings.Contains(back, "?") {
		back += "&" + starPromptParam + "=1"
	} else {
		back += "?" + starPromptParam + "=1"
	}
	return s.absolute(signInHref + "?" + url.Values{"return": {back}, "to": {starPurpose}}.Encode())
}

// starsAvailable reports whether visitors can star libraries: sign-in is available, and so are stars.
func (s *server) starsAvailable() bool {
	return s.Stars != nil && s.signInAvailable()
}

// withoutStarPrompt answers a library page's address with starPromptParam, which a visitor returns to after signing
// in to star it, and reports whether it did: it redirects to the address without it, with a prompt to star the
// library for a signed-in visitor, so the prompt shows once, and reloading the page doesn't repeat it.
func (s *server) withoutStarPrompt(w http.ResponseWriter, r *http.Request) bool {
	if !r.URL.Query().Has(starPromptParam) {
		return false
	}
	// The rest of the query keeps its order, so the address is the one the visitor left.
	var kept []string
	for pair := range strings.SplitSeq(r.URL.RawQuery, "&") {
		if name, _, _ := strings.Cut(pair, "="); name != starPromptParam {
			kept = append(kept, pair)
		}
	}
	target := url.URL{Path: r.URL.EscapedPath(), RawQuery: strings.Join(kept, "&")}
	if visitorOf(r.Context()).account == nil {
		redirect(w, r, target.String())
		return true
	}
	setNotice(w, starPromptKey)
	seeOther(w, r, target.String())
	return true
}

// libraryView describes lib for the page r asks for, with the star control a vetted library's pages show: for a
// signed-in visitor, whether they starred it, which it reads. After starring or unstarring, the button is focused,
// and after signing in to star the library, it's highlighted too, beside a prompt that names the library.
func (s *server) libraryView(r *http.Request, lib views.Library) (libraryView, error) {
	view := newLibraryView(lib)
	if !lib.Vetted || s.Stars == nil {
		return view, nil
	}
	v := visitorOf(r.Context())
	view.star = starView{shown: true, count: lib.Stars, fullName: lib.FullName()}
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
	switch v.noticeKey {
	case "starred", "unstarred":
		view.star.focused = true
	case starPromptKey:
		view.star.focused, view.star.prompt = !starred, !starred
		view.star.notice = "You're signed in. Star " + lib.FullName() + "?"
		if starred {
			view.star.notice = "You're signed in. You've starred this library already."
		}
	}
	return view, nil
}

// vettedCards describes vetted libraries for a list of them that r asks for, with their stars only when Rulemart has
// stars, as their own pages show them, and for a signed-in visitor, which they starred, which it reads.
func (s *server) vettedCards(r *http.Request, libraries []views.LibraryCard) ([]libraryCard, error) {
	cards := newLibraryCards(libraries, false)
	if s.Stars == nil {
		for i := range cards {
			cards[i].stars = 0
		}
		return cards, nil
	}
	v := visitorOf(r.Context())
	if v.account == nil {
		return cards, nil
	}
	stars, err := s.Stars.AccountStars(r.Context(), v.account.ID)
	if err != nil {
		return nil, err
	}
	mine := map[string]bool{}
	for _, star := range stars {
		mine[strings.ToLower(star.Library.Owner+"/"+star.Library.Name)] = true
	}
	for i, card := range cards {
		cards[i].starredByYou = mine[strings.ToLower(card.owner+"/"+card.name)]
	}
	return cards, nil
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
// parameter, or the library's page, saying so.
func (s *server) starLibrary(w http.ResponseWriter, r *http.Request) {
	s.changeStar(w, r, true, func(ctx context.Context, accountID int64, library string) error {
		_, err := s.Stars.Star(ctx, accountID, library)
		return err
	})
}

// unstarLibrary removes the signed-in visitor's star from the library the library parameter names, and returns as
// starLibrary does; to the stars page, it returns naming the library, which the page offers to star again.
func (s *server) unstarLibrary(w http.ResponseWriter, r *http.Request) {
	s.changeStar(w, r, false, s.Stars.Unstar)
}

// changeStar applies change, which stars the library the library parameter names when star is true and unstars it
// otherwise, to the signed-in visitor's star, and returns to the return parameter, or the library's page, which says
// what it did and focuses the star button. A visitor who isn't signed in, such as one whose session ended in another
// tab, is sent to sign in and return there, and changes nothing; a library that can't be starred or unstarred is
// missing.
func (s *server) changeStar(w http.ResponseWriter, r *http.Request, star bool, change func(ctx context.Context, accountID int64, library string) error) {
	query := r.URL.Query()
	library := query.Get("library")
	back := starReturn(query)
	v := visitorOf(r.Context())
	if v.account == nil {
		if star {
			seeOther(w, r, s.starSignIn(back))
		} else {
			seeOther(w, r, s.absolute(signInPageHref(back)))
		}
		return
	}
	err := change(r.Context(), v.account.ID, library)
	if errors.Is(err, app.ErrNotFound) {
		s.renderPrivate(w, r, http.StatusNotFound, messagePage(s.chrome, "Not found", noSuchLibrary(library, star)))
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	switch path, _, _ := strings.Cut(back, "?"); {
	case path == starsHref && !star:
		back = starsHref
		setSubjectNotice(w, unstarredHereKey, library)
	case path == starsHref:
		back = starsHref
		setSubjectNotice(w, starredHereKey, library)
	case star:
		setNotice(w, "starred")
	default:
		setNotice(w, "unstarred")
	}
	seeOther(w, r, back)
}

// noSuchLibrary says Rulemart has no library named library to star, when star is true, or to unstar.
func noSuchLibrary(library string, star bool) string {
	name := "by that name"
	if domain.Storable(library) && len(library) <= maxReturnLength {
		name = "named " + library
	}
	if star {
		return "Rulemart has no library " + name + " that can be starred. It stars only vetted libraries."
	}
	return "Rulemart has no library " + name + "."
}

// starReturn returns where a star or unstar returns, by its query: the return parameter, as a path on this site, or
// else, as when the return parameter isn't one, the page of the library the library parameter names, or home.
func starReturn(query url.Values) string {
	if back := returnPath(query.Get("return")); query.Get("return") == "/" || back != "/" {
		return back
	}
	owner, name, ok := strings.Cut(query.Get("library"), "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return "/"
	}
	return returnPath(libraryHref(owner, name))
}

// starsPage shows the signed-in visitor's stars, or sends anyone else to sign in first. Following unstarring a library
// here, it says so, and offers to star it again, focused; following starring it again, it focuses its Unstar.
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
	view := starsView{stars: newStarredViews(stars, time.Now())}
	switch v := visitorOf(r.Context()); v.noticeKey {
	case unstarredHereKey:
		if !starredAmong(stars, v.noticeSubject) {
			view.unstarred, view.starAgain = v.noticeSubject, starAction(starsHref, v.noticeSubject, starsHref, "")
		}
	case starredHereKey:
		for i := range view.stars {
			view.stars[i].focused = strings.EqualFold(view.stars[i].library.owner+"/"+view.stars[i].library.name, v.noticeSubject)
		}
	}
	s.renderPrivate(w, r, http.StatusOK, starsPage(s.chrome, view))
}

// namesLibrary reports whether text names a library as owner/name, in names GitHub allows, and nothing else.
func namesLibrary(text string) bool {
	owner, name, err := domain.ParseListedRepository(text)
	return err == nil && strings.EqualFold(owner+"/"+name, text)
}

// starredAmong reports whether stars holds the library fullName names, without regard to case.
func starredAmong(stars []views.StarredLibrary, fullName string) bool {
	for _, star := range stars {
		if strings.EqualFold(star.Library.Owner+"/"+star.Library.Name, fullName) {
			return true
		}
	}
	return false
}

// starView is a library's star control: its stars, and a way to star or unstar it.
type starView struct {
	// shown is false when the page shows no stars: for a library that isn't vetted, or without stars.
	shown    bool
	count    int
	fullName string
	// starred is true when the signed-in visitor starred the library, and action is where the button posts to star
	// or unstar it. For a visitor who isn't signed in, signIn leads to sign in and return; with neither, as when no one
	// can sign in, the control shows the count alone.
	starred        bool
	action, signIn string
	// focused is true when the page follows starring, unstarring, or signing in to star, and focuses the button.
	// prompt is true when it follows signing in to star a library the visitor hasn't starred, and highlights it, and
	// notice is the page's notice then, naming the library.
	focused, prompt bool
	notice          string
}

// starsView is the stars page: the visitor's stars, and the library they unstarred there, if any, with where its
// Star again button posts.
type starsView struct {
	stars                []starredView
	unstarred, starAgain string
}

// starredView is one of the visitor's stars on their stars page.
type starredView struct {
	library libraryCard
	// unvetted is true for a library the release no longer vets that a listing names, whose link carries nofollow,
	// and gone for one neither vetted nor listed, which has no page, so library.href is empty.
	unvetted, gone bool
	// starred says when the visitor starred it, in words, starredAt exactly, for machines, and starredTitle readably,
	// to the minute in UTC.
	starred, starredAt, starredTitle string
	// focused is true for the library the visitor just starred again here, whose Unstar the page focuses.
	focused bool
	// unstar is where its Unstar button posts.
	unstar string
}

// newStarredViews describes stars on the stars page as of now, whose Unstar buttons return to it.
func newStarredViews(stars []views.StarredLibrary, now time.Time) []starredView {
	starred := make([]starredView, len(stars))
	for i, star := range stars {
		card := newLibraryCards([]views.LibraryCard{star.Library}, !star.Vetted)[0]
		gone := !star.Vetted && !star.Listed
		if gone {
			card.href = ""
		}
		starred[i] = starredView{
			library: card, unvetted: !star.Vetted && star.Listed, gone: gone,
			starred: since(star.StarredAt, now), starredAt: star.StarredAt.UTC().Format(time.RFC3339),
			starredTitle: star.StarredAt.UTC().Format("2 Jan 2006, 15:04") + " UTC",
			unstar:       starAction(unstarHref, star.Library.Owner+"/"+star.Library.Name, starsHref, ""),
		}
	}
	return starred
}

// since says when t was, as of now: just now, minutes or hours ago within a day, so stars made the same day show their
// order, and the date after that.
func since(t, now time.Time) string {
	switch age := now.Sub(t); {
	case age < time.Minute:
		return "just now"
	case age < time.Hour:
		return plural(int(age/time.Minute), "minute", "minutes") + " ago"
	case age < 24*time.Hour:
		return plural(int(age/time.Hour), "hour", "hours") + " ago"
	}
	return "on " + date(t)
}

// starCountText says how many stars a library has, and whether the visitor starred it, for screen readers.
func starCountText(count int, mine bool) string {
	text := plural(count, "star", "stars")
	if mine {
		text += ", starred by you"
	}
	return text
}
