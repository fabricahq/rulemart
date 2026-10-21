package domain

import (
	"reflect"
	"strings"
	"testing"
)

// rule-library.yaml declares its license at library paths, such as a shared notice in assets/, and the license
// reads back at those paths.
func TestParseLegacyLicense_ReadsTheLicenseAtLibraryPaths(t *testing.T) {
	text := "formatVersion: 1\nname: Example\nlicense:\n  spdxExpression: MIT\n  file: LICENSE.md\n  notices: [assets/NOTICE.md, LICENSE.md]\n"
	license, err := parseLegacyLicense([]byte(text), "example/rules/rule-library.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if license.SPDXExpression == nil || *license.SPDXExpression != "MIT" || !reflect.DeepEqual(license.Files, []string{"LICENSE.md"}) || !reflect.DeepEqual(license.AttributionFiles, []string{"assets/NOTICE.md"}) {
		t.Errorf("got %+v; want MIT in LICENSE.md, with the notice assets/NOTICE.md", license)
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
