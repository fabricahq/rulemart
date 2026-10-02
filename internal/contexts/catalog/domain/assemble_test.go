package domain

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// files is a release's files, by path, held in memory.
type files map[string]string

func (f files) Open(path string) (File, error) {
	content, ok := f[path]
	if !ok {
		return nil, ErrFileMissing
	}
	return memoryFile(content), nil
}

type memoryFile string

func (f memoryFile) Size() int64           { return int64(len(f)) }
func (f memoryFile) Read() ([]byte, error) { return []byte(f), nil }

// unreadable is a file whose size is known but that a test expects never to be read.
type unreadable struct {
	t    *testing.T
	path string
	size int64
}

func (f unreadable) Size() int64 { return f.size }
func (f unreadable) Read() ([]byte, error) {
	f.t.Errorf("read %s, which assembly doesn't need", f.path)
	return nil, nil
}

// withUnreadable is a release's files plus ones a test expects never to be read.
type withUnreadable struct {
	files
	t          *testing.T
	unreadable map[string]int64
}

func (f withUnreadable) Open(path string) (File, error) {
	if size, ok := f.unreadable[path]; ok {
		return unreadable{f.t, path, size}, nil
	}
	return f.files.Open(path)
}

// markup stands in for render.Rule: it wraps the body in a paragraph, and uses as many bytes as that takes.
func markup(body string, _ RulePage, allowance int64) (string, int64, error) {
	html := "<p>" + body + "</p>\n"
	if int64(len(html)) > allowance {
		return "", 0, ErrOverAllowance
	}
	return html, int64(len(html)), nil
}

// limits leave room for every test library, except where a test lowers them.
var limits = ContentLimits{FileBytes: 1 << 20, ContentBytes: 1 << 30}

var repo = Repository{Host: GitHub, ID: "42", Owner: "example", Name: "rules", Description: "Example rules."}

const manifest = "formatVersion: 1\nlicense:\n  spdxExpression: MIT\n  file: LICENSE\n  notices: []\n"

// library returns the files of a library release: its manifest, license, and more.
func library(more files) files {
	f := files{"rule-library.yaml": manifest, "LICENSE": "MIT License\n"}
	for path, content := range more {
		f[path] = content
	}
	return f
}

// rule returns a rule file with title, HIGH impact, and a body that starts with the title as a heading, as Code
// Rules' template does, followed by body.
func rule(title, body string) string {
	return "---\ntitle: " + title + "\nwhenToRead: When changing " + strings.ToLower(title) + ".\nimpact: HIGH\n" +
		"impactDescription: Prevents mistakes in " + strings.ToLower(title) + ".\n---\n\n## " + title + "\n\n" + body + "\n"
}

// group returns a group's _group.yaml, describing it as name.
func group(name string) string {
	return "name: " + name + "\ndescription: " + name + " rules.\nwhenToRead: When the work involves " + strings.ToLower(name) + ".\n"
}

// snapshot returns library release number, whose tag holds record and whose commit holds f.
func snapshot(t *testing.T, number int, record string, f Files) ReleaseSnapshot {
	t.Helper()
	tag := ReleaseTag(number)
	parsed, err := coderules.ParseReleaseRecord([]byte(record), tag)
	if err != nil {
		t.Fatalf("%s: %v", tag, err)
	}
	return ReleaseSnapshot{
		Number: number, Tag: tag, TaggedAt: time.Date(2026, 9, number, 12, 0, 0, 0, time.UTC),
		CommitID: fmt.Sprintf("%040d", number), Record: parsed, Files: f,
	}
}

const firstRecord = `formatVersion: 1
release: 1
rules: {practices/testing/check-retry-backoff: 1.0.0, practices/testing/verify-retry-limits: 1.0.0, techs/go/return-errors: 1.0.0}
changes:
  practices/testing/check-retry-backoff: {change: new, summaries: [Add the rule.]}
  practices/testing/verify-retry-limits: {change: new, summaries: [Add the rule.]}
  techs/go/return-errors: {change: new, summaries: [Add the rule.]}
libraryFiles: [LICENSE, practices/testing/_group.yaml, rule-library.yaml, techs/go/_group.yaml]
`

// first publishes three rules in two groups.
func first(t *testing.T) ReleaseSnapshot {
	t.Helper()
	return snapshot(t, 1, firstRecord, library(files{
		"practices/testing/_group.yaml":            group("Testing"),
		"techs/go/_group.yaml":                     group("Go"),
		"practices/testing/check-retry-backoff.md": rule("Check retry backoff", "Retries wait longer after each attempt."),
		"practices/testing/verify-retry-limits.md": rule("Verify retry limits", "Stop after a fixed number of attempts."),
		"techs/go/return-errors.md":                rule("Return errors", "Return errors instead of panicking."),
	}))
}

// A release records each change and retirement, and a rule's content is its file at the release that published its
// current version: a later edit that no release recorded must not show.
func TestAssembleReadsEachRuleAtTheReleaseThatPublishedIt(t *testing.T) {
	second := snapshot(t, 2, `formatVersion: 1
release: 2
rules: {practices/testing/verify-retry-limits: 1.1.0, techs/go/return-errors: 1.0.0}
changes:
  practices/testing/verify-retry-limits: {change: minor, from: 1.0.0, summaries: [Count timeouts.]}
retired:
  practices/testing/check-retry-backoff: {lastVersion: 1.0.0, replacedBy: practices/testing/verify-retry-limits, summaries: [Merge it.]}
libraryFiles: [techs/go/_group.yaml]
`, library(files{
		"practices/testing/_group.yaml":            group("Testing"),
		"techs/go/_group.yaml":                     group("Go language"),
		"practices/testing/verify-retry-limits.md": rule("Verify retry limits", "Stop after a fixed number of attempts, timeouts included."),
		"techs/go/return-errors.md":                rule("Unreleased title", "An edit no library release recorded."),
	}))

	lib, err := Assemble(repo, []ReleaseSnapshot{first(t), second}, limits, markup)

	if err != nil {
		t.Fatal(err)
	}
	if lib.LicenseExpression != "MIT" || lib.LicenseFile != "LICENSE" || lib.Repository != repo {
		t.Errorf("library is %+v under %q in %q", lib.Repository, lib.LicenseExpression, lib.LicenseFile)
	}
	wantReleases := []Release{
		{Number: 1, CommitID: fmt.Sprintf("%040d", 1), TaggedAt: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)},
		// Release 1 lists every file, as a first release does, but adds them rather than updating them.
		{Number: 2, CommitID: fmt.Sprintf("%040d", 2), TaggedAt: time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC), UpdatesSharedFiles: true},
	}
	if !slices.Equal(lib.Releases, wantReleases) {
		t.Errorf("releases are %+v, want %+v", lib.Releases, wantReleases)
	}
	wantGroups := []Group{
		{Path: "practices/testing", Name: "Testing", Description: "Testing rules.", WhenToRead: "When the work involves testing."},
		{Path: "techs/go", Name: "Go language", Description: "Go language rules.", WhenToRead: "When the work involves go language."},
	}
	if !slices.Equal(lib.Groups, wantGroups) {
		t.Errorf("groups are %+v, want %+v", lib.Groups, wantGroups)
	}
	if got := describe(lib.Rules); !slices.Equal(got, []string{
		"practices/testing/check-retry-backoff in practices/testing: 1.0.0 release/1 new [Add the rule.]; retired in release/2 by practices/testing/verify-retry-limits [Merge it.]",
		"practices/testing/verify-retry-limits in practices/testing: 1.0.0 release/1 new [Add the rule.], 1.1.0 release/2 minor [Count timeouts.]; Verify retry limits",
		"techs/go/return-errors in techs/go: 1.0.0 release/1 new [Add the rule.]; Return errors",
	}) {
		t.Errorf("rules are %q", got)
	}
	retryLimits := lib.Rules[1].Content
	if !strings.Contains(retryLimits.HTML, "timeouts included") {
		t.Errorf("the current version's HTML is %s", retryLimits.HTML)
	}
	if retryLimits.Impact != "HIGH" || retryLimits.WhenToRead != "When changing verify retry limits." ||
		retryLimits.WhenToReadHTML != "<p>When changing verify retry limits.</p>\n" ||
		!strings.HasPrefix(retryLimits.Markdown, "---\ntitle: Verify retry limits\n") {
		t.Errorf("the current version's content is %+v", retryLimits)
	}
	if lib.CurrentRules() != 2 {
		t.Errorf("%d current rules, want 2", lib.CurrentRules())
	}
}

// describe returns each rule as "<path> in <group>: <versions>; <title, or retirement>".
func describe(rules []Rule) []string {
	var result []string
	for _, r := range rules {
		var versions []string
		for _, v := range r.Versions {
			versions = append(versions, fmt.Sprintf("%s %s %s %v", v.Number, ReleaseTag(v.Release), v.Change, v.Summaries))
		}
		end := ""
		if r.Content != nil {
			end = r.Content.Title
		} else {
			end = fmt.Sprintf("retired in %s by %s %v", ReleaseTag(r.RetiredIn), r.ReplacedBy, r.RetirementSummaries)
		}
		result = append(result, r.Path+" in "+r.Group+": "+strings.Join(versions, ", ")+"; "+end)
	}
	return result
}

// Assembly reads only the files the catalog stores. Never every file a release holds.
func TestAssembleReadsOnlyTheFilesItNeeds(t *testing.T) {
	release := first(t)
	release.Files = withUnreadable{files: release.Files.(files), t: t, unreadable: map[string]int64{
		"assets/huge.bin": 512 << 20,
		// The license file must exist, but the catalog only links to it.
		"LICENSE": 12,
	}}

	if _, err := Assemble(repo, []ReleaseSnapshot{release}, limits, markup); err != nil {
		t.Fatal(err)
	}
}

// A group stays while any rule belongs to it, so every rule's group exists. One whose rules are all retired keeps
// the metadata of the last release that had its _group.yaml.
func TestAssembleKeepsTheMetadataOfAGroupWhoseRulesAreAllRetired(t *testing.T) {
	second := snapshot(t, 2, `formatVersion: 1
release: 2
rules: {practices/testing/check-retry-backoff: 1.0.0, practices/testing/verify-retry-limits: 1.0.0}
retired: {techs/go/return-errors: {lastVersion: 1.0.0, summaries: [Retire it.]}}
`, library(files{
		"practices/testing/_group.yaml": group("Testing"),
	}))

	lib, err := Assemble(repo, []ReleaseSnapshot{first(t), second}, limits, markup)

	if err != nil {
		t.Fatal(err)
	}
	if got := lib.Groups[1]; got.Path != "techs/go" || got.Name != "Go" {
		t.Fatalf("the retired rules' group is %+v, want release/1's techs/go", got)
	}
	if r := lib.Rules[2]; r.Path != "techs/go/return-errors" || r.Group != "techs/go" || r.Content != nil {
		t.Fatalf("the retired rule is %+v", r)
	}
}

func TestAssembleRefusesALibraryMissingAFileItPublishes(t *testing.T) {
	for name, tc := range map[string]struct {
		remove, want string
	}{
		// The library page links to the license file, so a release whose manifest names a missing one is refused
		// rather than stored as a broken link.
		"the declared license file": {"LICENSE", "release/1: rule-library.yaml declares the license file LICENSE, which is missing"},
		"the manifest":              {"rule-library.yaml", "release/1: rule-library.yaml is missing"},
		"a current rule's file":     {"techs/go/return-errors.md", "release/1: techs/go/return-errors.md: the file doesn't exist"},
		"a current group's file":    {"techs/go/_group.yaml", "release/1: techs/go/_group.yaml: the file doesn't exist"},
	} {
		t.Run(name, func(t *testing.T) {
			release := first(t)
			delete(release.Files.(files), tc.remove)

			_, err := Assemble(repo, []ReleaseSnapshot{release}, limits, markup)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got error %v, want one containing %q", err, tc.want)
			}
		})
	}
}

func TestAssembleRefusesAFileLargerThanItsLimit(t *testing.T) {
	release := first(t)
	small := limits
	small.FileBytes = int64(len(release.Files.(files)["techs/go/return-errors.md"])) - 1

	_, err := Assemble(repo, []ReleaseSnapshot{release}, small, markup)

	if want := fmt.Sprintf("more than the %d ingestion reads", small.FileBytes); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("got error %v, want one containing %q", err, want)
	}
}

func TestAssembleRefusesRecordsThatDontFormOneHistory(t *testing.T) {
	second := snapshot(t, 2, `formatVersion: 1
release: 2
rules: {practices/testing/verify-retry-limits: 1.0.0, techs/go/return-errors: 1.0.0}
`, first(t).Files)

	_, err := Assemble(repo, []ReleaseSnapshot{first(t), second}, limits, markup)

	if want := "release/2.rules.practices/testing/check-retry-backoff"; err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("got error %v, want one naming %s", err, want)
	}
}

// sharedRule is the content every rule in the budget tests has. A release's files are stored once however many
// paths share them, so what a source fetches can't bound what assembly reads.
var sharedRule = "---\ntitle: Shared rule\nwhenToRead: When testing budgets.\nimpact: LOW\nimpactDescription: Tests budgets.\n---\n\n" +
	strings.Repeat("A paragraph that every rule repeats, so the release is small in Git and large once read.\n\n", 40)

// sharedRuleBytes returns the content assembly holds for one rule with sharedRule's content at path: its Markdown,
// its title, impact description, and reading guidance, and the HTML of its body and reading guidance.
func sharedRuleBytes(t *testing.T, path string) int64 {
	t.Helper()
	parsed, err := coderules.Parse(sharedRule, path, repo.FullName())
	if err != nil {
		t.Fatal(err)
	}
	document, err := coderules.SplitDocument(sharedRule, path)
	if err != nil {
		t.Fatal(err)
	}
	html, _, err := markup(document.Body, RulePage{}, 1<<40)
	if err != nil {
		t.Fatal(err)
	}
	whenToReadHTML, _, err := markup(strings.TrimSpace(parsed.WhenToRead), RulePage{}, 1<<40)
	if err != nil {
		t.Fatal(err)
	}
	return int64(len(sharedRule) + len(parsed.Title) + len(parsed.ImpactDescription) + len(parsed.WhenToRead) + len(html) +
		len(whenToReadHTML))
}

// sharedRules returns release/1 of a library with count rules that share sharedRule's content, and when
// sharedGroups, each in its own group with a long _group.yaml they share. It returns how many bytes of content
// assembly holds for the rules, and for the groups.
func sharedRules(t *testing.T, count int, sharedGroups bool) (release ReleaseSnapshot, rulesBytes, groupsBytes int64) {
	t.Helper()
	sharedGroup := "name: Shared\ndescription: " + strings.Repeat("A long description every group repeats. ", 1000) +
		"\nwhenToRead: When testing budgets.\n"
	f := library(files{"techs/go/_group.yaml": group("Go")})
	groupsBytes = int64(len(f["techs/go/_group.yaml"]))
	if sharedGroups {
		f, groupsBytes = library(nil), 0
	}
	var rules, changes strings.Builder
	for i := range count {
		path := fmt.Sprintf("techs/go/rule-%02d", i)
		if sharedGroups {
			path = fmt.Sprintf("techs/g-%02d/rule", i)
			f[GroupFile(fmt.Sprintf("techs/g-%02d", i))] = sharedGroup
			groupsBytes += int64(len(sharedGroup))
		}
		f[RuleFile(path)] = sharedRule
		rulesBytes += sharedRuleBytes(t, RuleFile(path))
		fmt.Fprintf(&rules, "  %s: 1.0.0\n", path)
		fmt.Fprintf(&changes, "  %s: {change: new, summaries: [Add the rule.]}\n", path)
	}
	record := "formatVersion: 1\nrelease: 1\nrules:\n" + rules.String() + "changes:\n" + changes.String()
	return snapshot(t, 1, record, f), rulesBytes, groupsBytes
}

func TestAssembleKeepsContentWithinItsBudget(t *testing.T) {
	for name, tc := range map[string]struct {
		sharedGroups bool
		// short returns a budget a byte short, for the rules' and groups' bytes, and refused names the file the
		// refusal blames.
		short   func(rules, groups int64) int64
		refused string
	}{
		"rules that share one file": {false, func(rules, _ int64) int64 { return rules - 1 }, "techs/go/rule-"},
		// Groups' metadata is held until the library is stored, like rules' content.
		"groups that share one file": {true, func(rules, groups int64) int64 { return rules + groups - 1 }, "/_group.yaml"},
	} {
		t.Run(name, func(t *testing.T) {
			release, rules, groups := sharedRules(t, 20, tc.sharedGroups)
			budget := limits

			budget.ContentBytes = rules + groups
			lib, err := Assemble(repo, []ReleaseSnapshot{release}, budget, markup)
			if err != nil || lib.CurrentRules() != 20 {
				t.Fatalf("within the budget: got %d rules, %v; want 20", lib.CurrentRules(), err)
			}

			budget.ContentBytes = tc.short(rules, groups)
			_, err = Assemble(repo, []ReleaseSnapshot{release}, budget, markup)
			want := fmt.Sprintf("more than %d bytes of content", budget.ContentBytes)
			if err == nil || !strings.Contains(err.Error(), tc.refused) || !strings.Contains(err.Error(), want) {
				t.Fatalf("a byte over the budget: got error %v, want %q naming %s", err, want, tc.refused)
			}
		})
	}
}

// A body whose HTML would pass what's left of the budget is refused with the budget's error, not rendering's.
func TestAssembleRefusesARuleWhoseHTMLWouldPassTheBudget(t *testing.T) {
	refuse := func(string, RulePage, int64) (string, int64, error) { return "", 0, ErrOverAllowance }

	_, err := Assemble(repo, []ReleaseSnapshot{first(t)}, limits, refuse)

	if want := fmt.Sprintf("release/1: practices/testing/check-retry-backoff.md: the library's rules and groups hold more than %d bytes of content", limits.ContentBytes); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("got error %v, want one containing %q", err, want)
	}
}

// Each rule is rendered for its page: its file, at the release that published its current version, with the
// library's latest release for library-wide links, and within what's left of the budget.
func TestAssembleRendersEachRuleForItsPage(t *testing.T) {
	second := snapshot(t, 2, `formatVersion: 1
release: 2
rules: {practices/testing/check-retry-backoff: 1.0.0, practices/testing/verify-retry-limits: 1.1.0, techs/go/return-errors: 1.0.0}
changes: {practices/testing/verify-retry-limits: {change: minor, from: 1.0.0, summaries: [Count timeouts.]}}
`, library(files{
		"practices/testing/_group.yaml":            group("Testing"),
		"techs/go/_group.yaml":                     group("Go"),
		"practices/testing/verify-retry-limits.md": rule("Verify retry limits", "Stop after a fixed number of attempts."),
	}))
	var pages []RulePage
	var texts []string
	var allowances []int64
	record := func(body string, page RulePage, allowance int64) (string, int64, error) {
		pages, texts, allowances = append(pages, page), append(texts, body), append(allowances, allowance)
		return markup(body, page, allowance)
	}

	if _, err := Assemble(repo, []ReleaseSnapshot{first(t), second}, limits, record); err != nil {
		t.Fatal(err)
	}

	backoff := RulePage{Repository: "example/rules", Path: "practices/testing/check-retry-backoff.md", Title: "Check retry backoff", Tag: "release/1", LatestTag: "release/2"}
	retryLimits := RulePage{Repository: "example/rules", Path: "practices/testing/verify-retry-limits.md", Title: "Verify retry limits", Tag: "release/2", LatestTag: "release/2"}
	returnErrors := RulePage{Repository: "example/rules", Path: "techs/go/return-errors.md", Title: "Return errors", Tag: "release/1", LatestTag: "release/2"}
	// Each rule's body, then its reading guidance, which is Markdown too.
	if want := []RulePage{backoff, backoff, retryLimits, retryLimits, returnErrors, returnErrors}; !slices.Equal(pages, want) {
		t.Errorf("rendered for %+v, want %+v", pages, want)
	}
	if !strings.Contains(texts[0], "## Check retry backoff") || texts[1] != "When changing check retry backoff." {
		t.Errorf("rendered %q", texts[:2])
	}
	if !slices.IsSortedFunc(allowances, func(a, b int64) int { return int(b - a) }) || allowances[0] >= limits.ContentBytes {
		t.Errorf("allowances are %v, want what's left of the budget, shrinking", allowances)
	}
}
