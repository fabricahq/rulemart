// Fetch a library's release tags into memory with go-git, and read their records and the files they tag.

package ingest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"

	"github.com/fabricahq/rulemart/third_party/coderules"
)

// errNoReleases reports a repository without library releases.
var errNoReleases = errors.New("the repository has no release/<number> tags; publish a library release with Code Rules first")

// maxTagBytes bounds one release tag object, its message and any signature included, as Code Rules bounds the tags
// it publishes.
const maxTagBytes = 8 << 20

// maxFileBytes bounds each rule, group, or manifest file ingestion reads.
const maxFileBytes = 1 << 20

// release is one library release, as its annotated release/<number> tag records it.
type release struct {
	tag      string
	commit   *object.Commit
	taggedAt time.Time
	record   coderules.ReleaseRecord
}

// updatesSharedFiles reports whether the release changed library-wide files, such as group metadata or shared
// assets, which Code Rules' release notes say for every release after the first that lists library files. A first
// release lists every file, which it adds rather than updates.
func (r release) updatesSharedFiles() bool {
	return r.record.Release > 1 && len(r.record.LibraryFiles) > 0
}

// fetchReleaseTags fetches the release/* tags of the repository at url, with their commits and trees but no other
// history, into memory that limits bound. url is any address go-git can fetch from, such as an HTTPS URL or, in tests, a local path. A
// repository without release tags, or with more than the limit, fails before anything is fetched.
func fetchReleaseTags(ctx context.Context, url string, limits fetchLimits) (*git.Repository, error) {
	repo, err := git.Init(newBoundedStorage(limits), nil)
	if err != nil {
		return nil, fmt.Errorf("create in-memory repository: %v", err)
	}
	remote, err := repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{url}})
	if err != nil {
		return nil, fmt.Errorf("add remote: %v", err)
	}
	if err := checkReleaseTagCount(ctx, remote, limits); err != nil {
		return nil, err
	}
	// Ingestion reads only the trees of tagged commits, so a shallow fetch leaves out every other commit's objects.
	err = remote.FetchContext(ctx, &git.FetchOptions{
		RefSpecs: []config.RefSpec{"+refs/tags/release/*:refs/tags/release/*"},
		Tags:     git.NoTags,
		Depth:    1,
	})
	var noMatch git.NoMatchingRefSpecError
	switch {
	case errors.As(err, &noMatch):
		return nil, errNoReleases
	case err != nil:
		return nil, fmt.Errorf("fetch release tags: %v", err)
	}
	return repo, nil
}

// checkReleaseTagCount lists the remote's references and fails when it has no release/<number> tags, or more than
// limits allow.
func checkReleaseTagCount(ctx context.Context, remote *git.Remote, limits fetchLimits) error {
	refs, err := remote.ListContext(ctx, &git.ListOptions{})
	if errors.Is(err, transport.ErrEmptyRemoteRepository) {
		return errNoReleases
	}
	if err != nil {
		return fmt.Errorf("list the repository's references: %v", err)
	}
	count := 0
	for _, ref := range refs {
		if _, err := coderules.ParseReleaseTag(ref.Name().Short()); ref.Name().IsTag() && err == nil {
			count++
		}
	}
	switch {
	case count == 0:
		return errNoReleases
	case count > limits.tags:
		return fmt.Errorf("the repository has %d release tags, more than the %d ingestion reads", count, limits.tags)
	}
	return nil
}

// readReleases parses the record of every release/<number> tag in repo, in number order. Like Code Rules, it skips
// other names under release/, such as release/01, and fails when no tag remains. Each tag must be an annotated tag
// of a commit, no larger than maxTagBytes, whose message is release notes followed by a record for that release.
func readReleases(repo *git.Repository) ([]release, error) {
	refs, err := repo.Tags()
	if err != nil {
		return nil, fmt.Errorf("list tags: %v", err)
	}
	var releases []release
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().Short()
		if _, err := coderules.ParseReleaseTag(name); err != nil {
			return nil
		}
		r, err := readRelease(repo, name, ref.Hash())
		if err != nil {
			return fmt.Errorf("read %s: %v", name, err)
		}
		releases = append(releases, r)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, errNoReleases
	}
	slices.SortFunc(releases, func(a, b release) int { return a.record.Release - b.record.Release })
	return releases, nil
}

// readRelease reads the annotated tag object hash, which the tag named name points to.
func readRelease(repo *git.Repository, name string, hash plumbing.Hash) (release, error) {
	encoded, err := repo.Storer.EncodedObject(plumbing.AnyObject, hash)
	if err != nil {
		return release{}, fmt.Errorf("load tag object: %v", err)
	}
	if encoded.Type() != plumbing.TagObject {
		return release{}, errors.New("the tag is a lightweight tag; a library release is an annotated tag")
	}
	if encoded.Size() > maxTagBytes {
		return release{}, fmt.Errorf("the tag object is %d bytes, more than the %d a library release can have", encoded.Size(), maxTagBytes)
	}
	raw, err := readObject(encoded)
	if err != nil {
		return release{}, err
	}
	_, record, err := coderules.ParseReleaseTagObject(name, raw)
	if err != nil {
		return release{}, fmt.Errorf("invalid release record: %v", err)
	}
	tag, err := object.DecodeTag(repo.Storer, encoded)
	if err != nil {
		return release{}, fmt.Errorf("decode tag object: %v", err)
	}
	if tag.TargetType != plumbing.CommitObject {
		return release{}, fmt.Errorf("the tag points to a %s; a library release tags a commit", tag.TargetType)
	}
	commit, err := tag.Commit()
	if err != nil {
		return release{}, fmt.Errorf("load the tagged commit %s: %v", tag.Target, err)
	}
	return release{tag: name, commit: commit, taggedAt: tag.Tagger.When, record: record}, nil
}

// readObject returns an object's content, as git cat-file -p prints it.
func readObject(encoded plumbing.EncodedObject) ([]byte, error) {
	reader, err := encoded.Reader()
	if err != nil {
		return nil, fmt.Errorf("open object: %v", err)
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read object: %v", err)
	}
	return content, nil
}

// errFileMissing reports that a tagged commit has no file at a path.
var errFileMissing = errors.New("the file doesn't exist")

// readFile returns the content of the file at path in commit. It fails with errFileMissing when there's none, and
// refuses a file larger than maxFileBytes. admit, when it isn't nil, is given the file's size before anything is
// read, and its error refuses the file.
func readFile(commit *object.Commit, path string, admit func(size int64) error) ([]byte, error) {
	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("load tree: %v", err)
	}
	file, err := tree.File(path)
	if errors.Is(err, object.ErrFileNotFound) || errors.Is(err, object.ErrDirectoryNotFound) || errors.Is(err, object.ErrEntryNotFound) {
		return nil, errFileMissing
	}
	if err != nil {
		return nil, fmt.Errorf("find file: %v", err)
	}
	if file.Size > maxFileBytes {
		return nil, fmt.Errorf("the file is %d bytes, more than the %d ingestion reads", file.Size, maxFileBytes)
	}
	if admit != nil {
		if err := admit(file.Size); err != nil {
			return nil, err
		}
	}
	reader, err := file.Reader()
	if err != nil {
		return nil, fmt.Errorf("open file: %v", err)
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read file: %v", err)
	}
	return content, nil
}
