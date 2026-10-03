package domain

import (
	"fmt"
	"reflect"
	"regexp"
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

func (f files) List(dir string, max int) ([]string, error) {
	var paths []string
	for path := range f {
		if strings.HasPrefix(path, dir) {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	return paths[:min(len(paths), max+1)], nil
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

// markup stands in for render.Renderer: it wraps Markdown in a paragraph, and code in a block, using as many bytes as
// that takes, and finds the links written as [text](destination), each destination once, using its bytes.
type markup struct{}

func (markup) Markdown(body string, _ MarkdownSource, allowance int64) (string, int64, error) {
	return within("<p>"+body+"</p>\n", allowance)
}

func (markup) Code(text, _ string, allowance int64) (string, int64, error) {
	return within("<pre>"+text+"</pre>\n", allowance)
}

// markdownLink matches a Markdown link or image written inline, holding its destination.
var markdownLink = regexp.MustCompile(`\]\(([^)\s]+)\)`)

func (markup) Links(body string, allowance int64) ([]string, int64, error) {
	var destinations []string
	var used int64
	for _, match := range markdownLink.FindAllStringSubmatch(body, -1) {
		if slices.Contains(destinations, match[1]) {
			continue
		}
		if used += int64(len(match[1])); used > allowance {
			return nil, 0, ErrOverAllowance
		}
		destinations = append(destinations, match[1])
	}
	return destinations, used, nil
}

// within returns html and its length, or ErrOverAllowance when it's longer than allowance.
func within(html string, allowance int64) (string, int64, error) {
	if int64(len(html)) > allowance {
		return "", 0, ErrOverAllowance
	}
	return html, int64(len(html)), nil
}

// rendering is a renderer that renders Markdown with markdown, and code as markup does, and finds links with links, or
// as markup does when it's nil.
type rendering struct {
	markup
	markdown func(body string, source MarkdownSource, allowance int64) (string, int64, error)
	links    func(body string, allowance int64) ([]string, int64, error)
}

func (r rendering) Markdown(body string, source MarkdownSource, allowance int64) (string, int64, error) {
	if r.markdown == nil {
		return r.markup.Markdown(body, source, allowance)
	}
	return r.markdown(body, source, allowance)
}

func (r rendering) Links(body string, allowance int64) ([]string, int64, error) {
	if r.links == nil {
		return r.markup.Links(body, allowance)
	}
	return r.links(body, allowance)
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

	lib, err := Assemble(repo, []ReleaseSnapshot{first(t), second}, limits, markup{})

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
		"practices/testing/check-retry-backoff in practices/testing: 1.0.0 release/1 new [Add the rule.] Check retry backoff; retired in release/2 by practices/testing/verify-retry-limits [Merge it.]",
		"practices/testing/verify-retry-limits in practices/testing: 1.0.0 release/1 new [Add the rule.] Verify retry limits, 1.1.0 release/2 minor [Count timeouts.] Verify retry limits; current",
		"techs/go/return-errors in techs/go: 1.0.0 release/1 new [Add the rule.] Return errors; current",
	}) {
		t.Errorf("rules are %q", got)
	}
	retryLimits := lib.Rules[1]
	if !strings.Contains(retryLimits.HTML, "timeouts included") {
		t.Errorf("the current version's HTML is %s", retryLimits.HTML)
	}
	current := retryLimits.Current().Content
	if current.Impact != "HIGH" || current.WhenToRead != "When changing verify retry limits." ||
		retryLimits.WhenToReadHTML != "<p>When changing verify retry limits.</p>\n" ||
		!strings.HasPrefix(current.Markdown, "---\ntitle: Verify retry limits\n") || !strings.Contains(current.Markdown, "timeouts included") {
		t.Errorf("the current version's content is %+v", current)
	}
	// Each older version is its file at the release that published it, and a retired rule keeps its versions' files,
	// though the release that retired it no longer holds them.
	if older := retryLimits.Versions[0].Content.Markdown; !strings.Contains(older, "Stop after a fixed number of attempts.\n") || strings.Contains(older, "timeouts") {
		t.Errorf("verify-retry-limits 1.0.0 is %q, want release/1's file", older)
	}
	// A retired rule's page shows its last version's body, rendered at the release that published it.
	if retired := lib.Rules[0]; !strings.Contains(retired.HTML, "Retries wait longer after each attempt.") || retired.WhenToReadHTML != "" ||
		!strings.Contains(retired.Current().Content.Markdown, "Retries wait longer after each attempt.") {
		t.Errorf("the retired rule is %+v", retired)
	}
	if lib.CurrentRules() != 2 {
		t.Errorf("%d current rules, want 2", lib.CurrentRules())
	}
}

// describe returns each rule as "<path> in <group>: <versions, each with its title>; <current, or retirement>".
func describe(rules []Rule) []string {
	var result []string
	for _, r := range rules {
		var versions []string
		for _, v := range r.Versions {
			versions = append(versions, fmt.Sprintf("%s %s %s %v %s", v.Number, ReleaseTag(v.Release), v.Change, v.Summaries, v.Content.Title))
		}
		end := ""
		if r.IsCurrent() {
			end = "current"
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

	if _, err := Assemble(repo, []ReleaseSnapshot{release}, limits, markup{}); err != nil {
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

	lib, err := Assemble(repo, []ReleaseSnapshot{first(t), second}, limits, markup{})

	if err != nil {
		t.Fatal(err)
	}
	if got := lib.Groups[1]; got.Path != "techs/go" || got.Name != "Go" {
		t.Fatalf("the retired rules' group is %+v, want release/1's techs/go", got)
	}
	if r := lib.Rules[2]; r.Path != "techs/go/return-errors" || r.Group != "techs/go" || r.IsCurrent() {
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

			_, err := Assemble(repo, []ReleaseSnapshot{release}, limits, markup{})

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got error %v, want one containing %q", err, tc.want)
			}
		})
	}
}

// Every version's file is read at the release that published it, so a release that lacks the file of a version it
// published is refused, though a later release holds the rule.
func TestAssembleRefusesALibraryMissingAnOlderVersionsFile(t *testing.T) {
	older := first(t)
	delete(older.Files.(files), "practices/testing/verify-retry-limits.md")
	second := snapshot(t, 2, `formatVersion: 1
release: 2
rules: {practices/testing/check-retry-backoff: 1.0.0, practices/testing/verify-retry-limits: 1.1.0, techs/go/return-errors: 1.0.0}
changes: {practices/testing/verify-retry-limits: {change: minor, from: 1.0.0, summaries: [Count timeouts.]}}
`, first(t).Files)

	_, err := Assemble(repo, []ReleaseSnapshot{older, second}, limits, markup{})

	if want := "release/1: practices/testing/verify-retry-limits.md: the file doesn't exist"; err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("got error %v, want one containing %q", err, want)
	}
}

// An older version's file is held until the library is stored, like the current one's, so it spends the budget.
func TestAssembleSpendsTheBudgetOnOlderVersions(t *testing.T) {
	second := snapshot(t, 2, `formatVersion: 1
release: 2
rules: {practices/testing/check-retry-backoff: 1.0.0, practices/testing/verify-retry-limits: 1.1.0, techs/go/return-errors: 1.0.0}
changes: {practices/testing/verify-retry-limits: {change: minor, from: 1.0.0, summaries: [Count timeouts.]}}
`, first(t).Files)
	onlyFirst, err := Assemble(repo, []ReleaseSnapshot{first(t)}, limits, markup{})
	if err != nil {
		t.Fatal(err)
	}
	budget := limits
	// release/1 alone fits, and release/2 adds no file, only a second version of one.
	budget.ContentBytes = contentBytes(onlyFirst)

	_, err = Assemble(repo, []ReleaseSnapshot{first(t), second}, budget, markup{})

	if want := fmt.Sprintf("more than %d bytes of content", budget.ContentBytes); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("got error %v, want %q", err, want)
	}
}

// contentBytes returns the content assembly held for lib: each version's file, title, impact description, and
// reading guidance, each current rule's HTML, and each group's metadata file.
func contentBytes(lib Library) int64 {
	var n int64
	for _, r := range lib.Rules {
		n += int64(len(r.HTML) + len(r.WhenToReadHTML))
		for _, v := range r.Versions {
			n += int64(len(v.Content.Markdown) + len(v.Content.Title) + len(v.Content.ImpactDescription) + len(v.Content.WhenToRead))
		}
	}
	for _, g := range lib.Groups {
		n += int64(len(group(g.Name)))
	}
	return n
}

func TestAssembleRefusesAFileLargerThanItsLimit(t *testing.T) {
	release := first(t)
	small := limits
	small.FileBytes = int64(len(release.Files.(files)["techs/go/return-errors.md"])) - 1

	_, err := Assemble(repo, []ReleaseSnapshot{release}, small, markup{})

	if want := fmt.Sprintf("more than the %d ingestion reads", small.FileBytes); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("got error %v, want one containing %q", err, want)
	}
}

func TestAssembleRefusesRecordsThatDontFormOneHistory(t *testing.T) {
	second := snapshot(t, 2, `formatVersion: 1
release: 2
rules: {practices/testing/verify-retry-limits: 1.0.0, techs/go/return-errors: 1.0.0}
`, first(t).Files)

	_, err := Assemble(repo, []ReleaseSnapshot{first(t), second}, limits, markup{})

	if want := "release/2.rules.practices/testing/check-retry-backoff"; err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("got error %v, want one naming %s", err, want)
	}
}

// sharedRule is the content every rule in the budget tests has. A release's files are stored once however many
// paths share them, so what a source fetches can't bound what assembly reads.
var sharedRule = "---\ntitle: Shared rule\nwhenToRead: When testing budgets.\nimpact: LOW\nimpactDescription: Tests budgets.\n---\n\n" +
	strings.Repeat("A paragraph that every rule repeats, so the release is small in Git and large once read.\n\n", 40)

// sharedRuleBytes returns the content assembly holds for one rule with sharedRule's content at path: its Markdown,
// its title, impact description, reading guidance, and tags, and the HTML of its body and reading guidance.
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
	html, _, err := markup{}.Markdown(document.Body, MarkdownSource{}, 1<<40)
	if err != nil {
		t.Fatal(err)
	}
	whenToReadHTML, _, err := markup{}.Markdown(strings.TrimSpace(parsed.WhenToRead), MarkdownSource{}, 1<<40)
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
			lib, err := Assemble(repo, []ReleaseSnapshot{release}, budget, markup{})
			if err != nil || lib.CurrentRules() != 20 {
				t.Fatalf("within the budget: got %d rules, %v; want 20", lib.CurrentRules(), err)
			}

			budget.ContentBytes = tc.short(rules, groups)
			_, err = Assemble(repo, []ReleaseSnapshot{release}, budget, markup{})
			want := fmt.Sprintf("more than %d bytes of content", budget.ContentBytes)
			if err == nil || !strings.Contains(err.Error(), tc.refused) || !strings.Contains(err.Error(), want) {
				t.Fatalf("a byte over the budget: got error %v, want %q naming %s", err, want, tc.refused)
			}
		})
	}
}

// A body whose HTML would pass what's left of the budget is refused with the budget's error, not rendering's.
func TestAssembleRefusesARuleWhoseHTMLWouldPassTheBudget(t *testing.T) {
	refuse := rendering{markdown: func(string, MarkdownSource, int64) (string, int64, error) { return "", 0, ErrOverAllowance }}

	_, err := Assemble(repo, []ReleaseSnapshot{first(t)}, limits, refuse)

	if want := fmt.Sprintf("release/1: practices/testing/check-retry-backoff.md: the library's rules and groups hold more than %d bytes of content", limits.ContentBytes); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("got error %v, want one containing %q", err, want)
	}
}

// A body whose links would pass what's left of the budget is refused with the budget's error, as one whose HTML would.
func TestAssembleRefusesARuleWhoseLinksWouldPassTheBudget(t *testing.T) {
	refuse := rendering{links: func(string, int64) ([]string, int64, error) { return nil, 0, ErrOverAllowance }}

	_, err := Assemble(repo, []ReleaseSnapshot{first(t)}, limits, refuse)

	if want := fmt.Sprintf("release/1: practices/testing/check-retry-backoff.md: the library's rules and groups hold more than %d bytes of content", limits.ContentBytes); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("got error %v, want one containing %q", err, want)
	}
}

// The links assembly finds in a rule spend the budget, each destination once, however many links name it.
func TestAssembleSpendsTheBudgetOnTheLinksItFinds(t *testing.T) {
	release := withAssets(t, "See [a](gone.md), [b](gone.md), and [c](gone.md).", nil)
	without, err := Assemble(repo, []ReleaseSnapshot{release}, assetLimits, markup{})
	if err != nil {
		t.Fatal(err)
	}
	budget := assetLimits

	budget.ContentBytes = contentBytes(without) + int64(len("gone.md"))
	if _, err := Assemble(repo, []ReleaseSnapshot{release}, budget, markup{}); err != nil {
		t.Fatalf("within the budget: %v", err)
	}

	budget.ContentBytes--
	_, err = Assemble(repo, []ReleaseSnapshot{release}, budget, markup{})
	if want := fmt.Sprintf("more than %d bytes of content", budget.ContentBytes); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("a byte over the budget: got error %v, want %q", err, want)
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
	var pages []MarkdownSource
	var texts []string
	var allowances []int64
	record := rendering{markdown: func(body string, page MarkdownSource, allowance int64) (string, int64, error) {
		pages, texts, allowances = append(pages, page), append(texts, body), append(allowances, allowance)
		return markup{}.Markdown(body, page, allowance)
	}}

	if _, err := Assemble(repo, []ReleaseSnapshot{first(t), second}, limits, record); err != nil {
		t.Fatal(err)
	}

	source := func(rule, title, tag string) MarkdownSource {
		return MarkdownSource{Repository: LibraryPlaceholder, File: RuleFile(rule), Rule: rule, Title: title, Tag: tag, LatestTag: "release/2"}
	}
	backoff := source("practices/testing/check-retry-backoff", "Check retry backoff", "release/1")
	retryLimits := source("practices/testing/verify-retry-limits", "Verify retry limits", "release/2")
	returnErrors := source("techs/go/return-errors", "Return errors", "release/1")
	// Each rule's body, then its reading guidance, which is Markdown too.
	if want := []MarkdownSource{backoff, backoff, retryLimits, retryLimits, returnErrors, returnErrors}; !reflect.DeepEqual(pages, want) {
		t.Errorf("rendered for %+v, want %+v", pages, want)
	}
	if !strings.Contains(texts[0], "## Check retry backoff") || texts[1] != "When changing check retry backoff." {
		t.Errorf("rendered %q", texts[:2])
	}
	if !slices.IsSortedFunc(allowances, func(a, b int64) int { return int(b - a) }) || allowances[0] >= limits.ContentBytes {
		t.Errorf("allowances are %v, want what's left of the budget, shrinking", allowances)
	}
}
