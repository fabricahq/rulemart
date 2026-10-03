// Package git fetches a library's release tags into memory with go-git, within limits, and returns each release
// as a snapshot: its tag, its record, and the tagged commit's files, which it reads only when asked. It's the only
// package that uses go-git.
package git

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// errNoReleases reports a repository without library releases.
var errNoReleases = errors.New("the repository has no release/<number> tags; publish a library release with Code Rules first")

// Fetch returns the release snapshots of the repository at url, in number order: one for each release/<number>
// tag, whose record it has parsed and whose files it reads when asked. url is any address go-git can fetch from,
// such as an HTTPS URL or, in tests, a local path. Errors name the tag at fault.
func Fetch(ctx context.Context, url string, limits domain.FetchLimits) ([]domain.ReleaseSnapshot, error) {
	repo, err := fetchReleaseTags(ctx, url, limits)
	if err != nil {
		return nil, err
	}
	return readReleases(ctx, repo, limits)
}

// ListReleaseTags returns the release/<number> tags the repository at url lists, with the IDs of the tag objects
// they point to, as Fetch reads them, without fetching any object: one request, as git ls-remote makes. It fails as
// Fetch does when the repository has no release tags, or more than limits allow. url is any address go-git can
// fetch from, such as an HTTPS URL or, in tests, a local path.
func ListReleaseTags(ctx context.Context, url string, limits domain.FetchLimits) (domain.ReleaseTags, error) {
	remote := gogit.NewRemote(nil, &config.RemoteConfig{Name: "origin", URLs: []string{url}})
	return listReleaseTags(withRefsLimit(ctx, limits.RefsBytes), remote, limits)
}

// fetchReleaseTags fetches the release/* tags of the repository at url, with their commits and trees but no other
// history, into memory that limits bound. url is any address go-git can fetch from, such as an HTTPS URL or, in tests, a local path. A
// repository without release tags, or with more than the limit, fails before anything is fetched.
func fetchReleaseTags(ctx context.Context, url string, limits domain.FetchLimits) (*gogit.Repository, error) {
	ctx = withRefsLimit(ctx, limits.RefsBytes)
	repo, err := gogit.Init(newBoundedStorage(limits), nil)
	if err != nil {
		return nil, fmt.Errorf("create in-memory repository: %v", err)
	}
	remote, err := repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{url}})
	if err != nil {
		return nil, fmt.Errorf("add remote: %v", err)
	}
	if _, err := listReleaseTags(ctx, remote, limits); err != nil {
		return nil, err
	}
	// Ingestion reads only the trees of tagged commits, so a shallow fetch leaves out every other commit's objects.
	err = remote.FetchContext(ctx, &gogit.FetchOptions{
		RefSpecs: []config.RefSpec{"+refs/tags/release/*:refs/tags/release/*"},
		Tags:     gogit.NoTags,
		Depth:    1,
	})
	var noMatch gogit.NoMatchingRefSpecError
	switch {
	case errors.As(err, &noMatch):
		return nil, errNoReleases
	case err != nil:
		return nil, fmt.Errorf("fetch release tags: %v", err)
	}
	return repo, nil
}

// listReleaseTags lists the remote's references and returns its release/<number> tags, the names Code Rules
// accepts, with the IDs they point to. It fails when there are none, or more than limits allow.
func listReleaseTags(ctx context.Context, remote *gogit.Remote, limits domain.FetchLimits) (domain.ReleaseTags, error) {
	refs, err := remote.ListContext(ctx, &gogit.ListOptions{})
	if errors.Is(err, transport.ErrEmptyRemoteRepository) {
		return nil, errNoReleases
	}
	if err != nil {
		return nil, fmt.Errorf("list the repository's references: %v", err)
	}
	tags := domain.ReleaseTags{}
	for _, ref := range refs {
		if !ref.Name().IsTag() {
			continue
		}
		if number, err := coderules.ParseReleaseTag(ref.Name().Short()); err == nil {
			tags[number] = ref.Hash().String()
		}
	}
	switch {
	case len(tags) == 0:
		return nil, errNoReleases
	case len(tags) > limits.Tags:
		return nil, fmt.Errorf("the repository has %d release tags, more than the %d ingestion reads", len(tags), limits.Tags)
	}
	return tags, nil
}

// readReleases parses the record of every release/<number> tag in repo, in number order, until ctx ends. Like Code
// Rules, it skips other names under release/, such as release/01, and fails when no tag remains. Each tag must be an
// annotated tag of a commit, no larger than limits.TagBytes, whose message is release notes followed by a record for
// that release.
func readReleases(ctx context.Context, repo *gogit.Repository, limits domain.FetchLimits) ([]domain.ReleaseSnapshot, error) {
	refs, err := repo.Tags()
	if err != nil {
		return nil, fmt.Errorf("list tags: %v", err)
	}
	var releases []domain.ReleaseSnapshot
	shared := &trees{storer: repo.Storer, decoded: map[plumbing.Hash]*object.Tree{}}
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().Short()
		if _, err := coderules.ParseReleaseTag(name); err != nil {
			return nil
		}
		// Parsing a record is the work here, so stop between records once ctx ends.
		if err := ctx.Err(); err != nil {
			return err
		}
		r, err := readRelease(repo, shared, name, ref.Hash(), limits)
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
	slices.SortFunc(releases, func(a, b domain.ReleaseSnapshot) int { return a.Number - b.Number })
	return releases, nil
}

// readRelease reads the annotated tag object hash, which the tag named name points to.
func readRelease(repo *gogit.Repository, shared *trees, name string, hash plumbing.Hash, limits domain.FetchLimits) (domain.ReleaseSnapshot, error) {
	encoded, err := repo.Storer.EncodedObject(plumbing.AnyObject, hash)
	if err != nil {
		return domain.ReleaseSnapshot{}, fmt.Errorf("load tag object: %v", err)
	}
	if encoded.Type() != plumbing.TagObject {
		return domain.ReleaseSnapshot{}, errors.New("the tag is a lightweight tag; a library release is an annotated tag")
	}
	if encoded.Size() > limits.TagBytes {
		return domain.ReleaseSnapshot{}, fmt.Errorf("the tag object is %d bytes, more than the %d a library release can have", encoded.Size(), limits.TagBytes)
	}
	raw, err := readObject(encoded)
	if err != nil {
		return domain.ReleaseSnapshot{}, err
	}
	_, record, err := coderules.ParseReleaseTagObject(name, raw)
	if err != nil {
		return domain.ReleaseSnapshot{}, fmt.Errorf("invalid release record: %v", err)
	}
	tag, err := object.DecodeTag(repo.Storer, encoded)
	if err != nil {
		return domain.ReleaseSnapshot{}, fmt.Errorf("decode tag object: %v", err)
	}
	if tag.TargetType != plumbing.CommitObject {
		return domain.ReleaseSnapshot{}, fmt.Errorf("the tag points to a %s; a library release tags a commit", tag.TargetType)
	}
	commit, err := tag.Commit()
	if err != nil {
		return domain.ReleaseSnapshot{}, fmt.Errorf("load the tagged commit %s: %v", tag.Target, err)
	}
	return domain.ReleaseSnapshot{
		Number: record.Release, Tag: name, TagID: hash.String(), TaggedAt: tag.Tagger.When, CommitID: commit.Hash.String(),
		Record: record,
		Files:  files{root: commit.TreeHash, trees: shared},
	}, nil
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

// files reads a tagged commit's files from the fetched objects in memory.
type files struct {
	// root is the commit's tree.
	root  plumbing.Hash
	trees *trees
}

// Open returns the file at path in the commit, or domain.ErrFileMissing when there's none.
func (f files) Open(path string) (domain.File, error) {
	tree, err := f.trees.get(f.root)
	if err != nil {
		return nil, fmt.Errorf("load tree: %v", err)
	}
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		entry, err := tree.FindEntry(segment)
		if errors.Is(err, object.ErrEntryNotFound) || errors.Is(err, object.ErrDirectoryNotFound) {
			return nil, domain.ErrFileMissing
		}
		if err != nil {
			return nil, fmt.Errorf("find file: %v", err)
		}
		if i == len(segments)-1 {
			if !entry.Mode.IsFile() {
				return nil, domain.ErrFileMissing
			}
			found, err := tree.TreeEntryFile(entry)
			if err != nil {
				return nil, fmt.Errorf("find file: %v", err)
			}
			return file{found}, nil
		}
		if entry.Mode != filemode.Dir {
			return nil, domain.ErrFileMissing
		}
		if tree, err = f.trees.get(entry.Hash); err != nil {
			return nil, fmt.Errorf("load tree: %v", err)
		}
	}
	return nil, domain.ErrFileMissing
}

// List returns the paths of the files in the directory dir, which ends with /, and in the directories inside it, in
// path order, or none when the commit has no such directory.
func (f files) List(dir string) ([]string, error) {
	tree, err := f.trees.get(f.root)
	if err != nil {
		return nil, fmt.Errorf("load tree: %v", err)
	}
	for segment := range strings.SplitSeq(strings.TrimSuffix(dir, "/"), "/") {
		entry, err := tree.FindEntry(segment)
		if errors.Is(err, object.ErrEntryNotFound) || errors.Is(err, object.ErrDirectoryNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("find directory: %v", err)
		}
		if entry.Mode != filemode.Dir {
			return nil, nil
		}
		if tree, err = f.trees.get(entry.Hash); err != nil {
			return nil, fmt.Errorf("load tree: %v", err)
		}
	}
	var paths []string
	if err := f.walk(tree, dir, &paths); err != nil {
		return nil, err
	}
	slices.Sort(paths)
	return paths, nil
}

// walk adds the path of every file in tree, whose path is prefix, and in the trees inside it, to paths.
func (f files) walk(tree *object.Tree, prefix string, paths *[]string) error {
	for _, entry := range tree.Entries {
		switch {
		case entry.Mode == filemode.Dir:
			inner, err := f.trees.get(entry.Hash)
			if err != nil {
				return fmt.Errorf("load tree: %v", err)
			}
			if err := f.walk(inner, prefix+entry.Name+"/", paths); err != nil {
				return err
			}
		case entry.Mode.IsFile():
			*paths = append(*paths, prefix+entry.Name)
		}
	}
	return nil
}

// trees decodes each tree object of one fetch once, however many releases or paths reach it. Releases share most of
// their trees, so decoding them per release would hold a large shared directory once for every release that reads
// it. The objects a fetch holds are bounded, so the trees decoded from them are too. It isn't safe for concurrent
// use.
type trees struct {
	storer  storer.EncodedObjectStorer
	decoded map[plumbing.Hash]*object.Tree
}

// get returns the tree object hash, decoding it the first time it's asked for.
func (t *trees) get(hash plumbing.Hash) (*object.Tree, error) {
	if tree, ok := t.decoded[hash]; ok {
		return tree, nil
	}
	tree, err := object.GetTree(t.storer, hash)
	if err != nil {
		return nil, err
	}
	t.decoded[hash] = tree
	return tree, nil
}

// file is one file of a commit, read only when asked.
type file struct {
	*object.File
}

func (f file) Size() int64 { return f.File.Size }

func (f file) Read() ([]byte, error) {
	reader, err := f.Reader()
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
