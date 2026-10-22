package web

import (
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// Once a check is done, its first step names the manifest it found where the latest library release keeps it:
// .trailguide/config.yaml for a library in trailguide's directory, and rule-library.yaml for one at the root.
func TestADoneCheckNamesTheManifestOfTheLatestRelease(t *testing.T) {
	for _, c := range []struct {
		layouts domain.ReleaseLayouts
		want    string
	}{
		{domain.ReleaseLayouts{1: domain.RootLayout, 3: domain.TrailGuideLayout}, ".trailguide/config.yaml"},
		{domain.ReleaseLayouts{1: domain.TrailGuideLayout, 3: domain.RootLayout}, "rule-library.yaml"},
		{domain.ReleaseLayouts{1: domain.RootLayout, 2: domain.CodeRulesLayout}, ".code-rules/config.yaml"},
	} {
		layouts, want := c.layouts, c.want
		lib := &views.Library{LatestRelease: 3, Layouts: layouts}
		steps := runView{fullName: "example/rules", library: lib}.steps()

		if got := steps[0].parts; len(got) != 3 || got[1] != runStrong(want) || got[2].text != " in example/rules" {
			t.Errorf("with layouts %v, the first step is %+v, want it to name %s", layouts, got, want)
		}
	}
}
