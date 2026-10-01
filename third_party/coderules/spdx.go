// Adapt SPDX expression validation while retaining authored spelling and baseline case rules.

package coderules

import (
	"github.com/github/go-spdx/v2/spdxexp"
	"github.com/github/go-spdx/v2/spdxexp/spdxlicenses"
	"regexp"
	"strings"
)

var spdxWord = regexp.MustCompile(`[A-Za-z0-9.-]+`)
var spdxReferenceException = regexp.MustCompile(`((?:DocumentRef-[A-Za-z0-9.-]+:)?LicenseRef-[A-Za-z0-9.-]+) +WITH +([A-Za-z0-9.-]+)`)

// validSPDXExpression delegates grammar to go-spdx, preserving case-sensitive IDs
// and case-insensitive operators from the reference. It never rewrites the output.
func validSPDXExpression(text string) bool {
	// go-spdx v2.7.0 dereferences a missing token after a trailing opening
	// parenthesis or DocumentRef colon. Neither can end a valid expression.
	end := strings.TrimRight(text, " ")
	if strings.HasSuffix(end, "(") || strings.HasSuffix(end, ":") {
		return false
	}
	valid := true
	// Normalize only operator tokens for the dependency; identifiers keep their spelling.
	normalized := spdxWord.ReplaceAllStringFunc(text, func(word string) string {
		upper := strings.ToUpper(word)
		if upper == "AND" || upper == "OR" || upper == "WITH" {
			return upper
		}
		if strings.HasPrefix(word, "LicenseRef-") || strings.HasPrefix(word, "DocumentRef-") {
			return word
		}
		active, canonical := spdxlicenses.IsActiveLicense(word)
		if active && canonical == word {
			return word
		}
		deprecated, canonical := spdxlicenses.IsDeprecatedLicense(word)
		if deprecated && canonical == word {
			return word
		}
		exception, canonical := spdxlicenses.IsException(word)
		if exception && canonical == word {
			return word
		}
		valid = false
		return word
	})
	if !valid {
		return false
	}
	// go-spdx v2.7.0 cannot parse WITH after custom references. Validate the
	// exception separately, then parse the unchanged reference and surrounding grammar.
	normalized = spdxReferenceException.ReplaceAllStringFunc(normalized, withoutReferenceException)
	// ExtractLicenses parses without the validator's whitespace/case normalization shortcuts.
	_, err := spdxexp.ExtractLicenses(normalized)
	return err == nil
}

// withoutReferenceException removes a recognized exception only from the grammar-check copy.
// Invalid exceptions stay intact so the dependency rejects them; authored text is retained.
func withoutReferenceException(text string) string {
	parts := spdxReferenceException.FindStringSubmatch(text)
	if ok, canonical := spdxlicenses.IsException(parts[2]); ok && canonical == parts[2] {
		return parts[1]
	}
	return text
}
