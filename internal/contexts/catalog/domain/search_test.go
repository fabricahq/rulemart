package domain

import (
	"strings"
	"testing"
)

func TestParseSearchQueryKeepsTheWordsAVisitorTyped(t *testing.T) {
	for text, want := range map[string]string{
		"retry limits":                  "retry limits",
		"  retry \t\n limits  ":         "retry limits",
		`"exact phrase" -excluded or x`: `"exact phrase" -excluded or x`,
		"<script>alert(1)</script>":     "<script>alert(1)</script>",
		"café über 日本語":                 "café über 日本語",
		// Control characters and invalid UTF-8, which Postgres would refuse, separate words like spaces.
		"retry\x00limits":   "retry limits",
		"retry\x1blimits":   "retry limits",
		"retry\xfflimits":   "retry limits",
		"retry\u0085limits": "retry limits",
		"\x00\xff\x7f":      "",
		// Invisible format characters, such as a zero-width space, would make a query that looks empty.
		"retry\u200blimits": "retry limits",
		"\u200b\ufeff":      "",
		"":                  "",
		"   ":               "",
	} {
		q := ParseSearchQuery(text)
		if q.String() != want || q.IsZero() != (want == "") {
			t.Errorf("ParseSearchQuery(%q) = %q (zero %v), want %q", text, q.String(), q.IsZero(), want)
		}
	}
}

func TestSearchQueryIsTooLongPastItsLimitInCharacters(t *testing.T) {
	for name, tc := range map[string]struct {
		text    string
		tooLong bool
	}{
		"at the limit":                      {strings.Repeat("a", MaxSearchQueryLength), false},
		"one past it":                       {strings.Repeat("a", MaxSearchQueryLength+1), true},
		"at the limit in multibyte letters": {strings.Repeat("é", MaxSearchQueryLength), false},
		"at the limit once spaces collapse": {"  " + strings.Repeat("a ", MaxSearchQueryLength/2) + "   ", false},
		"far past it":                       {strings.Repeat("retry ", 2000), true},
	} {
		t.Run(name, func(t *testing.T) {
			if got := ParseSearchQuery(tc.text).TooLong(); got != tc.tooLong {
				t.Fatalf("TooLong() = %v, want %v", got, tc.tooLong)
			}
		})
	}
}
