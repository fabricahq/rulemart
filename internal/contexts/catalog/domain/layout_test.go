package domain

import "testing"

func TestLayoutRepositoryPath_PlacesLibraryFilesWhereEachLayoutKeepsThem(t *testing.T) {
	for _, tc := range []struct {
		layout     Layout
		file, want string
	}{
		{RootLayout, "techs/go/return-errors.md", "techs/go/return-errors.md"},
		{RootLayout, "assets/lifecycle.svg", "assets/lifecycle.svg"},
		{TrailGuideLayout, "techs/go/return-errors.md", ".trailguide/rules/techs/go/return-errors.md"},
		{TrailGuideLayout, "practices/testing/assets/verify-retry-limits/", ".trailguide/rules/practices/testing/assets/verify-retry-limits/"},
		{TrailGuideLayout, "practices/testing/_group.yaml", ".trailguide/rules/practices/testing/_group.yaml"},
		{TrailGuideLayout, "assets/lifecycle.svg", ".trailguide/assets/lifecycle.svg"},
		{TrailGuideLayout, "config.yaml", ".trailguide/config.yaml"},
		// License and notice files stay at their paths from the repository root.
		{TrailGuideLayout, "LICENSE.md", "LICENSE.md"},
		{CodeRulesLayout, "techs/go/return-errors.md", ".code-rules/rules/techs/go/return-errors.md"},
		{CodeRulesLayout, "assets/lifecycle.svg", ".code-rules/assets/lifecycle.svg"},
		{CodeRulesLayout, "NOTICE.md", "NOTICE.md"},
	} {
		if got := tc.layout.RepositoryPath(tc.file); got != tc.want {
			t.Errorf("%q.RepositoryPath(%q) = %q, want %q", tc.layout, tc.file, got, tc.want)
		}
		if back, ok := tc.layout.LibraryPath(tc.want); !ok || back != tc.file {
			t.Errorf("%q.LibraryPath(%q) = %q, %v, want %q", tc.layout, tc.want, back, ok, tc.file)
		}
	}
}

func TestLayoutLibraryPath_LeavesOutFilesTheLibraryDoesNotPublish(t *testing.T) {
	for _, tc := range []struct {
		layout Layout
		file   string
	}{
		{TrailGuideLayout, ".trailguide/vendor/team/config.yaml"},
		{TrailGuideLayout, ".trailguide/generated/RULES.md"},
		{TrailGuideLayout, ".trailguide/rules/README.md"},
		{TrailGuideLayout, "techs/go/return-errors.md"},
		{TrailGuideLayout, "assets/lifecycle.svg"},
		{CodeRulesLayout, ".code-rules/generated/RULES.md"},
		{CodeRulesLayout, ".code-rules"},
		{CodeRulesLayout, ".trailguide/rules/techs/go/return-errors.md"},
	} {
		if got, ok := tc.layout.LibraryPath(tc.file); ok {
			t.Errorf("%q.LibraryPath(%q) = %q, want no library path", tc.layout, tc.file, got)
		}
	}
}

func TestLayoutManifestFile_NamesEachLayoutsManifest(t *testing.T) {
	for layout, want := range map[Layout]string{
		RootLayout: "rule-library.yaml", TrailGuideLayout: ".trailguide/config.yaml", CodeRulesLayout: ".code-rules/config.yaml",
	} {
		if got := layout.ManifestFile(); got != want {
			t.Errorf("%q.ManifestFile() = %q, want %q", layout, got, want)
		}
	}
}

func TestDetectLayout_FindsWhereTheReleaseKeepsTheLibrary(t *testing.T) {
	for name, tc := range map[string]struct {
		files files
		want  Layout
	}{
		"rule-library.yaml at the root": {library(files{"techs/go/return-errors.md": ""}), RootLayout},
		// A library in the earlier layout could be a project too, whose files were in .trailguide/.
		"rule-library.yaml beside a project's files": {library(files{".trailguide/config.yaml": ""}), RootLayout},
		"the Trail Guide directory":                  {files{".trailguide/config.yaml": "", "LICENSE": ""}, TrailGuideLayout},
		"the directory before the rename":            {files{".code-rules/config.yaml": "", "LICENSE": ""}, CodeRulesLayout},
		// Trail Guide reads .code-rules/ only while there's no .trailguide/.
		"both directories":    {files{".trailguide/rules/techs/go/x.md": "", ".code-rules/config.yaml": ""}, TrailGuideLayout},
		"no library at all":   {files{"README.md": ""}, RootLayout},
		"an empty repository": {files{}, RootLayout},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := DetectLayout(tc.files)
			if err != nil || got != tc.want {
				t.Fatalf("DetectLayout = %q, %v, want %q", got, err, tc.want)
			}
		})
	}
}

func TestLayoutResolveLibraryLink_ResolvesFromWhereTheFileLives(t *testing.T) {
	const rule = "practices/testing/verify-retry-limits.md"
	for _, tc := range []struct {
		layout       Layout
		destination  string
		want         string
		inTheLibrary bool
	}{
		{RootLayout, "../../assets/lifecycle.svg", "assets/lifecycle.svg", true},
		{RootLayout, "/assets/lifecycle.svg", "assets/lifecycle.svg", true},
		{RootLayout, "../../LICENSE", "LICENSE", true},
		// A moved rule sits two folders deeper, so its links to shared files climb one more folder, and to the
		// repository's root two more; a link that starts with / resolves from the Trail Guide directory.
		{TrailGuideLayout, "../../../assets/lifecycle.svg", "assets/lifecycle.svg", true},
		{TrailGuideLayout, "/assets/lifecycle.svg#top", "assets/lifecycle.svg", true},
		{TrailGuideLayout, "../../../../LICENSE", "LICENSE", true},
		{TrailGuideLayout, "assets/verify-retry-limits/example.go", "practices/testing/assets/verify-retry-limits/example.go", true},
		{CodeRulesLayout, "/assets/lifecycle.svg", "assets/lifecycle.svg", true},
		// The old relative link to a shared file now names a folder the library doesn't publish.
		{TrailGuideLayout, "../../assets/lifecycle.svg", "", false},
		{TrailGuideLayout, "../../../vendor/team/config.yaml", "", false},
		{TrailGuideLayout, "../../../../../above-the-root", "", false},
		{TrailGuideLayout, "https://example.com/", "", false},
	} {
		got, ok := tc.layout.ResolveLibraryLink(rule, tc.destination)
		if got != tc.want || ok != tc.inTheLibrary {
			t.Errorf("%q.ResolveLibraryLink(%q) = %q, %v, want %q, %v", tc.layout, tc.destination, got, ok, tc.want, tc.inTheLibrary)
		}
	}
}

func TestParseLayout_AcceptsOnlyTheLayoutsTheCatalogStores(t *testing.T) {
	for _, text := range []string{"", ".trailguide", ".code-rules"} {
		if got, err := ParseLayout(text); err != nil || string(got) != text {
			t.Errorf("ParseLayout(%q) = %q, %v", text, got, err)
		}
	}
	for _, text := range []string{"trailguide", ".trailguide/", "rules"} {
		if _, err := ParseLayout(text); err == nil {
			t.Errorf("ParseLayout(%q) succeeded, want an error", text)
		}
	}
}

func TestReleaseLayoutsOf_KeepsEachLayoutUntilTheNextMove(t *testing.T) {
	moved := ReleaseLayouts{1: RootLayout, 3: CodeRulesLayout, 5: TrailGuideLayout}
	for number, want := range map[int]Layout{1: RootLayout, 2: RootLayout, 3: CodeRulesLayout, 4: CodeRulesLayout, 5: TrailGuideLayout, 9: TrailGuideLayout} {
		if got := moved.Of(number); got != want {
			t.Errorf("release/%d keeps its files in %q, want %q", number, got, want)
		}
	}
	// A library stored before the catalog recorded layouts kept every release at the root.
	if got := (ReleaseLayouts(nil)).Of(4); got != RootLayout {
		t.Errorf("without layouts, release/4 keeps its files in %q, want the root", got)
	}
}
