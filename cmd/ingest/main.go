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
	"time"

	"github.com/fabricahq/rulemart/internal/database"
	"github.com/fabricahq/rulemart/internal/ingest"
	"github.com/fabricahq/rulemart/internal/migrate"
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
	owner, name, err := ingest.ParseRepositoryURL(repositoryURL)
	if err != nil {
		return err
	}
	github := ingest.GitHub{Client: &http.Client{Timeout: 30 * time.Second}, BaseURL: "https://api.github.com", Token: os.Getenv("GITHUB_TOKEN")}
	repo, err := github.Repository(ctx, owner, name)
	if err != nil {
		return err
	}
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
	result, err := ingest.Ingest(ctx, ingest.NewStore(db), repo)
	if err != nil {
		return err
	}
	log.Printf("ingested %s (GitHub repository ID %d): %d library releases, %d current rules, %d rows changed",
		repo.FullName(), repo.ID, result.Releases, result.Rules, result.Changed)
	return nil
}
