package app

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

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
	"github.com/fabricahq/rulemart/internal/platform/database/databasetest"
	"github.com/fabricahq/rulemart/internal/platform/postgrestest"
)

// sharedRules is how many rules the budget tests publish, all with one file's content, which Git stores once.
const sharedRules = 40

// sharedRule is the content every rule in the budget tests has.
var sharedRule = "---\ntitle: Shared rule\nwhenToRead: When testing budgets.\nimpact: LOW\nimpactDescription: Tests budgets.\n---\n\n" +
	strings.Repeat("A paragraph that every rule repeats, so the release is small in Git and large once read.\n\n", 40)

// goGroup is the _group.yaml of the group that holds sharedLibrary's rules.
const goGroup = "name: Go\ndescription: Go rules.\nwhenToRead: When writing Go.\n"

// sharedLibrary returns the path of a repository whose release/1 publishes sharedRules rules with sharedRule's
// content, and how many bytes of content ingestion reads and holds for them and their group.
func sharedLibrary(t *testing.T) (string, int64) {
	t.Helper()
	files := map[string][]byte{
		"rule-library.yaml":    []byte("formatVersion: 1\nlicense:\n  spdxExpression: MIT\n  file: LICENSE\n  notices: []\n"),
		"LICENSE":              []byte("MIT License\n"),
		"techs/go/_group.yaml": []byte(goGroup),
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

	return dir, sharedRules*sharedRuleBytes(t) + int64(len(goGroup))
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
	db, connString := databasetest.New(t)
	return NewStore(db), connString
}

// Git stores one blob for every path that has its content, so the fetch limits can't bound what ingestion reads
// when many rules share a file.
func TestIngestRefusesRuleContentPastItsBudgetWithoutWriting(t *testing.T) {
	store, connString := budgetStore(t)
	dir, total := sharedLibrary(t)
	repo := domain.Repository{Host: domain.GitHub, ID: "42", Owner: "example", Name: "rules", CloneURL: dir}

	_, err := ingest(context.Background(), store, repo, limits{fetch: defaultFetchLimits, contentBytes: total - 1})

	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("more than %d bytes of content", total-1)) {
		t.Fatalf("got error %v, want a refusal past %d bytes of rule content", err, total-1)
	}
	var libraries int
	postgrestest.QueryRow(t, connString, "SELECT count(*) FROM libraries", &libraries)
	if libraries != 0 {
		t.Fatal("a refused ingestion wrote the library")
	}
}

func TestIngestAcceptsRuleContentUpToItsBudget(t *testing.T) {
	store, _ := budgetStore(t)
	dir, total := sharedLibrary(t)
	repo := domain.Repository{Host: domain.GitHub, ID: "42", Owner: "example", Name: "rules", CloneURL: dir}

	result, err := ingest(context.Background(), store, repo, limits{fetch: defaultFetchLimits, contentBytes: total})

	if err != nil {
		t.Fatal(err)
	}
	if result.Rules != sharedRules {
		t.Fatalf("ingested %d rules, want %d", result.Rules, sharedRules)
	}
}

// sharedGroups is how many groups the group budget tests publish, all with one _group.yaml's content.
const sharedGroups = 20

// sharedGroup is the _group.yaml every group in the group budget tests has.
var sharedGroup = "name: Shared\ndescription: " + strings.Repeat("A long description every group repeats. ", 1000) +
	"\nwhenToRead: When testing budgets.\n"

// sharedGroupsLibrary returns the path of a repository whose release/1 publishes sharedGroups groups with
// sharedGroup's content, each with one rule with sharedRule's content, and how many bytes ingestion reads and
// renders for them all.
func sharedGroupsLibrary(t *testing.T) (string, int64) {
	t.Helper()
	files := map[string][]byte{
		"rule-library.yaml": []byte("formatVersion: 1\nlicense:\n  spdxExpression: MIT\n  file: LICENSE\n  notices: []\n"),
		"LICENSE":           []byte("MIT License\n"),
	}
	var rules, changes strings.Builder
	for i := range sharedGroups {
		group := fmt.Sprintf("techs/g-%02d", i)
		files[group+"/_group.yaml"] = []byte(sharedGroup)
		files[group+"/rule.md"] = []byte(sharedRule)
		fmt.Fprintf(&rules, "  %s/rule: 1.0.0\n", group)
		fmt.Fprintf(&changes, "  %s/rule: {change: new, summaries: [Add the rule.]}\n", group)
	}
	record := "formatVersion: 1\nrelease: 1\nrules:\n" + rules.String() + "changes:\n" + changes.String()
	dir := releasedRepository(t, files, "Library release 1.\n---\n"+record)
	return dir, sharedGroups * (sharedRuleBytes(t) + int64(len(sharedGroup)))
}

// sharedRuleBytes returns how many bytes of content ingestion reads and holds for one rule with sharedRule's
// content: its Markdown, its title, impact description, and reading guidance, and its HTML.
func sharedRuleBytes(t *testing.T) int64 {
	t.Helper()
	parsed, err := coderules.Parse(sharedRule, "techs/go/rule-00.md", "example/rules")
	if err != nil {
		t.Fatal(err)
	}
	document, err := coderules.SplitDocument(sharedRule, "techs/go/rule-00.md")
	if err != nil {
		t.Fatal(err)
	}
	html, err := renderRule(document.Body, rulePage{repository: "example/rules", path: "techs/go/rule-00.md", title: "Shared rule", tag: "release/1", latestTag: "release/1"}, unlimited())
	if err != nil {
		t.Fatal(err)
	}
	metadata := len(parsed.Title) + len(parsed.ImpactDescription) + len(parsed.WhenToRead)
	return int64(len(sharedRule) + metadata + len(html))
}

// Groups' metadata is held until the library is written, like rules' content, and a release can list many groups
// that share one _group.yaml, which Git stores once.
func TestIngestRefusesGroupMetadataPastTheBudgetWithoutWriting(t *testing.T) {
	store, connString := budgetStore(t)
	dir, total := sharedGroupsLibrary(t)
	repo := domain.Repository{Host: domain.GitHub, ID: "42", Owner: "example", Name: "rules", CloneURL: dir}

	_, err := ingest(context.Background(), store, repo, limits{fetch: defaultFetchLimits, contentBytes: total - 1})

	if err == nil || !strings.Contains(err.Error(), "_group.yaml") || !strings.Contains(err.Error(), fmt.Sprintf("more than %d bytes", total-1)) {
		t.Fatalf("got error %v, want a group file refused past %d bytes", err, total-1)
	}
	var libraries int
	postgrestest.QueryRow(t, connString, "SELECT count(*) FROM libraries", &libraries)
	if libraries != 0 {
		t.Fatal("a refused ingestion wrote the library")
	}
}

func TestIngestAcceptsGroupMetadataUpToTheBudget(t *testing.T) {
	store, _ := budgetStore(t)
	dir, total := sharedGroupsLibrary(t)
	repo := domain.Repository{Host: domain.GitHub, ID: "42", Owner: "example", Name: "rules", CloneURL: dir}

	result, err := ingest(context.Background(), store, repo, limits{fetch: defaultFetchLimits, contentBytes: total})

	if err != nil {
		t.Fatal(err)
	}
	if result.Rules != sharedGroups {
		t.Fatalf("ingested %d rules, want %d", result.Rules, sharedGroups)
	}
}
