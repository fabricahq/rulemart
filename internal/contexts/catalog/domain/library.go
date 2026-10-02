package domain

import (
	"time"

	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// Library is everything the catalog stores about one library, as its release tags publish it.
type Library struct {
	Repository Repository
	// LicenseExpression and LicenseFile are what rule-library.yaml declares at the latest release; each is empty
	// when it declares none.
	LicenseExpression, LicenseFile string
	// Releases are in number order, from 1.
	Releases []Release
	// Groups are the groups of every rule, current or retired, in path order.
	Groups []Group
	// Rules are every rule the releases published, in path order.
	Rules []Rule
}

// CurrentRules counts the rules that aren't retired.
func (l Library) CurrentRules() int {
	current := 0
	for _, r := range l.Rules {
		if r.Content != nil {
			current++
		}
	}
	return current
}

// Release is one library release, as its annotated release/<number> tag records it.
type Release struct {
	Number int
	// TagID is the hash of the annotated tag object, which a rewritten tag changes even when it tags the same commit.
	TagID string
	// CommitID is the hash of the commit the release tags.
	CommitID string
	TaggedAt time.Time
	// UpdatesSharedFiles reports whether the release changed library-wide files, such as group metadata or shared
	// assets, which Code Rules' release notes say for every release after the first that lists library files. A
	// first release lists every file, which it adds rather than updates.
	UpdatesSharedFiles bool
}

// Group is a group's metadata at the latest release that has its _group.yaml: the latest release while the group
// holds a current rule.
type Group struct {
	// Path is the group's directory, such as techs/go.
	Path                          string
	Name, Description, WhenToRead string
}

// Rule is a rule's history, with its current content while it's current.
type Rule struct {
	// Path is the rule's ID, its file's path without .md, such as techs/go/return-errors.
	Path string
	// Group is the path of the rule's group.
	Group string
	// Versions holds every version a library release published, oldest first. It's never empty, and its last entry
	// is the current version while the rule is current.
	Versions []Version
	// RetiredIn is the number of the library release that retired the rule, or 0 while the rule is current.
	RetiredIn int
	// ReplacedBy is the rule that replaced a retired rule, when its retirement named one.
	ReplacedBy string
	// RetirementSummaries holds one summary per change note that retired the rule; nil while the rule is current.
	RetirementSummaries []string
	// Content is the current version's content; nil when the rule is retired.
	Content *Content
}

// Current returns the rule's newest version.
func (r Rule) Current() Version { return r.Versions[len(r.Versions)-1] }

// Version is one version of a rule, as the library release that published it records it.
type Version struct {
	Number coderules.RuleVersion
	// Release is the number of the library release that published the version.
	Release   int
	Change    coderules.Change
	Summaries []string
}

// Content is a rule as one version published it.
type Content struct {
	Title, Impact, ImpactDescription, WhenToRead string
	// WhenToReadHTML is WhenToRead as the page shows it, rendered as Markdown as the body is.
	WhenToReadHTML string
	// Markdown is the rule's whole file, and HTML its body as the page shows it.
	Markdown, HTML string
}
