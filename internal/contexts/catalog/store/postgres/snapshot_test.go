package postgres

import (
	"context"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres/catalogdb"
	"github.com/fabricahq/rulemart/internal/platform/database/databasetest"
	"github.com/fabricahq/rulemart/internal/platform/postgrestest"
)

// A page's queries must agree with each other, so a page never mixes catalog states. An ingestion that commits
// while a page reads mustn't show in the page's later queries.
func TestReadSeesOneSnapshotWhileIngestionCommits(t *testing.T) {
	ctx := context.Background()
	_, connString := databasetest.New(t)
	postgrestest.Exec(t, connString, `INSERT INTO libraries (host, host_repository_id, owner, name, description, owner_avatar_url)
		VALUES ('github', '7', 'example', 'rules', 'Before ingestion.', '')`)
	s := New(databasetest.AsWebRole(t, connString))
	vetted := vettedKeys([]domain.LibraryKey{{Host: domain.GitHub, RepositoryID: "7"}})

	err := s.read(ctx, func(q *catalogdb.Queries) error {
		before, err := q.ListLibraries(ctx, vetted)
		if err != nil {
			return err
		}
		postgrestest.Exec(t, connString, `UPDATE libraries SET description = 'After ingestion.' WHERE host_repository_id = '7'`)
		after, err := q.ListLibraries(ctx, vetted)
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
