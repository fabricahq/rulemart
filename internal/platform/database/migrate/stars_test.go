package migrate

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/fabricahq/rulemart/internal/platform/postgrestest"
)

// No published release created the library stars 00013 drops, so a database that holds one came from an unreleased
// build. Migrating it stops before losing the star, and goes on once someone has removed it.
func TestMovingStarsToRulesRefusesADatabaseWhoseLibraryStarsHoldRows(t *testing.T) {
	ctx := context.Background()
	connString := postgrestest.New(t)
	if _, err := up(ctx, connString, migrationsThrough(t, 12)); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	insertCurrentRule(t, ctx, conn, "Return errors", "When handling failures.", "---\ntitle: Return errors\n---\nWrap errors.")
	exec := func(sql string) {
		t.Helper()
		if _, err := conn.Exec(ctx, sql); err != nil {
			t.Fatalf("run %q: %v", sql, err)
		}
	}
	exec(`INSERT INTO accounts (github_user_id, github_login, avatar_url) VALUES (1, 'octocat', '')`)
	exec(`INSERT INTO stars (account_id, library_id) SELECT a.id, l.id FROM accounts a, libraries l`)

	result, err := Up(ctx, connString)

	if err == nil || !strings.Contains(err.Error(), "stars holds rows") {
		t.Fatalf("got %v, want a refusal saying stars holds rows", err)
	}
	if len(result.Applied) != 0 {
		t.Errorf("reported %v applied, want none", appliedVersions(result))
	}
	var version, stars int64
	postgrestest.QueryRow(t, connString, "SELECT max(version_id) FROM goose_db_version", &version)
	postgrestest.QueryRow(t, connString, "SELECT count(*) FROM stars", &stars)
	if version != 12 || stars != 1 {
		t.Fatalf("the database is at version %d with %d stars, want 12 with its star", version, stars)
	}

	exec(`DELETE FROM stars`)
	if _, err := Up(ctx, connString); err != nil {
		t.Fatalf("migrating once the stars are gone: %v", err)
	}
}

// A star stays on its rule, but ingestion still deletes a rule that drops out of its library's history, as the worker,
// which can't touch stars itself; the rule's stars go with it.
func TestTheWorkerDeletesAStarredRuleAndItsStarsGoWithIt(t *testing.T) {
	ctx := context.Background()
	connString := postgrestest.New(t)
	if _, err := Up(ctx, connString); err != nil {
		t.Fatal(err)
	}
	owner, err := pgx.Connect(ctx, connString)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(ctx)
	insertCurrentRule(t, ctx, owner, "Return errors", "When handling failures.", "---\ntitle: Return errors\n---\nWrap errors.")
	postgrestest.Exec(t, postgrestest.AsWebRole(t, connString),
		`INSERT INTO accounts (github_user_id, github_login, avatar_url) VALUES (1, 'octocat', '')`)
	postgrestest.Exec(t, postgrestest.AsWebRole(t, connString),
		`INSERT INTO rule_stars (account_id, rule_id) SELECT a.id, r.id FROM accounts a, rules r`)

	postgrestest.Exec(t, postgrestest.AsWorkerRole(t, connString), `DELETE FROM rule_versions`)
	postgrestest.Exec(t, postgrestest.AsWorkerRole(t, connString), `DELETE FROM rules`)

	var stars int
	postgrestest.QueryRow(t, connString, "SELECT count(*) FROM rule_stars", &stars)
	if stars != 0 {
		t.Fatalf("%d stars outlived their rule", stars)
	}
}
