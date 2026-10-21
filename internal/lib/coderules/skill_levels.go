// Parse a library's skill levels: which of its skills projects that use it must, should, or may install.

package coderules

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// The levels a library gives its skills. A project that uses the library installs its required skills whatever it
// includes, its recommended skills when it includes Recommended, and an offered skill when it names it or includes
// Wildcard.
const (
	levelRequired    = "required"
	levelRecommended = "recommended"
	levelOffered     = "offered"
)

// skillLevelNames lists the levels from the strongest to the weakest.
var skillLevelNames = []string{levelRequired, levelRecommended, levelOffered}

// skillsDirectory is the folder, in a repository's Trail Guide directory, that holds the repository's authored skills,
// and, in a library's vendor snapshot, the skills the library publishes.
const skillsDirectory = "skills"

var sourceNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// SkillLevels lists, sorted, the IDs of the skills at each level the manifest names: the library's own skills,
// skills/<name>, and the skills it curates, the name of a source in its sources, a colon, and the skill's ID in that
// source's repository, such as mattpocock:skills/to-spec. Each list is empty, never nil, when the manifest names
// none, and a skill is in at most one list. Whether each own skill exists is checked against the library's skills/
// folder, and whether each curated one exists when a project imports it.
type SkillLevels struct {
	Required    []string `json:"required"`
	Recommended []string `json:"recommended"`
	Offered     []string `json:"offered"`
}

// parseSkillLevels validates library.skills at location: an object of required, recommended, and offered lists of
// skill IDs, each listed once, each curated one from one of sources, the names the configuration's sources declare.
// A missing object lists none.
func parseSkillLevels(input json.RawMessage, location string, sources []string) (SkillLevels, error) {
	result := SkillLevels{Required: []string{}, Recommended: []string{}, Offered: []string{}}
	if input == nil {
		return result, nil
	}
	fields, err := jsonObject(input, location)
	if err != nil {
		return SkillLevels{}, err
	}
	if err := knownJSONFields(fields, skillLevelNames, location); err != nil {
		return SkillLevels{}, err
	}
	seen := map[string]string{}
	for _, level := range skillLevelNames {
		raw, ok := fields[level]
		if !ok {
			continue
		}
		where := location + "." + level
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) != nil {
			return SkillLevels{}, invalid(where, "expected a list of the library's skill IDs, such as [skills/build-feature]")
		}
		ids := []string{}
		for i, item := range items {
			at := fmt.Sprintf("%s[%d]", where, i)
			id, err := jsonText(item, at)
			if err != nil {
				return SkillLevels{}, err
			}
			if _, _, curated := strings.Cut(id, ":"); curated {
				if err := validateCuratedSkill(id, at, sources); err != nil {
					return SkillLevels{}, err
				}
			} else if name, authored := strings.CutPrefix(id, skillsDirectory+"/"); !authored || !validSkillName(name) {
				return SkillLevels{}, invalid(at, "expected the ID of one of the library's skills, skills/<name>, the folder's path in .trailguide/, such as skills/build-feature, got "+quote(id))
			}
			if other, listed := seen[id]; listed {
				if other == level {
					return SkillLevels{}, invalid(at, id+" is listed more than once; list each skill once")
				}
				return SkillLevels{}, invalid(at, id+" is already "+other+"; give each skill one level")
			}
			seen[id] = level
			ids = append(ids, id)
		}
		slices.Sort(ids)
		switch level {
		case levelRequired:
			result.Required = ids
		case levelRecommended:
			result.Recommended = ids
		default:
			result.Offered = ids
		}
	}
	return result, nil
}

// validateCuratedSkill checks that id, listed at location, names a curated skill: the name of one of sources, a
// colon, and the skill's ID in that source's repository, such as mattpocock:skills/to-spec.
func validateCuratedSkill(id, location string, sources []string) error {
	source, skill, _ := strings.Cut(id, ":")
	if !sourceNamePattern.MatchString(source) {
		return invalid(location, "expected a curated skill as the name of a source in sources, a colon, and the skill's ID in that source's repository, such as mattpocock:skills/to-spec, got "+quote(id))
	}
	if err := validateSkillID(skill, location); err != nil {
		return err
	}
	if !slices.Contains(sources, source) {
		return invalid(location, id+" comes from "+source+", which isn't a source in sources; add sources."+source+" with the repository the skill comes from, and a ref unless the repository publishes library releases")
	}
	return nil
}

// validSkillName reports whether name follows the Agent Skills name grammar: 1 to 64 lowercase letters, digits, and
// hyphens, without a hyphen at either end or two in a row.
func validSkillName(name string) bool {
	if name == "" || len(name) > 64 || strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") || strings.Contains(name, "--") {
		return false
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}

// validateSkillID checks that id names a skill folder by its path in the repository that holds it, such as
// skills/build-feature: a relative path without empty, ".", or ".." segments, backslashes, control characters, a
// wildcard, or a trailing slash. It doesn't check that the folder exists.
func validateSkillID(id, location string) error {
	if !contained(id, strings.Split(id, "/")) || strings.Contains(id, "*") || len(strings.Split(id, "/")) > 63 {
		return invalid(location, "expected a skill ID, the skill folder's path in its repository, such as skills/build-feature, got "+quote(id))
	}
	return nil
}
