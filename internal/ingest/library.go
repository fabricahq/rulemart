// Read what a library's releases published: its license, its groups, and each current rule's content.

package ingest

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// library is everything the catalog stores about one library.
type library struct {
	repo Repository
	// licenseExpression and licenseFile are what rule-library.yaml declares at the latest release; each is empty
	// when it declares none.
	licenseExpression, licenseFile string
	// releases are in number order, from 1.
	releases []release
	// groups are the groups of every rule, current or retired, in ID order.
	groups []group
	// rules are every rule the releases published, in ID order.
	rules []rule
}

// group is a group's metadata at the latest release that has its _group.yaml: the latest release while the group
// holds a current rule.
type group struct {
	id   string
	meta coderules.GroupMetadata
}

// rule is a rule's history, with its current content while it's current.
type rule struct {
	ruleHistory
	group string
	// content is the current version's content; nil when the rule is retired.
	content *content
}

// content is a rule as one version published it.
type content struct {
	title, impact, impactDescription, whenToRead string
	// markdown is the rule's whole file, and html its body as the page shows it.
	markdown, html string
}

// readLibrary reads the files releases publish for repo: rule-library.yaml at the latest release, each group's
// _group.yaml at the latest release that has it, and each current rule's file at the release that published its
// current version.
func readLibrary(repo Repository, releases []release, histories []ruleHistory, contentBytes int64) (library, error) {
	latest := releases[len(releases)-1]
	lib := library{repo: repo, releases: releases}
	var err error
	if lib.licenseExpression, lib.licenseFile, err = readLicense(latest, repo.FullName()); err != nil {
		return library{}, err
	}
	// current reports, for the group of every rule, whether it holds a current rule.
	current := map[string]bool{}
	budget := &contentBudget{limit: contentBytes}
	for _, history := range histories {
		r, err := readRule(repo, releases, history, budget)
		if err != nil {
			return library{}, err
		}
		current[r.group] = current[r.group] || r.content != nil
		lib.rules = append(lib.rules, r)
	}
	for _, id := range slices.Sorted(maps.Keys(current)) {
		g, err := readGroup(releases, id, current[id], budget)
		if err != nil {
			return library{}, err
		}
		lib.groups = append(lib.groups, g)
	}
	return lib, nil
}

// contentBudget bounds the content one ingestion reads and holds until it writes the library: each rule's Markdown
// and HTML, and each group's metadata. Git stores a file once however many paths share it, so the fetch limits
// can't bound this: a small release can list thousands of rules or groups that share one large file.
type contentBudget struct {
	limit, spent int64
}

// spend records n more bytes of rule content, or refuses them when they would pass the limit.
func (b *contentBudget) spend(n int64) error {
	if b.spent+n > b.limit {
		return fmt.Errorf("the library's rules and groups hold more than %d bytes of content, which ingestion won't hold", b.limit)
	}
	b.spent += n
	return nil
}

// readLicense returns the license expression and file that rule-library.yaml declares at release, each empty
// when it declares none. It refuses a declared file the release doesn't hold.
func readLicense(r release, source string) (expression, file string, err error) {
	manifest, err := readFile(r.commit, "rule-library.yaml", nil)
	if errors.Is(err, errFileMissing) {
		return "", "", fmt.Errorf("%s: rule-library.yaml is missing; every Code Rules library has one", r.tag)
	}
	if err != nil {
		return "", "", fmt.Errorf("%s: rule-library.yaml: %v", r.tag, err)
	}
	license, err := coderules.ParseLibraryLicense(manifest, source)
	if err != nil {
		return "", "", fmt.Errorf("%s: %v", r.tag, err)
	}
	if license == nil {
		return "", "", nil
	}
	// The library page links to the license file at this release, so it must be there.
	file = license.Files[0]
	if _, err := readFile(r.commit, file, nil); errors.Is(err, errFileMissing) {
		return "", "", fmt.Errorf("%s: rule-library.yaml declares the license file %s, which is missing", r.tag, file)
	} else if err != nil {
		return "", "", fmt.Errorf("%s: %s: %v", r.tag, file, err)
	}
	if license.SPDXExpression != nil {
		expression = *license.SPDXExpression
	}
	return expression, file, nil
}

// readRule returns history's rule, reading a current rule's file at the release that published its current
// version.
func readRule(repo Repository, releases []release, history ruleHistory, budget *contentBudget) (rule, error) {
	path := history.id + ".md"
	groupID, err := coderules.GroupFromPath(path, path)
	if err != nil {
		return rule{}, err
	}
	r := rule{ruleHistory: history, group: groupID}
	if history.retiredIn != 0 {
		return r, nil
	}
	published := releases[history.current().release-1]
	text, err := readFile(published.commit, path, budget.spend)
	if err != nil {
		return rule{}, fmt.Errorf("%s: %s: %v", published.tag, path, err)
	}
	parsed, err := coderules.Parse(string(text), path, repo.FullName())
	if err != nil {
		return rule{}, fmt.Errorf("%s: %v", published.tag, err)
	}
	// The metadata the page shows is decoded from the frontmatter into copies of its own, which stay with the rule.
	if err := budget.spend(int64(len(parsed.Title) + len(parsed.ImpactDescription) + len(parsed.WhenToRead))); err != nil {
		return rule{}, fmt.Errorf("%s: %s: %v", published.tag, path, err)
	}
	document, err := coderules.SplitDocument(parsed.Document, path)
	if err != nil {
		return rule{}, fmt.Errorf("%s: %v", published.tag, err)
	}
	html, err := renderRule(document.Body, rulePage{
		repository: repo.FullName(), path: path, title: parsed.Title,
		tag: published.tag, latestTag: releases[len(releases)-1].tag,
	}, budget)
	if err != nil {
		return rule{}, fmt.Errorf("%s: %s: %v", published.tag, path, err)
	}
	r.content = &content{
		title: strings.TrimSpace(parsed.Title), impact: string(parsed.Impact),
		impactDescription: strings.TrimSpace(parsed.ImpactDescription), whenToRead: strings.TrimSpace(parsed.WhenToRead),
		markdown: parsed.Document, html: html,
	}
	return r, nil
}

// readGroup reads group id's _group.yaml, spending budget on it. A group with a current rule has one at the latest
// release. A group whose rules are all retired keeps the metadata of the latest release that has the file.
func readGroup(releases []release, id string, current bool, budget *contentBudget) (group, error) {
	path := id + "/_group.yaml"
	for i := len(releases) - 1; i >= 0; i-- {
		r := releases[i]
		text, err := readFile(r.commit, path, budget.spend)
		if errors.Is(err, errFileMissing) && !current {
			continue
		}
		if err != nil {
			return group{}, fmt.Errorf("%s: %s: %v", r.tag, path, err)
		}
		meta, err := coderules.ParseGroupMetadataYAML(text, path)
		if err != nil {
			return group{}, fmt.Errorf("%s: %v", r.tag, err)
		}
		return group{id: id, meta: meta}, nil
	}
	return group{}, fmt.Errorf("%s: no library release has it, though its rules were published", path)
}
