// Command migrate applies Rulemart's schema migrations. The release workflow runs it before publishing each release,
// and operators can run it against a local database or a Neon branch. Functions refuse to use a database that lacks
// their release's migrations.
//
// Set one of:
//   - DATABASE_URL: a direct connection string, such as a local database's, or Neon's direct one.
//   - DATABASE_URL_PARAMETER: the SSM parameter holding Neon's pooled connection string, which the functions use.
//     migrate reads it with the ambient AWS credentials and derives the direct connection string.
//
// Migrations need a direct connection: their lock needs a session that Neon's pooler doesn't keep.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/jackc/pgx/v5"

	"github.com/fabricahq/rulemart/internal/platform/database/migrate"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	connString, err := directConnString(ctx, os.Getenv("DATABASE_URL"), os.Getenv("DATABASE_URL_PARAMETER"))
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

// directConnString returns the direct connection string to migrate, from exactly one of databaseURL and
// parameterName. Its errors never include the connection string.
func directConnString(ctx context.Context, databaseURL, parameterName string) (string, error) {
	switch {
	case databaseURL != "" && parameterName != "":
		return "", errors.New("set DATABASE_URL or DATABASE_URL_PARAMETER, not both")
	case databaseURL != "":
		config, err := pgx.ParseConfig(databaseURL)
		if err != nil {
			return "", errors.New("DATABASE_URL isn't a valid Postgres connection string")
		}
		if strings.Contains(config.Host, "-pooler.") {
			return "", errors.New("DATABASE_URL points at Neon's connection pooler; use the direct connection string")
		}
		return databaseURL, nil
	case parameterName != "":
		pooled, err := readParameter(ctx, parameterName)
		if err != nil {
			return "", fmt.Errorf("read the connection string from SSM parameter %q: %w", parameterName, err)
		}
		return migrate.DirectConnString(pooled)
	default:
		return "", errors.New("set DATABASE_URL, or DATABASE_URL_PARAMETER to read the connection string from SSM")
	}
}

// readParameter returns the decrypted value of the SSM parameter name.
func readParameter(ctx context.Context, name string) (string, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return "", err
	}
	out, err := ssm.NewFromConfig(cfg).GetParameter(ctx, &ssm.GetParameterInput{Name: aws.String(name), WithDecryption: aws.Bool(true)})
	if err != nil {
		return "", err
	}
	return aws.ToString(out.Parameter.Value), nil
}
