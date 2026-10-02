// Package views holds what the catalog's pages read: the vetted libraries, a library with its groups and current
// rules, a current rule with its versions, the groups across libraries, one group's rules in every library, and
// search results. They're plain values, read from one state of the catalog, with nothing of how it's stored.
package views

import (
	"time"

	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// HomePage is the vetted libraries, ordered by owner and name, and their groups.
type HomePage struct {
	Libraries []LibraryCard
	Groups    GroupIndex
}

// LibraryCard is a vetted library in the list of libraries.
type LibraryCard struct {
	// Owner and Name are spelled as the code host spells them now.
	Owner, Name, Description string
	// OwnerAvatarURL is empty when the host reported none.
	OwnerAvatarURL string
	// Rules counts the library's current rules.
	Rules int
}

// Library is a vetted library, as every page about it describes it.
type Library struct {
	// Owner and Name are spelled as the code host spells them now.
	Owner, Name, Description string
	// OwnerAvatarURL is empty when the host reported none.
	OwnerAvatarURL string
	// LicenseExpression and LicenseFile are what the library declares; each is empty when it declares none.
	LicenseExpression, LicenseFile string
	// LatestRelease is the number of the library's latest release, tagged at LatestTaggedAt.
	LatestRelease  int
	LatestTaggedAt time.Time
}

// FullName returns the library's repository as owner/name.
func (l Library) FullName() string { return l.Owner + "/" + l.Name }

// LibraryPage is a library with the groups that hold current rules, in path order, and its current rules, in
// group path and then title order.
type LibraryPage struct {
	Library Library
	Groups  []Group
	Rules   []RuleCard
}

// Group is a group that holds current rules.
type Group struct {
	// Path is the group's ID, such as techs/go.
	Path string
	// Canonical is nil when Path isn't on Code Rules' canonical group list.
	Canonical *CanonicalGroup
	// Description and WhenToRead are what the library declares about the group.
	Description, WhenToRead string
	// Rules counts the group's current rules.
	Rules int
}

// CanonicalGroup is how pages show a group whose ID is on Code Rules' canonical group list: by the list's display
// name, which every library using the ID shares, rather than the name a library declares. Pages show any other
// group by its ID, flagged as not canonical.
type CanonicalGroup struct {
	Name string
	// Description is the list's one line saying which rules belong in the group.
	Description string
	// Icon is zero when Rulemart has no icon for the group.
	Icon GroupIcon
}

// GroupIcon is the icon pages show beside a canonical group.
type GroupIcon struct {
	// File is the icon's path under the site's icons, such as devicon/go-original.svg.
	File string
	// Monochrome marks an icon drawn in black or one dark color, which dark themes invert so it stays visible.
	Monochrome bool
	// Narrow marks an icon whose drawing is much narrower than its square, which pages draw larger.
	Narrow bool
}

// RuleCard is a current rule in a library's list of rules.
type RuleCard struct {
	// Path is the rule's ID, and Group its group's path.
	Path, Group   string
	Title, Impact string
	Version       coderules.RuleVersion
}

// RulePage is a current rule of a library, with every version, newest first.
type RulePage struct {
	Library  Library
	Rule     Rule
	Versions []Version
}

// Rule is a current rule as its page shows it.
type Rule struct {
	// Path is the rule's ID, and Group its group's path.
	Path, Group string
	// CanonicalGroup is nil when Group isn't on Code Rules' canonical group list.
	CanonicalGroup *CanonicalGroup
	Title, Impact  string
	// WhenToReadHTML is WhenToRead rendered as Markdown, or empty when the catalog holds no HTML rendered from it,
	// so the page shows WhenToRead as text.
	WhenToRead, WhenToReadHTML string
	HTML                       string
	Version                    coderules.RuleVersion
	// Release is the number of the library release that published the current version, tagged at PublishedAt.
	Release     int
	PublishedAt time.Time
}

// Version is one version of a rule.
type Version struct {
	Version coderules.RuleVersion
	// Release is the number of the library release that published the version, tagged at PublishedAt.
	Release     int
	PublishedAt time.Time
	Change      coderules.Change
	Summaries   []string
}

// LibraryRef names the library a rule or group comes from, on a page that shows more than one library.
type LibraryRef struct {
	// Owner and Name are spelled as the code host spells them now.
	Owner, Name string
	// OwnerAvatarURL is empty when the host reported none.
	OwnerAvatarURL string
}

// FullName returns the library's repository as owner/name.
func (l LibraryRef) FullName() string { return l.Owner + "/" + l.Name }

// LibraryGroup is a group as one vetted library holds it.
type LibraryGroup struct {
	// Path is the group's ID, such as techs/go.
	Path    string
	Library LibraryRef
	// Rules counts the library's current rules in the group.
	Rules int
}

// GroupIndex is every group that holds current rules in a vetted library, technologies and practices apart.
type GroupIndex struct {
	Techs, Practices []GroupSummary
}

// GroupSummary is a group in the index. A canonical group combines every vetted library that holds it; any other
// group stands alone, so it's one library's.
type GroupSummary struct {
	// Path is the group's ID, such as techs/go.
	Path string
	// Canonical is nil when Path isn't on Code Rules' canonical group list.
	Canonical *CanonicalGroup
	// Rules counts the current rules the group holds in Libraries.
	Rules int
	// Libraries hold the group, in owner and name order. A group that isn't canonical has exactly one.
	Libraries []LibraryRef
}

// GroupPage is a canonical group's current rules in every vetted library that holds it.
type GroupPage struct {
	// Path is the group's ID, such as techs/go.
	Path      string
	Canonical CanonicalGroup
	// Libraries hold the group's rules, in owner and name order; it's empty when no vetted library has the group.
	Libraries []GroupLibrary
}

// GroupLibrary is one library's current rules in a group, in title order.
type GroupLibrary struct {
	Library LibraryRef
	Rules   []RuleCard
}

// SearchResults are one page of the current rules of vetted libraries that match a search, best first.
type SearchResults struct {
	Results []SearchResult
	// Total counts every rule that matched, and Complete those of them that hold every term the search finds.
	Total, Complete int
	// NoWords reports a search with no word to find, which matches nothing: only words to leave out, or only words
	// search ignores, such as "the", or punctuation.
	NoWords bool
}

// SearchResult is a rule that matched a search, with its library.
type SearchResult struct {
	Library LibraryRef
	Rule    RuleCard
	// CanonicalGroup is nil when Rule.Group isn't on Code Rules' canonical group list.
	CanonicalGroup *CanonicalGroup
	// WhenToReadHTML is WhenToRead rendered as Markdown, or empty, as Rule's is.
	WhenToRead, WhenToReadHTML string
	// Missing holds the terms to find, as the visitor wrote them, that the rule doesn't hold; it's empty when the rule
	// holds every one.
	Missing []string
}
