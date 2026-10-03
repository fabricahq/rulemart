// A rule's tags: the topics its frontmatter lists, which its page links to search.

package domain

import (
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// ruleTags returns the topics a rule's file, document, lists under tags in its frontmatter, in its order, each once,
// trimmed: an array's strings, or text's parts between commas, as Code Rules' template writes them. Code Rules' parser
// accepted the file, which checks that tags is one or the other; frontmatter this decoder can't read gives none, since
// tags only lead to search.
func ruleTags(document string) []string {
	parts, err := coderules.SplitDocument(document, "")
	if err != nil {
		return []string{}
	}
	var frontmatter struct {
		Tags yaml.Node `yaml:"tags"`
	}
	if err := yaml.Unmarshal([]byte(parts.Frontmatter), &frontmatter); err != nil {
		return []string{}
	}
	var written []string
	switch frontmatter.Tags.Kind {
	case yaml.ScalarNode:
		written = strings.Split(frontmatter.Tags.Value, ",")
	case yaml.SequenceNode:
		for _, entry := range frontmatter.Tags.Content {
			written = append(written, entry.Value)
		}
	}
	tags, seen := []string{}, map[string]bool{}
	for _, tag := range written {
		if tag = strings.TrimSpace(tag); tag != "" && !seen[tag] {
			tags, seen[tag] = append(tags, tag), true
		}
	}
	return tags
}
