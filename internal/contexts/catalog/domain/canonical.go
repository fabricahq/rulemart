// Decide which groups are canonical: those whose IDs are on Code Rules' canonical group list.

package domain

import "github.com/fabricahq/rulemart/internal/lib/coderules"

// CanonicalGroup is a group on Code Rules' canonical group list. Every library whose group has its ID shares the
// group, under the list's display name.
type CanonicalGroup struct {
	ID, Name string
}

// CanonicalGroups is Code Rules' canonical group list. Its zero value is an empty list, on which no group is
// canonical.
type CanonicalGroups struct {
	byID map[string]CanonicalGroup
}

// NewCanonicalGroups returns the list holding groups, which coderules.ParseCanonicalGroups has validated.
func NewCanonicalGroups(groups []coderules.CanonicalGroup) CanonicalGroups {
	byID := make(map[string]CanonicalGroup, len(groups))
	for _, g := range groups {
		byID[g.ID] = CanonicalGroup{ID: g.ID, Name: g.Name}
	}
	return CanonicalGroups{byID: byID}
}

// Find returns the canonical group whose ID is exactly id, and whether there's one. The list has no aliases, and IDs
// match byte for byte, so a group such as techs/golang is never techs/go: it stands alone.
func (c CanonicalGroups) Find(id string) (CanonicalGroup, bool) {
	g, ok := c.byID[id]
	return g, ok
}
