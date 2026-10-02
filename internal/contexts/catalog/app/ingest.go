// Package app holds the catalog's operations: Ingester ingests a library, or updates one whose release tags changed,
// and Pages reads what the pages show.
//
// Ingest builds the catalog from a Code Rules library's release/<number> tags: it looks the library's repository
// up on its code host, fetches the release tags, assembles the library they publish, and replaces what the catalog
// stores about it in one transaction: its releases, groups, rules, and every rule version, with the current
// version's content rendered for the web.
package app

import (
	"context"
	"fmt"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store"
)

// Repositories looks repositories up on their code host.
type Repositories interface {
	// Repository returns the repository owner/name as the host describes it now.
	Repository(ctx context.Context, owner, name string) (domain.Repository, error)
	// RepositoryByID returns the repository whose ID on the host is id, as the host describes it now.
	RepositoryByID(ctx context.Context, id string) (domain.Repository, error)
}

// Fetch fetches the release snapshots of the repository at url within limits, as git.Fetch does.
type Fetch func(ctx context.Context, url string, limits domain.FetchLimits) ([]domain.ReleaseSnapshot, error)

// List lists the release tags of the repository at url within limits, without fetching them, as
// git.ListReleaseTags does.
type List func(ctx context.Context, url string, limits domain.FetchLimits) (domain.ReleaseTags, error)

// Ingester ingests libraries into the catalog. It takes its source of release snapshots as Fetch and List, and its
// Markdown renderer as Render, so the functions that only read pages carry neither a Git client nor a renderer.
type Ingester struct {
	Repositories Repositories
	Fetch        Fetch
	// List is needed only by Update.
	List List
	// Render renders a current rule's Markdown body, as render.Rule does.
	Render domain.Render
	Store  store.Writer
	Limits domain.Limits
}

// Result summarizes one ingestion.
type Result struct {
	// Repository is the library's repository, as its host describes it.
	Repository domain.Repository
	// Releases counts the library releases read, and Rules the current rules.
	Releases, Rules int
	// Changed counts the rows ingestion inserted, updated, or deleted. It's 0 when the catalog already matched the
	// tags.
	Changed int64
}

// Ingest makes the catalog's rows for the library at repositoryURL, such as https://github.com/owner/name, match
// its release tags: it resolves the repository, then ingests it with IngestRepository.
func (in Ingester) Ingest(ctx context.Context, repositoryURL string) (Result, error) {
	repo, err := in.Resolve(ctx, repositoryURL)
	if err != nil {
		return Result{}, err
	}
	return in.IngestRepository(ctx, repo)
}

// Resolve checks that repositoryURL names a GitHub repository, and looks the repository up on its host. It needs
// no Store.
func (in Ingester) Resolve(ctx context.Context, repositoryURL string) (domain.Repository, error) {
	owner, name, err := domain.ParseRepositoryURL(repositoryURL)
	if err != nil {
		return domain.Repository{}, err
	}
	return in.Repositories.Repository(ctx, owner, name)
}

// untilDone returns render, refusing to render once ctx ends. Rendering is most of an ingestion's work, so a job past
// its deadline stops between rules rather than running on until its function is stopped.
func untilDone(ctx context.Context, render domain.Render) domain.Render {
	return func(body string, page domain.RulePage, allowance int64) (string, int64, error) {
		if err := ctx.Err(); err != nil {
			return "", 0, err
		}
		return render(body, page, allowance)
	}
}

// IngestRepository makes the catalog's rows for the library in repo match its release tags. It writes nothing when
// a tag, its record, the history the records describe, or a file a release published is invalid, or when the
// library passes in.Limits; errors name the tag and file. Running it again on unchanged tags changes nothing.
func (in Ingester) IngestRepository(ctx context.Context, repo domain.Repository) (Result, error) {
	releases, err := in.Fetch(ctx, repo.CloneURL, in.Limits.Fetch)
	if err != nil {
		return Result{}, fmt.Errorf("ingest repository=%q: %v", repo.FullName(), err)
	}
	lib, err := domain.Assemble(repo, releases, in.Limits.Content, untilDone(ctx, in.Render))
	if err != nil {
		return Result{}, fmt.Errorf("ingest repository=%q: %v", repo.FullName(), err)
	}
	changed, err := in.Store.ReplaceLibrary(ctx, lib)
	if err != nil {
		return Result{}, fmt.Errorf("ingest repository=%q: %v", repo.FullName(), err)
	}
	return Result{Repository: repo, Releases: len(lib.Releases), Rules: lib.CurrentRules(), Changed: changed}, nil
}
