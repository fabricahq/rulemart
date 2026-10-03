// The search page, and how it shapes what it reads into what it shows: the rules that match a query, or every rule,
// under their groups, and the addresses of its pages.

package web

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// searchView is what the search page shows.
type searchView struct {
	// query is what the visitor searched for, cleaned; it's empty for every rule.
	query string
	// tooLong marks a query search didn't run, because it holds more than domain.MaxSearchQueryLength characters.
	tooLong bool
	// noWords marks a query with no word to find, such as only "the", which matches nothing.
	noWords bool
	// page numbers the page of results shown, from 1, of pages, which stops at app.MaxSearchPage, the last page a search
	// reads.
	page, pages int
	list        ruleListView
	// groups are the page's rules, under their groups, in the order of each group's first rule: first the current rules
	// that hold every word of the query, then the other current rules, then the retired rules, each among themselves.
	groups []resultGroupView
}

// resultTier is where a rule stands among a search's results, which come in tiers, each grouping its rules apart.
type resultTier int

const (
	// completeTier holds the current rules that hold every word of the query, and every current rule without one.
	completeTier resultTier = iota
	// partialTier holds the current rules that lack some of the query's words.
	partialTier
	// retiredTier holds the retired rules, after every current one.
	retiredTier
)

// tierOf returns the tier of r, a row of a search's results.
func tierOf(r views.RuleRow) resultTier {
	switch {
	case r.Retired:
		return retiredTier
	case len(r.Missing) > 0:
		return partialTier
	}
	return completeTier
}

// resultGroupView is a group's rules on a page of search's results, under a heading that leads to the group's page.
type resultGroupView struct {
	label groupLabel
	icon  groupIcon
	href  string
	// rules counts the group's rules in its tier that pass the filters, on this page and others.
	rules int
	tier  resultTier
	rows  []ruleRowView
}

// newSearchView returns what page shows of results, the rules query finds that choices keep; offersMine offers My
// libraries, and tooLong marks a query search didn't run.
func newSearchView(query domain.SearchQuery, choices domain.ListChoices, offersMine, tooLong bool, results views.RuleResults, page int, iconURL func(file string) string) searchView {
	words := queryWords(query)
	v := searchView{
		query: query.String(), tooLong: tooLong, noWords: results.NoWords, page: page,
		pages: min((results.Total+app.SearchPageSize-1)/app.SearchPageSize, app.MaxSearchPage),
		list:  newRuleListView(domain.SearchListPage, searchHref, searchParams(query.String()), choices, offersMine, results),
	}
	for _, r := range results.Rows {
		tier := tierOf(r)
		if n := len(v.groups); n == 0 || v.groups[n-1].label.id != r.Rule.Group || v.groups[n-1].tier != tier {
			v.groups = append(v.groups, resultGroupView{
				label: newGroupLabel(r.Rule.Group, r.CanonicalGroup), icon: newGroupIcon(r.CanonicalGroup, iconURL),
				href: withUnvetted(groupHref(r.Rule.Group), choices.Unvetted), rules: r.GroupRules, tier: tier,
			})
		}
		row := newListedRuleRow(r)
		row.marks = words
		last := &v.groups[len(v.groups)-1]
		last.rows = append(last.rows, row)
	}
	return v
}

// queryWords returns the words query finds, in lowercase, each once, which each result marks in its title: the words of
// its terms, without the or that joins two.
func queryWords(query domain.SearchQuery) []string {
	find, _ := query.Terms()
	var words []string
	for _, term := range find {
		for _, word := range strings.FieldsFunc(strings.ToLower(term.Text), notWordRune) {
			if word != "or" && !slices.Contains(words, word) {
				words = append(words, word)
			}
		}
	}
	return words
}

// textRun is a stretch of a title, marked when it's one of a search's words.
type textRun struct {
	text   string
	marked bool
}

// markWords splits text into runs, marking each of its words that is one of words, which are in lowercase, or a
// plural or singular of one, without regard to case. Text without such a word is one unmarked run.
func markWords(text string, words []string) []textRun {
	var runs []textRun
	add := func(text string, marked bool) {
		if n := len(runs); n > 0 && !marked && !runs[n-1].marked {
			runs[n-1].text += text
			return
		}
		runs = append(runs, textRun{text, marked})
	}
	for len(text) > 0 {
		start := strings.IndexFunc(text, func(r rune) bool { return !notWordRune(r) })
		if start < 0 {
			add(text, false)
			break
		}
		end := strings.IndexFunc(text[start:], notWordRune)
		if end < 0 {
			end = len(text) - start
		}
		word := text[start : start+end]
		add(text[:start], false)
		add(word, slices.ContainsFunc(words, func(w string) bool { return sameWord(strings.ToLower(word), w) }))
		text = text[start+end:]
	}
	return runs
}

// notWordRune reports whether r is outside a word: neither a letter nor a digit.
func notWordRune(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }

// sameWord reports whether a and b, in lowercase, are one word, or one is the other's plural by s, es, or y to ies.
func sameWord(a, b string) bool {
	plural := func(singular, plural string) bool {
		return plural == singular+"s" || plural == singular+"es" ||
			(strings.HasSuffix(singular, "y") && plural == strings.TrimSuffix(singular, "y")+"ies")
	}
	return a == b || plural(a, b) || plural(b, a)
}

// searchParams are search's own parameters for query: q, unless it's empty.
func searchParams(query string) url.Values {
	if query == "" {
		return nil
	}
	return url.Values{domain.QueryParam: {query}}
}

// title is the search page's document title, which names the query.
func (v searchView) title() string {
	if v.query == "" {
		return "All rules · Search · Rulemart"
	}
	return "“" + v.query + "” · Search · Rulemart"
}

// heading is the search page's heading.
func (v searchView) heading() string {
	if v.query == "" {
		return "All rules"
	}
	return "Rules matching “" + v.query + "”"
}

// groupsIn returns the page's groups of rules in tier, in order.
func (v searchView) groupsIn(tier resultTier) []resultGroupView {
	var groups []resultGroupView
	for _, g := range v.groups {
		if g.tier == tier {
			groups = append(groups, g)
		}
	}
	return groups
}

// empty reports whether the page shows no rules.
func (v searchView) empty() bool { return len(v.groups) == 0 }

// pageMissing reports a page past the last of a search's results.
func (v searchView) pageMissing() bool { return v.page > 1 && v.empty() && !v.tooLong && !v.noWords }

// pageHref is the address of page n of the search's results, with the same choices.
func (v searchView) pageHref(n int) string { return searchHrefFor(v.query, v.list.choices, n) }

// searchHrefFor is the address of page of the search for query, or of every rule when it's empty, with choices,
// leaving out the first page's number.
func searchHrefFor(query string, choices domain.ListChoices, page int) string {
	number := ""
	if page > 1 {
		number = strconv.Itoa(page)
	}
	return searchHrefNumbered(query, choices, number)
}

// searchHrefNumbered is the address of the page of the search for query, or of every rule when it's empty, with
// choices, that number names, spelled as given, or of the first page, without a number, when number is empty.
func searchHrefNumbered(query string, choices domain.ListChoices, number string) string {
	params := searchParams(query)
	if number != "" {
		if params == nil {
			params = url.Values{}
		}
		params.Set("page", number)
	}
	return addressOf(searchHref, choices.Values(domain.SearchListPage), params)
}

// search shows a page of the rules that the query in the q parameter matches, or of every rule without one, the page
// the page parameter numbers, from 1, with the choices the address holds that the visitor can make. Its page names no canonical address and asks
// search engines not to index it, since each query would otherwise be a page of its own. An address that spells its
// choices or page number another way, or names the first page's number, redirects to its own, and a page past the last
// is missing.
func (s *server) search(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	query := domain.ParseSearchQuery(params.Get(domain.QueryParam))
	choices := domain.ParseListChoices(domain.SearchListPage, params)
	page, spelled := searchPageNumber(params)
	own := searchHrefFor(params.Get(domain.QueryParam), choices, page)
	if page > app.MaxSearchPage {
		// A number too large to read keeps its spelling, so the page past the last says so rather than redirecting.
		own = searchHrefNumbered(params.Get(domain.QueryParam), choices, params.Get("page"))
	}
	if !spelled || !spelledAs(r, own) {
		redirect(w, r, own)
		return
	}
	choices, owners, ok := s.myLibraries(w, r, choices)
	if !ok {
		return
	}
	var results views.RuleResults
	var err error
	if page <= app.MaxSearchPage {
		results, err = s.catalog.SearchRules(r.Context(), query, choices, owners, page)
	}
	tooLong := errors.Is(err, app.ErrSearchQueryTooLong)
	if err != nil && !tooLong {
		s.fail(w, r, err)
		return
	}
	view := newSearchView(query, choices, visitorOf(r.Context()).account != nil, tooLong, results, page, s.assets.iconURL)
	status := http.StatusOK
	if view.pageMissing() {
		status = http.StatusNotFound
	}
	s.render(w, r, status, searchPage(s.chrome, view))
}

// searchPageNumber returns the page params number, and whether they spell it as its address does: the first page
// by no number, and any other in digits without a sign or leading zeros. A number that isn't a page's is 1, and a
// number too large to hold is past app.MaxSearchPage.
func searchPageNumber(params url.Values) (page int, spelled bool) {
	if !params.Has("page") {
		return 1, true
	}
	text := params.Get("page")
	page, err := strconv.Atoi(text)
	switch {
	case errors.Is(err, strconv.ErrRange) && text[0] >= '1' && text[0] <= '9':
		return app.MaxSearchPage + 1, strings.Trim(text, "0123456789") == ""
	case err != nil || page < 1:
		return 1, false
	}
	return page, page > 1 && text == strconv.Itoa(page)
}
