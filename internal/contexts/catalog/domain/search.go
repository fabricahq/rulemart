// Read the words a visitor searches the catalog for.

package domain

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxSearchQueryLength is the most characters a search may hold. It's ample for words a visitor types, and bounds
// what a crafted URL makes the database parse.
const MaxSearchQueryLength = 200

// SearchQuery is the text a visitor searches the catalog for, cleaned so the database accepts it: words separated by
// single spaces, without control or invisible format characters or invalid UTF-8. Its zero value is no query.
// ParseSearchQuery is the only way to make one.
type SearchQuery struct {
	text string
}

// ParseSearchQuery cleans text into a query: it treats each control character, each invisible format character such
// as a zero-width space, and each byte of invalid UTF-8 as a space, then trims the spaces and collapses each run of them to one. The query keeps the visitor's punctuation,
// which search reads as its syntax: quotes for a phrase, - to exclude a word, and or.
func ParseSearchQuery(text string) SearchQuery {
	var cleaned strings.Builder
	for len(text) > 0 {
		r, size := utf8.DecodeRuneInString(text)
		if (r == utf8.RuneError && size == 1) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			r = ' '
		}
		cleaned.WriteRune(r)
		text = text[size:]
	}
	return SearchQuery{text: strings.Join(strings.Fields(cleaned.String()), " ")}
}

// String returns the query's text, which is empty for the zero query.
func (q SearchQuery) String() string { return q.text }

// IsZero reports whether the query has no words.
func (q SearchQuery) IsZero() bool { return q.text == "" }

// TooLong reports whether the query holds more than MaxSearchQueryLength characters, so search won't run it.
func (q SearchQuery) TooLong() bool { return utf8.RuneCountInString(q.text) > MaxSearchQueryLength }
