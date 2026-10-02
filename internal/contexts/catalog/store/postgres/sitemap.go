// Read what the sitemap lists.

package postgres

import (
	"context"
	"fmt"
	"slices"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres/generated/catalogdb"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// Sitemap returns the vetted libraries, ordered by owner and name, each with its current rules in ID order, at most
// maxRules in all, and the IDs of the groups that hold current rules in them, from one state of the catalog.
func (s *Store) Sitemap(ctx context.Context, vetted []domain.LibraryKey, maxRules int) (views.Sitemap, error) {
	var sitemap views.Sitemap
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		keys := vettedKeys(vetted)
		libraries, err := q.ListSitemapLibraries(ctx, keys)
		if err != nil {
			return fmt.Errorf("list libraries: %v", err)
		}
		// One more than maxRules tells whether any were left out.
		rules, err := q.ListSitemapRules(ctx, catalogdb.ListSitemapRulesParams{Vetted: keys, MaxRules: int32(maxRules) + 1})
		if err != nil {
			return fmt.Errorf("list rules: %v", err)
		}
		if len(rules) > maxRules {
			rules, sitemap.Truncated = rules[:maxRules], true
		}
		byLibrary := map[int64][]views.SitemapRule{}
		for _, r := range rules {
			byLibrary[r.LibraryID] = append(byLibrary[r.LibraryID], views.SitemapRule{Path: r.Path, Updated: r.TaggedAt.Time.UTC()})
		}
		for _, lib := range libraries {
			sitemap.Libraries = append(sitemap.Libraries, views.SitemapLibrary{
				Owner: lib.Owner, Name: lib.Name, Updated: lib.TaggedAt.Time.UTC(), Rules: byLibrary[lib.ID],
			})
		}
		groups, err := libraryGroups(ctx, q, vetted)
		if err != nil {
			return err
		}
		for _, g := range groups {
			sitemap.Groups = append(sitemap.Groups, g.Path)
		}
		slices.Sort(sitemap.Groups)
		sitemap.Groups = slices.Compact(sitemap.Groups)
		return nil
	})
	if err != nil {
		return views.Sitemap{}, fmt.Errorf("load sitemap: %v", err)
	}
	return sitemap, nil
}
