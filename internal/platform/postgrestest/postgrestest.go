// Package postgrestest gives each integration test its own Postgres database, created on the server that
// RULEMART_TEST_DATABASE_URL names and dropped when the test ends.
package postgrestest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

// WebRole is the role the web function connects as. Infrastructure creates it in production; tests and local
// development create it with webRolePassword, a test value.
const WebRole = "rulemart_web"

// webRolePassword is the role's local password, the Makefile's LOCAL_WEB_ROLE_PASSWORD, so tests and make db agree
// on the role they create.
const webRolePassword = "rulemart-web-local"

// New creates an empty database for t and returns a connection string for it, as the server's user. It first
// makes sure the server has WebRole, which migrations grant access to.
func New(t *testing.T) string {
	t.Helper()
	server := Server(t)
	createWebRole(t, server)
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

// createWebRole creates WebRole on server unless it exists, and fails t unless the role is a plain LOGIN role
// that signs in with webRolePassword. Roles span the server, and tests in several packages create it at once, so
// losing that race to another test isn't an error.
func createWebRole(t *testing.T, server string) {
	t.Helper()
	ctx := context.Background()
	conn := connect(t, server)
	_, err := conn.Exec(ctx, "CREATE ROLE "+pgx.Identifier{WebRole}.Sanitize()+" LOGIN PASSWORD '"+webRolePassword+"'")
	var pgErr *pgconn.PgError
	if err != nil && !(errors.As(err, &pgErr) && (pgErr.Code == "42710" || pgErr.Code == "23505")) { // duplicate_object, unique_violation
		t.Fatalf("create role %s: %v", WebRole, err)
	}
	if err := checkRole(ctx, conn, WebRole); err != nil {
		t.Fatal(err)
	}
	web, err := pgx.Connect(ctx, AsWebRole(t, server))
	if err != nil {
		t.Fatalf("sign in as %s with the test password: %v; %s", WebRole, err, resetWebRole)
	}
	_ = web.Close(ctx)
}

// resetWebRole tells a developer how to replace a test server's rulemart_web that tests can't use. Tests never
// change or drop the shared role themselves.
const resetWebRole = "recreate the test server with make db-stop and make db, or drop the role (DROP OWNED BY " +
	WebRole + " in each database, then DROP ROLE " + WebRole + ") so the tests create it again"

// checkRole returns an error unless role is a plain LOGIN role, as infrastructure creates rulemart_web: it can log
// in, and it has no other attribute and belongs to no role.
func checkRole(ctx context.Context, conn *pgx.Conn, role string) error {
	var login bool
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
		FROM pg_roles r WHERE r.rolname = $1`, role).Scan(&login, &extra, &memberships)
	if err != nil {
		return fmt.Errorf("read role %s: %v", role, err)
	}
	var problems []string
	if !login {
		problems = append(problems, "it can't log in")
	}
	if len(extra) > 0 {
		problems = append(problems, "it has "+strings.Join(extra, ", "))
	}
	if len(memberships) > 0 {
		problems = append(problems, "it's a member of "+strings.Join(memberships, ", "))
	}
	if len(problems) > 0 {
		return fmt.Errorf("role %s on the test server isn't a plain LOGIN role like production's: %s; %s", role,
			strings.Join(problems, "; "), resetWebRole)
	}
	return nil
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
