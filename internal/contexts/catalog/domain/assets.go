// A library's supporting files, its assets: the files in a current rule's own asset directory, and the files in the
// library-root assets/ directory that its text, or a Markdown file among its assets, links to, as Code Rules copies
// them with the rule. Assembly reads each once, keeping its bytes within the caps.

package domain

import (
	"errors"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"
	"unicode/utf8"
)

// Asset is a supporting file of a library's current rules, as pages show it.
type Asset struct {
	// Path is the file's path in the repository: in a rule's asset directory, such as
	// practices/testing/assets/verify-retry-limits/loop.svg, or in SharedAssetDir, such as assets/glossary.md.
	Path string
	// Release is the number of the library release whose commit the copy is from: the one that published the current
	// version of the rule a file of its own belongs to, which covers it, or the latest, for a shared file.
	Release int
	Size    int64
	// MediaType is what assembly found the file to be, which AssetKindOf reads.
	MediaType string
	// Content is the file's bytes, or nil when they're past the caps, or the file is neither an image nor text.
	Content []byte
	// HTML is how a page shows a Markdown or text file it has the Content of: the Markdown rendered, or the text as
	// highlighted code. Empty for any other.
	HTML string
}

// AssetKind is how pages show an asset.
type AssetKind int

const (
	// AssetFile is a file pages only list, and link on GitHub.
	AssetFile AssetKind = iota
	// AssetImage is an image, which Rulemart serves when it keeps its bytes.
	AssetImage
	// AssetMarkdown is Markdown, which pages show rendered.
	AssetMarkdown
	// AssetText is any other UTF-8 text, which pages show as code.
	AssetText
)

// The media types assembly records for files other than images.
const (
	markdownMediaType = "text/markdown; charset=utf-8"
	textMediaType     = "text/plain; charset=utf-8"
	fileMediaType     = "application/octet-stream"
)

// imageMediaTypes are the image formats browsers show, by file extension, in lowercase.
var imageMediaTypes = map[string]string{
	".avif": "image/avif", ".gif": "image/gif", ".jpeg": "image/jpeg", ".jpg": "image/jpeg", ".png": "image/png",
	".svg": "image/svg+xml", ".webp": "image/webp",
}

// AssetKindOf returns how pages show an asset whose media type assembly recorded as mediaType.
func AssetKindOf(mediaType string) AssetKind {
	switch {
	case strings.HasPrefix(mediaType, "image/"):
		return AssetImage
	case mediaType == markdownMediaType:
		return AssetMarkdown
	case mediaType == textMediaType:
		return AssetText
	}
	return AssetFile
}

// assetMediaType returns the media type of the file at file, whose bytes are content, or nil when assembly didn't read
// them: an image or Markdown file by its name, a Markdown file only when what was read of it is UTF-8, and any other
// file read whose bytes are UTF-8 without a NUL byte as text.
func assetMediaType(file string, content []byte) string {
	extension := strings.ToLower(path.Ext(file))
	if mediaType, ok := imageMediaTypes[extension]; ok {
		return mediaType
	}
	text := content != nil && utf8.Valid(content) && !slices.Contains(content, 0)
	switch {
	case (extension == ".md" || extension == ".markdown") && (content == nil || text):
		return markdownMediaType
	case text:
		return textMediaType
	}
	return fileMediaType
}

// assetReader holds the assets assembly has read, each once.
type assetReader struct {
	// assets holds every asset read, by path; a shared file that no release holds isn't one.
	assets map[string]*Asset
	// missing holds the shared files links name that the latest release doesn't hold.
	missing map[string]bool
	// sharedLinks holds, for each shared Markdown file read, the shared files it links to.
	sharedLinks map[string][]string
	// sharedKept is how many bytes of shared files assembly keeps.
	sharedKept int64
}

// readAssets reads the assets of the current rule at rulePath, whose current version release r published as body and
// reading guidance: its own files, at r, then the shared files its text and Markdown files link to, at the latest
// release, and the shared files those link to in turn. It returns the paths of each, in path order.
func (a *assembly) readAssets(r ReleaseSnapshot, rulePath, body, whenToRead string) (own, shared []string, err error) {
	if own, err = a.readOwnAssets(r, rulePath); err != nil {
		return nil, nil, err
	}
	file := RuleFile(rulePath)
	queue := append(a.sharedTargets(file, body), a.sharedTargets(file, whenToRead)...)
	for _, p := range own {
		if asset := a.read.assets[p]; AssetKindOf(asset.MediaType) == AssetMarkdown && asset.Content != nil {
			queue = append(queue, a.sharedTargets(p, string(asset.Content))...)
		}
	}
	seen := map[string]bool{}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		if seen[p] {
			continue
		}
		seen[p] = true
		found, err := a.readSharedAsset(p)
		if err != nil {
			return nil, nil, err
		}
		if found {
			shared = append(shared, p)
			queue = append(queue, a.read.sharedLinks[p]...)
		}
	}
	slices.Sort(shared)
	return own, shared, nil
}

// readOwnAssets reads every file in the asset directory of the rule at rulePath, at release r, which published the
// rule's current version, keeping their bytes within the caps in path order, and returns their paths.
func (a *assembly) readOwnAssets(r ReleaseSnapshot, rulePath string) ([]string, error) {
	dir := RuleAssetDir(rulePath)
	paths, err := r.Files.List(dir)
	if err != nil {
		return nil, fmt.Errorf("list %s: %v", dir, err)
	}
	if len(a.read.assets)+len(paths) > a.limits.Assets {
		return nil, a.tooManyAssets()
	}
	var kept int64
	for _, p := range paths {
		file, err := r.Files.Open(p)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", p, err)
		}
		if a.read.assets[p], err = a.readAsset(r.Number, p, file, &kept); err != nil {
			return nil, err
		}
	}
	return paths, nil
}

// readSharedAsset reads the shared file at p, at the latest release, once, keeping its bytes within the shared files'
// cap, and for a Markdown file kept, finds the shared files it links to. It reports whether the release holds such a
// file: a link to one it doesn't hold stays a link to GitHub.
func (a *assembly) readSharedAsset(p string) (bool, error) {
	if a.read.assets[p] != nil {
		return true, nil
	}
	if a.read.missing[p] {
		return false, nil
	}
	latest := a.releases[len(a.releases)-1]
	file, err := latest.Files.Open(p)
	if errors.Is(err, ErrFileMissing) {
		a.read.missing[p] = true
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%s: %v", p, err)
	}
	if len(a.read.assets) >= a.limits.Assets {
		return false, a.tooManyAssets()
	}
	asset, err := a.readAsset(latest.Number, p, file, &a.read.sharedKept)
	if err != nil {
		return false, err
	}
	a.read.assets[p] = asset
	if AssetKindOf(asset.MediaType) == AssetMarkdown && asset.Content != nil {
		a.read.sharedLinks[p] = a.sharedTargets(p, string(asset.Content))
	}
	return true, nil
}

// tooManyAssets returns the error that refuses assets past limits.Assets.
func (a *assembly) tooManyAssets() error {
	return fmt.Errorf("the library's rules have more than %d assets, which ingestion won't list", a.limits.Assets)
}

// readAsset returns file, at p in library release, as an asset, with its bytes when it's an image or text within
// limits.AssetBytes and kept, the bytes kept of the files it's counted with, stays within limits.RuleAssetBytes,
// spending them from the budget and adding them to kept.
func (a *assembly) readAsset(release int, p string, file File, kept *int64) (*Asset, error) {
	asset := &Asset{Path: p, Release: release, Size: file.Size()}
	var content []byte
	if asset.Size <= a.limits.AssetBytes && *kept+asset.Size <= a.limits.RuleAssetBytes {
		var err error
		if content, err = file.Read(); err != nil {
			return nil, fmt.Errorf("%s: %v", p, err)
		}
	}
	asset.MediaType = assetMediaType(p, content)
	if content == nil || AssetKindOf(asset.MediaType) == AssetFile {
		return asset, nil
	}
	if err := a.budget.spend(int64(len(content))); err != nil {
		return nil, fmt.Errorf("%s: %v", p, err)
	}
	*kept += asset.Size
	asset.Content = content
	return asset, nil
}

// sharedTargets returns the shared files that the Markdown body of the file from links to, or shows as images.
func (a *assembly) sharedTargets(from, body string) []string {
	var targets []string
	for _, destination := range a.renderer.Links(body) {
		if target, _, ok := ResolveLink(from, destination); ok && strings.HasPrefix(target, SharedAssetDir) {
			targets = append(targets, target)
		}
	}
	return targets
}

// addresses returns where Rulemart shows each of paths, assets of the rule at rulePath, or nil for none.
func (a *assembly) addresses(rulePath string, paths []string) map[string]AssetAddress {
	if len(paths) == 0 {
		return nil
	}
	addresses := make(map[string]AssetAddress, len(paths))
	for _, p := range paths {
		asset := a.read.assets[p]
		address := AssetAddress{Page: AssetPagePath(a.repo.FullName(), rulePath, p)}
		if AssetKindOf(asset.MediaType) == AssetImage && asset.Content != nil {
			address.Image = AssetImagePath(address.Page)
		}
		addresses[p] = address
	}
	return addresses
}

// renderAssets renders the Markdown and text files among paths into their HTML, Markdown's links resolving as source,
// the rule's, says, but against each file.
func (a *assembly) renderAssets(paths []string, source MarkdownSource) error {
	for _, p := range paths {
		asset := a.read.assets[p]
		if asset.Content == nil {
			continue
		}
		var err error
		switch AssetKindOf(asset.MediaType) {
		case AssetMarkdown:
			// Its links resolve against its own file, and none of its headings repeats the rule's title above it.
			at := source
			at.File, at.Title = p, ""
			asset.HTML, err = a.render(string(asset.Content), at)
		case AssetText:
			asset.HTML, err = a.renderCode(string(asset.Content), p)
		}
		if err != nil {
			return fmt.Errorf("%s: %v", p, err)
		}
	}
	return nil
}

// renderSharedAssets renders the shared Markdown and text files, once every rule has found the ones it links to, so
// a shared file's links to the others lead to their pages, and other links to the latest release.
func (a *assembly) renderSharedAssets() error {
	var shared []string
	for p := range a.read.assets {
		if strings.HasPrefix(p, SharedAssetDir) {
			shared = append(shared, p)
		}
	}
	slices.Sort(shared)
	latest := a.releases[len(a.releases)-1].Tag
	return a.renderAssets(shared, MarkdownSource{
		Repository: a.repo.FullName(), Tag: latest, LatestTag: latest, Assets: a.addresses("", shared),
	})
}

// libraryAssets returns every asset read, in path order.
func (a *assembly) libraryAssets() []Asset {
	var assets []Asset
	for _, p := range slices.Sorted(maps.Keys(a.read.assets)) {
		assets = append(assets, *a.read.assets[p])
	}
	return assets
}
