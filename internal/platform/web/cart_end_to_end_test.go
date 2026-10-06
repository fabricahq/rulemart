package web_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	shipped "github.com/fabricahq/rulemart/catalog"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/render"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git/gittest"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/platform/database/databasetest"
	"github.com/fabricahq/rulemart/internal/platform/web"
)

// A browser's cart checks out against an ingested library, read as the web function's role, so a missing grant fails
// this test: a whole group, a rule of another group, a rule the library doesn't have, a library Rulemart doesn't have,
// and a key that names nothing. The answer says what became of each, and the commands import the group and the rule.
func TestABrowsersCartChecksOutAgainstTheCatalog(t *testing.T) {
	lib := gittest.NewLibrary(t)
	lib.Group("techs/go", "Go")
	lib.Group("practices/testing", "Testing")
	lib.Rule("techs/go/return-errors", "Return errors", "Wrap every returned error.")
	lib.Rule("techs/go/close-bodies", "Close bodies", "Close every response body.")
	lib.Rule("practices/testing/verify-retries", "Verify retries", "Test each retry.")
	lib.Release(1, `formatVersion: 1
release: 1
rules: {techs/go/return-errors: 1.0.0, techs/go/close-bodies: 1.0.0, practices/testing/verify-retries: 1.0.0}
changes:
  techs/go/return-errors: {change: new, summaries: [Add the rule.]}
  techs/go/close-bodies: {change: new, summaries: [Add the rule.]}
  practices/testing/verify-retries: {change: new, summaries: [Add the rule.]}
`)
	handler := newCheckoutSite(t, lib)

	resp := postCheckout(t, handler, `{"cart":["group::Example/Rules::techs/go","example/rules::practices/testing/verify-retries",
		"example/rules::techs/go/never-was","nowhere/rules::techs/go/x","example/rules::techs/go/return-errors","?"]}`, nil)

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d: %s", resp.Code, resp.Body)
	}
	answer := decode(t, resp)
	var states []string
	for _, l := range answer.Libraries {
		for _, it := range l.Items {
			states = append(states, l.FullName+" "+it.Kind+" "+it.ID+" "+it.State)
		}
	}
	want := []string{
		"example/rules group techs/go ready",
		"example/rules rule practices/testing/verify-retries ready",
		"example/rules rule techs/go/never-was missing",
		"example/rules rule techs/go/return-errors ready",
		"nowhere/rules rule techs/go/x gone",
	}
	if !slices.Equal(states, want) || !slices.Equal(answer.Unknown, []string{"?"}) {
		t.Errorf("got %q, unknown %q, want\n%q", states, answer.Unknown, want)
	}
	group := answer.Libraries[0].Items[0]
	if group.Title != "Go" || len(group.Rules) != 2 || group.RuleCount != 2 || group.Href != "/example/rules/techs/go" {
		t.Errorf("got the group %+v, want Go with its two rules", group)
	}
	commands := "code-rules project add library example \\\n  --repository https://github.com/example/rules.git \\\n" +
		"  --groups techs/go \\\n  --rules practices/testing/verify-retries\n\ncode-rules project sync\n\n" +
		"# Then make sure AGENTS.md tells agents to read .code-rules/generated/RULES.md"
	if got := oneStep(t, answer); !strings.HasSuffix(got, commands) {
		t.Errorf("the commands end\n%s\nwant\n%s", got, commands)
	}
	if !strings.Contains(answer.Prompt, "- The whole Go group (techs/go), including rules the library adds to it later") {
		t.Errorf("the prompt doesn't name the group:\n%s", answer.Prompt)
	}
}

// newCheckoutSite ingests lib, as GitHub repository 7, which Rulemart vets, and returns the site, reading the catalog as
// the web function's role.
func newCheckoutSite(t *testing.T, lib *gittest.Library) http.Handler {
	t.Helper()
	db, connString := databasetest.New(t)
	repo := lib.Repository(7)
	ingester := app.Ingester{Repositories: repositories{repo}, Fetch: git.Fetch, Renderer: render.Renderer{}, Store: postgres.New(db), Limits: domain.DefaultLimits}
	if _, err := ingester.Ingest(context.Background(), "https://github.com/"+repo.FullName()); err != nil {
		t.Fatal(err)
	}
	groups, err := shipped.CanonicalGroups()
	if err != nil {
		t.Fatal(err)
	}
	vetted := []domain.LibraryKey{{Host: repo.Host, RepositoryID: repo.ID}}
	webStore := postgres.New(databasetest.AsWebRole(t, connString))
	handler, err := web.New(app.Pages{Store: webStore, Vetted: vetted, Groups: groups}, web.Options{
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Carts: app.Carts{Store: webStore, Vetted: vetted, Groups: groups},
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

// A Lambda function's response holds at most 6 MB, so a checkout must fit however long the titles a library writes.
const lambdaResponseBytes = 6_000_000

// A library may write a title of any length, up to its whole rule file, so a cart of a whole group of many rules with
// very long titles, and of each of those rules, would answer with more than a Lambda function's response holds. The
// group lists its first rules by their titles, with how many it brings, and every title is cut short.
func TestACartOfRulesWithVeryLongTitlesChecksOutWithinAResponse(t *testing.T) {
	const rules = 30
	lib := gittest.NewLibrary(t)
	lib.Group("techs/go", "Go")
	var record, changes strings.Builder
	record.WriteString("formatVersion: 1\nrelease: 1\nrules:\n")
	var cart []string
	for i := range rules {
		id := fmt.Sprintf("techs/go/rule-%02d", i)
		title := fmt.Sprintf("Rule %02d ", i) + strings.Repeat("and a title that goes on ", 8_400)
		lib.Write(id+".md", "---\ntitle: "+title+"\nwhenToRead: When writing Go.\nimpact: HIGH\nimpactDescription: Prevents mistakes.\n---\n\nWrite Go.\n")
		fmt.Fprintf(&record, "  %s: 1.0.0\n", id)
		fmt.Fprintf(&changes, "  %s: {change: new, summaries: [Add the rule.]}\n", id)
		cart = append(cart, `"example/rules::`+id+`"`)
	}
	record.WriteString("changes:\n")
	record.WriteString(changes.String())
	lib.Release(1, record.String())
	handler := newCheckoutSite(t, lib)

	resp := postCheckout(t, handler, `{"cart":["group::example/rules::techs/go",`+strings.Join(cart, ",")+`]}`, nil)

	if resp.Code != http.StatusOK {
		t.Fatalf("got %d: %.500s", resp.Code, resp.Body)
	}
	if n := resp.Body.Len(); n > lambdaResponseBytes {
		t.Fatalf("answered %d bytes, more than a Lambda function's response holds", n)
	}
	answer := decode(t, resp)
	items := answer.Libraries[0].Items
	group := items[0]
	var listed []string
	for _, r := range group.Rules {
		listed = append(listed, r.Title[:len("Rule 00")])
	}
	if want := []string{"Rule 00", "Rule 01", "Rule 02", "Rule 03", "Rule 04"}; group.RuleCount != rules || !slices.Equal(listed, want) {
		t.Errorf("the group lists %q of %d rules, want %q of %d", listed, group.RuleCount, want, rules)
	}
	for _, r := range group.Rules {
		assertShortTitle(t, r.Title)
	}
	for _, it := range items[1:] {
		assertShortTitle(t, it.Title)
	}
	if commands := oneStep(t, answer); len(items) != rules+1 || !strings.Contains(commands, "--groups techs/go") {
		t.Errorf("got %d items and the commands\n%s\nwant the group and its %d rules, importing the group", len(items), commands, rules)
	}
}

// assertShortTitle fails t unless title is cut short, as the cart shows it, with an ellipsis.
func assertShortTitle(t *testing.T, title string) {
	t.Helper()
	if n := utf8.RuneCountInString(title); n > views.MaxCartTitleRunes || !strings.HasSuffix(title, "…") {
		t.Errorf("got a title of %d characters ending %q, want at most %d ending with an ellipsis", n, title[max(len(title)-10, 0):], views.MaxCartTitleRunes)
	}
}
