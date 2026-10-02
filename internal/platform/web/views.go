// Shape the catalog's page reads into what each page shows: text, counts, dates, and links.

package web

import (
	"cmp"
	"context"
	"io"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

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
	// githubURL is the repository, and ownerURL its owner, on GitHub.
	githubURL, ownerURL string
	// license is the declared SPDX expression, and licenseFile the declared license file, which licenseURL links;
	// each is empty when the library declares none.
	license, licenseFile, licenseURL string
	// latestHref leads to the latest release on the Library releases tab.
	latestTag, latestHref, updated string
	// groups counts the groups that hold current rules, rules the current rules, and releases the releases.
	groups, rules, releases int
	// star is the library's star control, which server.libraryView fills in for a vetted library's pages.
	star starView
}

// fullName returns the library's repository as owner/name.
func (l libraryView) fullName() string { return l.owner + "/" + l.name }

// fileAtVersionURL returns file on GitHub at library release n.
func (l libraryView) fileAtVersionURL(file string, n int) string {
	return domain.BlobURL(l.fullName(), domain.ReleaseTag(n), file)
}

// newLibraryView describes lib.
func newLibraryView(lib views.Library) libraryView {
	v := libraryView{
		vetted: lib.Vetted, href: libraryHref(lib.Owner, lib.Name), owner: lib.Owner, name: lib.Name, description: lib.Description,
		avatar: lib.OwnerAvatarURL, githubURL: domain.RepositoryURL(lib.FullName()),
		ownerURL: domain.OwnerURL(lib.Owner), latestTag: domain.ReleaseTag(lib.LatestRelease), updated: date(lib.LatestTaggedAt),
		license: lib.LicenseExpression, licenseFile: lib.LicenseFile,
		groups: lib.Groups, rules: lib.Rules, releases: lib.LatestRelease,
	}
	v.latestHref = releaseHref(v, lib.LatestRelease)
	if lib.LicenseFile != "" {
		v.licenseURL = domain.BlobURL(lib.FullName(), v.latestTag, lib.LicenseFile)
	}
	return v
}

// libraryCard is a library's row in a list of libraries.
type libraryCard struct {
	href, owner, name, description, avatar string
	rules                                  int
	// stars counts the library's stars, which the card shows when there are any and the library is vetted.
	stars int
	// unvetted marks a library that's only listed, whose link carries nofollow.
	unvetted bool
}

// newLibraryCards describes libraries, which are unvetted when unvetted is true.
func newLibraryCards(libraries []views.LibraryCard, unvetted bool) []libraryCard {
	cards := make([]libraryCard, len(libraries))
	for i, lib := range libraries {
		cards[i] = libraryCard{
			href: libraryHref(lib.Owner, lib.Name), owner: lib.Owner, name: lib.Name, description: lib.Description,
			avatar: lib.OwnerAvatarURL, rules: lib.Rules, stars: lib.Stars, unvetted: unvetted,
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
	// acrossHref is a canonical group's page across libraries, and empty for any other group.
	acrossHref string
	rules      []ruleCard
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

// ruleCard is a rule's entry in a library's list of rules.
type ruleCard struct {
	href, id, title, impact, version string
}

// libraryContents is a library's groups, split by kind, each with its rules in title order, and its retired rules.
type libraryContents struct {
	techs, practices []groupView
	retired          []retiredRuleCard
}

// retiredRuleCard is a retired rule's row on the All rules tab.
type retiredRuleCard struct {
	href, title, id, lastVersion string
	// retiredTag is the release that retired the rule, which retiredHref shows.
	retiredTag, retiredHref string
	// replacedBy is the rule that replaced it, then the rule that replaced that one, and so on, and renamed reports that
	// the first is the same rule under a new ID.
	replacedBy []ruleLink
	renamed    bool
}

// newLibraryContents groups the page's rules under its groups, keeping both orders. iconURL returns where the site
// serves an icon file.
func newLibraryContents(lib libraryView, page views.LibraryPage, iconURL func(file string) string) libraryContents {
	byGroup := map[string][]ruleCard{}
	for _, r := range page.Rules {
		byGroup[r.Group] = append(byGroup[r.Group], newRuleCard(lib.href, r))
	}
	var result libraryContents
	for _, g := range page.Groups {
		view := groupView{
			label: newGroupLabel(g.Path, g.Canonical), icon: newGroupIcon(g.Canonical, iconURL), anchor: groupAnchor(g.Path),
			rules: byGroup[g.Path], acrossHref: acrossHref(g.Path, g.Canonical),
		}
		view.blurb = g.Description
		if g.Canonical != nil {
			view.blurb = g.Canonical.Description
		}
		if strings.HasPrefix(g.Path, "practices/") {
			result.practices = append(result.practices, view)
		} else {
			result.techs = append(result.techs, view)
		}
	}
	for _, r := range page.Retired {
		result.retired = append(result.retired, retiredRuleCard{
			href: ruleHref(lib, r.Path), title: titleOrID(r.Title, r.Path), id: r.Path, lastVersion: r.LastVersion.String(),
			retiredTag: domain.ReleaseTag(r.RetiredIn), retiredHref: releaseHref(lib, r.RetiredIn), replacedBy: newRuleLinks(lib, r.Replacements),
			renamed: r.Renamed,
		})
	}
	// Retired rules are in the order the current ones are: technologies first, by group, then by title.
	slices.SortStableFunc(result.retired, func(a, b retiredRuleCard) int {
		return cmp.Or(
			cmp.Compare(kindOrder(a.id), kindOrder(b.id)),
			strings.Compare(path.Dir(a.id), path.Dir(b.id)),
			strings.Compare(strings.ToLower(a.title), strings.ToLower(b.title)),
			strings.Compare(a.id, b.id),
		)
	})
	return result
}

// kindOrder orders a rule or group ID by its kind: technologies, then practices.
func kindOrder(id string) int {
	if strings.HasPrefix(id, "practices/") {
		return 1
	}
	return 0
}

// all returns every group, technologies first.
func (c libraryContents) all() []groupView {
	return append(append([]groupView{}, c.techs...), c.practices...)
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
	// groupHref is the group's section on the library's All rules tab, and acrossHref a canonical group's page
	// across libraries, empty for any other group.
	groupHref, acrossHref string
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

// newRuleView describes the rule on page, a rule of lib.
func newRuleView(lib libraryView, page views.RulePage) ruleView {
	r, file := page.Rule, domain.RuleFile(page.Rule.Path)
	v := ruleView{
		library: lib, href: ruleHref(lib, r.Path), id: r.Path, title: titleOrID(r.Title, r.Path), impact: r.Impact,
		version: r.Version.String(), whenToRead: plainText(r.WhenToRead, r.WhenToReadHTML), whenToReadHTML: r.WhenToReadHTML,
		html:  r.HTML,
		group: newGroupLabel(r.Group, r.CanonicalGroup), groupHref: lib.href + "?tab=rules#" + groupAnchor(r.Group),
		acrossHref: acrossHref(r.Group, r.CanonicalGroup),
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
	if !lib.vetted {
		// A library that isn't vetted wrote its links; they lend it none of Rulemart's standing with search engines.
		v.html, v.whenToReadHTML = untrustedLinks(v.html), untrustedLinks(v.whenToReadHTML)
	}
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

// acrossHref is the page of the group at path across libraries when it's canonical, and empty otherwise.
func acrossHref(path string, canonical *views.CanonicalGroup) string {
	if canonical == nil {
		return ""
	}
	return groupHref(path)
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
