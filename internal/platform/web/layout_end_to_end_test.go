package web_test

import (
	"slices"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git/gittest"
)

// movedLibrary returns a library whose release/1 keeps its files at the repository's root, in the earlier layout,
// and whose release/2 moved them into .trailguide/, changing one rule and adding a shared file the other links to.
func movedLibrary(t *testing.T) *gittest.Library {
	t.Helper()
	lib := gittest.NewLibrary(t)
	lib.Group("practices/testing", "Testing")
	lib.Rule("practices/testing/verify-retry-limits", "Verify retry limits", "Stop after a fixed number of attempts.")
	lib.Rule("practices/testing/test-changed-behavior", "Test the behavior you changed", "Test it.")
	lib.Release(1, `formatVersion: 1
release: 1
rules: {practices/testing/test-changed-behavior: 1.0.0, practices/testing/verify-retry-limits: 1.0.0}
changes:
  practices/testing/test-changed-behavior: {change: new, summaries: [Add the rule.]}
  practices/testing/verify-retry-limits: {change: new, summaries: [Add the rule.]}
`)
	lib.MoveToTrailGuideDirectory()
	lib.Rule("practices/testing/verify-retry-limits", "Verify retry limits",
		"Stop after a fixed number of attempts. See [the lifecycle](../../../assets/lifecycle.svg).")
	lib.Write(".trailguide/assets/lifecycle.svg", loopSVG)
	lib.Release(2, `formatVersion: 1
release: 2
rules: {practices/testing/test-changed-behavior: 1.0.0, practices/testing/verify-retry-limits: 1.1.0}
changes: {practices/testing/verify-retry-limits: {change: minor, from: 1.0.0, summaries: [Show the lifecycle.]}}
libraryFiles: [assets/lifecycle.svg]
`)
	return lib
}

// Pages link each file on GitHub where the release that holds it keeps it: a rule's file at the root for a version
// published before the move, and in .trailguide/ after it, and a shared file in .trailguide/assets/.
func TestPagesLinkFilesOnGitHubWhereEachReleaseKeepsThem(t *testing.T) {
	handler := ingest(t, movedLibrary(t))
	const blob = "https://github.com/example/rules/blob/"

	for rule, want := range map[string]string{
		"verify-retry-limits":   blob + "release/2/.trailguide/rules/practices/testing/verify-retry-limits.md",
		"test-changed-behavior": blob + "release/1/practices/testing/test-changed-behavior.md",
	} {
		page := get(t, handler, library+"/practices/testing/"+rule).Body.String()
		if got := linksTo(t, page, blob); !slices.Contains(got, want) {
			t.Errorf("%s's file leads to %q, want %q", rule, got, want)
		}
	}
	asset := get(t, handler, library+"/assets/lifecycle.svg?rule=practices/testing/verify-retry-limits").Body.String()
	if got, want := links(t, asset, "View on GitHub"), blob+"release/2/.trailguide/assets/lifecycle.svg"; !slices.Contains(got, want) {
		t.Errorf("the shared file leads to %q, want %q", got, want)
	}
	if got, want := links(t, asset, "Raw"), "https://raw.githubusercontent.com/example/rules/refs/tags/release/2/.trailguide/assets/lifecycle.svg"; !slices.Contains(got, want) {
		t.Errorf("the shared file's raw copy is %q, want %q", got, want)
	}
}
