// What assembly needs from rendering: where a Markdown file came from, and a renderer that keeps within a byte
// allowance.

package domain

import (
	"errors"
	"net/url"
	"path"
	"strings"
)

// MarkdownSource is where a Markdown file came from, which its relative links and images resolve against: a rule's
// file, or one of the Markdown files among its assets.
type MarkdownSource struct {
	// Repository is the library's GitHub repository, as owner/name.
	Repository string
	// File is the Markdown file, such as practices/testing/verify-retry-limits.md.
	File string
	// Rule is the file of the rule whose version covers File: the rule's own file, or the rule's when File is one of
	// its own assets. It's empty for a shared asset, which no rule's version covers.
	Rule string
	// Title is the rule's title, which its page shows above the body, so the body's leading heading that repeats it
	// goes; empty for an asset, whose headings all stay.
	Title string
	// Tag published the rule's version, and LatestTag is the library's latest release. Files the rule's version
	// covers, its Markdown and its own assets, link to Tag; library-wide files link to LatestTag.
	Tag, LatestTag string
	// Assets are the files Rulemart shows on pages of their own, by their paths in the repository: a link to one
	// leads to its page, and an image of one loads from Rulemart when Rulemart keeps its bytes.
	Assets map[string]AssetAddress
}

// AssetAddress is where Rulemart shows an asset.
type AssetAddress struct {
	// Page is the asset's page.
	Page string
	// Image is where Rulemart serves the asset's bytes as an image, or empty when it doesn't: the file isn't an
	// image, or is past what ingestion keeps.
	Image string
}

// Renderer renders what pages show of a library's files within a byte allowance: Markdown, with Rulemart's link rules,
// and code. render.Renderer implements it; assembly takes it as a value so that what reads the catalog doesn't carry a
// Markdown renderer.
type Renderer interface {
	// Markdown returns the HTML a page shows for a Markdown body, from source, and how many bytes of allowance it used:
	// the HTML, and each link it rewrote. It fails with ErrOverAllowance, without building past allowance, when the HTML
	// would need more.
	Markdown(body string, source MarkdownSource, allowance int64) (html string, used int64, err error)
	// Code returns the HTML a page shows for the text of file, a text file in a library, as code, and how many bytes of
	// allowance it used, failing as Markdown does.
	Code(text, file string, allowance int64) (html string, used int64, err error)
	// Links returns the destinations of a Markdown body's links and images, as written, in the order they appear.
	Links(body string) []string
}

// ErrOverAllowance reports a render that would build more than its allowance.
var ErrOverAllowance = errors.New("the HTML needs more bytes than its allowance")

// ResolveLink returns the repository file that destination, a link or image in the Markdown file from, names, and the
// query and fragment to keep after it. A relative destination resolves against from's directory, and one that starts
// with / against the repository's root. target is empty for the root, including for destinations that climb above
// it. ok is false for a destination that isn't a path in the repository: one with a scheme or host, or only a query
// or fragment.
func ResolveLink(from, destination string) (target, suffix string, ok bool) {
	u, err := url.Parse(destination)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Opaque != "" || u.Path == "" {
		return "", "", false
	}
	if u.RawQuery != "" {
		suffix += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		suffix += "#" + u.EscapedFragment()
	}
	target = path.Clean(u.Path)
	if !strings.HasPrefix(u.Path, "/") {
		target = path.Join(path.Dir(from), u.Path)
	}
	target = strings.TrimPrefix(target, "/")
	if target == "." || target == ".." || strings.HasPrefix(target, "../") {
		target = ""
	}
	return target, suffix, true
}
