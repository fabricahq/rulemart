// Shape catalog rows into what each page shows: text, counts, dates, and links.

package site

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fabricahq/rulemart/internal/site/sitedb"
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
func newLibraryView(lib library) libraryView {
	v := libraryView{
		href: libraryHref(lib.owner, lib.name), owner: lib.owner, name: lib.name, description: lib.description,
		avatar: lib.avatar, githubURL: "https://github.com/" + lib.owner + "/" + lib.name,
		ownerURL: "https://github.com/" + lib.owner, latestTag: releaseTag(lib.latestRelease),
		updated: date(lib.latestAt),
	}
	v.latestURL = releaseNotesURL(v.githubURL, lib.latestRelease)
	v.license, v.licenseFile = lib.license, lib.licenseFile
	if lib.licenseFile != "" {
		v.licenseURL = v.githubURL + "/blob/" + v.latestTag + "/" + escapePath(lib.licenseFile)
	}
	return v
}

// libraryCard is a library's row in a list of libraries.
type libraryCard struct {
	href, owner, name, description, avatar string
	rules                                  int
}

func newLibraryCards(rows []sitedb.ListLibrariesRow) []libraryCard {
	cards := make([]libraryCard, len(rows))
	for i, row := range rows {
		cards[i] = libraryCard{
			href: libraryHref(row.Owner, row.Name), owner: row.Owner, name: row.Name, description: row.Description,
			avatar: row.OwnerAvatarUrl, rules: int(row.RuleCount),
		}
	}
	return cards
}

// groupView is a group with its current rules.
type groupView struct {
	id, name string
	// blurb tells a reader when the group applies. Technology names explain themselves, so only practices have one.
	blurb string
	// anchor is the group's section on the library's All rules tab.
	anchor string
	rules  []ruleCard
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

// newLibraryContents groups rules under groups, keeping both orders.
func newLibraryContents(lib libraryView, c contents) libraryContents {
	byGroup := map[string][]ruleCard{}
	for _, r := range c.rules {
		byGroup[r.GroupID] = append(byGroup[r.GroupID], ruleCard{
			href: lib.href + "/" + r.RuleID, id: r.RuleID, title: r.Title, impact: r.Impact,
			version: version(r.Major, r.Minor, r.Patch),
		})
	}
	var result libraryContents
	for _, g := range c.groups {
		view := groupView{id: g.GroupID, name: g.Name, anchor: groupAnchor(g.GroupID), rules: byGroup[g.GroupID]}
		if strings.HasPrefix(g.GroupID, "practices/") {
			view.blurb = g.WhenToRead
			result.practices = append(result.practices, view)
		} else {
			result.techs = append(result.techs, view)
		}
	}
	result.ruleCount = len(c.rules)
	return result
}

// all returns every group, technologies first.
func (c libraryContents) all() []groupView {
	return append(append([]groupView{}, c.techs...), c.practices...)
}

// ruleView is what a rule's page shows.
type ruleView struct {
	library                       libraryView
	href, id, title, impact       string
	version, whenToRead, html     string
	groupID, groupName, groupHref string
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

// newRuleView describes rule r of lib.
func newRuleView(lib libraryView, r rule) ruleView {
	v := ruleView{
		library: lib, href: lib.href + "/" + r.RuleID, id: r.RuleID, title: r.Title, impact: r.Impact,
		version: version(r.Major, r.Minor, r.Patch), whenToRead: r.WhenToRead, html: r.Html,
		groupID: r.GroupID, groupName: r.GroupName, groupHref: lib.href + "?tab=rules#" + groupAnchor(r.GroupID),
		updated: date(r.PublishedAt.Time), fileName: r.RuleID[strings.LastIndex(r.RuleID, "/")+1:] + ".md",
		fileURL: lib.githubURL + "/blob/" + releaseTag(int(r.Release)) + "/" + escapePath(r.RuleID+".md"),
	}
	for i, row := range r.versions {
		v.versions = append(v.versions, versionView{
			version: version(row.Major, row.Minor, row.Patch), tag: releaseTag(int(row.Release)),
			date: date(row.PublishedAt.Time), notesURL: releaseNotesURL(lib.githubURL, int(row.Release)),
			latest: i == 0, major: row.Change == "major", summaries: row.Summaries,
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

func releaseTag(number int) string { return "release/" + strconv.Itoa(number) }

// releaseNotesURL is the GitHub Release page Code Rules creates for a library release.
func releaseNotesURL(githubURL string, number int) string {
	return githubURL + "/releases/tag/" + releaseTag(number)
}

func version(major, minor, patch int32) string {
	return strconv.Itoa(int(major)) + "." + strconv.Itoa(int(minor)) + "." + strconv.Itoa(int(patch))
}

// date writes a day as pages show it, such as 2 Sep 2026, in UTC so every visitor and cache sees the same text.
func date(t time.Time) string {
	return t.UTC().Format("2 Jan 2006")
}

// escapePath percent-encodes each segment of a repository path for a URL.
func escapePath(file string) string {
	segments := strings.Split(file, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
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
