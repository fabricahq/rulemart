// Read a library's releases and what each changed, and compare two of its releases, or two versions of a rule.

package app

import (
	"context"
	"fmt"
	"slices"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// MaxComparedBytes is the most rule text one page compares: the two versions of each rule whose changes it shows,
// together. It keeps what a page reads and renders well within what one response can hold, however large a library's
// rules are, while leaving room for dozens of ordinary rules.
const MaxComparedBytes = 512 << 10

// ReleasesPage returns the vetted library owner/name, matched without regard to case, with its releases, newest
// first, each with what it changed since the release before it, or ErrNotFound.
func (p Pages) ReleasesPage(ctx context.Context, owner, name string) (views.ReleasesPage, error) {
	history, err := p.Store.LibraryHistory(ctx, p.Vetted, owner, name)
	if err != nil {
		return views.ReleasesPage{}, err
	}
	page := views.ReleasesPage{Library: history.Library}
	for _, release := range slices.Backward(history.Releases) {
		page.Releases = append(page.Releases, views.ReleaseNotes{
			Release:  release,
			Changes:  changes(history, release.Number-1, release.Number, nil),
			Versions: versionsAt(history, release.Number),
		})
	}
	return page, nil
}

// ReleaseComparison returns what changed in the vetted library owner/name between releases from and to, the older
// first whichever way round they're given, with the text of each changed rule within MaxComparedBytes. It fails with
// ErrNotFound when there's no such library or release.
func (p Pages) ReleaseComparison(ctx context.Context, owner, name string, from, to int) (views.ReleaseComparison, error) {
	from, to = min(from, to), max(from, to)
	history, texts, err := p.Store.ReleaseComparison(ctx, p.Vetted, owner, name, from, to, MaxComparedBytes)
	if err != nil {
		return views.ReleaseComparison{}, err
	}
	if from < 1 || to > len(history.Releases) {
		return views.ReleaseComparison{}, fmt.Errorf("compare releases of library %s/%s: release/%d...release/%d: %w", owner, name, from, to, ErrNotFound)
	}
	return views.ReleaseComparison{
		Library: history.Library, Releases: history.Releases, From: from, To: to,
		Changes: changes(history, from, to, texts),
	}, nil
}

// RuleComparison returns the rule at rulePath in the vetted library owner/name with the text of its versions from and
// to, the older first whichever way round they're given, within MaxComparedBytes. It fails with ErrNotFound when
// there's no such library or rule, or when either isn't a version of the rule.
func (p Pages) RuleComparison(ctx context.Context, owner, name, rulePath string, from, to coderules.RuleVersion) (views.RuleComparison, error) {
	if from.Compare(to) > 0 {
		from, to = to, from
	}
	comparison, err := p.Store.RuleComparison(ctx, p.Vetted, owner, name, rulePath, from, to, MaxComparedBytes)
	if err != nil {
		return views.RuleComparison{}, err
	}
	comparison.Page.Rule.CanonicalGroup = p.canonical(comparison.Page.Rule.Group)
	return comparison, nil
}

// changes returns how each rule of history changed between releases from and to, in path order, with the text of
// each changed rule that texts holds. Release 0 is the library before its first release, which holds no rules.
func changes(history views.LibraryHistory, from, to int, texts map[string]views.ComparedText) []views.RuleChange {
	var result []views.RuleChange
	for _, r := range history.Rules {
		old, inFrom := r.VersionAt(from)
		new, inTo := r.VersionAt(to)
		change := views.RuleChange{Rule: views.RuleRef{Path: r.Path, Title: r.Title, RetiredIn: r.RetiredIn}, Text: texts[r.Path]}
		switch {
		case inFrom && inTo && old == new, !inFrom && !inTo:
			continue
		case !inTo:
			change.Change, change.From = coderules.ChangeRetired, r.Versions[old].Version
			change.RetirementSummaries, change.ReplacedBy = r.RetirementSummaries, ruleRef(history, r.ReplacedBy)
			result = append(result, change)
			continue
		case !inFrom:
			change.Change, old = coderules.ChangeNew, -1
		}
		change.To = r.Versions[new].Version
		if old >= 0 {
			change.From = r.Versions[old].Version
		}
		for i := new; i > old; i-- {
			change.Versions = append(change.Versions, r.Versions[i])
			if change.Change != coderules.ChangeNew {
				change.Change = coderules.LargerChange(change.Change, r.Versions[i].Change)
			}
		}
		result = append(result, change)
	}
	return result
}

// versionsAt returns the version of every rule of history after release n, in path order.
func versionsAt(history views.LibraryHistory, n int) []views.RuleVersionRef {
	var result []views.RuleVersionRef
	for _, r := range history.Rules {
		if i, ok := r.VersionAt(n); ok {
			result = append(result, views.RuleVersionRef{Path: r.Path, Version: r.Versions[i].Version})
		}
	}
	return result
}

// ruleRef returns the rule of history at path, or nil when path is empty or names no rule of it.
func ruleRef(history views.LibraryHistory, path string) *views.RuleRef {
	i := slices.IndexFunc(history.Rules, func(r views.RuleHistory) bool { return r.Path == path })
	if path == "" || i < 0 {
		return nil
	}
	r := history.Rules[i]
	return &views.RuleRef{Path: r.Path, Title: r.Title, RetiredIn: r.RetiredIn}
}
