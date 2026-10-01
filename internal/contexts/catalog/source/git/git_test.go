package git_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git/gittest"
)

// limits leave room for every test repository.
var limits = git.Limits{Tags: 10, TagBytes: 1 << 20, PackBytes: 128 << 20, Objects: 1_000, ObjectBytes: 32 << 20, TotalBytes: 128 << 20}

const firstRecord = `formatVersion: 1
release: 1
rules: {techs/go/return-errors: 1.0.0}
changes: {techs/go/return-errors: {change: new, summaries: [Add the rule.]}}
`

// twoReleases returns a library whose release/1 publishes a rule, and whose release/2 changes it.
func twoReleases(t *testing.T) *gittest.Library {
	t.Helper()
	lib := gittest.NewLibrary(t)
	lib.Group("techs/go", "Go")
	lib.Rule("techs/go/return-errors", "Return errors", "Return errors instead of panicking.")
	lib.Release(1, firstRecord)
	lib.Rule("techs/go/return-errors", "Return errors", "Return errors to the caller.")
	lib.Release(2, `formatVersion: 1
release: 2
rules: {techs/go/return-errors: 1.0.1}
changes: {techs/go/return-errors: {change: patch, from: 1.0.0, summaries: [Fix a typo.]}}
`)
	return lib
}

func TestFetchReturnsEachReleaseWithItsRecordAndFiles(t *testing.T) {
	lib := twoReleases(t)

	releases, err := git.Fetch(context.Background(), lib.URL(), limits)

	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 2 {
		t.Fatalf("fetched %d releases, want 2", len(releases))
	}
	for i, r := range releases {
		if r.Number != i+1 || r.Tag != domain.ReleaseTag(i+1) || r.Record.Release != i+1 || len(r.CommitID) != 40 ||
			!r.TaggedAt.Equal(gittest.FirstTagged.AddDate(0, 0, i)) {
			t.Errorf("release %d is %+v", i+1, r)
		}
	}
	if releases[0].CommitID == releases[1].CommitID {
		t.Error("both releases tag one commit")
	}
	// Each release reads its own commit's files.
	for i, want := range []string{"instead of panicking", "to the caller"} {
		file, err := releases[i].Files.Open("techs/go/return-errors.md")
		if err != nil {
			t.Fatal(err)
		}
		content, err := file.Read()
		if err != nil || int64(len(content)) != file.Size() || !strings.Contains(string(content), want) {
			t.Errorf("release/%d's rule is %d bytes, %q, %v; want %d bytes with %q", i+1, len(content), content, err, file.Size(), want)
		}
	}
	if _, err := releases[0].Files.Open("techs/go/missing.md"); !errors.Is(err, domain.ErrFileMissing) {
		t.Errorf("opened a missing file: %v", err)
	}
}

func TestFetchRefusesATagThatIsntALibraryRelease(t *testing.T) {
	for name, tc := range map[string]struct {
		tag  func(lib *gittest.Library)
		want string
	}{
		"a record that isn't YAML": {
			tag:  func(lib *gittest.Library) { lib.Release(2, "formatVersion: 1\nrelease: [2\n") },
			want: "read release/2: invalid release record",
		},
		"a record for another release": {
			tag:  func(lib *gittest.Library) { lib.Release(2, firstRecord) },
			want: "read release/2: invalid release record",
		},
		"a tag object too large": {
			tag: func(lib *gittest.Library) {
				lib.Release(2, strings.Replace(firstRecord, "release: 1", "release: 2", 1)+"# "+strings.Repeat("x", 2<<20)+"\n")
			},
			want: "read release/2: the tag object is",
		},
	} {
		t.Run(name, func(t *testing.T) {
			lib := gittest.NewLibrary(t)
			lib.Release(1, firstRecord)
			tc.tag(lib)

			_, err := git.Fetch(context.Background(), lib.URL(), limits)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got error %v, want one containing %q", err, tc.want)
			}
		})
	}
}

func TestFetchRefusesARepositoryWithoutReleases(t *testing.T) {
	lib := gittest.NewLibrary(t)
	lib.Commit("Start the library")

	_, err := git.Fetch(context.Background(), lib.URL(), limits)

	if err == nil || !strings.Contains(err.Error(), "no release/<number> tags") {
		t.Fatalf("got error %v, want one saying the repository has no release tags", err)
	}
}

// Assembly reads only the trees at tagged commits, so history it doesn't read, even an object past the limits,
// isn't fetched.
func TestFetchFetchesOnlyTheTaggedCommits(t *testing.T) {
	lib := gittest.NewLibrary(t)
	lib.Write("assets/huge.bin", strings.Repeat("\x00", 40<<20))
	lib.Commit("Add a large file")
	lib.Remove("assets/huge.bin")
	lib.Commit("Remove the large file")
	lib.Release(1, firstRecord)

	releases, err := git.Fetch(context.Background(), lib.URL(), limits)

	if err != nil || len(releases) != 1 {
		t.Fatalf("fetched %d releases, %v; want 1", len(releases), err)
	}
}
