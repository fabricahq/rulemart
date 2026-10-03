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
	// Assets are the supporting files of the current rules, their own and the shared ones they link to, each once, in
	// path order.
	Assets []Asset
}

// CurrentRules counts the rules that aren't retired.
func (l Library) CurrentRules() int {
	current := 0
	for _, r := range l.Rules {
		if r.IsCurrent() {
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

// Rule is a rule's history, with what each version published.
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
	// HTML is the newest version's body as the rule's page shows it, rendered as Markdown: the current version's, or a
	// retired rule's last. WhenToReadHTML is the current version's reading guidance, rendered; empty when the rule is
	// retired.
	HTML, WhenToReadHTML string
	// Assets are the paths of the library's Assets that a current rule's page lists: its own files, in path order,
	// then the shared files its text and Markdown files link to, in path order. Nil once the rule is retired.
	Assets []string
}

// MaxReplacements bounds how far a chain of replacements is followed: a retired rule's replacement, then while that
// one is retired, the rule that replaced it, and so on. A library that retired a rule a release, each replaced by the
// next, could otherwise make every retired rule's chain as long as its history, and a page of them grow with the
// square of its rules. A star on a retired rule counts toward the current rule its chain reaches within the bound.
const MaxReplacements = 20

// IsCurrent reports whether the rule is current: no library release has retired it.
func (r Rule) IsCurrent() bool { return r.RetiredIn == 0 }

// Current returns the rule's newest version: its current version while it's current, and its last once retired.
func (r Rule) Current() Version { return r.Versions[len(r.Versions)-1] }

// Version is one version of a rule, as the library release that published it records it.
type Version struct {
	Number coderules.RuleVersion
	// Release is the number of the library release that published the version.
	Release   int
	Change    coderules.Change
	Summaries []string
	// Content is the rule's file as the version's release published it.
	Content Content
}

// Content is a rule as one version published it.
type Content struct {
	Title, Impact, ImpactDescription, WhenToRead string
	// Tags are the topics its frontmatter lists, in its order, each once; empty when it lists none.
	Tags []string
	// Markdown is the rule's whole file: its frontmatter and its body.
	Markdown string
}
