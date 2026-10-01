package web

import (
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
