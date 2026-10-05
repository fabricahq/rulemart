package main

import (
	"strings"
	"testing"
)

// A page's template actions come out of Markdown as written, in links' addresses, raw HTML, and text, and one alone
// in a paragraph comes out without it, so it can choose between paragraphs.
func TestRenderKeepsTemplateActionsAsWritten(t *testing.T) {
	page := "---\ntitle: T\n---\n\n## Step {#step}\n\nRead [the guide]({{.GuideURL}}), or <a href=\"{{.Href}}\" rel=\"nofollow\">this</a>, " +
		"\"{{.Warning}}\".\n\n{{if .On}}\n\nOn.\n\n{{else}}\n\nOff.\n\n{{end}}\n"

	got, err := render([]byte(page), "page.md")

	if err != nil {
		t.Fatal(err)
	}
	want := "---\ntitle: T\n---\n<!-- Generated from page.md by make generate. Edit page.md instead. -->\n" +
		"<h2 id=\"step\">Step</h2>\n" +
		"<p>Read <a href=\"{{.GuideURL}}\">the guide</a>, or <a href=\"{{.Href}}\" rel=\"nofollow\">this</a>, &quot;{{.Warning}}&quot;.</p>\n" +
		"{{if .On}}\n<p>On.</p>\n{{else}}\n<p>Off.</p>\n{{end}}\n"
	if string(got) != want {
		t.Errorf("rendered\n%s\nwant\n%s", got, want)
	}
}

// An action Markdown drops fails the render rather than vanish from the page.
func TestRenderRefusesAnActionMarkdownWouldNotKeep(t *testing.T) {
	_, err := render([]byte("---\ntitle: T\n---\n\nSee the guide.\n\n[guide]: {{.GuideURL}}\n"), "page.md")

	if err == nil || !strings.Contains(err.Error(), "{{.GuideURL}}") {
		t.Errorf("rendered with error %v", err)
	}
}

// A page without front matter, or whose front matter never closes, fails the render.
func TestRenderRefusesAPageWithoutFrontMatter(t *testing.T) {
	for _, page := range []string{"# Title\n", "---\ntitle: T\n\nBody.\n"} {
		if _, err := render([]byte(page), "page.md"); err == nil {
			t.Errorf("rendered %q", page)
		}
	}
}
