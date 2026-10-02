package web

import (
	"context"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/lib/textdiff"
)

func TestEveryImpactLevelHasAnExplanation(t *testing.T) {
	for _, level := range []string{"CRITICAL", "HIGH", "MEDIUM-HIGH", "MEDIUM", "LOW-MEDIUM", "LOW"} {
		if got := impactExplanation(level); !strings.HasSuffix(got, ".") || strings.HasPrefix(got, "Impact") {
			t.Errorf("impactExplanation(%q) = %q, want a sentence specific to the level", level, got)
		}
	}
	if got := impactExplanation("SEVERE"); got != "SEVERE impact, as the library declares it." {
		t.Errorf("an unknown level got %q", got)
	}
}

// A long ID wraps after a / or : first, keeping the parts between them whole when they fit, and shows every character
// it holds, escaped, in order.
func TestBreakableLetsAnIDWrapAtItsParts(t *testing.T) {
	part := func(text string) string { return `<span class="id-part">` + text + "</span>" }
	for text, want := range map[string]string{
		"fabricahq/public-rules:techs/go": part("fabricahq/") + "<wbr>" + part("public-rules:") + "<wbr>" + part("techs/") + "<wbr>" + part("go"),
		"use_template.md":                 part("use_<wbr>template.md"),
		"plain":                           part("plain"),
		"trailing/":                       part("trailing/"),
		"/x":                              part("/") + "<wbr>" + part("x"),
		"<b>&/x":                          part("&lt;b&gt;&amp;/") + "<wbr>" + part("x"),
		"":                                part(""),
	} {
		var out strings.Builder
		if err := breakable(text).Render(context.Background(), &out); err != nil || out.String() != want {
			t.Errorf("breakable(%q) = %q, %v; want %q", text, out.String(), err, want)
		}
	}
}

// A mark right after another, with no text between them, is set apart from it; one after a space keeps only the
// space, so the gap before it is no wider than any other.
func TestSegmentsSetApartOnlyAdjacentMarks(t *testing.T) {
	var out strings.Builder
	err := segments([]textdiff.Segment{
		{Op: textdiff.Equal, Text: "Log "}, {Op: textdiff.Insert, Text: "them"}, {Op: textdiff.Equal, Text: " and "},
		{Op: textdiff.Delete, Text: "continue."}, {Op: textdiff.Insert, Text: "carry on."},
	}).Render(context.Background(), &out)
	if err != nil {
		t.Fatal(err)
	}
	if want := `Log <ins>them</ins> and <del>continue.</del><ins class="g">carry on.</ins>`; out.String() != want {
		t.Errorf("got %s, want %s", out.String(), want)
	}
}
