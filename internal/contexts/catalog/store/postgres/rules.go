// Read lists of rules across libraries: a group's rules, every rule, or a search's matches.

package postgres

import (
	"context"
	"fmt"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres/generated/catalogdb"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// Rules returns one page of the list of rules list describes, as store.Reader's Rules does, matching groups by the
// names groups gives them. Its errors never include the list's query, which comes from a visitor.
func (s *Store) Rules(ctx context.Context, vetted []domain.LibraryKey, groups []domain.CanonicalGroup, list domain.RuleList, limit, skip int) (views.RuleResults, error) {
	if limit < 1 || skip < 0 {
		return views.RuleResults{}, fmt.Errorf("list rules: limit %d is below 1 or skip %d below 0", limit, skip)
	}
	find, exclude := list.Query.Terms()
	if !list.Query.IsZero() && len(find) == 0 {
		return views.RuleResults{NoWords: true}, nil
	}
	params := ruleListParams(vetted, groups, list, find, exclude, limit, skip)
	var rows []catalogdb.ListRulesRow
	var links map[int64][]views.RuleLink
	var searchable int64
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		var err error
		if params.StarRuleIds, params.StarCounts, err = countedStars(ctx, q, vetted, list.Group); err != nil {
			return err
		}
		if rows, err = q.ListRules(ctx, params); err != nil {
			return err
		}
		if links, err = retiredRuleLinks(ctx, q, rows); err != nil || rows[0].Unfiltered > 0 || params.MatchAll {
			return err
		}
		searchable, err = q.CountSearchableTerms(ctx, params.FindTerms)
		return err
	})
	if err != nil {
		return views.RuleResults{}, fmt.Errorf("list rules group=%q: %v", list.Group, err)
	}
	// Every row summarizes the list the same way, and a page without rules is one row of only the summary.
	summary := rows[0]
	results := views.RuleResults{
		Total: int(summary.Total), Complete: int(summary.Complete), Libraries: int(summary.Libraries),
		Unfiltered: int(summary.Unfiltered), RetiredRules: int(summary.RetiredRules),
		NoWords: summary.Unfiltered == 0 && !params.MatchAll && searchable == 0,
	}
	for i, owner := range summary.LibraryOwners {
		results.UnfilteredLibraries = append(results.UnfilteredLibraries, views.LibraryCount{
			Library: libraryRef(owner, summary.LibraryNames[i], summary.LibraryAvatarUrls[i]),
			Vetted:  summary.LibraryVetted[i], Rules: int(summary.LibraryRules[i]),
		})
	}
	for _, row := range rows {
		if !row.ID.Valid {
			continue
		}
		r := ruleRow(row, find)
		if r.Retired {
			r.Links = links[row.LibraryID.Int64]
		}
		results.Rows = append(results.Rows, r)
	}
	return results, nil
}

// ruleListParams returns the parameters of ListRules for one page of the list list describes, of at most limit rules
// after the first skip, with find and exclude its query's terms, and without stars, which countedStars counts.
func ruleListParams(vetted []domain.LibraryKey, groups []domain.CanonicalGroup, list domain.RuleList, find, exclude []domain.SearchTerm, limit, skip int) catalogdb.ListRulesParams {
	params := catalogdb.ListRulesParams{
		Vetted: vettedKeys(vetted), IncludeUnvetted: list.Unvetted, IncludeRetired: list.HoldsRetired(),
		GroupPath: list.Group, MatchAll: list.Query.IsZero(),
		CanonicalIds: make([]string, len(groups)), CanonicalNames: make([]string, len(groups)),
		Libraries: append([]string{}, list.Filters.Libraries...), Impact: string(list.Filters.Impact),
		MinStars: int32(list.Filters.MinStars), Kind: list.Filters.Kind, OrderBy: string(list.Order),
		FirstOwner: domain.FabricaOwner, MaxResults: int32(limit), Skip: int32(skip),
	}
	params.FindTerms, params.FindIdentifierTerms = termParams(find)
	params.ExcludeTerms, params.ExcludeIdentifierTerms = termParams(exclude)
	for i, g := range groups {
		params.CanonicalIds[i], params.CanonicalNames[i] = g.ID, g.Name
	}
	return params
}

// retiredRuleLinks returns how every rule of each library of page's retired rules was replaced, by library ID, which
// app.Pages follows to name each one's replacement.
func retiredRuleLinks(ctx context.Context, q *catalogdb.Queries, page []catalogdb.ListRulesRow) (map[int64][]views.RuleLink, error) {
	links := map[int64][]views.RuleLink{}
	for _, row := range page {
		id := row.LibraryID.Int64
		if _, read := links[id]; read || !row.Retired.Bool {
			continue
		}
		libraryLinks, err := ruleLinks(ctx, q, id)
		if err != nil {
			return nil, fmt.Errorf("read the rule links of library id=%d: %v", id, err)
		}
		links[id] = libraryLinks
	}
	return links, nil
}

// countedStars returns the stars of the current rules of the vetted libraries in the group at path, or in every group
// when it's empty, as ListRules takes them: rule IDs and their counts in step, of the rules with stars.
func countedStars(ctx context.Context, q *catalogdb.Queries, vetted []domain.LibraryKey, path string) ([]int64, []int32, error) {
	ids, err := q.ListCountedRuleIDs(ctx, catalogdb.ListCountedRuleIDsParams{Vetted: vettedKeys(vetted), GroupPath: path})
	if err != nil {
		return nil, nil, fmt.Errorf("list rules to count stars of: %v", err)
	}
	stars, err := ruleStars(ctx, q, ids)
	if err != nil {
		return nil, nil, err
	}
	ruleIDs, counts := make([]int64, 0, len(stars)), make([]int32, 0, len(stars))
	for id, n := range stars {
		ruleIDs, counts = append(ruleIDs, id), append(counts, int32(n))
	}
	return ruleIDs, counts, nil
}

// ruleRow describes a row of ListRules that holds a rule, naming each term of find the rule lacks.
func ruleRow(row catalogdb.ListRulesRow, find []domain.SearchTerm) views.RuleRow {
	r := views.RuleRow{
		Library: libraryRef(row.Owner.String, row.Name.String, row.OwnerAvatarUrl.String), Vetted: row.Vetted.Bool,
		Rule: views.RuleCard{
			Path: row.Path.String, Group: row.GroupPath.String, Title: row.Title, Impact: row.Impact,
			Version: version(row.Major.Int32, row.Minor.Int32, row.Patch.Int32), Stars: int(row.Stars),
		},
		Retired: row.Retired.Bool, GroupRules: int(row.GroupRules.Int64),
	}
	for _, ordinal := range row.Missing {
		r.Missing = append(r.Missing, find[ordinal-1].Text)
	}
	return r
}

// termParams returns terms' queries and identifier queries, in step, as search's parameters.
func termParams(terms []domain.SearchTerm) (queries, identifierQueries []string) {
	queries, identifierQueries = make([]string, len(terms)), make([]string, len(terms))
	for i, t := range terms {
		queries[i], identifierQueries[i] = t.Query, t.IdentifierQuery
	}
	return queries, identifierQueries
}
