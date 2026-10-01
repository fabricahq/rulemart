// Read the change summaries a release record lists, and validate rule IDs.

package coderules

import (
	"encoding/json"
	"strconv"
	"strings"
)

// summaryLine returns a change summary: nonblank, valid Unicode text on one line, without surrounding whitespace or
// control characters. A change note has one, and a release record lists one per note. Projects show summaries in
// terminals, where control characters, such as ESC, could rewrite what they display.
func summaryLine(input json.RawMessage, location string) (string, error) {
	text, err := jsonText(input, location)
	if err != nil {
		return "", err
	}
	summary := strings.TrimFunc(text, jsWhitespace)
	if strings.ContainsAny(summary, "\n\r") {
		return "", invalid(location, "expected one line")
	}
	if strings.ContainsFunc(summary, func(r rune) bool { return r < 0x20 || (r >= 0x7f && r <= 0x9f) }) {
		return "", invalid(location, "expected text without control characters, such as tabs or escape sequences")
	}
	return summary, nil
}

// recordSummaries reads a release record's non-empty list of summaries, one per change note, in order.
func recordSummaries(input json.RawMessage, location string) ([]string, error) {
	var items []json.RawMessage
	if json.Unmarshal(input, &items) != nil || len(items) == 0 {
		return nil, invalid(location, "expected a list with one summary per change note")
	}
	summaries := make([]string, len(items))
	for i, item := range items {
		summary, err := summaryLine(item, location+"["+strconv.Itoa(i)+"]")
		if err != nil {
			return nil, err
		}
		summaries[i] = summary
	}
	return summaries, nil
}

// ValidateRuleID accepts a library rule ID: a rule's contained path without its final .md, such as
// practices/testing/verify-retry-limits. It accepts exactly the IDs of valid rule paths, so the ID of a rule file
// named example.md.md is example.md. It checks syntax only; it does not establish that the rule exists.
func ValidateRuleID(id, location string) error {
	_, err := GroupFromPath(id+".md", location)
	return err
}
