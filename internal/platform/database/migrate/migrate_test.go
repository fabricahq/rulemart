package migrate

import (
	"context"
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fabricahq/rulemart/db"
	"github.com/fabricahq/rulemart/internal/platform/postgrestest"
)

// The web function connects as the web role, which may read the catalog and the schema version, through its
// membership in the catalog reader role, and nothing else.
func TestMigrationsLetTheWebRoleReadTheCatalogAndNothingElse(t *testing.T) {
	ctx := context.Background()
	connString := postgrestest.New(t)
	if _, err := Up(ctx, connString); err != nil {
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

// Migrations grant to the catalog reader role, never to the web role, so infrastructure can replace or rotate the
// login without a migration.
func TestMigrationsGrantTheCatalogReaderRoleAndNotTheWebRole(t *testing.T) {
	ctx := context.Background()
	connString := postgrestest.New(t)
	if _, err := Up(ctx, connString); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	grants := func(role string) []string {
		t.Helper()
		var got []string
		err := conn.QueryRow(ctx, `
			SELECT ARRAY(
				SELECT 'table ' || c.relname || ' ' || a.privilege_type
				FROM pg_class c CROSS JOIN LATERAL aclexplode(c.relacl) a WHERE a.grantee = to_regrole($1)
				UNION ALL
				SELECT 'schema ' || n.nspname || ' ' || a.privilege_type
				FROM pg_namespace n CROSS JOIN LATERAL aclexplode(n.nspacl) a WHERE a.grantee = to_regrole($1)
				ORDER BY 1)`, role).Scan(&got)
		if err != nil {
			t.Fatalf("read %s's grants: %v", role, err)
		}
		return got
	}

	want := []string{
		"schema public USAGE",
		"table goose_db_version SELECT",
		"table libraries SELECT",
		"table library_groups SELECT",
		"table library_releases SELECT",
		"table rule_versions SELECT",
		"table rules SELECT",
	}
	if got := grants(postgrestest.CatalogReaderRole); !slices.Equal(got, want) {
		t.Errorf("%s has %q, want %q", postgrestest.CatalogReaderRole, got, want)
	}
	if got := grants(postgrestest.WebRole); len(got) > 0 {
		t.Errorf("%s has its own grants %q; migrations must grant to %s", postgrestest.WebRole, got, postgrestest.CatalogReaderRole)
	}
}

// Grants can't narrow what a privileged role already holds, so the migration must refuse a catalog reader role that
// isn't the plain NOLOGIN group role infrastructure creates, rather than give its members a false boundary. Roles span
// the server, so each case gives the migration its own role instead of changing the one other tests share.
func TestMigrationsRefuseACatalogReaderRoleThatIsMissingOrPrivileged(t *testing.T) {
	server := postgrestest.Server(t)
	for name, tc := range map[string]struct {
		create []string
		want   string
	}{
		"missing":          {nil, "does not exist"},
		"able to log in":   {[]string{"CREATE ROLE %s LOGIN"}, "can log in"},
		"a superuser":      {[]string{"CREATE ROLE %s NOLOGIN SUPERUSER"}, "is privileged"},
		"with CREATEROLE":  {[]string{"CREATE ROLE %s NOLOGIN CREATEROLE"}, "is privileged"},
		"with CREATEDB":    {[]string{"CREATE ROLE %s NOLOGIN CREATEDB"}, "is privileged"},
		"with BYPASSRLS":   {[]string{"CREATE ROLE %s NOLOGIN BYPASSRLS"}, "is privileged"},
		"with REPLICATION": {[]string{"CREATE ROLE %s NOLOGIN REPLICATION"}, "is privileged"},
		"a member of a predefined role": {
			[]string{"CREATE ROLE %s NOLOGIN", "GRANT pg_read_all_data TO %s"}, "is privileged",
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			role := "rulemart_reader_" + postgrestest.RandomHex(t, 6)
			// Registered before the database, so it runs after the database, and the grants in it, are dropped.
			t.Cleanup(func() { postgrestest.Exec(t, server, "DROP ROLE IF EXISTS "+role) })
			for _, statement := range tc.create {
				postgrestest.Exec(t, server, strings.ReplaceAll(statement, "%s", role))
			}
			connString := postgrestest.New(t)

			result, err := up(ctx, connString, migrationsGrantingTo(t, role))
			if err == nil || !strings.Contains(err.Error(), role+" "+tc.want) || !strings.Contains(err.Error(), "infrastructure creates it") {
				t.Fatalf("got %v, want a refusal saying %s %s and that infrastructure creates it", err, role, tc.want)
			}
			if got := appliedVersions(result); !slices.Equal(got, []int64{1, 2}) {
				t.Errorf("reported %v applied, want the two before the refused migration", got)
			}
			var version int64
			postgrestest.QueryRow(t, connString, "SELECT max(version_id) FROM goose_db_version", &version)
			if version != 2 {
				t.Errorf("the database is at version %d, want 2, before the refused migration", version)
			}
		})
	}

	// The same migrations accept a plain NOLOGIN role, so the refusals above come from the role's shape.
	t.Run("a plain NOLOGIN role", func(t *testing.T) {
		role := "rulemart_reader_" + postgrestest.RandomHex(t, 6)
		t.Cleanup(func() { postgrestest.Exec(t, server, "DROP ROLE IF EXISTS "+role) })
		postgrestest.Exec(t, server, "CREATE ROLE "+role+" NOLOGIN")
		if _, err := up(context.Background(), postgrestest.New(t), migrationsGrantingTo(t, role)); err != nil {
			t.Fatal(err)
		}
	})
}

// The migrate command logs what Up reports, which CI keeps: each migration it applied, oldest first, and the
// version the database reached, even when it was already current.
func TestUpReportsTheMigrationsItAppliedAndTheVersionReached(t *testing.T) {
	ctx := context.Background()
	connString := postgrestest.New(t)
	required, err := RequiredVersion()
	if err != nil {
		t.Fatal(err)
	}

	first, err := Up(ctx, connString)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Up(ctx, connString)
	if err != nil {
		t.Fatal(err)
	}

	applied := appliedVersions(first)
	if len(applied) != int(required) || applied[0] != 1 || applied[len(applied)-1] != required || !slices.IsSorted(applied) {
		t.Errorf("the first run reported %v applied, want 1 through %d", applied, required)
	}
	if first.Applied[0].File != "00001_create_hello_messages.sql" {
		t.Errorf("the first migration's file is %q", first.Applied[0].File)
	}
	if first.Version != required || second.Version != required || len(second.Applied) != 0 {
		t.Errorf("reported %+v, then %+v; want version %d both times, and nothing applied the second time", first, second, required)
	}
}

// appliedVersions returns the version of each migration result reports applied, in order.
func appliedVersions(result Result) []int64 {
	var versions []int64
	for _, migration := range result.Applied {
		versions = append(versions, migration.Version)
	}
	return versions
}

// migrationsGrantingTo returns the embedded migrations with the catalog reader role's name replaced by role.
func migrationsGrantingTo(t *testing.T, role string) fs.FS {
	t.Helper()
	migrations := fstest.MapFS{}
	err := fs.WalkDir(db.Migrations, "migrations", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := fs.ReadFile(db.Migrations, name)
		if err != nil {
			return err
		}
		replaced := strings.ReplaceAll(string(content), postgrestest.CatalogReaderRole, role)
		migrations[strings.TrimPrefix(name, "migrations/")] = &fstest.MapFile{Data: []byte(replaced)}
		return nil
	})
	if err != nil {
		t.Fatalf("read the embedded migrations: %v", err)
	}
	return migrations
}
