// The cart: what a visitor collects from libraries to adopt, as a whole group or a single rule, which their browser
// keeps by key, and checkout resolves against the catalog.

package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// MaxCartItems is how many items one cart holds. It bounds what checkout resolves and the texts it writes, and leaves
// room for every group of several libraries; a visitor who wants more rules of a group adds the group.
const MaxCartItems = 100

// MaxCartKeyLength bounds a cart key. Code Rules' IDs and GitHub's names are far shorter.
const MaxCartKeyLength = 400

// CartItemKind is what a cart item holds of its library.
type CartItemKind string

const (
	// CartGroup holds every rule of one group, including rules later releases add to it.
	CartGroup CartItemKind = "group"
	// CartRule holds one rule.
	CartRule CartItemKind = "rule"
)

// CartItem names what a cart holds: a library, by its owner and name, and of it a group or a rule, by its ID.
type CartItem struct {
	Owner, Name string
	Kind        CartItemKind
	// Path is the group's or rule's ID, such as techs/go or techs/go/return-errors.
	Path string
}

// FullName returns the item's library as owner/name.
func (i CartItem) FullName() string { return i.Owner + "/" + i.Name }

// Group returns the ID of the group a rule belongs to, the first two parts of its ID, or a group's own ID.
func (i CartItem) Group() string {
	parts := strings.SplitN(i.Path, "/", 3)
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

// groupKeyPrefix starts the key of a whole group, which a rule's key, starting with its library's owner, never does:
// no GitHub owner has a colon in its name.
const groupKeyPrefix = "group::"

// Key returns the item as the browser's cart keeps it, as the prototype does: owner/name::techs/go/return-errors for
// a rule, and group::owner/name::techs/go for a whole group.
func (i CartItem) Key() string {
	if i.Kind == CartGroup {
		return groupKeyPrefix + i.FullName() + "::" + i.Path
	}
	return i.FullName() + "::" + i.Path
}

// ErrNotCartItem reports a key that names no item a cart can hold.
var ErrNotCartItem = errors.New("not a cart item")

// idPart matches one part of a group's or rule's ID as pages spell it: Code Rules' IDs are lowercase, and Rulemart
// matches them without regard to case.
var idPart = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ParseCartKey returns the item key names, as Key writes it. It fails with ErrNotCartItem when key names no item,
// such as text no library or ID can hold, or a key longer than MaxCartKeyLength, without checking that the item
// exists.
func ParseCartKey(key string) (CartItem, error) {
	item := CartItem{Kind: CartRule}
	rest := key
	if strings.HasPrefix(key, groupKeyPrefix) {
		item.Kind, rest = CartGroup, strings.TrimPrefix(key, groupKeyPrefix)
	}
	library, path, ok := strings.Cut(rest, "::")
	owner, name, named := strings.Cut(library, "/")
	if len(key) > MaxCartKeyLength || !ok || !named || !gitHubName.MatchString(owner) || !gitHubName.MatchString(name) ||
		!validCartPath(item.Kind, path) {
		return CartItem{}, fmt.Errorf("parse cart key %q: %w", key, ErrNotCartItem)
	}
	item.Owner, item.Name, item.Path = owner, name, path
	return item, nil
}

// validCartPath reports whether path can be the ID of an item of kind: a group's two parts, or a rule's three or
// more, as Code Rules lays them out.
func validCartPath(kind CartItemKind, path string) bool {
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
