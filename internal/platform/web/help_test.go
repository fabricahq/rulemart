package web_test

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// The FAQ answers every question as a disclosure, and leads to feedback for any other, under its own address.
func TestFAQAnswersEachQuestionAndLeadsToFeedback(t *testing.T) {
	handler := newSiteAt(t, newCatalog(), "https://rulemart.example")

	resp := get(t, handler, "/faq")

	if resp.Code != http.StatusOK {
		t.Fatalf("answered %d", resp.Code)
	}
	page := resp.Body.String()
	if !strings.Contains(page, "<title>FAQ · Rulemart</title>") {
		t.Error("the title doesn't name the FAQ")
	}
	assertShows(t, page, "FAQ Questions and answers", "Still have a question? Ask us .",
		// What an account adds: the dashboard's project tracking and checkout's project picker.
		"Easier checkout", "Project tracking")
	questions := []string{
		"What is Rulemart?", "Can I use Rulemart with any project?", "What is Code Rules?", "How do I use Rulemart?",
		"What are rules, groups, and libraries?", "Should I stay in sync with a rule or fork it?", "How are rules versioned?",
		"Who can publish a library?", "Do I need a Rulemart account?", "How do I give feedback?",
	}
	if got := summaries(t, page); !slices.Equal(got, questions) {
		if got := links(t, page, "About Rulemart"); !slices.Contains(got, "/about") {
			t.Errorf("the What is Rulemart answer leads to %q, want /about", got)
		}
		t.Errorf("the questions are %q, want %q", got, questions)
	}
	// Each question's + or − marker, which CSS draws, is hidden from screen readers, so the disclosure's name is the
	// question alone.
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	for n := range doc.Descendants() {
		if n.Data == "summary" && find(n, func(c *html.Node) bool { return attribute(c, "aria-hidden") == "true" }) == nil {
			t.Errorf("%q doesn't hide its marker from screen readers", nodeText(n))
		}
	}
	for text, want := range map[string]string{
		"Ask us":              "/feedback",
		"feedback page":       "/feedback",
		"unvetted libraries":  "/unvetted",
		"Code Rules":          "https://code-rules.fabricahq.com",
		"semantic versioning": "https://code-rules.fabricahq.com/reference/rule-versions/#choose-a-version-change",
	} {
		if got := links(t, page, text); !slices.Contains(got, want) {
			t.Errorf("%q leads to %q, want %s", text, got, want)
		}
	}
	if got := canonicalLinks(t, page); !slices.Equal(got, []string{"https://rulemart.example/faq"}) {
		t.Errorf("names %q as canonical", got)
	}
}

// summaries returns the text of each disclosure's summary in an HTML body's main content, in order.
func summaries(t *testing.T, body string) []string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	main := find(doc, func(n *html.Node) bool { return n.Data == "main" })
	if main == nil {
		t.Fatal("the page has no main content")
	}
	var texts []string
	for n := range main.Descendants() {
		if n.Type == html.ElementNode && n.Data == "summary" {
			texts = append(texts, nodeText(n))
		}
	}
	return texts
}

// Each feedback topic opens a new issue, prefilled, in the repository that owns the topic: Rulemart's for Rulemart,
// Code Rules' for Code Rules, titled with the topic and labeled feedback and topic:<key>. Something broken leads to
// the Report a problem form, and feedback about one rule goes to its library.
func TestFeedbackTopicsOpenPrefilledIssuesInTheRepositoryThatOwnsThem(t *testing.T) {
	handler := newSiteAt(t, newCatalog(), "https://rulemart.example")

	resp := get(t, handler, "/feedback")

	if resp.Code != http.StatusOK {
		t.Fatalf("answered %d", resp.Code)
	}
	page := resp.Body.String()
	assertShows(t, page,
		"Feedback Give us feedback",
		"Rulemart Using Rulemart Finding rules, library and rule pages, checking out, and anything missing. ↗ GitHub",
		"Something broken on Rulemart A page that's wrong, empty, or won't load. ↗ GitHub",
		"Code Rules The tool The code-rules CLI: commands, syncing, error messages, and config. ↗ GitHub",
		"Rule format Frontmatter fields, groups, impact levels, and how rules are written. ↗ GitHub",
		"Documentation Anything unclear, wrong, or hard to find. ↗ GitHub",
		"A specific rule Open an issue in the library's repository, which the rule's page links.",
		"Anything else Ideas and questions Anything that doesn't fit above. ↗ GitHub",
	)
	for label, want := range map[string]struct{ repository, key string }{
		"Using Rulemart":      {"fabricahq/rulemart", "hub"},
		"The tool":            {"fabricahq/code-rules", "cli"},
		"Rule format":         {"fabricahq/code-rules", "format"},
		"Documentation":       {"fabricahq/code-rules", "docs"},
		"Ideas and questions": {"fabricahq/rulemart", "other"},
	} {
		got := links(t, page, label)
		if len(got) != 1 {
			t.Errorf("%s links %q", label, got)
			continue
		}
		u, err := url.Parse(got[0])
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		if u.Host != "github.com" || u.Path != "/"+want.repository+"/issues/new" {
			t.Errorf("%s opens an issue at %s", label, got[0])
		}
		if q.Get("title") != "["+label+"] " || q.Get("labels") != "feedback,topic:"+want.key {
			t.Errorf("%s is titled %q with labels %q", label, q.Get("title"), q.Get("labels"))
		}
		if body := q.Get("body"); !strings.HasPrefix(body, "**Topic:** "+label+"\n") || !strings.HasSuffix(body, "Opened from Rulemart") {
			t.Errorf("%s has the body %q", label, body)
		}
	}
	if got := links(t, page, "Something broken on Rulemart"); !slices.Equal(got, []string{"https://github.com/fabricahq/rulemart/issues/new?template=report-a-problem.yml"}) {
		t.Errorf("something broken leads to %q", got)
	}
	if got := canonicalLinks(t, page); !slices.Equal(got, []string{"https://rulemart.example/feedback"}) {
		t.Errorf("names %q as canonical", got)
	}
}
