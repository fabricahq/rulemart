// The pages about Rulemart itself, what it is and how it treats visitors' data, and the places on GitHub where
// visitors ask for a library's vetting or report a problem, which keep no reports of Rulemart's own.

package web

import (
	"net/http"
	"net/url"
)

const (
	aboutHref   = "/about"
	vettingHref = aboutHref + "/vetting"
	privacyHref = "/privacy"
)

const (
	// repositoryURL is Rulemart's public repository, whose issues take reports and requests.
	repositoryURL = "https://github.com/fabricahq/rulemart"
	// vettedFileURL is the file that vets libraries, whose history shows when and why each was vetted.
	vettedFileURL = repositoryURL + "/blob/main/catalog/vetted.yaml"
	// reportFormsURL lets a visitor choose which report to file, and reportProblemURL opens the Report a problem form.
	reportFormsURL   = repositoryURL + "/issues/new/choose"
	reportProblemURL = repositoryURL + "/issues/new?template=report-a-problem.yml"
	// codeRulesURL is Code Rules' documentation, and codeRulesLibrariesURL its page on libraries and their releases.
	codeRulesURL          = "https://code-rules.fabricahq.com"
	codeRulesLibrariesURL = codeRulesURL + "/concepts/libraries/"
)

// issueFormURL returns the address that opens a new issue in Rulemart's repository with the issue form template,
// a file in .github/ISSUE_TEMPLATE, its title, and each of its fields filled in from fields, by field ID.
func issueFormURL(template, title string, fields map[string]string) string {
	q := url.Values{"template": {template}, "title": {title}}
	for id, value := range fields {
		q.Set(id, value)
	}
	return repositoryURL + "/issues/new?" + q.Encode()
}

// reportLibraryURL returns where a visitor reports the library fullName, owner/name, with it filled in.
func reportLibraryURL(fullName string) string {
	return issueFormURL("report-a-library.yml", "Report: "+fullName, map[string]string{"library": fullName})
}

// askToVetURL is where a visitor asks Fabrica to vet a library.
var askToVetURL = issueFormURL("ask-to-vet-a-library.yml", "Vet: ", nil)

// about shows what Rulemart and Code Rules are, and how Rulemart is meant to be used.
func (s *server) about(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, aboutPage(s.pageChrome(aboutHref)))
}

// vetting shows what vetting a library means, what an unvetted one is, how a library gets vetted, and where to
// report a problem.
func (s *server) vetting(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, vettingPage(s.pageChrome(vettingHref), s.listingAvailable()))
}

// privacy shows what Rulemart keeps about visitors, why, for how long, and who else handles it.
func (s *server) privacy(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, privacyPage(s.pageChrome(privacyHref), s.AnalyticsToken != ""))
}
