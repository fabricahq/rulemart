package render

import (
	"errors"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
)

// unlimited is an allowance no test body comes near.
const unlimited = 1 << 40

var page = domain.RulePage{
	Repository: "example/rules", Path: "practices/testing/verify-retry-limits.md", Title: "Verify retry limits",
	Tag: "release/2", LatestTag: "release/5",
}

func TestRenderShowsRawHTMLAsText(t *testing.T) {
	for name, body := range map[string]string{
		"block":  "<script>alert(1)</script>\n\nText.",
		"inline": "Press <img src=x onerror=alert(1)> now.",
	} {
		t.Run(name, func(t *testing.T) {
			html, _, err := Rule(body, page, unlimited)
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
	html, _, err := Rule("[click](javascript:alert(1))", page, unlimited)
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
			html, _, err := Rule(tc.body, page, unlimited)
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
			html, _, err := Rule(tc.markdown, page, unlimited)
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
	html, _, err := Rule("```go\nreturn nil // done\n```\n\n```unknown-language\n<b>\n```", page, unlimited)
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
			_, _, err := Rule(body, page, 1<<20)
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
	html, used, err := Rule("[a][d] and [b][d]\n\n[d]: check-retry-backoff.md\n", page, unlimited)
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
	html, _, err := Rule("| a | b |\n| - | - |\n| 1 | 2 |\n\n~~old~~\n\n- [x] done\n\nSee https://example.com.", page, unlimited)
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

	html, _, err := Rule(body, page, 64<<20)

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
	html, _, err := Rule("```go\r\nx := 1\r\nreturn x\r\n```\r\n", page, unlimited)
	if err != nil {
		t.Fatal(err)
	}
	text := regexp.MustCompile(`<[^>]+>`).ReplaceAllString(html, "")
	if !strings.Contains(text, "x := 1\nreturn x") || strings.Count(text, "return") != 1 {
		t.Fatalf("got %q", html)
	}
}
