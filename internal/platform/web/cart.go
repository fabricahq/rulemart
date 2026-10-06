// Check out the cart a visitor's browser keeps: the cart's page posts its keys and choices, and the answer resolves
// each against the catalog and holds the prompt and commands that import it.

package web

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strings"

	accountsapp "github.com/fabricahq/rulemart/internal/contexts/accounts/app"
	accounts "github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// Carts checks out the carts visitors' browsers keep. catalog/app.Carts implements it.
type Carts interface {
	// Checkout resolves cart against the catalog, with the texts that import it into target, or fails with
	// app.ErrCartTooLarge when it holds more than domain.MaxCartItems keys.
	Checkout(ctx context.Context, cart app.Cart, target domain.CheckoutTarget) (views.Checkout, error)
}

const (
	// cartHref is the cart's page.
	cartHref = "/cart"
	// checkoutHref answers a POST of a cart, as checkoutRequest describes it, with its checkout, as checkoutResponse
	// does.
	checkoutHref = cartHref + "/checkout.json"
	// legacyCartHref and legacyCheckoutHref are the signed-in cart's old page and checkout, which redirect to cartHref.
	legacyCartHref     = accountHref + "/cart"
	legacyCheckoutHref = legacyCartHref + "/checkout"
)

// maxCheckoutBytes bounds a checkout's request: room for domain.MaxCartItems keys of domain.MaxCartKeyLength
// characters, twice, as the cart and its forks, and the rest of the choices.
const maxCheckoutBytes = 4 * domain.MaxCartItems * domain.MaxCartKeyLength

// checkoutRequest is what the cart's page posts: the cart as the browser keeps it, in localStorage's rulemart-cart.
type checkoutRequest struct {
	// Cart is the cart's keys, in order, as domain.CartItem's Key writes them.
	Cart []string `json:"cart"`
	// Fork marks the keys of the rules the visitor forks. RestOfGroups marks the libraries, as owner/name, whose rules'
	// groups the visitor adds the rest of, and Confirmed the libraries the visitor confirmed adding from though Rulemart
	// doesn't vet them; each names at most domain.MaxCartItems libraries, as many as a cart's items can, each in at
	// most domain.MaxCartKeyLength characters, since checkout looks each library up in them.
	Fork         map[string]bool `json:"fork"`
	RestOfGroups map[string]bool `json:"restOfGroups"`
	Confirmed    map[string]bool `json:"confirmed"`
	// Repo is what the visitor wrote as their project's repository, which may name none.
	Repo string `json:"repo"`
	// Project is the signed-in visitor's project the checkout is for, as owner/name, new for one that doesn't use Code
	// Rules yet, which Repo then names, or empty for the first of their projects.
	Project string `json:"project"`
}

// checkoutResponse is a cart's checkout, as cart-page.js shows it.
type checkoutResponse struct {
	Libraries []checkoutLibraryJSON `json:"libraries"`
	// Unknown are the cart's keys that name nothing a cart can hold, which cart-page.js drops.
	Unknown []string `json:"unknown"`
	// Repository is the project's repository, as owner/name, which the texts name, or empty. RepositoryInvalid is
	// true when the visitor wrote one that names no GitHub repository.
	Repository        string `json:"repository"`
	RepositoryInvalid bool   `json:"repositoryInvalid"`
	// Mode is known for one of the visitor's projects, new for one that doesn't use Code Rules yet, and unknown for one
	// Rulemart knows nothing of.
	Mode string `json:"mode"`
	// Prompt and Commands import every ready item, and are empty when none is. Commands are steps the Commands tab
	// shows apart, each with its own Copy, as domain.Checkout's Commands says.
	Prompt   string            `json:"prompt"`
	Commands []commandStepJSON `json:"commands"`
	// Pin is the release the Commands tab's footnote suggests pinning a library to, or nil.
	Pin *pinJSON `json:"pin"`
}

// commandStepJSON is one step of a checkout's commands: its heading, empty when the commands are one step, and its
// commands.
type commandStepJSON struct {
	Heading  string `json:"heading"`
	Commands string `json:"commands"`
}

// pinJSON is a library's latest release, which a project could pin it to: the library, as owner/name, the release's
// tag, and the option of the library's add library command that pins it.
type pinJSON struct {
	Library string `json:"library"`
	Release string `json:"release"`
	Option  string `json:"option"`
}

// checkoutLibraryJSON is a library a cart names, with the cart's items from it.
type checkoutLibraryJSON struct {
	// Owner and Name are spelled as GitHub spells them now, or as the cart does when Gone, and FullName is both as
	// owner/name. Href is the library's page, and Avatar its owner's, both empty when Gone: the library is neither
	// vetted nor listed, so Rulemart has no page for it.
	Owner    string `json:"owner"`
	Name     string `json:"name"`
	FullName string `json:"fullName"`
	Href     string `json:"href"`
	Avatar   string `json:"avatar"`
	Gone     bool   `json:"gone"`
	// Vetted is false for a library Rulemart doesn't vet, whose items checkout leaves out until Confirmed.
	Vetted    bool               `json:"vetted"`
	Confirmed bool               `json:"confirmed"`
	Items     []checkoutItemJSON `json:"items"`
	// RestOfGroups offers to add the rest of the groups of the library's rules that stay in sync, or is nil.
	RestOfGroups *restOfGroupsJSON `json:"restOfGroups"`
}

// checkoutItemJSON is one item of a cart.
type checkoutItemJSON struct {
	// Key is as the cart sent it; Kind is rule or group.
	Key  string `json:"key"`
	Kind string `json:"kind"`
	// State is ready, retired, missing, gone, or unvetted, as views.CartItemState says.
	State string `json:"state"`
	// ID is the rule's or group's ID; Title a rule's title, or its ID when the catalog doesn't have its title, or a
	// group's name, or its ID when it isn't canonical; Href the item's page, empty when it has none.
	ID    string    `json:"id"`
	Title string    `json:"title"`
	Href  string    `json:"href"`
	Group groupJSON `json:"group"`
	// Version is a rule's newest version, Fork whether the visitor forks it, InGroup whether the cart holds its group
	// whole, which brings it, and RetiredIn the tag of the release that retired it, if one did.
	Version   string `json:"version,omitempty"`
	Fork      bool   `json:"fork,omitempty"`
	InGroup   bool   `json:"inGroup,omitempty"`
	RetiredIn string `json:"retiredIn,omitempty"`
	// Rules are the first previewRules of the current rules a whole group brings, and RuleCount how many it brings.
	Rules     []ruleLinkJSON `json:"rules,omitempty"`
	RuleCount int            `json:"ruleCount,omitempty"`
}

// previewRules is how many of a whole group's rules its item lists, as the cart's page shows them, which links the rest
// to the group's page: a group may bring thousands, and the answer must fit a response.
const previewRules = 5

// groupJSON is a group as the cart shows it: its ID, its name, which is its ID when it isn't canonical, and its icon.
type groupJSON struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Canonical bool      `json:"canonical"`
	Icon      *iconJSON `json:"icon"`
}

// iconJSON is a group's icon, as groupTile draws it.
type iconJSON struct {
	Src        string `json:"src"`
	Monochrome bool   `json:"monochrome"`
	Narrow     bool   `json:"narrow"`
	LightTile  bool   `json:"lightTile"`
}

// ruleLinkJSON is a rule a group brings, by its title, or its ID when the catalog doesn't have its title, and its page.
type ruleLinkJSON struct {
	Title string `json:"title"`
	Href  string `json:"href"`
}

// restOfGroupsJSON offers to add the other rules of Groups, by name, Rules of them, or says the visitor did when Added.
type restOfGroupsJSON struct {
	Groups []string `json:"groups"`
	Rules  int      `json:"rules"`
	Added  bool     `json:"added"`
}

// cartPage shows the cart's page, which cart-page.js fills from the cart the browser keeps. It's the same for every
// visitor but in where the cart's rules go, which offers to sign in to anyone who isn't, and offers a signed-in
// visitor their projects, from what Rulemart read of their GitHub account.
func (s *server) cartPage(w http.ResponseWriter, r *http.Request) {
	v := visitorOf(r.Context())
	view := projectsView{signIn: v.signIn, withGitHub: v.withGitHub, signedIn: v.account != nil}
	if v.account != nil {
		gitHub, ok := s.gitHubView(w, r, *v.account, cartHref)
		if !ok {
			return
		}
		view.gitHub = gitHub
		for _, p := range gitHub.snapshot.Projects {
			view.projects = append(view.projects, newProjectOption(p))
		}
	}
	s.render(w, r, http.StatusOK, cartPage(s.chrome, view))
}

// projectsView is what checkout's Where it goes shows.
type projectsView struct {
	// signIn is where signing in starts, empty when sign-in isn't available, and withGitHub is true when it's with
	// GitHub. signedIn is true for a signed-in visitor.
	signIn               string
	withGitHub, signedIn bool
	// gitHub is what Rulemart read of a signed-in visitor's GitHub account, and projects their projects in it.
	gitHub   gitHubView
	projects []projectOption
}

// projectOption is one of the visitor's projects, as checkout's picker offers it.
type projectOption struct {
	repository string
	private    bool
	// uses names the libraries it imports.
	uses string
}

func newProjectOption(p accounts.Project) projectOption {
	var uses []string
	for _, source := range p.Sources {
		uses = append(uses, cmp.Or(source.Library, source.Name))
	}
	return projectOption{repository: p.FullName(), private: p.Private, uses: strings.Join(uses, ", ")}
}

// redirectToCart redirects the signed-in cart's old addresses to the cart's page, keeping the query.
func (s *server) redirectToCart(w http.ResponseWriter, r *http.Request) {
	redirect(w, r, withQuery(cartHref, r))
}

// checkout answers a cart that the cart's page posts, as checkoutRequest describes it, with its checkout. Another site
// can't post one (withSameOriginWrites), and the answer is the visitor's alone, so nothing caches it. A request that
// isn't such a cart, holds more than domain.MaxCartItems keys, or names more libraries than checkoutRequest allows, is
// refused with 400.
//
// It's the site's only request with a body, so the script that posts it, cart-checkout.js, sends the body's SHA-256 in
// x-amz-content-sha256: CloudFront's origin access control signs a request's body only when the viewer sends that
// header, and the function URL refuses a body it didn't sign with 403 before this handler runs. Nothing here reads the
// header, and nothing local checks it, so only production breaks if the script stops sending it.
func (s *server) checkout(w http.ResponseWriter, r *http.Request) {
	if media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || media != "application/json" {
		s.writeJSON(w, r, http.StatusUnsupportedMediaType, map[string]string{"error": "send the cart as application/json"})
		return
	}
	var req checkoutRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCheckoutBytes))
	if err := decoder.Decode(&req); err != nil || !boundedLibraries(req.RestOfGroups) || !boundedLibraries(req.Confirmed) {
		s.writeJSON(w, r, http.StatusBadRequest, map[string]string{"error": "the cart isn't one Rulemart can read"})
		return
	}
	target, repositoryInvalid, ok := s.checkoutTarget(w, r, req)
	if !ok {
		return
	}
	checkout, err := s.Carts.Checkout(r.Context(), app.Cart{Keys: req.Cart, Forks: req.Fork, RestOfGroups: req.RestOfGroups, Confirmed: req.Confirmed}, target)
	switch {
	case errors.Is(err, app.ErrCartTooLarge):
		s.writeJSON(w, r, http.StatusBadRequest, map[string]string{"error": "the cart holds more items than it may"})
		return
	case err != nil:
		s.logFailure(r, err)
		s.writeJSON(w, r, http.StatusServiceUnavailable, checkoutUnavailable)
		return
	}
	s.writeJSON(w, r, http.StatusOK, newCheckoutResponse(checkout, target, repositoryInvalid, s.assets.iconURL))
}

// boundedLibraries reports whether libraries, one of checkoutRequest's choices that name libraries, stays within the
// bounds it says.
func boundedLibraries(libraries map[string]bool) bool {
	if len(libraries) > domain.MaxCartItems {
		return false
	}
	for name := range libraries {
		if len(name) > domain.MaxCartKeyLength {
			return false
		}
	}
	return true
}

// checkoutUnavailable answers a checkout Rulemart can't give, which cart-page.js shows as being unable to show the cart.
var checkoutUnavailable = map[string]string{"error": "Rulemart can't check out right now"}

// checkoutTarget returns the project req's checkout is for, and whether what the visitor wrote as its repository names
// none though it isn't empty: for a signed-in visitor with projects, the one req names, or the first, whose sources the
// texts add to, unless req says it's a new project; otherwise the repository the visitor wrote, which Rulemart knows
// nothing of, as it is when Rulemart has no read of their projects to offer: the read failed, another is under way, or
// the visitor must sign in again, where the page offers to enter a repository instead. It answers the request with a
// failure, and returns false, when reading the visitor's projects fails otherwise.
func (s *server) checkoutTarget(w http.ResponseWriter, r *http.Request, req checkoutRequest) (domain.CheckoutTarget, bool, bool) {
	v := visitorOf(r.Context())
	if v.account == nil || s.GitHubAccounts == nil {
		target, invalid := writtenTarget(req.Repo, domain.ProjectUnknown)
		return target, invalid, true
	}
	snapshot, err := s.GitHubAccounts.Snapshot(r.Context(), *v.account, v.token)
	if err != nil && !errors.Is(err, accountsapp.ErrNoGitHubToken) && !errors.Is(err, accountsapp.ErrGitHubRead) &&
		!errors.Is(err, accountsapp.ErrGitHubReading) {
		s.logFailure(r, err)
		s.writeJSON(w, r, http.StatusServiceUnavailable, checkoutUnavailable)
		return domain.CheckoutTarget{}, false, false
	}
	if len(snapshot.Projects) == 0 {
		target, invalid := writtenTarget(req.Repo, domain.ProjectUnknown)
		return target, invalid, true
	}
	if req.Project == "new" {
		target, invalid := writtenTarget(req.Repo, domain.ProjectNew)
		return target, invalid, true
	}
	project := snapshot.Projects[0]
	for _, p := range snapshot.Projects {
		if strings.EqualFold(p.FullName(), req.Project) {
			project = p
		}
	}
	return knownTarget(project), false, true
}

// knownTarget returns the checkout target of the visitor's project p, which imports its sources under their names.
func knownTarget(p accounts.Project) domain.CheckoutTarget {
	target := domain.CheckoutTarget{Mode: domain.ProjectKnown, Repository: p.FullName(), Sources: map[string]string{}}
	for _, source := range p.Sources {
		if source.Library != "" {
			target.Sources[strings.ToLower(source.Library)] = source.Name
		}
	}
	return target
}

// writtenTarget returns a checkout target of mode, from repo, what the visitor wrote as its repository, and whether
// repo names none though it isn't empty.
func writtenTarget(repo string, mode domain.ProjectMode) (domain.CheckoutTarget, bool) {
	target := domain.CheckoutTarget{Mode: mode}
	if repo == "" {
		return target, false
	}
	owner, name, err := domain.ParseGitHubRepository(repo)
	if err != nil {
		return target, true
	}
	target.Repository = owner + "/" + name
	return target, false
}

// newCheckoutResponse describes checkout, for target, as cart-page.js reads it. iconURL returns where the site serves
// an icon file.
func newCheckoutResponse(checkout views.Checkout, target domain.CheckoutTarget, repositoryInvalid bool, iconURL func(string) string) checkoutResponse {
	resp := checkoutResponse{
		Libraries: []checkoutLibraryJSON{}, Unknown: checkout.Unknown, Repository: target.Repository, Mode: string(target.Mode),
		RepositoryInvalid: repositoryInvalid, Prompt: checkout.Prompt, Commands: []commandStepJSON{},
	}
	if resp.Unknown == nil {
		resp.Unknown = []string{}
	}
	for _, step := range checkout.Commands {
		resp.Commands = append(resp.Commands, commandStepJSON{Heading: step.Heading, Commands: step.Commands})
	}
	if pin := checkout.PinExample; pin != nil {
		resp.Pin = &pinJSON{Library: pin.Library, Release: domain.ReleaseTag(pin.Release), Option: pin.Option()}
	}
	for _, lib := range checkout.Libraries {
		l := checkoutLibraryJSON{
			Owner: lib.Library.Owner, Name: lib.Library.Name, FullName: lib.Library.FullName(), Gone: lib.Gone,
			Vetted: lib.Vetted, Confirmed: lib.Confirmed,
		}
		href := libraryHref(lib.Library.Owner, lib.Library.Name)
		if !lib.Gone {
			l.Href, l.Avatar = href, lib.Library.OwnerAvatarURL
		}
		for _, it := range lib.Items {
			l.Items = append(l.Items, newCheckoutItemJSON(it, href, lib.Gone, iconURL))
		}
		if lib.RestOfGroupsRules > 0 || (lib.RestOfGroupsAdded && len(lib.RestOfGroups) > 0) {
			rest := &restOfGroupsJSON{Rules: lib.RestOfGroupsRules, Added: lib.RestOfGroupsAdded}
			for _, g := range lib.RestOfGroups {
				rest.Groups = append(rest.Groups, newGroupLabel(g.Path, g.Canonical).display())
			}
			l.RestOfGroups = rest
		}
		resp.Libraries = append(resp.Libraries, l)
	}
	return resp
}

// newCheckoutItemJSON describes it, an item of the library whose page is libraryHref, which is gone or not.
func newCheckoutItemJSON(it views.ResolvedItem, libraryHref string, gone bool, iconURL func(string) string) checkoutItemJSON {
	label := newGroupLabel(it.Group.Path, it.Group.Canonical)
	item := checkoutItemJSON{
		Key: it.Key, Kind: string(it.Item.Kind), State: string(it.State), ID: it.Item.Path,
		Group: groupJSON{ID: label.id, Name: label.display(), Canonical: label.canonical}, Fork: it.Fork, InGroup: it.InGroup,
	}
	if icon := newGroupIcon(it.Group.Canonical, iconURL); icon.src != "" {
		item.Group.Icon = &iconJSON{Src: icon.src, Monochrome: icon.monochrome, Narrow: icon.narrow, LightTile: icon.lightTile}
	}
	switch it.Item.Kind {
	case domain.CartGroup:
		item.Title = label.display()
		if it.State != views.CartItemMissing && !gone {
			item.Href = libraryGroupHref(libraryHref, it.Item.Path)
		}
		for _, r := range it.Rules[:min(len(it.Rules), previewRules)] {
			item.Rules = append(item.Rules, ruleLinkJSON{Title: titleOrID(r.Title, r.Path), Href: ruleHref(libraryHref, r.Path)})
		}
		item.RuleCount = len(it.Rules)
	case domain.CartRule:
		item.Title = titleOrID(it.Title, it.Item.Path)
		if it.State != views.CartItemMissing && !gone {
			item.Href = ruleHref(libraryHref, it.Item.Path)
		}
		if it.Version != (coderules.RuleVersion{}) {
			item.Version = it.Version.String()
		}
		if it.RetiredIn > 0 {
			item.RetiredIn = domain.ReleaseTag(it.RetiredIn)
		}
	}
	return item
}

// writeJSON answers r with status and value as JSON, which no cache keeps. It encodes the whole answer before writing,
// so an answer larger than maxPageBytes, which the bounds on what a checkout lists keep it from reaching, is refused,
// before anything of it is written, as a checkout Rulemart can't give.
func (s *server) writeJSON(w http.ResponseWriter, r *http.Request, status int, value any) {
	body, err := json.Marshal(value)
	switch {
	case err != nil:
		s.logFailure(r, err)
		s.writeJSON(w, r, http.StatusServiceUnavailable, checkoutUnavailable)
		return
	case len(body) > maxPageBytes:
		s.Log.WarnContext(r.Context(), "response too large", "route", s.route(r), "method", r.Method, "requestID", s.requestID(r),
			"bytes", len(body))
		s.writeJSON(w, r, http.StatusServiceUnavailable, checkoutUnavailable)
		return
	}
	header := w.Header()
	header.Set("Content-Type", "application/json; charset=utf-8")
	header.Set("Cache-Control", privateCache)
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
