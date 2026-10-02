// Package views holds what the catalog's pages read: the vetted libraries, a library with its groups and rules, its
// releases and what changed between two of them, a rule with its versions and what changed between two of them, the
// groups across libraries, one group's rules in every library, and search results. They're plain values, read from
// one state of the catalog, with nothing of how it's stored.
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
	// LatestRelease is the number of the library's latest release, tagged at LatestTaggedAt. Releases are numbered
	// from 1 without gaps, so it's also how many there are.
	LatestRelease  int
	LatestTaggedAt time.Time
	// Groups counts the groups that hold current rules, and Rules the current rules.
	Groups, Rules int
}

// FullName returns the library's repository as owner/name.
func (l Library) FullName() string { return l.Owner + "/" + l.Name }

// LibraryPage is a library with the groups that hold current rules, in path order, its current rules, in group path
// and then title order, and its retired rules, in path order.
type LibraryPage struct {
	Library Library
	Groups  []Group
	Rules   []RuleCard
	Retired []RetiredRuleCard
	// Links are how every rule of the library was replaced, which pages read to tell a rename from a replacement.
	Links []RuleLink
}

// RetiredRuleCard is a retired rule in a library's list of rules.
type RetiredRuleCard struct {
	// Path is the rule's ID.
	Path string
	// Title is its last version's, or empty when the catalog doesn't have it yet.
	Title       string
	LastVersion coderules.RuleVersion
	// RetiredIn is the number of the library release that retired the rule.
	RetiredIn int
	// ReplacedBy is the ID of the rule that replaced it, or empty when its retirement named none.
	ReplacedBy string
	// Replacements are that rule, then while it's retired, the rule that replaced it, and so on, to a rule current now.
	Replacements []RuleRef
	// Renamed reports that the replacement is the same rule under a new ID: added by the release that retired this
	// one, under its title.
	Renamed bool
}

// RuleLink is how a rule of a library relates to the rule that replaced it, which pages follow to name a retired
// rule's replacement, the rule that replaced that one, and so on, and to tell a rename from a replacement.
type RuleLink struct {
	// Path is the rule's ID, and Title its newest version's title, empty when the catalog doesn't have it yet.
	Path, Title string
	// RetiredIn is the number of the library release that retired the rule, or 0 while it's current.
	RetiredIn int
	// ReplacedBy is the ID of the rule that replaced it, or empty.
	ReplacedBy string
	// FirstRelease is the number of the library release that added the rule, with the title FirstTitle; LastTitle is
	// its newest version's. Either title is empty when the catalog doesn't have it yet.
	FirstRelease          int
	FirstTitle, LastTitle string
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
	// LightTile marks a colored icon drawn mostly in dark colors, which pages show on a light tile in every theme.
	LightTile bool
}

// RuleCard is a current rule in a library's list of rules.
type RuleCard struct {
	// Path is the rule's ID, and Group its group's path.
	Path, Group   string
	Title, Impact string
	Version       coderules.RuleVersion
}

// RulePage is a rule of a library, current or retired, with every version, newest first.
type RulePage struct {
	Library  Library
	Rule     Rule
	Versions []Version
	// Replaces are the retired rules whose retirement named this one as their replacement, in path order, and
	// RenamedFrom is the one this rule renamed, if any, which Replaces leaves out.
	Replaces    []RuleRef
	RenamedFrom *RuleRef
	// Links are how every rule of the library was replaced.
	Links []RuleLink
}

// Rule is a rule as its page shows it: its current version while it's current, and its last once retired.
type Rule struct {
	// Path is the rule's ID, and Group its group's path.
	Path, Group string
	// CanonicalGroup is nil when Group isn't on Code Rules' canonical group list.
	CanonicalGroup *CanonicalGroup
	// Title, Impact, and WhenToRead are the version's, and empty for a retired rule whose last version's content the
	// catalog doesn't have yet.
	Title, Impact, WhenToRead string
	// WhenToReadHTML is WhenToRead rendered as Markdown, or empty when the catalog holds no HTML rendered from it, or
	// the rule is retired, so the page shows WhenToRead as text.
	WhenToReadHTML string
	// HTML is the current version's body; empty when the rule is retired.
	HTML    string
	Version coderules.RuleVersion
	// Release is the number of the library release that published the version, tagged at PublishedAt.
	Release     int
	PublishedAt time.Time
	// Retirement is nil while the rule is current.
	Retirement *Retirement
}

// Retirement is how a library release retired a rule.
type Retirement struct {
	// Release is the number of the library release that retired the rule, tagged at RetiredAt.
	Release   int
	RetiredAt time.Time
	// Summaries holds one summary per change note that retired the rule.
	Summaries []string
	// Replacements are the rule that replaced it, then, while that one is retired too, the rule that replaced that
	// one, and so on; empty when the retirement named none.
	Replacements []RuleRef
	// Renamed reports that the rule's replacement is the same rule under a new ID.
	Renamed bool
}

// RuleRef names another rule of the same library.
type RuleRef struct {
	// Path is the rule's ID.
	Path string
	// Title is its newest version's, or empty when the catalog doesn't have it yet.
	Title string
	// RetiredIn is the number of the library release that retired the rule, or 0 while it's current.
	RetiredIn int
}

// Version is one version of a rule.
type Version struct {
	Version coderules.RuleVersion
	// Release is the number of the library release that published the version, tagged at PublishedAt.
	Release     int
	PublishedAt time.Time
	Change      coderules.Change
	Summaries   []string
	// Title is the rule's title as the version published it, which only a library's history reads; empty elsewhere,
	// and when the catalog doesn't have the version's content yet.
	Title string
}

// RuleComparison is a rule's page with the text of two of its versions, From older than To, to compare.
type RuleComparison struct {
	Page     RulePage
	From, To coderules.RuleVersion
	Text     ComparedText
}

// ComparedText is the text of two versions of a rule, as a page compares them.
type ComparedText struct {
	// Old and New are the versions' Markdown files, when State is TextShown.
	Old, New string
	State    TextState
	// OldRelease and NewRelease number the library releases that published each version, whose files a page can
	// link to when it can't show them.
	OldRelease, NewRelease int
}

// TextState says whether a page can show two versions' text.
type TextState int

const (
	// TextShown means the page has both texts.
	TextShown TextState = iota
	// TextMissing means the catalog doesn't have one of them yet: a release that stored only current versions'
	// text ingested the library, and the worker hasn't ingested it again.
	TextMissing
	// TextTooLarge means the two texts, with those the page shows before them, pass what a page compares.
	TextTooLarge
)

// LibraryHistory is everything a library's releases published: each release, and every rule's versions.
type LibraryHistory struct {
	Library Library
	// Releases are in number order, from 1.
	Releases []Release
	// Rules are every rule the releases published, current or retired, in path order.
	Rules []RuleHistory
}

// Release is one library release.
type Release struct {
	Number   int
	TaggedAt time.Time
	// UpdatesSharedFiles reports whether the release changed library-wide files, such as group metadata or shared
	// assets, after the first release.
	UpdatesSharedFiles bool
}

// RuleHistory is one rule's versions.
type RuleHistory struct {
	// Path is the rule's ID.
	Path string
	// Title is its newest version's, or empty when the catalog doesn't have it yet.
	Title string
	// Versions are oldest first, and never empty.
	Versions []Version
	// RetiredIn is the number of the library release that retired the rule, or 0 while it's current.
	RetiredIn int
	// ReplacedBy is the ID of the rule that replaced a retired rule, or empty when its retirement named none.
	ReplacedBy string
	// RetirementSummaries holds one summary per change note that retired the rule; nil while it's current.
	RetirementSummaries []string
}

// VersionAt returns the index in Versions of the rule's version after library release n: the newest version
// published by n or before, unless a release by n retired the rule. ok is false when the rule didn't exist after n.
func (r RuleHistory) VersionAt(n int) (index int, ok bool) {
	if r.RetiredIn != 0 && r.RetiredIn <= n {
		return 0, false
	}
	for i := len(r.Versions) - 1; i >= 0; i-- {
		if r.Versions[i].Release <= n {
			return i, true
		}
	}
	return 0, false
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

// ReleasesPage is one page of a library's releases, newest first, each with what it published.
type ReleasesPage struct {
	Library  Library
	Releases []ReleaseNotes
	// Older is the newest release the next page starts with, or 0 when this page ends with release 1, and Newer the
	// one the page before starts with, or 0 when this is the first page.
	Older, Newer int
	// AllReleases are every release of the library, in number order, to compare.
	AllReleases []Release
}

// ReleaseNotes is what one library release published, as Code Rules' release notes describe it: how each rule changed
// since the release before it, and every rule's version after it.
type ReleaseNotes struct {
	Release Release
	// Changes are in rule path order; the first release's are every rule it added.
	Changes []RuleChange
	// Versions are the version of every rule after the release, in rule path order.
	Versions []RuleVersionRef
}

// RuleVersionRef names one version of a rule of the same library.
type RuleVersionRef struct {
	// Path is the rule's ID.
	Path    string
	Version coderules.RuleVersion
}

// ReleaseComparison is what changed in a library between two of its releases, From before To.
type ReleaseComparison struct {
	Library Library
	// Releases are every release of the library, in number order, to compare others.
	Releases []Release
	From, To int
	// Changes are in rule path order.
	Changes []RuleChange
	// SharedFiles reports that a release after From, up to To, changed library-wide files.
	SharedFiles bool
}

// ChangeRenamed is how a rule changed when a release retired it and added it under a new ID, with the same title.
// Code Rules records a rename as a retirement and a new rule; pages show the two as one change.
const ChangeRenamed coderules.Change = "renamed"

// RuleChange is how one rule changed between two library releases.
type RuleChange struct {
	Rule RuleRef
	// Change is the largest change between the releases: new for a rule the later one has and the earlier doesn't,
	// retired for one the earlier has and the later doesn't, and otherwise the largest of major, minor, and patch
	// among the versions published between them.
	Change coderules.Change
	// From is the rule's version after the earlier release, and zero for a new rule; To is its version after the later
	// one, and zero for a retired rule.
	From, To coderules.RuleVersion
	// Versions are the versions published after the earlier release, up to the later one, newest first; none for a
	// retired rule.
	Versions []Version
	// RetirementSummaries and Replacements describe a retired rule's retirement: the rule that replaced it, then, while
	// that one was retired by the later release too, the rule that replaced that one, and so on.
	RetirementSummaries []string
	Replacements        []RuleRef
	// RenamedFrom is the rule a renamed one had been, for a ChangeRenamed; From is that rule's last version.
	RenamedFrom *RuleRef
	// Text is the text of From and To, for a rule that changed or was renamed, in a comparison of releases.
	Text ComparedText
}

// VersionPair names two versions whose text a comparison of releases reads, by their places in a library's history:
// a rule's index in its Rules, and each version's index in that rule's Versions. A renamed rule's pair spans two
// rules.
type VersionPair struct {
	// Key names the pair in the texts the comparison returns.
	Key                 string
	OldRule, OldVersion int
	NewRule, NewVersion int
}
