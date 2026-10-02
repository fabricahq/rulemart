// Command migrate applies Rulemart's schema migrations to the database at DATABASE_URL. Run it before deploying a
// release whose functions need a newer schema; they refuse to use a database that lacks it.
//
// DATABASE_URL must be Neon's direct connection string, not the pooled one: the migration lock needs a session that
// the pooler doesn't keep. LOG_LEVEL and RULEMART_RELEASE configure its logs, as internal/platform/logging
// describes.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

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

// run applies the pending migrations to the database at DATABASE_URL, and logs the schema version it reached.
func run(logger *slog.Logger) error {
	connString := os.Getenv("DATABASE_URL")
	if connString == "" {
		return errors.New("DATABASE_URL is not set")
	}
	// The parse error would repeat the connection string, so leave it out.
	cfg, err := pgx.ParseConfig(connString)
	if err != nil {
		return errors.New("DATABASE_URL isn't a valid Postgres connection string")
	}
	if strings.Contains(cfg.Host, "-pooler.") {
		return errors.New("DATABASE_URL points at Neon's connection pooler; use the direct connection string")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := migrate.Up(ctx, connString); err != nil {
		return err
	}
	version, err := migrate.RequiredVersion()
	if err != nil {
		return err
	}
	logger.Info("migrated", "schema", version)
	return nil
}
