// Provenance: the file Code Rules generates in a project, .code-rules/generated/provenance.json, which names the
// libraries the project imports and the version of each rule it holds.

package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// ProvenancePath is where a project keeps the provenance file Code Rules generates, from its repository's root.
const ProvenancePath = ".code-rules/generated/provenance.json"

// provenanceFile is the part of provenance.json a read needs, as Code Rules' build writes it (internal/build/output.go,
// renderProvenance): each source's name, repository, release, and groups, and each rule's ID and origin. Code Rules
// publishes no parser for it yet, so this reads only those fields, and ignores the rest.
type provenanceFile struct {
	Sources []struct {
		Name       string   `json:"name"`
		Repository string   `json:"repository"`
		Release    int      `json:"release"`
		Groups     []string `json:"groups"`
	} `json:"sources"`
	Rules []struct {
		ID     string `json:"id"`
		Origin struct {
			Source string `json:"source"`
			// Version is null for a local rule, and for an imported file that isn't a published version.
			Version *string `json:"version"`
		} `json:"origin"`
	} `json:"rules"`
}

// ParseProvenance returns the sources a provenance file names, by name, each with the groups it imports and the rules
// the project holds of it at a published version. A rule whose origin is local, such as a fork, holds no library's
// version, so it's left out, as is one whose ID or version Code Rules wouldn't write. It fails for a file larger than
// MaxProvenanceBytes, one that isn't a provenance file's JSON, or one that names no sources.
func ParseProvenance(data []byte) ([]Source, error) {
	if len(data) > MaxProvenanceBytes {
		return nil, fmt.Errorf("parse provenance: the file is larger than %d bytes", MaxProvenanceBytes)
	}
	var file provenanceFile
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&file); err != nil {
		return nil, fmt.Errorf("parse provenance: %v", err)
	}
	if len(file.Sources) == 0 {
		return nil, errors.New("parse provenance: it names no sources")
	}
	sources := make([]Source, 0, len(file.Sources))
	index := map[string]int{}
	for _, s := range file.Sources {
		if s.Name == "" || index[s.Name] != 0 {
			continue
		}
		sources = append(sources, Source{
			Name: s.Name, Library: gitHubLibrary(s.Repository), Release: max(s.Release, 0), Groups: slices.Clone(s.Groups),
		})
		index[s.Name] = len(sources)
	}
	for _, rule := range file.Rules {
		at := index[rule.Origin.Source]
		if at == 0 || rule.Origin.Version == nil {
			continue
		}
		path, ok := strings.CutPrefix(rule.ID, rule.Origin.Source+":")
		if !ok || coderules.ValidateRuleID(path, "rule") != nil {
			continue
		}
		version, err := coderules.ParseRuleVersion(*rule.Origin.Version, "version")
		if err != nil {
			continue
		}
		sources[at-1].Rules = append(sources[at-1].Rules, PinnedRule{Path: path, Version: version})
	}
	slices.SortFunc(sources, func(a, b Source) int { return strings.Compare(a.Name, b.Name) })
	return sources, nil
}

// gitHubLibrary returns the repository a source's address names on GitHub, as owner/name, such as fabricahq/public-rules
// for https://github.com/fabricahq/public-rules.git, or empty for an address on another host or one it can't read.
func gitHubLibrary(address string) string {
	rest, ok := strings.CutPrefix(address, "https://github.com/")
	if !ok {
		return ""
	}
	rest = strings.TrimSuffix(strings.TrimSuffix(rest, "/"), ".git")
	owner, name, ok := strings.Cut(rest, "/")
	if !ok || !githubLogin.MatchString(owner) || name == "" || strings.ContainsAny(name, "/?#") || len(name) > 100 {
		return ""
	}
	return owner + "/" + name
}
