// Where a library release keeps its files in its repository, and how a library path, the path rule IDs, release
// records, and pages use, maps to a repository path, the path the release's commit and GitHub use.

package domain

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// Layout is where a library release keeps its files in its repository: the directory that holds them, which the
// catalog stores for each release. Rule IDs and every library path stay the same in each layout, so a library that
// moved its files keeps its history.
type Layout string

const (
	// RootLayout keeps a library at its repository's root, as Syllabus did before it moved libraries into
	// .syllabus/: rule-library.yaml, and the techs/, practices/, assets/, and changes/ directories. Every repository
	// path is the library path of the same name.
	RootLayout Layout = ""
	// SyllabusLayout keeps a library in .syllabus/: its manifest is the library section of .syllabus/config.yaml,
	// its groups are in .syllabus/rules/, and its shared assets and change notes in .syllabus/assets/ and
	// .syllabus/changes/. License and notice files stay at their paths from the repository root.
	SyllabusLayout Layout = coderules.Directory
	// CodeRulesLayout is SyllabusLayout in the directory's name from before Code Rules was renamed Syllabus.
	CodeRulesLayout Layout = coderules.LegacyDirectory
)

// ParseLayout returns the layout whose directory is text, as the catalog stores it.
func ParseLayout(text string) (Layout, error) {
	switch layout := Layout(text); layout {
	case RootLayout, SyllabusLayout, CodeRulesLayout:
		return layout, nil
	}
	return "", fmt.Errorf("unknown library layout %q", text)
}

// DetectLayout returns the layout of the release whose commit holds files, as Syllabus reads it: RootLayout when the
// root holds rule-library.yaml, or else the Syllabus directory the commit has, .syllabus, or before the rename,
// .code-rules. A commit that has neither is read in RootLayout, the layout every library had before.
func DetectLayout(files Files) (Layout, error) {
	if _, err := files.Open(coderules.LegacyManifest); err == nil {
		return RootLayout, nil
	} else if !errors.Is(err, ErrFileMissing) {
		return "", fmt.Errorf("%s: %v", coderules.LegacyManifest, err)
	}
	for _, layout := range []Layout{SyllabusLayout, CodeRulesLayout} {
		// One file shows the directory is there; Syllabus reads a commit's .code-rules/ only while it has no .syllabus/.
		held, err := files.List(string(layout)+"/", 0)
		if err != nil {
			return "", fmt.Errorf("list %s/: %v", layout, err)
		}
		if len(held) > 0 {
			return layout, nil
		}
	}
	return RootLayout, nil
}

// ManifestFile returns the repository path of the manifest, the file whose license the library declares.
func (l Layout) ManifestFile() string {
	if l == RootLayout {
		return coderules.LegacyManifest
	}
	return l.RepositoryPath(coderules.Manifest)
}

// RepositoryPath returns where the library file, or directory, at library path file lives in the repository, such as
// .syllabus/rules/techs/go/return-errors.md for techs/go/return-errors.md in SyllabusLayout.
func (l Layout) RepositoryPath(file string) string {
	if l == RootLayout {
		return file
	}
	repository := coderules.RepositoryPath(file)
	if inside, found := strings.CutPrefix(repository, coderules.Directory+"/"); found {
		return string(l) + "/" + inside
	}
	return repository
}

// LibraryPath returns the library path of the repository file, or directory, at file, the inverse of RepositoryPath.
// It reports false for a path that holds no library file, such as a project's files in the Syllabus directory.
func (l Layout) LibraryPath(file string) (string, bool) {
	switch l {
	case RootLayout:
		return file, true
	case CodeRulesLayout:
		if inside, found := strings.CutPrefix(file, string(l)+"/"); found {
			file = coderules.Directory + "/" + inside
		} else if file == string(l) || strings.HasPrefix(file, coderules.Directory+"/") {
			return "", false
		}
	}
	return coderules.LibraryPath(file)
}

// ResolveLink returns the repository file that destination, a link or image in the library file at library path
// from, names, and the query and fragment to keep after it. A relative destination resolves against the directory of
// from's repository path, and one that starts with / against the directory that holds the library: the repository's
// root in RootLayout, and the Syllabus directory otherwise, as Syllabus resolves it. target is empty for the root,
// including for destinations that climb above it. ok is false for a destination that isn't a path in the repository:
// one with a scheme or host, or only a query or fragment.
func (l Layout) ResolveLink(from, destination string) (target, suffix string, ok bool) {
	target, suffix, rooted, ok := resolveLink(l.RepositoryPath(from), destination)
	if ok && rooted && l != RootLayout {
		target = path.Join(string(l), target)
	}
	return target, suffix, ok
}

// ReleaseLayouts is where a library's releases keep its files: by number, the layout of each release whose layout
// differs from the one before's, the first release's included. A release it doesn't name keeps its files as the
// release before it does, and an empty ReleaseLayouts keeps every release in RootLayout.
type ReleaseLayouts map[int]Layout

// Of returns the layout of release number.
func (r ReleaseLayouts) Of(number int) Layout {
	from, layout := 0, RootLayout
	for n, l := range r {
		if n <= number && n > from {
			from, layout = n, l
		}
	}
	return layout
}

// ResolveLibraryLink returns the library path of the file that destination, a link or image in the library file at
// library path from, names, as ResolveLink finds it. ok is false when the destination isn't a path in the
// repository, or names the root or a file that isn't the library's.
func (l Layout) ResolveLibraryLink(from, destination string) (file string, ok bool) {
	target, _, ok := l.ResolveLink(from, destination)
	if !ok || target == "" {
		return "", false
	}
	return l.LibraryPath(target)
}
