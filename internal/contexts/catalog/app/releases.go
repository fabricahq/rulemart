// Read a library's releases and what each changed, and compare two of its releases, or two versions of a rule.

package app

import (
	"context"
	"fmt"

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

// ReleasesPage returns the vetted library owner/name, matched without regard to case, with the page of its releases
// that holds release, or the first page when release is 0. Pages list releases newest first, each with what it
// changed since the release before it: whole releases while their rows fit MaxReleaseRows, each counting releaseRows
// more, and at least one, the first page starting at the latest release and each later one where the one before it
// ends. It fails with ErrNotFound when there's no such library or release.
func (p Pages) ReleasesPage(ctx context.Context, owner, name string, release int) (views.ReleasesPage, error) {
	history, err := p.Store.LibraryHistory(ctx, p.Vetted, owner, name)
	if err != nil {
		return views.ReleasesPage{}, err
	}
	latest := len(history.Releases)
	if release == 0 {
		release = latest
	}
	if release < 1 || release > latest {
		return views.ReleasesPage{}, fmt.Errorf("load releases of library %s/%s: release/%d: %w", owner, name, release, ErrNotFound)
	}
	page := views.ReleasesPage{Library: history.Library, AllReleases: history.Releases}
	for start := latest; ; {
		page.Releases, page.Older = releasesFrom(history, start)
		if page.Older < release {
			return page, nil
		}
		page.Newer, start = start, page.Older
	}
}

// releasesFrom returns the page of history's releases that starts with release start, and the release the next page
// starts with, or 0 when the page ends with release 1.
func releasesFrom(history views.LibraryHistory, start int) (releases []views.ReleaseNotes, older int) {
	rows := 0
	for n := start; n >= 1; n-- {
		notes := views.ReleaseNotes{
			Release:  history.Releases[n-1],
			Changes:  changes(history, n-1, n, nil),
			Versions: versionsAt(history, n),
		}
		rows += releaseRows + len(notes.Changes) + len(notes.Versions)
		if len(releases) > 0 && rows > MaxReleaseRows {
			return releases, n
		}
		releases = append(releases, notes)
	}
	return releases, 0
}

// ReleaseComparison returns what changed in the vetted library owner/name between releases from and to, the older
// first whichever way round they're given, with the text of each changed or renamed rule within MaxComparedBytes. It
// fails with ErrNotFound when there's no such library or release.
func (p Pages) ReleaseComparison(ctx context.Context, owner, name string, from, to int) (views.ReleaseComparison, error) {
	from, to = min(from, to), max(from, to)
	pick := func(history views.LibraryHistory) []views.VersionPair { return comparedPairs(history, from, to) }
	history, texts, err := p.Store.ReleaseComparison(ctx, p.Vetted, owner, name, pick, MaxComparedBytes)
	if err != nil {
		return views.ReleaseComparison{}, err
	}
	if from < 1 || to > len(history.Releases) {
		return views.ReleaseComparison{}, fmt.Errorf("compare releases of library %s/%s: release/%d...release/%d: %w", owner, name, from, to, ErrNotFound)
	}
	comparison := views.ReleaseComparison{
		Library: history.Library, Releases: history.Releases, From: from, To: to,
		Changes: changes(history, from, to, texts),
	}
	for _, r := range history.Releases[from:to] {
		comparison.SharedFiles = comparison.SharedFiles || r.UpdatesSharedFiles
	}
	return comparison, nil
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
// title it had after release to, or a retired rule by its last, with the text of each changed or renamed rule that
// texts holds. A rule renamed between them shows once, as a ChangeRenamed of its new ID. Release 0 is the library
// before its first release, which holds no rules.
func changes(history views.LibraryHistory, from, to int, texts map[string]views.ComparedText) []views.RuleChange {
	links := historyLinks(history)
	index := ruleIndex(history)
	renames := renamesBetween(history, links, from, to)
	renamedOld := map[int]bool{}
	for _, old := range renames {
		renamedOld[old] = true
	}
	var result []views.RuleChange
	for i, r := range history.Rules {
		if renamedOld[i] {
			continue
		}
		old, inFrom := r.VersionAt(from)
		new, inTo := r.VersionAt(to)
		change := views.RuleChange{Rule: views.RuleRef{Path: r.Path, RetiredIn: r.RetiredIn}, Text: texts[r.Path]}
		switch {
		case inFrom && inTo && old == new, !inFrom && !inTo:
			continue
		case !inTo:
			change.Rule.Title = titleAt(r, old)
			change.Change, change.From = coderules.ChangeRetired, r.Versions[old].Version
			change.RetirementSummaries, change.Replacements = r.RetirementSummaries, titledAt(history, index, links.replacements(r.Path), to)
			result = append(result, change)
			continue
		case !inFrom:
			change.Change, old = coderules.ChangeNew, -1
			if o, ok := renames[i]; ok {
				before := history.Rules[o]
				last := len(before.Versions) - 1
				change.Change, change.From = views.ChangeRenamed, before.Versions[last].Version
				change.RenamedFrom = &views.RuleRef{Path: before.Path, Title: titleAt(before, last), RetiredIn: before.RetiredIn}
			}
		}
		change.Rule.Title, change.To = titleAt(r, new), r.Versions[new].Version
		if old >= 0 {
			change.From = r.Versions[old].Version
		}
		for i := new; i > old; i-- {
			change.Versions = append(change.Versions, r.Versions[i])
			if change.Change != coderules.ChangeNew && change.Change != views.ChangeRenamed {
				change.Change = coderules.LargerChange(change.Change, r.Versions[i].Change)
			}
		}
		result = append(result, change)
	}
	return result
}

// renamesBetween returns the rules of history renamed between releases from and to: by the index of the new rule, the
// index of the old one, which a release after from retired and replaced by the new one, which that release added
// under the old one's title, and which is still current after to.
func renamesBetween(history views.LibraryHistory, links ruleLinks, from, to int) map[int]int {
	index := ruleIndex(history)
	renames := map[int]int{}
	for i, r := range history.Rules {
		_, inFrom := r.VersionAt(from)
		_, inTo := r.VersionAt(to)
		if !inFrom || inTo || !links.renamed(r.Path) {
			continue
		}
		n, ok := index[r.ReplacedBy]
		if !ok {
			continue
		}
		if _, inTo := history.Rules[n].VersionAt(to); inTo {
			renames[n] = i
		}
	}
	return renames
}

// comparedPairs returns the pairs of versions whose text a comparison of releases from and to shows, in path order:
// each changed rule's versions after each release, keyed by its path, and each renamed rule's last version before the
// rename with the new rule's after to, keyed by the new rule's path.
func comparedPairs(history views.LibraryHistory, from, to int) []views.VersionPair {
	renames := renamesBetween(history, historyLinks(history), from, to)
	var pairs []views.VersionPair
	for i, r := range history.Rules {
		old, inFrom := r.VersionAt(from)
		new, inTo := r.VersionAt(to)
		switch o, renamed := renames[i]; {
		case renamed:
			pairs = append(pairs, views.VersionPair{Key: r.Path, OldRule: o, OldVersion: len(history.Rules[o].Versions) - 1, NewRule: i, NewVersion: new})
		case inFrom && inTo && old != new:
			pairs = append(pairs, views.VersionPair{Key: r.Path, OldRule: i, OldVersion: old, NewRule: i, NewVersion: new})
		}
	}
	return pairs
}

// titledAt names each rule of refs by the title it had after release n, which a page about that release shows, rather
// than its newest, and keeps the newest for a rule that release didn't have.
// index is ruleIndex's of history.
func titledAt(history views.LibraryHistory, index map[string]int, refs []views.RuleRef, n int) []views.RuleRef {
	for i, ref := range refs {
		r, ok := index[ref.Path]
		if !ok {
			continue
		}
		if v, ok := history.Rules[r].VersionAt(n); ok {
			refs[i].Title = titleAt(history.Rules[r], v)
		}
	}
	return refs
}

// ruleIndex returns the index of each rule of history by its path.
func ruleIndex(history views.LibraryHistory) map[string]int {
	index := make(map[string]int, len(history.Rules))
	for i, r := range history.Rules {
		index[r.Path] = i
	}
	return index
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
