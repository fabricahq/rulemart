// Parse, compare, and advance rule versions and the change levels that move them.

package coderules

import (
	"fmt"
	"regexp"
	"strconv"
)

// RuleVersion is a published rule version: plain major.minor.patch, without prerelease or build suffixes.
type RuleVersion struct {
	Major, Minor, Patch int
}

// Change is how one library release changed a rule.
type Change string

// Changes a change note or release record can give a rule. Major, minor, and patch apply only to a rule that has a version.
const (
	ChangeMajor   Change = "major"
	ChangeMinor   Change = "minor"
	ChangePatch   Change = "patch"
	ChangeNew     Change = "new"
	ChangeRetired Change = "retired"
)

// FirstRuleVersion is every new rule's version, including every rule in a library's first library release.
var FirstRuleVersion = RuleVersion{Major: 1}

// MaxRuleVersionComponent is the largest major, minor, or patch number a rule version can have.
const MaxRuleVersionComponent = 999_999_999

var ruleVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})$`)

// ParseRuleVersion accepts only canonical major.minor.patch text, such as 1.3.0, with components up to MaxRuleVersionComponent.
func ParseRuleVersion(text, location string) (RuleVersion, error) {
	parts := ruleVersionPattern.FindStringSubmatch(text)
	if parts == nil {
		return RuleVersion{}, invalid(location, "invalid rule version "+quote(text)+": expected major.minor.patch, such as 1.3.0")
	}
	major, _ := strconv.Atoi(parts[1])
	minor, _ := strconv.Atoi(parts[2])
	patch, _ := strconv.Atoi(parts[3])
	return RuleVersion{major, minor, patch}, nil
}

// String returns the canonical major.minor.patch text.
func (v RuleVersion) String() string {
	return strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor) + "." + strconv.Itoa(v.Patch)
}

// Compare returns -1, 0, or 1 as v is older than, equal to, or newer than other.
func (v RuleVersion) Compare(other RuleVersion) int {
	for _, pair := range [][2]int{{v.Major, other.Major}, {v.Minor, other.Minor}, {v.Patch, other.Patch}} {
		if pair[0] != pair[1] {
			if pair[0] < pair[1] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// Next returns the version after v for a major, minor, or patch change, resetting lower components.
// It returns v unchanged for any other change, and an error when the changed component would exceed
// MaxRuleVersionComponent, so it never returns a version ParseRuleVersion rejects.
func (v RuleVersion) Next(change Change) (RuleVersion, error) {
	next := v
	switch change {
	case ChangeMajor:
		next = RuleVersion{Major: v.Major + 1}
	case ChangeMinor:
		next = RuleVersion{Major: v.Major, Minor: v.Minor + 1}
	case ChangePatch:
		next = RuleVersion{Major: v.Major, Minor: v.Minor, Patch: v.Patch + 1}
	}
	if next.Major > MaxRuleVersionComponent || next.Minor > MaxRuleVersionComponent || next.Patch > MaxRuleVersionComponent {
		return v, fmt.Errorf("a %s change from %s exceeds the largest rule version number, %d", change, v, MaxRuleVersionComponent)
	}
	return next, nil
}

// MarshalText writes the canonical version text, so JSON and YAML records hold "1.3.0".
func (v RuleVersion) MarshalText() ([]byte, error) {
	return []byte(v.String()), nil
}

// UnmarshalText accepts only canonical version text.
func (v *RuleVersion) UnmarshalText(text []byte) error {
	parsed, err := ParseRuleVersion(string(text), "version")
	if err != nil {
		return err
	}
	*v = parsed
	return nil
}

// changeRank orders version changes so several notes on one rule resolve to the largest; other changes rank zero.
func changeRank(change Change) int {
	switch change {
	case ChangePatch:
		return 1
	case ChangeMinor:
		return 2
	case ChangeMajor:
		return 3
	}
	return 0
}

// LargerChange returns whichever of two version changes moves a rule further; ties return a.
func LargerChange(a, b Change) Change {
	if changeRank(b) > changeRank(a) {
		return b
	}
	return a
}
