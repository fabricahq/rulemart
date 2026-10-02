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

// sitemapReads stands in for the store's sitemap read, recording the most rules it was asked for.
type sitemapReads struct {
	store.Reader
	sitemap  views.Sitemap
	maxRules int
}

func (r *sitemapReads) Sitemap(_ context.Context, _ []domain.LibraryKey, maxRules int) (views.Sitemap, error) {
	r.maxRules = maxRules
	return r.sitemap, nil
}

// Only a canonical group has a page of its own, so the sitemap lists only canonical groups, and reads at most
// app.MaxSitemapRules rules, which leaves room in one sitemap file for every other page.
func TestSitemapListsOnlyCanonicalGroups(t *testing.T) {
	r := &sitemapReads{sitemap: views.Sitemap{
		Libraries: []views.SitemapLibrary{{Owner: "acme", Name: "backend"}},
		Groups:    []string{"practices/testing", "practices/zz-review", "techs/go", "techs/golang"},
	}}
	pages := app.Pages{Store: r, Groups: canonicalList(t)}

	got, err := pages.Sitemap(context.Background())

	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"practices/testing", "techs/go"}; !slices.Equal(got.Groups, want) {
		t.Errorf("got groups %q, want %q", got.Groups, want)
	}
	if len(got.Libraries) != 1 {
		t.Errorf("got libraries %+v", got.Libraries)
	}
	if r.maxRules != app.MaxSitemapRules || app.MaxSitemapRules+1000 > app.MaxSitemapURLs {
		t.Errorf("read at most %d rules, of %d URLs", r.maxRules, app.MaxSitemapURLs)
	}
}
