// Follow how a library's rules replaced each other: a retired rule's replacements, and renames.

package app

import (
	"maps"
	"slices"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// ruleLinks indexes a library's rule links by path.
type ruleLinks map[string]views.RuleLink

func newRuleLinks(links []views.RuleLink) ruleLinks {
	byPath := make(ruleLinks, len(links))
	for _, l := range links {
		byPath[l.Path] = l
	}
	return byPath
}

// historyLinks returns the links of every rule in history.
func historyLinks(history views.LibraryHistory) ruleLinks {
	byPath := make(ruleLinks, len(history.Rules))
	for _, r := range history.Rules {
		first, last := r.Versions[0], r.Versions[len(r.Versions)-1]
		byPath[r.Path] = views.RuleLink{
			Path: r.Path, Title: r.Title, RetiredIn: r.RetiredIn, ReplacedBy: r.ReplacedBy,
			FirstRelease: first.Release, FirstTitle: first.Title, LastTitle: last.Title,
		}
	}
	return byPath
}

// replacements returns the rule that replaced the rule at path, then, while that one was retired by release by, or
// at all when by is 0, the rule that replaced it, and so on. The chain ends at a rule still current then, one whose
// retirement named no replacement, or one it already named, so a cycle in a library's records can't loop.
func (l ruleLinks) replacements(path string, by int) []views.RuleRef {
	var chain []views.RuleRef
	seen := map[string]bool{path: true}
	for next := l[path].ReplacedBy; next != "" && !seen[next]; {
		seen[next] = true
		link, ok := l[next]
		if !ok {
			chain = append(chain, views.RuleRef{Path: next})
			break
		}
		chain = append(chain, views.RuleRef{Path: link.Path, Title: link.Title, RetiredIn: link.RetiredIn})
		if link.RetiredIn == 0 || (by != 0 && link.RetiredIn > by) {
			break
		}
		next = link.ReplacedBy
	}
	return chain
}

// renamed reports whether the rule at path was renamed: retired by the release that added its replacement, which
// had the title the retired rule last had. Code Rules records a rename as a retirement and a new rule.
func (l ruleLinks) renamed(path string) bool {
	old, ok := l[path]
	if !ok || old.RetiredIn == 0 || old.ReplacedBy == "" {
		return false
	}
	replacement, ok := l[old.ReplacedBy]
	return ok && replacement.FirstRelease == old.RetiredIn && old.LastTitle != "" && replacement.FirstTitle == old.LastTitle
}

// replaced returns the rules whose retirement named the rule at path as their replacement, in path order: the one it
// renamed, if any, and the others.
func (l ruleLinks) replaced(path string) (renamedFrom *views.RuleRef, replaces []views.RuleRef) {
	for _, p := range slices.Sorted(maps.Keys(l)) {
		link := l[p]
		if link.ReplacedBy != path {
			continue
		}
		ref := views.RuleRef{Path: link.Path, Title: link.Title, RetiredIn: link.RetiredIn}
		if l.renamed(link.Path) && renamedFrom == nil {
			renamedFrom = &ref
		} else {
			replaces = append(replaces, ref)
		}
	}
	return renamedFrom, replaces
}
