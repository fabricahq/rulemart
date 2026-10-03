package domain

import (
	"testing"

	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

func TestUpdatesCountsNewerAndRetiredRulesOnly(t *testing.T) {
	v := func(major, minor, patch int) coderules.RuleVersion { return coderules.RuleVersion{Major: major, Minor: minor, Patch: patch} }
	states := RuleStates{
		Current: map[string]coderules.RuleVersion{"techs/go/a": v(2, 0, 0), "techs/go/b": v(1, 0, 1), "techs/go/c": v(1, 0, 0)},
		Retired: map[string]bool{"techs/go/old": true},
	}
	for name, tc := range map[string]struct {
		pinned []PinnedVersion
		want   int
	}{
		"nothing pinned":           {nil, 0},
		"up to date":               {[]PinnedVersion{{"techs/go/a", v(2, 0, 0)}, {"techs/go/c", v(1, 0, 0)}}, 0},
		"a major and a patch":      {[]PinnedVersion{{"techs/go/a", v(1, 9, 9)}, {"techs/go/b", v(1, 0, 0)}}, 2},
		"retired":                  {[]PinnedVersion{{"techs/go/old", v(1, 0, 0)}}, 1},
		"newer than the library's": {[]PinnedVersion{{"techs/go/c", v(1, 1, 0)}}, 0},
		"dropped from the library": {[]PinnedVersion{{"techs/go/gone", v(1, 0, 0)}}, 0},
	} {
		if got := states.Updates(tc.pinned); got != tc.want {
			t.Errorf("%s: got %d, want %d", name, got, tc.want)
		}
	}
}
