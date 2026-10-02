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

// The web function connects as the web role, which may read the catalog, its listings, and the schema version,
// through its membership in the catalog reader role, and write accounts, sessions, and listings, through its
// membership in the accounts writer role, and nothing else.
func TestMigrationsLetTheWebRoleReadTheCatalogSignVisitorsInAndListLibrariesAndNothingElse(t *testing.T) {
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
		"add an account":     `INSERT INTO accounts (github_user_id, github_login, avatar_url) VALUES (1, 'octocat', '')`,
		"rename it":          `UPDATE accounts SET github_login = 'renamed' WHERE github_user_id = 1`,
		"sign it in":         `INSERT INTO sessions (token_hash, account_id, expires_at) SELECT sha256('token'), id, now() + interval '1 day' FROM accounts`,
		"find its session":   `SELECT a.github_login FROM sessions s JOIN accounts a ON a.id = s.account_id`,
		"list a library":     `INSERT INTO listings (account_id, host, owner, name) SELECT id, 'github', 'octocat', 'rules' FROM accounts`,
		"count its requests": `SELECT count(*) FROM listing_requests WHERE requested_at > now() - interval '1 day'`,
		"record a request":   `INSERT INTO listing_requests (account_id) SELECT id FROM accounts`,
		"forget old ones":    `DELETE FROM listing_requests WHERE requested_at < now() - interval '1 day'`,
		"find its listings":  `SELECT l.owner, l.name FROM listings l JOIN accounts a ON a.id = l.account_id`,
		"try it again":       `UPDATE listings SET requested_at = now(), failure = NULL`,
		"remove it":          `DELETE FROM listings WHERE name = 'gone'`,
		"sign it out":        `DELETE FROM sessions`,
		"delete the account": `DELETE FROM accounts`,
	} {
		if _, err := conn.Exec(ctx, statement); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, statement := range map[string]string{
		"write the catalog":      `INSERT INTO libraries (host, host_repository_id, owner, name, description, owner_avatar_url) VALUES ('github', '1', 'o', 'n', '', '')`,
		"extend a session":       `UPDATE sessions SET expires_at = expires_at + interval '1 year'`,
		"resolve a listing":      `UPDATE listings SET host_repository_id = '1'`,
		"give a listing away":    `UPDATE listings SET account_id = NULL`,
		"fake a check":           `UPDATE listings SET checked_at = now()`,
		"empty the accounts":     `TRUNCATE accounts CASCADE`,
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

// The worker connects as the worker role, which may write the catalog's rows, through its membership in the catalog
// writer role, and nothing else: it can't delete a library, empty a table, change the schema, or read the skeleton's
// table.
func TestMigrationsLetTheWorkerRoleWriteTheCatalogAndNothingElse(t *testing.T) {
	ctx := context.Background()
	connString := postgrestest.New(t)
	if _, err := Up(ctx, connString); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, postgrestest.AsWorkerRole(t, connString))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)

	for name, statement := range map[string]string{
		"add a library": `INSERT INTO libraries (host, host_repository_id, owner, name, description, owner_avatar_url)
			VALUES ('github', '1', 'o', 'n', '', '')`,
		"rename it":           `UPDATE libraries SET name = 'renamed' WHERE host_repository_id = '1'`,
		"add a release":       `INSERT INTO library_releases (library_id, number, commit_id, tagged_at, updates_shared_files) SELECT id, 1, 'c', now(), false FROM libraries`,
		"delete a release":    `DELETE FROM library_releases`,
		"read the schema":     `SELECT count(*) FROM goose_db_version`,
		"delete stale rules":  `DELETE FROM rules`,
		"delete stale groups": `DELETE FROM library_groups`,
		"delete old versions": `DELETE FROM rule_versions`,
		"read the listings":   `SELECT owner, name, host_repository_id FROM listings`,
		"record a check":      `UPDATE listings SET host_repository_id = '1', checked_at = now(), failure = 'broken'`,
	} {
		if _, err := conn.Exec(ctx, statement); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, statement := range map[string]string{
		"delete a library":       `DELETE FROM libraries`,
		"empty a table":          `TRUNCATE rule_versions`,
		"change the schema":      `CREATE TABLE intruder (id integer)`,
		"change the version":     `DELETE FROM goose_db_version`,
		"read skeleton messages": `SELECT count(*) FROM hello_messages`,
		"read the accounts":      `SELECT count(*) FROM accounts`,
		"read the sessions":      `SELECT count(*) FROM sessions`,
		"list a library":         `INSERT INTO listings (account_id, host, owner, name) VALUES (1, 'github', 'o', 'n')`,
		"read listing requests":  `SELECT count(*) FROM listing_requests`,
		"remove a listing":       `DELETE FROM listings`,
		"give a listing away":    `UPDATE listings SET account_id = NULL`,
		"rename a listing":       `UPDATE listings SET name = 'other'`,
	} {
		_, err := conn.Exec(ctx, statement)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" { // insufficient_privilege
			t.Errorf("%s: got %v, want permission denied", name, err)
		}
	}
}

// Migrations grant to the group roles, never to the login roles, so infrastructure can replace or rotate a login
// without a migration.
func TestMigrationsGrantTheGroupRolesAndNotTheLogins(t *testing.T) {
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
				SELECT 'column ' || c.relname || '.' || t.attname || ' ' || a.privilege_type
				FROM pg_attribute t JOIN pg_class c ON c.oid = t.attrelid CROSS JOIN LATERAL aclexplode(t.attacl) a
				WHERE a.grantee = to_regrole($1)
				UNION ALL
				SELECT 'schema ' || n.nspname || ' ' || a.privilege_type
				FROM pg_namespace n CROSS JOIN LATERAL aclexplode(n.nspacl) a WHERE a.grantee = to_regrole($1)
				ORDER BY 1)`, role).Scan(&got)
		if err != nil {
			t.Fatalf("read %s's grants: %v", role, err)
		}
		return got
	}

	for role, want := range map[string][]string{
		postgrestest.CatalogReaderRole: {
			"schema public USAGE",
			"table goose_db_version SELECT",
			"table libraries SELECT",
			"table library_groups SELECT",
			"table library_releases SELECT",
			"table listings SELECT",
			"table rule_versions SELECT",
			"table rules SELECT",
		},
		postgrestest.CatalogWriterRole: {
			"column listings.checked_at UPDATE",
			"column listings.failure UPDATE",
			"column listings.host_repository_id UPDATE",
			"schema public USAGE",
			"table goose_db_version SELECT",
			"table libraries INSERT",
			"table libraries SELECT",
			"table libraries UPDATE",
			"table library_groups DELETE",
			"table library_groups INSERT",
			"table library_groups SELECT",
			"table library_groups UPDATE",
			"table library_releases DELETE",
			"table library_releases INSERT",
			"table library_releases SELECT",
			"table library_releases UPDATE",
			"table listings SELECT",
			"table rule_versions DELETE",
			"table rule_versions INSERT",
			"table rule_versions SELECT",
			"table rule_versions UPDATE",
			"table rules DELETE",
			"table rules INSERT",
			"table rules SELECT",
			"table rules UPDATE",
		},
		postgrestest.AccountsWriterRole: {
			"column listings.failure UPDATE",
			"column listings.requested_at UPDATE",
			"schema public USAGE",
			"table accounts DELETE",
			"table accounts INSERT",
			"table accounts SELECT",
			"table accounts UPDATE",
			"table listing_requests DELETE",
			"table listing_requests INSERT",
			"table listing_requests SELECT",
			"table listings DELETE",
			"table listings INSERT",
			"table listings SELECT",
			"table sessions DELETE",
			"table sessions INSERT",
			"table sessions SELECT",
		},
	} {
		if got := grants(role); !slices.Equal(got, want) {
			t.Errorf("%s has %q, want %q", role, got, want)
		}
	}
	for login, groups := range map[string]string{
		postgrestest.WebRole:    postgrestest.CatalogReaderRole + " and " + postgrestest.AccountsWriterRole,
		postgrestest.WorkerRole: postgrestest.CatalogWriterRole,
	} {
		if got := grants(login); len(got) > 0 {
			t.Errorf("%s has its own grants %q; migrations must grant to %s", login, got, groups)
		}
	}
}

// Grants can't narrow what a privileged role already holds, so each migration that grants a group role must refuse
// one that isn't the plain NOLOGIN group role infrastructure creates, rather than give its members a false boundary.
// Roles span the server, so each case gives the migrations its own role instead of changing the one other tests
// share.
func TestMigrationsRefuseAGroupRoleThatIsMissingOrPrivileged(t *testing.T) {
	server := postgrestest.Server(t)
	for _, group := range []struct {
		role string
		// before is the version the database is at when the migration that grants role refuses it.
		before int64
	}{
		{postgrestest.CatalogReaderRole, 2},
		{postgrestest.CatalogWriterRole, 4},
		{postgrestest.AccountsWriterRole, 8},
	} {
		t.Run(group.role, func(t *testing.T) {
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
					role := "rulemart_group_" + postgrestest.RandomHex(t, 6)
					// Registered before the database, so it runs after the database, and the grants in it, are dropped.
					t.Cleanup(func() { postgrestest.Exec(t, server, "DROP ROLE IF EXISTS "+role) })
					for _, statement := range tc.create {
						postgrestest.Exec(t, server, strings.ReplaceAll(statement, "%s", role))
					}
					connString := postgrestest.New(t)

					result, err := up(ctx, connString, migrationsGrantingTo(t, group.role, role))
					if err == nil || !strings.Contains(err.Error(), role+" "+tc.want) || !strings.Contains(err.Error(), "infrastructure creates it") {
						t.Fatalf("got %v, want a refusal saying %s %s and that infrastructure creates it", err, role, tc.want)
					}
					if got := appliedVersions(result); len(got) != int(group.before) || got[len(got)-1] != group.before {
						t.Errorf("reported %v applied, want 1 through %d, those before the refused migration", got, group.before)
					}
					var version int64
					postgrestest.QueryRow(t, connString, "SELECT max(version_id) FROM goose_db_version", &version)
					if version != group.before {
						t.Errorf("the database is at version %d, want %d, before the refused migration", version, group.before)
					}
				})
			}

			// The same migrations accept a plain NOLOGIN role, so the refusals above come from the role's shape.
			t.Run("a plain NOLOGIN role", func(t *testing.T) {
				role := "rulemart_group_" + postgrestest.RandomHex(t, 6)
				t.Cleanup(func() { postgrestest.Exec(t, server, "DROP ROLE IF EXISTS "+role) })
				postgrestest.Exec(t, server, "CREATE ROLE "+role+" NOLOGIN")
				if _, err := up(context.Background(), postgrestest.New(t), migrationsGrantingTo(t, group.role, role)); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
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

// migrationsGrantingTo returns the embedded migrations with the group role named group replaced by role.
func migrationsGrantingTo(t *testing.T, group, role string) fs.FS {
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
		replaced := strings.ReplaceAll(string(content), group, role)
		migrations[strings.TrimPrefix(name, "migrations/")] = &fstest.MapFile{Data: []byte(replaced)}
		return nil
	})
	if err != nil {
		t.Fatalf("read the embedded migrations: %v", err)
	}
	return migrations
}
