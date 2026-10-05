package web

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
)

// Each content page's sections keep the IDs other pages and readers link to.
func TestContentPagesKeepTheirSectionIDs(t *testing.T) {
	pages, err := loadContentPages(contentFiles, newContentValues(true, true))
	if err != nil {
		t.Fatal(err)
	}
	for name, test := range map[string]struct {
		page contentPage
		ids  []string
	}{
		"vetting": {pages.vetting, []string{"vetting", "unvetted", "get-vetted", "report"}},
		"privacy": {pages.privacy, []string{"browsing", "cart", "account", "github", "adds", "cookies", "logs", "analytics", "others", "delete"}},
	} {
		var body strings.Builder
		if err := test.page.body.Render(context.Background(), &body); err != nil {
			t.Fatal(err)
		}
		for _, id := range test.ids {
			if !strings.Contains(body.String(), ` id="`+id+`"`) {
				t.Errorf("%s has no section with ID %s", name, id)
			}
		}
	}
}

// A content page that names a value the pages don't have, in any branch, or whose template or front matter is
// broken, stops the server from starting.
func TestContentPagesThatCannotRenderFailTheStart(t *testing.T) {
	const front = "---\ntitle: T\nheading: H\ndescription: D\neyebrow: E\n---\n"
	for problem, page := range map[string]string{
		"an unknown value":                     front + "<p>{{.Missing}}</p>",
		"an unknown value in an unused branch": front + "<p>{{if .CanList}}{{.Missing}}{{end}}</p>",
		"an unknown value in the lede":         strings.Replace(front, "eyebrow: E\n", "eyebrow: E\nlede: \"{{.Missing}}\"\n", 1) + "<p>Body.</p>",
		"an unclosed action":                   front + "<p>{{if .CanList}}Listing.</p>",
		"no front matter":                      "<p>Body.</p>",
		"an unknown front matter field":        strings.Replace(front, "eyebrow: E\n", "eyebrow: E\nsubtitle: S\n", 1) + "<p>Body.</p>",
		"no heading":                           strings.Replace(front, "heading: H\n", "", 1) + "<p>Body.</p>",
	} {
		files := fstest.MapFS{}
		for _, name := range []string{"about", "vetting", "privacy"} {
			files["content/generated/"+name+".html"] = &fstest.MapFile{Data: []byte(front + "<p>Fine.</p>")}
		}
		files["content/generated/vetting.html"] = &fstest.MapFile{Data: []byte(page)}

		if _, err := loadContentPages(files, newContentValues(false, false)); err == nil {
			t.Errorf("a page with %s loaded", problem)
		}
	}
}
