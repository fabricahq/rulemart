// Package coderules parses and validates Code Rules' rule, group, library manifest, release record, and canonical
// group list formats without accessing the filesystem. It is a temporary copy of part of Code Rules' internal rules
// package; see README.md.
package coderules

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var (
	groupPattern    = regexp.MustCompile(`^(techs|practices)/[a-z][a-z0-9-]*$`)
	rulePartPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)
)

// ValidationError identifies invalid user input. Use errors.As to inspect it,
// and the returned error's Error method to retain any enclosing operation context.
type ValidationError struct {
	// Location is a caller-supplied field path, including the original array index
	// when applicable. It is diagnostic text, never a filesystem path to access.
	Location string
	// Problem explains the failure to a human. Callers must not branch on its text.
	Problem string
}

func (e *ValidationError) Error() string { return e.Location + ": " + e.Problem }

// ValidateGroupID accepts a technology or practice group ID, except the reserved
// assets name. It checks syntax only; it does not establish that the group exists.
func ValidateGroupID(value, location string) error {
	if !groupPattern.MatchString(value) {
		return invalid(location, "invalid group ID "+quote(value)+": expected format "+groupPattern.String())
	}
	if strings.HasSuffix(value, "/assets") {
		return invalid(location, "invalid group ID "+quote(value)+`: "assets" is a reserved group name`)
	}
	return nil
}

// GroupFromPath returns the owning group of a contained Markdown rule path.
// Paths use forward slashes on every OS and are never cleaned or normalized.
// Containment errors take precedence over rule syntax, then group syntax errors.
func GroupFromPath(path, location string) (string, error) {
	parts := strings.Split(path, "/")
	if !contained(path, parts) {
		return "", invalid(location, "expected a contained relative path, got "+quote(path)+`: use forward slashes; no leading slash, drive prefix, empty segments, "." or ".." segments, or control characters`)
	}
	if len(parts) < 3 || !strings.HasSuffix(path, ".md") {
		return "", invalid(location, "invalid rule path "+quote(path)+": expected a .md file beneath a group directory")
	}
	for i, part := range parts {
		if i > 0 && part == "assets" {
			return "", invalid(location, "invalid rule path "+quote(path)+`: "assets" is reserved for supporting files, not rules`)
		}
		if i >= 2 && !rulePartPattern.MatchString(part) {
			return "", invalid(location, "invalid rule path "+quote(path)+": rule file and subdirectory names must match "+rulePartPattern.String())
		}
	}
	group := strings.Join(parts[:2], "/")
	if err := ValidateGroupID(group, location); err != nil {
		return "", fmt.Errorf("rule path %s: %w", quote(path), err)
	}
	return group, nil
}

func contained(path string, parts []string) bool {
	if len(path) >= 2 && path[1] == ':' && ((path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z')) {
		return false
	}
	for _, r := range path {
		if r == '\\' || r < 32 || r == 127 {
			return false
		}
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func invalid(location, problem string) error {
	return &ValidationError{Location: location, Problem: problem}
}

// quote preserves the reference's JSON string spelling for Unicode scalar text.
// strconv.Quote uses Go-only escapes; encoding/json also escapes HTML and U+2028.
func quote(value string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 32 {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// VersionedRule returns the ID of the rule whose versions cover file: the rule's own Markdown file, or a file in
// its asset directory, assets/<rule-name>/ beside it. It reports false for library-wide files, such as group
// metadata, group READMEs, and the library-root assets/ directory, and for paths that can't be rule content.
func VersionedRule(file string) (string, bool) {
	parts := strings.Split(file, "/")
	if len(parts) < 3 || (parts[0] != "techs" && parts[0] != "practices") {
		return "", false
	}
	for i := 2; i < len(parts); i++ {
		if parts[i] == "assets" {
			if i+2 >= len(parts) {
				return "", false
			}
			return strings.Join(parts[:i], "/") + "/" + parts[i+1], true
		}
	}
	if IsGroupReadme(file) {
		return "", false
	}
	if _, err := GroupFromPath(file, file); err != nil {
		return "", false
	}
	return strings.TrimSuffix(file, ".md"), true
}

// isRuleContent reports whether path is part of some rule's version: a rule's Markdown file, or a file in an asset
// directory inside a technology or practice group, since every such directory belongs to a rule. Group metadata
// and the library-root assets/ directory are library-wide instead; group READMEs are authoring notes, neither
// rule content nor library-wide. ParseLibraryLicense rejects declared license and notice files that are rule
// content, so every path is one or the other, never both.
func isRuleContent(path string) bool {
	parts := strings.Split(path, "/")
	if parts[0] != "techs" && parts[0] != "practices" {
		return false
	}
	if slices.Contains(parts[1:], "assets") {
		return true
	}
	if _, err := GroupFromPath(path, path); err == nil {
		return !IsGroupReadme(path)
	}
	return false
}

// IsGroupReadme recognizes only the orientation file directly inside a valid group, outside rule and asset paths.
func IsGroupReadme(file string) bool {
	group, found := strings.CutSuffix(file, "/README.md")
	return found && ValidateGroupID(group, file) == nil
}
