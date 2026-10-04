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

// Sitemap returns the vetted libraries, ordered by owner and name, each with the groups that hold its current rules and
// those rules, both in ID order, and the IDs of the groups that hold current rules in them, in ID order, at most
// maxEntries groups, library groups, and rules in all, in that order of preference, from one state of the catalog.
func (s *Store) Sitemap(ctx context.Context, vetted []domain.LibraryKey, maxEntries int) (views.Sitemap, error) {
	var sitemap views.Sitemap
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		groups, err := libraryGroups(ctx, q, vetted, false)
		if err != nil {
			return err
		}
		for _, g := range groups {
			sitemap.Groups = append(sitemap.Groups, g.Path)
		}
		slices.Sort(sitemap.Groups)
		sitemap.Groups = slices.Compact(sitemap.Groups)
		if len(sitemap.Groups) > maxEntries {
			sitemap.Groups, sitemap.Truncated = sitemap.Groups[:maxEntries], true
		}
		keys := vettedKeys(vetted)
		libraries, err := q.ListSitemapLibraries(ctx, keys)
		if err != nil {
			return fmt.Errorf("list libraries: %v", err)
		}
		maxGroups := maxEntries - len(sitemap.Groups)
		// One more than maxGroups tells whether any were left out.
		libraryGroups, err := q.ListSitemapLibraryGroups(ctx, catalogdb.ListSitemapLibraryGroupsParams{
			Vetted: keys, MaxGroups: int32(maxGroups) + 1,
		})
		if err != nil {
			return fmt.Errorf("list library groups: %v", err)
		}
		if len(libraryGroups) > maxGroups {
			libraryGroups, sitemap.Truncated = libraryGroups[:maxGroups], true
		}
		groupsByLibrary := map[int64][]views.SitemapGroup{}
		for _, g := range libraryGroups {
			groupsByLibrary[g.LibraryID] = append(groupsByLibrary[g.LibraryID], views.SitemapGroup{
				Path: g.Path, Updated: g.TaggedAt.Time.UTC(),
			})
		}
		maxRules := maxGroups - len(libraryGroups)
		// One more than maxRules tells whether any were left out.
		rules, err := q.ListSitemapRules(ctx, catalogdb.ListSitemapRulesParams{Vetted: keys, MaxRules: int32(maxRules) + 1})
		if err != nil {
			return fmt.Errorf("list rules: %v", err)
		}
		if len(rules) > maxRules {
			rules, sitemap.Truncated = rules[:maxRules], true
		}
		rulesByLibrary := map[int64][]views.SitemapRule{}
		for _, r := range rules {
			rulesByLibrary[r.LibraryID] = append(rulesByLibrary[r.LibraryID], views.SitemapRule{
				Path: r.Path, Updated: r.TaggedAt.Time.UTC(),
			})
		}
		for _, lib := range libraries {
			sitemap.Libraries = append(sitemap.Libraries, views.SitemapLibrary{
				Owner: lib.Owner, Name: lib.Name, Updated: lib.TaggedAt.Time.UTC(),
				Groups: groupsByLibrary[lib.ID], Rules: rulesByLibrary[lib.ID],
			})
		}
		return nil
	})
	if err != nil {
		return views.Sitemap{}, fmt.Errorf("load sitemap: %v", err)
	}
	return sitemap, nil
}
