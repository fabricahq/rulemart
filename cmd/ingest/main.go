// Command ingest stores what a Code Rules library's release tags published in Rulemart's catalog:
//
//	ingest https://github.com/<owner>/<repository>
//
// It looks the repository up on GitHub, reads every release/<number> tag, and replaces the catalog's rows for the
// library in one transaction. Running it again on unchanged tags changes nothing. Operators run it; pages show only
// the libraries catalog/vetted.yaml lists.
//
// Set DATABASE_URL to a connection string, or DATABASE_URL_PARAMETER to the SSM parameter holding one. GITHUB_TOKEN,
// when set, authenticates the GitHub lookup.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/github"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres"
	"github.com/fabricahq/rulemart/internal/platform/database"
	"github.com/fabricahq/rulemart/internal/platform/migrate"
)

func main() {
	log.SetFlags(0)
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: ingest https://github.com/<owner>/<repository>")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if err := run(ctx, os.Args[1]); err != nil {
		log.Fatal(err)
	}
}

// run ingests the library at repositoryURL and reports the result.
func run(ctx context.Context, repositoryURL string) error {
	source, err := database.SourceFromEnv(ctx, os.Getenv)
	if err != nil {
		return err
	}
	schemaVersion, err := migrate.RequiredVersion()
	if err != nil {
		return err
	}
	db := source.Open(schemaVersion)
	defer db.Close()
	ingester := app.Ingester{
		Repositories: github.Client{Client: &http.Client{Timeout: 30 * time.Second}, BaseURL: "https://api.github.com", Token: os.Getenv("GITHUB_TOKEN")},
		Fetch:        git.Fetch,
		Store:        postgres.New(db),
		Limits:       domain.DefaultLimits,
	}
	result, err := ingester.Ingest(ctx, repositoryURL)
	if err != nil {
		return err
	}
	log.Print(summary(result))
	return nil
}

// summary describes what an ingestion did.
func summary(result app.Result) string {
	repo := result.Repository
	return fmt.Sprintf("ingested %s (%s repository %s): %s, %s, %s", repo.FullName(), repo.Host, repo.ID,
		count(int64(result.Releases), "library release", "library releases"),
		count(int64(result.Rules), "current rule", "current rules"),
		count(result.Changed, "row changed", "rows changed"))
}

// count writes n with the noun phrase that agrees with it, such as "1 current rule" or "2 current rules".
func count(n int64, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.FormatInt(n, 10) + " " + many
}
