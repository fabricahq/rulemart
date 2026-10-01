package web

import (
	"context"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/platform/database/databasetest"
	"github.com/fabricahq/rulemart/internal/platform/postgrestest"
	"github.com/fabricahq/rulemart/internal/platform/web/sitedb"
)

// A page's queries must agree with each other, so a page never mixes catalog states. An ingestion that commits
// while a page reads mustn't show in the page's later queries.
func TestReadSeesOneSnapshotWhileIngestionCommits(t *testing.T) {
	ctx := context.Background()
	_, connString := databasetest.New(t)
	postgrestest.Exec(t, connString, `INSERT INTO libraries (host, host_repository_id, owner, name, description, owner_avatar_url)
		VALUES ('github', '7', 'example', 'rules', 'Before ingestion.', '')`)
	store := NewStore(databasetest.AsWebRole(t, connString), []domain.LibraryKey{{Host: domain.GitHub, RepositoryID: "7"}})

	err := store.read(ctx, func(q *sitedb.Queries) error {
		before, err := q.ListLibraries(ctx, store.vetted)
		if err != nil {
			return err
		}
		postgrestest.Exec(t, connString, `UPDATE libraries SET description = 'After ingestion.' WHERE host_repository_id = '7'`)
		after, err := q.ListLibraries(ctx, store.vetted)
		if err != nil {
			return err
		}
		if before[0].Description != "Before ingestion." || after[0].Description != "Before ingestion." {
			t.Errorf("one read saw %q, then %q", before[0].Description, after[0].Description)
		}
		return nil
	})

	if err != nil {
		t.Fatal(err)
	}
}
