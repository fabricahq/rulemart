package domain_test

import (
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
)

// An address's query holds each choice in its own parameter; ParseListChoices reads only what its page offers, and
// Values writes the choices back leaving out every default.
func TestListChoicesReadAndWriteTheirAddress(t *testing.T) {
	for _, tc := range []struct {
		name  string
		page  domain.ListPage
		query string
		want  domain.ListChoices
		// address is what Values writes.
		address string
	}{
		{"a group's defaults", domain.GroupListPage, "", domain.ListChoices{Order: domain.MostStarred}, ""},
		{"search's defaults", domain.SearchListPage, "", domain.ListChoices{Order: domain.BestMatch}, ""},
		{"a library list's defaults", domain.LibraryListPage, "", domain.ListChoices{}, ""},
		{"unvetted libraries", domain.LibraryListPage, "unvetted=1", domain.ListChoices{Unvetted: true}, "unvetted=1"},
		{"a library list leaves out what it doesn't offer", domain.LibraryListPage, "impact=high&sort=new&unvetted=1",
			domain.ListChoices{Unvetted: true}, "unvetted=1"},
		{"a library", domain.GroupListPage, "libs=zeta%2Fgo", domain.ListChoices{
			Filters: domain.RuleFilters{Libraries: []string{"zeta/go"}}, Order: domain.MostStarred}, "libs=zeta%2Fgo"},
		{"libraries joined, in order, in lowercase, each once", domain.GroupListPage, "libs=Zeta/Go,fabricahq/rules,zeta/go",
			domain.ListChoices{Filters: domain.RuleFilters{Libraries: []string{"zeta/go", "fabricahq/rules"}}, Order: domain.MostStarred},
			"libs=zeta%2Fgo%2Cfabricahq%2Frules"},
		{"libraries repeated, as a form sends them", domain.GroupListPage, "libs=zeta/go&libs=fabricahq/rules",
			domain.ListChoices{Filters: domain.RuleFilters{Libraries: []string{"zeta/go", "fabricahq/rules"}}, Order: domain.MostStarred},
			"libs=zeta%2Fgo%2Cfabricahq%2Frules"},
		{"names that can't be libraries", domain.GroupListPage, "libs=zeta,a/b/c,/go,zeta/,a%20b/c,zeta/go",
			domain.ListChoices{Filters: domain.RuleFilters{Libraries: []string{"zeta/go"}}, Order: domain.MostStarred},
			"libs=zeta%2Fgo"},
		{"critical and high", domain.GroupListPage, "impact=high", domain.ListChoices{
			Filters: domain.RuleFilters{Impact: domain.HighImpact}, Order: domain.MostStarred}, "impact=high"},
		{"medium and lower", domain.SearchListPage, "impact=medium", domain.ListChoices{
			Filters: domain.RuleFilters{Impact: domain.LowerImpact}, Order: domain.BestMatch}, "impact=medium"},
		{"both impacts keep every rule", domain.GroupListPage, "impact=high&impact=medium", domain.ListChoices{Order: domain.MostStarred}, ""},
		{"an impact it doesn't offer", domain.GroupListPage, "impact=low", domain.ListChoices{Order: domain.MostStarred}, ""},
		{"stars", domain.GroupListPage, "stars=50", domain.ListChoices{Filters: domain.RuleFilters{MinStars: 50}, Order: domain.MostStarred}, "stars=50"},
		{"any stars", domain.GroupListPage, "stars=0", domain.ListChoices{Order: domain.MostStarred}, ""},
		{"stars it has no threshold for", domain.GroupListPage, "stars=7&stars=-1&stars=1e2",
			domain.ListChoices{Order: domain.MostStarred}, ""},
		{"a kind", domain.SearchListPage, "kind=practices", domain.ListChoices{
			Filters: domain.RuleFilters{Kind: "practices"}, Order: domain.BestMatch}, "kind=practices"},
		{"both kinds keep every rule", domain.SearchListPage, "kind=techs&kind=practices", domain.ListChoices{Order: domain.BestMatch}, ""},
		{"a group's page has no kind", domain.GroupListPage, "kind=techs", domain.ListChoices{Order: domain.MostStarred}, ""},
		{"my libraries", domain.GroupListPage, "mine=1", domain.ListChoices{
			Filters: domain.RuleFilters{Mine: true}, Order: domain.MostStarred}, "mine=1"},
		{"my libraries in search", domain.SearchListPage, "mine=1", domain.ListChoices{
			Filters: domain.RuleFilters{Mine: true}, Order: domain.BestMatch}, "mine=1"},
		{"my libraries other than 1", domain.GroupListPage, "mine=yes", domain.ListChoices{Order: domain.MostStarred}, ""},
		{"a library list has no my libraries", domain.LibraryListPage, "mine=1", domain.ListChoices{}, ""},
		{"newest", domain.GroupListPage, "sort=new", domain.ListChoices{Order: domain.Newest}, "sort=new"},
		{"most starred, a group's default", domain.GroupListPage, "sort=stars", domain.ListChoices{Order: domain.MostStarred}, ""},
		{"most starred in search", domain.SearchListPage, "sort=stars", domain.ListChoices{Order: domain.MostStarred}, "sort=stars"},
		{"a group's page has no best match", domain.GroupListPage, "sort=best", domain.ListChoices{Order: domain.MostStarred}, ""},
		{"an empty sort", domain.SearchListPage, "sort=", domain.ListChoices{Order: domain.BestMatch}, ""},
		{"retired rules", domain.GroupListPage, "retired=1", domain.ListChoices{Retired: true, Order: domain.MostStarred}, "retired=1"},
		{"retired rules on search for every rule", domain.SearchListPage, "retired=1", domain.ListChoices{Retired: true, Order: domain.BestMatch}, "retired=1"},
		{"search for words always finds retired rules", domain.SearchListPage, "q=errors&retired=1", domain.ListChoices{Order: domain.BestMatch}, ""},
		{"every choice, in any order", domain.SearchListPage, "unvetted=1&sort=new&kind=techs&stars=100&impact=high&libs=zeta/go&mine=1",
			domain.ListChoices{Unvetted: true, Filters: domain.RuleFilters{
				Libraries: []string{"zeta/go"}, Mine: true, Impact: domain.HighImpact, MinStars: 100, Kind: "techs",
			}, Order: domain.Newest},
			"impact=high&kind=techs&libs=zeta%2Fgo&mine=1&sort=new&stars=100&unvetted=1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values, err := url.ParseQuery(tc.query)
			if err != nil {
				t.Fatal(err)
			}

			got := domain.ParseListChoices(tc.page, values)

			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
			if written := got.Values(tc.page).Encode(); written != tc.address {
				t.Errorf("wrote %q, want %q", written, tc.address)
			}
			if again := domain.ParseListChoices(tc.page, got.Values(tc.page)); !reflect.DeepEqual(again, got) {
				t.Errorf("read back %+v, want %+v", again, got)
			}
		})
	}
}

// An address filters by at most MaxLibraryFilters libraries.
func TestListChoicesFilterByAtMostMaxLibraryFilters(t *testing.T) {
	var libraries []string
	for i := range domain.MaxLibraryFilters + 1 {
		libraries = append(libraries, fmt.Sprintf("owner/library-%d", i))
	}

	got := domain.ParseListChoices(domain.GroupListPage, url.Values{"libs": {strings.Join(libraries, ",")}})

	if n := len(got.Filters.Libraries); n != domain.MaxLibraryFilters || got.Filters.Libraries[n-1] != libraries[n-1] {
		t.Errorf("got %d libraries, ending %q; want the first %d", n, got.Filters.Libraries[n-1], domain.MaxLibraryFilters)
	}
}

// The defaults' address names nothing, so a page's own address is its path alone.
func TestListChoicesWriteNothingForTheirDefaults(t *testing.T) {
	for _, page := range []domain.ListPage{domain.GroupListPage, domain.SearchListPage, domain.LibraryListPage} {
		if got := domain.ParseListChoices(page, url.Values{}).Values(page); len(got) != 0 {
			t.Errorf("page %d: wrote %v, want nothing", page, got)
		}
	}
}
