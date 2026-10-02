package postgres_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres"
	"github.com/fabricahq/rulemart/internal/platform/database/databasetest"
)

// Until this release is deployed, the one before it keeps ingesting, storing reading guidance without its HTML. The
// pages show HTML only while it was rendered from the guidance a version holds, so they never show an old one, and
// the checkpoint asks for the library to be ingested again until it has.
func TestReadingGuidanceHTMLShowsOnlyWhileItWasRenderedFromTheGuidance(t *testing.T) {
	ctx := context.Background()
	s, connString := newStore(t)
	lib := withTags("https://github.com/example/rules.git", "a1", "b2", "c3")
	replace(t, s, lib)
	reader := postgres.New(databasetest.AsWebRole(t, connString))
	key := domain.LibraryKey{Host: domain.GitHub, RepositoryID: "7"}
	read := func() (whenToRead, html string, unrendered bool) {
		t.Helper()
		page, err := reader.RulePage(ctx, vetted, "example", "rules", "techs/go/return-errors")
		if err != nil {
			t.Fatal(err)
		}
		results, err := reader.Search(ctx, vetted, nil, domain.ParseSearchQuery("return errors"), 1, 0)
		if err != nil || len(results.Results) != 1 || results.Results[0].WhenToReadHTML != page.Rule.WhenToReadHTML {
			t.Fatalf("search found %+v, %v; want the rule with the page's guidance", results.Results, err)
		}
		checkpoint, _, err := s.Checkpoint(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		return page.Rule.WhenToRead, page.Rule.WhenToReadHTML, checkpoint.Unrendered
	}

	if text, html, unrendered := read(); text != "When changing Return errors." ||
		html != "<p>When changing <code>Return errors</code>.</p>\n" || unrendered {
		t.Fatalf("after ingestion: %q, %q, unrendered %v", text, html, unrendered)
	}

	// The release before this one ingests a library release that changes the guidance, as it would.
	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `UPDATE rule_versions SET when_to_read = 'When failing.' WHERE html IS NOT NULL`); err != nil {
		t.Fatal(err)
	}
	if text, html, unrendered := read(); text != "When failing." || html != "" || !unrendered {
		t.Fatalf("after an older release's ingestion: %q, %q, unrendered %v", text, html, unrendered)
	}

	replace(t, s, lib)
	if _, html, unrendered := read(); html == "" || unrendered {
		t.Fatalf("after ingesting again: %q, unrendered %v", html, unrendered)
	}
}
