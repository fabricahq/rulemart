// Package testdb gives each integration test its own Postgres database, created on the server that
// RULEMART_TEST_DATABASE_URL names and dropped when the test ends.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
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

// createWebRole creates WebRole on server unless it exists. Roles span the server, and tests in several packages
// create it at once, so losing that race to another test isn't an error.
func createWebRole(t *testing.T, server string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, server)
	if err != nil {
		t.Fatalf("connect to the test Postgres server (start one with make db): %v", err)
	}
	defer conn.Close(ctx)
	_, err = conn.Exec(ctx, "CREATE ROLE "+pgx.Identifier{WebRole}.Sanitize()+" LOGIN PASSWORD '"+webRolePassword+"'")
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "42710" || pgErr.Code == "23505") { // duplicate_object, unique_violation
		return
	}
	if err != nil {
		t.Fatalf("create role %s: %v", WebRole, err)
	}
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

// WithUser returns connString with its user and password replaced.
func WithUser(t *testing.T, connString, user, password string) string {
	t.Helper()
	u, err := url.Parse(connString)
	if err != nil {
		t.Fatalf("parse test connection string: %v", err)
	}
	u.User = url.UserPassword(user, password)
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
