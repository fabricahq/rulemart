// Package gittest builds Syllabus library repositories on disk for tests that fetch and ingest them.
package gittest

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
)

// FirstTagged is when a fixture's first tag is made; each later tag is a day after the one before.
var FirstTagged = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

// Library is a Syllabus library's Git repository, built on disk for a test. Its methods fail the test on error.
type Library struct {
	t    *testing.T
	dir  string
	repo *git.Repository
	tags int
	// rules is the directory Group and Rule write in: the repository's root in the earlier layout, or the Syllabus
	// directory's rules/.
	rules string
}

// syllabusConfig is the configuration of a library in the Syllabus directory, whose library section declares an MIT
// license in LICENSE.
const syllabusConfig = "schemaVersion: 2\nsources: {}\nlibrary:\n  license:\n    spdxExpression: MIT\n    file: LICENSE\n    notices: []\n"

// NewLibrary returns a repository in the earlier layout, which Syllabus libraries had before they moved into
// .syllabus/: its working tree holds only rule-library.yaml at the root, declaring an MIT license, and its LICENSE
// file, and Group and Rule write at the root.
func NewLibrary(t *testing.T) *Library {
	t.Helper()
	l := newRepository(t)
	l.Write("rule-library.yaml", "formatVersion: 1\nlicense:\n  spdxExpression: MIT\n  file: LICENSE\n  notices: []\n")
	l.Write("LICENSE", "MIT License\n")
	return l
}

// NewSyllabusLibrary returns a repository that keeps its library in the Syllabus directory: its working tree holds
// only .syllabus/config.yaml, whose library section declares an MIT license, and its LICENSE file at the root, and
// Group and Rule write in .syllabus/rules/.
func NewSyllabusLibrary(t *testing.T) *Library {
	t.Helper()
	l := newRepository(t)
	l.Write(".syllabus/config.yaml", syllabusConfig)
	l.Write("LICENSE", "MIT License\n")
	l.rules = ".syllabus/rules/"
	return l
}

// newRepository returns an empty repository in a directory the test removes.
func newRepository(t *testing.T) *Library {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	return &Library{t: t, dir: dir, repo: repo}
}

// MoveToSyllabusDirectory moves a library in the earlier layout into the Syllabus directory, as its maintainer
// would: rule-library.yaml becomes .syllabus/config.yaml's library section, techs/ and practices/ move into
// .syllabus/rules/, and assets/ and changes/ into .syllabus/. Group and Rule write in .syllabus/rules/ after it.
func (l *Library) MoveToSyllabusDirectory() {
	l.t.Helper()
	l.Remove("rule-library.yaml")
	l.Write(".syllabus/config.yaml", syllabusConfig)
	for from, to := range map[string]string{
		"techs": ".syllabus/rules/techs", "practices": ".syllabus/rules/practices",
		"assets": ".syllabus/assets", "changes": ".syllabus/changes",
	} {
		if err := os.MkdirAll(filepath.Join(l.dir, filepath.FromSlash(path.Dir(to))), 0o755); err != nil {
			l.t.Fatal(err)
		}
		err := os.Rename(filepath.Join(l.dir, from), filepath.Join(l.dir, filepath.FromSlash(to)))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			l.t.Fatal(err)
		}
	}
	l.rules = ".syllabus/rules/"
}

// Write puts content at path in the working tree.
func (l *Library) Write(path, content string) {
	l.t.Helper()
	full := filepath.Join(l.dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		l.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		l.t.Fatal(err)
	}
}

// Remove deletes path from the working tree.
func (l *Library) Remove(path string) {
	l.t.Helper()
	if err := os.Remove(filepath.Join(l.dir, filepath.FromSlash(path))); err != nil {
		l.t.Fatal(err)
	}
}

// Group writes group id's _group.yaml, describing it as name.
func (l *Library) Group(id, name string) {
	l.t.Helper()
	l.Write(l.rules+id+"/_group.yaml", "name: "+name+"\ndescription: "+name+" rules.\nwhenToRead: When the work involves "+
		strings.ToLower(name)+".\n")
}

// Rule writes rule id's file with title, HIGH impact, and a body that starts with the title as a heading, as Code
// Rules' template does, followed by body.
func (l *Library) Rule(id, title, body string) {
	l.t.Helper()
	l.Write(l.rules+id+".md", "---\ntitle: "+title+"\nwhenToRead: When changing "+strings.ToLower(title)+".\nimpact: HIGH\n"+
		"impactDescription: Prevents mistakes in "+strings.ToLower(title)+".\n---\n\n## "+title+"\n\n"+body+"\n")
}

// Release commits the working tree and tags it release/<number>, with release notes and then record, the YAML
// release record.
func (l *Library) Release(number int, record string) {
	l.t.Helper()
	l.Tag("release/"+strconv.Itoa(number), l.Commit("Prepare library release "+strconv.Itoa(number)),
		"Library release "+strconv.Itoa(number)+".\n---\n"+record)
}

// Commit commits every change in the working tree and returns the commit.
func (l *Library) Commit(message string) *object.Commit {
	l.t.Helper()
	worktree, err := l.repo.Worktree()
	if err != nil {
		l.t.Fatal(err)
	}
	if err := worktree.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		l.t.Fatal(err)
	}
	hash, err := worktree.Commit(message, &git.CommitOptions{Author: signature(FirstTagged), AllowEmptyCommits: true})
	if err != nil {
		l.t.Fatal(err)
	}
	commit, err := l.repo.CommitObject(hash)
	if err != nil {
		l.t.Fatal(err)
	}
	return commit
}

// Tag makes an annotated tag of commit, a day after the previous tag.
func (l *Library) Tag(name string, commit *object.Commit, message string) {
	l.t.Helper()
	when := FirstTagged.AddDate(0, 0, l.tags)
	if _, err := l.repo.CreateTag(name, commit.Hash, &git.CreateTagOptions{Tagger: signature(when), Message: message}); err != nil {
		l.t.Fatal(err)
	}
	l.tags++
}

// Retag replaces the annotated tag name with one of the same commit whose message is message, a day after the
// previous tag, as a library that rewrites a release's tag does.
func (l *Library) Retag(name, message string) {
	l.t.Helper()
	ref, err := l.repo.Tag(name)
	if err != nil {
		l.t.Fatal(err)
	}
	tag, err := l.repo.TagObject(ref.Hash())
	if err != nil {
		l.t.Fatal(err)
	}
	commit, err := tag.Commit()
	if err != nil {
		l.t.Fatal(err)
	}
	if err := l.repo.DeleteTag(name); err != nil {
		l.t.Fatal(err)
	}
	l.Tag(name, commit, message)
}

// URL returns where git.Fetch fetches the library from: its directory.
func (l *Library) URL() string { return l.dir }

// Repository describes the library as GitHub would: example/rules, with GitHub repository ID id.
func (l *Library) Repository(id int64) domain.Repository {
	return domain.Repository{
		Host: domain.GitHub, ID: strconv.FormatInt(id, 10), Owner: "example", Name: "rules", Description: "Example rules for tests.",
		OwnerAvatarURL: "https://avatars.githubusercontent.com/u/1?v=4", CloneURL: l.URL(),
	}
}

func signature(when time.Time) *object.Signature {
	return &object.Signature{Name: "Library Author", Email: "author@example.com", When: when}
}
