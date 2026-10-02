// Sign in with GitHub, sign out, and the account page: the session cookie, the sign-in flow's state, and who each
// request is from.

package web

import (
	"cmp"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	accountsapp "github.com/fabricahq/rulemart/internal/contexts/accounts/app"
	accounts "github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/github"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
)

// Accounts signs visitors in and out. accounts/app.Sessions implements it.
type Accounts interface {
	// SignIn signs identity in with a new session, ending replacing's session, if any.
	SignIn(ctx context.Context, identity accounts.Identity, replacing accounts.SessionToken) (accounts.Account, accounts.Session, error)
	// Account returns the account token signs in, or fails with accountsapp.ErrSignedOut.
	Account(ctx context.Context, token accounts.SessionToken) (accounts.Account, error)
	SignOut(ctx context.Context, token accounts.SessionToken) error
	// SignOutEverywhere and DeleteAccount act on the account token signs in, only while its session lasts, and fail
	// with accountsapp.ErrSignedOut once it has ended.
	SignOutEverywhere(ctx context.Context, token accounts.SessionToken) error
	DeleteAccount(ctx context.Context, token accounts.SessionToken) error
}

// GitHub sends visitors to GitHub to sign in, and learns who they are from the code GitHub sends back.
// accounts/github.Client implements it.
type GitHub interface {
	// AuthorizationURL returns the GitHub page that asks the visitor to authorize Rulemart, and then sends them to
	// redirectURI with a code and state.
	AuthorizationURL(state, challenge, redirectURI string) string
	// Identify returns the user GitHub's code signs in.
	Identify(ctx context.Context, code, verifier, redirectURI string) (accounts.Identity, error)
}

const (
	// sessionCookie holds a signed-in browser's session token. __Host- makes browsers refuse it unless it's Secure,
	// for the whole site, and set by this host itself, so no other site under fabricahq.com can set or read it.
	sessionCookie = "__Host-rulemart-session"
	// signInCookie holds a sign-in in progress: its OAuth state, its PKCE verifier, and where to return.
	signInCookie = "__Host-rulemart-sign-in"
	// noticeCookie names a notice for the next page to show once, such as that the visitor signed out, by a key of
	// notices. Its value is never shown. CloudFront keys its cache on it, and the page that shows it clears it, which
	// keeps that one response out of every cache, so pages for everyone stay cached.
	noticeCookie = "__Host-rulemart-notice"
	// noticeLifetime is how long a notice waits for the page that shows it: the redirect to it.
	noticeLifetime = time.Minute
	// signInLifetime is how long a visitor has to authorize Rulemart on GitHub.
	signInLifetime = 10 * time.Minute
	// maxReturnLength bounds where a sign-in may return, well within a cookie's 4 KiB.
	maxReturnLength = 2000
)

const (
	// signInHref is the sign-in page, which takes where to return in its return parameter. POST to it starts
	// signing in with GitHub.
	signInHref = "/sign-in"
	// signOutHref signs out with POST, and returns to its return parameter.
	signOutHref = "/sign-out"
	// accountHref is the signed-in visitor's account page. GitHub has no account named account, so paths under it
	// can't hide a library's page.
	accountHref = "/account"
	// gitHubCallbackHref is where GitHub sends a visitor back with a code: the OAuth app's callback URL.
	gitHubCallbackHref = accountHref + "/github/callback"
	// signOutEverywhereHref and deleteAccountHref end every session of the account, and delete it, with POST.
	signOutEverywhereHref = accountHref + "/sign-out-everywhere"
	deleteAccountHref     = accountHref + "/delete"
)

// visitor is who a request is from, which the frame's header shows: a signed-in account, or a visitor who can sign
// in, or neither when sign-in isn't available.
type visitor struct {
	// account is the signed-in account, or nil.
	account *accounts.Account
	// token is the session cookie's token while account is signed in.
	token accounts.SessionToken
	// signIn is the sign-in page's address with this page to return to, or empty when sign-in isn't available.
	signIn string
	// withGitHub is true when signing in is with GitHub, rather than only as a local build's test users.
	withGitHub bool
	// signOut is where the sign-out form posts, with this page to return to.
	signOut string
	// here is the page's own address, as a return path, which its forms return to.
	here string
	// onAccountPage, onListingsPage, and onStarsPage are true on the account page, the listings page, and the stars
	// page, which the menu marks as current.
	onAccountPage, onListingsPage, onStarsPage bool
	// listings and stars are true when visitors can list and star libraries, so the menu links the listings and the
	// stars pages.
	listings, stars bool
	// onSignInPage is true on the sign-in page, whose header leaves its Sign in link out.
	onSignInPage bool
	// notice is a notice for this page to show once, from noticeCookie, or empty, and noticeKey is the key of notices
	// the cookie named, which the page clears as it renders.
	notice, noticeKey string
}

// notices are what a notice cookie may name, by key, and what each says.
var notices = map[string]string{
	"signed-out":               "You're signed out.",
	"signed-out-everywhere":    "You're signed out of every browser.",
	"account-deleted":          "Rulemart deleted your account and signed you out everywhere. Signing in again starts a new account.",
	"listed":                   "Listed. Rulemart is checking the repository, which usually takes a few seconds.",
	"listing-removed":          "Your listing is removed.",
	"listing-removed-listed":   "Your listing is removed. Its library no longer shows on Rulemart.",
	"listing-removed-vetted":   "Your listing is removed. Its library stays on Rulemart, which vetted it.",
	"listing-removed-checking": "Your listing is removed, and Rulemart stopped checking it.",
	"listing-retried":          "Rulemart is checking the repository again.",
	"listing-not-failed":       "That listing isn't failing any more, so there's nothing to try again.",
	"starred":                  "You starred this library. It's on Your stars.",
	"unstarred":                "You unstarred this library.",
	// A library's page says what follows signing in to star it, naming the library, as starPromptNotice does.
	starPromptKey: "You're signed in.",
}

// setNotice has the next page show the notice notices names by key, once.
func setNotice(w http.ResponseWriter, key string) {
	setCookie(w, noticeCookie, key, noticeLifetime)
}

type visitorKey struct{}

// visitorOf returns who the request whose context is ctx is from, or no one when the request wasn't a page's.
func visitorOf(ctx context.Context) visitor {
	v, _ := ctx.Value(visitorKey{}).(visitor)
	return v
}

// signInAvailable reports whether visitors can sign in: with GitHub, or in a local build as a test user.
func (s *server) signInAvailable() bool {
	return s.Accounts != nil && (s.GitHub != nil || DevSignIn)
}

// withVisitor finds who r is from, by its session cookie, before next shows a page. A path that holds text Postgres
// can't, which names nothing Rulemart has, is missing before next sees it; parameters are each handler's to read.
func (s *server) withVisitor(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r, ok := s.visit(w, r)
		if !ok {
			return
		}
		if !domain.Storable(r.URL.Path) {
			s.notFound(w, r)
			return
		}
		next(w, r)
	}
}

// visit returns r with who it's from in its context, by its session cookie, which visitorOf reads, so its page's
// header shows them. A cookie that no longer signs anyone in is cleared. A failure to read the session fails the
// request, rather than showing a signed-in visitor a page as if they weren't: visit answers it and returns false.
func (s *server) visit(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	back := returnPath(r.URL.RequestURI())
	v := visitor{
		here: back, onAccountPage: r.URL.Path == accountHref, onListingsPage: r.URL.Path == listingsHref,
		onStarsPage: r.URL.Path == starsHref, listings: s.listingAvailable(), stars: s.starsAvailable(),
	}
	if s.signInAvailable() {
		v.signIn = s.absolute(signInPageHref(back))
		v.withGitHub = s.GitHub != nil
	}
	if cookie, err := r.Cookie(noticeCookie); err == nil {
		// The page that shows a notice clears it as it renders, so a redirect first leaves it for the next page.
		if v.notice = notices[cookie.Value]; v.notice == "" {
			clearCookie(w, noticeCookie)
		} else {
			v.noticeKey = cookie.Value
		}
	}
	if s.Accounts != nil {
		if token, ok := sessionToken(r); ok {
			account, err := s.Accounts.Account(r.Context(), token)
			switch {
			case errors.Is(err, accountsapp.ErrSignedOut):
				clearCookie(w, sessionCookie)
			case err != nil:
				s.fail(w, r, err)
				return r, false
			default:
				v.account, v.token = &account, token
				v.signOut = signOutHref + returnQuery(publicPath(back))
			}
		} else if hasCookie(r, sessionCookie) {
			clearCookie(w, sessionCookie)
		}
	}
	return r.WithContext(context.WithValue(r.Context(), visitorKey{}, v)), true
}

// sessionToken returns r's session token, or false when it has none or the cookie can't hold one.
func sessionToken(r *http.Request) (accounts.SessionToken, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return "", false
	}
	return accounts.ParseSessionToken(cookie.Value)
}

// hasCookie reports whether r carries the cookie name, whatever its value.
func hasCookie(r *http.Request, name string) bool {
	_, err := r.Cookie(name)
	return err == nil
}

// absolute returns href on BaseURL when there is one, so a link followed from CloudFront's own domain reaches the
// public origin, where sign-in's cookies and GitHub's callback live.
func (s *server) absolute(href string) string {
	if s.BaseURL == nil {
		return href
	}
	return s.BaseURL.String() + href
}

// signInPageHref returns the sign-in page's address, to return to back.
func signInPageHref(back string) string {
	return signInHref + returnQuery(back)
}

// returnQuery returns the query string that names back as where to return, or nothing for the home page.
func returnQuery(back string) string {
	if back == "/" {
		return ""
	}
	return "?" + url.Values{"return": {back}}.Encode()
}

// returnPath returns target as a path on this site to return to after signing in or out, or / when it isn't one: an
// absolute URL, a path another host could take, such as //evil.example or /\evil.example, one with a backslash or a
// control character anywhere, one too long for the sign-in cookie, a page of the sign-in flow itself, or anything
// under /account/, which holds the flow's callback and actions that take POST, except the listings and stars pages.
// The account page itself is one.
func returnPath(target string) string {
	if target == "" || len(target) > maxReturnLength || !strings.HasPrefix(target, "/") || strings.HasPrefix(target, "//") ||
		strings.ContainsFunc(target, func(r rune) bool { return r == '\\' || r < 0x20 || r == 0x7f }) {
		return "/"
	}
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil || u.Opaque != "" {
		return "/"
	}
	if u.Path == signInHref || u.Path == signOutHref || (strings.HasPrefix(u.Path, accountHref+"/") && accountPages[u.Path] == "") {
		return "/"
	}
	u.Fragment, u.RawFragment = "", ""
	return u.String()
}

// accountPages are the pages under /account/ that only a signed-in visitor can see, which signing in may return to,
// and what the sign-in page says to a visitor on their way to each.
var accountPages = map[string]string{
	listingsHref: "Sign in to see your listings.",
	starsHref:    "Sign in to see your stars.",
}

// publicPath returns back, a return path, or / when back is the account page or one of accountPages, which a
// signed-out visitor can't see.
func publicPath(back string) string {
	if path, _, _ := strings.Cut(back, "?"); path == accountHref || accountPages[path] != "" {
		return "/"
	}
	return back
}

// signInPage shows the sign-in page, or returns a signed-in visitor where the return parameter says. Its to parameter
// may say why the visitor is signing in, such as starPurpose, which the page says.
func (s *server) signInPage(w http.ResponseWriter, r *http.Request) {
	back := returnPath(r.URL.Query().Get("return"))
	switch {
	case visitorOf(r.Context()).account != nil:
		seeOther(w, r, back)
	case !s.signInAvailable():
		s.renderSignIn(w, r, http.StatusNotFound, back, "")
	case publicPath(back) != back:
		path, _, _ := strings.Cut(back, "?")
		s.renderSignIn(w, r, http.StatusOK, back, cmp.Or(accountPages[path], "Sign in to see your account."))
	case r.URL.Query().Get("to") == starPurpose && back != "/":
		s.renderSignIn(w, r, http.StatusOK, back, "Sign in to star libraries. You'll come back to this one.")
	case r.URL.Query().Get("to") == starPurpose:
		// The return path was refused, so the page promises no return.
		s.renderSignIn(w, r, http.StatusOK, back, "Sign in to star libraries.")
	default:
		s.renderSignIn(w, r, http.StatusOK, back, "")
	}
}

// renderSignIn shows the sign-in page with status and notice, which says why the last attempt failed, if it did. It
// can't be cached, since a notice belongs to one visitor's attempt.
func (s *server) renderSignIn(w http.ResponseWriter, r *http.Request, status int, back, notice string) {
	view := signInView{notice: notice, available: s.signInAvailable()}
	if s.GitHub != nil {
		view.gitHub = signInHref + returnQuery(back)
		w.Header().Set("Content-Security-Policy", signInContentSecurityPolicy)
	}
	if s.Accounts != nil {
		view.testUsers = testUserViews(back)
	}
	// The page is the way to sign in, so its header doesn't link to it again, but keeps its place.
	v := visitorOf(r.Context())
	v.onSignInPage = true
	r = r.WithContext(context.WithValue(r.Context(), visitorKey{}, v))
	s.renderPrivate(w, r, status, signInPage(s.chrome, view))
}

// startSignIn sends the visitor to GitHub to authorize Rulemart. It keeps the flow's state, its PKCE verifier, and
// where to return in a short-lived cookie, which the callback checks, so only this browser can finish this sign-in.
func (s *server) startSignIn(w http.ResponseWriter, r *http.Request) {
	if s.GitHub == nil {
		s.notFound(w, r)
		return
	}
	back := returnPath(r.URL.Query().Get("return"))
	flow := signInFlow{state: github.NewState(), verifier: github.NewVerifier(), back: back}
	setCookie(w, signInCookie, flow.encode(), signInLifetime)
	seeOther(w, r, s.GitHub.AuthorizationURL(flow.state, github.Challenge(flow.verifier), s.callbackURL(r)))
}

// gitHubCallback finishes a sign-in GitHub sent back: it checks that this browser started it, has GitHub say who
// the visitor is, signs them in, and returns them where they started. Every way it fails clears the flow, and logs
// why, without the code, the state, or any token.
func (s *server) gitHubCallback(w http.ResponseWriter, r *http.Request) {
	if s.GitHub == nil {
		s.notFound(w, r)
		return
	}
	query := r.URL.Query()
	flow, ok := readSignInFlow(r)
	clearCookie(w, signInCookie)
	if !ok {
		flow.back = "/"
	}
	switch {
	case !ok:
		s.refuseSignIn(w, r, "no sign-in in progress", "/", expiredNotice)
		return
	case subtle.ConstantTimeCompare([]byte(query.Get("state")), []byte(flow.state)) != 1:
		s.refuseSignIn(w, r, "state mismatch", flow.back, expiredNotice)
		return
	case query.Get("error") == "access_denied":
		s.Log.InfoContext(r.Context(), "sign-in canceled", "route", s.route(r), "requestID", s.requestID(r))
		if !s.goOnSignedIn(w, r, flow.back) {
			s.renderSignIn(w, r, http.StatusOK, flow.back, canceledNotice)
		}
		return
	case query.Get("error") != "" || query.Get("code") == "":
		s.refuseSignIn(w, r, "GitHub returned no code", flow.back, failedNotice)
		return
	}
	identity, err := s.GitHub.Identify(r.Context(), query.Get("code"), flow.verifier, s.callbackURL(r))
	switch {
	case errors.Is(err, github.ErrCodeRefused):
		s.refuseSignIn(w, r, "GitHub refused the code", flow.back, incompleteNotice)
	case err != nil:
		s.Log.WarnContext(r.Context(), "sign-in failed", "route", s.route(r), "requestID", s.requestID(r), "error", err.Error())
		if !s.goOnSignedIn(w, r, flow.back) {
			s.renderSignIn(w, r, http.StatusBadGateway, flow.back, failedNotice)
		}
	default:
		s.signIn(w, r, identity, flow.back)
	}
}

// goOnSignedIn sends a visitor who is already signed in to back, and reports whether it did. A callback that can't
// complete leaves nothing for a signed-in visitor to do, such as one who went Back to it, so they go on as they were.
func (s *server) goOnSignedIn(w http.ResponseWriter, r *http.Request, back string) bool {
	if visitorOf(r.Context()).account == nil {
		return false
	}
	seeOther(w, r, back)
	return true
}

// signIn signs identity in, replacing the session the browser held, if any, and returns it to back.
func (s *server) signIn(w http.ResponseWriter, r *http.Request, identity accounts.Identity, back string) {
	replacing, _ := sessionToken(r)
	account, session, err := s.Accounts.SignIn(r.Context(), identity, replacing)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	setCookie(w, sessionCookie, string(session.Token), accounts.SessionLifetime)
	s.Log.InfoContext(r.Context(), "signed in", "route", s.route(r), "requestID", s.requestID(r), "accountID", account.ID)
	seeOther(w, r, back)
}

// refuseSignIn logs why a callback was refused, and shows the sign-in page with notice, or sends a visitor who is
// already signed in on to back.
func (s *server) refuseSignIn(w http.ResponseWriter, r *http.Request, reason, back, notice string) {
	s.Log.WarnContext(r.Context(), "sign-in refused", "route", s.route(r), "requestID", s.requestID(r), "reason", reason)
	if !s.goOnSignedIn(w, r, back) {
		s.renderSignIn(w, r, http.StatusBadRequest, back, notice)
	}
}

// What the sign-in page says when the last attempt failed.
const (
	expiredNotice  = "That sign-in expired, or started in another browser. Sign in again."
	canceledNotice = "You didn't authorize Rulemart on GitHub, so you're not signed in."
	failedNotice   = "GitHub couldn't confirm who you are. Try again in a minute."
	// incompleteNotice is for a code GitHub refused, such as one used already.
	incompleteNotice = "That sign-in didn't complete. Sign in again."
)

// callbackURL returns where GitHub sends the visitor back: on BaseURL, or without one, as locally, on the host the
// request came to over HTTP. GitHub accepts only the OAuth app's callback URL, so a forged host gets nowhere.
func (s *server) callbackURL(r *http.Request) string {
	if s.BaseURL != nil {
		return s.BaseURL.String() + gitHubCallbackHref
	}
	return "http://" + r.Host + gitHubCallbackHref
}

// signOut ends the browser's session, clears its cookie, and returns it to the return parameter.
func (s *server) signOut(w http.ResponseWriter, r *http.Request) {
	if token, ok := sessionToken(r); ok {
		if err := s.Accounts.SignOut(r.Context(), token); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	clearCookie(w, sessionCookie)
	setNotice(w, "signed-out")
	seeOther(w, r, publicPath(returnPath(r.URL.Query().Get("return"))))
}

// accountPage shows the signed-in visitor's account, or sends anyone else to sign in first.
func (s *server) accountPage(w http.ResponseWriter, r *http.Request) {
	account, ok := s.signedIn(w, r, accountHref)
	if !ok {
		return
	}
	s.renderPrivate(w, r, http.StatusOK, accountPage(s.chrome, newAccountView(account)))
}

// signOutEverywhere ends every session of the signed-in account, this browser's too, and returns home saying so. A
// visitor already signed out, such as from a tab left open after signing out elsewhere, goes home told they're
// signed out.
func (s *server) signOutEverywhere(w http.ResponseWriter, r *http.Request) {
	v := visitorOf(r.Context())
	if v.account == nil {
		s.signedOutAlready(w, r)
		return
	}
	err := s.Accounts.SignOutEverywhere(r.Context(), v.token)
	if errors.Is(err, accountsapp.ErrSignedOut) {
		s.signedOutAlready(w, r)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	clearCookie(w, sessionCookie)
	setNotice(w, "signed-out-everywhere")
	seeOther(w, r, "/")
}

// signedOutAlready answers an account action from a visitor whose session has ended: it clears the cookie, if any,
// and returns home, saying they're signed out.
func (s *server) signedOutAlready(w http.ResponseWriter, r *http.Request) {
	if hasCookie(r, sessionCookie) {
		clearCookie(w, sessionCookie)
	}
	setNotice(w, "signed-out")
	seeOther(w, r, "/")
}

// deleteAccount deletes the signed-in account and ends its sessions, then returns home, saying so. A visitor whose
// session has ended, even after this request began, may no longer act for the account, and goes home told they're
// signed out.
func (s *server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	v := visitorOf(r.Context())
	if v.account == nil {
		s.signedOutAlready(w, r)
		return
	}
	err := s.Accounts.DeleteAccount(r.Context(), v.token)
	if errors.Is(err, accountsapp.ErrSignedOut) {
		s.signedOutAlready(w, r)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.Log.InfoContext(r.Context(), "deleted account", "route", s.route(r), "requestID", s.requestID(r), "accountID", v.account.ID)
	clearCookie(w, sessionCookie)
	setNotice(w, "account-deleted")
	seeOther(w, r, "/")
}

// signedIn returns the signed-in account, or sends the visitor to sign in and return to back, and returns false.
func (s *server) signedIn(w http.ResponseWriter, r *http.Request, back string) (accounts.Account, bool) {
	v := visitorOf(r.Context())
	if v.account == nil {
		seeOther(w, r, s.absolute(signInPageHref(back)))
		return accounts.Account{}, false
	}
	return *v.account, true
}

// signInFlow is a sign-in in progress, which its cookie holds.
type signInFlow struct {
	state, verifier string
	// back is where to return once signed in, as returnPath allows.
	back string
}

// encode returns the flow as a cookie value: its state, verifier, and return path, the last base64url-encoded, joined
// by dots. The state and verifier are base64url text, which has no dots.
func (f signInFlow) encode() string {
	return f.state + "." + f.verifier + "." + base64.RawURLEncoding.EncodeToString([]byte(f.back))
}

// readSignInFlow returns the sign-in in progress that r's cookie holds, or false when it holds none.
func readSignInFlow(r *http.Request) (signInFlow, bool) {
	cookie, err := r.Cookie(signInCookie)
	if err != nil {
		return signInFlow{}, false
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" {
		return signInFlow{}, false
	}
	back, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return signInFlow{}, false
	}
	// The cookie is the browser's to change, so its return path is checked again.
	return signInFlow{state: parts[0], verifier: parts[1], back: returnPath(string(back))}, true
}

// setCookie sets the cookie name to value for lifetime: secure, for the whole site, out of reach of scripts, and sent
// with requests from other sites only when they navigate here, as GitHub's callback does.
func setCookie(w http.ResponseWriter, name, value string, lifetime time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: value, Path: "/", MaxAge: int(lifetime.Seconds()),
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

// clearCookie tells the browser to delete the cookie name.
func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

// seeOther answers with a redirect to target that a browser follows with GET, and that can't be cached.
func seeOther(w http.ResponseWriter, r *http.Request, target string) {
	w.Header().Set("Cache-Control", privateCache)
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// signInView is what the sign-in page shows.
type signInView struct {
	// notice says why the last attempt to sign in failed, or is empty.
	notice string
	// available is false when there's no way to sign in, and the page says so.
	available bool
	// gitHub is where the GitHub button posts, or empty when GitHub sign-in isn't configured.
	gitHub string
	// testUsers are the test users a local build can sign in as; release builds have none.
	testUsers []testUserView
}

// testUserView is a test user to sign in as, and where its button posts.
type testUserView struct {
	login, action string
}

// accountView is what the account page shows of the signed-in account.
type accountView struct {
	login, avatar, profileURL, gitHubUserID, since string
	// testUser is true for a local build's test user, which isn't a GitHub user, so the page links no profile.
	testUser bool
}

func newAccountView(account accounts.Account) accountView {
	return accountView{
		login: account.Login, avatar: account.AvatarURL, profileURL: account.ProfileURL(),
		gitHubUserID: strconv.FormatInt(account.GitHubUserID, 10), since: date(account.CreatedAt),
		testUser: isTestUser(account.GitHubUserID),
	}
}
