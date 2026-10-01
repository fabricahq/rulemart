package ingest

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// taggedRepository commits files into a new repository on disk, tags the commit release/1 through release/<tags>,
// and returns the repository's path.
func taggedRepository(t *testing.T, files map[string][]byte, tags int) string {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := worktree.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		t.Fatal(err)
	}
	author := &object.Signature{Name: "Author", Email: "author@example.com", When: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	commit, err := worktree.Commit("Add files", &git.CommitOptions{Author: author, AllowEmptyCommits: true})
	if err != nil {
		t.Fatal(err)
	}
	for n := 1; n <= tags; n++ {
		if _, err := repo.CreateTag("release/"+strconv.Itoa(n), commit, &git.CreateTagOptions{Tagger: author, Message: "Release."}); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return b
}

// small are limits a test can pass with a few small files.
var small = fetchLimits{tags: 10, packBytes: 1 << 20, objects: 100, objectBytes: 64 << 10, totalBytes: 256 << 10}

func TestFetchReleaseTagsStaysWithinItsLimits(t *testing.T) {
	for name, tc := range map[string]struct {
		files  map[string][]byte
		tags   int
		limits func(fetchLimits) fetchLimits
		want   string
	}{
		"too many release tags": {
			tags:   3,
			limits: func(l fetchLimits) fetchLimits { l.tags = 2; return l },
			want:   "3 release tags, more than the 2",
		},
		"a pack too large": {
			files:  map[string][]byte{"noise.bin": randomBytes(t, 48<<10)},
			tags:   1,
			limits: func(l fetchLimits) fetchLimits { l.packBytes = 16 << 10; return l },
			want:   "more than 16384 bytes",
		},
		"too many objects": {
			files:  map[string][]byte{"a": []byte("a"), "b": []byte("b"), "c": []byte("c"), "d": []byte("d")},
			tags:   1,
			limits: func(l fetchLimits) fetchLimits { l.objects = 4; return l },
			want:   "more than 4 objects",
		},
		"an object too large": {
			files:  map[string][]byte{"zeros.bin": make([]byte, 128<<10)},
			tags:   1,
			limits: func(l fetchLimits) fetchLimits { return l },
			want:   "131072-byte object",
		},
		"too many bytes": {
			files: map[string][]byte{
				"a.bin": randomBytes(t, 40<<10), "b.bin": randomBytes(t, 40<<10), "c.bin": randomBytes(t, 40<<10),
			},
			tags:   1,
			limits: func(l fetchLimits) fetchLimits { l.totalBytes = 100 << 10; return l },
			want:   "more than 102400 bytes of objects",
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := taggedRepository(t, tc.files, tc.tags)

			_, err := fetchReleaseTags(context.Background(), dir, tc.limits(small))

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got error %v, want one containing %q", err, tc.want)
			}
		})
	}
}

// Two similar files make Git send one as a delta of the other, whose size checkPack reads from the delta.
func TestFetchReleaseTagsFetchesARepositoryWithinItsLimits(t *testing.T) {
	base := randomBytes(t, 40<<10)
	similar := append(append([]byte{}, base...), "changed"...)
	dir := taggedRepository(t, map[string][]byte{"a.md": []byte("a"), "base.bin": base, "similar.bin": similar}, 3)

	repo, err := fetchReleaseTags(context.Background(), dir, small)

	if err != nil {
		t.Fatal(err)
	}
	tags, err := countTags(repo)
	if err != nil || tags != 3 {
		t.Fatalf("fetched %d release tags, %v; want 3", tags, err)
	}
}

// countTags counts the fetched tags.
func countTags(repo *git.Repository) (int, error) {
	tags, err := repo.Tags()
	if err != nil {
		return 0, err
	}
	count := 0
	err = tags.ForEach(func(*plumbing.Reference) error { count++; return nil })
	return count, err
}

func TestDeltaTargetSizeReadsTheSecondSize(t *testing.T) {
	for name, tc := range map[string]struct {
		delta []byte
		want  int64
		ok    bool
	}{
		"one-byte sizes":         {[]byte{0x05, 0x07, 0x90}, 7, true},
		"a multi-byte target":    {[]byte{0x05, 0x80, 0x80, 0x02}, 2 << 14, true},
		"a multi-byte source":    {[]byte{0xff, 0x01, 0x10}, 16, true},
		"a truncated target":     {[]byte{0x05, 0x80}, 0, false},
		"no sizes":               {nil, 0, false},
		"a size too long to fit": {append([]byte{0x01}, []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x01}...), 0, false},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := deltaTargetSize(tc.delta)
			if (err == nil) != tc.ok || got != tc.want {
				t.Fatalf("got %d, %v; want %d, ok=%v", got, err, tc.want, tc.ok)
			}
		})
	}
}
