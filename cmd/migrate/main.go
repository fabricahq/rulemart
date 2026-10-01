// Command migrate applies Rulemart's schema migrations to the database at DATABASE_URL. Run it before deploying a
// release whose functions need a newer schema; they refuse to use a database that lacks it.
//
// DATABASE_URL must be Neon's direct connection string, not the pooled one: the migration lock needs a session that
// the pooler doesn't keep.
package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fabricahq/rulemart/internal/platform/migrate"
)

func main() {
	connString := os.Getenv("DATABASE_URL")
	if connString == "" {
		log.Fatal("DATABASE_URL is not set")
	}
	// The parse error would repeat the connection string, so leave it out.
	cfg, err := pgx.ParseConfig(connString)
	if err != nil {
		log.Fatal("DATABASE_URL isn't a valid Postgres connection string")
	}
	if strings.Contains(cfg.Host, "-pooler.") {
		log.Fatal("DATABASE_URL points at Neon's connection pooler; use the direct connection string")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := migrate.Up(ctx, connString); err != nil {
		log.Fatal(err)
	}
	version, err := migrate.RequiredVersion()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("database schema is at version %d", version)
}
