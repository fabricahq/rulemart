// Command migrate-database applies Rulemart's schema migrations. The release's "Migrate the database" workflow runs
// it after a release is approved and before it's published, and operators run it against a local database or a Neon
// branch. Functions refuse to use a database that lacks their release's migrations.
//
// Set one of:
//   - DATABASE_URL: a direct connection string, such as a local database's, or Neon's direct one.
//   - DATABASE_URL_PARAMETER: the SSM parameter holding Neon's pooled connection string, which the functions use.
//     migrate-database reads it with the ambient AWS credentials and derives the direct connection string.
//
// Migrations need a direct connection: their lock needs a session that Neon's pooler doesn't keep.
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fabricahq/rulemart/internal/platform/database"
	"github.com/fabricahq/rulemart/internal/platform/database/migrate"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	connString, err := directConnString(ctx, os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	if err := migrate.Up(ctx, connString); err != nil {
		log.Fatal(err)
	}
	version, err := migrate.RequiredVersion()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("database schema is at version %d", version)
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
