// Package ingest builds the catalog from a Code Rules library's release/<number> tags. It fetches a library's
// repository into memory, reads every release record and the files the releases published, and replaces what the
// catalog stores about the library in one transaction: its releases, groups, rules, and every rule version, with
// the current version's content rendered for the web.
package ingest

import (
	"context"
	"fmt"

	"github.com/fabricahq/rulemart/third_party/coderules"
)

// Repository is a library's GitHub repository, as GitHub describes it.
type Repository struct {
	// ID is GitHub's repository ID, which identifies the library across renames and transfers.
	ID          int64
	Owner, Name string
	Description string
	// OwnerAvatarURL is the owner's avatar on GitHub's avatar host; empty when unknown.
	OwnerAvatarURL string
	// CloneURL is where ingestion fetches the release tags, such as https://github.com/owner/name.git.
	CloneURL string
}

// FullName returns the repository's owner/name.
func (r Repository) FullName() string { return r.Owner + "/" + r.Name }

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
func Ingest(ctx context.Context, store *Store, repo Repository) (Result, error) {
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
func ingest(ctx context.Context, store *Store, repo Repository, limits limits) (Result, error) {
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
func load(ctx context.Context, repo Repository, limits limits) (library, error) {
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
