// Read what the sitemap lists: the pages search engines may index.

package app

import (
	"context"
	"slices"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// MaxSitemapURLs is how many addresses one sitemap file may list, by the sitemap protocol.
const MaxSitemapURLs = 50_000

// MaxSitemapRules is how many rules the sitemap lists at most, which leaves room in one file for the site's own pages,
// every vetted library, and every canonical group, whose list holds under a hundred. Rules past it stay out until
// the sitemap is split into several files.
const MaxSitemapRules = 45_000

// Sitemap returns the vetted libraries with their current rules, at most MaxSitemapRules of them, and the canonical
// groups that hold them, whose pages exist. Any other group has no page of its own.
func (p Pages) Sitemap(ctx context.Context) (views.Sitemap, error) {
	sitemap, err := p.Store.Sitemap(ctx, p.Vetted, MaxSitemapRules)
	if err != nil {
		return views.Sitemap{}, err
	}
	sitemap.Groups = slices.DeleteFunc(sitemap.Groups, func(id string) bool { return p.canonical(id) == nil })
	return sitemap, nil
}
