package domain

import "testing"

// verifyRetryLimits is the Markdown of a rule whose version release/2 published, in a library whose latest release is
// release/5, with one asset Rulemart keeps as an image and one it shows only on a page.
var verifyRetryLimits = MarkdownSource{
	Repository: "example/rules", File: "practices/testing/verify-retry-limits.md", Rule: "practices/testing/verify-retry-limits",
	Tag: "release/2", LatestTag: "release/5",
	Assets: map[string]AssetAddress{
		"practices/testing/assets/verify-retry-limits/loop.svg": {
			Page:  "/example/rules/practices/testing/verify-retry-limits/assets/loop.svg",
			Image: "/example/rules/practices/testing/verify-retry-limits/assets/loop.svg?raw=1",
		},
		"assets/glossary.md": {Page: "/example/rules/assets/glossary.md"},
	},
}

func TestLinkURLLeadsToAnAssetsPageOrTheFileOnGitHub(t *testing.T) {
	const blob = "https://github.com/example/rules/blob/"
	for destination, want := range map[string]string{
		// A known asset's page keeps the fragment and drops the query its author wrote.
		"../../assets/glossary.md?plain=1#terms": "/example/rules/assets/glossary.md#terms",
		"assets/verify-retry-limits/loop.svg":    "/example/rules/practices/testing/verify-retry-limits/assets/loop.svg",
		// The rule's own file and asset directory are at the rule's release.
		"verify-retry-limits.md#why":           blob + "release/2/practices/testing/verify-retry-limits.md#why",
		"assets/verify-retry-limits/notes.txt": blob + "release/2/practices/testing/assets/verify-retry-limits/notes.txt",
		// Anything else in the library is at the latest release.
		"check-retry-backoff.md":                     blob + "release/5/practices/testing/check-retry-backoff.md",
		"assets/verify-retry-limits-extra/notes.txt": blob + "release/5/practices/testing/assets/verify-retry-limits-extra/notes.txt",
		"/LICENSE":   blob + "release/5/LICENSE",
		"../../../x": "https://github.com/example/rules/tree/release/5",
		// What isn't a path in the repository stays as written.
		"https://code-rules.fabricahq.com/": "https://code-rules.fabricahq.com/",
		"#validation":                       "#validation",
	} {
		if got := verifyRetryLimits.LinkURL(destination); got != want {
			t.Errorf("LinkURL(%q) = %q, want %q", destination, got, want)
		}
	}
}

func TestLinkURLLeadsASharedFilesLinksToTheLatestRelease(t *testing.T) {
	glossary := MarkdownSource{Repository: "example/rules", File: "assets/glossary.md", Tag: "release/5", LatestTag: "release/5"}

	if got, want := glossary.LinkURL("../practices/testing/verify-retry-limits.md"), "https://github.com/example/rules/blob/release/5/practices/testing/verify-retry-limits.md"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestImageURLLoadsAKeptImageFromRulemartAndOthersFromGitHub(t *testing.T) {
	const raw = "https://raw.githubusercontent.com/example/rules/refs/tags/"
	for destination, want := range map[string]string{
		"assets/verify-retry-limits/loop.svg": "/example/rules/practices/testing/verify-retry-limits/assets/loop.svg?raw=1",
		// An asset Rulemart shows on a page but doesn't keep as an image, or a file it doesn't list, loads from GitHub.
		"../../assets/glossary.md":           raw + "release/5/assets/glossary.md",
		"assets/verify-retry-limits/big.png": raw + "release/2/practices/testing/assets/verify-retry-limits/big.png",
		"../../assets/diagram.png":           raw + "release/5/assets/diagram.png",
		"https://example.com/badge.svg":      "https://example.com/badge.svg",
		"../../../above-the-root.png":        "../../../above-the-root.png",
	} {
		if got := verifyRetryLimits.ImageURL(destination); got != want {
			t.Errorf("ImageURL(%q) = %q, want %q", destination, got, want)
		}
	}
}

// Assembly renders links with LibraryPlaceholder, which a page of a library replaces with the library's address:
// links to its pages, its images, and its files on GitHub, raw or not, and nothing else.
func TestLinksForLibraryLeadsWithinTheLibraryAPageShows(t *testing.T) {
	stored := `<p><a href="/_owner_/_name_/assets/glossary.md#terms">glossary</a> <img src="/_owner_/_name_/techs/go/x/assets/a.svg?raw=1" alt="a"> ` +
		`<a href="https://github.com/_owner_/_name_/tree/release/5">repository</a> ` +
		`<img src="https://raw.githubusercontent.com/_owner_/_name_/refs/tags/release/2/a.png" alt="b"> ` +
		`<a href="https://example.com/_owner_/_name_/">elsewhere</a> /_owner_/_name_/ in text</p>`

	got := LinksForLibrary(stored, "stranger/rules")

	want := `<p><a href="/stranger/rules/assets/glossary.md#terms">glossary</a> <img src="/stranger/rules/techs/go/x/assets/a.svg?raw=1" alt="a"> ` +
		`<a href="https://github.com/stranger/rules/tree/release/5">repository</a> ` +
		`<img src="https://raw.githubusercontent.com/stranger/rules/refs/tags/release/2/a.png" alt="b"> ` +
		`<a href="https://example.com/_owner_/_name_/">elsewhere</a> /_owner_/_name_/ in text</p>`
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}
