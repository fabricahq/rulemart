package web_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// Search lists its rules under their groups, each heading leading to the group's page and counting its rules, the
// rules that hold every word first, and offers Kind besides a group's filters, and three orders.
func TestSearchPageGroupsItsRulesUnderTheirGroups(t *testing.T) {
	c := newBrowsingCatalog()
	partial := closeBodiesRow
	partial.Missing = []string{"handling"}
	results := c.results["errors"]
	results.Rows = append(slices.Clone(results.Rows), partial)
	results.Total, results.Complete = 3, 2
	c.results["errors handling"] = results
	handler := newSite(t, c)

	page := get(t, handler, "/search?q=errors").Body.String()
	words := get(t, handler, "/search?q=errors+handling").Body.String()

	assertShows(t, page,
		"Search Rules matching “errors”",
		"Libraries example/ rules 1 other/ go-rules 1 Kind Technologies Practices Impact",
		"2 rules in 2 libraries Best match Most starred Newest",
		"Go techs/go 3 Return errors with context HIGH example/rules",
		"techs/golang not canonical 1 Wrap errors MEDIUM other/go-rules",
	)
	if got := headings(t, page, "h2"); !slices.Equal(got, []string{"Go techs/go 3", "techs/golang not canonical 1"}) {
		t.Errorf("the groups are headed %q", got)
	}
	for text, want := range map[string]string{
		"Go techs/go":  "/g/techs/go",
		"techs/golang": "/g/techs/golang",
		"Most starred": "/search?q=errors&sort=stars",
		"Newest":       "/search?q=errors&sort=new",
		"Best match":   "/search?q=errors",
		"Wrap errors":  "/other/go-rules/techs/golang/wrap-errors",
	} {
		if got := links(t, page, text); !slices.Contains(got, want) {
			t.Errorf("%s links %q, want %s", text, got, want)
		}
	}
	_, _, fields := filterFields(t, page)
	if fieldNames(fields)[0] != "q=errors" || !slices.Contains(fieldNames(fields), "kind=practices") || slices.Contains(fieldNames(fields), "retired=1") {
		t.Errorf("the search form's fields are %q, want q, Kind, and no retired rules", fieldNames(fields))
	}
	assertShows(t, words, "3 rules in 2 libraries · 2 match every word",
		"Rules that match some of your words Go techs/go 3 Close response bodies MEDIUM Missing: handling other/go-rules")
	if unvetted := get(t, handler, "/search?q=errors&unvetted=1").Body.String(); !slices.Contains(links(t, unvetted, "Go techs/go"), "/g/techs/go?unvetted=1") {
		t.Errorf("with unvetted libraries, the group leads to %q", links(t, unvetted, "Go techs/go"))
	}
}

// Search without a query lists every rule; one that finds nothing says so, and one whose filters keep nothing leads
// back to the list without them.
func TestSearchPageListsEveryRuleAndSaysWhenNoneMatch(t *testing.T) {
	handler := newSite(t, newBrowsingCatalog())

	all := get(t, handler, "/search").Body.String()
	none := get(t, handler, "/search?q=nothing").Body.String()

	assertShows(t, all, "Search All rules", "4 rules in 2 libraries", "Go techs/go 3 Return errors with context")
	if !strings.Contains(all, "<title>All rules · Search · Rulemart</title>") {
		t.Error("the title doesn't say it lists every rule")
	}
	assertShows(t, none, "Rules matching “nothing”", "No rules match. Try a broader word, or tell us what you were looking for .")
	if got := links(t, none, "tell us what you were looking for"); !slices.Equal(got, []string{"/feedback"}) {
		t.Errorf("the empty search leads to %q", got)
	}
}

// Paging a search keeps its choices.
func TestSearchPagesKeepTheirChoices(t *testing.T) {
	c := newBrowsingCatalog()
	results := c.results["errors"]
	results.Total = 45
	c.results["errors"], c.results["errors page 2"] = results, results

	page := get(t, newSite(t, c), "/search?kind=techs&page=2&q=errors").Body.String()

	if got := links(t, page, "Next"); !slices.Equal(got, []string{"/search?kind=techs&page=3&q=errors"}) {
		t.Errorf("Next leads to %q", got)
	}
	if got := links(t, page, "Previous"); !slices.Equal(got, []string{"/search?kind=techs&q=errors"}) {
		t.Errorf("Previous leads to %q", got)
	}
	if got := links(t, page, "Newest"); !slices.Equal(got, []string{"/search?kind=techs&q=errors&sort=new"}) {
		t.Errorf("a sort tab leads to %q, want the first page", got)
	}
}

// Search offers retired rules as a group's page does while it lists every rule, off by default, and a search for words
// always finds them; either way they follow every current rule, under a heading of their own.
func TestSearchOffersRetiredRulesForEveryRuleAndListsThemLast(t *testing.T) {
	var chosen []domain.ListChoices
	c := newBrowsingCatalog()
	c.chosen = &chosen
	retired := closeBodiesRow
	retired.Retired, retired.GroupRules = true, 1
	results := c.results["errors"]
	results.Rows = append(slices.Clone(results.Rows), retired)
	c.results["errors"] = results
	handler := newSite(t, c)

	all := get(t, handler, "/search").Body.String()
	withRetired := get(t, handler, "/search?retired=1").Body.String()
	words := get(t, handler, "/search?q=errors").Body.String()

	_, _, fields := filterFields(t, all)
	if !slices.Contains(fieldNames(fields), "retired=1") || slices.Contains(checkedFields(fields), "retired=1") {
		t.Errorf("every rule's form holds %q, want retired rules offered, off", fieldNames(fields))
	}
	_, _, fields = filterFields(t, withRetired)
	if !slices.Contains(checkedFields(fields), "retired=1") || len(chosen) < 2 || !chosen[1].Retired {
		t.Errorf("with retired rules, the form holds %q and the catalog read %+v", checkedFields(fields), chosen)
	}
	_, _, fields = filterFields(t, words)
	if slices.Contains(fieldNames(fields), "retired=1") {
		t.Errorf("a search for words offers retired rules: %q", fieldNames(fields))
	}
	if resp := get(t, handler, "/search?q=errors&retired=1"); resp.Code != http.StatusMovedPermanently || resp.Header().Get("Location") != "/search?q=errors" {
		t.Errorf("a search for words with retired rules got %d to %q", resp.Code, resp.Header().Get("Location"))
	}
	if got, want := headings(t, words, "h2"), []string{"Go techs/go 3", "techs/golang not canonical 1", "Retired rules", "Go techs/go 1"}; !slices.Equal(got, want) {
		t.Errorf("the search's headings are %q, want %q", got, want)
	}
	assertShows(t, words, "Retired rules Go techs/go 1 Close response bodies MEDIUM Retired other/go-rules")
}

// Search marks the query's words in each result's title, in any case and as a plural or singular, and nothing in a
// title without them, escaping the rest of the title.
func TestSearchMarksTheQuerysWordsInResultTitles(t *testing.T) {
	c := newBrowsingCatalog()
	retries, ampersand, other := closeBodiesRow, namePackagesRow, wrapErrorsRow
	retries.Rule.Title = "Cap retries per request"
	ampersand.Rule.Title = "Retry & back off"
	other.Rule.Title = "Wrap errors"
	c.results["retry request"] = views.RuleResults{Rows: []views.RuleRow{retries, ampersand, other}, Total: 3, Complete: 3, Libraries: 1,
		Unfiltered: 3, UnfilteredLibraries: []views.LibraryCount{{Library: otherRef, Vetted: true, Rules: 3}}}
	handler := newSite(t, c)

	page := get(t, handler, "/search?q=retry+request").Body.String()
	all := get(t, handler, "/search").Body.String()

	for _, want := range []string{
		`Cap <mark>retries</mark> per <mark>request</mark></h3>`,
		`<mark>Retry</mark> &amp; back off</h3>`,
		`>Wrap errors</h3>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the results don't hold %s", want)
		}
	}
	if strings.Contains(all, "<mark>") {
		t.Error("every rule's list marks words without a query")
	}
}
