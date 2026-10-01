// Separate a rule's frontmatter envelope from its body without interpreting either.

package coderules

import "regexp"

var documentPattern = regexp.MustCompile(`(?s)^---\r?\n(.*?)\r?\n---(?:\r?\n|$)(.*)$`)

// DocumentText contains exact input text excluding the delimiter lines and their
// adjacent newlines. Either field may be empty; neither is parsed or trimmed.
type DocumentText struct {
	Frontmatter string `json:"frontmatter"`
	Body        string `json:"body"`
}

// SplitDocument separates required YAML frontmatter from Markdown. It validates
// only the delimiters, not YAML syntax, metadata fields, or Markdown content.
// On error, the returned document is the zero value.
func SplitDocument(text, location string) (DocumentText, error) {
	parts := documentPattern.FindStringSubmatch(text)
	if parts == nil {
		return DocumentText{}, invalid(location, "expected YAML frontmatter followed by Markdown")
	}
	return DocumentText{Frontmatter: parts[1], Body: parts[2]}, nil
}
