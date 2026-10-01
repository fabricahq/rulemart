// Package app holds the catalog's operations. Ingest builds the catalog from a Code Rules library's release/<number>
// tags: it fetches a library's repository into memory, reads every release record and the files the releases
// published, and replaces what the catalog stores about the library in one transaction: its releases, groups,
// rules, and every rule version, with the current version's content rendered for the web.
package app

import (
	"context"
	"fmt"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// Result summarizes one ingestion.
type Result struct {
	// Releases counts the library releases read, and Rules the current rules.
	Releases, Rules int
	// Changed counts the rows ingestion inserted, updated, or deleted. It's 0 when the catalog already matched the
	// tags.
	Changed int64
}

// Ingest makes the catalog's rows for repo's library match its release tags. It writes nothing when a tag, its
// record, the history the records describe, or a file a release published is invalid; errors name the tag and file.
// Running it again on unchanged tags changes nothing.
func Ingest(ctx context.Context, store *Store, repo domain.Repository) (Result, error) {
	return ingest(ctx, store, repo, defaultLimits)
}

// limits bounds the memory one ingestion uses.
type limits struct {
	fetch fetchLimits
	// contentBytes bounds the content ingestion reads and holds: every rule's Markdown and HTML, and every group's
	// metadata.
	contentBytes int64
}

// defaultLimits leave room for any real library: 256 MiB of content is tens of thousands of long rules.
var defaultLimits = limits{fetch: defaultFetchLimits, contentBytes: 256 << 20}

// ingest is Ingest within limits.
func ingest(ctx context.Context, store *Store, repo domain.Repository, limits limits) (Result, error) {
	lib, err := load(ctx, repo, limits)
	if err != nil {
		return Result{}, fmt.Errorf("ingest repository=%q: %v", repo.FullName(), err)
	}
	changed, err := store.replace(ctx, lib)
	if err != nil {
		return Result{}, fmt.Errorf("ingest repository=%q: %v", repo.FullName(), err)
	}
	current := 0
	for _, r := range lib.rules {
		if r.content != nil {
			current++
		}
	}
	return Result{Releases: len(lib.releases), Rules: current, Changed: changed}, nil
}

// load fetches repo's release tags and reads the library they publish, within limits.
func load(ctx context.Context, repo domain.Repository, limits limits) (library, error) {
	git, err := fetchReleaseTags(ctx, repo.CloneURL, limits.fetch)
	if err != nil {
		return library{}, err
	}
	releases, err := readReleases(git)
	if err != nil {
		return library{}, err
	}
	records := make([]coderules.ReleaseRecord, len(releases))
	for i, r := range releases {
		records[i] = r.record
	}
	histories, err := buildHistory(records)
	if err != nil {
		return library{}, err
	}
	return readLibrary(repo, releases, histories, limits.contentBytes)
}
