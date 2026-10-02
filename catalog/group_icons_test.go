package catalog

import (
	"io/fs"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
)

// icons is where the web server's static files keep the vendored icons that group-icons.yaml names.
const icons = "../internal/platform/web/static/icons"

// A named icon that isn't vendored shows a broken image, and a vendored icon nobody names ships unused.
func TestGroupIconsNameExactlyTheVendoredIcons(t *testing.T) {
	entries, err := parseGroupIcons(groupIconsYAML)
	if err != nil {
		t.Fatal(err)
	}
	named := map[string]bool{}
	for _, icon := range entries {
		named[icon.File] = true
	}
	vendored := map[string]bool{}
	err = filepath.WalkDir(icons, func(path string, entry fs.DirEntry, err error) error {
		if ext := filepath.Ext(path); err == nil && (ext == ".svg" || ext == ".png") {
			file, _ := filepath.Rel(icons, path)
			vendored[filepath.ToSlash(file)] = true
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, file := range slices.Sorted(maps.Keys(named)) {
		if !vendored[file] {
			t.Errorf("group-icons.yaml names %s, which isn't in %s", file, icons)
		}
	}
	for _, file := range slices.Sorted(maps.Keys(vendored)) {
		if !named[file] {
			t.Errorf("%s/%s is vendored, but group-icons.yaml doesn't name it", icons, file)
		}
	}
}

func TestParseGroupIconsReadsEachGroupsIcon(t *testing.T) {
	got, err := parseGroupIcons([]byte("# Icons.\npractices/testing:\n  file: lucide/flask-conical.svg\n  monochrome: true\n" +
		"techs/go:\n  file: devicon/go-original.svg\n  narrow: true\n" + "techs/goose:\n  file: community/goose.png\n" + "techs/zustand:\n  file: devicon/zustand-original.svg\n  lightTile: true\n"))
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]domain.GroupIcon{
		"practices/testing": {File: "lucide/flask-conical.svg", Monochrome: true},
		"techs/go":          {File: "devicon/go-original.svg", Narrow: true},
		"techs/goose":       {File: "community/goose.png"},
		"techs/zustand":     {File: "devicon/zustand-original.svg", LightTile: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v; want %+v", got, want)
	}
}

func TestParseGroupIconsRejectsAmbiguousEntries(t *testing.T) {
	for name, input := range map[string]string{
		"an unknown field":                      "techs/go: {file: devicon/go.svg, color: blue}\n",
		"no file":                               "techs/go: {monochrome: true}\n",
		"a file outside the icons":              "techs/go: {file: ../favicon.svg}\n",
		"a file outside a set":                  "techs/go: {file: go.svg}\n",
		"a URL":                                 "techs/go: {file: 'https://example.com/go.svg'}\n",
		"a file that isn't an SVG or PNG":       "techs/go: {file: devicon/go.gif}\n",
		"a repeated group":                      "techs/go: {file: devicon/go.svg}\ntechs/go: {file: devicon/go.svg}\n",
		"groups out of order":                   "techs/go: {file: devicon/go.svg}\npractices/testing: {file: lucide/flask-conical.svg}\n",
		"a monochrome that isn't true or false": "techs/go: {file: devicon/go.svg, monochrome: sometimes}\n",
		"a monochrome icon on a light tile":     "techs/go: {file: devicon/go.svg, monochrome: true, lightTile: true}\n",
		"a second document":                     "techs/go: {file: devicon/go.svg}\n---\ntechs/rust: {file: devicon/rust.svg}\n",
		"a list":                                "- techs/go\n",
		"nothing":                               "",
	} {
		t.Run(name, func(t *testing.T) {
			if icons, err := parseGroupIcons([]byte(input)); err == nil {
				t.Fatalf("accepted %+v", icons)
			}
		})
	}
}

// The icons file must name only canonical groups, or the web function couldn't start.
func TestCanonicalGroupsGivesTheShippedIconsToCanonicalGroups(t *testing.T) {
	groups, err := CanonicalGroups()
	if err != nil {
		t.Fatal(err)
	}

	if group, _ := groups.Find("practices/testing"); group.Icon.File == "" {
		t.Errorf("practices/testing has no icon: %+v", group)
	}
}
