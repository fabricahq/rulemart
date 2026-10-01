package migrate

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fabricahq/rulemart/internal/platform/postgrestest"
)

// The web function connects as the web role, which may read the catalog and the schema version, and nothing else.
func TestMigrationsLetTheWebRoleReadTheCatalogAndNothingElse(t *testing.T) {
	ctx := context.Background()
	connString := postgrestest.New(t)
	if err := Up(ctx, connString); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, postgrestest.AsWebRole(t, connString))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)

	for _, table := range []string{"libraries", "library_releases", "library_groups", "rules", "rule_versions", "goose_db_version"} {
		if _, err := conn.Exec(ctx, "SELECT count(*) FROM "+table); err != nil {
			t.Errorf("read %s: %v", table, err)
		}
		for _, privilege := range []string{"INSERT", "UPDATE", "DELETE", "TRUNCATE", "REFERENCES", "TRIGGER"} {
			var has bool
			if err := conn.QueryRow(ctx, "SELECT has_table_privilege(current_user, $1, $2)", table, privilege).Scan(&has); err != nil {
				t.Fatal(err)
			}
			if has {
				t.Errorf("the web role has %s on %s", privilege, table)
			}
		}
	}
	for name, statement := range map[string]string{
		"write the catalog":      `INSERT INTO libraries (host, host_repository_id, owner, name, description, owner_avatar_url) VALUES ('github', '1', 'o', 'n', '', '')`,
		"change the schema":      `CREATE TABLE intruder (id integer)`,
		"read skeleton messages": `SELECT count(*) FROM hello_messages`,
	} {
		_, err := conn.Exec(ctx, statement)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" { // insufficient_privilege
			t.Errorf("%s: got %v, want permission denied", name, err)
		}
	}
}
