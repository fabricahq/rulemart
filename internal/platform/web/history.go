// Shape a library's releases, and comparisons of two releases or two rule versions, into what their pages show.

package web

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
	"github.com/fabricahq/rulemart/internal/lib/textdiff"
)

// Text Code Rules' generated release notes use, which a library's releases show the same way.
const (
	majorWarning      = "Code that complied with the previous rule version could fail the new one, so review these before updating."
	sharedFilesNote   = "This library release also updates shared files, such as group descriptions or shared assets."
	onlySharedFiles   = "It updates shared files, such as group descriptions or shared assets."
	releasesIntroText = "A library release publishes new versions of one or more rules at once. Each rule keeps its own version."
)

// changeKinds are the kinds of change, in the order Code Rules' release notes list them, with each section's title
// and the word its count uses.
var changeKinds = []struct {
	change      coderules.Change
	title, word string
}{
	{coderules.ChangeNew, "New rules", "new"},
	{coderules.ChangeMajor, "Major changes", "major"},
	{coderules.ChangeMinor, "Minor changes", "minor"},
	{coderules.ChangePatch, "Patch changes", "patch"},
	{coderules.ChangeRetired, "Retired rules", "retired"},
}

// diffMode is how a comparison shows changed text: words marked in Markdown blocks, or a unified diff of lines.
type diffMode string

const (
	diffWords diffMode = "words"
	diffLines diffMode = "lines"
)

// parseDiffMode reads a comparison's view parameter: lines, or words for anything else.
func parseDiffMode(text string) diffMode {
	if text == string(diffLines) {
		return diffLines
	}
	return diffWords
}

// releasesHref is the path of a library's Library releases tab, and releaseHref of one release on it.
func releasesHref(lib libraryView) string { return lib.href + "?tab=releases" }

func releaseHref(lib libraryView, n int) string { return releasesHref(lib) + "#" + releaseAnchor(n) }

// releaseAnchor is the fragment of a release's card on the Library releases tab.
func releaseAnchor(n int) string { return "release-" + strconv.Itoa(n) }

// releaseComparisonHref is the path of the comparison of a library's releases from and to.
func releaseComparisonHref(lib libraryView, from, to int, mode diffMode) string {
	q := url.Values{"tab": {"releases"}, "from": {strconv.Itoa(from)}, "to": {strconv.Itoa(to)}}
	if mode == diffLines {
		q.Set("view", string(mode))
	}
	return lib.href + "?" + q.Encode()
}

// ruleHref is the path of a rule's page in lib.
func ruleHref(lib libraryView, rulePath string) string { return lib.href + "/" + rulePath }

// ruleComparisonHref is the path of the comparison of a rule's versions from and to.
func ruleComparisonHref(lib libraryView, rulePath string, from, to coderules.RuleVersion, mode diffMode) string {
	q := url.Values{"tab": {"versions"}, "from": {from.String()}, "to": {to.String()}}
	if mode == diffLines {
		q.Set("view", string(mode))
	}
	return ruleHref(lib, rulePath) + "?" + q.Encode()
}

// releaseCard is one release on a library's Library releases tab, laid out like its generated release notes.
type releaseCard struct {
	anchor, tag, date, notesURL string
	latest, major               bool
	// summary is the opening sentence, such as "Library release 2 changes 3 rules: 1 major and 2 new."
	summary  string
	sections []changeSection
	// sharedFiles notes that a release that changed rules also updated shared files.
	sharedFiles bool
	versions    []versionRow
}

// versionRow is a rule's version after a release, in its list of every rule's version.
type versionRow struct {
	href, id, version string
}

// changeSection is the rules that changed one way, such as Major changes.
type changeSection struct {
	title string
	// warning says to review major changes before updating.
	warning bool
	items   []changeItem
}

// changeItem is one rule in a changeSection.
type changeItem struct {
	href, title, id string
	// versions is "1.0.0 → 1.1.0", or one version for a new or retired rule, which compareHref links when Rulemart can
	// compare them.
	versions, compareHref string
	// notes are the summaries of each version published between the releases, newest first, labeled with their
	// version when there's more than one. A retired rule has its retirement's summaries as one note.
	notes []versionNote
	// replacedBy is the rule that replaced a retired one, or nil.
	replacedBy *ruleLink
}

// versionNote is one version's change summaries; version is empty when the list needs no label.
type versionNote struct {
	version   string
	summaries []string
}

// ruleLink names another rule of the library, linking its page.
type ruleLink struct {
	href, title, id string
}

func newRuleLink(lib libraryView, r views.RuleRef) ruleLink {
	return ruleLink{href: ruleHref(lib, r.Path), title: titleOrID(r.Title, r.Path), id: r.Path}
}

// titleOrID returns a rule's title, or its ID when the catalog doesn't have its title yet.
func titleOrID(title, id string) string {
	if title == "" {
		return id
	}
	return title
}

// newReleaseCards describes each release of page, in its order.
func newReleaseCards(lib libraryView, page views.ReleasesPage) []releaseCard {
	cards := make([]releaseCard, len(page.Releases))
	for i, notes := range page.Releases {
		n := notes.Release.Number
		card := releaseCard{
			anchor: releaseAnchor(n), tag: domain.ReleaseTag(n), date: date(notes.Release.TaggedAt),
			notesURL: domain.ReleaseNotesURL(lib.fullName(), n), latest: n == page.Library.LatestRelease,
			sections: newChangeSections(lib, notes.Changes, n == 1),
		}
		card.major = hasMajor(notes.Changes)
		switch {
		case n == 1:
			card.summary = "Library release 1 publishes " + plural(len(notes.Changes), "rule", "rules") + "."
		case len(notes.Changes) == 0:
			card.summary = "Library release " + strconv.Itoa(n) + " changes no rules."
			if notes.Release.UpdatesSharedFiles {
				card.summary += " " + onlySharedFiles
			}
		default:
			card.summary = "Library release " + strconv.Itoa(n) + " changes " + plural(len(notes.Changes), "rule", "rules") +
				": " + countPhrase(notes.Changes) + "."
			card.sharedFiles = notes.Release.UpdatesSharedFiles
		}
		for _, v := range notes.Versions {
			card.versions = append(card.versions, versionRow{href: ruleHref(lib, v.Path), id: v.Path, version: v.Version.String()})
		}
		cards[i] = card
	}
	return cards
}

// hasMajor reports whether any change is major.
func hasMajor(changes []views.RuleChange) bool {
	for _, c := range changes {
		if c.Change == coderules.ChangeMajor {
			return true
		}
	}
	return false
}

// countPhrase counts changes by kind, in the order Code Rules' release notes use, such as "1 major and 2 new".
func countPhrase(changes []views.RuleChange) string {
	var parts []string
	for _, kind := range changeKinds {
		n := 0
		for _, c := range changes {
			if c.Change == kind.change {
				n++
			}
		}
		if n > 0 {
			parts = append(parts, strconv.Itoa(n)+" "+kind.word)
		}
	}
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " and " + parts[1]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + ", and " + parts[len(parts)-1]
}

// newChangeSections sorts changes into sections by kind, in the order Code Rules' release notes use. A first release's
// rules are all new, and its notes don't repeat each one's "Add the rule.", so firstRelease leaves out their notes.
func newChangeSections(lib libraryView, changes []views.RuleChange, firstRelease bool) []changeSection {
	var sections []changeSection
	for _, kind := range changeKinds {
		section := changeSection{title: kind.title, warning: kind.change == coderules.ChangeMajor}
		for _, c := range changes {
			if c.Change == kind.change {
				section.items = append(section.items, newChangeItem(lib, c, firstRelease))
			}
		}
		if len(section.items) > 0 {
			sections = append(sections, section)
		}
	}
	return sections
}

// newChangeItem describes how one rule changed. Its title leads to the rule's Versions tab, or to a retired rule's
// page, which lists its versions.
func newChangeItem(lib libraryView, c views.RuleChange, firstRelease bool) changeItem {
	item := changeItem{href: ruleHref(lib, c.Rule.Path), title: titleOrID(c.Rule.Title, c.Rule.Path), id: c.Rule.Path}
	if c.Rule.RetiredIn == 0 {
		item.href += "?tab=versions"
	}
	switch c.Change {
	case coderules.ChangeRetired:
		item.versions = "last version " + c.From.String()
		item.notes = []versionNote{{summaries: c.RetirementSummaries}}
		if c.ReplacedBy != nil {
			link := newRuleLink(lib, *c.ReplacedBy)
			item.replacedBy = &link
		}
		return item
	case coderules.ChangeNew:
		item.versions = c.To.String()
	default:
		item.versions = c.From.String() + " → " + c.To.String()
		item.compareHref = ruleComparisonHref(lib, c.Rule.Path, c.From, c.To, diffWords)
	}
	if firstRelease {
		return item
	}
	for _, v := range c.Versions {
		note := versionNote{summaries: v.Summaries}
		if len(c.Versions) > 1 {
			note.version = v.Version.String()
		}
		item.notes = append(item.notes, note)
	}
	return item
}

// hunkHeader writes a hunk's header as git does, such as "@@ -6,14 +6,16 @@".
func hunkHeader(h textdiff.Hunk) string {
	return "@@ -" + strconv.Itoa(h.OldStart) + "," + strconv.Itoa(h.OldLines) + " +" + strconv.Itoa(h.NewStart) + "," +
		strconv.Itoa(h.NewLines) + " @@"
}

// lineNumber writes a line's number on one side of a diff, or nothing for a line that side doesn't have.
func lineNumber(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// lineSign marks a deleted or inserted line, as a unified diff does.
func lineSign(op textdiff.Op) string {
	switch op {
	case textdiff.Delete:
		return "−"
	case textdiff.Insert:
		return "+"
	}
	return ""
}

// releasePicker returns the choices of releases to compare, newest first, choosing the latest and the one before it.
func releasePicker(releases []views.Release) (from, to []releaseOption) {
	latest := len(releases)
	return releaseOptions(releases, latest-1), releaseOptions(releases, latest)
}

// releaseOptions returns every release as a choice, newest first, with release n chosen.
func releaseOptions(releases []views.Release, n int) []releaseOption {
	options := make([]releaseOption, 0, len(releases))
	for i := len(releases) - 1; i >= 0; i-- {
		r := releases[i]
		options = append(options, releaseOption{
			value: strconv.Itoa(r.Number), label: domain.ReleaseTag(r.Number), selected: r.Number == n,
		})
	}
	return options
}

// releaseOption is a release in a comparison's choice of releases.
type releaseOption struct {
	value, label string
	selected     bool
}

// releaseComparisonView is what changed in a library between two releases, and the changed rules' text.
type releaseComparisonView struct {
	from, to, mode string
	fromTag, toTag string
	// fromOptions and toOptions are every release, newest first, with the compared one selected.
	fromOptions, toOptions []releaseOption
	// same is a comparison of a release with itself.
	same    bool
	summary string
	major   bool
	// sections list the changes, and diffs the changed rules' text, in path order.
	sections []changeSection
	diffs    []diffView
	// backHref leads to the Library releases tab, and wordsHref and linesHref show this comparison each way.
	backHref, wordsHref, linesHref string
}

// newReleaseComparisonView describes comparison, showing changed text as mode says.
func newReleaseComparisonView(lib libraryView, comparison views.ReleaseComparison, mode diffMode) releaseComparisonView {
	from, to := comparison.From, comparison.To
	v := releaseComparisonView{
		from: strconv.Itoa(from), to: strconv.Itoa(to), mode: string(mode),
		fromTag: domain.ReleaseTag(from), toTag: domain.ReleaseTag(to), same: from == to,
		sections: newChangeSections(lib, comparison.Changes, false), major: hasMajor(comparison.Changes),
		backHref: releasesHref(lib), wordsHref: releaseComparisonHref(lib, from, to, diffWords),
		linesHref: releaseComparisonHref(lib, from, to, diffLines),
	}
	v.fromOptions, v.toOptions = releaseOptions(comparison.Releases, from), releaseOptions(comparison.Releases, to)
	if len(comparison.Changes) == 0 {
		v.summary = "No rules changed between " + v.fromTag + " and " + v.toTag + "."
	} else {
		v.summary = plural(len(comparison.Changes), "rule", "rules") + " changed between " + v.fromTag + " and " + v.toTag +
			": " + countPhrase(comparison.Changes) + "."
	}
	for _, c := range comparison.Changes {
		switch c.Change {
		case coderules.ChangeMajor, coderules.ChangeMinor, coderules.ChangePatch:
			d := newDiffView(lib, c.Rule.Path, c.From, c.To, c.Text, mode)
			d.title = titleOrID(c.Rule.Title, c.Rule.Path)
			v.diffs = append(v.diffs, d)
		}
	}
	return v
}

// ruleComparisonView is what changed between two versions of a rule, and its text.
type ruleComparisonView struct {
	from, to, mode string
	// fromOptions and toOptions are every version, newest first, with the compared one selected.
	fromOptions, toOptions []releaseOption
	same                   bool
	major                  bool
	// notes are the versions after from, up to to, newest first.
	notes []versionView
	// span says which releases the versions are from, such as "Between release/1 and release/4."
	span string
	diff diffView
	// backHref leads to the Versions tab, and wordsHref and linesHref show this comparison each way.
	backHref, wordsHref, linesHref string
}

// newRuleComparisonView describes comparison, a comparison of rule r's versions, showing changed text as mode says.
func newRuleComparisonView(r ruleView, comparison views.RuleComparison, mode diffMode) ruleComparisonView {
	from, to := comparison.From, comparison.To
	lib := r.library
	v := ruleComparisonView{
		from: from.String(), to: to.String(), mode: string(mode), same: from == to,
		backHref: r.href + "?tab=versions", wordsHref: ruleComparisonHref(lib, r.id, from, to, diffWords),
		linesHref: ruleComparisonHref(lib, r.id, from, to, diffLines),
	}
	var fromRelease, toRelease int
	for i, version := range comparison.Page.Versions {
		value := version.Version.String()
		v.fromOptions = append(v.fromOptions, releaseOption{value: value, label: value, selected: version.Version == from})
		v.toOptions = append(v.toOptions, releaseOption{value: value, label: value, selected: version.Version == to})
		switch {
		case version.Version == from:
			fromRelease = version.Release
		case version.Version.Compare(from) > 0 && version.Version.Compare(to) <= 0:
			v.notes = append(v.notes, r.versions[i])
			v.major = v.major || version.Change == coderules.ChangeMajor
		}
		if version.Version == to {
			toRelease = version.Release
		}
	}
	v.span = "Between " + domain.ReleaseTag(fromRelease) + " and " + domain.ReleaseTag(toRelease) + "."
	v.diff = newDiffView(lib, r.id, from, to, comparison.Text, mode)
	return v
}

// diffView is one rule file's changes between two versions.
type diffView struct {
	// path is the rule's file, href its page, and title its title, which only a comparison of releases shows.
	path, href, title string
	from, to          string
	// fromURL and toURL are the file at each version's release on GitHub, where a reader can compare text the page
	// can't show.
	fromURL, toURL string
	state          views.TextState
	// unchanged marks versions whose files have the same lines.
	unchanged      bool
	added, removed int
	mode           diffMode
	words          []textdiff.Part
	lines          textdiff.LineDiff
}

// newDiffView compares rule rulePath's versions from and to in lib, whose text is text, as mode says.
func newDiffView(lib libraryView, rulePath string, from, to coderules.RuleVersion, text views.ComparedText, mode diffMode) diffView {
	file := domain.RuleFile(rulePath)
	d := diffView{
		path: file, href: ruleHref(lib, rulePath), from: from.String(), to: to.String(),
		state: text.State, mode: mode,
	}
	d.fromURL, d.toURL = lib.fileAtVersionURL(file, text.OldRelease), lib.fileAtVersionURL(file, text.NewRelease)
	if text.State != views.TextShown {
		return d
	}
	d.lines = textdiff.Lines(text.Old, text.New)
	d.added, d.removed, d.unchanged = d.lines.Added, d.lines.Removed, len(d.lines.Hunks) == 0
	if mode == diffWords {
		d.words = textdiff.Words(text.Old, text.New)
	}
	return d
}
