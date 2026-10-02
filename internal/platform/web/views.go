// Shape the catalog's page reads into what each page shows: text, counts, dates, and links.

package web

import (
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// libraryView is what every page about a library shows of it.
type libraryView struct {
	href, owner, name, description, avatar string
	// githubURL is the repository, and ownerURL its owner, on GitHub.
	githubURL, ownerURL string
	// license is the declared SPDX expression, and licenseFile the declared license file, which licenseURL links;
	// each is empty when the library declares none.
	license, licenseFile, licenseURL string
	latestTag, latestURL, updated    string
}

// newLibraryView describes lib.
func newLibraryView(lib views.Library) libraryView {
	v := libraryView{
		href: libraryHref(lib.Owner, lib.Name), owner: lib.Owner, name: lib.Name, description: lib.Description,
		avatar: lib.OwnerAvatarURL, githubURL: domain.RepositoryURL(lib.FullName()),
		ownerURL: domain.OwnerURL(lib.Owner), latestTag: domain.ReleaseTag(lib.LatestRelease),
		latestURL: domain.ReleaseNotesURL(lib.FullName(), lib.LatestRelease), updated: date(lib.LatestTaggedAt),
		license: lib.LicenseExpression, licenseFile: lib.LicenseFile,
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
}

func newLibraryCards(libraries []views.LibraryCard) []libraryCard {
	cards := make([]libraryCard, len(libraries))
	for i, lib := range libraries {
		cards[i] = libraryCard{
			href: libraryHref(lib.Owner, lib.Name), owner: lib.Owner, name: lib.Name, description: lib.Description,
			avatar: lib.OwnerAvatarURL, rules: lib.Rules,
		}
	}
	return cards
}

// groupView is a group with its current rules.
type groupView struct {
	label groupLabel
	icon  groupIcon
	// blurb tells a reader when the group applies. Technology names explain themselves, so only practices have one.
	blurb string
	// anchor is the group's section on the library's All rules tab.
	anchor string
	rules  []ruleCard
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
	// monochrome icons are inverted in dark themes, and narrow ones drawn larger.
	monochrome, narrow bool
}

// newGroupIcon returns the icon pages show beside a group: a canonical group's when Rulemart has one, and otherwise
// none. iconURL returns where the site serves an icon file.
func newGroupIcon(canonical *views.CanonicalGroup, iconURL func(file string) string) groupIcon {
	if canonical == nil || canonical.Icon.File == "" {
		return groupIcon{}
	}
	icon := canonical.Icon
	return groupIcon{src: iconURL(icon.File), monochrome: icon.Monochrome, narrow: icon.Narrow}
}

// ruleCard is a rule's entry in a library's list of rules.
type ruleCard struct {
	href, id, title, impact, version string
}

// libraryContents is a library's groups, split by kind, each with its rules in title order.
type libraryContents struct {
	techs, practices []groupView
	ruleCount        int
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
			rules: byGroup[g.Path],
		}
		if strings.HasPrefix(g.Path, "practices/") {
			view.blurb = g.WhenToRead
			result.practices = append(result.practices, view)
		} else {
			result.techs = append(result.techs, view)
		}
	}
	result.ruleCount = len(page.Rules)
	return result
}

// all returns every group, technologies first.
func (c libraryContents) all() []groupView {
	return append(append([]groupView{}, c.techs...), c.practices...)
}

// ruleView is what a rule's page shows.
type ruleView struct {
	library                   libraryView
	href, id, title, impact   string
	version, html             string
	// whenToRead is the reading guidance as text, and whenToReadHTML as rendered Markdown, or empty when the catalog
	// holds no HTML for it, so the page shows the text.
	whenToRead, whenToReadHTML string
	group                     groupLabel
	groupHref                 string
	// updated is when the release that published the current version was tagged.
	updated string
	// fileURL is the rule's file on GitHub, at the release that published the current version.
	fileURL, fileName string
	versions          []versionView
}

// versionView is one row of a rule's Versions tab.
type versionView struct {
	version, tag, date, notesURL string
	latest, major                bool
	summaries                    []string
}

// newRuleView describes the rule on page, a rule of lib.
func newRuleView(lib libraryView, page views.RulePage) ruleView {
	r, file := page.Rule, domain.RuleFile(page.Rule.Path)
	v := ruleView{
		library: lib, href: lib.href + "/" + r.Path, id: r.Path, title: r.Title, impact: r.Impact,
		version: r.Version.String(), whenToRead: plainText(r.WhenToRead, r.WhenToReadHTML), whenToReadHTML: r.WhenToReadHTML,
		html: r.HTML,
		group: newGroupLabel(r.Group, r.CanonicalGroup), groupHref: lib.href + "?tab=rules#" + groupAnchor(r.Group),
		updated: date(r.PublishedAt), fileName: path.Base(file),
		fileURL: domain.BlobURL(page.Library.FullName(), domain.ReleaseTag(r.Release), file),
	}
	for i, version := range page.Versions {
		v.versions = append(v.versions, versionView{
			version: version.Version.String(), tag: domain.ReleaseTag(version.Release),
			date: date(version.PublishedAt), notesURL: domain.ReleaseNotesURL(page.Library.FullName(), version.Release),
			latest: i == 0, major: version.Change == coderules.ChangeMajor, summaries: version.Summaries,
		})
	}
	return v
}

// libraryHref is the path of a library's page.
func libraryHref(owner, name string) string {
	return "/" + url.PathEscape(owner) + "/" + url.PathEscape(name)
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
