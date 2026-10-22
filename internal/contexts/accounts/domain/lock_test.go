package domain

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/trailguide"
)

// lockConfig takes go rules from fabrica, on GitHub, pinning one, excluding one, and replacing one, and skills from
// tools, elsewhere.
const lockConfig = `schemaVersion: 2
sources:
  fabrica:
    repository: https://github.com/fabricahq/public-library.git
    ref: release/2
    rules:
      include:
        - techs/go/*
      pins:
        - id: techs/go/naming
          version: "1.0.0"
          reason: Waiting on the rename.
      exclude:
        - id: techs/go/logging
          reason: We log through our platform.
        - id: techs/go/errors
          reason: Our services wrap errors.
          replacedBy: techs/go/wrap-errors
  tools:
    repository: https://gitlab.com/acme/tools.git
    ref: main
    skills:
      include:
        - "*"
`

// lockFile returns the lockfile Trail Guide would write for lockConfig.
func lockFile(t *testing.T) []byte {
	t.Helper()
	one, two := 1, 2
	v1, v2 := trailguide.RuleVersion{Major: 1}, trailguide.RuleVersion{Major: 2}
	commit := func(c string) string { return strings.Repeat(c, 40) }
	lock := trailguide.Lock{Sources: map[string]trailguide.LockedSource{
		"fabrica": {
			Kind: trailguide.KindLibrary, Repository: "https://github.com/fabricahq/public-library.git", Ref: "release/2", Release: 2, Commit: commit("2"),
			ReleaseCommits: map[int]string{1: commit("1"), 2: commit("2")},
			Selectors:      trailguide.LockSelectors{Groups: json.RawMessage(`["techs/go"]`), Rules: []string{}, Skills: []string{}},
			Groups:         []string{"techs/go"},
			Rules: map[string]trailguide.LockedRule{
				"techs/go/errors": {Version: &v1, Release: &one}, "techs/go/logging": {Version: &v1, Release: &one},
				"techs/go/naming": {Version: &v1, Release: &one}, "techs/go/tests": {Version: &v2, Release: &two}, "techs/go/draft": {},
			},
			RetiredRules: []string{}, Skills: map[string]trailguide.LockedSkill{}, AvailableSkills: map[string]trailguide.LockedAvailableSkill{},
			Files: map[string]string{"config.yaml": strings.Repeat("a", 64)}, Executable: []string{}, Curated: map[string]trailguide.LockedCurated{},
		},
		"tools": {
			Kind: trailguide.KindNonLibrary, Repository: "https://gitlab.com/acme/tools.git", Ref: "main", Commit: commit("3"),
			Selectors: trailguide.LockSelectors{Skills: []string{"*"}}, Skills: map[string]trailguide.LockedSkill{"tools/plan": {Included: true, UsedBy: []string{}}},
			AvailableSkills: map[string]trailguide.LockedAvailableSkill{"tools/plan": {}}, Files: map[string]string{"tools/plan/SKILL.md": strings.Repeat("d", 64)}, Executable: []string{},
		},
	}}
	data, err := lock.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// Only the rules agents receive at a published version that an update could change count: an excluded, replaced,
// pinned, or unreleased rule doesn't, and a source elsewhere than GitHub keeps no library.
func TestParseLockKeepsTheActiveRulesAnUpdateCouldChange(t *testing.T) {
	sources, err := ParseLock([]byte(lockConfig), lockFile(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 {
		t.Fatalf("got %+v, want two sources", sources)
	}
	fabrica, tools := sources[0], sources[1]
	want := []PinnedRule{{Path: "techs/go/tests", Version: trailguide.RuleVersion{Major: 2}}}
	if fabrica.Name != "fabrica" || fabrica.Library != "fabricahq/public-library" || fabrica.Release != 2 || !slices.Equal(fabrica.Groups, []string{"techs/go"}) || !slices.Equal(fabrica.Rules, want) {
		t.Errorf("fabrica is %+v", fabrica)
	}
	if tools.Name != "tools" || tools.Library != "" || tools.Release != 0 || len(tools.Rules) != 0 {
		t.Errorf("tools is %+v", tools)
	}
}

func TestParseLockRefusesAProjectItCantRead(t *testing.T) {
	lock := lockFile(t)
	for name, files := range map[string][2]string{
		"an edited lockfile":       {lockConfig, strings.Replace(string(lock), `"release/2"`, `"release/3"`, 1)},
		"a source without entry":   {strings.Replace(lockConfig, "  tools:", "  other:\n    repository: https://github.com/acme/other.git\n    ref: main\n    skills:\n      include: [\"*\"]\n  tools:", 1), string(lock)},
		"an invalid configuration": {"schemaVersion: 2\nsources: []\n", string(lock)},
		"no sources":               {"schemaVersion: 2\nsources: {}\n", `{"lockVersion": 1, "sources": {}}`},
		"too large":                {lockConfig, string(lock) + strings.Repeat(" ", MaxLockBytes)},
	} {
		if sources, err := ParseLock([]byte(files[0]), []byte(files[1])); err == nil {
			t.Errorf("%s: parsed %+v", name, sources)
		}
	}
}
