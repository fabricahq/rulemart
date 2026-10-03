package web_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"

	shipped "github.com/fabricahq/rulemart/catalog"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/render"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git/gittest"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres"
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
	db, connString := databasetest.New(t)
	repo := lib.Repository(7)
	ingester := app.Ingester{Repositories: repositories{repo}, Fetch: git.Fetch, Render: render.Rule, Store: postgres.New(db), Limits: domain.DefaultLimits}
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
	if group.Title != "Go" || len(group.Rules) != 2 || group.Href != "/example/rules/techs/go" {
		t.Errorf("got the group %+v, want Go with its two rules", group)
	}
	commands := "code-rules project add library example \\\n  --repository https://github.com/example/rules.git \\\n" +
		"  --groups techs/go \\\n  --rules practices/testing/verify-retries\n\ncode-rules project sync\n\n" +
		"# Then make sure AGENTS.md tells agents to read .code-rules/generated/RULES.md"
	if !strings.HasSuffix(answer.Commands, commands) {
		t.Errorf("the commands end\n%s\nwant\n%s", answer.Commands, commands)
	}
	if !strings.Contains(answer.Prompt, "- The whole Go group (techs/go), including rules the library adds to it later") {
		t.Errorf("the prompt doesn't name the group:\n%s", answer.Prompt)
	}
}
