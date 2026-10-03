// Check out the cart a visitor's browser keeps: cart-page.js posts its keys and choices, and the answer resolves
// each against the catalog and holds the prompt and commands that import it.

package web

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"

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
)

// maxCheckoutBytes bounds a checkout's request: room for domain.MaxCartItems keys of domain.MaxCartKeyLength
// characters, twice, as the cart and its forks, and the rest of the choices.
const maxCheckoutBytes = 4 * domain.MaxCartItems * domain.MaxCartKeyLength

// checkoutRequest is what cart-page.js posts: the cart as the browser keeps it, in localStorage's rulemart-cart.
type checkoutRequest struct {
	// Cart is the cart's keys, in order, as domain.CartItem's Key writes them.
	Cart []string `json:"cart"`
	// Fork marks the keys of the rules the visitor forks. Full marks the libraries, as owner/name, whose rules' groups
	// the visitor adds whole, and Confirmed the libraries the visitor confirmed adding from though Rulemart doesn't vet
	// them.
	Fork      map[string]bool `json:"fork"`
	Full      map[string]bool `json:"full"`
	Confirmed map[string]bool `json:"confirmed"`
	// Repo is what the visitor wrote as their project's repository, which may name none.
	Repo string `json:"repo"`
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
	// Prompt and Commands import every ready item, and are empty when none is.
	Prompt   string `json:"prompt"`
	Commands string `json:"commands"`
	// Pin is the release the Commands tab's footnote suggests pinning a library to, or nil.
	Pin *pinJSON `json:"pin"`
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
	// Upsell offers to add the rest of the groups of the library's rules that stay in sync, or is nil.
	Upsell *upsellJSON `json:"upsell"`
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
	// Version is a rule's newest version, Fork whether the visitor forks it, and RetiredIn the tag of the release
	// that retired it, if one did.
	Version   string `json:"version,omitempty"`
	Fork      bool   `json:"fork,omitempty"`
	RetiredIn string `json:"retiredIn,omitempty"`
	// Rules are the current rules a whole group brings.
	Rules []ruleLinkJSON `json:"rules,omitempty"`
}

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

// upsellJSON offers to add the other rules of Groups, by name, Extra of them, or says the visitor did when Full.
type upsellJSON struct {
	Groups []string `json:"groups"`
	Extra  int      `json:"extra"`
	Full   bool     `json:"full"`
}

// cartPage shows the cart's page, which cart-page.js fills from the cart the browser keeps. It's the same for every
// visitor but in where the cart's rules go, which offers to sign in to anyone who isn't.
func (s *server) cartPage(w http.ResponseWriter, r *http.Request) {
	v := visitorOf(r.Context())
	s.render(w, r, http.StatusOK, cartPage(s.chrome, v.signIn, v.withGitHub, v.account != nil))
}

// checkout answers a cart that cart-page.js posts, as checkoutRequest describes it, with its checkout. Another site
// can't post one (withSameOriginWrites), and the answer is the visitor's alone, so nothing caches it. A request that
// isn't such a cart, or holds more than domain.MaxCartItems keys, is refused with 400.
func (s *server) checkout(w http.ResponseWriter, r *http.Request) {
	if media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || media != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "send the cart as application/json"})
		return
	}
	var req checkoutRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCheckoutBytes))
	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "the cart isn't one Rulemart can read"})
		return
	}
	target, repositoryInvalid := checkoutTarget(req.Repo)
	checkout, err := s.Carts.Checkout(r.Context(), app.Cart{Keys: req.Cart, Forks: req.Fork, Full: req.Full, Confirmed: req.Confirmed}, target)
	switch {
	case errors.Is(err, app.ErrCartTooLarge):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "the cart holds more items than it may"})
		return
	case err != nil:
		s.Log.ErrorContext(r.Context(), "request failed", "route", s.route(r), "method", r.Method, "requestID", s.requestID(r),
			"status", http.StatusServiceUnavailable, "error", err.Error())
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Rulemart can't check out right now"})
		return
	}
	writeJSON(w, http.StatusOK, newCheckoutResponse(checkout, target, repositoryInvalid, s.assets.iconURL))
}

// checkoutTarget returns the project a checkout is for, from repo, what the visitor wrote as its repository, and
// whether repo names none though it isn't empty. Rulemart doesn't know the visitor's projects yet, so it can't tell
// whether one uses Code Rules.
func checkoutTarget(repo string) (domain.CheckoutTarget, bool) {
	target := domain.CheckoutTarget{Mode: domain.ProjectUnknown}
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
		Libraries: []checkoutLibraryJSON{}, Unknown: checkout.Unknown, Repository: target.Repository,
		RepositoryInvalid: repositoryInvalid, Prompt: checkout.Prompt, Commands: checkout.Commands,
	}
	if resp.Unknown == nil {
		resp.Unknown = []string{}
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
		if lib.Extra > 0 || (lib.Full && len(lib.UpsellGroups) > 0) {
			upsell := &upsellJSON{Extra: lib.Extra, Full: lib.Full}
			for _, g := range lib.UpsellGroups {
				upsell.Groups = append(upsell.Groups, newGroupLabel(g.Path, g.Canonical).display())
			}
			l.Upsell = upsell
		}
		resp.Libraries = append(resp.Libraries, l)
	}
	return resp
}

// newCheckoutItemJSON describes it, an item of the library whose page is libraryHref, which is gone or not.
func newCheckoutItemJSON(it views.CheckoutItem, libraryHref string, gone bool, iconURL func(string) string) checkoutItemJSON {
	label := newGroupLabel(it.Group.Path, it.Group.Canonical)
	item := checkoutItemJSON{
		Key: it.Key, Kind: string(it.Item.Kind), State: string(it.State), ID: it.Item.Path,
		Group: groupJSON{ID: label.id, Name: label.display(), Canonical: label.canonical}, Fork: it.Fork,
	}
	if icon := newGroupIcon(it.Group.Canonical, iconURL); icon.src != "" {
		item.Group.Icon = &iconJSON{Src: icon.src, Monochrome: icon.monochrome, Narrow: icon.narrow, LightTile: icon.lightTile}
	}
	switch it.Item.Kind {
	case domain.CartGroup:
		item.Title = label.display()
		if it.State != views.CartItemMissing && !gone {
			item.Href = libraryHref + "/" + it.Item.Path
		}
		for _, r := range it.Rules {
			item.Rules = append(item.Rules, ruleLinkJSON{Title: titleOrID(r.Title, r.Path), Href: libraryHref + "/" + r.Path})
		}
	case domain.CartRule:
		item.Title = titleOrID(it.Title, it.Item.Path)
		if it.State != views.CartItemMissing && !gone {
			item.Href = libraryHref + "/" + it.Item.Path
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

// writeJSON answers with status and value as JSON, which no cache keeps.
func writeJSON(w http.ResponseWriter, status int, value any) {
	header := w.Header()
	header.Set("Content-Type", "application/json; charset=utf-8")
	header.Set("Cache-Control", privateCache)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
