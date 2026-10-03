package render

import (
	"errors"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// unlimited is an allowance no test body comes near.
const unlimited = 1 << 40

var page = domain.MarkdownSource{
	Repository: "example/rules", File: "practices/testing/verify-retry-limits.md", Rule: "practices/testing/verify-retry-limits",
	Title: "Verify retry limits", Tag: "release/2", LatestTag: "release/5",
}

func TestRenderShowsRawHTMLAsText(t *testing.T) {
	for name, body := range map[string]string{
		"block":  "<script>alert(1)</script>\n\nText.",
		"inline": "Press <img src=x onerror=alert(1)> now.",
	} {
		t.Run(name, func(t *testing.T) {
			html, _, err := Markdown(body, page, unlimited)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(html, "<script") || strings.Contains(html, "<img") {
				t.Fatalf("raw HTML reached the page: %s", html)
			}
			if !strings.Contains(html, "&lt;") {
				t.Fatalf("raw HTML wasn't shown as text: %s", html)
			}
		})
	}
}

func TestRenderDropsDangerousLinks(t *testing.T) {
	html, _, err := Markdown("[click](javascript:alert(1))", page, unlimited)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "javascript:") {
		t.Fatalf("kept a javascript: link: %s", html)
	}
}

func TestRenderDropsALeadingHeadingThatRepeatsTheTitle(t *testing.T) {
	for name, tc := range map[string]struct {
		body, want, unwanted string
	}{
		"repeats the title": {"## Verify retry limits\n\nStop after a fixed number.", "<p>Stop", "<h2"},
		"another heading":   {"## Why\n\nStop after a fixed number.", `<h2 id="why">Why</h2>`, ""},
		"a later repeat":    {"Intro.\n\n## Verify retry limits\n", "<h2", ""},
	} {
		t.Run(name, func(t *testing.T) {
			html, _, err := Markdown(tc.body, page, unlimited)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(html, tc.want) || (tc.unwanted != "" && strings.Contains(html, tc.unwanted)) {
				t.Fatalf("got %s", html)
			}
		})
	}
}

func TestRenderPointsRelativeLinksAtGitHub(t *testing.T) {
	for name, tc := range map[string]struct{ markdown, want string }{
		"the rule's own asset, at the rule's release": {
			"[example](assets/verify-retry-limits/example.md#limits)",
			`href="https://github.com/example/rules/blob/release/2/practices/testing/assets/verify-retry-limits/example.md#limits"`,
		},
		"a library-wide file, at the latest release": {
			"[lifecycle](../../assets/retry-lifecycle.md)",
			`href="https://github.com/example/rules/blob/release/5/assets/retry-lifecycle.md"`,
		},
		"another rule, at the latest release": {
			"[backoff](check-retry-backoff.md)",
			`href="https://github.com/example/rules/blob/release/5/practices/testing/check-retry-backoff.md"`,
		},
		"a root-relative path": {
			"[license](/LICENSE)",
			`href="https://github.com/example/rules/blob/release/5/LICENSE"`,
		},
		"a path above the root, to the repository": {
			"[up](../../../../x)",
			`href="https://github.com/example/rules/tree/release/5"`,
		},
		"a path with spaces": {
			"[notes](<assets/verify-retry-limits/my notes.md>)",
			`href="https://github.com/example/rules/blob/release/2/practices/testing/assets/verify-retry-limits/my%20notes.md"`,
		},
		"an image, from GitHub's raw files": {
			"![loop](assets/verify-retry-limits/loop.png)",
			`src="https://raw.githubusercontent.com/example/rules/refs/tags/release/2/practices/testing/assets/verify-retry-limits/loop.png"`,
		},
		"an absolute URL, unchanged": {
			"[docs](https://code-rules.fabricahq.com/)",
			`href="https://code-rules.fabricahq.com/"`,
		},
		"a fragment, unchanged": {
			"[below](#validation)",
			`href="#validation"`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			html, _, err := Markdown(tc.markdown, page, unlimited)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(html, tc.want) {
				t.Fatalf("got %s, want %s", html, tc.want)
			}
		})
	}
}

func TestRenderHighlightsFencedCodeInKnownLanguages(t *testing.T) {
	html, _, err := Markdown("```go\nreturn nil // done\n```\n\n```unknown-language\n<b>\n```", page, unlimited)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<pre><code class="language-go"><span class="hl-keyword">return</span>`,
		`<span class="hl-comment">// done`,
		`<pre><code class="language-unknown-language">&lt;b&gt;`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("got %s, want %s", html, want)
		}
	}
}

// A small body can expand: every reference to one long link definition repeats its destination in the HTML,
// escaped. Rendering must stop at its allowance, rather than keep building, or escaping, the rest of the page.
func TestRenderStopsAtItsAllowanceWhileExpandingReferenceLinks(t *testing.T) {
	for name, character := range map[string]string{
		"a destination that needs no escaping": "a",
		// HTML escaping turns each & into &amp;, so every reference would allocate five times the destination.
		"a destination that escaping expands": "&",
	} {
		t.Run(name, func(t *testing.T) {
			const references = 2_000
			destination := "assets/" + strings.Repeat(character, 32<<10) // 32 KiB, repeated in every reference's link
			body := strings.Repeat("[x][d] ", references) + "\n\n[d]: " + destination + "\n"

			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			_, _, err := Markdown(body, page, 1<<20)
			runtime.ReadMemStats(&after)

			if !errors.Is(err, domain.ErrOverAllowance) {
				t.Fatalf("got error %v, want a refusal past the 1 MiB allowance", err)
			}
			// Rendering every reference would allocate at least the destination per reference: over 64 MiB here.
			if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 32<<20 {
				t.Fatalf("rendering allocated %d MiB for a 1 MiB allowance", allocated>>20)
			}
		})
	}
}

// What a render uses is its HTML, and each distinct rewritten link once, however many references share it.
func TestRenderCountsTheHTMLAndEachRewrittenLinkOnce(t *testing.T) {
	html, used, err := Markdown("[a][d] and [b][d]\n\n[d]: check-retry-backoff.md\n", page, unlimited)
	if err != nil {
		t.Fatal(err)
	}
	link := "https://github.com/example/rules/blob/release/5/practices/testing/check-retry-backoff.md"
	if want := int64(len(html) + len(link)); used != want || strings.Count(html, link) != 2 {
		t.Fatalf("used %d for %s, want %d", used, html, want)
	}
}

// The renderer names GitHub's extensions' renderers itself, so each must still render what the parser finds.
func TestRenderRendersGitHubExtensions(t *testing.T) {
	html, _, err := Markdown("| a | b |\n| - | - |\n| 1 | 2 |\n\n~~old~~\n\n- [x] done\n\nSee https://example.com.", page, unlimited)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<table>", "<td>1</td>", "<del>old</del>", `<input checked="" disabled="" type="checkbox"`, `<a href="https://example.com">`} {
		if !strings.Contains(html, want) {
			t.Errorf("got %s, want %s", html, want)
		}
	}
}

// Some code makes chroma's lexers take minutes, and rule files come from repositories Rulemart doesn't control, so
// highlighting has a time budget per rule. Past it, the rest of the code is shown escaped, without highlighting.
func TestRenderStopsHighlightingCodeThatTakesTooLong(t *testing.T) {
	slow := strings.Repeat("a ", 100_000)
	body := "```java\n" + slow + "\n```\n\n```go\nreturn nil\n```\n"
	start := time.Now()

	html, _, err := Markdown(body, page, 64<<20)

	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("rendering took %s", elapsed)
	}
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `<pre><code class="language-java">`) || !strings.Contains(html, slow[:2000]) ||
		!strings.Contains(html, `<pre><code class="language-go">return nil`) {
		t.Fatalf("the code isn't all shown, or the second block is still highlighted: %.300s", html)
	}
}

// Highlighting writes the code chroma tokenised, and escapes any rest of it, so with Windows line endings too, the
// page must show every character once.
func TestRenderShowsHighlightedCodeWithWindowsLineEndingsOnce(t *testing.T) {
	html, _, err := Markdown("```go\r\nx := 1\r\nreturn x\r\n```\r\n", page, unlimited)
	if err != nil {
		t.Fatal(err)
	}
	text := regexp.MustCompile(`<[^>]+>`).ReplaceAllString(html, "")
	if !strings.Contains(text, "x := 1\nreturn x") || strings.Count(text, "return") != 1 {
		t.Fatalf("got %q", html)
	}
}

// withAssets is page with the assets Rulemart shows: an own diagram it keeps, an own example too large to keep, and a
// shared glossary.
var withAssets = func() domain.MarkdownSource {
	p := page
	p.Assets = map[string]domain.AssetAddress{
		"practices/testing/assets/verify-retry-limits/loop.svg": {
			Page:  "/example/rules/practices/testing/verify-retry-limits/assets/loop.svg",
			Image: "/example/rules/practices/testing/verify-retry-limits/assets/loop.svg?raw=1",
		},
		"practices/testing/assets/verify-retry-limits/big.png": {Page: "/example/rules/practices/testing/verify-retry-limits/assets/big.png"},
		"assets/glossary.md": {Page: "/example/rules/assets/glossary.md"},
	}
	return p
}()

// A link to an asset Rulemart shows leads to its page, keeping a fragment; an image of one it keeps loads from
// Rulemart, and one it doesn't from GitHub; other relative links still lead to GitHub.
func TestRenderPointsLinksToAssetsAtTheirPages(t *testing.T) {
	for name, tc := range map[string]struct{ markdown, want string }{
		"an own asset": {
			"[loop](assets/verify-retry-limits/loop.svg)",
			`href="/example/rules/practices/testing/verify-retry-limits/assets/loop.svg"`,
		},
		"a shared asset, with its fragment": {
			"[glossary](../../assets/glossary.md#regression-test)",
			`href="/example/rules/assets/glossary.md#regression-test"`,
		},
		"a shared asset, without the query a link wrote": {
			"[glossary](../../assets/glossary.md?plain=1)",
			`href="/example/rules/assets/glossary.md"`,
		},
		"an image Rulemart keeps": {
			"![loop](assets/verify-retry-limits/loop.svg)",
			`src="/example/rules/practices/testing/verify-retry-limits/assets/loop.svg?raw=1"`,
		},
		"an image too large to keep, from GitHub": {
			"![big](assets/verify-retry-limits/big.png)",
			`src="https://raw.githubusercontent.com/example/rules/refs/tags/release/2/practices/testing/assets/verify-retry-limits/big.png"`,
		},
		"a file that isn't an asset, on GitHub": {
			"[missing](../../assets/missing.md)",
			`href="https://github.com/example/rules/blob/release/5/assets/missing.md"`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			html, _, err := Markdown(tc.markdown, withAssets, unlimited)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(html, tc.want) {
				t.Fatalf("got %s, want %s", html, tc.want)
			}
		})
	}
}

// A Markdown asset's links resolve against its own directory, and lead to GitHub at the release that holds each file:
// the rule's for its own assets, and the latest for others.
func TestRenderResolvesAMarkdownAssetsLinksAgainstItsOwnFile(t *testing.T) {
	asset := withAssets
	asset.File, asset.Title = "practices/testing/assets/verify-retry-limits/why.md", ""
	html, _, err := Markdown("# Why\n\nSee the [glossary](../../../../assets/glossary.md), [the loop](loop.svg), "+
		"and [the rule](../../verify-retry-limits.md).", asset, unlimited)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<h1 id="why">Why</h1>`,
		`href="/example/rules/assets/glossary.md"`,
		`href="/example/rules/practices/testing/verify-retry-limits/assets/loop.svg"`,
		`href="https://github.com/example/rules/blob/release/2/practices/testing/verify-retry-limits.md"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("got %s, want %s", html, want)
		}
	}
}

// A text file shows as code, highlighted in a language chroma knows by its name, and escaped otherwise.
func TestCodeHighlightsTextFilesByTheirNames(t *testing.T) {
	for name, tc := range map[string]struct{ file, text, want string }{
		"Go":           {"example.go", "return nil", `<pre><code class="language-go"><span class="hl-keyword">return</span>`},
		"JSON":         {"cases.json", `{"a": 1}`, `<pre><code class="language-json">`},
		"unknown":      {"notes.unknown", "<b>", `<pre><code class="language-unknown">&lt;b&gt;</code></pre>`},
		"no extension": {"Makefile.d/x", "<b>", `<pre><code>&lt;b&gt;</code></pre>`},
	} {
		t.Run(name, func(t *testing.T) {
			html, used, err := Code(tc.text, tc.file, unlimited)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(html, tc.want) || used != int64(len(html)) {
				t.Fatalf("got %s, used %d", html, used)
			}
		})
	}
	if _, _, err := Code(strings.Repeat("x", 100), "big.txt", 50); !errors.Is(err, domain.ErrOverAllowance) {
		t.Fatalf("got %v, want a refusal past the allowance", err)
	}
}

// Links finds every link and image a page would show, including one that names a definition, and none in raw HTML or
// code, which pages show as text.
func TestLinksFindsTheLinksAndImagesAPageShows(t *testing.T) {
	got, _, err := Links("See [a](a.md) and ![b](b.png), [c][d], `[e](e.md)`, and <a href=\"f.md\">f</a>.\n\n[d]: c.md\n", unlimited)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a.md", "b.png", "c.md"}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// Links finds each destination once, in the order it first appears, however many links and references name it, and
// uses its bytes once; it stops at its allowance rather than hold more.
func TestLinksFindsEachDestinationOnceWithinItsAllowance(t *testing.T) {
	body := "[a][d], [b](x.md), [c][d], ![e](x.md)\n\n[d]: retry.md\n"
	const used = int64(len("retry.md") + len("x.md"))

	got, spent, err := Links(body, used)

	if want := []string{"retry.md", "x.md"}; err != nil || !slices.Equal(got, want) || spent != used {
		t.Fatalf("got %q using %d, %v; want %q using %d", got, spent, err, want, used)
	}
	if _, _, err := Links(body, used-1); !errors.Is(err, domain.ErrOverAllowance) {
		t.Fatalf("a byte short of the allowance: got %v, want a refusal", err)
	}
}

// Assembly finds a rule's links to its shared assets before it renders the rule, so finding them must not expand as
// rendering could: many references to one long link definition hold its destination once, within the content budget.
func TestAssemblyFindsLinksWithoutExpandingReferences(t *testing.T) {
	const references = 2_000
	destination := "../../assets/" + strings.Repeat("a", 32<<10) // 32 KiB, which every reference names
	body := strings.Repeat("[x][d] ", references) + "\n\n[d]: " + destination + "\n"
	release := oneRule(t, body)
	limits := domain.ContentLimits{FileBytes: 1 << 20, ContentBytes: 1 << 20, AssetBytes: 1 << 10, RuleAssetBytes: 1 << 10, Assets: 10}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, err := domain.Assemble(assembled, []domain.ReleaseSnapshot{release}, limits, Renderer{})
	runtime.ReadMemStats(&after)

	if want := "more than 1048576 bytes of content"; err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("got error %v, want the budget's refusal, %q", err, want)
	}
	// Finding every reference's link would allocate at least the destination per reference: over 64 MiB here.
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 32<<20 {
		t.Fatalf("assembly allocated %d MiB for a 1 MiB budget", allocated>>20)
	}
}

var assembled = domain.Repository{Host: domain.GitHub, ID: "42", Owner: "example", Name: "rules"}

// oneRule returns release/1 of a library whose one rule, techs/go/return-errors, has body.
func oneRule(t *testing.T, body string) domain.ReleaseSnapshot {
	t.Helper()
	const record = `formatVersion: 1
release: 1
rules: {techs/go/return-errors: 1.0.0}
changes: {techs/go/return-errors: {change: new, summaries: [Add the rule.]}}
`
	parsed, err := coderules.ParseReleaseRecord([]byte(record), "release/1")
	if err != nil {
		t.Fatal(err)
	}
	return domain.ReleaseSnapshot{
		Number: 1, Tag: "release/1", CommitID: strings.Repeat("1", 40), Record: parsed,
		Files: memoryFiles{
			"rule-library.yaml":    "formatVersion: 1\n",
			"techs/go/_group.yaml": "name: Go\ndescription: Go rules.\nwhenToRead: When the work involves Go.\n",
			"techs/go/return-errors.md": "---\ntitle: Return errors\nwhenToRead: When returning errors.\nimpact: HIGH\n" +
				"impactDescription: Prevents lost errors.\n---\n\n## Return errors\n\n" + body,
		},
	}
}

// memoryFiles is a release's files, by path, held in memory.
type memoryFiles map[string]string

func (f memoryFiles) Open(path string) (domain.File, error) {
	content, ok := f[path]
	if !ok {
		return nil, domain.ErrFileMissing
	}
	return memoryFile(content), nil
}

func (f memoryFiles) List(dir string, max int) ([]string, error) {
	var paths []string
	for path := range f {
		if strings.HasPrefix(path, dir) {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	return paths[:min(len(paths), max+1)], nil
}

type memoryFile string

func (f memoryFile) Size() int64           { return int64(len(f)) }
func (f memoryFile) Read() ([]byte, error) { return []byte(f), nil }
