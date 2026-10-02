package web

import (
	"context"
	"strings"
	"testing"
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

// A long ID wraps after its separators, and shows every character it holds, escaped, in order.
func TestBreakableLetsAnIDWrapAfterItsSeparators(t *testing.T) {
	for text, want := range map[string]string{
		"fabricahq/public-rules:techs/go": "fabricahq/<wbr>public-<wbr>rules:<wbr>techs/<wbr>go",
		"use-template.md":                 "use-<wbr>template.<wbr>md",
		"plain":                           "plain",
		"trailing-":                       "trailing-",
		"-/x":                             "-<wbr>/<wbr>x",
		"<b>&-x":                          "&lt;b&gt;&amp;-<wbr>x",
		"":                                "",
	} {
		var out strings.Builder
		if err := breakable(text).Render(context.Background(), &out); err != nil || out.String() != want {
			t.Errorf("breakable(%q) = %q, %v; want %q", text, out.String(), err, want)
		}
	}
}
