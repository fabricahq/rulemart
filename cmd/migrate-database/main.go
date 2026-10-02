// Command migrate-database applies Rulemart's schema migrations. The release's "Migrate the database" workflow runs
// it after a release is approved and before it's published, and operators run it against a local database or a Neon
// branch. Functions refuse to use a database that lacks their release's migrations.
//
// Set one of:
//   - DATABASE_URL: a direct connection string, such as a local database's, or Neon's direct one.
//   - DATABASE_URL_PARAMETER: the SSM parameter holding Neon's pooled connection string, which the functions use.
//     migrate-database reads it with the ambient AWS credentials and derives the direct connection string.
//
// Migrations need a direct connection: their lock needs a session that Neon's pooler doesn't keep. LOG_LEVEL and
// RULEMART_RELEASE configure its logs, as internal/platform/logging describes.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fabricahq/rulemart/internal/platform/database"
	"github.com/fabricahq/rulemart/internal/platform/database/migrate"
	"github.com/fabricahq/rulemart/internal/platform/logging"
)

func main() {
	logger, err := logging.New(os.Stdout, os.Getenv)
	if err != nil {
		logging.StartupFailed(logger, err)
		os.Exit(1)
	}
	// The log package then writes JSON lines through logger too.
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("migration failed", "error", err.Error())
		os.Exit(1)
	}
}

// run applies the pending migrations to the database the environment names, and logs each one it applied and the
// schema version the database reached. When a migration fails, it logs those applied before it.
func run(logger *slog.Logger) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	connString, err := directConnString(ctx, os.Getenv)
	if err != nil {
		return err
	}
	result, err := migrate.Up(ctx, connString)
	for _, migration := range result.Applied {
		logger.Info("applied migration", "version", migration.Version, "file", migration.File,
			"duration_ms", logging.Milliseconds(migration.Duration))
	}
	if err != nil {
		return err
	}
	logger.Info("migrated", "schema", result.Version, "applied", len(result.Applied))
	return nil
}

// directConnString returns the direct connection string to migrate, from the source getenv names, as
// database.SourceFromEnv chooses it. Its errors never include the connection string.
func directConnString(ctx context.Context, getenv func(string) string) (string, error) {
	source, err := database.SourceFromEnv(ctx, getenv)
	if err != nil {
		return "", err
	}
	connString, err := source.ConnString(ctx)
	if err != nil {
		return "", err
	}
	if source.FromParameter() {
		// The parameter holds the pooled connection string the functions use.
		return migrate.DirectConnString(connString)
	}
	config, err := pgx.ParseConfig(connString)
	if err != nil {
		// The parse error would repeat the connection string, so leave it out.
		return "", errors.New("DATABASE_URL isn't a valid Postgres connection string")
	}
	if strings.Contains(config.Host, "-pooler.") {
		return "", errors.New("DATABASE_URL points at Neon's connection pooler; use the direct connection string")
	}
	return connString, nil
}
