package ingest

import (
	"runtime"
	"strings"
	"testing"
)

// unlimited is a content budget no test body comes near.
func unlimited() *contentBudget { return &contentBudget{limit: 1 << 40} }

var page = rulePage{
	repository: "example/rules", path: "practices/testing/verify-retry-limits.md", title: "Verify retry limits",
	tag: "release/2", latestTag: "release/5",
}

func TestRenderRuleShowsRawHTMLAsText(t *testing.T) {
	for name, body := range map[string]string{
		"block":  "<script>alert(1)</script>\n\nText.",
		"inline": "Press <img src=x onerror=alert(1)> now.",
	} {
		t.Run(name, func(t *testing.T) {
			html, err := renderRule(body, page, unlimited())
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

func TestRenderRuleDropsDangerousLinks(t *testing.T) {
	html, err := renderRule("[click](javascript:alert(1))", page, unlimited())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "javascript:") {
		t.Fatalf("kept a javascript: link: %s", html)
	}
}

func TestRenderRuleDropsALeadingHeadingThatRepeatsTheTitle(t *testing.T) {
	for name, tc := range map[string]struct {
		body, want, unwanted string
	}{
		"repeats the title": {"## Verify retry limits\n\nStop after a fixed number.", "<p>Stop", "<h2"},
		"another heading":   {"## Why\n\nStop after a fixed number.", `<h2 id="why">Why</h2>`, ""},
		"a later repeat":    {"Intro.\n\n## Verify retry limits\n", "<h2", ""},
	} {
		t.Run(name, func(t *testing.T) {
			html, err := renderRule(tc.body, page, unlimited())
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(html, tc.want) || (tc.unwanted != "" && strings.Contains(html, tc.unwanted)) {
				t.Fatalf("got %s", html)
			}
		})
	}
}

func TestRenderRulePointsRelativeLinksAtGitHub(t *testing.T) {
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
			html, err := renderRule(tc.markdown, page, unlimited())
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(html, tc.want) {
				t.Fatalf("got %s, want %s", html, tc.want)
			}
		})
	}
}

func TestRenderRuleHighlightsFencedCodeInKnownLanguages(t *testing.T) {
	html, err := renderRule("```go\nreturn nil // done\n```\n\n```unknown-language\n<b>\n```", page, unlimited())
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

// A small body can expand: every reference to one long link definition repeats its destination in the HTML, and in
// the rewritten link. Rendering must stop at the budget rather than build the whole page and then measure it.
func TestRenderRuleStopsAtTheBudgetWhileExpandingReferenceLinks(t *testing.T) {
	const references = 2_000
	destination := "assets/" + strings.Repeat("a", 32<<10) // 32 KiB, repeated in every reference's link
	body := strings.Repeat("[x][d] ", references) + "\n\n[d]: " + destination + "\n"
	budget := &contentBudget{limit: 1 << 20}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, err := renderRule(body, page, budget)
	runtime.ReadMemStats(&after)

	if err == nil || !strings.Contains(err.Error(), "more than 1048576 bytes of Markdown and HTML") {
		t.Fatalf("got error %v, want a refusal past the 1 MiB budget", err)
	}
	// Building the whole page would allocate the destination about twice per reference: over 128 MiB here.
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 32<<20 {
		t.Fatalf("rendering allocated %d MiB for a 1 MiB budget", allocated>>20)
	}
}
