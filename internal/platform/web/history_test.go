package web_test

import (
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

var (
	v100 = coderules.RuleVersion{Major: 1}
	v110 = coderules.RuleVersion{Major: 1, Minor: 1}
	v200 = coderules.RuleVersion{Major: 2}
)

// Text of two versions of return-errors. The newer one changes a word, and adds a paragraph that holds markup a
// library could publish to attack visitors.
const (
	returnErrorsV1 = "---\ntitle: Return errors\n---\n\n## Return errors\n\nWrap every returned error.\n"
	returnErrorsV2 = "---\ntitle: Return errors with context\n---\n\n## Return errors\n\nWrap each returned error.\n\n" +
		"<script>alert(1)</script>\n"
)

// changes is how the example library's rules changed between release 1 and release 3: a minor change to
// verify-retry-limits, a major one to return-errors, a new rule, and check-retry-backoff retired for
// verify-retry-limits.
func changes(texts bool) []views.RuleChange {
	retry := views.RuleRef{Path: "practices/testing/verify-retry-limits", Title: "Verify retry limits"}
	result := []views.RuleChange{
		{Rule: views.RuleRef{Path: "practices/testing/check-retry-backoff", Title: "Check retry backoff", RetiredIn: 3},
			Change: coderules.ChangeRetired, From: v100, RetirementSummaries: []string{"Merge it."}, Replacements: []views.RuleRef{retry}},
		{Rule: retry, Change: coderules.ChangeMinor, From: v100, To: v110,
			Versions: []views.Version{{Version: v110, Release: 2, PublishedAt: day(2), Change: coderules.ChangeMinor, Summaries: []string{"Count timeouts as attempts."}}}},
		{Rule: views.RuleRef{Path: "techs/go/close-bodies", Title: "Close bodies"}, Change: coderules.ChangeNew, To: v100,
			Versions: []views.Version{{Version: v100, Release: 3, PublishedAt: day(3), Change: coderules.ChangeNew, Summaries: []string{"Add the rule."}}}},
		{Rule: views.RuleRef{Path: "techs/go/return-errors", Title: "Return errors with context"}, Change: coderules.ChangeMajor, From: v100, To: v200,
			Versions: []views.Version{{Version: v200, Release: 3, PublishedAt: day(3), Change: coderules.ChangeMajor, Summaries: []string{"Require context on every error.", "Add an example."}}}},
	}
	if texts {
		result[1].Text = views.ComparedText{State: views.TextTooLarge, OldRelease: 1, NewRelease: 2}
		result[3].Text = views.ComparedText{Old: returnErrorsV1, New: returnErrorsV2, OldRelease: 1, NewRelease: 3}
	}
	return result
}

var exampleReleases = []views.Release{{Number: 1, TaggedAt: day(1)}, {Number: 2, TaggedAt: day(2), UpdatesSharedFiles: true}, {Number: 3, TaggedAt: day(3)}}

// historyCatalog returns newCatalog's library with its releases, a comparison of its first and last releases, and
// comparisons of return-errors' versions: one it can show, one it can't yet, and one of identical files.
func historyCatalog() catalog {
	c := newCatalog()
	all := changes(false)
	c.releases["example/rules"] = views.ReleasesPage{Library: exampleRules, AllReleases: exampleReleases, Releases: []views.ReleaseNotes{
		{Release: exampleReleases[2], Changes: []views.RuleChange{all[0], all[2], all[3]}, Versions: []views.RuleVersionRef{
			{Path: "practices/testing/verify-retry-limits", Version: v110}, {Path: "techs/go/close-bodies", Version: v100},
			{Path: "techs/go/return-errors", Version: v200},
		}},
		{Release: exampleReleases[1], Changes: []views.RuleChange{all[1]}},
		{Release: exampleReleases[0], Changes: []views.RuleChange{
			{Rule: views.RuleRef{Path: "practices/testing/verify-retry-limits", Title: "Verify retry limits"}, Change: coderules.ChangeNew, To: v100,
				Versions: []views.Version{{Version: v100, Release: 1, Change: coderules.ChangeNew, Summaries: []string{"Add the rule."}}}},
		}},
	}}
	c.releaseComparisons["example/rules 1...3"] = views.ReleaseComparison{
		Library: exampleRules, Releases: exampleReleases, From: 1, To: 3, Changes: changes(true),
	}
	c.releaseComparisons["example/rules 3...3"] = views.ReleaseComparison{Library: exampleRules, Releases: exampleReleases, From: 3, To: 3}
	page := c.rules["example/rules/techs/go/return-errors"]
	c.ruleComparisons["example/rules/techs/go/return-errors 1.0.0...2.0.0"] = views.RuleComparison{
		Page: page, From: v100, To: v200,
		Text: views.ComparedText{Old: returnErrorsV1, New: returnErrorsV2, OldRelease: 1, NewRelease: 3},
	}
	c.ruleComparisons["example/rules/techs/go/return-errors 2.0.0...2.0.0"] = views.RuleComparison{Page: page, From: v200, To: v200}
	retry := c.rules["example/rules/practices/testing/verify-retry-limits"]
	c.ruleComparisons["example/rules/practices/testing/verify-retry-limits 1.0.0...1.1.0"] = views.RuleComparison{
		Page: retry, From: v100, To: v110, Text: views.ComparedText{State: views.TextMissing, OldRelease: 1, NewRelease: 2},
	}
	return c
}

// hrefs returns every href in an HTML body.
func hrefs(t *testing.T, body string) []string {
	t.Helper()
	var result []string
	for _, field := range strings.Split(body, `href="`)[1:] {
		result = append(result, strings.ReplaceAll(field[:strings.Index(field, `"`)], "&amp;", "&"))
	}
	return result
}

func assertLinks(t *testing.T, body string, want ...string) {
	t.Helper()
	got := hrefs(t, body)
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Errorf("no link to %s", w)
		}
	}
}

// The Library releases tab shows each release as its release notes do, newest first, linking each change to the
// rule's versions and its comparison, and each release to its GitHub Release page.
func TestReleasesTabListsWhatEachReleaseChanged(t *testing.T) {
	resp := get(t, newSite(t, historyCatalog()), library+"?tab=releases")

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d", resp.Code)
	}
	page := resp.Body.String()
	assertShows(t, page,
		"Groups , 2 All rules , 2 Library releases , 3",
		"release/3 Latest Major 3 Sep 2026 Compare with release/2 Release notes Library release 3 changes 3 rules: 1 new, 1 major, and 1 retired. "+
			"New rules Close bodies techs/go/close-bodies 1.0.0 Add the rule. "+
			"Major changes Code that complied with the previous rule version could fail the new one, so review these before updating. "+
			"Return errors with context techs/go/return-errors 1.0.0 → 2.0.0 Require context on every error. Add an example. "+
			"Retired rules Check retry backoff practices/testing/check-retry-backoff last version 1.0.0 Merge it. Replaced by Verify retry limits . "+
			"All rule versions in this library release Rule Version practices/testing/verify-retry-limits 1.1.0",
		"release/2 2 Sep 2026 Compare with release/1 Release notes Library release 2 changes 1 rule: 1 minor. Minor changes "+
			"Verify retry limits practices/testing/verify-retry-limits 1.0.0 → 1.1.0 Count timeouts as attempts. "+
			"This library release also updates shared files, such as group descriptions or shared assets.",
		// A first release adds every rule, and its notes don't repeat "Add the rule." for each.
		"release/1 1 Sep 2026 Release notes Library release 1 publishes 1 rule. New rules Verify retry limits practices/testing/verify-retry-limits 1.0.0 About",
	)
	if strings.Index(page, `id="release-3"`) > strings.Index(page, `id="release-2"`) {
		t.Error("release/2 comes before release/3")
	}
	// Each release is a heading, and each kind of change one under it.
	for _, want := range []string{`<h2 class="font-mono text-[15px] font-semibold" id="release-3-title">`, `<h3 class="mt-[18px] mb-1.5 text-[13px] font-semibold">Major changes</h3>`} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %s", want)
		}
	}
	assertLinks(t, page,
		"https://github.com/example/rules/releases/tag/release/3",
		"#release-3", "/example/rules?from=2&tab=releases&to=3",
		"/example/rules/techs/go/return-errors?tab=versions",
		"/example/rules/techs/go/return-errors?from=1.0.0&tab=versions&to=2.0.0",
		"/example/rules/practices/testing/check-retry-backoff",
		"/example/rules/practices/testing/verify-retry-limits",
	)
}

// Choosing releases to compare needs no script: the form submits them to the tab, the latest and the one before it
// chosen.
func TestReleasesTabChoosesReleasesToCompare(t *testing.T) {
	page := get(t, newSite(t, historyCatalog()), library+"?tab=releases").Body.String()

	for _, want := range []string{
		`<form class="flex flex-wrap items-center gap-2 text-[14px]" action="/example/rules" method="get"><input type="hidden" name="tab" value="releases">`,
		`<select id="compare-from"`, `<option value="2" selected>release/2</option>`,
		`<select id="compare-to"`, `<option value="3" selected>release/3</option>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %s", want)
		}
	}
	if strings.Count(page, " selected>") != 2 {
		t.Error("the form doesn't choose exactly one release on each side")
	}
}

// A page of releases leads to the next, older page, and a page of older releases back to the newest; a release
// elsewhere links to the page that starts with it.
func TestReleasesTabPagesThroughOlderReleases(t *testing.T) {
	c := historyCatalog()
	newest := c.releases["example/rules"]
	newest.Releases, newest.Older = newest.Releases[:1], 2
	c.releases["example/rules"] = newest
	older := views.ReleasesPage{Library: exampleRules, AllReleases: exampleReleases, Releases: historyCatalog().releases["example/rules"].Releases[1:], Newer: 3}
	c.releases["example/rules release=2"], c.releases["example/rules release=1"] = older, older
	handler := newSite(t, c)

	first := get(t, handler, library+"?tab=releases").Body.String()
	second := get(t, handler, library+"?tab=releases&release=2")

	assertShows(t, first, "Older library releases →")
	assertLinks(t, first, "/example/rules?tab=releases&release=2")
	if second.Code != http.StatusOK {
		t.Fatalf("got %d", second.Code)
	}
	assertShows(t, second.Body.String(), "← Newer library releases", "release/2", "release/1")
	assertLinks(t, second.Body.String(), "/example/rules?tab=releases")
	if strings.Contains(second.Body.String(), `id="release-3"`) || strings.Contains(visibleText(t, second.Body.String()), "Older library releases") {
		t.Error("the page of older releases shows release/3, or a page past release/1")
	}
	// A link to a release leads to the page that holds it; the first page has one address.
	versions := get(t, handler, errorsRule+"?tab=versions").Body.String()
	assertLinks(t, versions, "/example/rules?tab=releases#release-3", "/example/rules?tab=releases&release=1#release-1")
	if resp := get(t, handler, library+"?tab=releases&release=3"); resp.Code != http.StatusFound || resp.Header().Get("Location") != "/example/rules?tab=releases" {
		t.Errorf("a release on the first page answered %d to %q", resp.Code, resp.Header().Get("Location"))
	}
}

// A link to a release the library doesn't have, or to compare one, answers not found within the library's releases:
// its header and tabs, the form to choose releases, and a way back to them.
func TestReleasesAnswerNotFoundWithinTheLibrary(t *testing.T) {
	handler := newSite(t, historyCatalog())
	for _, query := range []string{"release=0", "release=x", "release=02", "release=9", "from=one&to=3", "from=1", "from=1&to=9"} {
		t.Run(query, func(t *testing.T) {
			resp := get(t, handler, library+"?tab=releases&"+query)

			if resp.Code != http.StatusNotFound {
				t.Fatalf("got %d", resp.Code)
			}
			assertShows(t, resp.Body.String(), "example / rules", "Library releases , 3", "Compare",
				"Not found This library has no such release to show or compare. See all of this library's releases")
			if !strings.Contains(resp.Body.String(), `<meta name="robots" content="noindex">`) {
				t.Error("the page is indexed")
			}
		})
	}
	if resp := get(t, handler, "/example/missing?tab=releases&from=1&to=2"); resp.Code != http.StatusNotFound || strings.Contains(resp.Body.String(), "no such release") {
		t.Errorf("a missing library answered %d", resp.Code)
	}
}

// One release can hold as many rules as Code Rules allows a library, 10,000, and a page shows a release whole, so its
// card must stay well within what one response can hold: its changes take short markup, and its table of every
// rule's version leads to GitHub instead.
func TestReleasesTabShowsTheLargestReleaseWithinAResponse(t *testing.T) {
	c := newCatalog()
	notes := views.ReleaseNotes{Release: views.Release{Number: 1, TaggedAt: day(1)}}
	for i := range 10000 {
		path := fmt.Sprintf("practices/a-long-group-name/a-long-rule-id-of-sixty-or-so-characters-%05d", i)
		notes.Changes = append(notes.Changes, views.RuleChange{
			Rule: views.RuleRef{Path: path, Title: "A rule title of ordinary length for a rule"}, Change: coderules.ChangeNew, To: v100,
			Versions: []views.Version{{Version: v100, Release: 1, Change: coderules.ChangeNew, Summaries: []string{"Add the rule."}}},
		})
		notes.Versions = append(notes.Versions, views.RuleVersionRef{Path: path, Version: v100})
	}
	c.releases["example/rules"] = views.ReleasesPage{Library: exampleRules, AllReleases: exampleReleases[:1], Releases: []views.ReleaseNotes{notes}}

	resp := get(t, newSite(t, c), library+"?tab=releases")

	if resp.Code != http.StatusOK || resp.Body.Len() > 4<<20 {
		t.Fatalf("got %d with %d bytes", resp.Code, resp.Body.Len())
	}
	assertShows(t, resp.Body.String(), "Library release 1 publishes 10000 rules.",
		"This library release holds 10000 rules. Its release notes on GitHub list every rule's version.")
}

// A comparison of distant releases can change thousands of rules. A page shows diffs while they fit what it renders,
// each counting toward it, and then says how many more it leaves to their own comparisons.
func TestReleaseComparisonShowsDiffsWhileTheyFit(t *testing.T) {
	c := historyCatalog()
	comparison := views.ReleaseComparison{Library: exampleRules, Releases: exampleReleases, From: 1, To: 3}
	for i := range 4000 {
		comparison.Changes = append(comparison.Changes, views.RuleChange{
			Rule: views.RuleRef{Path: fmt.Sprintf("techs/go/rule-%04d", i), Title: "A rule"}, Change: coderules.ChangeMinor, From: v100, To: v110,
			Versions: []views.Version{{Version: v110, Release: 3, Change: coderules.ChangeMinor, Summaries: []string{"Add an example."}}},
			Text:     views.ComparedText{Old: "Text.\n", New: "Text, longer.\n", OldRelease: 1, NewRelease: 3},
		})
	}
	c.releaseComparisons["example/rules 1...3"] = comparison

	resp := get(t, newSite(t, c), library+"?tab=releases&from=1&to=3")

	if resp.Code != http.StatusOK || resp.Body.Len() > 4<<20 {
		t.Fatalf("got %d with %d bytes", resp.Code, resp.Body.Len())
	}
	shown := strings.Count(resp.Body.String(), `<section class="anchor-card mt-3 scroll-mt-20 overflow-hidden`)
	if shown == 0 || shown >= 4000 {
		t.Fatalf("showed %d diffs, want some, not all", shown)
	}
	assertShows(t, resp.Body.String(), fmt.Sprintf("The text of %d more rules is more than one page shows.", 4000-shown))
}

// A change's summary is one line, of any length a library writes, so pages shorten a very long one.
func TestPagesShortenAVeryLongSummary(t *testing.T) {
	c := newCatalog()
	page := c.rules["example/rules/techs/go/return-errors"]
	page.Versions = slices.Clone(page.Versions)
	page.Versions[0].Summaries = []string{strings.Repeat("Long summary ", 1000)}
	c.rules["example/rules/techs/go/return-errors"] = page

	body := get(t, newSite(t, c), errorsRule+"?tab=versions").Body.String()

	if n := strings.Count(body, "Long summary"); n == 0 || n > 100 {
		t.Fatalf("shows the summary's words %d times, want a shortened summary", n)
	}
	assertShows(t, body, "Long summary…")
}

// Comparing releases lists every change between them, then each changed rule's text with the changed words marked,
// as text, or says why it can't show it.
func TestReleaseComparisonShowsWhatChangedAndTheText(t *testing.T) {
	resp := get(t, newSite(t, historyCatalog()), library+"?tab=releases&from=3&to=1")

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d", resp.Code)
	}
	page := resp.Body.String()
	assertShows(t, page,
		"← All library releases Compare",
		"What changed 4 rules changed between release/1 and release/3: 1 new, 1 major, 1 minor, and 1 retired. "+
			"Includes a major change. Work that complied with release/1 could fail release/3, so review the changes before updating.",
		"The text of 2 rules that changed between release/1 and release/3.",
		"Verify retry limits practices/testing/verify-retry-limits.md 1.0.0 → 1.1.0 View at 1.1.0 These changes are too large to show here. "+
			"Compare the files on GitHub: 1.0.0 and 1.1.0 .",
		"Return errors with context techs/go/return-errors.md 1.0.0 → 2.0.0 +4 −2 View at 2.0.0",
		"<script>alert(1)</script>",
	)
	for _, want := range []string{
		// Only a mark right after another stands apart from it; one after a space keeps the space alone.
		`<del>every</del><ins class="g">each</ins>`,
		`&lt;script&gt;alert(1)&lt;/script&gt;`,
		`<meta name="robots" content="noindex">`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %s", want)
		}
	}
	if strings.Contains(page, "<script>alert") {
		t.Error("the page runs a rule's markup")
	}
	assertLinks(t, page,
		"/example/rules?tab=releases",
		"/example/rules?from=1&tab=releases&to=3&view=lines",
		"https://github.com/example/rules/blob/release/1/practices/testing/verify-retry-limits.md",
		"https://github.com/example/rules/blob/release/2/practices/testing/verify-retry-limits.md",
		"https://github.com/example/rules/blob/release/3/techs/go/return-errors.md",
	)
}

// The lines view is a unified diff, numbered on each side, keeping the comparison's view when it compares others.
func TestReleaseComparisonShowsLinesOnRequest(t *testing.T) {
	page := get(t, newSite(t, historyCatalog()), library+"?tab=releases&from=1&to=3&view=lines").Body.String()

	assertShows(t, page, "@@ -1,7 +1,9 @@ 1 1 --- 2 − title: Return errors 2 + title: Return errors with context 3 3 ---")
	for _, want := range []string{`<input type="hidden" name="view" value="lines">`, `aria-current="page">Lines</a>`} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %s", want)
		}
	}
}

func TestReleaseComparisonOfOneReleaseAsksForTwo(t *testing.T) {
	page := get(t, newSite(t, historyCatalog()), library+"?tab=releases&from=3&to=3").Body.String()

	assertShows(t, page, "Choose two different library releases to compare.")
}

// A rule's Versions tab compares each version with the one before, and the first with the latest, and leads to the
// release that published each.
func TestRuleVersionsTabLeadsToComparisonsAndReleases(t *testing.T) {
	page := get(t, newSite(t, historyCatalog()), errorsRule+"?tab=versions").Body.String()

	assertLinks(t, page,
		"/example/rules/techs/go/return-errors?from=1.0.0&tab=versions&to=2.0.0",
		"/example/rules?tab=releases#release-3", "/example/rules?tab=releases&release=1#release-1",
	)
}

func TestRuleComparisonShowsWhatChangedAndTheText(t *testing.T) {
	resp := get(t, newSite(t, historyCatalog()), errorsRule+"?tab=versions&from=2.0.0&to=1.0.0")

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d", resp.Code)
	}
	page := resp.Body.String()
	assertShows(t, page,
		"Rule Versions , 2 ← All versions Compare",
		"What changed 1 version Includes a major change. Work that complied with 1.0.0 could fail 2.0.0, so review the changes before updating. "+
			"2.0.0 Major release/3 3 Sep 2026 Require context on every error. Add an example. Changed text Between release/1 and release/3. "+
			"techs/go/return-errors.md 1.0.0 → 2.0.0 +4 −2 View at 2.0.0",
	)
	for _, want := range []string{`<h2 class="text-[12px] font-medium tracking-[.12em] text-muted uppercase">What changed</h2>`, `<h3 class="mono font-semibold">2.0.0</h3>`, `id="diff-techs_go_return-errors-title"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %s", want)
		}
	}
	if !strings.Contains(page, `<meta name="robots" content="noindex">`) || strings.Contains(page, "<script>alert") {
		t.Error("the comparison is indexed, or runs a rule's markup")
	}
}

func TestRuleComparisonSaysWhenItCantShowTheText(t *testing.T) {
	handler := newSite(t, historyCatalog())

	missing := get(t, handler, retryRule+"?tab=versions&from=1.0.0&to=1.1.0").Body.String()
	same := get(t, handler, errorsRule+"?tab=versions&from=2.0.0&to=2.0.0").Body.String()

	assertShows(t, missing, "Rulemart doesn't have the text of these versions yet. It reads them the next time it updates the library.")
	assertShows(t, same, "Choose two different versions to compare.")
}

// Short text can make a long diff: every line changed, or a few words in each of thousands of paragraphs. A page
// renders a bounded number of a diff's rows and marks, so it stays far below what one response can hold, and says
// when it can't show them.
func TestRuleComparisonBoundsWhatItRenders(t *testing.T) {
	var lines, oldBlocks, newBlocks strings.Builder
	for i := range 10000 {
		lines.WriteString("a\n")
		if i < 3000 {
			fmt.Fprintf(&oldBlocks, "Unchanged %d.\n\nx x x x\n\n", i)
			fmt.Fprintf(&newBlocks, "Unchanged %d.\n\nx y x y\n\n", i)
		}
	}
	for name, tc := range map[string]struct {
		text views.ComparedText
		view string
	}{
		"every line changed, as lines":                       {views.ComparedText{Old: lines.String(), New: strings.ReplaceAll(lines.String(), "a", "b")}, "lines"},
		"changed words in thousands of paragraphs, as words": {views.ComparedText{Old: oldBlocks.String(), New: newBlocks.String()}, "words"},
	} {
		t.Run(name, func(t *testing.T) {
			c := historyCatalog()
			comparison := c.ruleComparisons["example/rules/techs/go/return-errors 1.0.0...2.0.0"]
			comparison.Text = tc.text
			c.ruleComparisons["example/rules/techs/go/return-errors 1.0.0...2.0.0"] = comparison

			resp := get(t, newSite(t, c), errorsRule+"?tab=versions&from=1.0.0&to=2.0.0&view="+tc.view)

			if resp.Code != http.StatusOK || resp.Body.Len() > 256<<10 {
				t.Fatalf("got %d with %d bytes", resp.Code, resp.Body.Len())
			}
			assertShows(t, resp.Body.String(), "These changes are too large to show here.")
		})
	}
}

// A retired rule's page says when and why it was retired, and leads to what replaced it and to its versions.
func TestRetiredRulePageShowsItsRetirementAndVersions(t *testing.T) {
	c := newCatalog()
	c.rules["example/rules/practices/testing/check-retry-backoff"] = views.RulePage{
		Library: exampleRules,
		Rule: views.Rule{
			Path: "practices/testing/check-retry-backoff", Group: "practices/testing", CanonicalGroup: testingGroup,
			Title: "Check retry backoff", Impact: "HIGH", Version: v110, Release: 2, PublishedAt: day(2),
			HTML: "<h2 id=\"rule\">Rule</h2>\n<p>Wait longer after each attempt.</p>\n<h3 id=\"why\">Why</h3>\n",
			Retirement: &views.Retirement{Release: 3, RetiredAt: day(3), Summaries: []string{"Merge it."},
				Replacements: []views.RuleRef{
					{Path: "practices/testing/verify-retries", Title: "Verify retries", RetiredIn: 3},
					{Path: "practices/testing/verify-retry-limits", Title: "Verify retry limits"},
				}},
		},
		Versions: []views.Version{
			{Version: v110, Release: 2, PublishedAt: day(2), Change: coderules.ChangeMinor, Summaries: []string{"Wait longer."}},
			{Version: v100, Release: 1, PublishedAt: day(1), Change: coderules.ChangeNew, Summaries: []string{"Add the rule."}},
		},
	}

	resp := get(t, newSite(t, c), library+"/practices/testing/check-retry-backoff")

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d", resp.Code)
	}
	page := resp.Body.String()
	assertShows(t, page,
		"Check retry backoff Retired Last version 1.1.0 Retired in release/3 · 3 Sep 2026 Merge it. "+
			"Replaced by Verify retries , itself replaced by Verify retry limits .",
		// It shows its last version's text, and links that file at the release that published it.
		"Text of version 1.1.0 View on GitHub Rule Wait longer after each attempt. Why",
		"Versions · 2 1.1.0 release/2 2 Sep 2026 Wait longer. Compare with 1.0.0",
	)
	if strings.Contains(visibleText(t, page), "Latest") || strings.Contains(visibleText(t, page), "Rule Versions") {
		t.Error("a retired rule's page marks a latest version or shows tabs")
	}
	// The text's headings sit under the section's heading, a level below it.
	for _, want := range []string{`<h3 id="rule">Rule</h3>`, `<h4 id="why">Why</h4>`, `aria-label="check-retry-backoff.md at 1.1.0 on GitHub"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the page doesn't have %s", want)
		}
	}
	assertLinks(t, page, "/example/rules?tab=releases#release-3", retryRule, "/example/rules/practices/testing/verify-retries",
		"https://github.com/example/rules/blob/release/2/practices/testing/check-retry-backoff.md")
}

func TestAllRulesTabListsRetiredRules(t *testing.T) {
	c := newCatalog()
	lib := c.pages["example/rules"]
	lib.Retired = []views.RetiredRuleCard{{Path: "practices/testing/check-retry-backoff", Title: "Check retry backoff",
		LastVersion: v100, RetiredIn: 3, ReplacedBy: "practices/testing/verify-retries", Replacements: []views.RuleRef{
			{Path: "practices/testing/verify-retries", Title: "Verify retries", RetiredIn: 4},
			{Path: "practices/testing/verify-retry-limits", Title: "Verify retry limits"},
		}}}
	c.pages["example/rules"] = lib

	page := get(t, newSite(t, c), library+"?tab=rules").Body.String()

	assertShows(t, page, "Retired Check retry backoff practices/testing/check-retry-backoff · last version 1.0.0 · retired in release/3 · replaced by practices/testing/verify-retries , itself replaced by practices/testing/verify-retry-limits ›")
}

// A comparison's releases or versions come from its URL, so one that isn't a release number or version, or that the
// library or rule doesn't have, is a missing page.
func TestComparisonsAnswerNotFoundForWhatTheyCantCompare(t *testing.T) {
	handler := newSite(t, historyCatalog())
	for name, path := range map[string]string{
		"a release that isn't a number":     library + "?tab=releases&from=one&to=3",
		"a release with a leading zero":     library + "?tab=releases&from=01&to=3",
		"release 0":                         library + "?tab=releases&from=0&to=3",
		"only one release":                  library + "?tab=releases&from=1",
		"releases the library doesn't have": library + "?tab=releases&from=1&to=9",
		"a version that isn't one":          errorsRule + "?tab=versions&from=1&to=2.0.0",
		"only one version":                  errorsRule + "?tab=versions&to=2.0.0",
		"a version the rule doesn't have":   errorsRule + "?tab=versions&from=1.5.0&to=2.0.0",
	} {
		t.Run(name, func(t *testing.T) {
			if resp := get(t, handler, path); resp.Code != http.StatusNotFound {
				t.Fatalf("got %d", resp.Code)
			}
		})
	}
}

// A rename shows once, as a rename, leading to the diff of the old rule's last text with the new rule's.
func TestReleasesShowARenameAsOneChange(t *testing.T) {
	c := historyCatalog()
	renamed := views.RuleChange{
		Rule: views.RuleRef{Path: "techs/go/name-tests-by-behavior", Title: "Name tests after behavior"}, Change: views.ChangeRenamed,
		From: v100, To: v100, RenamedFrom: &views.RuleRef{Path: "techs/go/name-tests", Title: "Name tests after behavior", RetiredIn: 3},
		Versions: []views.Version{{Version: v100, Release: 3, Change: coderules.ChangeNew, Summaries: []string{"Rename it."}}},
	}
	page := c.releases["example/rules"]
	page.Releases[0].Changes = append(page.Releases[0].Changes, renamed)
	c.releases["example/rules"] = page
	comparison := c.releaseComparisons["example/rules 1...3"]
	renamed.Text = views.ComparedText{Old: "Name tests.\n", New: "Name tests by behavior.\n", OldRelease: 1, NewRelease: 3}
	comparison.Changes = append(comparison.Changes, renamed)
	c.releaseComparisons["example/rules 1...3"] = comparison
	handler := newSite(t, c)

	releases := get(t, handler, library+"?tab=releases").Body.String()
	compared := get(t, handler, library+"?tab=releases&from=1&to=3").Body.String()

	// The rename names the new rule's version, not the old and new rules' as a change from one to the other.
	assertShows(t, releases, "1 new, 1 renamed, 1 major, and 1 retired",
		"Renamed rules Name tests after behavior techs/go/name-tests-by-behavior 1.0.0 Renamed from techs/go/name-tests . Compare the text Rename it.")
	assertLinks(t, releases, "/example/rules?from=2&tab=releases&to=3#diff-techs_go_name-tests-by-behavior")
	assertShows(t, compared, "Name tests after behavior techs/go/name-tests.md → techs/go/name-tests-by-behavior.md 1.0.0 +1 −1 View at 1.0.0")

	// A rename that kept the text says so.
	comparison.Changes[len(comparison.Changes)-1].Text.New = "Name tests.\n"
	c.releaseComparisons["example/rules 1...3"] = comparison
	unchanged := get(t, newSite(t, c), library+"?tab=releases&from=1&to=3").Body.String()
	assertShows(t, unchanged, "techs/go/name-tests.md → techs/go/name-tests-by-behavior.md 1.0.0 View at 1.0.0 The text is the same; only the rule's ID changed.")
	assertLinks(t, compared, "#diff-techs_go_name-tests-by-behavior", "https://github.com/example/rules/blob/release/3/techs/go/name-tests-by-behavior.md")
}

// A rule's Rule tab names what it replaced or renamed, as its Versions tab does, and the All rules tab tells a
// renamed rule from a replaced one.
func TestRulePagesNameRenamesAndReplacements(t *testing.T) {
	c := newCatalog()
	page := c.rules["example/rules/"+"techs/go/return-errors"]
	page.RenamedFrom = &views.RuleRef{Path: "techs/go/wrap-errors", Title: "Wrap errors", RetiredIn: 3}
	page.Replaces = []views.RuleRef{{Path: "techs/go/old-errors", Title: "Old errors", RetiredIn: 2}}
	c.rules["example/rules/techs/go/return-errors"] = page
	lib := c.pages["example/rules"]
	lib.Retired = []views.RetiredRuleCard{
		{Path: "techs/go/wrap-errors", Title: "Wrap errors", LastVersion: v100, RetiredIn: 3, ReplacedBy: "techs/go/return-errors", Renamed: true,
			Replacements: []views.RuleRef{{Path: "techs/go/return-errors", Title: "Return errors"}}},
		{Path: "practices/testing/a-old", Title: "A old", LastVersion: v100, RetiredIn: 2},
	}
	c.pages["example/rules"] = lib
	handler := newSite(t, c)

	for _, tab := range []string{"", "?tab=versions"} {
		assertShows(t, get(t, handler, errorsRule+tab).Body.String(),
			"Renamed from techs/go/wrap-errors in release/3 .", "Replaces Old errors techs/go/old-errors , retired in release/2 .")
	}
	// Retired rules are in the order current ones are: technologies first.
	assertShows(t, get(t, handler, library+"?tab=rules").Body.String(),
		"Retired Wrap errors techs/go/wrap-errors · last version 1.0.0 · retired in release/3 · renamed to techs/go/return-errors › A old")
}

// A comparison names the releases in its range that changed shared files, and offers words or lines only for a diff.
func TestReleaseComparisonNotesSharedFilesAndOffersViewsOnlyForDiffs(t *testing.T) {
	c := historyCatalog()
	c.releaseComparisons["example/rules 2...3"] = views.ReleaseComparison{Library: exampleRules, Releases: exampleReleases, From: 2, To: 3, SharedFiles: []int{3}}
	c.releaseComparisons["example/rules 1...3"] = views.ReleaseComparison{Library: exampleRules, Releases: exampleReleases, From: 1, To: 3, SharedFiles: []int{2, 3}}
	handler := newSite(t, c)

	page := get(t, handler, library+"?tab=releases&from=2&to=3").Body.String()
	same := get(t, handler, library+"?tab=releases&from=3&to=3").Body.String()

	assertShows(t, page, "No rules changed between release/2 and release/3. release/3 updates shared files, such as group descriptions or shared assets.")
	assertShows(t, get(t, handler, library+"?tab=releases&from=1&to=3").Body.String(),
		"release/2 and release/3 update shared files, such as group descriptions or shared assets.")
	// A long range counts them.
	c.releaseComparisons["example/rules 1...3"] = views.ReleaseComparison{Library: exampleRules, Releases: exampleReleases, From: 1, To: 3, SharedFiles: []int{2, 3, 4, 5}}
	assertShows(t, get(t, newSite(t, c), library+"?tab=releases&from=1&to=3").Body.String(),
		"4 library releases after release/1, up to release/3, update shared files")
	for name, body := range map[string]string{"no changes": page, "the same release": same} {
		if strings.Contains(body, `aria-label="Show changes as"`) {
			t.Errorf("%s: the page offers words or lines with no diff", name)
		}
	}
}

// A long list of changes folds behind a disclosure that counts what it holds.
func TestReleasesFoldALongListOfChanges(t *testing.T) {
	c := newCatalog()
	notes := views.ReleaseNotes{Release: views.Release{Number: 1, TaggedAt: day(1)}}
	for i := range 127 {
		notes.Changes = append(notes.Changes, views.RuleChange{Rule: views.RuleRef{Path: fmt.Sprintf("techs/go/rule-%03d", i), Title: "A rule"}, Change: coderules.ChangeNew, To: v100})
	}
	c.releases["example/rules"] = views.ReleasesPage{Library: exampleRules, AllReleases: exampleReleases[:1], Releases: []views.ReleaseNotes{notes}}

	page := get(t, newSite(t, c), library+"?tab=releases").Body.String()

	assertShows(t, page, "Library release 1 publishes 127 rules.", "techs/go/rule-009 1.0.0 Show 117 more new rules Show fewer A rule techs/go/rule-010")
}

// A Major mark says what a major change means without hovering, and leads to how versions work.
func TestMajorMarksExplainThemselves(t *testing.T) {
	page := get(t, newSite(t, historyCatalog()), errorsRule+"?tab=versions").Body.String()

	if !strings.Contains(page, `href="https://code-rules.fabricahq.com/reference/rule-versions/#choose-a-version-change" title="Major change: work that complied with the previous version could fail this one." aria-label="Major change: work that complied with the previous version could fail this one. How versions work.">Major</a>`) {
		t.Error("the Major mark doesn't explain itself")
	}
}

// The lines view folds the unchanged lines between hunks, which a reader can show.
func TestLineDiffShowsHiddenLinesOnRequest(t *testing.T) {
	var old, new strings.Builder
	for i := range 20 {
		fmt.Fprintf(&old, "line %d\n", i)
		if i == 15 {
			new.WriteString("changed\n")
		} else {
			fmt.Fprintf(&new, "line %d\n", i)
		}
	}
	c := historyCatalog()
	comparison := c.ruleComparisons["example/rules/techs/go/return-errors 1.0.0...2.0.0"]
	comparison.Text = views.ComparedText{Old: old.String(), New: new.String(), OldRelease: 1, NewRelease: 3}
	c.ruleComparisons["example/rules/techs/go/return-errors 1.0.0...2.0.0"] = comparison

	page := get(t, newSite(t, c), errorsRule+"?tab=versions&from=1.0.0&to=2.0.0&view=lines").Body.String()

	assertShows(t, page, "⋯ Show Hide 12 unchanged lines 1 1 line 0", "@@ -13,7 +13,7 @@", "⋯ Show Hide 1 unchanged line 20 20 line 19")
}

// A long chain of replacements names the first replacement and the last, and counts the ones between.
func TestReplacementChainsNameTheirEnds(t *testing.T) {
	c := historyCatalog()
	comparison := c.releaseComparisons["example/rules 1...3"]
	var chain []views.RuleRef
	for i := range 20 {
		chain = append(chain, views.RuleRef{Path: fmt.Sprintf("techs/go/r%02d", i), Title: fmt.Sprintf("Rule %02d", i)})
	}
	comparison.Changes = slices.Clone(comparison.Changes)
	comparison.Changes[0].Replacements = chain
	c.releaseComparisons["example/rules 1...3"] = comparison

	page := get(t, newSite(t, c), library+"?tab=releases&from=1&to=3").Body.String()

	assertShows(t, page, "Replaced by Rule 00 , itself replaced by Rule 01 , and after 17 more, by Rule 19 .")
	if strings.Contains(visibleText(t, page), "Rule 02") {
		t.Error("the page names a replacement in the middle of the chain")
	}
}

// Diffs' fragments tell apart rule IDs that differ only in where a slash or hyphen is.
func TestDiffFragmentsAreDistinct(t *testing.T) {
	c := historyCatalog()
	comparison := c.releaseComparisons["example/rules 1...3"]
	text := views.ComparedText{Old: "a\n", New: "b\n", OldRelease: 1, NewRelease: 3}
	comparison.Changes = []views.RuleChange{
		{Rule: views.RuleRef{Path: "techs/go-a/b", Title: "One"}, Change: coderules.ChangeMinor, From: v100, To: v110, Text: text},
		{Rule: views.RuleRef{Path: "techs/go/a-b", Title: "Two"}, Change: coderules.ChangeMinor, From: v100, To: v110, Text: text},
	}
	c.releaseComparisons["example/rules 1...3"] = comparison

	page := get(t, newSite(t, c), library+"?tab=releases&from=1&to=3").Body.String()

	ids := regexp.MustCompile(`<section[^>]* id="(diff-[^"]+)"`).FindAllStringSubmatch(page, -1)
	if len(ids) != 2 || ids[0][1] == ids[1][1] {
		t.Fatalf("diff fragments are %q", ids)
	}
}
