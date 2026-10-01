// Package migrate applies Rulemart's schema migrations and reports the schema version a release needs.
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path"

	_ "github.com/jackc/pgx/v5/stdlib" // Registers the pgx driver with database/sql for goose.
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/fabricahq/rulemart/db"
)

// Up applies every pending migration to the database at connString. It holds a Postgres advisory lock while it runs,
// as goose recommends when several processes may migrate at once, such as overlapping deploys. connString must be a
// direct connection: a transaction-mode pooler, such as Neon's, doesn't keep the session the lock needs.
func Up(ctx context.Context, connString string) error {
	migrations, err := fs.Sub(db.Migrations, "migrations")
	if err != nil {
		return fmt.Errorf("load migrations: %v", err)
	}
	return up(ctx, connString, migrations)
}

// up applies every pending migration in migrations, a directory of numbered SQL files, as Up does.
func up(ctx context.Context, connString string, migrations fs.FS) error {
	conn, err := sql.Open("pgx", connString)
	if err != nil {
		return fmt.Errorf("open database for migrations: %v", err)
	}
	defer conn.Close()
	provider, err := newProvider(conn, migrations)
	if err != nil {
		return err
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("apply migrations: %v", err)
	}
	return nil
}

// RequiredVersion returns the newest migration version in this release, which a database must have applied before
// the functions use it.
func RequiredVersion() (int64, error) {
	names, err := fs.Glob(db.Migrations, "migrations/*.sql")
	if err != nil {
		return 0, fmt.Errorf("list migrations: %v", err)
	}
	var newest int64
	for _, name := range names {
		version, err := goose.NumericComponent(path.Base(name))
		if err != nil {
			return 0, fmt.Errorf("read migration version name=%q: %v", name, err)
		}
		newest = max(newest, version)
	}
	if newest == 0 {
		return 0, fmt.Errorf("list migrations: no migrations are embedded")
	}
	return newest, nil
}

// newProvider returns a goose provider for migrations that serializes runs with a session lock.
func newProvider(conn *sql.DB, migrations fs.FS) (*goose.Provider, error) {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, fmt.Errorf("create migration lock: %v", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, conn, migrations, goose.WithSessionLocker(locker))
	if err != nil {
		return nil, fmt.Errorf("load migrations: %v", err)
	}
	return provider, nil
}
