// Check carts out: resolve what a browser's cart names against the catalog, and write the texts that import it into
// a project.

package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// ErrCartTooLarge reports a cart of more than domain.MaxCartItems keys, which no page writes.
var ErrCartTooLarge = errors.New("the cart holds more items than it may")

// Carts checks carts out, as the web function does, writing nothing: a cart lives in the visitor's browser, which
// sends it. Only items of a library Vetted holds, or of one a listing names that the visitor confirmed, are imported.
type Carts struct {
	Store  store.Carts
	Vetted []domain.LibraryKey
	// Groups is Code Rules' canonical group list, which names groups in the cart and the texts.
	Groups domain.CanonicalGroups
}

// Cart is a browser's cart: its keys, in the order it holds them, and what the visitor chose for its items.
type Cart struct {
	Keys []string
	// Forks are the keys of the rules the visitor forks rather than keep in sync.
	Forks map[string]bool
	// Full and Confirmed name libraries, as owner/name, matched without regard to case: Full those whose rules'
	// groups the visitor imports whole, and Confirmed those the visitor confirmed adding from though Rulemart doesn't
	// vet them.
	Full, Confirmed map[string]bool
}

// Checkout resolves cart against the catalog, and returns each item's state with the texts that import every ready
// item into target. It fails with ErrCartTooLarge when cart holds more than domain.MaxCartItems keys. A key the cart
// repeats counts once, and one that names no item is listed as unknown.
func (c Carts) Checkout(ctx context.Context, cart Cart, target domain.CheckoutTarget) (views.Checkout, error) {
	if len(cart.Keys) > domain.MaxCartItems {
		return views.Checkout{}, fmt.Errorf("check out cart keys=%d: %w", len(cart.Keys), ErrCartTooLarge)
	}
	items, keys, unknown := parseCart(cart.Keys)
	libraries, err := c.Store.CartLibraries(ctx, c.Vetted, items)
	if err != nil {
		return views.Checkout{}, err
	}
	checkout := views.Checkout{Unknown: unknown}
	var imports []domain.CheckoutLibrary
	for _, named := range groupByLibrary(items, keys) {
		lib := findCartLibrary(libraries, named.items[0].FullName())
		resolved := c.resolve(named.items, named.keys, lib, cart)
		checkout.Libraries = append(checkout.Libraries, resolved)
		imports = append(imports, importsOf(resolved, lib))
	}
	texts := domain.NewCheckout(target, imports)
	checkout.Commands, checkout.Prompt = texts.Commands(), texts.Prompt()
	if pin, ok := texts.PinExample(); ok {
		checkout.PinExample = &pin
	}
	return checkout, nil
}

// parseCart returns the items keys name, each once, in order, with the key that named each, and the keys that name
// none, each once.
func parseCart(keys []string) (items []domain.CartItem, named, unknown []string) {
	seen := map[string]bool{}
	for _, key := range keys {
		if seen[key] {
			continue
		}
		seen[key] = true
		item, err := domain.ParseCartKey(key)
		if err != nil {
			unknown = append(unknown, key)
			continue
		}
		items, named = append(items, item), append(named, key)
	}
	return items, named, unknown
}

// cartLibraryItems are a cart's items from one library, with their keys, in the cart's order.
type cartLibraryItems struct {
	items []domain.CartItem
	keys  []string
}

// groupByLibrary returns items, each named by the key at the same index, by library, matched without regard to case,
// in the order the cart first names each.
func groupByLibrary(items []domain.CartItem, keys []string) []cartLibraryItems {
	var libraries []cartLibraryItems
	at := map[string]int{}
	for i, item := range items {
		name := strings.ToLower(item.FullName())
		j, ok := at[name]
		if !ok {
			j, at[name] = len(libraries), len(libraries)
			libraries = append(libraries, cartLibraryItems{})
		}
		libraries[j].items = append(libraries[j].items, item)
		libraries[j].keys = append(libraries[j].keys, keys[i])
	}
	return libraries
}

// findCartLibrary returns the library of libraries named fullName, without regard to case, or nil.
func findCartLibrary(libraries []views.CartLibrary, fullName string) *views.CartLibrary {
	i := slices.IndexFunc(libraries, func(l views.CartLibrary) bool { return strings.EqualFold(l.Library.FullName(), fullName) })
	if i < 0 {
		return nil
	}
	return &libraries[i]
}

// hasName reports whether names holds fullName, as Cart's maps of libraries do, without regard to case.
func hasName(names map[string]bool, fullName string) bool {
	for name, on := range names {
		if on && strings.EqualFold(name, fullName) {
			return true
		}
	}
	return false
}

// resolve returns one library's items, each named by the key at the same index, as the catalog has them in lib, or
// as gone when lib is nil, with the visitor's choices in cart, and the offer to import the rest of their rules' groups.
func (c Carts) resolve(items []domain.CartItem, keys []string, lib *views.CartLibrary, cart Cart) views.CheckoutLibrary {
	resolved := views.CheckoutLibrary{Library: views.LibraryRef{Owner: items[0].Owner, Name: items[0].Name}, Gone: lib == nil}
	if lib != nil {
		resolved.Library, resolved.Vetted, resolved.LatestRelease = lib.Library, lib.Vetted, lib.LatestRelease
		resolved.Confirmed = !lib.Vetted && hasName(cart.Confirmed, lib.Library.FullName())
		resolved.Full = hasName(cart.Full, lib.Library.FullName())
	}
	for i, item := range items {
		resolved.Items = append(resolved.Items, c.resolveItem(item, keys[i], lib, resolved.Confirmed, cart.Forks[keys[i]]))
	}
	resolved.UpsellGroups, resolved.Extra = upsell(resolved, lib)
	return resolved
}

// resolveItem returns item, named by key, of the library lib, resolved: gone, unvetted unless confirmed, missing,
// retired, or ready, in that order of precedence. fork is the visitor's choice for a rule.
func (c Carts) resolveItem(item domain.CartItem, key string, lib *views.CartLibrary, confirmed, fork bool) views.CheckoutItem {
	it := views.CheckoutItem{Key: key, Item: item, State: views.CartItemReady, Group: c.group(item.Group())}
	if lib == nil {
		it.State = views.CartItemGone
		return it
	}
	switch item.Kind {
	case domain.CartGroup:
		for _, r := range lib.Rules {
			if strings.EqualFold(r.Group, item.Path) && r.RetiredIn == 0 {
				it.Item.Path, it.Group = r.Group, c.group(r.Group)
				it.Rules = append(it.Rules, r)
			}
		}
		if len(it.Rules) == 0 {
			it.State = views.CartItemMissing
		}
	case domain.CartRule:
		r, ok := findCartRule(lib.Rules, item.Path)
		if !ok {
			it.State = views.CartItemMissing
			break
		}
		it.Item.Path, it.Group, it.Title, it.Version, it.RetiredIn, it.Fork = r.Path, c.group(r.Group), r.Title, r.Version, r.RetiredIn, fork
		if r.RetiredIn > 0 {
			it.State = views.CartItemRetired
		}
	}
	if !lib.Vetted && !confirmed {
		it.State = views.CartItemUnvetted
	}
	return it
}

// findCartRule returns the rule of rules at path, matched without regard to case, preferring the rule spelled exactly
// so, as pages find a rule.
func findCartRule(rules []views.CartRule, path string) (views.CartRule, bool) {
	var found *views.CartRule
	for i, r := range rules {
		if r.Path == path {
			return r, true
		}
		if found == nil && strings.EqualFold(r.Path, path) {
			found = &rules[i]
		}
	}
	if found == nil {
		return views.CartRule{}, false
	}
	return *found, true
}

// group returns the group path as checkout names it, with its place on the canonical group list, if any.
func (c Carts) group(path string) views.CheckoutGroup {
	return views.CheckoutGroup{Path: path, Canonical: canonicalGroup(c.Groups, path)}
}

// upsell returns the groups of the library's ready rules that stay in sync, other than groups the cart holds whole,
// in the cart's order, and how many of their current rules in lib the cart doesn't hold, or 0 when the visitor imports
// them already.
func upsell(resolved views.CheckoutLibrary, lib *views.CartLibrary) ([]views.CheckoutGroup, int) {
	whole, held := map[string]bool{}, map[string]bool{}
	for _, it := range resolved.Items {
		if it.State != views.CartItemReady {
			continue
		}
		if it.Item.Kind == domain.CartGroup {
			whole[strings.ToLower(it.Item.Path)] = true
		} else {
			held[strings.ToLower(it.Item.Path)] = true
		}
	}
	var groups []views.CheckoutGroup
	for _, it := range resolved.Items {
		path := strings.ToLower(it.Group.Path)
		if it.State == views.CartItemReady && it.Item.Kind == domain.CartRule && !it.Fork && !whole[path] &&
			!slices.ContainsFunc(groups, func(g views.CheckoutGroup) bool { return strings.EqualFold(g.Path, path) }) {
			groups = append(groups, it.Group)
		}
	}
	if resolved.Full || lib == nil {
		return groups, 0
	}
	extra := 0
	for _, r := range lib.Rules {
		inGroup := slices.ContainsFunc(groups, func(g views.CheckoutGroup) bool { return strings.EqualFold(g.Path, r.Group) })
		if inGroup && r.RetiredIn == 0 && !held[strings.ToLower(r.Path)] {
			extra++
		}
	}
	return groups, extra
}

// importsOf returns what checkout imports of resolved, the library lib as the cart holds it: its ready items, with the
// visitor's choices.
func importsOf(resolved views.CheckoutLibrary, lib *views.CartLibrary) domain.CheckoutLibrary {
	imports := domain.CheckoutLibrary{
		Owner: resolved.Library.Owner, Name: resolved.Library.Name, Vetted: resolved.Vetted, Release: resolved.LatestRelease,
		Full: resolved.Full,
	}
	if lib != nil {
		imports.Commit = lib.LatestCommit
	}
	for _, it := range resolved.Items {
		if it.State != views.CartItemReady {
			continue
		}
		group := domain.CheckoutGroup{ID: it.Group.Path, Name: it.Group.Path}
		if it.Group.Canonical != nil {
			group.Name = it.Group.Canonical.Name
		}
		switch {
		case it.Item.Kind == domain.CartGroup:
			imports.Groups = append(imports.Groups, group)
		case it.Fork:
			imports.Forks = append(imports.Forks, domain.CheckoutRule{ID: it.Item.Path, Group: group, Version: it.Version.String()})
		default:
			imports.Rules = append(imports.Rules, domain.CheckoutRule{ID: it.Item.Path, Group: group, Version: it.Version.String()})
		}
	}
	return imports
}
