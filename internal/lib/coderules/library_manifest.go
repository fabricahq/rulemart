// Place a library's files in its repository, and validate the library section of its configuration, the manifest,
// and the license it declares, returning safe library paths for callers to read.

package coderules

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Directory is the Trail Guide directory at the repository root. It holds a library's manifest, rules, shared
// assets, and change notes, beside the project files of the same repository.
const Directory = ".trailguide"

// LegacyDirectory is the Trail Guide directory's name from before Code Rules was renamed Trail Guide. Projects that
// haven't renamed it, and every library release published before the rename, keep their files there.
const LegacyDirectory = ".code-rules"

// Manifest is the library path of the library's manifest: the repository's configuration, .trailguide/config.yaml,
// whose library section makes the repository a library.
const Manifest = "config.yaml"

// LegacyManifest is the manifest at the repository root of a library in the layout before version 2, which also
// kept its groups, shared assets, and change notes at the repository root.
const LegacyManifest = "rule-library.yaml"

// RepositoryPath returns where the library file at library path file lives in its repository. The manifest is
// .trailguide/config.yaml; groups, at techs/ and practices/, live in .trailguide/rules/; shared assets, at assets/,
// and change notes, at changes/, live in .trailguide/. Any other path is a license or notice file, at the same path
// relative to the repository root.
func RepositoryPath(file string) string {
	if file == Manifest {
		return Directory + "/" + Manifest
	}
	switch first, _, _ := strings.Cut(file, "/"); first {
	case "techs", "practices":
		return Directory + "/rules/" + file
	case "assets", "changes":
		return Directory + "/" + file
	}
	return file
}

// LibraryPath returns the library path of the file or directory at repository path file, the inverse of
// RepositoryPath. It reports false for a path that holds no library file: anything else in .trailguide/, such as
// vendor/, generated/, or rules/README.md, and a path outside .trailguide/ whose library path would be one of a
// file inside it, such as assets/logo.svg or config.yaml at the repository root.
func LibraryPath(file string) (string, bool) {
	inside, found := strings.CutPrefix(file, Directory+"/")
	if !found {
		if file == Directory || RepositoryPath(file) != file {
			return "", false
		}
		return file, true
	}
	if rule, found := strings.CutPrefix(inside, "rules/"); found {
		if first, _, _ := strings.Cut(rule, "/"); first == "techs" || first == "practices" {
			return rule, true
		}
		return "", false
	}
	if first, _, _ := strings.Cut(inside, "/"); first == "assets" || first == "changes" || inside == Manifest {
		return inside, true
	}
	return "", false
}

// Library is a configuration's library section, the manifest, which makes the repository a library.
type Library struct {
	// License is nil when the library declares no license.
	License *LicenseDeclaration `json:"license"`
}

// skillLevelsProblem rejects library.skills, which a later version of Trail Guide reads.
const skillLevelsProblem = "skill levels aren't supported yet in this version of Trail Guide; remove library.skills"

// ParseLibraryManifest validates text, a library's whole .trailguide/config.yaml, and returns its library section,
// or nil when the configuration has none, which means the repository isn't a library. Errors prefix their location
// with location, such as team/config.yaml. It checks syntax only; callers read the declared files themselves.
// Rulemart's copy reads only the configuration's schemaVersion and library fields: the rest configures the
// repository as a project, which a library's catalog doesn't read.
func ParseLibraryManifest(text []byte, location string) (*Library, error) {
	if !utf8.Valid(text) {
		return nil, invalid(location, "expected UTF-8 text")
	}
	_, data, err := authoredYAML(text, location)
	if err != nil {
		return nil, err
	}
	fields, err := jsonObject(data, location)
	if err != nil {
		return nil, err
	}
	var schema float64
	if json.Unmarshal(fields["schemaVersion"], &schema) != nil || schema != 2 {
		return nil, invalid(location+": schemaVersion", "only version 2 is supported")
	}
	raw, ok := fields["library"]
	if !ok {
		return nil, nil
	}
	library, err := parseLibrary(raw)
	var validation *ValidationError
	if errors.As(err, &validation) {
		return nil, invalid(location+": "+validation.Location, validation.Problem)
	}
	return library, err
}

// parseLibrary validates a configuration's library section. Its only field is license; skill levels are rejected
// until Trail Guide supports them.
func parseLibrary(input json.RawMessage) (*Library, error) {
	fields, err := jsonObject(input, "library")
	if err != nil {
		return nil, err
	}
	if _, ok := fields["skills"]; ok {
		return nil, invalid("library.skills", skillLevelsProblem)
	}
	if err := knownJSONFields(fields, []string{"license"}, "library"); err != nil {
		return nil, err
	}
	result := &Library{}
	if raw, ok := fields["license"]; ok {
		if result.License, err = parseLicense(raw, "library.license"); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// parseLicense validates a license declaration and returns the library paths of its declared files, without
// checking file presence. Declared paths are relative to the repository root and must be library-wide files, never
// a rule's Markdown file or a file in an asset directory inside a group. Unknown fields are rejected.
func parseLicense(input json.RawMessage, location string) (*LicenseDeclaration, error) {
	fields, err := jsonObject(input, location)
	if err != nil {
		return nil, err
	}
	if err := knownJSONFields(fields, []string{"file", "notices", "spdxExpression"}, location); err != nil {
		return nil, err
	}
	file, err := libraryTermsPath(fields["file"], location+".file")
	if err != nil {
		return nil, err
	}
	var rawNotices []json.RawMessage
	if json.Unmarshal(fields["notices"], &rawNotices) != nil || rawNotices == nil {
		return nil, invalid(location+".notices", "expected an array of notice paths")
	}
	notices := make([]string, len(rawNotices))
	for i, raw := range rawNotices {
		path, err := libraryTermsPath(raw, fmt.Sprintf("%s.notices[%d]", location, i))
		if err != nil {
			return nil, err
		}
		notices[i] = path
	}
	var expression *string
	if raw, ok := fields["spdxExpression"]; ok {
		text, err := jsonText(raw, location+".spdxExpression")
		if err != nil {
			return nil, err
		}
		if strings.ContainsFunc(text, controlCharacter) {
			return nil, invalid(location+".spdxExpression", "expected a single-line license expression")
		}
		if !validSPDXExpression(text) {
			return nil, invalid(location+".spdxExpression", "expected an SPDX expression using recognized identifiers or LicenseRef- custom terms")
		}
		expression = &text
	}
	result := LicenseDeclaration{SPDXExpression: expression, Files: []string{file}, AttributionFiles: []string{}}
	seen := map[string]bool{file: true}
	for _, path := range notices {
		if !seen[path] {
			result.AttributionFiles = append(result.AttributionFiles, path)
			seen[path] = true
		}
	}
	return &result, nil
}

// libraryTermsPath reads a declared license or notice path, relative to the repository root, and returns its library
// path. The file must be library-wide: outside .trailguide/, or in its assets/ or a group in its rules/. A rule's
// Markdown file or a file in an asset directory inside a group belongs to that rule's version, so declaring it would
// let a library release replace a rule version's content.
func libraryTermsPath(input json.RawMessage, location string) (string, error) {
	declared, err := jsonPath(input, location)
	if err != nil {
		return "", err
	}
	path, ok := LibraryPath(declared)
	if !ok || path == Manifest || strings.HasPrefix(path, "changes/") {
		return "", invalid(location, quote(declared)+" can't hold a license or notice file; keep it outside .trailguide/, such as LICENSE.md at the repository root, or in .trailguide/assets/. At the repository root, config.yaml and the assets/, changes/, practices/, and techs/ folders are reserved for the library's own files")
	}
	if isRuleContent(path) {
		return "", invalid(location, quote(declared)+" belongs to a rule's version; keep license and notice files outside rules and their asset directories, such as LICENSE.md at the repository root")
	}
	return path, nil
}
