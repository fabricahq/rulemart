// Command ingest stores what a Code Rules library's release tags published in Rulemart's catalog:
//
//	ingest https://github.com/<owner>/<repository>
//
// It looks the repository up on GitHub, reads every release/<number> tag, and replaces the catalog's rows for the
// library in one transaction. Running it again on unchanged tags changes nothing. Operators run it to backfill a
// library, vetted or not; the worker function keeps vetted libraries current, and pages show only the libraries
// catalog/vetted.yaml lists.
//
// Set DATABASE_URL to a connection string, or DATABASE_URL_PARAMETER to the SSM parameter holding one, such as the
// worker's, whose login can write the catalog and nothing else. GITHUB_TOKEN, when set, authenticates the GitHub
// lookup. LOG_LEVEL and RULEMART_RELEASE configure its logs, as internal/platform/logging describes.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/render"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/github"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres"
	"github.com/fabricahq/rulemart/internal/platform/database"
	"github.com/fabricahq/rulemart/internal/platform/database/migrate"
	"github.com/fabricahq/rulemart/internal/platform/logging"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: ingest https://github.com/<owner>/<repository>")
		os.Exit(2)
	}
	logger, err := logging.New(os.Stdout, os.Getenv)
	if err != nil {
		logging.StartupFailed(logger, err)
		os.Exit(1)
	}
	// The log package then writes JSON lines through logger too.
	slog.SetDefault(logger)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	result, err := run(ctx, os.Args[1])
	if err != nil {
		logger.Error("ingest failed", "error", err.Error())
		os.Exit(1)
	}
	repo := result.Repository
	logger.Info("ingested", "library", repo.FullName(), "host", repo.Host, "repository", repo.ID,
		"releases", result.Releases, "rules", result.Rules, "changed", result.Changed)
}

// run ingests the library at repositoryURL and returns what it did. It checks the URL and looks the repository up
// on GitHub before it reads the database settings, so a mistyped URL is reported as one.
func run(ctx context.Context, repositoryURL string) (app.Result, error) {
	ingester := app.Ingester{
		Repositories: github.Client{Client: &http.Client{Timeout: 30 * time.Second}, BaseURL: "https://api.github.com", Token: os.Getenv("GITHUB_TOKEN")},
		Fetch:        git.Fetch,
		Render:       render.Rule,
		Limits:       domain.DefaultLimits,
	}
	repo, err := ingester.Resolve(ctx, repositoryURL)
	if err != nil {
		return app.Result{}, err
	}
	source, err := database.SourceFromEnv(ctx, os.Getenv)
	if err != nil {
		return app.Result{}, err
	}
	schemaVersion, err := migrate.RequiredVersion()
	if err != nil {
		return app.Result{}, err
	}
	db := source.Open(schemaVersion)
	defer db.Close()
	ingester.Store = postgres.New(db)
	return ingester.IngestRepository(ctx, repo)
}
