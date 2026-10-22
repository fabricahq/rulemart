package domain

import (
	"reflect"
	"strings"
	"testing"
)

// rule-library.yaml declares its license at paths from the repository root, which read back as written, even where
// the current layout places or reserves them: .trailguide/ and rules/ hold no library files in RootLayout.
func TestParseLegacyLicense_KeepsTheDeclaredPaths(t *testing.T) {
	for name, tc := range map[string]struct{ file, notice string }{
		"the library root":          {"LICENSE.md", "assets/NOTICE.md"},
		"rules/":                    {"rules/LICENSE.md", "rules/NOTICE.md"},
		".trailguide/":              {".trailguide/assets/LICENSE.md", ".code-rules/NOTICE.md"},
		"a group's README":          {"LICENSE.md", "techs/go/README.md"},
		"a folder beside the rules": {"licenses/LICENSE.md", "techs/NOTICE.md"},
	} {
		t.Run(name, func(t *testing.T) {
			text := "formatVersion: 1\nname: Example\nlicense:\n  spdxExpression: MIT\n  file: " + tc.file + "\n  notices: [" + tc.notice + ", " + tc.file + "]\n"
			license, err := parseLegacyLicense([]byte(text), "example/rules/rule-library.yaml")
			if err != nil {
				t.Fatal(err)
			}
			if license.SPDXExpression == nil || *license.SPDXExpression != "MIT" || !reflect.DeepEqual(license.Files, []string{tc.file}) || !reflect.DeepEqual(license.AttributionFiles, []string{tc.notice}) {
				t.Errorf("got %+v; want MIT in %s, with the notice %s", license, tc.file, tc.notice)
			}
		})
	}
	if none, err := parseLegacyLicense([]byte("formatVersion: 1\n"), "rule-library.yaml"); err != nil || none != nil {
		t.Errorf("got %+v, %v for a manifest without a license; want none", none, err)
	}
}

func TestParseLegacyLicense_RefusesAnInvalidManifest(t *testing.T) {
	for name, tc := range map[string]struct{ text, want string }{
		"another format version": {"formatVersion: 2\n", "rule-library.yaml: formatVersion: only library formatVersion 1 is supported"},
		"the renamed expression": {"formatVersion: 1\nlicense: {expression: MIT, file: LICENSE.md, notices: []}\n", "rule-library.yaml: license.expression: renamed"},
		"a rule's file":          {"formatVersion: 1\nlicense: {file: techs/go/return-errors.md, notices: []}\n", "rule-library.yaml: license.file: "},
		"an unknown field":       {"formatVersion: 1\nlicense: {file: LICENSE.md, notices: [], url: x}\n", "rule-library.yaml: license: unknown field url"},
		"a rule's asset":         {"formatVersion: 1\nlicense: {file: LICENSE.md, notices: [practices/testing/assets/NOTICE.md]}\n", "rule-library.yaml: license.notices[0]: "},
		"a path outside":         {"formatVersion: 1\nlicense: {file: ../LICENSE.md, notices: []}\n", "rule-library.yaml: license.file: expected a contained"},
		"an unknown SPDX ID":     {"formatVersion: 1\nlicense: {spdxExpression: Nope, file: LICENSE.md, notices: []}\n", "rule-library.yaml: license.spdxExpression: "},
		"no notices":             {"formatVersion: 1\nlicense: {file: LICENSE.md}\n", "rule-library.yaml: license.notices: "},
	} {
		t.Run(name, func(t *testing.T) {
			license, err := parseLegacyLicense([]byte(tc.text), "rule-library.yaml")
			if err == nil || !strings.HasPrefix(err.Error(), tc.want) || license != nil {
				t.Errorf("got %+v, %v; want an error starting %q", license, err, tc.want)
			}
		})
	}
}
