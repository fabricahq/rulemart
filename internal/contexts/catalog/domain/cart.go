// The cart: what a signed-in account collects from libraries to adopt, as a whole library, a group, or a rule.

package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// MaxCartItems is how many items one cart holds. It bounds each account's rows and the prompt checkout writes, and
// leaves room for every group of several libraries; a visitor who wants more rules of a group adds the group.
const MaxCartItems = 100

// MaxCartPathLength bounds a group's or rule's ID in a cart, as the cart_items table does. Code Rules' IDs are far
// shorter.
const MaxCartPathLength = 1000

// CartItemKind is what a cart item holds of its library.
type CartItemKind string

const (
	// CartLibrary holds every group of a library, as Code Rules' groups: "*" imports them, including groups later
	// releases add.
	CartLibrary CartItemKind = "library"
	// CartGroup holds every rule of one group, including rules later releases add to it.
	CartGroup CartItemKind = "group"
	// CartRule holds one rule.
	CartRule CartItemKind = "rule"
)

// CartItem names what a cart holds: a library, by its owner and name, and of it the whole library, a group, or a
// rule, by its ID.
type CartItem struct {
	Owner, Name string
	Kind        CartItemKind
	// Path is the group's or rule's ID, such as techs/go or techs/go/return-errors, and empty for a whole library.
	Path string
}

// FullName returns the item's library as owner/name.
func (i CartItem) FullName() string { return i.Owner + "/" + i.Name }

// Group returns the ID of the group a rule belongs to, the first two parts of its ID, or a group's own ID; it's empty
// for a whole library.
func (i CartItem) Group() string {
	parts := strings.SplitN(i.Path, "/", 3)
	if i.Kind == CartLibrary || len(parts) < 2 {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

// ErrNotCartItem reports form input that names no item a cart can hold.
var ErrNotCartItem = errors.New("not a cart item")

// idPart matches one part of a group's or rule's ID as pages and forms spell it: Code Rules' IDs are lowercase, and
// Rulemart matches them without regard to case.
var idPart = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// gitHubNamePart matches a GitHub owner's or repository's name, as a cart names its library: every library is on
// GitHub.
var gitHubNamePart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// ParseCartItem returns the item a form names: its library as owner/name, and at most one of a group's ID and a
// rule's, or the whole library when it names neither. It fails with ErrNotCartItem when they name no item, such as
// text no library or ID can hold, without checking that the item exists.
func ParseCartItem(library, group, rule string) (CartItem, error) {
	owner, name, ok := strings.Cut(library, "/")
	if !ok || !gitHubNamePart.MatchString(owner) || !gitHubNamePart.MatchString(name) || (group != "" && rule != "") {
		return CartItem{}, fmt.Errorf("parse cart item library=%q group=%q rule=%q: %w", library, group, rule, ErrNotCartItem)
	}
	item := CartItem{Owner: owner, Name: name, Kind: CartLibrary}
	switch {
	case group != "":
		item.Kind, item.Path = CartGroup, group
	case rule != "":
		item.Kind, item.Path = CartRule, rule
	}
	if !validCartPath(item.Kind, item.Path) {
		return CartItem{}, fmt.Errorf("parse cart item library=%q group=%q rule=%q: %w", library, group, rule, ErrNotCartItem)
	}
	return item, nil
}

// validCartPath reports whether path can be the ID of an item of kind: a group's two parts, or a rule's three or
// more, as Code Rules lays them out.
func validCartPath(kind CartItemKind, path string) bool {
	if kind == CartLibrary {
		return path == ""
	}
	if len(path) > MaxCartPathLength {
		return false
	}
	parts := strings.Split(path, "/")
	if (kind == CartGroup && len(parts) != 2) || (kind == CartRule && len(parts) < 3) {
		return false
	}
	for _, part := range parts {
		if !idPart.MatchString(part) || part == "." || part == ".." {
			return false
		}
	}
	return true
}

// Covering returns the item of cart, one library's items, that imports item already, and true, or false when none
// does: the whole library covers each of its groups and rules, and a group covers each of its rules.
func Covering(item CartItem, cart []CartItem) (CartItem, bool) {
	if item.Kind == CartLibrary {
		return CartItem{}, false
	}
	for _, other := range cart {
		if other.Kind == CartLibrary {
			return other, true
		}
	}
	if item.Kind != CartRule {
		return CartItem{}, false
	}
	for _, other := range cart {
		if other.Kind == CartGroup && strings.EqualFold(other.Path, item.Group()) {
			return other, true
		}
	}
	return CartItem{}, false
}
