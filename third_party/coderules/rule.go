// Parse complete rule documents while retaining their authored text.

package coderules

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/nlnwa/whatwg-url/url"
	"go.yaml.in/yaml/v4"
)

// Impact is the rule's declared consequence level. Parse accepts only the six constants.
type Impact string

const (
	ImpactCritical   Impact = "CRITICAL"
	ImpactHigh       Impact = "HIGH"
	ImpactMediumHigh Impact = "MEDIUM-HIGH"
	ImpactMedium     Impact = "MEDIUM"
	ImpactLowMedium  Impact = "LOW-MEDIUM"
	ImpactLow        Impact = "LOW"
)

// Attribution is an author-declared citation, not a license grant or verified origin.
type Attribution struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

// Rule contains validated selection fields and one exact original document.
// Document includes delimiters, frontmatter, and body without reserialization.
// SplitDocument exposes its original sections when needed. Attribution is an
// empty slice when absent; validated tags remain in Document.
type Rule struct {
	ID                string        `json:"id"`
	Group             string        `json:"group"`
	Path              string        `json:"path"`
	Title             string        `json:"title"`
	Impact            Impact        `json:"impact"`
	ImpactDescription string        `json:"impactDescription"`
	WhenToRead        string        `json:"whenToRead"`
	Attribution       []Attribution `json:"attribution"`
	Document          string        `json:"document"`
}

// Parse validates a rule's path, UTF-8 document, YAML metadata, nonblank body, and attribution.
// Unknown metadata and attribution fields are rejected.
// Source is a caller-owned alias, not an authenticated origin. Path is a relative
// rule path; neither argument causes file access. Text values retain whitespace.
// On any failure, Parse returns the zero Rule and a ValidationError.
func Parse(text, path, source string) (Rule, error) {
	location := source + ":" + path
	group, err := GroupFromPath(path, location)
	if err != nil {
		return Rule{}, err
	}
	if !utf8.ValidString(text) {
		return Rule{}, invalid(location, "expected UTF-8 text")
	}
	document, err := SplitDocument(text, location)
	if err != nil {
		return Rule{}, err
	}
	fields, err := ruleYAML(document.Frontmatter, location)
	if err != nil {
		return Rule{}, err
	}
	for _, key := range []string{"license", "licenses"} {
		if _, exists := fields[key]; exists {
			return Rule{}, invalid(location+"."+key, "declare one license for the whole library in rule-library.yaml; rule-level licenses are unsupported")
		}
	}
	if err := ruleKnownFields(fields, location, "title", "impact", "impactDescription", "whenToRead", "tags", "attribution"); err != nil {
		return Rule{}, err
	}
	result, err := ruleFields(fields, location)
	if err != nil {
		return Rule{}, err
	}
	if strings.TrimFunc(document.Body, jsWhitespace) == "" {
		return Rule{}, invalid(location+".body", "expected nonempty text")
	}
	result.Attribution, err = ruleAttribution(fields["attribution"], location+".attribution")
	if err != nil {
		return Rule{}, err
	}
	result.ID = source + ":" + strings.TrimSuffix(path, ".md")
	result.Group, result.Path = group, path
	result.Document = text
	return result, nil
}

// ruleKnownFields rejects the first unknown field in sorted order and lists the allowed names.
func ruleKnownFields(fields map[string]*yaml.Node, location string, allowed ...string) error {
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		if !slices.Contains(allowed, key) {
			return invalid(location+"."+key, "unknown field; allowed fields: "+strings.Join(allowed, ", "))
		}
	}
	return nil
}

// ruleFields validates the required selection fields and optional tags in reference order.
func ruleFields(fields map[string]*yaml.Node, location string) (Rule, error) {
	var r Rule
	var err error
	r.Title, err = ruleText(fields["title"], location+".title")
	if err != nil {
		return Rule{}, err
	}
	impact, err := ruleText(fields["impact"], location+".impact")
	if err != nil {
		return Rule{}, err
	}
	r.Impact, err = ParseImpact(impact, location)
	if err != nil {
		return Rule{}, err
	}
	r.ImpactDescription, err = ruleText(fields["impactDescription"], location+".impactDescription")
	if err != nil {
		return Rule{}, err
	}
	if tags, exists := fields["tags"]; exists {
		if err := ruleTags(tags, location+".tags"); err != nil {
			return Rule{}, err
		}
	}
	r.WhenToRead, err = ruleText(fields["whenToRead"], location+".whenToRead")
	if err != nil {
		return Rule{}, err
	}
	return r, nil
}

// ruleText requires a nonblank YAML string and returns its untrimmed value.
func ruleText(node *yaml.Node, location string) (string, error) {
	if node == nil || !yamlString(node) || strings.TrimFunc(node.Value, jsWhitespace) == "" {
		return "", invalid(location, "expected nonempty text")
	}
	return node.Value, nil
}

// ruleTags accepts nonblank text or distinct nonblank strings, reporting invalid entries before duplicates.
func ruleTags(node *yaml.Node, location string) error {
	if yamlString(node) {
		_, err := ruleText(node, location)
		return err
	}
	if !yamlArray(node) {
		return invalid(location, "expected an array of strings")
	}
	seen := make(map[string]bool)
	duplicate := false
	for i, entry := range yamlEntries(node) {
		text, err := ruleText(entry, fmt.Sprintf("%s[%d]", location, i))
		if err != nil {
			return err
		}
		duplicate = duplicate || seen[text]
		seen[text] = true
	}
	if duplicate {
		return invalid(location, "duplicate entries")
	}
	return nil
}

// ruleAttribution validates citations and normalizes HTTP(S) URLs, returning an empty slice when absent.
func ruleAttribution(node *yaml.Node, location string) ([]Attribution, error) {
	result := []Attribution{}
	if node == nil {
		return result, nil
	}
	if !yamlArray(node) {
		return nil, invalid(location, "expected an attribution array")
	}
	for i, entry := range yamlEntries(node) {
		where := fmt.Sprintf("%s[%d]", location, i)
		fields, err := yamlObject(entry, where)
		if err != nil {
			return nil, err
		}
		if err := ruleKnownFields(fields, where, "url", "description"); err != nil {
			return nil, err
		}
		text, err := ruleText(fields["url"], where+".url")
		if err != nil {
			return nil, err
		}
		parsed, err := url.Parse(text)
		if err != nil {
			return nil, invalid(where+".url", "expected an absolute HTTP(S) URL")
		}
		if (parsed.Scheme() != "http" && parsed.Scheme() != "https") || parsed.Username() != "" || parsed.Password() != "" {
			return nil, invalid(where+".url", "expected an absolute HTTP(S) URL without credentials")
		}
		description, err := ruleText(fields["description"], where+".description")
		if err != nil {
			return nil, err
		}
		result = append(result, Attribution{URL: parsed.Href(false), Description: description})
	}
	return result, nil
}

// ParseImpact accepts the six declared consequence levels without normalizing authored values.
func ParseImpact(value, location string) (Impact, error) {
	impact := Impact(value)
	switch impact {
	case ImpactCritical, ImpactHigh, ImpactMediumHigh, ImpactMedium, ImpactLowMedium, ImpactLow:
		return impact, nil
	default:
		return "", invalid(location, fmt.Sprintf("impact must be one of %s, %s, %s, %s, %s, %s; got %s",
			ImpactCritical, ImpactHigh, ImpactMediumHigh, ImpactMedium, ImpactLowMedium, ImpactLow, quote(value)))
	}
}
