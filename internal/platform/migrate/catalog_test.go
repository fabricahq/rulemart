package migrate

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fabricahq/rulemart/internal/platform/postgrestest"
)

// A rule, and each of its versions, belong to one library, so a reference to another library's group or release
// must fail even though the referenced row exists.
func TestCatalogRefusesReferencesAcrossLibraries(t *testing.T) {
	ctx := context.Background()
	connString := postgrestest.New(t)
	if err := Up(ctx, connString); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var a, b, aGroup, bGroup, aRelease, bRelease, aRule int64
	scan := func(sql string, dest *int64, args ...any) {
		t.Helper()
		if err := conn.QueryRow(ctx, sql, args...).Scan(dest); err != nil {
			t.Fatalf("run %q: %v", sql, err)
		}
	}
	insertLibrary := `INSERT INTO libraries (host, host_repository_id, owner, name, description, owner_avatar_url)
		VALUES ('github', $1, $2, 'rules', '', '') RETURNING id`
	scan(insertLibrary, &a, "1", "a")
	scan(insertLibrary, &b, "2", "b")
	insertGroup := `INSERT INTO library_groups (library_id, path, name, description, when_to_read)
		VALUES ($1, 'techs/go', 'Go', 'Go rules.', 'When writing Go.') RETURNING id`
	scan(insertGroup, &aGroup, a)
	scan(insertGroup, &bGroup, b)
	insertRelease := `INSERT INTO library_releases (library_id, number, commit_id, tagged_at, updates_shared_files)
		VALUES ($1, 1, 'commit', now(), false) RETURNING id`
	scan(insertRelease, &aRelease, a)
	scan(insertRelease, &bRelease, b)
	scan(`INSERT INTO rules (library_id, group_id, path) VALUES ($1, $2, 'techs/go/a') RETURNING id`, &aRule, a, aGroup)
	insertVersion := `INSERT INTO rule_versions (library_id, rule_id, release_id, major, minor, patch, change, summaries)
		VALUES ($1, $2, $3, 1, 0, 0, 'new', '{Add the rule.}')`

	for name, tc := range map[string]struct {
		sql  string
		args []any
	}{
		"a rule in another library's group": {
			`INSERT INTO rules (library_id, group_id, path) VALUES ($1, $2, 'techs/go/b')`, []any{a, bGroup},
		},
		"a rule retired by another library's release": {
			`INSERT INTO rules (library_id, group_id, path, retired_in_release_id, retirement_summaries)
				VALUES ($1, $2, 'techs/go/c', $3, '{Retire it.}')`, []any{a, aGroup, bRelease},
		},
		"a version published by another library's release": {insertVersion, []any{a, aRule, bRelease}},
		"a version of another library's rule":              {insertVersion, []any{b, aRule, bRelease}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := conn.Exec(ctx, tc.sql, tc.args...)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23503" { // foreign_key_violation
				t.Fatalf("got %v, want a foreign key violation", err)
			}
		})
	}
	// The same references within one library are fine, so the refusals above come from crossing libraries.
	if _, err := conn.Exec(ctx, insertVersion, a, aRule, aRelease); err != nil {
		t.Fatalf("a version of a library's own rule and release: %v", err)
	}
}
