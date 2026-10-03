package app_test

import (
	"context"
	"slices"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// sitemapReads stands in for the store's sitemap read, recording the most groups and rules it was asked for.
type sitemapReads struct {
	store.Reader
	sitemap    views.Sitemap
	maxEntries int
}

func (r *sitemapReads) Sitemap(_ context.Context, _ []domain.LibraryKey, maxEntries int) (views.Sitemap, error) {
	r.maxEntries = maxEntries
	return r.sitemap, nil
}

// Every group a vetted library holds has a page of its own, canonical or not, so the sitemap lists each, and reads at
// most app.MaxSitemapGroupsAndRules groups and rules, which leaves room in one sitemap file for every other page.
func TestSitemapListsEveryGroupOfTheVettedLibraries(t *testing.T) {
	groups := []string{"practices/testing", "practices/zz-review", "techs/go", "techs/golang"}
	r := &sitemapReads{sitemap: views.Sitemap{
		Libraries: []views.SitemapLibrary{{Owner: "acme", Name: "backend"}},
		Groups:    slices.Clone(groups),
	}}
	pages := app.Pages{Store: r, Groups: canonicalList(t)}

	got, err := pages.Sitemap(context.Background())

	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Groups, groups) {
		t.Errorf("got groups %q, want %q", got.Groups, groups)
	}
	if len(got.Libraries) != 1 {
		t.Errorf("got libraries %+v", got.Libraries)
	}
	if r.maxEntries != app.MaxSitemapGroupsAndRules || app.MaxSitemapGroupsAndRules+1000 > app.MaxSitemapURLs {
		t.Errorf("read at most %d groups and rules, of %d URLs", r.maxEntries, app.MaxSitemapURLs)
	}
}
