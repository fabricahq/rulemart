// Updates waiting for a project: how far the rules a project holds of a library are behind the library's own.

package domain

import "github.com/fabricahq/rulemart/internal/lib/coderules"

// RuleStates is a library's rules as they stand: each current rule's version, and its retired rules, by ID.
type RuleStates struct {
	Current map[string]coderules.RuleVersion
	Retired map[string]bool
}

// PinnedVersion is a library's rule at the version a project holds.
type PinnedVersion struct {
	Path    string
	Version coderules.RuleVersion
}

// Updates counts the rules of pinned that code-rules project update would change: each the library has since published
// a newer version of, and each it has retired. A rule the library doesn't hold, such as one it dropped from its
// history, counts as neither. Rules the library added to a group the project imports aren't counted: the project's
// configuration may exclude them, which its provenance file doesn't record.
func (s RuleStates) Updates(pinned []PinnedVersion) int {
	n := 0
	for _, p := range pinned {
		if current, ok := s.Current[p.Path]; ok && current.Compare(p.Version) > 0 || s.Retired[p.Path] {
			n++
		}
	}
	return n
}
