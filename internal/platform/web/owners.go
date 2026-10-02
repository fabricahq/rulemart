// An owner's page: the vetted libraries a GitHub owner publishes, at /{owner}, or at /o/{login} for an owner whose
// login is the name of one of the site's own pages.

package web

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// ownerAliasPrefix starts every owner's other address, /o/{login}, the canonical one for an owner whose login a
// site page reserves.
const ownerAliasPrefix = "/o/"

// accountSections are the first segments of the account pages, reserved like siteSections, which the account routes
// take only when sign-in is available.
var accountSections = []string{strings.TrimPrefix(accountHref, "/"), strings.TrimPrefix(signInHref, "/"), strings.TrimPrefix(signOutHref, "/")}

// reservedOwner reports whether login, in any case, is the first segment of one of the site's own pages, so its
// owner's page can't be at /{login}. GitHub has users named g, faq, browse, list, and o, among others.
func reservedOwner(login string) bool {
	lower := strings.ToLower(login)
	if _, ok := siteSections[lower]; ok {
		return true
	}
	return slices.Contains(accountSections, lower)
}

// ownerHref is the path of an owner's page: /{login}, or /o/{login} when a site page reserves the login.
func ownerHref(login string) string {
	if reservedOwner(login) {
		return ownerAliasPrefix + url.PathEscape(login)
	}
	return "/" + url.PathEscape(login)
}

// ownerView is what an owner's page shows.
type ownerView struct {
	login, avatar, href, githubURL string
	libraries                      []libraryCard
}

func newOwnerView(page views.OwnerPage, libraries []libraryCard) ownerView {
	return ownerView{login: page.Login, avatar: page.AvatarURL, href: ownerHref(page.Login), githubURL: domain.OwnerURL(page.Login), libraries: libraries}
}

// owner shows the page of the owner the path names at /{owner}: the owner's login as GitHub spells it, their avatar,
// and their vetted libraries. An owner with no vetted library has no page, even with a listed one, so listing a
// repository can't create a page under Rulemart's address, and a login a site page reserves has its page at /o/
// instead. Another spelling of the login redirects to GitHub's.
func (s *server) owner(w http.ResponseWriter, r *http.Request) {
	login := r.PathValue("owner")
	if reservedOwner(login) {
		s.notFound(w, r)
		return
	}
	s.ownerPage(w, r, login)
}

// ownerAlias shows the page of the owner the path names at /o/{login} when a site page reserves the login, and
// otherwise redirects to the owner's own address.
func (s *server) ownerAlias(w http.ResponseWriter, r *http.Request) {
	login := r.PathValue("login")
	if !reservedOwner(login) {
		redirect(w, r, withQuery(ownerHref(login), r))
		return
	}
	s.ownerPage(w, r, login)
}

// ownerPage shows the owner login's page at its address, redirecting another spelling of the login there.
func (s *server) ownerPage(w http.ResponseWriter, r *http.Request, login string) {
	page, err := s.catalog.OwnerPage(r.Context(), login)
	if errors.Is(err, app.ErrNotFound) {
		s.notFound(w, r)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if page.Login != login {
		redirect(w, r, withQuery(ownerHref(page.Login), r))
		return
	}
	cards, err := s.vettedCards(r, page.Libraries)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	view := newOwnerView(page, cards)
	s.render(w, r, http.StatusOK, ownerPage(s.pageChrome(view.href), view))
}
