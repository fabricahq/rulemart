// Assemble a library from its release snapshots: its history, license, groups, each current rule's content, and its
// assets.

package domain

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/trailguide"
)

// Assemble returns the library that releases publish for repo. releases are the library's release snapshots in
// number order. It checks that their records form one history, finds where each release keeps the library's files,
// its Layout, then reads the manifest at the latest release, each group's _group.yaml at the latest release that has
// it, each rule's file at the release that published each of its versions, and each current rule's assets, rendering
// with renderer each rule's newest version, a current rule's body and reading guidance and a retired rule's last
// body, and the Markdown and text among the assets. Files are named by their library paths, which are the same in
// every layout. Errors name the release and file at fault.
func Assemble(repo Repository, releases []ReleaseSnapshot, limits ContentLimits, renderer Renderer) (Library, error) {
	if len(releases) == 0 {
		return Library{}, errors.New("the library has no releases")
	}
	records := make([]trailguide.ReleaseRecord, len(releases))
	for i, r := range releases {
		records[i] = r.Record
	}
	histories, err := buildHistory(records)
	if err != nil {
		return Library{}, err
	}
	a := assembly{
		repo: repo, releases: releases, layouts: make([]Layout, len(releases)), limits: limits, renderer: renderer,
		budget: contentBudget{limit: limits.ContentBytes},
		read:   assetReader{assets: map[string]*Asset{}, missing: map[string]bool{}, sharedLinks: map[string][]string{}},
	}
	lib := Library{Repository: repo}
	for i, r := range releases {
		if a.layouts[i], err = DetectLayout(r.Files); err != nil {
			return Library{}, fmt.Errorf("%s: %v", r.Tag, err)
		}
		lib.Releases = append(lib.Releases, r.release(a.layouts[i]))
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
	if err := a.renderSharedAssets(); err != nil {
		return Library{}, fmt.Errorf("%s: %v", a.latest().Tag, err)
	}
	lib.Assets = a.libraryAssets()
	return lib, nil
}

// assembly reads one library's files within its limits.
type assembly struct {
	repo     Repository
	releases []ReleaseSnapshot
	// layouts holds where each of releases keeps the library's files, in the same order.
	layouts []Layout
	limits  ContentLimits
	// renderer renders rules' Markdown, and their assets' Markdown and code.
	renderer Renderer
	budget   contentBudget
	read     assetReader
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

// layout returns where release r, one of the library's releases, keeps the library's files.
func (a *assembly) layout(r ReleaseSnapshot) Layout { return a.layouts[r.Number-1] }

// latest returns the library's latest release.
func (a *assembly) latest() ReleaseSnapshot { return a.releases[len(a.releases)-1] }

// open returns the file at library path path in release r, without reading it, as openFile does.
func (a *assembly) open(r ReleaseSnapshot, path string) (File, error) {
	return a.openFile(r, a.layout(r).RepositoryPath(path))
}

// openFile returns the file at repository path path in release r, without reading it. It fails with ErrFileMissing
// when there's none, and refuses a file larger than ContentLimits.FileBytes.
func (a *assembly) openFile(r ReleaseSnapshot, path string) (File, error) {
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

// readLicense returns the license expression and file that the manifest declares at the latest release, each empty
// when it declares none. The file is its path in the repository, as the latest release holds it. It refuses a
// declared file the release doesn't hold.
func (a *assembly) readLicense() (expression, file string, err error) {
	latest := a.latest()
	layout := a.layout(latest)
	manifestFile := layout.ManifestFile()
	manifest, err := a.openFile(latest, manifestFile)
	if errors.Is(err, ErrFileMissing) && layout == RootLayout {
		return "", "", fmt.Errorf("%s: the library's manifest is missing: Trail Guide reads it from the library section of %s, or from %s at the repository root in the earlier layout", latest.Tag, TrailGuideLayout.ManifestFile(), manifestFile)
	}
	if errors.Is(err, ErrFileMissing) {
		return "", "", fmt.Errorf("%s: %s is missing; every Trail Guide library has one", latest.Tag, manifestFile)
	}
	if err != nil {
		return "", "", fmt.Errorf("%s: %s: %v", latest.Tag, manifestFile, err)
	}
	text, err := manifest.Read()
	if err != nil {
		return "", "", fmt.Errorf("%s: %s: %v", latest.Tag, manifestFile, err)
	}
	license, err := a.parseLicense(layout, text)
	if err != nil {
		return "", "", fmt.Errorf("%s: %v", latest.Tag, err)
	}
	if license == nil {
		return "", "", nil
	}
	// The library page links to the license file at this release, so it must be there.
	file = layout.RepositoryPath(license.Files[0])
	if _, err := a.openFile(latest, file); errors.Is(err, ErrFileMissing) {
		return "", "", fmt.Errorf("%s: %s declares the license file %s, which is missing", latest.Tag, manifestFile, file)
	} else if err != nil {
		return "", "", fmt.Errorf("%s: %s: %v", latest.Tag, file, err)
	}
	if license.SPDXExpression != nil {
		expression = *license.SPDXExpression
	}
	return expression, file, nil
}

// parseLicense returns the license that text, the manifest of a release in layout, declares, or nil when it declares
// none: rule-library.yaml's license in RootLayout, and otherwise the license in the library section of the
// configuration, which must have one.
func (a *assembly) parseLicense(layout Layout, text []byte) (*trailguide.LicenseDeclaration, error) {
	if layout == RootLayout {
		return parseLegacyLicense(text, a.repo.FullName()+"/"+trailguide.LegacyManifest)
	}
	manifestFile := layout.ManifestFile()
	library, err := trailguide.ParseLibraryManifest(text, a.repo.FullName()+"/"+manifestFile)
	var format *trailguide.UnsupportedLibraryFormatError
	if errors.As(err, &format) {
		return nil, unsupportedFormat(manifestFile, format)
	}
	if err != nil {
		return nil, err
	}
	if library == nil {
		return nil, fmt.Errorf("%s has no library section, which makes a repository a Trail Guide library", manifestFile)
	}
	return library.License, nil
}

// unsupportedFormat explains to the library's maintainer or lister that Rulemart can't read the library format that
// manifestFile declares, and what changes that: a newer Rulemart, or a library release in the format it reads. Only
// this library fails; the catalog keeps the releases it ingested before.
func unsupportedFormat(manifestFile string, format *trailguide.UnsupportedLibraryFormatError) error {
	if format.FormatVersion > format.Supported {
		return fmt.Errorf("%s declares library format %d, which Rulemart doesn't read yet: it reads library format %d. Rulemart will read this release once it supports that format", manifestFile, format.FormatVersion, format.Supported)
	}
	return fmt.Errorf("%s declares library format %d, which Rulemart no longer reads: it reads library format %d. Move the library to that format and publish a library release", manifestFile, format.FormatVersion, format.Supported)
}

// readRule returns history's rule in its group, with each version's content: the rule's file at the release that
// published the version. It renders the newest version's body, and while the rule is current, its reading guidance,
// and reads its assets.
func (a *assembly) readRule(history Rule) (Rule, error) {
	path := RuleFile(history.Path)
	group, err := trailguide.GroupFromPath(path, path)
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
		tags := ruleTags(parsed.Document)
		if err := a.budget.spend(int64(len(strings.Join(tags, "")))); err != nil {
			return Rule{}, fmt.Errorf("%s: %s: %v", published.Tag, path, err)
		}
		r.Versions[i].Content = Content{
			Title: strings.TrimSpace(parsed.Title), Impact: string(parsed.Impact),
			ImpactDescription: strings.TrimSpace(parsed.ImpactDescription), WhenToRead: strings.TrimSpace(parsed.WhenToRead),
			Tags: tags, Markdown: parsed.Document,
		}
		if i < len(r.Versions)-1 {
			continue
		}
		if r, err = a.renderRule(r, published, parsed); err != nil {
			return Rule{}, fmt.Errorf("%s: %v", published.Tag, err)
		}
	}
	return r, nil
}

// readVersion reads and parses the rule file at path in release r, spending the budget on what the catalog keeps of
// it: the file, and its title, impact description, and reading guidance, which are decoded into copies of their own.
// Errors name the file, by its path in the repository.
func (a *assembly) readVersion(r ReleaseSnapshot, path string) (trailguide.Rule, error) {
	text, err := a.readContent(r, path)
	if err != nil {
		return trailguide.Rule{}, fmt.Errorf("%s: %v", a.layout(r).RepositoryPath(path), err)
	}
	parsed, err := trailguide.ParseRule(string(text), path, a.repo.FullName())
	if err != nil {
		return trailguide.Rule{}, err
	}
	if err := a.budget.spend(int64(len(parsed.Title) + len(parsed.ImpactDescription) + len(parsed.WhenToRead))); err != nil {
		return trailguide.Rule{}, fmt.Errorf("%s: %v", path, err)
	}
	return parsed, nil
}

// renderRule returns r with its newest version, parsed, the rule's file at release published, rendered for the rule's
// page: its body, and while the rule is current, its reading guidance, and its assets, which it reads first, so links
// to them lead to their pages.
func (a *assembly) renderRule(r Rule, published ReleaseSnapshot, parsed trailguide.Rule) (Rule, error) {
	file := RuleFile(r.Path)
	document, err := trailguide.SplitDocument(parsed.Document, file)
	if err != nil {
		return Rule{}, err
	}
	whenToRead := strings.TrimSpace(parsed.WhenToRead)
	latest := a.latest()
	source := MarkdownSource{
		Repository: LibraryPlaceholder, File: file, Rule: r.Path, Title: parsed.Title,
		Tag: published.Tag, LatestTag: latest.Tag, Layout: a.layout(published), LatestLayout: a.layout(latest),
	}
	if r.IsCurrent() {
		own, shared, err := a.readAssets(published, r.Path, document.Body, whenToRead)
		if err != nil {
			return Rule{}, err
		}
		r.Assets = append(own, shared...)
		source.Assets = a.addresses(r.Path, r.Assets)
		// A shared file is rendered once, with the shared files, since a page of its own shows it to every rule.
		if err := a.renderAssets(own, source); err != nil {
			return Rule{}, err
		}
	}
	if r.HTML, err = a.render(document.Body, source); err != nil {
		return Rule{}, fmt.Errorf("%s: %v", file, err)
	}
	if !r.IsCurrent() {
		return r, nil
	}
	// The reading guidance is Markdown too, whose links resolve against the rule's file as the body's do.
	if r.WhenToReadHTML, err = a.render(whenToRead, source); err != nil {
		return Rule{}, fmt.Errorf("%s: reading guidance: %v", file, err)
	}
	return r, nil
}

// render renders Markdown from source within what's left of the budget, and spends what it used.
func (a *assembly) render(body string, source MarkdownSource) (string, error) {
	return a.spendRendering(a.renderer.Markdown(body, source, a.budget.remaining()))
}

// renderCode renders the text of file as code within what's left of the budget, and spends what it used.
func (a *assembly) renderCode(text, file string) (string, error) {
	return a.spendRendering(a.renderer.Code(text, file, a.budget.remaining()))
}

// spendRendering spends what a render used from the budget, and returns its HTML, or its error, as spendAllowance
// does.
func (a *assembly) spendRendering(html string, used int64, err error) (string, error) {
	if err := a.spendAllowance(used, err); err != nil {
		return "", err
	}
	return html, nil
}

// spendAllowance spends what the renderer used of the allowance it had, what's left of the budget, or returns its
// error, as the budget's refusal when it would have passed the allowance.
func (a *assembly) spendAllowance(used int64, err error) error {
	if errors.Is(err, ErrOverAllowance) {
		return a.budget.exceeded()
	}
	if err != nil {
		return err
	}
	return a.budget.spend(used)
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
			return Group{}, fmt.Errorf("%s: %s: %v", r.Tag, a.layout(r).RepositoryPath(file), err)
		}
		meta, err := trailguide.ParseGroupMetadata(text, file)
		if err != nil {
			return Group{}, fmt.Errorf("%s: %v", r.Tag, err)
		}
		return Group{Path: path, Name: meta.Name, Description: meta.Description, WhenToRead: meta.WhenToRead}, nil
	}
	return Group{}, fmt.Errorf("%s: no library release has it, though its rules were published", file)
}
