package git_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git/gittest"
)

// limits leave room for every test repository.
var limits = domain.FetchLimits{
	Tags: 10, TagBytes: 1 << 20, PackBytes: 128 << 20, Objects: 1_000, ObjectBytes: 32 << 20, TotalBytes: 128 << 20,
	ListedEntries: 10_000, ListDepth: 8,
}

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

// A release lists the files of a directory and the directories inside it, in path order, and nothing for a directory
// it doesn't have or a path that names a file.
func TestListReturnsTheFilesInADirectory(t *testing.T) {
	lib := gittest.NewLibrary(t)
	lib.Group("techs/go", "Go")
	lib.Rule("techs/go/return-errors", "Return errors", "Return errors instead of panicking.")
	lib.Write("techs/go/assets/return-errors/z.go", "package z\n")
	lib.Write("techs/go/assets/return-errors/deep/a.md", "# A\n")
	lib.Write("techs/go/assets/return-errors/b.svg", "<svg/>")
	lib.Release(1, firstRecord)
	releases, err := git.Fetch(context.Background(), lib.URL(), limits)
	if err != nil {
		t.Fatal(err)
	}
	files := releases[0].Files

	for dir, want := range map[string][]string{
		"techs/go/assets/return-errors/": {
			"techs/go/assets/return-errors/b.svg", "techs/go/assets/return-errors/deep/a.md", "techs/go/assets/return-errors/z.go",
		},
		"techs/go/assets/missing/":   nil,
		"techs/go/return-errors.md/": nil,
	} {
		got, err := files.List(dir, 3)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, want) {
			t.Errorf("List(%q) = %q, want %q", dir, got, want)
		}
	}
}

// A tree whose entries all point at one shared subtree lists a number of paths exponential in its depth from a few
// objects, so listing must stop once it's past the most the caller takes, rather than walk them all.
func TestListStopsPastMaxInATreeThatFansOut(t *testing.T) {
	const width, depth, max = 1_000, 3, 10
	lib := gittest.NewLibrary(t)
	repo, err := gogit.PlainOpen(lib.URL())
	if err != nil {
		t.Fatal(err)
	}
	// Each level holds width entries of the level below: files at the bottom, then directories, so the tree under
	// fan/ holds width^depth files.
	hash := storeObject(t, repo, plumbing.BlobObject, []byte("x"))
	mode := filemode.Regular
	for range depth {
		hash = storeTree(t, repo, fanOut(width, mode, hash))
		mode = filemode.Dir
	}
	root := storeTree(t, repo, []object.TreeEntry{{Name: "fan", Mode: filemode.Dir, Hash: hash}})
	lib.Tag("release/1", storeCommit(t, repo, root), "Library release 1.\n---\n"+firstRecord)
	releases, err := git.Fetch(context.Background(), lib.URL(), limits)
	if err != nil {
		t.Fatal(err)
	}

	paths, err := releases[0].Files.List("fan/", max)

	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != max+1 {
		t.Fatalf("listed %d paths, want %d, one past the most asked for", len(paths), max+1)
	}
}

// A tree whose directories share one subtree holding no file, only directories or submodules, reaches a number of
// entries exponential in its depth from a few objects, and listing finds no file past which to stop, so it must stop
// past the entries it visits instead, and refuse the directory, rather than walk them all.
func TestListRefusesATreeThatFansOutWithoutFiles(t *testing.T) {
	const width, depth = 2, 60
	for name, bottom := range map[string]func(*gogit.Repository) plumbing.Hash{
		"empty directories": func(repo *gogit.Repository) plumbing.Hash { return storeTree(t, repo, nil) },
		"submodules": func(repo *gogit.Repository) plumbing.Hash {
			return storeTree(t, repo, fanOut(width, filemode.Submodule, plumbing.NewHash(strings.Repeat("ab", 20))))
		},
	} {
		t.Run(name, func(t *testing.T) {
			lib := gittest.NewLibrary(t)
			repo, err := gogit.PlainOpen(lib.URL())
			if err != nil {
				t.Fatal(err)
			}
			hash := bottom(repo)
			for range depth {
				hash = storeTree(t, repo, fanOut(width, filemode.Dir, hash))
			}
			root := storeTree(t, repo, []object.TreeEntry{{Name: "fan", Mode: filemode.Dir, Hash: hash}})
			lib.Tag("release/1", storeCommit(t, repo, root), "Library release 1.\n---\n"+firstRecord)
			deep := limits
			deep.ListDepth = depth + 1 // only the entries visited stop it
			releases, err := git.Fetch(context.Background(), lib.URL(), deep)
			if err != nil {
				t.Fatal(err)
			}

			listed := make(chan error, 1)
			go func() {
				_, err := releases[0].Files.List("fan/", 10)
				listed <- err
			}()

			select {
			case err := <-listed:
				if err == nil || !strings.Contains(err.Error(), "entries") {
					t.Fatalf("got %v, want a refusal past the entries listing visits", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("still listing a tree of directories after 10 seconds")
			}
		})
	}
}

// A directory nested deeper than listing descends is refused, though it holds few entries.
func TestListRefusesADirectoryNestedDeeperThanItsLimit(t *testing.T) {
	lib := gittest.NewLibrary(t)
	lib.Write("shallow/"+strings.Repeat("d/", limits.ListDepth)+"a.md", "# A\n")
	lib.Write("deep/"+strings.Repeat("d/", limits.ListDepth+1)+"a.md", "# A\n")
	lib.Release(1, firstRecord)
	releases, err := git.Fetch(context.Background(), lib.URL(), limits)
	if err != nil {
		t.Fatal(err)
	}

	if paths, err := releases[0].Files.List("shallow/", 10); err != nil || len(paths) != 1 {
		t.Fatalf("at the limit: got %q, %v; want the one file", paths, err)
	}
	if _, err := releases[0].Files.List("deep/", 10); err == nil || !strings.Contains(err.Error(), "directories deep") {
		t.Fatalf("past the limit: got %v, want a refusal", err)
	}
}

// Listing stops once the fetch's context ends, since assembly lists files within the same ingestion.
func TestListStopsOnceTheFetchsContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	releases, err := git.Fetch(ctx, twoReleases(t).URL(), limits)
	if err != nil {
		t.Fatal(err)
	}

	cancel()
	_, err = releases[0].Files.List("techs/", 10)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want the context's end", err)
	}
}

// fanOut returns width entries of mode, in name order, each pointing at hash.
func fanOut(width int, mode filemode.FileMode, hash plumbing.Hash) []object.TreeEntry {
	entries := make([]object.TreeEntry, width)
	for i := range entries {
		entries[i] = object.TreeEntry{Name: fmt.Sprintf("e%04d", i), Mode: mode, Hash: hash}
	}
	return entries
}

// storeTree stores a tree of entries in repo and returns its hash.
func storeTree(t *testing.T, repo *gogit.Repository, entries []object.TreeEntry) plumbing.Hash {
	t.Helper()
	encoded := repo.Storer.NewEncodedObject()
	if err := (&object.Tree{Entries: entries}).Encode(encoded); err != nil {
		t.Fatal(err)
	}
	return storeEncoded(t, repo, encoded)
}

// storeCommit stores a commit of the tree root in repo and returns it.
func storeCommit(t *testing.T, repo *gogit.Repository, root plumbing.Hash) *object.Commit {
	t.Helper()
	author := object.Signature{Name: "Library Author", Email: "author@example.com", When: gittest.FirstTagged}
	encoded := repo.Storer.NewEncodedObject()
	commit := &object.Commit{Author: author, Committer: author, Message: "Fan out", TreeHash: root}
	if err := commit.Encode(encoded); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.CommitObject(storeEncoded(t, repo, encoded))
	if err != nil {
		t.Fatal(err)
	}
	return stored
}

// storeObject stores content as an object of type kind in repo and returns its hash.
func storeObject(t *testing.T, repo *gogit.Repository, kind plumbing.ObjectType, content []byte) plumbing.Hash {
	t.Helper()
	encoded := repo.Storer.NewEncodedObject()
	encoded.SetType(kind)
	writer, err := encoded.Writer()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return storeEncoded(t, repo, encoded)
}

func storeEncoded(t *testing.T, repo *gogit.Repository, encoded plumbing.EncodedObject) plumbing.Hash {
	t.Helper()
	hash, err := repo.Storer.SetEncodedObject(encoded)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}
