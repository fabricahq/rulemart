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
	// versionsDocs explains rule versions and their change levels, which a Major mark links to.
	versionsDocs = "https://code-rules.fabricahq.com/reference/rule-versions/#choose-a-version-change"
	// majorExplanation says what a major change means, which a Major mark says to assistive technology too.
	majorExplanation = "Major change: work that complied with the previous version could fail this one."
)

// changeKinds are the kinds of change, in the order Code Rules' release notes list them, with each section's title
// and the word its count uses. Code Rules records a rename as a new rule and a retired one; Rulemart lists it once,
// after the new rules.
var changeKinds = []struct {
	change      coderules.Change
	title, word string
}{
	{coderules.ChangeNew, "New rules", "new"},
	{views.ChangeRenamed, "Renamed rules", "renamed"},
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

// releasesHref is the path of a library's Library releases tab, which starts at its latest release.
func releasesHref(lib libraryView) string { return lib.href + "?tab=releases" }

// releasesPageHref is the path of the page of a library's releases that holds release n, or the first page when n is
// 0 or the latest release. The tab redirects a release on its first page to the first page's own address.
func releasesPageHref(lib libraryView, n int) string {
	if n == 0 || n == lib.releases {
		return releasesHref(lib)
	}
	return releasesHref(lib) + "&release=" + strconv.Itoa(n)
}

// releaseHref is the path of release n's card on a library's Library releases tab, on the page that holds it.
func releaseHref(lib libraryView, n int) string {
	return releasesPageHref(lib, n) + "#" + releaseAnchor(n)
}

// releaseTagClass styles a link to a release by its tag the same wherever a page shows one.
const releaseTagClass = "font-mono text-[13px] text-muted decoration-border-strong underline-offset-3 hover:text-ink hover:decoration-ink"

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

// maxVersionRows bounds the table of every rule's version that a release's card holds. A larger release's table, as
// long as its library, leads to its GitHub Release page instead, which lists them all.
const maxVersionRows = 1000

// releaseCard is one release on a library's Library releases tab, laid out like its generated release notes.
type releaseCard struct {
	anchor, tag, date, notesURL string
	// compareHref compares the release with the one before it, previous; both are empty for the first release.
	compareHref, previous string
	latest, major         bool
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
	// items are shown, and more, when a long list folds them, follow behind a disclosure that counts them.
	items, more []changeItem
	// noun names the items more counts, such as "new rules".
	noun string
}

// Long lists of changes, such as a first release's every rule, fold: past foldChangesAfter items, a section shows
// shownBeforeFold and folds the rest.
const (
	foldChangesAfter = 20
	shownBeforeFold  = 10
)

// changeItem is one rule in a changeSection.
type changeItem struct {
	href, title, id string
	// versionsLabel is text before versions, such as "last version" for a retired rule.
	versionsLabel string
	// versions is "1.0.0 → 1.1.0", or one version for a new or retired rule, which compareHref links when Rulemart can
	// compare them.
	versions, compareHref string
	// notes are the summaries of each version published between the releases, newest first, labeled with their
	// version when there's more than one. A retired rule has its retirement's summaries as one note.
	notes []versionNote
	// replacedBy is the rule that replaced a retired one, then the one that replaced that one, and so on.
	replacedBy []ruleLink
	// renamedFrom is the ID a renamed rule had before, or nil, and textHref leads to the old rule's last text compared
	// with the new rule's.
	renamedFrom *ruleLink
	textHref    string
}

// versionNote is one version's change summaries; version is empty when the list needs no label, and major marks a
// major change.
type versionNote struct {
	version   string
	major     bool
	summaries []string
}

// ruleLink names another rule of the library, linking its page.
type ruleLink struct {
	href, title, id string
}

func newRuleLink(lib libraryView, r views.RuleRef) ruleLink {
	return ruleLink{href: ruleHref(lib, r.Path), title: titleOrID(r.Title, r.Path), id: r.Path}
}

func newRuleLinks(lib libraryView, refs []views.RuleRef) []ruleLink {
	links := make([]ruleLink, len(refs))
	for i, r := range refs {
		links[i] = newRuleLink(lib, r)
	}
	return links
}

// chainStep is one rule in a chain of replacements, with the words before it and after it, which pages show on one
// line so no space comes before the period.
type chainStep struct {
	prefix, suffix string
	link           ruleLink
	// id is the rule's ID, shown beside its title for a rename, whose title is the same, and empty otherwise.
	id string
}

// chainSteps words a chain of replacements: "Replaced by A, itself replaced by B.", or "Renamed to A." for a rename. A
// chain of more than three names its first two rules and its last, and counts those between.
func chainSteps(links []ruleLink, renamed bool) []chainStep {
	steps := make([]chainStep, len(links))
	for i, link := range links {
		steps[i] = chainStep{prefix: ", itself replaced by ", link: link}
		if i == 0 {
			steps[i].prefix = "Replaced by "
			if renamed {
				steps[i].prefix, steps[i].id = "Renamed to ", link.id
			}
		}
	}
	// A long chain names its first two rules and its last, and counts the ones between.
	if n := len(steps); n > 3 {
		last := steps[n-1]
		last.prefix = ", and after " + strconv.Itoa(n-3) + " more, by "
		steps = append(steps[:2:2], last)
	}
	if n := len(steps); n > 0 {
		steps[n-1].suffix = "."
	}
	return steps
}

// rowChainSteps words a chain of replacements by the rules' IDs, within a line that names a retired rule: "replaced
// by a, itself replaced by b", or "renamed to a", with no period, collapsed as chainSteps collapses a long chain.
func rowChainSteps(links []ruleLink, renamed bool) []chainStep {
	steps := chainSteps(links, renamed)
	for i := range steps {
		steps[i].id, steps[i].suffix = steps[i].link.id, ""
	}
	if len(steps) > 0 {
		steps[0].prefix = strings.ToLower(steps[0].prefix)
	}
	return steps
}

// titleOrID returns a rule's title, or its ID when the catalog doesn't have its title yet.
func titleOrID(title, id string) string {
	if title == "" {
		return id
	}
	return title
}

// releasesView is one page of a library's releases.
type releasesView struct {
	cards []releaseCard
	// newerHref leads to the page of the newest releases, and olderHref to the next page; each is empty when there's
	// none.
	newerHref, olderHref string
	// from and to choose two releases to compare: the latest and the one before it.
	from, to []releaseOption
}

// newReleasesView describes page, a page of lib's releases.
func newReleasesView(lib libraryView, page views.ReleasesPage) releasesView {
	v := releasesView{cards: newReleaseCards(lib, page)}
	v.from, v.to = releasePicker(page.AllReleases)
	if len(page.Releases) > 0 && page.Releases[0].Release.Number != lib.releases {
		v.newerHref = releasesPageHref(lib, page.Newer)
	}
	if page.Older != 0 {
		v.olderHref = releasesPageHref(lib, page.Older)
	}
	return v
}

// newReleaseCards describes each release of page, in its order.
func newReleaseCards(lib libraryView, page views.ReleasesPage) []releaseCard {
	cards := make([]releaseCard, len(page.Releases))
	for i, notes := range page.Releases {
		n := notes.Release.Number
		card := releaseCard{
			anchor: releaseAnchor(n), tag: domain.ReleaseTag(n), date: date(notes.Release.TaggedAt),
			notesURL: domain.ReleaseNotesURL(lib.fullName(), n), latest: n == page.Library.LatestRelease,
			sections: newChangeSections(lib, notes.Changes, n == 1, n),
		}
		if n > 1 {
			card.compareHref, card.previous = releaseComparisonHref(lib, n-1, n, diffWords), domain.ReleaseTag(n-1)
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

// newChangeSections sorts changes into sections by kind, in the order Code Rules' release notes use, folding a long
// one. A first release's rules are all new, and its notes don't repeat each one's "Add the rule.", so firstRelease
// leaves out their notes. release is the release a card shows, whose comparison with the one before shows a rename's
// text, or 0 on a comparison's page, which shows it itself.
func newChangeSections(lib libraryView, changes []views.RuleChange, firstRelease bool, release int) []changeSection {
	var sections []changeSection
	for _, kind := range changeKinds {
		section := changeSection{title: kind.title, warning: kind.change == coderules.ChangeMajor, noun: strings.ToLower(kind.title)}
		for _, c := range changes {
			if c.Change == kind.change {
				section.items = append(section.items, newChangeItem(lib, c, firstRelease, release))
			}
		}
		if len(section.items) > foldChangesAfter {
			section.items, section.more = section.items[:shownBeforeFold], section.items[shownBeforeFold:]
		}
		if len(section.items) > 0 {
			sections = append(sections, section)
		}
	}
	return sections
}

// newChangeItem describes how one rule changed. Its title leads to the rule's Versions tab, or to a retired rule's
// page, which lists its versions. A rename's versions lead to the diff of its old rule's last text with its new
// rule's: on release's comparison with the release before, or on this page when release is 0.
func newChangeItem(lib libraryView, c views.RuleChange, firstRelease bool, release int) changeItem {
	item := changeItem{href: ruleHref(lib, c.Rule.Path), title: titleOrID(c.Rule.Title, c.Rule.Path), id: c.Rule.Path}
	if c.Rule.RetiredIn == 0 {
		item.href += "?tab=versions"
	}
	switch c.Change {
	case coderules.ChangeRetired:
		item.versionsLabel, item.versions = "last version", c.From.String()
		item.notes = []versionNote{{summaries: shortened(c.RetirementSummaries)}}
		item.replacedBy = newRuleLinks(lib, c.Replacements)
		return item
	case coderules.ChangeNew:
		item.versions = c.To.String()
	case views.ChangeRenamed:
		from := newRuleLink(lib, *c.RenamedFrom)
		// The old rule's version and the new rule's belong to different rules, so the item names only the new one's.
		item.renamedFrom, item.versions = &from, c.To.String()
		item.textHref = "#" + diffAnchor(c.Rule.Path)
		if release != 0 {
			item.textHref = releaseComparisonHref(lib, release-1, release, diffWords) + item.textHref
		}
	default:
		item.versions = c.From.String() + " → " + c.To.String()
		item.compareHref = ruleComparisonHref(lib, c.Rule.Path, c.From, c.To, diffWords)
	}
	if firstRelease {
		return item
	}
	for _, v := range c.Versions {
		note := versionNote{summaries: shortened(v.Summaries)}
		if len(c.Versions) > 1 {
			note.version, note.major = v.Version.String(), v.Change == coderules.ChangeMajor
		}
		item.notes = append(item.notes, note)
	}
	return item
}

// diffAnchor is the fragment of a rule's diff on a comparison of releases. Its slashes become underscores, which a
// Code Rules ID never holds, so IDs that differ in where a slash or hyphen is, such as techs/go-a/b and techs/go/a-b,
// stay apart.
func diffAnchor(rulePath string) string { return "diff-" + strings.ReplaceAll(rulePath, "/", "_") }

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

// lineClass is the class of a deleted or inserted line's row, which the stylesheet colors; empty for any other.
func lineClass(op textdiff.Op) string {
	switch op {
	case textdiff.Delete:
		return "d"
	case textdiff.Insert:
		return "i"
	}
	return ""
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

// versionPicker returns the choices of a rule's versions, newest first, to compare, choosing the newest and the one
// before it.
func versionPicker(versions []views.Version) (from, to []releaseOption) {
	for i, v := range versions {
		value := v.Version.String()
		from = append(from, releaseOption{value: value, label: value, selected: i == 1})
		to = append(to, releaseOption{value: value, label: value, selected: i == 0})
	}
	return from, to
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

// diffPanelMarks is what a diff's panel counts toward maxDiffMarks besides its diff: its header and links, so a page
// shows a bounded number of panels however many rules changed.
const diffPanelMarks = 25

// maxSummaryRunes bounds a change summary as pages show it. A summary is one line a library writes, of any length, so
// a very long one is cut short.
const maxSummaryRunes = 1000

// shortened returns summaries, each cut to maxSummaryRunes, with an ellipsis where it's cut.
func shortened(summaries []string) []string {
	result := make([]string, len(summaries))
	for i, summary := range summaries {
		if runes := []rune(summary); len(runes) > maxSummaryRunes {
			summary = strings.TrimRight(string(runes[:maxSummaryRunes]), " ") + "…"
		}
		result[i] = summary
	}
	return result
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
	// omitted counts the changed rules whose diffs didn't fit the page.
	omitted int
	// sharedFiles says that a release between them changed library-wide files.
	sharedFiles string
	// backHref leads to the Library releases tab, and wordsHref and linesHref show this comparison each way.
	backHref, wordsHref, linesHref string
}

// newReleaseComparisonView describes comparison, showing changed text as mode says.
func newReleaseComparisonView(lib libraryView, comparison views.ReleaseComparison, mode diffMode) releaseComparisonView {
	from, to := comparison.From, comparison.To
	v := releaseComparisonView{
		from: strconv.Itoa(from), to: strconv.Itoa(to), mode: string(mode),
		fromTag: domain.ReleaseTag(from), toTag: domain.ReleaseTag(to), same: from == to,
		sections: newChangeSections(lib, comparison.Changes, false, 0), major: hasMajor(comparison.Changes),
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
	if comparison.SharedFiles {
		v.sharedFiles = "Library releases after " + v.fromTag + ", up to " + v.toTag +
			", update shared files, such as group descriptions or shared assets."
	}
	budget := newDiffBudget()
	for _, c := range comparison.Changes {
		switch c.Change {
		case coderules.ChangeMajor, coderules.ChangeMinor, coderules.ChangePatch, views.ChangeRenamed:
			if !budget.spend(diffPanelMarks) {
				v.omitted++
				continue
			}
			oldPath := c.Rule.Path
			if c.RenamedFrom != nil {
				oldPath = c.RenamedFrom.Path
			}
			d := newDiffView(lib, oldPath, c.Rule.Path, c.From, c.To, c.Text, mode, budget)
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
	v.diff = newDiffView(lib, r.id, r.id, from, to, comparison.Text, mode, newDiffBudget())
	return v
}

// maxDiffMarks bounds how many rows, blocks, and marks one page's diffs render. Short text can make a long diff, such
// as every line of a file changed, so the text a page compares doesn't bound what it renders; this keeps a page's
// diffs to a few hundred kilobytes of markup, well within what one response can hold, while showing ordinary changes
// to dozens of rules.
const maxDiffMarks = 10_000

// diffBudget is what's left of maxDiffMarks for the diffs a page has yet to show.
type diffBudget struct {
	left int
}

func newDiffBudget() *diffBudget { return &diffBudget{left: maxDiffMarks} }

// spend takes n marks from the budget, or reports false, taking none, when fewer are left.
func (b *diffBudget) spend(n int) bool {
	if n > b.left {
		return false
	}
	b.left -= n
	return true
}

// lineMarks counts what a line diff renders: each hunk's header, each line with its segments, and the unchanged lines
// it hides, which a reader can show.
func lineMarks(d textdiff.LineDiff) int {
	n := len(d.After)
	for _, h := range d.Hunks {
		n += 1 + len(h.Before)
		for _, line := range h.Lines {
			n += 1 + len(line.Segments)
		}
	}
	return n
}

// wordMarks counts what a word diff renders: each part, and each block with its segments.
func wordMarks(parts []textdiff.Part) int {
	n := 0
	for _, p := range parts {
		n++
		for _, b := range p.Blocks {
			n += 1 + len(b.Segments)
		}
	}
	return n
}

// diffView is one rule file's changes between two versions.
type diffView struct {
	// path is the rule's file, href its page, and title its title, which only a comparison of releases shows. A
	// renamed rule's old file is oldPath, which is path otherwise.
	path, oldPath, href, title string
	// anchor is the diff's fragment on its page.
	anchor   string
	from, to string
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

// versions names the versions compared, "1.0.0 → 1.1.0", or one version when both are the same, such as a rule
// renamed without a new version number, whose header shows the move between files instead.
func (d diffView) versions() string {
	if d.from == d.to {
		return d.to
	}
	return d.from + " → " + d.to
}

// renamed reports that the diff compares two rules' files: a rule's last text before a rename with its new ID's.
func (d diffView) renamed() bool { return d.oldPath != d.path }

// newDiffView compares version from of rule oldPath, which is rulePath unless the rule was renamed, with version to of
// rule rulePath in lib, whose text is text, as mode says, spending budget on what it renders. A diff that needs more
// than is left shows as too large.
func newDiffView(lib libraryView, oldPath, rulePath string, from, to coderules.RuleVersion, text views.ComparedText, mode diffMode, budget *diffBudget) diffView {
	file, oldFile := domain.RuleFile(rulePath), domain.RuleFile(oldPath)
	d := diffView{
		path: file, oldPath: oldFile, href: ruleHref(lib, rulePath), anchor: diffAnchor(rulePath),
		from: from.String(), to: to.String(), state: text.State, mode: mode,
	}
	d.fromURL, d.toURL = lib.fileAtVersionURL(oldFile, text.OldRelease), lib.fileAtVersionURL(file, text.NewRelease)
	if text.State != views.TextShown {
		return d
	}
	d.lines = textdiff.Lines(text.Old, text.New)
	d.added, d.removed, d.unchanged = d.lines.Added, d.lines.Removed, len(d.lines.Hunks) == 0
	marks := lineMarks(d.lines)
	if mode == diffWords {
		d.words = textdiff.Words(text.Old, text.New)
		marks = wordMarks(d.words)
	}
	if !budget.spend(marks) {
		d.state, d.lines, d.words = views.TextTooLarge, textdiff.LineDiff{}, nil
	}
	return d
}
