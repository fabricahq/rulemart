// Star and unstar the current rules of vetted libraries from their pages, and see the rules one starred.

package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// Stars stars rules for signed-in visitors. catalog/app.Stars implements it.
type Stars interface {
	// Star stars the current rule at rulePath in the vetted library library names, as owner/name, for the account, or
	// fails with app.ErrNotFound when there's no such rule. Starring a rule twice keeps one star.
	Star(ctx context.Context, accountID int64, library, rulePath string) error
	// Unstar removes every star of the account's that counts toward the rule Star finds, if it has any, or fails with
	// app.ErrNotFound as Star does.
	Unstar(ctx context.Context, accountID int64, library, rulePath string) error
	// Starred reports whether one of the account's stars counts toward the current rule at rulePath in the library
	// owner/name: one on the rule, or on a retired rule it replaced.
	Starred(ctx context.Context, accountID int64, owner, name, rulePath string) (bool, error)
	// AccountStars returns the current rules of vetted libraries the account's stars count toward, most recently
	// starred first.
	AccountStars(ctx context.Context, accountID int64) ([]views.StarredRule, error)
}

const (
	// starsHref stars, with POST, the rule its library and rule parameters name, as owner/name and its ID, and
	// unstarHref unstars it; both return to their return parameter, or the rule's page. GitHub has no account named
	// stars, so neither hides an owner's or a library's page.
	starsHref  = "/stars"
	unstarHref = starsHref + "/remove"
	// starredHref is the signed-in visitor's Starred rules.
	starredHref = accountHref + "/stars"
	// starPurpose is the sign-in page's to parameter for a visitor who signs in to star a rule.
	starPurpose = "star"
	// starPromptParam marks a rule page's address that a visitor returns to after signing in to star it. The page
	// takes it off, and prompts them, once, to star the rule.
	starPromptParam = "star"
	// starPromptKey is the notice that prompts a visitor who signed in to star a rule.
	starPromptKey = "star-prompt"
)

// starExplanation says what a star is for, beside the star button, and on the pages that list stars.
const starExplanation = "A star keeps a rule on your Starred rules, and tells others it's worth a look: they see only how many stars it has."

// starSignIn returns where a visitor who isn't signed in goes to sign in and star the rule whose page is back,
// returning to it with starPromptParam.
func (s *server) starSignIn(back string) string {
	if strings.Contains(back, "?") {
		back += "&" + starPromptParam + "=1"
	} else {
		back += "?" + starPromptParam + "=1"
	}
	return s.absolute(signInHref + "?" + url.Values{"return": {back}, "to": {starPurpose}}.Encode())
}

// starsAvailable reports whether visitors can star rules: sign-in is available, and so are stars.
func (s *server) starsAvailable() bool {
	return s.Stars != nil && s.signInAvailable()
}

// withoutStarPrompt answers a rule page's address with starPromptParam, which a visitor returns to after signing in
// to star it, and reports whether it did: it redirects to the address without it, with a prompt to star the rule for
// a signed-in visitor, so the prompt shows once, and reloading the page doesn't repeat it.
func (s *server) withoutStarPrompt(w http.ResponseWriter, r *http.Request) bool {
	if !r.URL.Query().Has(starPromptParam) {
		return false
	}
	// The rest of the query keeps its order, so the address is the one the visitor left. A name is compared decoded,
	// as Query reads it, so a parameter whose name is encoded, such as %73tar, comes off too, and the redirect can't
	// lead back to itself.
	var kept []string
	for pair := range strings.SplitSeq(r.URL.RawQuery, "&") {
		if name, _, _ := strings.Cut(pair, "="); !isParam(name, starPromptParam) {
			kept = append(kept, pair)
		}
	}
	// The path keeps the spelling the visitor's browser sent, which RawPath holds when it differs from Path's.
	target := url.URL{Path: r.URL.Path, RawPath: r.URL.RawPath, RawQuery: strings.Join(kept, "&")}
	if visitorOf(r.Context()).account == nil {
		redirect(w, r, target.String())
		return true
	}
	setNotice(w, starPromptKey)
	seeOther(w, r, target.String())
	return true
}

// isParam reports whether name, a query parameter's name as the address encodes it, decodes to param.
func isParam(name, param string) bool {
	decoded, err := url.QueryUnescape(name)
	return err == nil && decoded == param
}

// starControl returns the star control a rule's page shows of the rule r, for the page req asks for: for a signed-in
// visitor, whether their star counts toward it, which it reads. After starring or unstarring, the button is focused,
// and after signing in to star the rule, it's highlighted too, beside a prompt that names the rule. Only a current
// rule of a vetted library has one: for any other, it's the zero starView, which shows nothing.
func (s *server) starControl(req *http.Request, r ruleView, stars int) (starView, error) {
	if !r.library.vetted || r.retired != nil {
		return starView{}, nil
	}
	v := visitorOf(req.Context())
	star := starView{shown: true, count: stars, title: r.title}
	switch {
	case s.Stars != nil && v.account == nil && v.signIn != "":
		star.signIn = s.starSignIn(v.here)
		return star, nil
	case s.Stars == nil || v.account == nil:
		// The count alone, which shows nothing until the rule has stars.
		star.shown = stars > 0
		return star, nil
	}
	starred, err := s.Stars.Starred(req.Context(), v.account.ID, r.library.owner, r.library.name, r.id)
	if err != nil {
		return starView{}, err
	}
	star.starred = starred
	star.action = starAction(starsHref, r.library.fullName(), r.id, v.here, r.href)
	if starred {
		star.action = starAction(unstarHref, r.library.fullName(), r.id, v.here, r.href)
	}
	switch v.noticeKey {
	case "starred", "unstarred":
		star.focused = true
	case starPromptKey:
		star.focused, star.prompt = !starred, !starred
		star.notice = "You're signed in. Star " + r.title + "?"
		if starred {
			star.notice = "You're signed in. You've starred this rule already."
		}
	}
	return star, nil
}

// starAction returns where a form posts to star or unstar the rule at rulePath in the library fullName, whose page is
// rulePage, from the page here: path, starsHref or unstarHref, with the library and rule, and here to return to unless
// it's the rule's page, where a star returns anyway.
func starAction(path, fullName, rulePath, here, rulePage string) string {
	query := url.Values{"library": {fullName}, "rule": {rulePath}}
	if here != rulePage {
		query.Set("return", here)
	}
	return path + "?" + query.Encode()
}

// starRule stars the rule the library and rule parameters name for the signed-in visitor, and returns to the return
// parameter, or the rule's page, saying so.
func (s *server) starRule(w http.ResponseWriter, r *http.Request) {
	s.changeStar(w, r, true, s.Stars.Star)
}

// unstarRule removes the signed-in visitor's stars from the rule the library and rule parameters name, and returns as
// starRule does.
func (s *server) unstarRule(w http.ResponseWriter, r *http.Request) {
	s.changeStar(w, r, false, s.Stars.Unstar)
}

// changeStar applies change, which stars the rule the library and rule parameters name when star is true and
// unstars it otherwise, for the signed-in visitor, and returns to the return parameter, or the rule's page, which says
// what it did and focuses the star button. A visitor who isn't signed in, such as one whose session ended in another
// tab, is sent to sign in and return there, and changes nothing; a rule that can't be starred is missing.
func (s *server) changeStar(w http.ResponseWriter, r *http.Request, star bool, change func(ctx context.Context, accountID int64, library, rulePath string) error) {
	query := r.URL.Query()
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
	err := change(r.Context(), v.account.ID, query.Get("library"), query.Get("rule"))
	if errors.Is(err, app.ErrNotFound) {
		s.renderPrivate(w, r, http.StatusNotFound, messagePage(s.chrome, "Not found",
			"Rulemart has no rule here that can be starred. It stars only the current rules of vetted libraries."))
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if star {
		setNotice(w, "starred")
	} else {
		setNotice(w, "unstarred")
	}
	seeOther(w, r, back)
}

// starReturn returns where a star or unstar returns, by its query: the return parameter, as a path on this site, or
// else, as when the return parameter isn't one, the page of the rule the library and rule parameters name, or home.
func starReturn(query url.Values) string {
	if back := returnPath(query.Get("return")); query.Get("return") == "/" || back != "/" {
		return back
	}
	owner, name, ok := strings.Cut(query.Get("library"), "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") || query.Get("rule") == "" {
		return "/"
	}
	return returnPath(libraryHref(owner, name) + "/" + query.Get("rule"))
}

// starredPage shows the rules the signed-in visitor's stars count toward, or sends anyone else to sign in first.
func (s *server) starredPage(w http.ResponseWriter, r *http.Request) {
	account, ok := s.signedIn(w, r, starredHref)
	if !ok {
		return
	}
	starred, err := s.Stars.AccountStars(r.Context(), account.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.renderPrivate(w, r, http.StatusOK, starredPage(s.chrome, newStarredViews(starred, s.assets.iconURL)))
}

// newStarredViews describes the rules on a visitor's Starred rules as rule results. iconURL returns where the site
// serves an icon file.
func newStarredViews(starred []views.StarredRule, iconURL func(file string) string) []ruleResultView {
	results := make([]ruleResultView, len(starred))
	for i, s := range starred {
		results[i] = newRuleResult(s.Library, s.Rule, s.CanonicalGroup, iconURL)
		results[i].starredAs = s.StarredAs
	}
	return results
}

// starView is a rule's star control: its stars, and a way to star or unstar it.
type starView struct {
	// shown is false when the page shows no control: for a retired rule, one of a library that isn't vetted, or one
	// without stars where no one can star it.
	shown bool
	count int
	// title is the rule's title, which screen readers hear in the control's name.
	title string
	// starred is true when one of the signed-in visitor's stars counts toward the rule, and action is where the button
	// posts to star or unstar it. For a visitor who isn't signed in, signIn leads to sign in and return; with neither,
	// as when no one can sign in, the control shows the count alone.
	starred        bool
	action, signIn string
	// focused is true when the page follows starring, unstarring, or signing in to star, and focuses the button.
	// prompt is true when it follows signing in to star a rule the visitor hasn't starred, and highlights it, and
	// notice is the page's notice then, naming the rule.
	focused, prompt bool
	notice          string
}

// formatCount writes a count, which is never negative, as pages show it, with a comma between each group of three digits, such as 1,234.
func formatCount(n int) string {
	digits := strconv.Itoa(n)
	var out strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out.WriteByte(',')
		}
		out.WriteRune(d)
	}
	return out.String()
}

// starCountText says how many stars a rule has, in words, for screen readers and hover text.
func starCountText(count int) string {
	if count == 1 {
		return "1 star"
	}
	return formatCount(count) + " stars"
}
