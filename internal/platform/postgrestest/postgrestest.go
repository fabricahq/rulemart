// Package postgrestest gives each integration test its own Postgres database, created on the server that
// RULEMART_TEST_DATABASE_URL names and dropped when the test ends.
package postgrestest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/jackc/pgx/v5"
)

// ServerEnv names the variable holding a connection string for a Postgres server where tests may create databases.
const ServerEnv = "RULEMART_TEST_DATABASE_URL"

// Server returns the connection string in ServerEnv. Without one, it skips t locally and fails t in CI, where the
// workflow provides a server.
func Server(t *testing.T) string {
	t.Helper()
	server := os.Getenv(ServerEnv)
	if server != "" {
		return server
	}
	if os.Getenv("CI") != "" {
		t.Fatalf("%s is not set; CI must provide a Postgres server", ServerEnv)
	}
	t.Skipf("set %s to a Postgres server to run database tests", ServerEnv)
	return ""
}

// CatalogReaderRole is the group role migrations grant catalog reads to. It can't log in; WebRole is its member.
// Infrastructure creates it in production; tests and local development create it too.
const CatalogReaderRole = "rulemart_catalog_reader"

// WebRole is the login role the web function connects as. It has no grants of its own, and reads the catalog through
// its membership in CatalogReaderRole. Infrastructure creates it in production; tests and local development create it
// with webRolePassword, a test value.
const WebRole = "rulemart_web"

// webRolePassword is the role's local password, the Makefile's LOCAL_WEB_ROLE_PASSWORD, so tests and make db agree
// on the role they create.
const webRolePassword = "rulemart-web-local"

// CatalogWriterRole is the group role migrations grant what ingestion writes. It can't log in; WorkerRole is its
// member. Infrastructure creates it in production; tests and local development create it too.
const CatalogWriterRole = "rulemart_catalog_writer"

// WorkerRole is the login role the worker function and the ingest command connect as. It has no grants of its own,
// and writes the catalog through its membership in CatalogWriterRole. Infrastructure creates it in production; tests
// and local development create it with workerRolePassword, a test value.
const WorkerRole = "rulemart_worker"

// workerRolePassword is the role's local password, the Makefile's LOCAL_WORKER_ROLE_PASSWORD.
const workerRolePassword = "rulemart-worker-local"

// login is a login role infrastructure creates, with its local password and the group role it's a member of.
type login struct {
	name, password, group string
}

// logins are the login roles New makes sure the server has, each with its group.
var logins = []login{
	{WebRole, webRolePassword, CatalogReaderRole},
	{WorkerRole, workerRolePassword, CatalogWriterRole},
}

// New creates an empty database for t and returns a connection string for it, as the server's user. It first
// makes sure the server has the roles infrastructure creates: CatalogReaderRole and WebRole, its member, and
// CatalogWriterRole and WorkerRole, its member.
func New(t *testing.T) string {
	t.Helper()
	server := Server(t)
	createRoles(t, server)
	name := "rulemart_test_" + RandomHex(t, 8)
	exec(t, server, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize())
	t.Cleanup(func() { exec(t, server, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)") })
	return withDatabase(t, server, name)
}

// AsWebRole returns connString with WebRole as its user, to connect as the web function does.
func AsWebRole(t *testing.T, connString string) string {
	t.Helper()
	return WithUser(t, connString, WebRole, webRolePassword)
}

// AsWorkerRole returns connString with WorkerRole as its user, to connect as the worker function does.
func AsWorkerRole(t *testing.T, connString string) string {
	t.Helper()
	return WithUser(t, connString, WorkerRole, workerRolePassword)
}

// rolesLock is the advisory lock key that serializes creating the shared roles.
const rolesLock = 7_392_614_028

// createRoles creates each login role and its group on server unless they exist, as infrastructure creates them: a
// NOLOGIN group role, and a LOGIN role that signs in with its local password and is a member of it. It fails t unless
// each has that shape. Roles span the server, and tests in several packages create them at once, so one transaction
// at a time creates them, under an advisory lock.
func createRoles(t *testing.T, server string) {
	t.Helper()
	ctx := context.Background()
	conn := connect(t, server)
	err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", rolesLock); err != nil {
			return fmt.Errorf("lock the shared roles: %v", err)
		}
		create := func(role, statement string) error {
			var found bool
			err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT FROM pg_roles WHERE rolname = $1)", role).Scan(&found)
			if err == nil && !found {
				_, err = tx.Exec(ctx, statement)
			}
			if err != nil {
				return fmt.Errorf("create role %s: %v", role, err)
			}
			return nil
		}
		for _, l := range logins {
			group, name := pgx.Identifier{l.group}.Sanitize(), pgx.Identifier{l.name}.Sanitize()
			if err := create(l.group, "CREATE ROLE "+group+" NOLOGIN"); err != nil {
				return err
			}
			if err := create(l.name, "CREATE ROLE "+name+" LOGIN PASSWORD '"+l.password+"' IN ROLE "+group); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range logins {
		if err := checkRole(ctx, conn, l.group, false); err != nil {
			t.Fatal(err)
		}
		if err := checkRole(ctx, conn, l.name, true, l.group); err != nil {
			t.Fatal(err)
		}
		signedIn, err := pgx.Connect(ctx, WithUser(t, server, l.name, l.password))
		if err != nil {
			t.Fatalf("sign in as %s with the test password: %v; %s", l.name, err, resetRoles)
		}
		_ = signedIn.Close(ctx)
	}
}

// resetRoles tells a developer how to replace a test server's shared roles that tests can't use. Tests never change
// or drop the shared roles themselves.
const resetRoles = "recreate the test server with make db-stop and make db, or drop the roles (DROP OWNED BY " +
	sharedRoles + " in each database, then DROP ROLE " + sharedRoles + ") so the tests create them again"

// sharedRoles names the roles tests create, logins before their groups, so DROP ROLE can drop them in order.
const sharedRoles = WebRole + ", " + WorkerRole + ", " + CatalogReaderRole + ", " + CatalogWriterRole

// checkRole returns an error unless role has the shape infrastructure gives it: it can log in only if login is true,
// has no other attribute, and is a member of exactly memberOf.
func checkRole(ctx context.Context, conn *pgx.Conn, role string, login bool, memberOf ...string) error {
	var canLogin bool
	var extra, memberships []string
	err := conn.QueryRow(ctx, `
		SELECT r.rolcanlogin,
		       array_remove(ARRAY[
		           CASE WHEN r.rolsuper THEN 'SUPERUSER' END,
		           CASE WHEN r.rolcreaterole THEN 'CREATEROLE' END,
		           CASE WHEN r.rolcreatedb THEN 'CREATEDB' END,
		           CASE WHEN r.rolbypassrls THEN 'BYPASSRLS' END,
		           CASE WHEN r.rolreplication THEN 'REPLICATION' END], NULL),
		       ARRAY(SELECT g.rolname FROM pg_auth_members m JOIN pg_roles g ON g.oid = m.roleid
		             WHERE m.member = r.oid ORDER BY g.rolname)
		FROM pg_roles r WHERE r.rolname = $1`, role).Scan(&canLogin, &extra, &memberships)
	if err != nil {
		return fmt.Errorf("read role %s: %v", role, err)
	}
	var problems []string
	switch {
	case login && !canLogin:
		problems = append(problems, "it can't log in")
	case !login && canLogin:
		problems = append(problems, "it can log in")
	}
	if len(extra) > 0 {
		problems = append(problems, "it has "+strings.Join(extra, ", "))
	}
	if want := slices.Sorted(slices.Values(memberOf)); !slices.Equal(memberships, want) {
		only := ""
		if len(want) > 0 {
			only = "only "
		}
		problems = append(problems, "it's a member of "+roleList(memberships)+", but should be a member of "+only+
			roleList(want))
	}
	if len(problems) > 0 {
		return fmt.Errorf("role %s on the test server isn't shaped like production's: %s; %s", role,
			strings.Join(problems, "; "), resetRoles)
	}
	return nil
}

// roleList names roles for an error message.
func roleList(roles []string) string {
	if len(roles) == 0 {
		return "no role"
	}
	return strings.Join(roles, ", ")
}

// connect opens a connection to connString for t, closed when t ends.
func connect(t *testing.T, connString string) *pgx.Conn {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		t.Fatalf("connect to the test Postgres server (start one with make db): %v", err)
	}
	t.Cleanup(func() { conn.Close(ctx) })
	return conn
}

// Parameter stands in for an SSM parameter whose value is a connection string that never changes.
type Parameter string

// GetParameter returns p as the parameter's value.
func (p Parameter) GetParameter(context.Context, *ssm.GetParameterInput, ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	return &ssm.GetParameterOutput{Parameter: &types.Parameter{Value: aws.String(string(p))}}, nil
}

// Exec runs one statement on the database at connString and fails t if it errors.
func Exec(t *testing.T, connString, statement string, args ...any) {
	t.Helper()
	exec(t, connString, statement, args...)
}

func exec(t *testing.T, connString, statement string, args ...any) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		t.Fatalf("connect to the test Postgres server (start one with make db): %v", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, statement, args...); err != nil {
		t.Fatalf("run %q: %v", statement, err)
	}
}

// QueryRow runs a query returning one row on the database at connString, scans it into dest, and fails t if it
// errors.
func QueryRow(t *testing.T, connString, query string, dest ...any) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	defer conn.Close(ctx)
	if err := conn.QueryRow(ctx, query).Scan(dest...); err != nil {
		t.Fatalf("run %q: %v", query, err)
	}
}

// WithUser returns connString with its user and password replaced. It removes user and password from the query
// too, since pgx applies those after the URL's userinfo.
func WithUser(t *testing.T, connString, user, password string) string {
	t.Helper()
	u, err := url.Parse(connString)
	if err != nil {
		t.Fatalf("parse test connection string: %v", err)
	}
	u.User = url.UserPassword(user, password)
	query := u.Query()
	query.Del("user")
	query.Del("password")
	u.RawQuery = query.Encode()
	return u.String()
}

// withDatabase returns connString pointed at database name.
func withDatabase(t *testing.T, connString, name string) string {
	t.Helper()
	u, err := url.Parse(connString)
	if err != nil {
		t.Fatalf("parse %s: %v", ServerEnv, err)
	}
	u.Path = "/" + name
	return u.String()
}

// RandomHex returns n random bytes as hex, for names that must not collide across tests.
func RandomHex(t *testing.T, n int) string {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}
