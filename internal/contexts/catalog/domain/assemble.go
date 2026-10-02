// Assemble a library from its release snapshots: its history, license, groups, and each current rule's content.

package domain

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// Assemble returns the library that releases publish for repo. releases are the library's release snapshots in
// number order. It checks that their records form one history, then reads rule-library.yaml at the latest
// release, each group's _group.yaml at the latest release that has it, and each rule's file at the release that
// published each of its versions, rendering the current version of each current rule with render. Errors name the
// release and file at fault.
func Assemble(repo Repository, releases []ReleaseSnapshot, limits ContentLimits, render Render) (Library, error) {
	if len(releases) == 0 {
		return Library{}, errors.New("the library has no releases")
	}
	records := make([]coderules.ReleaseRecord, len(releases))
	for i, r := range releases {
		records[i] = r.Record
	}
	histories, err := buildHistory(records)
	if err != nil {
		return Library{}, err
	}
	a := assembly{repo: repo, releases: releases, limits: limits, renderBody: render, budget: contentBudget{limit: limits.ContentBytes}}
	lib := Library{Repository: repo}
	for _, r := range releases {
		lib.Releases = append(lib.Releases, r.release())
	}
	if lib.LicenseExpression, lib.LicenseFile, err = a.readLicense(); err != nil {
		return Library{}, err
	}
	// current reports, for the group of every rule, whether it holds a current rule.
	current := map[string]bool{}
	for _, history := range histories {
		r, err := a.readRule(history)
		if err != nil {
			return Library{}, err
		}
		current[r.Group] = current[r.Group] || r.IsCurrent()
		lib.Rules = append(lib.Rules, r)
	}
	for _, path := range slices.Sorted(maps.Keys(current)) {
		g, err := a.readGroup(path, current[path])
		if err != nil {
			return Library{}, err
		}
		lib.Groups = append(lib.Groups, g)
	}
	return lib, nil
}

// assembly reads one library's files within its limits.
type assembly struct {
	repo     Repository
	releases []ReleaseSnapshot
	limits   ContentLimits
	// renderBody renders a current rule's Markdown body.
	renderBody Render
	budget     contentBudget
}

// contentBudget is what's left of ContentLimits.ContentBytes as assembly reads and renders.
type contentBudget struct {
	limit, spent int64
}

// spend records n more bytes of content, or refuses them when they would pass the limit.
func (b *contentBudget) spend(n int64) error {
	if b.spent+n > b.limit {
		return b.exceeded()
	}
	b.spent += n
	return nil
}

// remaining returns how many more bytes the budget allows.
func (b *contentBudget) remaining() int64 { return b.limit - b.spent }

// exceeded returns the error that refuses content past the limit.
func (b *contentBudget) exceeded() error {
	return fmt.Errorf("the library's rules and groups hold more than %d bytes of content, which ingestion won't hold", b.limit)
}

// open returns the file at path in release r, without reading it. It fails with ErrFileMissing when there's none,
// and refuses a file larger than ContentLimits.FileBytes.
func (a *assembly) open(r ReleaseSnapshot, path string) (File, error) {
	file, err := r.Files.Open(path)
	if err != nil {
		return nil, err
	}
	if file.Size() > a.limits.FileBytes {
		return nil, fmt.Errorf("the file is %d bytes, more than the %d ingestion reads", file.Size(), a.limits.FileBytes)
	}
	return file, nil
}

// readContent returns the file at path in release r, spending its size from the budget before reading it.
func (a *assembly) readContent(r ReleaseSnapshot, path string) ([]byte, error) {
	file, err := a.open(r, path)
	if err != nil {
		return nil, err
	}
	if err := a.budget.spend(file.Size()); err != nil {
		return nil, err
	}
	return file.Read()
}

// readLicense returns the license expression and file that rule-library.yaml declares at the latest release, each
// empty when it declares none. It refuses a declared file the release doesn't hold.
func (a *assembly) readLicense() (expression, file string, err error) {
	latest := a.releases[len(a.releases)-1]
	manifest, err := a.open(latest, ManifestFile)
	if errors.Is(err, ErrFileMissing) {
		return "", "", fmt.Errorf("%s: %s is missing; every Code Rules library has one", latest.Tag, ManifestFile)
	}
	if err != nil {
		return "", "", fmt.Errorf("%s: %s: %v", latest.Tag, ManifestFile, err)
	}
	text, err := manifest.Read()
	if err != nil {
		return "", "", fmt.Errorf("%s: %s: %v", latest.Tag, ManifestFile, err)
	}
	license, err := coderules.ParseLibraryLicense(text, a.repo.FullName())
	if err != nil {
		return "", "", fmt.Errorf("%s: %v", latest.Tag, err)
	}
	if license == nil {
		return "", "", nil
	}
	// The library page links to the license file at this release, so it must be there.
	file = license.Files[0]
	if _, err := a.open(latest, file); errors.Is(err, ErrFileMissing) {
		return "", "", fmt.Errorf("%s: %s declares the license file %s, which is missing", latest.Tag, ManifestFile, file)
	} else if err != nil {
		return "", "", fmt.Errorf("%s: %s: %v", latest.Tag, file, err)
	}
	if license.SPDXExpression != nil {
		expression = *license.SPDXExpression
	}
	return expression, file, nil
}

// readRule returns history's rule in its group, with each version's content: the rule's file at the release that
// published the version. While the rule is current, it also renders the current version's body.
func (a *assembly) readRule(history Rule) (Rule, error) {
	path := RuleFile(history.Path)
	group, err := coderules.GroupFromPath(path, path)
	if err != nil {
		return Rule{}, err
	}
	r := history
	r.Group = group
	r.Versions = slices.Clone(history.Versions)
	for i, v := range r.Versions {
		published := a.releases[v.Release-1]
		parsed, err := a.readVersion(published, path)
		if err != nil {
			return Rule{}, fmt.Errorf("%s: %v", published.Tag, err)
		}
		r.Versions[i].Content = Content{
			Title: strings.TrimSpace(parsed.Title), Impact: string(parsed.Impact),
			ImpactDescription: strings.TrimSpace(parsed.ImpactDescription), WhenToRead: strings.TrimSpace(parsed.WhenToRead),
			Markdown: parsed.Document,
		}
		if !r.IsCurrent() || i < len(r.Versions)-1 {
			continue
		}
		if r.HTML, err = a.renderRule(published, path, parsed); err != nil {
			return Rule{}, fmt.Errorf("%s: %s: %v", published.Tag, path, err)
		}
	}
	return r, nil
}

// readVersion reads and parses the rule file at path in release r, spending the budget on what the catalog keeps of
// it: the file, and its title, impact description, and reading guidance, which are decoded into copies of their own.
// Errors name the file.
func (a *assembly) readVersion(r ReleaseSnapshot, path string) (coderules.Rule, error) {
	text, err := a.readContent(r, path)
	if err != nil {
		return coderules.Rule{}, fmt.Errorf("%s: %v", path, err)
	}
	parsed, err := coderules.Parse(string(text), path, a.repo.FullName())
	if err != nil {
		return coderules.Rule{}, err
	}
	if err := a.budget.spend(int64(len(parsed.Title) + len(parsed.ImpactDescription) + len(parsed.WhenToRead))); err != nil {
		return coderules.Rule{}, fmt.Errorf("%s: %v", path, err)
	}
	return parsed, nil
}

// renderRule renders the body of parsed, the rule file at path in release r, for the rule's page.
func (a *assembly) renderRule(r ReleaseSnapshot, path string, parsed coderules.Rule) (string, error) {
	document, err := coderules.SplitDocument(parsed.Document, path)
	if err != nil {
		return "", err
	}
	return a.render(document.Body, RulePage{
		Repository: a.repo.FullName(), Path: path, Title: parsed.Title,
		Tag: r.Tag, LatestTag: a.releases[len(a.releases)-1].Tag,
	})
}

// render renders a rule's body within what's left of the budget, and spends what it used.
func (a *assembly) render(body string, page RulePage) (string, error) {
	html, used, err := a.renderBody(body, page, a.budget.remaining())
	if errors.Is(err, ErrOverAllowance) {
		return "", a.budget.exceeded()
	}
	if err != nil {
		return "", err
	}
	if err := a.budget.spend(used); err != nil {
		return "", err
	}
	return html, nil
}

// readGroup reads the _group.yaml of the group at path, spending budget on it. A group with a current rule has one
// at the latest release. A group whose rules are all retired keeps the metadata of the latest release that has the
// file.
func (a *assembly) readGroup(path string, current bool) (Group, error) {
	file := GroupFile(path)
	for i := len(a.releases) - 1; i >= 0; i-- {
		r := a.releases[i]
		text, err := a.readContent(r, file)
		if errors.Is(err, ErrFileMissing) && !current {
			continue
		}
		if err != nil {
			return Group{}, fmt.Errorf("%s: %s: %v", r.Tag, file, err)
		}
		meta, err := coderules.ParseGroupMetadataYAML(text, file)
		if err != nil {
			return Group{}, fmt.Errorf("%s: %v", r.Tag, err)
		}
		return Group{Path: path, Name: meta.Name, Description: meta.Description, WhenToRead: meta.WhenToRead}, nil
	}
	return Group{}, fmt.Errorf("%s: no library release has it, though its rules were published", file)
}
