package postgres_test

import (
	"context"
	"maps"
	"slices"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
)

// withTags returns exampleRules fetched from cloneURL, with release/<n>'s tag object ID tagIDs[n-1].
func withTags(cloneURL string, tagIDs ...string) domain.Library {
	lib := exampleRules
	lib.Repository.CloneURL = cloneURL
	lib.Releases = slices.Clone(lib.Releases)
	for i := range lib.Releases {
		lib.Releases[i].TagID = tagIDs[i]
	}
	return lib
}

// The check reads back where ingestion last fetched a library and which tag each release had.
func TestCheckpointReadsWhatTheLastReplacementStored(t *testing.T) {
	s, _ := newStore(t)
	replace(t, s, withTags("https://github.com/example/rules.git", "a1", "b2", "c3"))
	replace(t, s, withTags("https://github.com/example/renamed.git", "a1", "b2", "d4"))

	checkpoint, found, err := s.Checkpoint(context.Background(), domain.LibraryKey{Host: domain.GitHub, RepositoryID: "7"})

	if err != nil || !found {
		t.Fatalf("found %v, %v", found, err)
	}
	want := domain.ReleaseTags{1: "a1", 2: "b2", 3: "d4"}
	if checkpoint.CloneURL != "https://github.com/example/renamed.git" || !maps.Equal(checkpoint.Tags, want) {
		t.Fatalf("read %+v, want the second replacement's clone URL and tags %v", checkpoint, want)
	}
}

// A release before the clone URL and tag IDs were stored left them NULL, which the check must see as unknown.
func TestCheckpointReadsUnrecordedValuesAsEmpty(t *testing.T) {
	s, _ := newStore(t)
	replace(t, s, exampleRules)

	checkpoint, found, err := s.Checkpoint(context.Background(), domain.LibraryKey{Host: domain.GitHub, RepositoryID: "7"})

	if err != nil || !found {
		t.Fatalf("found %v, %v", found, err)
	}
	if want := (domain.ReleaseTags{1: "", 2: "", 3: ""}); checkpoint.CloneURL != "" || !maps.Equal(checkpoint.Tags, want) {
		t.Fatalf("read %+v, want no clone URL and tags %v", checkpoint, want)
	}
}

// A release before every version kept its content stored only the current versions', so the check must see the
// library as missing content until ingestion stores it again.
func TestCheckpointReportsVersionsMissingTheirContent(t *testing.T) {
	s, connString := newStore(t)
	replace(t, s, exampleRules)
	key := domain.LibraryKey{Host: domain.GitHub, RepositoryID: "7"}
	if checkpoint, _, err := s.Checkpoint(context.Background(), key); err != nil || checkpoint.MissingContent {
		t.Fatalf("read %+v, %v; want no content missing", checkpoint, err)
	}
	lines(t, connString, `UPDATE rule_versions SET title = NULL, impact = NULL, impact_description = NULL, when_to_read = NULL,
		markdown = NULL WHERE html IS NULL RETURNING id::text`)

	checkpoint, _, err := s.Checkpoint(context.Background(), key)

	if err != nil || !checkpoint.MissingContent {
		t.Fatalf("read %+v, %v; want content missing", checkpoint, err)
	}
}

func TestCheckpointFindsNothingForALibraryNeverIngested(t *testing.T) {
	s, _ := newStore(t)
	replace(t, s, exampleRules)

	_, found, err := s.Checkpoint(context.Background(), domain.LibraryKey{Host: domain.GitHub, RepositoryID: "8"})

	if err != nil || found {
		t.Fatalf("found %v, %v; want nothing", found, err)
	}
}
