package web_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git/gittest"
)

// The paths of the asset library's rule and its assets' pages.
const (
	behaviorRule  = library + "/practices/testing/test-changed-behavior"
	loopPage      = behaviorRule + "/assets/loop.svg"
	whyPage       = behaviorRule + "/assets/why.md"
	casesPage     = behaviorRule + "/assets/cases.json"
	bigPage       = behaviorRule + "/assets/big.png"
	glossaryPage  = library + "/assets/glossary.md"
	behaviorQuery = "?rule=practices/testing/test-changed-behavior"
)

// loopSVG is an image a rule shows, with a script that must never run from Rulemart's origin.
const loopSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 4 4"><script>alert(1)</script><rect width="4" height="4"/></svg>`

// newAssetSite ingests a library whose rule has its own image, Markdown, code, and an image too large to keep, and
// links a shared glossary, a shared file no release has, and another rule, and whose Markdown file links the glossary
// too; a second rule links the glossary. It returns the pages' handler.
func newAssetSite(t *testing.T) http.Handler {
	t.Helper()
	lib := gittest.NewLibrary(t)
	lib.Group("practices/testing", "Testing")
	lib.Rule("practices/testing/test-changed-behavior", "Test the behavior you changed",
		"![The loop](assets/test-changed-behavior/loop.svg)\n\nSee [why](assets/test-changed-behavior/why.md), "+
			"[the cases](assets/test-changed-behavior/cases.json), [the glossary](../../assets/glossary.md#terms), "+
			"[the gone file](../../assets/gone.md), and [another rule](verify-retry-limits.md).")
	lib.Rule("practices/testing/verify-retry-limits", "Verify retry limits", "See [the glossary](../../assets/glossary.md).")
	lib.Write("practices/testing/assets/test-changed-behavior/loop.svg", loopSVG)
	lib.Write("practices/testing/assets/test-changed-behavior/why.md", "## Why it works\n\nRead the [glossary](../../../../assets/glossary.md).\n")
	lib.Write("practices/testing/assets/test-changed-behavior/cases.json", "{\"cases\": [1.005]}\n")
	lib.Write("practices/testing/assets/test-changed-behavior/big.png", strings.Repeat("x", 300<<10))
	lib.Write("assets/glossary.md", "## Terms\n\nA regression test fails on the old code.\n")
	lib.Release(1, `formatVersion: 1
release: 1
rules:
  practices/testing/test-changed-behavior: 1.0.0
  practices/testing/verify-retry-limits: 1.0.0
changes:
  practices/testing/test-changed-behavior: {change: new, summaries: [Add the rule.]}
  practices/testing/verify-retry-limits: {change: new, summaries: [Add the rule.]}
`)
	return ingest(t, lib)
}

// A rule's text leads to its assets' pages, a shared one's naming the rule, loads its images from Rulemart when it
// keeps them, and leads any other relative link to GitHub.
func TestRulePageLinksAndListsItsAssets(t *testing.T) {
	handler := newAssetSite(t)

	page := get(t, handler, behaviorRule).Body.String()

	if !strings.Contains(page, `<img src="`+loopPage+`?raw=1" alt="The loop">`) {
		t.Errorf("the rule's image doesn't load from Rulemart:\n%s", page)
	}
	for text, want := range map[string]string{
		"why":           whyPage,
		"the cases":     casesPage,
		"the glossary":  glossaryPage + behaviorQuery + "#terms",
		"the gone file": "https://github.com/example/rules/blob/release/1/assets/gone.md",
		"another rule":  "https://github.com/example/rules/blob/release/1/practices/testing/verify-retry-limits.md",
	} {
		if got := links(t, page, text); !slices.Contains(got, want) {
			t.Errorf("%q leads to %q, want %q", text, got, want)
		}
	}
	assertRunsNothingFromRules(t, page)
}

// Each asset's page says what it is to the rule and shows it: Markdown rendered with its links, code highlighted, an
// image from Rulemart, or, for a file Rulemart doesn't keep, a link to it on GitHub; the Assets panel marks it.
func TestAssetPagesShowEachAsset(t *testing.T) {
	handler := newAssetSite(t)

	for path, want := range map[string]struct {
		shows    []string
		contains string
	}{
		whyPage: {[]string{
			"rules › Testing › Test the behavior you changed",
			"Assets 5 files big.png 300 KB cases.json 19 B loop.svg 117 B why.md 70 B Shared across the library glossary.md 51 B " +
				"Not part of this rule's version. Projects get the copy from the newest library release. These files come with " +
				"the rule when you add it.",
			"why.md Supporting file for this rule, part of version 1.0.0 · 70 B View on GitHub Raw",
			"Why it works Read the glossary", "← Back to Test the behavior you changed",
		}, `href="` + glossaryPage + behaviorQuery + `"`},
		casesPage: {[]string{"cases.json Supporting file for this rule, part of version 1.0.0 · 19 B"}, `<pre><code class="language-json">`},
		loopPage:  {[]string{"loop.svg Supporting file"}, `<img class="mx-auto block h-auto max-w-full rounded-[8px]" src="` + loopPage + `?raw=1" alt="loop.svg">`},
		bigPage: {
			[]string{"big.png Supporting file for this rule, part of version 1.0.0 · 300 KB", "Rulemart doesn't keep a copy of this file. View it on GitHub"},
			`href="https://github.com/example/rules/blob/release/1/practices/testing/assets/test-changed-behavior/big.png"`,
		},
		glossaryPage + behaviorQuery: {[]string{
			"glossary.md Shared file in example/rules, used by this rule. Not part of the rule's version; this copy is from release/1 · 51 B",
			"Terms A regression test fails on the old code.", "← Back to Test the behavior you changed",
		}, `href="https://raw.githubusercontent.com/example/rules/refs/tags/release/1/assets/glossary.md"`},
	} {
		t.Run(path, func(t *testing.T) {
			resp := get(t, handler, path)
			if resp.Code != http.StatusOK {
				t.Fatalf("got %d", resp.Code)
			}
			page := resp.Body.String()
			assertShows(t, page, want.shows...)
			if !strings.Contains(page, want.contains) {
				t.Errorf("the page doesn't hold %s:\n%s", want.contains, page)
			}
			if current := strings.Count(page, `aria-current="page"`); current != 1 {
				t.Errorf("the Assets panel marks %d files, want the page's", current)
			}
			assertRunsNothingFromRules(t, page)
		})
	}
}

// A shared asset's page shows it with the rule its address names, or without one, or with one that doesn't list it,
// with the first rule that does, and names its address without the rule as canonical.
func TestSharedAssetPageShowsTheRuleItsAddressNames(t *testing.T) {
	handler := newAssetSite(t)

	for path, rule := range map[string]string{
		glossaryPage + "?rule=practices/testing/verify-retry-limits": "Verify retry limits",
		glossaryPage:                     "Test the behavior you changed",
		glossaryPage + "?rule=techs/x/y": "Test the behavior you changed",
	} {
		resp := get(t, handler, path)
		if resp.Code != http.StatusOK {
			t.Fatalf("%s: got %d", path, resp.Code)
		}
		assertShows(t, resp.Body.String(), "← Back to "+rule)
	}
}

// Rulemart serves an image it keeps from its own origin, as the type ingestion recorded, for a day, under a policy
// that runs nothing; it serves no other file, and no image it doesn't keep.
func TestAssetImagesAreServedFromRulemartOnly(t *testing.T) {
	handler := newAssetSite(t)

	resp := get(t, handler, loopPage+"?raw=1")

	if resp.Code != http.StatusOK || resp.Body.String() != loopSVG {
		t.Fatalf("got %d %q", resp.Code, resp.Body.String())
	}
	for header, want := range map[string]string{
		"Content-Type":            "image/svg+xml",
		"X-Content-Type-Options":  "nosniff",
		"Cache-Control":           "public, max-age=86400",
		"Content-Security-Policy": "default-src 'none'; style-src 'unsafe-inline'; sandbox",
	} {
		if got := resp.Header().Get(header); got != want {
			t.Errorf("%s is %q, want %q", header, got, want)
		}
	}
	for _, path := range []string{whyPage + "?raw=1", bigPage + "?raw=1", glossaryPage + "?raw=1", behaviorRule + "/assets/none.svg?raw=1"} {
		if resp := get(t, handler, path); resp.Code != http.StatusNotFound {
			t.Errorf("%s: got %d, want 404", path, resp.Code)
		}
	}
}

// An address under a rule or the library's shared directory that names no asset the rule lists is missing.
func TestAssetPagesOfNoAssetAreMissing(t *testing.T) {
	handler := newAssetSite(t)

	for _, path := range []string{
		behaviorRule + "/assets/none.md",
		library + "/assets/gone.md",
		library + "/assets/naming/none.md",
		library + "/practices/testing/verify-retry-limits/assets/loop.svg",
		library + "/practices/testing/no-such-rule/assets/loop.svg",
	} {
		if resp := get(t, handler, path); resp.Code != http.StatusNotFound {
			t.Errorf("%s: got %d, want 404", path, resp.Code)
		}
	}
}
