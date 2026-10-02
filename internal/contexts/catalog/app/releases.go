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

// MaxReleaseRows bounds what one page of a library's releases lists: each release's changes and the version of every
// rule after it. A library's releases list every rule's version again and again, so their notes grow with rules times
// releases; a page of at most this many rows stays well within what one response can hold.
const MaxReleaseRows = 2000

// releaseRows is what a release's card counts toward MaxReleaseRows besides the rules it lists: its header and notes,
// so a page of releases that list few rules or none is bounded too.
const releaseRows = 10

// ReleasesPage returns the vetted library owner/name, matched without regard to case, with its releases from release
// until back, newest first, each with what it changed since the release before it: whole releases while their rows
// fit MaxReleaseRows, each counting releaseRows more, and at least one. until 0 starts at the latest release. It fails with ErrNotFound when there's no
// such library or release.
func (p Pages) ReleasesPage(ctx context.Context, owner, name string, until int) (views.ReleasesPage, error) {
	history, err := p.Store.LibraryHistory(ctx, p.Vetted, owner, name)
	if err != nil {
		return views.ReleasesPage{}, err
	}
	if until == 0 {
		until = len(history.Releases)
	}
	if until < 1 || until > len(history.Releases) {
		return views.ReleasesPage{}, fmt.Errorf("load releases of library %s/%s: release/%d: %w", owner, name, until, ErrNotFound)
	}
	page := views.ReleasesPage{Library: history.Library, AllReleases: history.Releases}
	rows := 0
	for n := until; n >= 1; n-- {
		notes := views.ReleaseNotes{
			Release:  history.Releases[n-1],
			Changes:  changes(history, n-1, n, nil),
			Versions: versionsAt(history, n),
		}
		rows += releaseRows + len(notes.Changes) + len(notes.Versions)
		if len(page.Releases) > 0 && rows > MaxReleaseRows {
			page.Older = n
			break
		}
		page.Releases = append(page.Releases, notes)
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

// changes returns how each rule of history changed between releases from and to, in path order, each named by the
// title it had after release to, or a retired rule by its last, with the text of each changed rule that texts holds. Release 0 is the library before its first release, which holds no rules.
func changes(history views.LibraryHistory, from, to int, texts map[string]views.ComparedText) []views.RuleChange {
	var result []views.RuleChange
	for _, r := range history.Rules {
		old, inFrom := r.VersionAt(from)
		new, inTo := r.VersionAt(to)
		change := views.RuleChange{Rule: views.RuleRef{Path: r.Path, RetiredIn: r.RetiredIn}, Text: texts[r.Path]}
		switch {
		case inFrom && inTo && old == new, !inFrom && !inTo:
			continue
		case !inTo:
			change.Rule.Title = titleAt(r, old)
			change.Change, change.From = coderules.ChangeRetired, r.Versions[old].Version
			change.RetirementSummaries, change.ReplacedBy = r.RetirementSummaries, ruleRef(history, r.ReplacedBy)
			result = append(result, change)
			continue
		case !inFrom:
			change.Change, old = coderules.ChangeNew, -1
		}
		change.Rule.Title, change.To = titleAt(r, new), r.Versions[new].Version
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

// titleAt returns the title rule r had at its version i, or its newest title when the catalog doesn't have that
// version's.
func titleAt(r views.RuleHistory, i int) string {
	if title := r.Versions[i].Title; title != "" {
		return title
	}
	return r.Title
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
