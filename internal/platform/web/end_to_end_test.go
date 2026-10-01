package web_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/render"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git/gittest"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres"
	"github.com/fabricahq/rulemart/internal/platform/database/databasetest"
)

// hostileHTML is raw HTML in the current version of a rule, which a library could publish to attack visitors.
const hostileHTML = "<script>alert(1)</script>\n\nPress <img src=x onerror=alert(2)> to <a href=\"javascript:alert(3)\">continue</a>."

// repositories describes every repository as repo, as GitHub would describe it.
type repositories struct{ repo domain.Repository }

func (r repositories) Repository(context.Context, string, string) (domain.Repository, error) {
	return r.repo, nil
}

// newIngestedSite ingests a library whose rule holds hostileHTML into a new database, and returns the pages'
// handler, reading as the web function's role, so a table the migrations don't grant it fails these tests.
func newIngestedSite(t *testing.T) http.Handler {
	t.Helper()
	db, connString := databasetest.New(t)
	lib := gittest.NewLibrary(t)
	lib.Group("techs/go", "Go")
	lib.Rule("techs/go/return-errors", "Return errors", "Wrap every returned error.\n\n"+hostileHTML)
	lib.Release(1, `formatVersion: 1
release: 1
rules: {techs/go/return-errors: 1.0.0}
changes: {techs/go/return-errors: {change: new, summaries: [Add the rule.]}}
`)
	repo := lib.Repository(7)
	ingester := app.Ingester{Repositories: repositories{repo}, Fetch: git.Fetch, Render: render.Rule, Store: postgres.New(db), Limits: domain.DefaultLimits}
	if _, err := ingester.Ingest(context.Background(), "https://github.com/"+repo.FullName()); err != nil {
		t.Fatal(err)
	}
	pages := app.Pages{Store: postgres.New(databasetest.AsWebRole(t, connString)), Vetted: []domain.LibraryKey{{Host: repo.Host, RepositoryID: repo.ID}}}
	return newSite(t, pages)
}

// Every page reads as the web function's role, from a library that ingestion stored.
func TestPagesShowAnIngestedLibrary(t *testing.T) {
	handler := newIngestedSite(t)

	for path, want := range map[string]string{
		"/":                          "example/rules · 1 rule",
		library:                      "Technologies · 1 Go techs/go 1 rule ›",
		library + "?tab=rules":       "Return errors HIGH 1.0.0 techs/go/return-errors",
		errorsRule:                   "Wrap every returned error.",
		errorsRule + "?tab=versions": "1.0.0 Latest release/1 1 Sep 2026 Add the rule.",
	} {
		resp := get(t, handler, path)
		if resp.Code != http.StatusOK {
			t.Fatalf("%s: got %d", path, resp.Code)
		}
		assertShows(t, resp.Body.String(), want)
	}
}

// A rule's raw HTML must reach visitors as text: the page shows it escaped, and runs or loads nothing from it.
func TestRulePageShowsRawHTMLAsText(t *testing.T) {
	handler := newIngestedSite(t)

	resp := get(t, handler, errorsRule)

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d", resp.Code)
	}
	assertShows(t, resp.Body.String(), "<script>alert(1)</script>", "Press <img src=x onerror=alert(2)> to")
	doc, err := html.Parse(strings.NewReader(resp.Body.String()))
	if err != nil {
		t.Fatal(err)
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, attr := range n.Attr {
				if strings.HasPrefix(attr.Key, "on") || strings.Contains(attr.Val, "javascript:") {
					t.Errorf("<%s> has %s=%q", n.Data, attr.Key, attr.Val)
				}
			}
			if n.Data == "script" && !strings.HasPrefix(attribute(n, "src"), "/_static/") {
				t.Errorf("a script that isn't Rulemart's own: src=%q", attribute(n, "src"))
			}
			if n.Data == "img" && !strings.HasPrefix(attribute(n, "src"), "https://") {
				t.Errorf("an image from the rule: src=%q", attribute(n, "src"))
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
}
