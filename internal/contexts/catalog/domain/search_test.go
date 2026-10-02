package domain

import (
	"slices"
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

func TestSearchQueryTermsReadWordsPhrasesAlternativesAndExclusions(t *testing.T) {
	word := func(text string) SearchTerm { return SearchTerm{Text: text, Query: text} }
	for text, want := range map[string]struct{ find, exclude []SearchTerm }{
		"retry limits": {find: []SearchTerm{word("retry"), word("limits")}},
		`"retry limits" jitter`: {find: []SearchTerm{
			{Text: `"retry limits"`, Query: `"retry limits"`}, word("jitter"),
		}},
		// An unclosed quote runs to the end, as websearch_to_tsquery reads it.
		`jitter "retry limits`: {find: []SearchTerm{word("jitter"), {Text: `"retry limits"`, Query: `"retry limits"`}}},
		// A word that joins words with -, /, or : is a phrase of them that also matches the IDs pages show.
		"keep-tests-independent": {find: []SearchTerm{
			{Text: "keep-tests-independent", Query: `"keep tests independent"`, IdentifierQuery: `"keep tests independent"`},
		}},
		"fabricahq/public-rules:practices/testing/keep-tests-independent": {find: []SearchTerm{{
			Text:            "fabricahq/public-rules:practices/testing/keep-tests-independent",
			Query:           `"fabricahq public rules practices testing keep tests independent"`,
			IdentifierQuery: `"fabricahq public rules practices testing keep tests independent"`,
		}}},
		`"techs/go errors"`: {find: []SearchTerm{{Text: `"techs/go errors"`, Query: `"techs go errors"`, IdentifierQuery: `"techs go errors"`}}},
		// A hyphen leaves a word out only at the start of a word.
		"testing -react": {find: []SearchTerm{word("testing")}, exclude: []SearchTerm{word("react")}},
		`retry -"error boundary" --techs/react`: {
			find: []SearchTerm{word("retry")},
			exclude: []SearchTerm{
				{Text: `"error boundary"`, Query: `"error boundary"`},
				{Text: "techs/react", Query: `"techs react"`, IdentifierQuery: `"techs react"`},
			},
		},
		`retry"jitter"`: {find: []SearchTerm{word("retry"), {Text: `"jitter"`, Query: `"jitter"`}}},
		`"a"-b`:         {find: []SearchTerm{{Text: `"a"`, Query: `"a"`}, word("b")}},
		// or joins the terms on either side into one that either satisfies.
		"retry or jitter limits": {find: []SearchTerm{{Text: "retry or jitter", Query: "retry or jitter"}, word("limits")}},
		// Only an alternative that joins words matches IDs.
		"a OR b or public-rules": {find: []SearchTerm{{Text: "a or b or public-rules", Query: `a or b or "public rules"`, IdentifierQuery: `"public rules"`}}},
		"techs/go or x or retry-limits": {find: []SearchTerm{{Text: "techs/go or x or retry-limits", Query: `"techs go" or x or "retry limits"`,
			IdentifierQuery: `"techs go" or "retry limits"`}}},
		// or with nothing to join on one side is a word, which search ignores as it does "the".
		"or retry":    {find: []SearchTerm{word("or"), word("retry")}},
		"retry or":    {find: []SearchTerm{word("retry"), word("or")}},
		"retry or -x": {find: []SearchTerm{word("retry"), word("or")}, exclude: []SearchTerm{word("x")}},
		// Separators and hyphens on their own, and empty phrases, hold no word.
		`- -- / : "" -""`: {},
		"":                {},
	} {
		find, exclude := ParseSearchQuery(text).Terms()
		if !slices.Equal(find, want.find) || !slices.Equal(exclude, want.exclude) {
			t.Errorf("Terms of %q = %+v, %+v; want %+v, %+v", text, find, exclude, want.find, want.exclude)
		}
	}
}
