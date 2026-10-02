// Decide which groups are canonical: those whose IDs are on Code Rules' canonical group list.

package domain

import (
	"fmt"
	"maps"
	"slices"

	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// CanonicalGroup is a group on Code Rules' canonical group list. Every library whose group has its ID shares the
// group, under the list's display name.
type CanonicalGroup struct {
	ID, Name string
	// Description is the list's one line saying which rules belong in the group, which pages show rather than what
	// any one library says about it.
	Description string
	// Icon is zero when Rulemart has no icon for the group.
	Icon GroupIcon
}

// GroupIcon is the icon pages show beside a canonical group.
type GroupIcon struct {
	// File is the icon's path under the site's icons, such as devicon/go-original.svg.
	File string
	// Monochrome marks an icon drawn in black or one dark color, which dark themes invert so it stays visible.
	Monochrome bool
	// Narrow marks an icon whose drawing is much narrower than its square, such as Go's gopher, which pages draw
	// larger so it looks as big as square logos.
	Narrow bool
}

// CanonicalGroups is Code Rules' canonical group list, with Rulemart's icons for its groups. Its zero value is an
// empty list, on which no group is canonical.
type CanonicalGroups struct {
	byID map[string]CanonicalGroup
}

// NewCanonicalGroups returns the list holding groups, which coderules.ParseCanonicalGroups has validated, with
// icons, keyed by group ID. It rejects an icon for a group that isn't on the list.
func NewCanonicalGroups(groups []coderules.CanonicalGroup, icons map[string]GroupIcon) (CanonicalGroups, error) {
	byID := make(map[string]CanonicalGroup, len(groups))
	for _, g := range groups {
		byID[g.ID] = CanonicalGroup{ID: g.ID, Name: g.Name, Description: g.Description, Icon: icons[g.ID]}
	}
	for _, id := range slices.Sorted(maps.Keys(icons)) {
		if _, ok := byID[id]; !ok {
			return CanonicalGroups{}, fmt.Errorf("group icons: %s has an icon but isn't on the canonical group list", id)
		}
	}
	return CanonicalGroups{byID: byID}, nil
}

// Find returns the canonical group whose ID is exactly id, and whether there's one. The list has no aliases, and IDs
// match byte for byte, so a group such as techs/golang is never techs/go: it stands alone.
func (c CanonicalGroups) Find(id string) (CanonicalGroup, bool) {
	g, ok := c.byID[id]
	return g, ok
}

// All returns every group on the list, in ID order.
func (c CanonicalGroups) All() []CanonicalGroup {
	groups := make([]CanonicalGroup, 0, len(c.byID))
	for _, id := range slices.Sorted(maps.Keys(c.byID)) {
		groups = append(groups, c.byID[id])
	}
	return groups
}
