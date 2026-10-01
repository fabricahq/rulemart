// Package ingesttest builds Code Rules library repositories and catalog databases for tests of ingestion and the
// pages that show what it stored.
package ingesttest

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/fabricahq/rulemart/catalog"
	"github.com/fabricahq/rulemart/internal/ingest"
	"github.com/fabricahq/rulemart/internal/platform/database"
	"github.com/fabricahq/rulemart/internal/platform/migrate"
	"github.com/fabricahq/rulemart/internal/platform/postgrestest"
)

// FirstTagged is when a fixture's first tag is made; each later tag is a day after the one before.
var FirstTagged = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

// Library is a Code Rules library's Git repository, built on disk for a test. Its methods fail the test on error.
type Library struct {
	t    *testing.T
	dir  string
	repo *git.Repository
	tags int
}

// NewLibrary returns a repository whose working tree holds only rule-library.yaml, declaring an MIT license, and
// its LICENSE file.
func NewLibrary(t *testing.T) *Library {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	l := &Library{t: t, dir: dir, repo: repo}
	l.Write("rule-library.yaml", "formatVersion: 1\nlicense:\n  spdxExpression: MIT\n  file: LICENSE\n  notices: []\n")
	l.Write("LICENSE", "MIT License\n")
	return l
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
	l.Write(id+"/_group.yaml", "name: "+name+"\ndescription: "+name+" rules.\nwhenToRead: When the work involves "+
		strings.ToLower(name)+".\n")
}

// Rule writes rule id's file with title, HIGH impact, and a body that starts with the title as a heading, as Code
// Rules' template does, followed by body.
func (l *Library) Rule(id, title, body string) {
	l.t.Helper()
	l.Write(id+".md", "---\ntitle: "+title+"\nwhenToRead: When changing "+strings.ToLower(title)+".\nimpact: HIGH\n"+
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

// Repository describes the library as GitHub would: example/rules, with GitHub repository ID id.
func (l *Library) Repository(id int64) ingest.Repository {
	return ingest.Repository{
		Host: catalog.GitHub, ID: strconv.FormatInt(id, 10), Owner: "example", Name: "rules", Description: "Example rules for tests.",
		OwnerAvatarURL: "https://avatars.githubusercontent.com/u/1?v=4", CloneURL: l.dir,
	}
}

func signature(when time.Time) *object.Signature {
	return &object.Signature{Name: "Library Author", Email: "author@example.com", When: when}
}

// NewDatabase returns a DB for a new, migrated test database, and its connection string. It closes the DB when
// the test ends.
func NewDatabase(t *testing.T) (*database.DB, string) {
	t.Helper()
	connString := postgrestest.New(t)
	if err := migrate.Up(context.Background(), connString); err != nil {
		t.Fatal(err)
	}
	version, err := migrate.RequiredVersion()
	if err != nil {
		t.Fatal(err)
	}
	db := database.New(postgrestest.Parameter(connString), "test-database", version)
	t.Cleanup(db.Close)
	return db, connString
}

// NewWebDatabase returns a DB for the test database at connString that connects as postgrestest.WebRole, as the web
// function does, so it has only the access migrations grant that role. It closes the DB when the test ends.
func NewWebDatabase(t *testing.T, connString string) *database.DB {
	t.Helper()
	version, err := migrate.RequiredVersion()
	if err != nil {
		t.Fatal(err)
	}
	db := database.New(postgrestest.Parameter(postgrestest.AsWebRole(t, connString)), "test-web-database", version)
	t.Cleanup(db.Close)
	return db
}
