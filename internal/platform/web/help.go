// The pages that help visitors: the FAQ, and the feedback page, whose topics open prefilled GitHub issues in the
// repository that owns each topic.

package web

import (
	"net/http"
	"net/url"
)

const (
	faqHref      = "/faq"
	feedbackHref = "/feedback"
)

// codeRulesRepositoryURL is Code Rules' public repository, whose issues take feedback about the tool, the rule
// format, and the documentation.
const codeRulesRepositoryURL = "https://github.com/fabricahq/code-rules"

// feedbackTopic is one row of the feedback page: what the feedback is about, and the repository whose issues take it.
type feedbackTopic struct {
	// key names the topic's label, topic:<key>, which both repositories have.
	key, label, example string
	// repository is the GitHub repository, owner/name, that owns the topic.
	repository string
}

// feedbackGroup is one heading of the feedback page and its topics.
type feedbackGroup struct {
	name   string
	topics []feedbackTopic
}

// feedbackGroups are the feedback page's topics, in its order. Rulemart topics open issues in Rulemart's repository,
// and Code Rules topics in Code Rules'.
var feedbackGroups = []feedbackGroup{
	{name: "Rulemart", topics: []feedbackTopic{
		{key: "hub", label: "Using Rulemart", example: "Finding rules, library and rule pages, checking out, and anything missing.", repository: "fabricahq/rulemart"},
	}},
	{name: "Code Rules", topics: []feedbackTopic{
		{key: "cli", label: "The tool", example: "The code-rules CLI: commands, syncing, error messages, and config.", repository: "fabricahq/code-rules"},
		{key: "format", label: "Rule format", example: "Frontmatter fields, groups, impact levels, and how rules are written.", repository: "fabricahq/code-rules"},
		{key: "docs", label: "Documentation", example: "Anything unclear, wrong, or hard to find.", repository: "fabricahq/code-rules"},
	}},
	{name: "Anything else", topics: []feedbackTopic{
		{key: "other", label: "Ideas and questions", example: "Anything that doesn't fit above.", repository: "fabricahq/rulemart"},
	}},
}

// issueURL returns the address that opens a new issue about the topic in its repository: titled with the topic's
// label as a prefix, since GitHub drops labels for people who can't set them, with the labels feedback and
// topic:<key>, and a body that asks what happened and why it matters.
func (t feedbackTopic) issueURL() string {
	body := "**Topic:** " + t.label + "\n\n**What happened, or what would you change?**\n\n\n**Why it matters to you:**\n\n\n---\nOpened from Rulemart"
	q := url.Values{"title": {"[" + t.label + "] "}, "body": {body}, "labels": {"feedback,topic:" + t.key}}
	return "https://github.com/" + t.repository + "/issues/new?" + q.Encode()
}

// faq shows the questions visitors ask most, and their answers.
func (s *server) faq(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, faqPage(s.pageChrome(faqHref)))
}

// feedback shows the topics a visitor can give feedback on, each opening a prefilled issue on GitHub.
func (s *server) feedback(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, feedbackPage(s.pageChrome(feedbackHref)))
}
