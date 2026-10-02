package domain

import (
	"errors"
	"time"

	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// ReleaseSnapshot is one library release as a source fetched it: its annotated release/<number> tag, the record
// the tag holds, and the files of the commit it tags.
type ReleaseSnapshot struct {
	Number int
	Tag    string
	// TagID is the hash of the annotated tag object, which holds the record.
	TagID    string
	TaggedAt time.Time
	// CommitID is the hash of the commit the release tags.
	CommitID string
	Record   coderules.ReleaseRecord
	// Files reads the tagged commit's files. Assembly reads only the ones it needs.
	Files Files
}

// release returns the release as the catalog stores it. Code Rules' release notes say a release updated shared
// files for every release after the first that lists library files; a first release lists every file, which it
// adds rather than updates.
func (s ReleaseSnapshot) release() Release {
	return Release{
		Number: s.Number, TagID: s.TagID, CommitID: s.CommitID, TaggedAt: s.TaggedAt,
		UpdatesSharedFiles: s.Number > 1 && len(s.Record.LibraryFiles) > 0,
	}
}

// Files are the files a release's commit holds, by path.
type Files interface {
	// Open returns the file at path without reading it, or ErrFileMissing when there's none.
	Open(path string) (File, error)
}

// File is one file of a release's commit, whose size is known before it's read.
type File interface {
	Size() int64
	Read() ([]byte, error)
}

// ErrFileMissing reports that a release's commit has no file at a path.
var ErrFileMissing = errors.New("the file doesn't exist")
