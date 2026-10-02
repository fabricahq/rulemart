// Package databasetest opens database.DBs on migrated test databases, as the server's user or as a function's role.
// It's apart from postgrestest because the database package's own tests use postgrestest.
package databasetest

import (
	"context"
	"testing"

	"github.com/fabricahq/rulemart/internal/platform/database"
	"github.com/fabricahq/rulemart/internal/platform/database/migrate"
	"github.com/fabricahq/rulemart/internal/platform/postgrestest"
)

// New returns a DB for a new, migrated test database, and its connection string. It closes the DB when the test
// ends.
func New(t *testing.T) (*database.DB, string) {
	t.Helper()
	connString := postgrestest.New(t)
	if _, err := migrate.Up(context.Background(), connString); err != nil {
		t.Fatal(err)
	}
	return open(t, connString, "test-database"), connString
}

// AsWebRole returns a DB for the test database at connString that connects as postgrestest.WebRole, as the web
// function does, so it has only the access migrations grant postgrestest.CatalogReaderRole, its group. It closes the
// DB when the test ends.
func AsWebRole(t *testing.T, connString string) *database.DB {
	t.Helper()
	return open(t, postgrestest.AsWebRole(t, connString), "test-web-database")
}

// AsWorkerRole returns a DB for the test database at connString that connects as postgrestest.WorkerRole, as the
// worker function does, so it has only the access migrations grant postgrestest.CatalogWriterRole, its group. It
// closes the DB when the test ends.
func AsWorkerRole(t *testing.T, connString string) *database.DB {
	t.Helper()
	return open(t, postgrestest.AsWorkerRole(t, connString), "test-worker-database")
}

// open returns a DB for connString that expects the migrations' schema version, closed when the test ends.
func open(t *testing.T, connString, parameterName string) *database.DB {
	t.Helper()
	version, err := migrate.RequiredVersion()
	if err != nil {
		t.Fatal(err)
	}
	db := database.New(postgrestest.Parameter(connString), parameterName, version)
	t.Cleanup(db.Close)
	return db
}
