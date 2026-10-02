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
// as a zero-width space, and each byte of invalid UTF-8 as a space, then trims the spaces and collapses each run of
// them to one. The query keeps the visitor's punctuation, which Terms reads as its syntax.
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

// SearchTerm is a word or phrase a search finds, or leaves out.
type SearchTerm struct {
	// Text is the term as the visitor wrote it, such as retry-limits or "retry limits", which a page names when a rule
	// lacks it.
	Text string
	// Query is the term in websearch_to_tsquery's syntax: a word, a quoted phrase, or either joined by or, quoted
	// so that the database reads no other syntax in it.
	Query string
	// IdentifierQuery holds the term's words or phrases that join words with -, /, or :, such as
	// keep-tests-independent or techs/go, in Query's syntax, or is empty when there are none. They match the IDs and
	// names pages show, a rule's, its group's, and its library's, as well as the rule's text. Of retry or techs/go,
	// only techs/go matches IDs.
	IdentifierQuery string
}

// Terms reads the query as the terms a matching rule holds, and the terms it doesn't, each in the visitor's order:
//
//   - Words are separated by spaces, and a quoted phrase is one term. A quote left open runs to the end.
//   - A hyphen leaves the word or phrase after it out, but only at the start of a word.
//   - or joins the terms on either side into one that either satisfies, such as retry or jitter.
//   - Within a word, -, /, and : join words into a phrase, so that keep-tests-independent finds the rule with that
//     ID, and techs/go the rules in that group, but not those in techs/goose.
//
// Terms never fail: punctuation that holds no word is dropped.
func (q SearchQuery) Terms() (find, exclude []SearchTerm) {
	// joining is true after an or that joins the term before it to the next.
	joining := false
	tokens := searchTokens(q.text)
	for i, t := range tokens {
		if t.isOr() && len(find) > 0 && i > 0 && !tokens[i-1].excluded && i+1 < len(tokens) && !tokens[i+1].excluded && !tokens[i+1].isOr() {
			joining = true
			continue
		}
		term, ok := t.term()
		switch {
		case !ok:
			joining = false
		case t.excluded:
			exclude = append(exclude, term)
		case joining:
			last := &find[len(find)-1]
			last.Text += " or " + term.Text
			last.Query += " or " + term.Query
			last.IdentifierQuery = joinAlternatives(last.IdentifierQuery, term.IdentifierQuery)
			joining = false
		default:
			find = append(find, term)
		}
	}
	return find, exclude
}

// searchToken is a word or quoted phrase of a query, as the visitor typed it.
type searchToken struct {
	// text is the word, or the phrase between its quotes.
	text             string
	phrase, excluded bool
}

// searchTokens splits text, a cleaned query, into its words and quoted phrases.
func searchTokens(text string) []searchToken {
	var tokens []searchToken
	afterSpace := true
	for i := 0; i < len(text); {
		if text[i] == ' ' {
			afterSpace = true
			i++
			continue
		}
		excluded := false
		if afterSpace && text[i] == '-' {
			hyphens := i
			for hyphens < len(text) && text[hyphens] == '-' {
				hyphens++
			}
			if hyphens == len(text) || text[hyphens] == ' ' {
				i = hyphens
				continue
			}
			excluded, i = true, hyphens
		}
		afterSpace = false
		if text[i] == '"' {
			body := text[i+1:]
			end := strings.IndexByte(body, '"')
			if end < 0 {
				end = len(body)
				i = len(text)
			} else {
				i += end + 2
			}
			tokens = append(tokens, searchToken{text: body[:end], phrase: true, excluded: excluded})
			continue
		}
		end := i
		for end < len(text) && text[end] != ' ' && text[end] != '"' {
			end++
		}
		tokens = append(tokens, searchToken{text: text[i:end], excluded: excluded})
		i = end
	}
	return tokens
}

// isOr reports whether t is the word or, which joins the terms on either side.
func (t searchToken) isOr() bool { return !t.phrase && !t.excluded && strings.EqualFold(t.text, "or") }

// term returns the search term t is, or false when it holds no word.
func (t searchToken) term() (SearchTerm, bool) {
	words := strings.FieldsFunc(t.text, func(r rune) bool { return r == ' ' || isWordJoiner(r) })
	if len(words) == 0 {
		return SearchTerm{}, false
	}
	joined := strings.ContainsFunc(strings.Trim(t.text, " -/:"), isWordJoiner)
	if t.phrase {
		phrase := `"` + strings.Join(words, " ") + `"`
		return SearchTerm{Text: `"` + strings.TrimSpace(t.text) + `"`, Query: phrase, IdentifierQuery: ifJoined(joined, phrase)}, true
	}
	if len(words) == 1 {
		return SearchTerm{Text: strings.Trim(t.text, "-/:"), Query: words[0]}, true
	}
	phrase := `"` + strings.Join(words, " ") + `"`
	return SearchTerm{Text: strings.Trim(t.text, "-/:"), Query: phrase, IdentifierQuery: ifJoined(joined, phrase)}, true
}

// ifJoined returns query when its words were joined with -, /, or :, and nothing otherwise.
func ifJoined(joined bool, query string) string {
	if joined {
		return query
	}
	return ""
}

// joinAlternatives joins two queries with or, either of which may be empty.
func joinAlternatives(a, b string) string {
	if a == "" || b == "" {
		return a + b
	}
	return a + " or " + b
}

// isWordJoiner reports whether r joins words into one, as in rule IDs, group IDs, and source-qualified IDs.
func isWordJoiner(r rune) bool { return r == '-' || r == '/' || r == ':' }
