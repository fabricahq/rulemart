package git_test

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git/gittest"
)

// Releases share most of their trees, and assembly reads the same paths from many of them, so a large directory
// they share must be held once, not once per release: otherwise a library of many small releases that share one large
// directory makes a fetch within its limits hold gigabytes.
func TestFetchHoldsADirectoryTheReleasesShareOnce(t *testing.T) {
	const releases, entries = 40, 2_000
	lib := gittest.NewLibrary(t)
	lib.Rule("techs/go/return-errors", "Return errors", "Return errors instead of panicking.")
	for i := range entries {
		lib.Write(fmt.Sprintf("techs/go/%s-%04d.txt", strings.Repeat("long-name-", 10), i), "x")
	}
	lib.Release(1, firstRecord)
	for n := 2; n <= releases; n++ {
		lib.Write("notes.txt", fmt.Sprintf("release %d", n))
		lib.Release(n, fmt.Sprintf("formatVersion: 1\nrelease: %d\nrules: {techs/go/return-errors: 1.0.0}\n", n))
	}
	bounded := limits
	bounded.Tags = releases
	snapshots, err := git.Fetch(context.Background(), lib.URL(), bounded)
	if err != nil {
		t.Fatal(err)
	}
	before := heapInUse()

	for _, s := range snapshots {
		if _, err := s.Files.Open("techs/go/return-errors.md"); err != nil {
			t.Fatal(err)
		}
	}

	grown := heapInUse() - before
	// One decoded copy of techs/go holds about 300 KB; a copy per release would hold about 12 MB.
	if grown > 4<<20 {
		t.Fatalf("reading one file from each of %d releases kept %d more bytes", releases, grown)
	}
	runtime.KeepAlive(snapshots)
}

// heapInUse returns the bytes the heap holds after a collection.
func heapInUse() int64 {
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return int64(stats.HeapAlloc)
}
