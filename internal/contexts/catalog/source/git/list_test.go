package git_test

import (
	"context"
	"maps"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git/gittest"
)

// The check compares what listing returns with what ingestion stored from Fetch, so both must name each release by
// the same tag object, and skip the same tags.
func TestListReleaseTagsNamesTheTagObjectsFetchReads(t *testing.T) {
	lib := twoReleases(t)
	head := lib.Commit("Work after the releases")
	lib.Tag("v1.0.0", head, "Not a library release.")
	lib.Tag("release/01", head, "Code Rules skips a number with a leading zero.")

	listed, err := git.ListReleaseTags(context.Background(), lib.URL(), limits)
	if err != nil {
		t.Fatal(err)
	}
	releases, err := git.Fetch(context.Background(), lib.URL(), limits)
	if err != nil {
		t.Fatal(err)
	}

	fetched := domain.ReleaseTags{}
	for _, r := range releases {
		if len(r.TagID) != 40 || r.TagID == r.CommitID {
			t.Errorf("release/%d's tag ID is %q, want the tag object's hash, not the commit's", r.Number, r.TagID)
		}
		fetched[r.Number] = r.TagID
	}
	if !maps.Equal(listed, fetched) || len(listed) != 2 {
		t.Fatalf("listed %v, fetched %v; want the same two releases", listed, fetched)
	}
}

// Rewriting a release's tag on the same commit can change its record, so it must change what listing returns.
func TestListReleaseTagsSeesARewrittenTag(t *testing.T) {
	lib := twoReleases(t)
	before, err := git.ListReleaseTags(context.Background(), lib.URL(), limits)
	if err != nil {
		t.Fatal(err)
	}
	lib.Retag("release/2", "Library release 2, rewritten.\n---\nformatVersion: 1\nrelease: 2\n")

	after, err := git.ListReleaseTags(context.Background(), lib.URL(), limits)

	if err != nil {
		t.Fatal(err)
	}
	if after[1] != before[1] || after[2] == before[2] || len(after) != 2 {
		t.Fatalf("listed %v before rewriting release/2 and %v after", before, after)
	}
}

func TestListReleaseTagsRefusesWhatFetchRefusesBeforeFetching(t *testing.T) {
	empty := gittest.NewLibrary(t)
	empty.Commit("Start the library")
	for name, tc := range map[string]struct {
		url    string
		limits domain.FetchLimits
		want   string
	}{
		"no release tags":       {empty.URL(), limits, "no release/<number> tags"},
		"more release tags":     {twoReleases(t).URL(), domain.FetchLimits{Tags: 1}, "2 release tags, more than the 1"},
		"an unreachable remote": {filepath.Join(t.TempDir(), "missing"), limits, "list the repository's references"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := git.ListReleaseTags(context.Background(), tc.url, tc.limits)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got error %v, want one containing %q", err, tc.want)
			}
		})
	}
}
