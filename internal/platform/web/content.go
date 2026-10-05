// The pages whose words are written in Markdown, in content/<page>.md, which make generate renders as
// content/<page>.generated.html: each page's front matter, then its body as HTML. Both may hold template actions, such
// as {{.CodeRulesURL}}, which run when the server starts.

package web

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"strings"

	"github.com/a-h/templ"
	"go.yaml.in/yaml/v4"
)

//go:embed content/*.generated.html
var contentFiles embed.FS

// contentValues are what a content page's template actions can name, as {{.CodeRulesURL}} or {{if .CanList}}.
type contentValues struct {
	// CanList is true when visitors can list a library on this server, so the vetting page leads to the form.
	CanList bool
	// Analytics is true when this server counts visits with Cloudflare Web Analytics, which the privacy page names.
	Analytics bool

	CodeRulesURL, CodeRulesLibrariesURL, RepositoryURL, VettedFileURL, AskToVetURL, ReportFormsURL string
	UnvettedHref, ListHref, VettingHref, PrivacyHref, FeedbackHref, DashboardHref                  string
	UnvettedOptInLabel, UnvettedWarningText                                                        string
}

// newContentValues returns the values content pages name, on a server where canList and analytics say what it offers.
func newContentValues(canList, analytics bool) contentValues {
	return contentValues{
		CanList: canList, Analytics: analytics,
		CodeRulesURL: codeRulesURL, CodeRulesLibrariesURL: codeRulesLibrariesURL, RepositoryURL: repositoryURL,
		VettedFileURL: vettedFileURL, AskToVetURL: askToVetURL, ReportFormsURL: reportFormsURL,
		UnvettedHref: unvettedHref, ListHref: listHref, VettingHref: vettingHref, PrivacyHref: privacyHref,
		FeedbackHref: feedbackHref, DashboardHref: dashboardHref,
		UnvettedOptInLabel: unvettedOptInLabel, UnvettedWarningText: unvettedWarningText,
	}
}

// contentPage is a content page as a server shows it.
type contentPage struct {
	// title names the page in browsers' tabs and search results, and heading is its h1.
	title, heading, description, eyebrow string
	// lede is the HTML under the heading, nil for a page without one, and body the HTML of its prose.
	lede, body templ.Component
}

// contentPages are the pages written in Markdown.
type contentPages struct {
	about, vetting, privacy contentPage
}

// loadContentPages returns the content pages in files, as their generated HTML, with their template actions run with
// values. It runs them with every combination of values' switches too, so a page that names a value contentValues
// lacks, or isn't valid HTML, fails every server's start, whichever switches it has.
func loadContentPages(files fs.FS, values contentValues) (contentPages, error) {
	load := func(name string) (contentPage, error) {
		page, err := loadContentPage(files, name, values)
		if err != nil {
			return contentPage{}, fmt.Errorf("load content page %s: %v", name, err)
		}
		return page, nil
	}
	var pages contentPages
	var err error
	if pages.about, err = load("about"); err != nil {
		return contentPages{}, err
	}
	if pages.vetting, err = load("vetting"); err != nil {
		return contentPages{}, err
	}
	if pages.privacy, err = load("privacy"); err != nil {
		return contentPages{}, err
	}
	return pages, nil
}

// contentFrontMatter is what a content page's front matter says.
type contentFrontMatter struct {
	Title       string `yaml:"title"`
	Heading     string `yaml:"heading"`
	Description string `yaml:"description"`
	Eyebrow     string `yaml:"eyebrow"`
	// Lede is HTML, and may hold template actions.
	Lede string `yaml:"lede"`
}

// loadContentPage returns the page name, from content/<name>.generated.html in files, with values.
func loadContentPage(files fs.FS, name string, values contentValues) (contentPage, error) {
	generated, err := fs.ReadFile(files, "content/"+name+".generated.html")
	if err != nil {
		return contentPage{}, err
	}
	frontMatter, body, err := splitContentFrontMatter(generated)
	if err != nil {
		return contentPage{}, err
	}
	var front contentFrontMatter
	decoder := yaml.NewDecoder(bytes.NewReader(frontMatter))
	decoder.KnownFields(true)
	if err := decoder.Decode(&front); err != nil {
		return contentPage{}, fmt.Errorf("read front matter: %v", err)
	}
	for field, value := range map[string]string{"title": front.Title, "heading": front.Heading, "description": front.Description, "eyebrow": front.Eyebrow} {
		if value == "" {
			return contentPage{}, fmt.Errorf("front matter has no %s", field)
		}
	}
	lede, err := runContentTemplate("lede", front.Lede, values)
	if err != nil {
		return contentPage{}, err
	}
	prose, err := runContentTemplate("body", string(body), values)
	if err != nil {
		return contentPage{}, err
	}
	page := contentPage{title: front.Title, heading: front.Heading, description: front.Description, eyebrow: front.Eyebrow, body: templ.Raw(prose)}
	if lede != "" {
		page.lede = templ.Raw(lede)
	}
	return page, nil
}

// runContentTemplate returns the HTML text makes with values, after checking that it makes HTML with every
// combination of values' switches.
func runContentTemplate(name, text string, values contentValues) (string, error) {
	t, err := template.New(name).Option("missingkey=error").Parse(text)
	if err != nil {
		return "", fmt.Errorf("parse %s: %v", name, err)
	}
	var html string
	for _, canList := range []bool{false, true} {
		for _, analytics := range []bool{false, true} {
			variant := values
			variant.CanList, variant.Analytics = canList, analytics
			var out strings.Builder
			if err := t.Execute(&out, variant); err != nil {
				return "", fmt.Errorf("run %s: %v", name, err)
			}
			if variant == values {
				html = strings.TrimSpace(out.String())
			}
		}
	}
	return html, nil
}

// splitContentFrontMatter returns page's front matter, between its opening and closing --- lines, and the body after.
func splitContentFrontMatter(page []byte) (frontMatter, body []byte, err error) {
	const fence = "---\n"
	if !bytes.HasPrefix(page, []byte(fence)) {
		return nil, nil, errors.New("doesn't start with front matter, a --- line")
	}
	end := bytes.Index(page[len(fence):], []byte("\n"+fence))
	if end < 0 {
		return nil, nil, errors.New("front matter has no closing --- line")
	}
	return page[len(fence) : len(fence)+end+1], page[len(fence)+end+len("\n"+fence):], nil
}
