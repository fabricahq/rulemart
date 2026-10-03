// What assembly needs from rendering: where a Markdown file came from, where its links lead, and a renderer that keeps
// within a byte allowance.

package domain

import (
	"errors"
	"net/url"
	"path"
	"regexp"
	"strings"
)

// MarkdownSource is where a Markdown file came from, which its relative links and images resolve against: a rule's
// file, or one of the Markdown files among its assets.
type MarkdownSource struct {
	// Repository is the library's GitHub repository, as owner/name, which links to its pages on Rulemart and its files
	// on GitHub name. Assembly renders with LibraryPlaceholder in its place.
	Repository string
	// File is the Markdown file, such as practices/testing/verify-retry-limits.md.
	File string
	// Rule is the path of the rule whose version covers File, such as practices/testing/verify-retry-limits: File is
	// the rule's own file, or one of its own assets. It's empty for a shared asset, which no rule's version covers.
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

// Renderer renders what pages show of a library's files within a byte allowance: Markdown, its links and images leading
// where its MarkdownSource says, and code. render.Renderer implements it; assembly takes it as a value so that what reads the catalog doesn't carry a
// Markdown renderer.
type Renderer interface {
	// Markdown returns the HTML a page shows for a Markdown body, from source, and how many bytes of allowance it used:
	// the HTML, and each link it rewrote. It fails with ErrOverAllowance, without building past allowance, when the HTML
	// would need more.
	Markdown(body string, source MarkdownSource, allowance int64) (html string, used int64, err error)
	// Code returns the HTML a page shows for the text of file, a text file in a library, as code, and how many bytes of
	// allowance it used, failing as Markdown does.
	Code(text, file string, allowance int64) (html string, used int64, err error)
	// Links returns the distinct destinations of a Markdown body's links and images, as a page reads them, with
	// backslash escapes and character references resolved, in the order they first appear, and how many bytes of
	// allowance they used: each destination once, however many references name it. It fails as Markdown does when they
	// would need more.
	Links(body string, allowance int64) (destinations []string, used int64, err error)
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

// LinkURL returns where a link in the file leads: a relative destination opens an asset's page on Rulemart, keeping its
// fragment, or the file on GitHub at the release that holds it, and anything else, such as an absolute URL or a
// fragment, stays as written.
func (s MarkdownSource) LinkURL(destination string) string {
	file, suffix, ok := ResolveLink(s.File, destination)
	if !ok {
		return destination
	}
	if asset, ok := s.Assets[file]; ok {
		if _, fragment, _ := strings.Cut(suffix, "#"); fragment != "" {
			return asset.Page + "#" + fragment
		}
		return asset.Page
	}
	if file == "" {
		return TreeURL(s.Repository, s.tagFor(file)) + suffix
	}
	return BlobURL(s.Repository, s.tagFor(file), file) + suffix
}

// ImageURL returns where an image in the file loads from: an asset Rulemart keeps loads from Rulemart, another relative
// source from GitHub at the release that holds it, and an absolute one stays as written.
func (s MarkdownSource) ImageURL(destination string) string {
	file, suffix, ok := ResolveLink(s.File, destination)
	if !ok || file == "" {
		return destination
	}
	if asset, ok := s.Assets[file]; ok && asset.Image != "" {
		return asset.Image
	}
	return RawURL(s.Repository, s.tagFor(file), file) + suffix
}

// tagFor returns the release whose tree holds file as the page shows it: the rule's own release for its file and its
// asset directory, and the latest release for everything else.
func (s MarkdownSource) tagFor(file string) string {
	if s.Rule != "" && (file == RuleFile(s.Rule) || strings.HasPrefix(file, RuleAssetDir(s.Rule))) {
		return s.Tag
	}
	return s.LatestTag
}

// LibraryPlaceholder stands in for a library's owner/name in the links assembly renders, which LinksForLibrary
// replaces with the address of the library a page shows. Stored text then names no library: copied to another
// library's rows, or read after the library's repository is renamed, it still leads only within the library whose
// page shows it. No GitHub login holds an underscore, so no library's address holds it.
const LibraryPlaceholder = "_owner_/_name_"

// placeholderLink matches the start of a link or image's address that names LibraryPlaceholder: a page of Rulemart's,
// a file on GitHub, or a raw file there. Rendering escapes every < in text, so only its own attributes match.
var placeholderLink = regexp.MustCompile(`((?:href|src)="(?:https://github\.com|https://raw\.githubusercontent\.com)?/)` +
	regexp.QuoteMeta(LibraryPlaceholder) + `/`)

// LinksForLibrary returns html, as assembly rendered it, with each link and image that names LibraryPlaceholder naming
// fullName, the owner/name of the library whose page shows it.
func LinksForLibrary(html, fullName string) string {
	return placeholderLink.ReplaceAllString(html, "${1}"+strings.ReplaceAll(fullName, "$", "$$")+"/")
}
