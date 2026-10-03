// Where things live: a library release's tag, the files in a library, and the GitHub URLs that show them.

package domain

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ManifestFile is the file every Code Rules library has at its root.
const ManifestFile = "rule-library.yaml"

// ReleaseTag returns the tag of library release number, such as release/3.
func ReleaseTag(number int) string { return "release/" + strconv.Itoa(number) }

// RuleFile returns the file of the rule at rulePath, such as techs/go/return-errors.md.
func RuleFile(rulePath string) string { return rulePath + ".md" }

// GroupFile returns the metadata file of the group at groupPath, such as techs/go/_group.yaml.
func GroupFile(groupPath string) string { return groupPath + "/_group.yaml" }

// RuleAssetDir returns the asset directory of the rule at rulePath, assets/<rule name>/ beside its file, such as
// practices/testing/assets/verify-retry-limits/ for practices/testing/verify-retry-limits. The rule's version covers
// every file in it.
func RuleAssetDir(rulePath string) string {
	dir, name := path.Split(rulePath)
	return dir + "assets/" + name + "/"
}

// SharedAssetDir is the library-root directory of the files a library's rules share, which no rule's version covers.
const SharedAssetDir = "assets/"

// AssetPagePath returns the path of the page Rulemart shows an asset of the rule at rulePath in the library fullName
// on, owner/name: a file of the rule's own, at /owner/name/<rule ID>/assets/<its path in the rule's asset directory>,
// and a shared one, whose path starts with SharedAssetDir, at /owner/name/<its path>.
func AssetPagePath(fullName, rulePath, assetPath string) string {
	if file, own := strings.CutPrefix(assetPath, RuleAssetDir(rulePath)); own {
		return "/" + EscapePath(fullName) + "/" + EscapePath(rulePath) + "/assets/" + EscapePath(file)
	}
	return "/" + EscapePath(fullName) + "/" + EscapePath(assetPath)
}

// AssetRawParam is the query parameter that asks an asset's page for the asset's bytes instead, which Rulemart
// serves for images it keeps.
const AssetRawParam = "raw"

// AssetImagePath returns where Rulemart serves the bytes of the image whose page is at page.
func AssetImagePath(page string) string { return page + "?" + AssetRawParam + "=1" }

// EscapePath percent-encodes each segment of a repository path for a URL.
func EscapePath(file string) string {
	segments := strings.Split(file, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}

// RepositoryURL returns the GitHub page of the repository owner/name, given as fullName.
func RepositoryURL(fullName string) string { return "https://github.com/" + fullName }

// OwnerURL returns the GitHub page of a repository owner.
func OwnerURL(owner string) string { return "https://github.com/" + owner }

// TreeURL returns the GitHub page of the repository fullName's root at tag.
func TreeURL(fullName, tag string) string { return RepositoryURL(fullName) + "/tree/" + tag }

// BlobURL returns the GitHub page of file in the repository fullName at tag.
func BlobURL(fullName, tag, file string) string {
	return RepositoryURL(fullName) + "/blob/" + tag + "/" + EscapePath(file)
}

// RawURL returns where GitHub serves file in the repository fullName at tag, such as for an image.
func RawURL(fullName, tag, file string) string {
	return "https://raw.githubusercontent.com/" + fullName + "/refs/tags/" + tag + "/" + EscapePath(file)
}

// ReleaseNotesURL returns the GitHub Release page Code Rules creates for library release number of the repository
// fullName.
func ReleaseNotesURL(fullName string, number int) string {
	return RepositoryURL(fullName) + "/releases/tag/" + ReleaseTag(number)
}

// gitHubName matches a GitHub owner or repository name.
var gitHubName = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// ParseRepositoryURL returns the owner and name in a GitHub repository URL, such as
// https://github.com/fabricahq/code-rules-test-library. It accepts a trailing slash or .git suffix.
func ParseRepositoryURL(raw string) (owner, name string, err error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.RawQuery != "" || u.Fragment != "" {
		return "", "", fmt.Errorf("repository URL %q: expected https://github.com/<owner>/<repository>", raw)
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), "/"), ".git"), "/")
	if len(parts) != 2 || !gitHubName.MatchString(parts[0]) || !gitHubName.MatchString(parts[1]) {
		return "", "", fmt.Errorf("repository URL %q: expected https://github.com/<owner>/<repository>", raw)
	}
	return parts[0], parts[1], nil
}

// Storable reports whether text is text Postgres can hold: UTF-8 without a NUL byte. Every name the catalog stores,
// of a library, rule, or group, is, so text that isn't names nothing in it, and looking it up would fail rather than
// find nothing.
func Storable(text string) bool {
	return utf8.ValidString(text) && !strings.ContainsRune(text, 0)
}
