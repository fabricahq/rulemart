// Shape the catalog's page reads into what each page shows: text, counts, dates, and links.

package web

import (
	"context"
	"io"
	"maps"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/a-h/templ"
	"golang.org/x/net/html"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// libraryView is what every page about a library shows of it.
type libraryView struct {
	// vetted is false for a library that's only listed: each of its pages warns that it isn't vetted, carries noindex
	// and nofollow, and names no canonical address.
	vetted                                 bool
	href, owner, name, description, avatar string
	// githubURL is the repository on GitHub.
	githubURL string
	// ownerHref leads to the library's owner: their page on Rulemart for a vetted library, or, since an owner has a
	// page only with a vetted library, their profile on GitHub for any other.
	ownerHref string
	// license is the declared SPDX expression, and licenseFile the declared license file, which licenseURL links;
	// each is empty when the library declares none.
	license, licenseFile, licenseURL string
	// latestHref leads to the latest release on the Library releases tab.
	latestTag, latestHref, updated string
	// groups counts the groups that hold current rules, rules the current rules, and releases the releases.
	groups, rules, releases int
	// addedBy is the login of the account whose listing named the library, or empty for a library Rulemart vetted
	// without one, and addedOn when the library came to Rulemart.
	addedBy, addedOn string
}

// fullName returns the library's repository as owner/name.
func (l libraryView) fullName() string { return l.owner + "/" + l.name }

// fileAtVersionURL returns file on GitHub at library release n.
func (l libraryView) fileAtVersionURL(file string, n int) string {
	return domain.BlobURL(l.fullName(), domain.ReleaseTag(n), file)
}

// maxSummary is the most characters a page's description holds: search engines show about 160, and cut the rest.
const maxSummary = 200

// summary returns text as a page's description: its whitespace collapsed, and cut at a word to at most maxSummary
// characters, with an ellipsis when it's cut.
func summary(text string) string {
	words := strings.Fields(text)
	if whole := strings.Join(words, " "); utf8.RuneCountInString(whole) <= maxSummary {
		return whole
	}
	// Cut to the words that fit with room for the ellipsis.
	var out strings.Builder
	length := 0
	for _, word := range words {
		n := utf8.RuneCountInString(word)
		if length > 0 {
			n++
		}
		if length+n > maxSummary-1 {
			break
		}
		if length > 0 {
			out.WriteByte(' ')
		}
		out.WriteString(word)
		length += n
	}
	if length == 0 {
		// A first word longer than a description: cut it.
		return string([]rune(words[0])[:maxSummary-1]) + "…"
	}
	return out.String() + "…"
}

// summary returns what a library's pages say about it to search engines: its own description, or else what it
// holds.
func (l libraryView) summary() string {
	if l.description != "" {
		return l.description
	}
	return l.fullName() + ": " + plural(l.rules, "rule", "rules") + " for coding agents, in a Code Rules library on Rulemart."
}

// newLibraryView describes lib.
func newLibraryView(lib views.Library) libraryView {
	v := libraryView{
		vetted: lib.Vetted, href: libraryHref(lib.Owner, lib.Name), owner: lib.Owner, name: lib.Name, description: lib.Description,
		avatar: lib.OwnerAvatarURL, githubURL: domain.RepositoryURL(lib.FullName()),
		latestTag: domain.ReleaseTag(lib.LatestRelease), updated: date(lib.LatestTaggedAt),
		license: lib.LicenseExpression, licenseFile: lib.LicenseFile,
		groups: lib.Groups, rules: lib.Rules, releases: lib.LatestRelease, addedBy: lib.AddedBy, addedOn: date(lib.AddedAt),
	}
	v.latestHref = releaseHref(v, lib.LatestRelease)
	if lib.Vetted {
		v.ownerHref = ownerHref(lib.Owner)
	} else {
		v.ownerHref = domain.OwnerURL(lib.Owner)
	}
	if lib.LicenseFile != "" {
		v.licenseURL = domain.BlobURL(lib.FullName(), v.latestTag, lib.LicenseFile)
	}
	return v
}

// libraryCard is a library's row in a list of libraries.
type libraryCard struct {
	href, owner, name, description, avatar string
	rules                                  int
	// vetted marks a library the release vets, which shows the check mark; any other is only listed, and its link
	// carries nofollow.
	vetted bool
	// tagged marks an unvetted library in a list that holds vetted ones too, which shows the Unvetted chip.
	tagged bool
}

// newLibraryCards describes libraries, tagging each unvetted one when tagUnvetted is true, for a list that mixes them
// with vetted ones.
func newLibraryCards(libraries []views.LibraryCard, tagUnvetted bool) []libraryCard {
	cards := make([]libraryCard, len(libraries))
	for i, lib := range libraries {
		cards[i] = libraryCard{
			href: libraryHref(lib.Owner, lib.Name), owner: lib.Owner, name: lib.Name, description: lib.Description,
			avatar: lib.OwnerAvatarURL, rules: lib.Rules, vetted: lib.Vetted, tagged: tagUnvetted && !lib.Vetted,
		}
	}
	return cards
}

// groupView is a group with its current rules.
type groupView struct {
	label groupLabel
	icon  groupIcon
	// blurb says which rules belong in the group: the canonical list's description of a canonical group, as the
	// groups page shows it, and the library's of any other.
	blurb string
	// anchor is the group's section on the library's All rules tab.
	anchor string
	// acrossHref is the group's page across libraries, including unvetted ones when the library is one, so it lists the
	// library's own rules.
	acrossHref string
	// rules are the group's current rules, in title order, and retired its retired rules, in title order, which the
	// All rules tab shows after them when asked to.
	rules, retired []ruleRowView
}

// groupLabel is how pages name a group: a canonical group by the canonical list's name, and any other group by its
// ID, flagged as not canonical, never by the name its library declares.
type groupLabel struct {
	id string
	// name is empty when the group isn't canonical.
	name      string
	canonical bool
}

func newGroupLabel(id string, canonical *views.CanonicalGroup) groupLabel {
	if canonical == nil {
		return groupLabel{id: id}
	}
	return groupLabel{id: id, name: canonical.Name, canonical: true}
}

// display returns how text names the group: a canonical group by its name, and any other by its ID.
func (l groupLabel) display() string {
	if l.canonical {
		return l.name
	}
	return l.id
}

// initial returns the first letter of a canonical group's name, which stands in for an icon it doesn't have.
func (l groupLabel) initial() string {
	for _, r := range l.name {
		return strings.ToUpper(string(r))
	}
	return ""
}

// notCanonicalExplanation is the hover text of a group's "not canonical" flag.
const notCanonicalExplanation = "Not on Code Rules' canonical group list, which names the groups libraries share, " +
	"so this group stands alone."

// groupIcon is the icon beside a group.
type groupIcon struct {
	// src is empty when the group has no icon.
	src string
	// monochrome icons are inverted in dark themes, narrow ones drawn larger, and lightTile ones shown on a light tile
	// in every theme.
	monochrome, narrow, lightTile bool
}

// newGroupIcon returns the icon pages show beside a group: a canonical group's when Rulemart has one, and otherwise
// none. iconURL returns where the site serves an icon file.
func newGroupIcon(canonical *views.CanonicalGroup, iconURL func(file string) string) groupIcon {
	if canonical == nil || canonical.Icon.File == "" {
		return groupIcon{}
	}
	icon := canonical.Icon
	return groupIcon{src: iconURL(icon.File), monochrome: icon.Monochrome, narrow: icon.Narrow, lightTile: icon.LightTile}
}

// ruleRowView is a rule in a list of rules, as every list on the site shows one, the prototype's ruleResult: its
// title and impact, its library, and its stars.
type ruleRowView struct {
	href, title, impact string
	stars               int
	library             libraryRefView
	// unvetted marks a rule of a library that's only listed, in a list that includes such libraries: its row shows the
	// Unvetted chip, and its link carries nofollow.
	unvetted bool
	// group names the rule's group in a list that shows it, such as the visitor's starred rules, and is nil in a list
	// of one group's rules or under its group's heading.
	group *groupLabel
	// retired marks a retired rule, which the row draws grayed out, with the Retired chip, and replacedBy names the last
	// of the rules that replaced it, by title, or is empty when none did. renamed reports that the replacement is the
	// same rule under a new ID, so replacedBy names it by ID instead, the one thing that tells the two apart.
	retired    bool
	replacedBy string
	renamed    bool
	// retiredIn is the tag of the library release that retired the rule, in a list of one library's rules, or empty.
	retiredIn string
	// missing holds the words of a search, as the visitor wrote them, that the rule doesn't hold.
	missing []string
	// marks holds the words of a search, in lowercase, that the row marks in its title, or none outside search.
	marks []string
	// starredAs is the ID of the retired rule the visitor starred, which this one replaced, or empty.
	starredAs string
}

// newRuleRow describes r, a rule of lib, as a row; unvetted is true for a rule of a library that's only listed, in a
// list that includes such libraries.
func newRuleRow(lib libraryRefView, unvetted bool, r views.RuleCard) ruleRowView {
	return ruleRowView{
		href: ruleHref(lib.href, r.Path), title: titleOrID(r.Title, r.Path), impact: r.Impact, stars: r.Stars, library: lib,
		unvetted: unvetted,
	}
}

// newListedRuleRow describes r, a row of a list of rules across libraries, with its retirement, naming the last of its
// replacements, and the words of a search it lacks.
func newListedRuleRow(r views.RuleRow) ruleRowView {
	row := newRuleRow(newLibraryRefView(r.Library), !r.Vetted, r.Rule)
	row.retired, row.missing = r.Retired, r.Missing
	if r.Replacement != nil {
		row.replacedBy = titleOrID(r.Replacement.Title, r.Replacement.Path)
		if r.Renamed {
			row.replacedBy, row.renamed = r.Replacement.Path, true
		}
	}
	return row
}

// libraryContents is a library's groups, split by kind, in path order, each with its current rules and its retired
// rules, each in title order. A group whose rules are all retired is among them, without current rules.
type libraryContents struct {
	techs, practices []groupView
	// hasRetired reports whether the library has retired rules.
	hasRetired bool
}

// newLibraryContents groups the page's rules under their groups. iconURL returns where the site serves an icon file.
func newLibraryContents(lib libraryView, page views.LibraryPage, iconURL func(file string) string) libraryContents {
	ref := libraryRefView{href: lib.href, owner: lib.owner, name: lib.name, avatar: lib.avatar}
	current, retired := map[string][]ruleRowView{}, map[string][]ruleRowView{}
	for _, r := range page.Rules {
		current[r.Group] = append(current[r.Group], newRuleRow(ref, false, r))
	}
	// A group whose rules are all retired has no row of its own, so its retired rules name it, as the canonical list
	// shows it.
	retiredGroups := map[string]views.Group{}
	for _, r := range page.Retired {
		retired[r.Group] = append(retired[r.Group], newRetiredRuleRow(lib, ref, r))
		retiredGroups[r.Group] = views.Group{Path: r.Group, Canonical: r.CanonicalGroup}
	}
	groups := slices.Clone(page.Groups)
	for _, g := range page.Groups {
		delete(retiredGroups, g.Path)
	}
	groups = append(groups, slices.Collect(maps.Values(retiredGroups))...)
	slices.SortFunc(groups, func(a, b views.Group) int { return strings.Compare(a.Path, b.Path) })
	result := libraryContents{hasRetired: len(page.Retired) > 0}
	for _, g := range groups {
		view := groupView{
			label: newGroupLabel(g.Path, g.Canonical), icon: newGroupIcon(g.Canonical, iconURL), anchor: groupAnchor(g.Path),
			rules: current[g.Path], retired: retired[g.Path], acrossHref: withUnvetted(groupHref(g.Path), !lib.vetted),
		}
		slices.SortStableFunc(view.retired, func(a, b ruleRowView) int {
			return strings.Compare(strings.ToLower(a.title), strings.ToLower(b.title))
		})
		view.blurb = g.Description
		if g.Canonical != nil {
			view.blurb = g.Canonical.Description
		}
		if kindOf(g.Path) == practicesKind {
			result.practices = append(result.practices, view)
		} else {
			result.techs = append(result.techs, view)
		}
	}
	return result
}

// newRetiredRuleRow describes r, a retired rule of lib, whose page names it as ref, as a row on the All rules tab:
// grayed out, with the release that retired it and the last of the rules that replaced it.
func newRetiredRuleRow(lib libraryView, ref libraryRefView, r views.RetiredRuleCard) ruleRowView {
	row := ruleRowView{
		href: ruleHref(lib.href, r.Path), title: titleOrID(r.Title, r.Path), impact: r.Impact, library: ref,
		retired: true, retiredIn: domain.ReleaseTag(r.RetiredIn), renamed: r.Renamed,
	}
	if n := len(r.Replacements); n > 0 {
		last := r.Replacements[n-1]
		row.replacedBy = titleOrID(last.Title, last.Path)
		if r.Renamed {
			row.replacedBy = last.Path
		}
	}
	return row
}

// practiceBlurb returns what a library group page's line adds after the group's rules: a practice's blurb, after a
// separator, or nothing for a technology, whose name says what it is.
func practiceBlurb(g groupView) string {
	if g.blurb == "" || kindOf(g.label.id) != practicesKind {
		return ""
	}
	return " · " + g.blurb
}

// retiredReplacementWord returns how a retired rule's row words its replacement: as first when it starts the line, and
// as after when it follows the release that retired the rule, retiredIn.
func retiredReplacementWord(retiredIn, first, after string) string {
	if retiredIn == "" {
		return first
	}
	return after
}

// retiredSeparator returns what separates the release that retired a row's rule from its replacement, when it names
// one.
func retiredSeparator(r ruleRowView) string {
	if r.replacedBy == "" {
		return ""
	}
	return ", "
}

// selectionParam carries the groups ticked on a library's Groups tab, by ID, joined by commas, so the ticks outlive a
// visit to a group's page and back.
const selectionParam = "sel"

// groupSelection is the groups ticked on a library's Groups tab, by ID, in the tab's order.
type groupSelection []string

// newGroupSelection reads the groups query ticks, among groups, by their IDs, matched without regard to case, in the
// order the tab lists them. A value may join several by commas, and the parameter may repeat; anything else is left
// out.
func newGroupSelection(query url.Values, groups []groupView) groupSelection {
	ticked := map[string]bool{}
	for _, value := range query[selectionParam] {
		for id := range strings.SplitSeq(value, ",") {
			ticked[strings.ToLower(id)] = true
		}
	}
	var selection groupSelection
	for _, g := range groups {
		if ticked[strings.ToLower(g.label.id)] {
			selection = append(selection, g.label.id)
		}
	}
	return selection
}

// has reports whether the group id is ticked.
func (s groupSelection) has(id string) bool { return slices.Contains(s, id) }

// query returns the selection as a query string, ?sel=techs/go,practices/testing, or empty when nothing is ticked.
// Group IDs hold only lowercase letters, digits, hyphens, and one slash, which a query holds as they are.
func (s groupSelection) query() string {
	if len(s) == 0 {
		return ""
	}
	return "?" + selectionParam + "=" + strings.Join(s, ",")
}

// current returns the groups that hold current rules, technologies first, which the Groups tab lists.
func (c libraryContents) current() []groupView {
	var groups []groupView
	for _, g := range c.all() {
		if len(g.rules) > 0 {
			groups = append(groups, g)
		}
	}
	return groups
}

// ofKind returns the groups of kind among groups.
func ofKind(groups []groupView, kind groupKind) []groupView {
	var matching []groupView
	for _, g := range groups {
		if kindOf(g.label.id) == kind {
			matching = append(matching, g)
		}
	}
	return matching
}

// all returns every group, technologies first.
func (c libraryContents) all() []groupView {
	return append(append([]groupView{}, c.techs...), c.practices...)
}

// group returns the group whose ID is id, matched without regard to case, that holds current rules, and whether there
// is one.
func (c libraryContents) group(id string) (groupView, bool) {
	all := c.current()
	i := slices.IndexFunc(all, func(g groupView) bool { return strings.EqualFold(g.label.id, id) })
	if i < 0 {
		return groupView{}, false
	}
	return all[i], true
}

// ruleView is what a rule's page shows.
type ruleView struct {
	library                 libraryView
	href, id, title, impact string
	version, html           string
	// whenToRead is the reading guidance as text, and whenToReadHTML as rendered Markdown, or empty when the catalog
	// holds no HTML for it, so the page shows the text.
	whenToRead, whenToReadHTML string
	group                      groupLabel
	// acrossHref is the group's page across libraries, including unvetted ones when the library is one, so it lists
	// the rule.
	acrossHref string
	// updated is when the release that published the current version was tagged.
	updated string
	// fileURL is the rule's file on GitHub, at the release that published the current version.
	fileURL, fileName string
	versions          []versionView
	// compareHref compares the oldest version with the newest; empty when there's only one.
	compareHref string
	// retired is nil while the rule is current.
	retired *retiredView
	// replaces are the retired rules this one replaced, and renamedFrom the one it renamed, or nil.
	replaces    []replacedRule
	renamedFrom *replacedRule
	// tags are the topics the current version lists, each leading to a search for it.
	tags []tagView
	// assets are the rule's own files, then the shared files it links to, which its Rule tab lists.
	assets []assetView
	// star is the rule's star control, which the rule's page fills in.
	star starView
	// groupIcon is the icon of the rule's group, which the page fills in, and groupRules counts the group's current
	// rules, this one included, which the cart's dialog offers whole.
	groupIcon  groupIcon
	groupRules int
}

// tagView is one of a rule's tags, which leads to a search for it.
type tagView struct {
	text, href string
}

// newTagViews describes tags, each leading to search.
func newTagViews(tags []string) []tagView {
	views := make([]tagView, len(tags))
	for i, tag := range tags {
		views[i] = tagView{text: tag, href: searchHref + "?" + url.Values{domain.QueryParam: {tag}}.Encode()}
	}
	return views
}

// retiredView is how a library release retired a rule.
type retiredView struct {
	tag, href, date string
	summaries       []string
	// replacedBy is the rule that replaced it, then the one that replaced that one, and so on, to a current rule.
	replacedBy []ruleLink
	// renamed reports that the replacement is the same rule under a new ID.
	renamed bool
}

// replacedRule is a retired rule that a rule replaced, and the release that retired it.
type replacedRule struct {
	rule         ruleLink
	tag, tagHref string
}

// versionView is one row of a rule's Versions tab.
type versionView struct {
	version, tag, date, notesURL string
	// releaseHref leads to the release on the library's Library releases tab, and compareHref compares the version
	// with the one before it, previous; both compare fields are empty for the first version.
	releaseHref, previous, compareHref string
	latest, major                      bool
	summaries                          []string
}

// summary returns what a rule's pages say about it to search engines: when to read it, or else what it is.
func (r ruleView) summary() string {
	if r.whenToRead != "" {
		return r.whenToRead
	}
	return r.title + ": a rule for coding agents in " + r.library.fullName() + ", a Code Rules library on Rulemart."
}

// newRuleView describes the rule on page, a rule of lib.
func newRuleView(lib libraryView, page views.RulePage) ruleView {
	r, file := page.Rule, domain.RuleFile(page.Rule.Path)
	v := ruleView{
		library: lib, href: ruleHref(lib.href, r.Path), id: r.Path, title: titleOrID(r.Title, r.Path), impact: r.Impact,
		version: r.Version.String(), whenToRead: plainText(r.WhenToRead, r.WhenToReadHTML),
		group:      newGroupLabel(r.Group, r.CanonicalGroup),
		acrossHref: withUnvetted(groupHref(r.Group), !lib.vetted),
		updated:    date(r.PublishedAt), fileName: path.Base(file),
		fileURL: domain.BlobURL(page.Library.FullName(), domain.ReleaseTag(r.Release), file),
	}
	for i, version := range page.Versions {
		row := versionView{
			version: version.Version.String(), tag: domain.ReleaseTag(version.Release),
			date: date(version.PublishedAt), notesURL: domain.ReleaseNotesURL(page.Library.FullName(), version.Release),
			releaseHref: releaseHref(lib, version.Release),
			latest:      i == 0 && r.Retirement == nil, major: version.Change == coderules.ChangeMajor, summaries: shortened(version.Summaries),
		}
		if i+1 < len(page.Versions) {
			previous := page.Versions[i+1].Version
			row.previous, row.compareHref = previous.String(), ruleComparisonHref(lib, r.Path, previous, version.Version, diffWords)
		}
		v.versions = append(v.versions, row)
	}
	if n := len(page.Versions); n > 1 {
		v.compareHref = ruleComparisonHref(lib, r.Path, page.Versions[n-1].Version, page.Versions[0].Version, diffWords)
	}
	// The group holds the rule itself while it's current, whatever the links say.
	v.groupRules = max(page.GroupRuleCount(), 1)
	v.tags, v.assets = newTagViews(r.Tags), newAssetViews(v, page.Assets)
	v.html, v.whenToReadHTML = pageHTML(r.HTML, lib, r.Path), pageHTML(r.WhenToReadHTML, lib, r.Path)
	if retirement := r.Retirement; retirement != nil {
		v.retired = &retiredView{
			tag: domain.ReleaseTag(retirement.Release), href: releaseHref(lib, retirement.Release),
			date: date(retirement.RetiredAt), summaries: shortened(retirement.Summaries),
		}
		v.retired.replacedBy, v.retired.renamed = newRuleLinks(lib, retirement.Replacements), retirement.Renamed
		// A retired rule's text sits under the page's heading for its last version, so its headings go a level down.
		v.html = demoteHeadings(v.html)
	}
	replaced := func(r views.RuleRef) replacedRule {
		return replacedRule{rule: newRuleLink(lib, r), tag: domain.ReleaseTag(r.RetiredIn), tagHref: releaseHref(lib, r.RetiredIn)}
	}
	for _, r := range page.Replaces {
		v.replaces = append(v.replaces, replaced(r))
	}
	if page.RenamedFrom != nil {
		from := replaced(*page.RenamedFrom)
		v.renamedFrom = &from
	}
	return v
}

// libraryHref is the path of a library's page.
func libraryHref(owner, name string) string {
	return "/" + url.PathEscape(owner) + "/" + url.PathEscape(name)
}

// libraryGroupHref is the path of the page of the group id of the library whose page is library.
func libraryGroupHref(library, id string) string {
	kind, name, _ := strings.Cut(id, "/")
	return library + "/" + url.PathEscape(kind) + "/" + url.PathEscape(name)
}

// groupAnchor is the fragment of a group's section on the All rules tab.
func groupAnchor(groupID string) string {
	return "group-" + strings.ReplaceAll(groupID, "/", "-")
}

// date writes a day as pages show it, such as 2 Sep 2026, in UTC so every visitor and cache sees the same text.
func date(t time.Time) string {
	return t.UTC().Format("2 Jan 2006")
}

// plural returns "1 rule" or "n rules".
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// pluralWord returns one or many by n, without the number.
func pluralWord(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// impactLevelsDocs is Code Rules' explanation of impact levels.
const impactLevelsDocs = "https://code-rules.fabricahq.com/reference/rule-authoring/#describe-impact-through-consequences"

// impactExplanations say what each impact level means, following Code Rules' rule-authoring reference: how serious
// the problem is that a rule helps prevent, not how much code applying it changes.
var impactExplanations = map[string]string{
	"CRITICAL":    "Critical impact: this rule helps prevent severe harm, such as irreversible data loss or a major security breach.",
	"HIGH":        "High impact: this rule helps prevent substantial correctness, reliability, or maintainability problems.",
	"MEDIUM-HIGH": "Medium-high impact: the problems this rule helps prevent fall between medium and high.",
	"MEDIUM":      "Medium impact: this rule helps prevent meaningful but bounded defects or recurring development friction.",
	"LOW-MEDIUM":  "Low-medium impact: the problems this rule helps prevent fall between low and medium.",
	"LOW":         "Low impact: this rule improves local clarity or consistency, with limited consequences.",
}

// impactExplanation returns the hover text for an impact label, and a plain one for a level Code Rules doesn't
// define.
func impactExplanation(level string) string {
	if explanation, ok := impactExplanations[level]; ok {
		return explanation
	}
	return level + " impact, as the library declares it."
}

// plainText returns a rule's reading guidance as text, for places that show no markup, such as a search result or a
// page's description: the text of html, the guidance rendered as Markdown, or when there's none, text as written.
// Ingestion's renderer wrote html, escaping every character of the guidance that markup would read, so its text is the
// text nodes, unescaped, with runs of spaces collapsed.
func plainText(text, rendered string) string {
	if rendered == "" {
		return text
	}
	var out strings.Builder
	tokens := html.NewTokenizer(strings.NewReader(rendered))
	for {
		switch tokens.Next() {
		case html.ErrorToken:
			return strings.Join(strings.Fields(out.String()), " ")
		case html.TextToken:
			out.Write(tokens.Text())
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			// Block elements, such as paragraphs and list items, separate words.
			if name, _ := tokens.TagName(); !inlineElements[string(name)] {
				out.WriteByte(' ')
			}
		}
	}
}

// inlineElements are the elements Markdown renders within a line of text, which separate no words.
var inlineElements = map[string]bool{"a": true, "code": true, "em": true, "strong": true, "del": true, "img": true, "span": true}

// breakable shows text, such as an ID or a file name, so it wraps at its parts: each part up to a / or : stays whole
// on a line unless it's longer than the line, and then breaks after a hyphen or _, as words do. It writes the parts
// escaped, with no space between them, which a templ template would add.
func breakable(text string) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		var out strings.Builder
		for i, part := range breakParts(text, "/:") {
			if i > 0 {
				out.WriteString("<wbr>")
			}
			out.WriteString(`<span class="id-part">`)
			for j, piece := range breakParts(part, "_") {
				if j > 0 {
					out.WriteString("<wbr>")
				}
				out.WriteString(templ.EscapeString(piece))
			}
			out.WriteString("</span>")
		}
		_, err := io.WriteString(w, out.String())
		return err
	})
}

// breakParts splits text after each of separators, except at its end. Joined, the parts are text.
func breakParts(text, separators string) []string {
	var parts []string
	for len(text) > 0 {
		i := strings.IndexAny(text, separators)
		if i < 0 || i == len(text)-1 {
			break
		}
		parts, text = append(parts, text[:i+1]), text[i+1:]
	}
	return append(parts, text)
}

// labelStyle is the type of a label: small, uppercase, and spaced.
const labelStyle = "text-[12px] font-medium tracking-[.12em] text-muted uppercase"

// linkTag matches the start of a link's tag.
var linkTag = regexp.MustCompile(`<a\s`)

// pageHTML returns stored, HTML that ingestion's renderer wrote for the rule at rulePath in lib or one of its assets,
// as a page of lib shows it: with each link to a shared asset's page naming the rule, and when lib isn't vetted, every
// link marked as its author's.
func pageHTML(stored string, lib libraryView, rulePath string) string {
	// A shared asset's page shows it as this rule's.
	html := ruleContext(stored, lib, rulePath)
	if !lib.vetted {
		// A library that isn't vetted wrote its links; they lend it none of Rulemart's standing with search engines.
		html = untrustedLinks(html)
	}
	return html
}

// ruleContext returns rendered, HTML ingestion's renderer wrote for the rule at rulePath in lib, with each link to a
// shared asset's page naming the rule, so that page shows the asset as the rule's. The renderer escapes every < in
// text and writes each link's href itself, so only its links match.
func ruleContext(rendered string, lib libraryView, rulePath string) string {
	shared := regexp.MustCompile(`href="(` + regexp.QuoteMeta(lib.href+"/"+domain.SharedAssetDir) + `[^"#?]*)(#[^"]*)?"`)
	return shared.ReplaceAllString(rendered, `href="$1?`+ruleParam+`=`+ruleQuery(rulePath)+`$2"`)
}

// untrustedLinks returns rendered, HTML that ingestion's renderer wrote, with every link marked rel="nofollow ugc", as
// links an unvetted library's author wrote. The renderer escapes every < in text and writes no rel, so only its link
// tags match.
func untrustedLinks(rendered string) string {
	return linkTag.ReplaceAllLiteralString(rendered, `<a rel="nofollow ugc" `)
}

// headingTag matches the start or end tag of an HTML heading of levels 1 to 5.
var headingTag = regexp.MustCompile(`<(/?)h([1-5])\b`)

// demoteHeadings returns rendered, HTML that ingestion's renderer wrote, with each heading a level lower: an h2 becomes
// an h3. The renderer escapes every < in text, so only its tags match.
func demoteHeadings(rendered string) string {
	return headingTag.ReplaceAllStringFunc(rendered, func(tag string) string {
		level := tag[len(tag)-1] - '0'
		return tag[:len(tag)-1] + strconv.Itoa(int(level)+1)
	})
}
