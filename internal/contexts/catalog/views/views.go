// Package views holds what the catalog's pages read: the vetted libraries, a library with its groups and current
// rules, and a current rule with its versions. They're plain values, read from one state of the catalog, with
// nothing of how it's stored.
package views

import (
	"time"

	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

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
	// Icon is zero when Rulemart has no icon for the group.
	Icon GroupIcon
}

// GroupIcon is the icon pages show beside a canonical group.
type GroupIcon struct {
	// File is the icon's path under the site's icons, such as devicon/go-original.svg.
	File string
	// Monochrome marks an icon drawn in black or one dark color, which dark themes invert so it stays visible.
	Monochrome bool
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
	CanonicalGroup   *CanonicalGroup
	Title, Impact    string
	WhenToRead, HTML string
	Version          coderules.RuleVersion
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
