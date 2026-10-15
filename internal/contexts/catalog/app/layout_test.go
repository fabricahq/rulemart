package app_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git/gittest"
)

// A library that keeps its files in .syllabus/ ingests as one at the root did: the manifest's license, its groups
// and rules by their IDs, and their assets by their library paths.
func TestIngestStoresALibraryInTheSyllabusDirectory(t *testing.T) {
	ingester, pages := newCatalog(t)
	lib := gittest.NewSyllabusLibrary(t)
	lib.Group("techs/go", "Go")
	lib.Rule(returnErrors, "Return errors", "See [an example](assets/return-errors/example.go) and [the lifecycle](/assets/lifecycle.md).")
	lib.Write(".syllabus/rules/techs/go/assets/return-errors/example.go", "package errors\n")
	lib.Write(".syllabus/assets/lifecycle.md", "# Lifecycle\n\nErrors return to the caller.\n")
	// A project's files beside the library's aren't the library's.
	lib.Write(".syllabus/generated/RULES.md", "# Rules\n")
	lib.Release(1, `formatVersion: 1
release: 1
rules: {techs/go/return-errors: 1.0.0}
changes: {techs/go/return-errors: {change: new, summaries: [Add the rule.]}}
libraryFiles: [LICENSE, assets/lifecycle.md, config.yaml, techs/go/_group.yaml]
`)

	if _, err := ingest(ingester, lib.Repository(42)); err != nil {
		t.Fatal(err)
	}

	page, err := pages.LibraryPage(context.Background(), "example", "rules")
	if err != nil {
		t.Fatal(err)
	}
	if l := page.Library; l.LicenseExpression != "MIT" || l.LicenseFile != "LICENSE" || l.Layouts.Of(1) != domain.SyllabusLayout {
		t.Errorf("the library is %+v, want MIT in LICENSE, with release/1 in .syllabus/", l)
	}
	if len(page.Groups) != 1 || page.Groups[0].Path != "techs/go" || len(page.Rules) != 1 || page.Rules[0].Path != returnErrors {
		t.Errorf("the library holds %+v and %+v, want techs/go with %s", page.Groups, page.Rules, returnErrors)
	}
	rule, err := pages.RulePage(context.Background(), "example", "rules", returnErrors)
	if err != nil {
		t.Fatal(err)
	}
	var assets []string
	for _, a := range rule.Assets {
		assets = append(assets, a.Path)
	}
	if want := []string{"techs/go/assets/return-errors/example.go", "assets/lifecycle.md"}; !slices.Equal(assets, want) {
		t.Errorf("%s lists assets %q, want %q", returnErrors, assets, want)
	}
	if html := domain.LinksForLibrary(rule.Rule.HTML, "example/rules"); !strings.Contains(html, `href="/example/rules/assets/lifecycle.md"`) ||
		!strings.Contains(html, `href="/example/rules/techs/go/return-errors/assets/example.go"`) {
		t.Errorf("%s's links don't lead to its assets' pages: %s", returnErrors, html)
	}
}

// A library that moved into .syllabus/ keeps its history: the releases before the move, which kept their files at
// the root, still give their rule versions, and the releases after it give theirs from .syllabus/.
func TestIngestKeepsTheHistoryOfALibraryThatMovedIntoTheSyllabusDirectory(t *testing.T) {
	ingester, pages := newCatalog(t)
	lib := firstRelease(t)
	lib.MoveToSyllabusDirectory()
	lib.Rule(retryLimits, "Verify retry limits", "Every retry loop stops after a fixed number of attempts, including timeouts.")
	lib.Release(2, `formatVersion: 1
release: 2
rules:
  practices/testing/check-retry-backoff: 1.0.0
  practices/testing/verify-retry-limits: 1.1.0
  techs/go/return-errors: 1.0.0
changes:
  practices/testing/verify-retry-limits: {change: minor, from: 1.0.0, summaries: [Cover timeouts.]}
`)

	if _, err := ingest(ingester, lib.Repository(42)); err != nil {
		t.Fatal(err)
	}

	page, err := pages.LibraryPage(context.Background(), "example", "rules")
	if err != nil {
		t.Fatal(err)
	}
	if l := page.Library; l.LatestRelease != 2 || l.Layouts.Of(1) != domain.RootLayout || l.Layouts.Of(2) != domain.SyllabusLayout ||
		l.LicenseFile != "LICENSE" || len(page.Groups) != 2 || len(page.Rules) != 3 {
		t.Errorf("the library page is %+v, want release/1 at the root, release/2 in .syllabus/, and three rules in two groups", page)
	}
	rule, err := pages.RulePage(context.Background(), "example", "rules", retryLimits)
	if err != nil {
		t.Fatal(err)
	}
	if rule.Rule.Release != 2 || !strings.Contains(rule.Rule.HTML, "including timeouts") || len(rule.Versions) != 2 {
		t.Errorf("%s shows release/%d's %s, with versions %+v", retryLimits, rule.Rule.Release, rule.Rule.HTML, rule.Versions)
	}
	comparison, err := pages.RuleComparison(context.Background(), "example", "rules", retryLimits, rule.Versions[1].Version, rule.Versions[0].Version)
	if err != nil {
		t.Fatal(err)
	}
	if text := comparison.Text; strings.Contains(text.Old, "timeouts") || !strings.Contains(text.Old, "fixed number of attempts") ||
		!strings.Contains(text.New, "including timeouts") {
		t.Errorf("%s 1.0.0...1.1.0 compares %+v, want release/1's file at the root and release/2's in .syllabus/", retryLimits, text)
	}
	unchanged, err := pages.RulePage(context.Background(), "example", "rules", returnErrors)
	if err != nil || unchanged.Rule.Release != 1 || !strings.Contains(unchanged.Rule.HTML, "instead of panicking") {
		t.Errorf("%s is %+v, %v; want its version from release/1", returnErrors, unchanged.Rule, err)
	}
}
