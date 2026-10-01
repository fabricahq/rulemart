package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/fabricahq/rulemart/internal/database"
	"github.com/fabricahq/rulemart/internal/migrate"
	"github.com/fabricahq/rulemart/internal/testdb"
	"github.com/fabricahq/rulemart/third_party/coderules"
)

// sharedRules is how many rules the budget tests publish, all with one file's content, which Git stores once.
const sharedRules = 40

// sharedRule is the content every rule in the budget tests has.
var sharedRule = "---\ntitle: Shared rule\nwhenToRead: When testing budgets.\nimpact: LOW\nimpactDescription: Tests budgets.\n---\n\n" +
	strings.Repeat("A paragraph that every rule repeats, so the release is small in Git and large once read.\n\n", 40)

// sharedLibrary returns the path of a repository whose release/1 publishes sharedRules rules with sharedRule's
// content, and how many bytes of Markdown and HTML ingestion reads and renders for all of them.
func sharedLibrary(t *testing.T) (string, int64) {
	t.Helper()
	files := map[string][]byte{
		"rule-library.yaml":    []byte("formatVersion: 1\nlicense:\n  spdxExpression: MIT\n  file: LICENSE\n  notices: []\n"),
		"LICENSE":              []byte("MIT License\n"),
		"techs/go/_group.yaml": []byte("name: Go\ndescription: Go rules.\nwhenToRead: When writing Go.\n"),
	}
	var rules, changes strings.Builder
	for i := range sharedRules {
		id := fmt.Sprintf("techs/go/rule-%02d", i)
		files[id+".md"] = []byte(sharedRule)
		fmt.Fprintf(&rules, "  %s: 1.0.0\n", id)
		fmt.Fprintf(&changes, "  %s: {change: new, summaries: [Add the rule.]}\n", id)
	}
	record := "formatVersion: 1\nrelease: 1\nrules:\n" + rules.String() + "changes:\n" + changes.String()
	dir := releasedRepository(t, files, "Library release 1.\n---\n"+record)

	document, err := coderules.SplitDocument(sharedRule, "techs/go/rule-00.md")
	if err != nil {
		t.Fatal(err)
	}
	html, err := renderRule(document.Body, rulePage{repository: "example/rules", path: "techs/go/rule-00.md", title: "Shared rule", tag: "release/1", latestTag: "release/1"}, unlimited())
	if err != nil {
		t.Fatal(err)
	}
	return dir, sharedRules * int64(len(sharedRule)+len(html))
}

// releasedRepository commits files into a new repository on disk, tags the commit release/1 with message, and
// returns the repository's path.
func releasedRepository(t *testing.T, files map[string][]byte, message string) string {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
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
	commit, err := worktree.Commit("Add rules", &git.CommitOptions{Author: author})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateTag("release/1", commit, &git.CreateTagOptions{Tagger: author, Message: message}); err != nil {
		t.Fatal(err)
	}
	return dir
}

// budgetStore returns a Store on a new, migrated test database, and its connection string.
func budgetStore(t *testing.T) (*Store, string) {
	t.Helper()
	connString := testdb.New(t)
	if err := migrate.Up(context.Background(), connString); err != nil {
		t.Fatal(err)
	}
	version, err := migrate.RequiredVersion()
	if err != nil {
		t.Fatal(err)
	}
	db := database.New(testdb.Parameter(connString), "test-database", version)
	t.Cleanup(db.Close)
	return NewStore(db), connString
}

// Git stores one blob for every path that has its content, so the fetch limits can't bound what ingestion reads
// when many rules share a file.
func TestIngestRefusesRuleContentPastItsBudgetWithoutWriting(t *testing.T) {
	store, connString := budgetStore(t)
	dir, total := sharedLibrary(t)
	repo := Repository{ID: 42, Owner: "example", Name: "rules", CloneURL: dir}

	_, err := ingest(context.Background(), store, repo, limits{fetch: defaultFetchLimits, contentBytes: total - 1})

	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("more than %d bytes of Markdown and HTML", total-1)) {
		t.Fatalf("got error %v, want a refusal past %d bytes of rule content", err, total-1)
	}
	var libraries int
	testdb.QueryRow(t, connString, "SELECT count(*) FROM libraries", &libraries)
	if libraries != 0 {
		t.Fatal("a refused ingestion wrote the library")
	}
}

func TestIngestAcceptsRuleContentUpToItsBudget(t *testing.T) {
	store, _ := budgetStore(t)
	dir, total := sharedLibrary(t)
	repo := Repository{ID: 42, Owner: "example", Name: "rules", CloneURL: dir}

	result, err := ingest(context.Background(), store, repo, limits{fetch: defaultFetchLimits, contentBytes: total})

	if err != nil {
		t.Fatal(err)
	}
	if result.Rules != sharedRules {
		t.Fatalf("ingested %d rules, want %d", result.Rules, sharedRules)
	}
}
