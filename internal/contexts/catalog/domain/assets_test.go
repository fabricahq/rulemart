package domain

import (
	"bytes"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// assetLimits keep at most 10 bytes of an asset, and 15 of a rule's own assets together, or of the shared ones.
var assetLimits = ContentLimits{FileBytes: 1 << 20, ContentBytes: 1 << 30, AssetBytes: 10, RuleAssetBytes: 15, Assets: 100}

// linkLimits keep Markdown files long enough to link, at most 100 bytes of each, and 60 of a rule's or the shared
// ones together.
var linkLimits = ContentLimits{FileBytes: 1 << 20, ContentBytes: 1 << 30, AssetBytes: 100, RuleAssetBytes: 60, Assets: 100}

// assetRecord publishes one rule, techs/go/return-errors.
const assetRecord = `formatVersion: 1
release: 1
rules: {techs/go/return-errors: 1.0.0}
changes: {techs/go/return-errors: {change: new, summaries: [Add the rule.]}}
`

// withAssets returns release/1 of a library whose one rule, techs/go/return-errors, has body, and more files.
func withAssets(t *testing.T, body string, more files) ReleaseSnapshot {
	t.Helper()
	f := library(files{"techs/go/_group.yaml": group("Go"), "techs/go/return-errors.md": rule("Return errors", body)})
	for path, content := range more {
		f[path] = content
	}
	return snapshot(t, 1, assetRecord, f)
}

// assetsOf returns the library's assets by path.
func assetsOf(lib Library) map[string]Asset {
	assets := map[string]Asset{}
	for _, a := range lib.Assets {
		assets[a.Path] = a
	}
	return assets
}

// A rule's own files are listed whatever their size or type, and their bytes kept, in path order, while each is
// within the cap for a file, and together within the cap for a rule; a file that's neither an image nor text is only
// listed.
func TestAssembleKeepsAssetBytesWithinTheCaps(t *testing.T) {
	const dir = "techs/go/assets/return-errors/"
	release := withAssets(t, "Return errors.", files{
		dir + "a.txt":       "eight b.",          // kept: 8 of 15
		dir + "b.txt":       "eight b.",          // past the rule's cap: 16 of 15
		dir + "c.svg":       "<svg>past</svg>.",  // past the file's cap: 16 of 10
		dir + "d.png":       "5 b.png",           // kept: 15 of 15
		dir + "e.bin":       "\x00\x01",          // neither an image nor text
		dir + "f/g.md":      "# G",               // within both caps, but the rule's are spent
		"techs/go/other.md": "not an asset file", // beside the directory, not in it
	})

	lib, err := Assemble(repo, []ReleaseSnapshot{release}, assetLimits, markup{})

	if err != nil {
		t.Fatal(err)
	}
	want := []string{dir + "a.txt", dir + "b.txt", dir + "c.svg", dir + "d.png", dir + "e.bin", dir + "f/g.md"}
	if got := lib.Rules[0].Assets; !slices.Equal(got, want) {
		t.Fatalf("the rule lists %q, want %q", got, want)
	}
	assets := assetsOf(lib)
	for path, tc := range map[string]struct {
		size      int64
		mediaType string
		kept      bool
		html      string
	}{
		dir + "a.txt":  {8, "text/plain; charset=utf-8", true, "<pre>eight b.</pre>\n"},
		dir + "b.txt":  {8, "application/octet-stream", false, ""},
		dir + "c.svg":  {16, "image/svg+xml", false, ""},
		dir + "d.png":  {7, "image/png", true, ""},
		dir + "e.bin":  {2, "application/octet-stream", false, ""},
		dir + "f/g.md": {3, "text/markdown; charset=utf-8", false, ""},
	} {
		a := assets[path]
		if a.Size != tc.size || a.MediaType != tc.mediaType || (a.Content != nil) != tc.kept || a.HTML != tc.html || a.Release != 1 {
			t.Errorf("%s is %+v, want size %d, %s, kept %t, HTML %q", path, a, tc.size, tc.mediaType, tc.kept, tc.html)
		}
	}
	if !bytes.Equal(assets[dir+"d.png"].Content, []byte("5 b.png")) {
		t.Errorf("kept %q of d.png", assets[dir+"d.png"].Content)
	}
}

// The library-root files a rule links to are its shared assets, and so are those its own Markdown files link to, and
// those a shared Markdown file links to in turn, each read once at the latest release; a shared file nothing links
// to isn't one, and a link to a file the release doesn't have stays a link.
func TestAssembleFindsSharedAssetsThroughLinks(t *testing.T) {
	release := withAssets(t, "See the [glossary](../../assets/glossary.md), [the gone file](../../assets/gone.md), and [z](../../assets/z.md).", files{
		"techs/go/assets/return-errors/why.md": "Read [naming](../../../../assets/naming/tests.md#names).", // 56 bytes
		"assets/glossary.md":                   "Terms. See [checklist](checklist.md).",                    // 37 bytes
		"assets/z.md":                          "Last word.",                                               // 10 bytes
		"assets/naming/tests.md":               "Name tests.",                                              // 11 bytes
		"assets/checklist.md":                  "Check.",                                                   // 6 bytes
		"assets/unlinked.md":                   "No rule links here.",
	})

	lib, err := Assemble(repo, []ReleaseSnapshot{release}, linkLimits, markup{})

	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"techs/go/assets/return-errors/why.md", "assets/checklist.md", "assets/glossary.md", "assets/naming/tests.md", "assets/z.md",
	}
	if got := lib.Rules[0].Assets; !slices.Equal(got, want) {
		t.Fatalf("the rule lists %q, want %q", got, want)
	}
	if got := len(lib.Assets); got != 5 {
		t.Errorf("the library has %d assets, want 5: %+v", got, lib.Assets)
	}
	// The shared files' cap is their own, 60 bytes together, kept in the order they're found: the rule's links, then
	// its own Markdown's, then each shared file's.
	assets := assetsOf(lib)
	for path, kept := range map[string]bool{
		"assets/glossary.md": true, "assets/z.md": true, "assets/naming/tests.md": true, "assets/checklist.md": false,
		"techs/go/assets/return-errors/why.md": true,
	} {
		if got := assets[path].Content != nil; got != kept {
			t.Errorf("%s kept %t, want %t", path, got, kept)
		}
	}
}

// Shared files are found once for every rule that links to them, and a rule's page lists only the ones it links to.
func TestAssembleListsEachRulesOwnSharedAssets(t *testing.T) {
	release := snapshot(t, 1, firstRecord, library(files{
		"practices/testing/_group.yaml":            group("Testing"),
		"techs/go/_group.yaml":                     group("Go"),
		"practices/testing/check-retry-backoff.md": rule("Check retry backoff", "See [a](../../assets/a.md)."),
		"practices/testing/verify-retry-limits.md": rule("Verify retry limits", "See [a](../../assets/a.md) and [b](../../assets/b.md)."),
		"techs/go/return-errors.md":                rule("Return errors", "Nothing shared."),
		"assets/a.md":                              "A.",
		"assets/b.md":                              "B.",
	}))

	lib, err := Assemble(repo, []ReleaseSnapshot{release}, assetLimits, markup{})

	if err != nil {
		t.Fatal(err)
	}
	for i, want := range [][]string{{"assets/a.md"}, {"assets/a.md", "assets/b.md"}, nil} {
		if got := lib.Rules[i].Assets; !slices.Equal(got, want) {
			t.Errorf("%s lists %q, want %q", lib.Rules[i].Path, got, want)
		}
	}
	if len(lib.Assets) != 2 {
		t.Errorf("the library has %+v, want each shared file once", lib.Assets)
	}
}

// A rule's own files are its current version's, at the release that published it, which covers them; shared files
// are the latest release's. A retired rule lists none.
func TestAssembleReadsAssetsAtTheReleaseThatHoldsThem(t *testing.T) {
	first := withAssets(t, "See the [glossary](../../assets/glossary.md).", files{
		"techs/go/assets/return-errors/x.txt": "old",
		"assets/glossary.md":                  "Old terms.",
	})
	second := snapshot(t, 2, `formatVersion: 1
release: 2
rules: {techs/go/return-errors: 1.0.0}
libraryFiles: [assets/glossary.md]
`, library(files{
		"techs/go/_group.yaml":                group("Go"),
		"techs/go/return-errors.md":           rule("Return errors", "See the [glossary](../../assets/glossary.md)."),
		"techs/go/assets/return-errors/x.txt": "an edit no release published",
		"assets/glossary.md":                  "New terms.",
	}))

	lib, err := Assemble(repo, []ReleaseSnapshot{first, second}, assetLimits, markup{})

	if err != nil {
		t.Fatal(err)
	}
	assets := assetsOf(lib)
	if own := assets["techs/go/assets/return-errors/x.txt"]; string(own.Content) != "old" || own.Release != 1 {
		t.Errorf("the rule's own file is %+v, want release/1's", own)
	}
	if shared := assets["assets/glossary.md"]; string(shared.Content) != "New terms." || shared.Release != 2 {
		t.Errorf("the shared file is %+v, want release/2's", shared)
	}
}

// A Markdown file among a rule's assets renders from its own place: its links resolve against it, and lead to the
// rule's other assets' pages. Its images load from Rulemart only when Rulemart keeps them, and a shared Markdown file
// renders once, with no rule's release.
func TestAssembleRendersMarkdownAssetsWithTheirLinks(t *testing.T) {
	const dir = "techs/go/assets/return-errors/"
	release := withAssets(t, "![loop](assets/return-errors/loop.svg) [why](assets/return-errors/why.md)", files{
		dir + "loop.svg":     "<svg/>",
		dir + "why.md":       "See [the glossary](../../../../assets/glossary.md).",
		dir + "big.png":      strings.Repeat("x", 101),
		"assets/glossary.md": "Terms.",
	})
	sources := map[string]MarkdownSource{}
	record := rendering{markdown: func(body string, source MarkdownSource, allowance int64) (string, int64, error) {
		if _, seen := sources[source.File]; !seen {
			sources[source.File] = source
		}
		return markup{}.Markdown(body, source, allowance)
	}}

	lib, err := Assemble(repo, []ReleaseSnapshot{release}, linkLimits, record)

	if err != nil {
		t.Fatal(err)
	}
	addresses := map[string]AssetAddress{
		dir + "big.png": {Page: "/example/rules/techs/go/return-errors/assets/big.png"},
		dir + "loop.svg": {
			Page:  "/example/rules/techs/go/return-errors/assets/loop.svg",
			Image: "/example/rules/techs/go/return-errors/assets/loop.svg?raw=1",
		},
		dir + "why.md":       {Page: "/example/rules/techs/go/return-errors/assets/why.md"},
		"assets/glossary.md": {Page: "/example/rules/assets/glossary.md"},
	}
	ruleSource := MarkdownSource{
		Repository: "example/rules", File: "techs/go/return-errors.md", Rule: "techs/go/return-errors", Title: "Return errors",
		Tag: "release/1", LatestTag: "release/1", Assets: addresses,
	}
	why := ruleSource
	why.File, why.Title = dir+"why.md", ""
	glossary := MarkdownSource{
		Repository: "example/rules", File: "assets/glossary.md", Tag: "release/1", LatestTag: "release/1",
		Assets: map[string]AssetAddress{"assets/glossary.md": addresses["assets/glossary.md"]},
	}
	for file, want := range map[string]MarkdownSource{ruleSource.File: ruleSource, why.File: why, glossary.File: glossary} {
		if got := sources[file]; !reflect.DeepEqual(got, want) {
			t.Errorf("rendered %s from\n%+v, want\n%+v", file, got, want)
		}
	}
	if html := assetsOf(lib)[dir+"why.md"].HTML; html != "<p>See [the glossary](../../../../assets/glossary.md).</p>\n" {
		t.Errorf("why.md's HTML is %q", html)
	}
}

// Tags are listed in the frontmatter's order, each once, from text between commas or an array.
func TestAssembleKeepsEachVersionsTags(t *testing.T) {
	for name, tc := range map[string]struct {
		tags string
		want []string
	}{
		"text between commas": {"tags: go, errors ,, go\n", []string{"go", "errors"}},
		"an array":            {"tags: [errors, \"wrapping, context\"]\n", []string{"errors", "wrapping, context"}},
		"none":                {"", []string{}},
	} {
		t.Run(name, func(t *testing.T) {
			file := strings.Replace(rule("Return errors", "Return errors."), "impact: HIGH\n", "impact: HIGH\n"+tc.tags, 1)
			release := snapshot(t, 1, assetRecord, library(files{"techs/go/_group.yaml": group("Go"), "techs/go/return-errors.md": file}))

			lib, err := Assemble(repo, []ReleaseSnapshot{release}, assetLimits, markup{})

			if err != nil {
				t.Fatal(err)
			}
			if got := lib.Rules[0].Current().Content.Tags; !slices.Equal(got, tc.want) || got == nil {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// A file exactly at the cap for a file is kept, and one a byte past it isn't, though the rule's cap has room for it.
func TestAssembleKeepsAnAssetExactlyAtTheCapForAFile(t *testing.T) {
	const dir = "techs/go/assets/return-errors/"
	release := withAssets(t, "Return errors.", files{
		dir + "a.txt": "eleven b...", // past the file's cap: 11 of 10
		dir + "b.txt": "ten bytes.",  // at the file's cap: 10 of 10
	})

	lib, err := Assemble(repo, []ReleaseSnapshot{release}, assetLimits, markup{})

	if err != nil {
		t.Fatal(err)
	}
	assets := assetsOf(lib)
	if a := assets[dir+"a.txt"]; a.Content != nil {
		t.Errorf("kept %q of a file a byte past the cap", a.Content)
	}
	if b := assets[dir+"b.txt"]; string(b.Content) != "ten bytes." {
		t.Errorf("kept %q of a file at the cap, want all of it", b.Content)
	}
}

// The bytes kept of assets and the HTML of the ones rendered are held until the library is stored, like rules'
// content, so they spend the budget.
func TestAssembleSpendsTheBudgetOnAssets(t *testing.T) {
	const dir = "techs/go/assets/return-errors/"
	without, err := Assemble(repo, []ReleaseSnapshot{withAssets(t, "Return errors.", nil)}, assetLimits, markup{})
	if err != nil {
		t.Fatal(err)
	}
	release := withAssets(t, "Return errors.", files{
		dir + "a.txt": "eight b.", // 8 bytes, and 20 of HTML: <pre>eight b.</pre>\n
		dir + "b.png": "5 b.png",  // 7 bytes, and no HTML
	})
	budget := assetLimits

	budget.ContentBytes = contentBytes(without) + 8 + 20 + 7
	if _, err := Assemble(repo, []ReleaseSnapshot{release}, budget, markup{}); err != nil {
		t.Fatalf("within the budget: %v", err)
	}

	budget.ContentBytes--
	_, err = Assemble(repo, []ReleaseSnapshot{release}, budget, markup{})
	if want := fmt.Sprintf("more than %d bytes of content", budget.ContentBytes); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("a byte over the budget: got error %v, want %q", err, want)
	}
}

// Shared files count toward the limit on assets with the rule's own, so a library whose rules link to more is refused.
func TestAssembleRefusesMoreSharedAssetsThanTheLimit(t *testing.T) {
	release := withAssets(t, "See [a](../../assets/a.md) and [b](../../assets/b.md).", files{
		"techs/go/assets/return-errors/own.txt": "own",
		"assets/a.md":                           "A.",
		"assets/b.md":                           "B.",
	})
	few := assetLimits
	few.Assets = 2

	_, err := Assemble(repo, []ReleaseSnapshot{release}, few, markup{})

	if err == nil || !strings.Contains(err.Error(), "more than 2 assets") {
		t.Fatalf("got %v, want a refusal past 2 assets", err)
	}
}

// A library whose rules have more assets than the limit is refused, rather than listing them all.
func TestAssembleRefusesMoreAssetsThanTheLimit(t *testing.T) {
	more := files{}
	for _, name := range []string{"a", "b", "c"} {
		more["techs/go/assets/return-errors/"+name+".txt"] = name
	}
	few := assetLimits
	few.Assets = 2

	_, err := Assemble(repo, []ReleaseSnapshot{withAssets(t, "Return errors.", more)}, few, markup{})

	if err == nil || !strings.Contains(err.Error(), "more than 2 assets") {
		t.Fatalf("got %v, want a refusal past 2 assets", err)
	}
}

// fanOut is a release's files plus a directory that holds more files than any limit, as a Git tree whose directories
// share one subtree does. Listing it returns as many as asked for, and one more, and records the most asked for.
type fanOut struct {
	files
	dir   string
	asked *int
}

func (f fanOut) List(dir string, max int) ([]string, error) {
	if dir != f.dir {
		return f.files.List(dir, max)
	}
	*f.asked = max
	paths := make([]string, max+1)
	for i := range paths {
		paths[i] = fmt.Sprintf("%s%07d.txt", dir, i)
	}
	return paths, nil
}

// A rule's asset directory is listed only as far as the assets the limit leaves room for, after the ones other rules
// listed, so a directory holding more files than memory does is refused rather than listed.
func TestAssembleListsARulesAssetsOnlyAsFarAsTheLimitAllows(t *testing.T) {
	release := first(t)
	f := release.Files.(files)
	f["practices/testing/assets/check-retry-backoff/a.txt"] = "a"
	f["practices/testing/assets/check-retry-backoff/b.txt"] = "b"
	var asked int
	release.Files = fanOut{files: f, dir: "techs/go/assets/return-errors/", asked: &asked}
	few := assetLimits
	few.Assets = 5

	_, err := Assemble(repo, []ReleaseSnapshot{release}, few, markup{})

	if err == nil || !strings.Contains(err.Error(), "more than 5 assets") {
		t.Fatalf("got %v, want a refusal past 5 assets", err)
	}
	if asked != 3 {
		t.Errorf("listed the directory that fans out up to %d files, want 3, what's left of the limit", asked)
	}
}
