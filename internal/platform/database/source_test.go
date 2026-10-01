package database

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fabricahq/rulemart/internal/platform/migrate"
	"github.com/fabricahq/rulemart/internal/platform/postgrestest"
)

func env(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

func TestSourceFromEnvConnectsWithDatabaseURL(t *testing.T) {
	connString := postgrestest.New(t)
	if err := migrate.Up(context.Background(), connString); err != nil {
		t.Fatal(err)
	}
	version, err := migrate.RequiredVersion()
	if err != nil {
		t.Fatal(err)
	}

	source, err := SourceFromEnv(context.Background(), env(map[string]string{"DATABASE_URL": connString}))
	if err != nil {
		t.Fatal(err)
	}
	db := source.Open(version)
	defer db.Close()

	err = db.Run(context.Background(), func(pool *pgxpool.Pool) error { return pool.Ping(context.Background()) })
	if err != nil {
		t.Fatalf("connect with DATABASE_URL: %v", err)
	}
}

func TestSourceFromEnvRequiresExactlyOneSource(t *testing.T) {
	for name, vars := range map[string]map[string]string{
		"neither": {},
		"both":    {"DATABASE_URL": "postgres://localhost/rulemart", "DATABASE_URL_PARAMETER": "/rulemart/database-url"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := SourceFromEnv(context.Background(), env(vars)); err == nil {
				t.Fatal("accepted the environment")
			}
		})
	}
}
