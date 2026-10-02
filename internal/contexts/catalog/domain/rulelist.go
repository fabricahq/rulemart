// Say which rules a list of rules across libraries holds, a group's page or a search, and in what order.

package domain

import "strings"

// FabricaOwner owns Fabrica's libraries, which lists put first among libraries otherwise equal, while stars are few.
const FabricaOwner = "fabricahq"

// ImpactBand narrows a list of rules by the impact their libraries declare.
type ImpactBand string

const (
	// AnyImpact narrows nothing.
	AnyImpact ImpactBand = ""
	// HighImpact keeps rules of CRITICAL or HIGH impact.
	HighImpact ImpactBand = "high"
	// LowerImpact keeps every other rule: MEDIUM-HIGH and below, and a rule without a declared impact.
	LowerImpact ImpactBand = "medium"
)

// RuleOrder orders a list of rules.
type RuleOrder string

const (
	// BestMatch orders a search's rules by how well they match it, as search ranks them.
	BestMatch RuleOrder = "best"
	// MostStarred orders rules by their stars, most first.
	MostStarred RuleOrder = "stars"
	// Newest orders rules by when the library release that first published each one was tagged, newest first.
	Newest RuleOrder = "new"
)

// RuleFilters narrow a list of rules. The zero value narrows nothing.
type RuleFilters struct {
	// Libraries are the libraries to keep, each as LibraryKeyOf spells it, or none to keep every library.
	Libraries []string
	Impact    ImpactBand
	// MinStars keeps rules with at least this many stars; 0 keeps every rule.
	MinStars int
	// Kind keeps the rules of one kind of group, techs or practices, or of both when empty.
	Kind string
}

// IsZero reports whether the filters keep every rule.
func (f RuleFilters) IsZero() bool {
	return len(f.Libraries) == 0 && f.Impact == AnyImpact && f.MinStars == 0 && f.Kind == ""
}

// RuleList says which rules a list across libraries holds, before and after its filters, and in what order: a group's
// page, every rule, or a search's matches.
type RuleList struct {
	// Query is what a search finds; the zero query lists every rule.
	Query SearchQuery
	// Group is the ID of the one group whose rules the list holds, matched exactly, or empty for every group.
	Group string
	// Unvetted adds the libraries listings name that the release doesn't vet to the vetted ones.
	Unvetted bool
	// Retired adds retired rules to a list without a query; a search for words always finds them.
	Retired bool
	Filters RuleFilters
	// Order is BestMatch, MostStarred, or Newest. Every order puts a retired rule after the current rules it ties with.
	Order RuleOrder
}

// HoldsRetired reports whether the list holds retired rules: when it asks for them, or searches for words.
func (l RuleList) HoldsRetired() bool { return l.Retired || !l.Query.IsZero() }

// LibraryKeyOf returns owner/name as RuleFilters.Libraries holds it: in lowercase, since libraries are matched without
// regard to case.
func LibraryKeyOf(owner, name string) string { return strings.ToLower(owner + "/" + name) }
