// Lockfile: what Trail Guide 0.7 and later record in a project, .trailguide/lock.json beside its configuration, which
// replaces the provenance file. Rulemart reads both through Trail Guide's projectview package, so it reads a project
// as Trail Guide does.

package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/trailguide"
	"github.com/fabricahq/trailguide/trailguide/projectview"
)

// MaxLockBytes bounds the lockfile a read parses. A project's grows with its sources' rules, skills, and files, about
// two hundred bytes each, so this holds several thousand.
const MaxLockBytes = 1 << 20

// LockPath returns where a project whose files are in directory, ProjectDirectory or LegacyProjectDirectory, keeps its
// lockfile, from its repository's root.
func LockPath(directory string) string { return directory + "/" + trailguide.LockFile }

// ParseLock returns the sources a project's configuration and lockfile name, by name, each with the groups it imports
// and the rules it holds at a published version that trailguide project update could change: the active rules it
// doesn't pin. An excluded or replaced rule, which agents don't receive, and a pinned rule, which updates keep, are
// left out, as are a source whose name Trail Guide wouldn't take, and an authored rule, which holds no library's
// version. It fails for a lockfile larger than MaxLockBytes, a configuration or lockfile Trail Guide would refuse, a
// lockfile without an entry for every source the configuration declares, and a project with no sources.
func ParseLock(config, lock []byte) ([]Source, error) {
	if len(lock) > MaxLockBytes {
		return nil, fmt.Errorf("parse lockfile: the file is larger than %d bytes", MaxLockBytes)
	}
	view, err := projectview.Read(config, lock)
	if err != nil {
		return nil, fmt.Errorf("parse lockfile: %w", err)
	}
	if len(view.Sources) == 0 {
		return nil, errors.New("parse lockfile: the project has no sources")
	}
	sources := make([]Source, 0, len(view.Sources))
	index := map[string]int{}
	for _, s := range view.Sources {
		if !sourceName.MatchString(s.Name) || s.Name == "local" {
			continue
		}
		sources = append(sources, Source{Name: s.Name, Library: gitHubLibrary(s.Repository), Release: s.Release, Groups: slices.Clone(s.Groups)})
		index[s.Name] = len(sources)
	}
	for _, rule := range view.Rules {
		at := index[rule.Source]
		if at == 0 || !rule.Active || rule.Pin != nil || rule.Version == nil {
			continue
		}
		sources[at-1].Rules = append(sources[at-1].Rules, PinnedRule{Path: rule.Path, Version: *rule.Version})
	}
	slices.SortFunc(sources, func(a, b Source) int { return strings.Compare(a.Name, b.Name) })
	return sources, nil
}
