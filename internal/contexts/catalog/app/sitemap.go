// Read what the sitemap lists: the pages search engines may index.

package app

import (
	"context"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// MaxSitemapURLs is how many addresses one sitemap file may list, by the sitemap protocol.
const MaxSitemapURLs = 50_000

// MaxSitemapGroupsAndRules is how many groups, library groups, and rules the sitemap lists at most, which leaves room
// in one file for the site's own pages, every vetted library, and every owner. Pages past it stay out until the
// sitemap is split into several files.
const MaxSitemapGroupsAndRules = 45_000

// Sitemap returns the vetted libraries with their groups and current rules, and the groups that hold them, canonical or
// not, since every group has a page, at most MaxSitemapGroupsAndRules groups, library groups, and rules in all.
func (p Pages) Sitemap(ctx context.Context) (views.Sitemap, error) {
	return p.Store.Sitemap(ctx, p.Vetted, MaxSitemapGroupsAndRules)
}
