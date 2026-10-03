// Package views holds what the catalog's pages read: the vetted libraries, an owner's libraries, a library with its
// groups and rules, its releases and what changed between two of them, a rule with its versions and what changed
// between two of them, the groups across libraries, lists of rules across libraries, a group's or a search's, the sitemap,
// an account's listings and starred rules, and a cart's checkout. They're plain values, read from one state of the catalog,
// with nothing of how it's stored.
package views

import (
	"strings"
	"time"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// HomePage is the vetted libraries, ordered by owner and name, and their groups.
type HomePage struct {
	Libraries []LibraryCard
	Groups    GroupIndex
}

// LibraryCard is a library in a list of libraries.
type LibraryCard struct {
	// Owner and Name are spelled as the code host spells them now.
	Owner, Name, Description string
	// OwnerAvatarURL is empty when the host reported none.
	OwnerAvatarURL string
	// Rules counts the library's current rules.
	Rules int
	// Vetted is false for a library that's only listed, which a list includes only when the visitor asks for unvetted
	// libraries.
	Vetted bool
}

// OwnerPage is an owner's page: the vetted libraries they publish.
type OwnerPage struct {
	// Login is the owner's login as the code host spells it now, and AvatarURL their avatar, empty when the host
	// reported none; both are read from the owner's libraries.
	Login, AvatarURL string
	// Libraries are the owner's vetted libraries, ordered by name without regard to case, and never empty: an owner
	// without one has no page.
	Libraries []LibraryCard
}

// Library is a vetted or listed library, as every page about it describes it.
type Library struct {
	// Vetted is false for a library that's only listed, whose pages warn that it hasn't been vetted.
	Vetted bool
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

// RuleCard is a current rule in a list of rules.
type RuleCard struct {
	// Path is the rule's ID, and Group its group's path.
	Path, Group   string
	Title, Impact string
	Version       coderules.RuleVersion
	// Stars counts the accounts whose stars count toward the rule: on it, or on a retired rule whose chain of
	// replacements reaches it. It's 0 in a library that isn't vetted, whose stars stay stored, uncounted.
	Stars int
}

// RulePage is a rule of a library, current or retired, with every version, newest first.
type RulePage struct {
	Library  Library
	Rule     Rule
	Versions []Version
	// Assets are the files a current rule's page lists: its own, in path order, then the shared files it links to, in
	// path order. A retired rule has none.
	Assets []Asset
	// Replaces are the retired rules whose retirement named this one as their replacement, in path order, and
	// RenamedFrom is the one this rule renamed, if any, which Replaces leaves out.
	Replaces    []RuleRef
	RenamedFrom *RuleRef
	// Links are how every rule of the library was replaced: the page read lists every rule the library has had,
	// current or retired, so GroupRuleCount can count from it.
	Links []RuleLink
}

// GroupRuleCount returns how many current rules the rule's group holds in its library.
func (p RulePage) GroupRuleCount() int {
	n := 0
	for _, l := range p.Links {
		if l.RetiredIn == 0 && strings.HasPrefix(l.Path, p.Rule.Group+"/") {
			n++
		}
	}
	return n
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
	HTML string
	// Tags are the topics the version's frontmatter lists, in its order; empty when it lists none, or the catalog
	// doesn't have them yet.
	Tags    []string
	Version coderules.RuleVersion
	// Release is the number of the library release that published the version, tagged at PublishedAt.
	Release     int
	PublishedAt time.Time
	// Retirement is nil while the rule is current.
	Retirement *Retirement
	// Stars counts the accounts whose stars count toward a current rule, as RuleCard's do; it's 0 for a retired rule,
	// whose stars count toward its replacement, and in a library that isn't vetted.
	Stars int
}

// Asset is one of a rule's supporting files, as its page lists it.
type Asset struct {
	// Path is the file's path in the repository: in the rule's asset directory, or under domain.SharedAssetDir.
	Path string
	Size int64
	// MediaType is what ingestion found the file to be, which domain.AssetKindOf reads.
	MediaType string
	// Release is the number of the library release whose commit the copy is from.
	Release int
	// Kept reports whether the catalog keeps the file's bytes: Rulemart serves an image it keeps, and a page shows
	// Markdown or text it keeps.
	Kept bool
}

// AssetPage is one of a rule's assets, with the page of the rule whose Assets list it.
type AssetPage struct {
	Page  RulePage
	Asset Asset
	// HTML is how the page shows a Markdown or text file the catalog keeps: rendered, or as code; empty otherwise.
	HTML string
}

// AssetImage is an image the catalog keeps, as Rulemart serves it.
type AssetImage struct {
	MediaType string
	Content   []byte
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

// LibraryGroup is a group as one library holds it.
type LibraryGroup struct {
	// Path is the group's ID, such as techs/go.
	Path    string
	Library LibraryRef
	// Vetted is false for a library that's only listed.
	Vetted bool
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
	// Vetted is false when only listed libraries hold the group, in an index that includes them.
	Vetted bool
}

// GroupPage is a group's page: the group, and its rules across libraries.
type GroupPage struct {
	// Path is the group's ID, such as techs/go.
	Path string
	// Canonical is nil when Path isn't on Code Rules' canonical group list.
	Canonical *CanonicalGroup
	Rules     RuleResults
}

// RuleResults are one page of a list of rules across libraries, a group's or a search's, as domain.RuleList describes
// it.
type RuleResults struct {
	// Rows are the page's rules, in the list's order, each group's together.
	Rows []RuleRow
	// Total counts the rules that pass the list's filters, Complete those of them that hold every word a search finds,
	// and Libraries the libraries they come from.
	Total, Complete, Libraries int
	// Unfiltered counts the rules the list holds before its filters, and UnfilteredLibraries the libraries they come
	// from, Fabrica's first, then by owner and name, each with how many of them it holds.
	Unfiltered          int
	UnfilteredLibraries []LibraryCount
	// RetiredRules counts the retired rules of the list's group, or of every group, in its libraries, whether or not
	// the list holds them, so a page offers to show them only when there are some.
	RetiredRules int
	// NoWords reports a search with no word to find, which matches nothing: only words to leave out, or only words
	// search ignores, such as "the", or punctuation.
	NoWords bool
}

// LibraryCount is a library in a list of rules, with how many of the list's rules, before its filters, it holds.
type LibraryCount struct {
	Library LibraryRef
	// Vetted is false for a library that's only listed.
	Vetted bool
	Rules  int
}

// RuleRow is a rule in a list of rules across libraries.
type RuleRow struct {
	Library LibraryRef
	// Vetted is false for a rule of a library that's only listed.
	Vetted bool
	// Rule is the rule's newest version; its Stars are 0 for a retired rule, or one of a library that isn't vetted.
	Rule RuleCard
	// CanonicalGroup is nil when Rule.Group isn't on Code Rules' canonical group list.
	CanonicalGroup *CanonicalGroup
	// Retired marks a rule a library release retired. Replacement is the last rule of its chain of replacements to now,
	// the one current now unless the chain ends at a rule retired without one, or nil when its retirement named none;
	// Renamed reports that each step of the chain renamed the rule, so Replacement is the same rule under a new ID.
	Retired     bool
	Replacement *RuleRef
	Renamed     bool
	// Links are how every rule of a retired rule's library was replaced, which the store reads for app.Pages to follow
	// to its Replacement; they're nil for a current rule.
	Links []RuleLink
	// Missing holds the words to find, as the visitor wrote them, that the rule doesn't hold; it's empty when the rule
	// holds every one.
	Missing []string
	// GroupRules counts the rules of the rule's group that pass the list's filters, on this page and others.
	GroupRules int
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
	// SharedFiles are the numbers of the releases after From, up to To, that changed library-wide files, in order.
	SharedFiles []int
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

// AccountListing is one of an account's listings, as its listings page shows it.
type AccountListing struct {
	ID int64
	// Owner and Name are the repository's as its lister gave them.
	Owner, Name string
	// RepositoryID is the code host's ID for the repository, or empty until the worker has looked it up.
	RepositoryID string
	State        domain.ListingState
	// Library is the library the listing names, as the code host spells it now, while the catalog stores it, and the
	// zero LibraryRef until then.
	Library LibraryRef
	// Failure says why the last check failed, or is empty.
	Failure string
	// ListedAt is when the account listed it, RequestedAt when it last asked for a check, by listing it or trying it
	// again, and CheckedAt when the worker last finished checking it, or the zero time until it does.
	ListedAt, RequestedAt, CheckedAt time.Time
}

// StarredRule is a current rule of a vetted library that an account's stars count toward, on its Starred rules.
type StarredRule struct {
	Library LibraryRef
	Rule    RuleCard
	// CanonicalGroup is nil when Rule.Group isn't on Code Rules' canonical group list.
	CanonicalGroup *CanonicalGroup
	// StarredAs is the ID of the retired rule the account starred, which Rule replaced, directly or through other
	// retired rules; it's empty when the account starred Rule itself.
	StarredAs string
	// StarredAt is when the account last starred Rule or a rule it replaced.
	StarredAt time.Time
}

// CartLibrary is a library a cart names, as checkout reads it from one snapshot of the catalog.
type CartLibrary struct {
	Library LibraryRef
	// Vetted is false for a library the release doesn't vet, which a listing names.
	Vetted bool
	// LatestRelease is the number of the library's latest release, and LatestCommit the commit its tag pointed to when
	// Rulemart ingested it.
	LatestRelease int
	LatestCommit  string
	// Rules are the rules, current and retired, of the groups the cart names of the library, a rule's or a whole
	// group's, in group path, then title and ID order.
	Rules []CartRule
}

// MaxCartTitleRunes bounds a rule's title as a cart shows it. A library may write a title of any length, up to its whole
// rule file, so a longer one is cut short, which keeps a checkout within what a response holds.
const MaxCartTitleRunes = 200

// CartRule is a rule of a group a cart names, with its newest version.
type CartRule struct {
	// Path is the rule's ID, and Group its group's, as the library spells them.
	Path, Group string
	// Title is the newest version's, cut to MaxCartTitleRunes characters with an ellipsis when it's longer, or empty when
	// the catalog doesn't have it yet.
	Title   string
	Version coderules.RuleVersion
	// RetiredIn is the number of the library release that retired the rule, or 0 while it's current.
	RetiredIn int
}

// Checkout is a browser's cart resolved against the catalog, and the texts that import what it can.
type Checkout struct {
	// Libraries are in the order the cart first names an item of each, with the cart's items from it.
	Libraries []ResolvedLibrary
	// Unknown are the cart's keys that name no item a cart can hold, which nothing resolves.
	Unknown []string
	// Commands and Prompt import every item whose State is CartItemReady, and are empty when none is. Commands are in
	// steps, as domain.Checkout's Commands says.
	Commands []domain.CommandStep
	Prompt   string
	// PinExample is the release the Commands tab's footnote suggests pinning a library to, or nil when the commands
	// follow no vetted library.
	PinExample *domain.ReleasePin
}

// ResolvedLibrary is a library a cart names, with the cart's items from it.
type ResolvedLibrary struct {
	// Library is spelled as the code host spells it now, or as the cart does for a library that's Gone, which has no
	// avatar, release, or pages: the catalog has no library by that name that's vetted or listed.
	Library LibraryRef
	Gone    bool
	// Vetted is false for a library the release doesn't vet, and Confirmed true when the visitor confirmed adding its
	// items anyway, without which checkout leaves them out.
	Vetted, Confirmed bool
	LatestRelease     int
	Items             []ResolvedItem
	// RestOfGroups are the groups of the library's rules that stay in sync, other than groups the cart holds whole,
	// whose other rules the visitor can add too. RestOfGroupsAdded is the visitor's choice to add them, and
	// RestOfGroupsRules counts their current rules the cart doesn't hold, 0 once they're added.
	RestOfGroups      []ResolvedGroup
	RestOfGroupsRules int
	RestOfGroupsAdded bool
}

// ResolvedGroup is a group as checkout names it.
type ResolvedGroup struct {
	// Path is the group's ID, as the library spells it. Canonical is nil when it isn't on Code Rules' canonical group
	// list.
	Path      string
	Canonical *CanonicalGroup
}

// ResolvedItem is one item of a cart, as checkout resolved it.
type ResolvedItem struct {
	// Key is the item's key, as the cart sent it, and Item what it names, with its library as the cart spells it and
	// its path as the library does, when it has the item.
	Key   string
	Item  domain.CartItem
	State CartItemState
	// Group is the item's group, a rule's or a group's own, as the library spells it.
	Group ResolvedGroup
	// Title, Version, and RetiredIn are a rule's, as CartRule's are; they're zero for a group, and for a rule the
	// library doesn't have.
	Title     string
	Version   coderules.RuleVersion
	RetiredIn int
	// Fork is true for a rule the visitor forks rather than keep in sync, which no unvetted library's rule is. InGroup is
	// true for a ready rule whose group the cart holds whole, ready too, which brings the rule, so checkout imports it on
	// its own only as a fork.
	Fork, InGroup bool
	// Rules are a whole group's current rules, which it brings.
	Rules []CartRule
}

// CartItemState says whether checkout imports a cart's item, or why it leaves it out.
type CartItemState string

const (
	// CartItemReady is an item checkout imports.
	CartItemReady CartItemState = "ready"
	// CartItemRetired is a rule a library release retired.
	CartItemRetired CartItemState = "retired"
	// CartItemMissing is a rule the library doesn't have, or a group without current rules.
	CartItemMissing CartItemState = "missing"
	// CartItemGone is an item of a library that's neither vetted nor listed, which has no pages.
	CartItemGone CartItemState = "gone"
	// CartItemUnvetted is an item checkout would import but for its library, which the release doesn't vet and the
	// visitor didn't confirm adding from, such as one added before the library lost its vetting. An item checkout
	// leaves out for another reason says that reason instead, since confirming wouldn't include it.
	CartItemUnvetted CartItemState = "unvetted"
)

// Sitemap is what search engines may index: every vetted library, with its current rules, and the groups that hold
// them.
type Sitemap struct {
	// Libraries are ordered by owner and name without regard to case.
	Libraries []SitemapLibrary
	// Groups are the IDs of the groups that hold the libraries' current rules, each once, in ID order, canonical or
	// not.
	Groups []string
	// Truncated reports that the sitemap left out groups or rules past the most it reads, after the first groups and
	// its libraries' first rules.
	Truncated bool
}

// SitemapLibrary is a vetted library in the sitemap.
type SitemapLibrary struct {
	Owner, Name string
	// Updated is when the library's latest release was tagged.
	Updated time.Time
	// Rules are its current rules, in ID order.
	Rules []SitemapRule
}

// SitemapRule is a current rule in the sitemap.
type SitemapRule struct {
	Path string
	// Updated is when the release that published the rule's current version was tagged.
	Updated time.Time
}
