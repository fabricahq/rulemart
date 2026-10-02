// Read what pages across libraries show: the vetted libraries' groups, one group's rules in each library, and search.

package postgres

import (
	"context"
	"fmt"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres/generated/catalogdb"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// Groups returns each group that holds current rules in a vetted library, and with unvetted, in a library a listing
// names too, once for each library that holds it, in path order and then the library's owner and name.
func (s *Store) Groups(ctx context.Context, vetted []domain.LibraryKey, unvetted bool) ([]views.LibraryGroup, error) {
	var groups []views.LibraryGroup
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		var err error
		groups, err = libraryGroups(ctx, q, vetted, unvetted)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("load groups unvetted=%t: %v", unvetted, err)
	}
	return groups, nil
}

// libraryGroups returns each group that holds current rules in a library, as Groups does.
func libraryGroups(ctx context.Context, q *catalogdb.Queries, vetted []domain.LibraryKey, unvetted bool) ([]views.LibraryGroup, error) {
	rows, err := q.ListLibraryGroups(ctx, catalogdb.ListLibraryGroupsParams{Vetted: vettedKeys(vetted), IncludeUnvetted: unvetted})
	if err != nil {
		return nil, fmt.Errorf("list groups: %v", err)
	}
	groups := make([]views.LibraryGroup, len(rows))
	for i, row := range rows {
		groups[i] = views.LibraryGroup{
			Path: row.Path, Library: libraryRef(row.Owner, row.Name, row.OwnerAvatarUrl), Vetted: row.Vetted,
			Rules: int(row.RuleCount),
		}
	}
	return groups, nil
}

// GroupRules returns the current rules of the group at path in each vetted library that holds it, by library in owner
// and name order, and each library's in title order.
func (s *Store) GroupRules(ctx context.Context, vetted []domain.LibraryKey, path string) ([]views.GroupLibrary, error) {
	var rows []catalogdb.ListGroupRulesRow
	var stars map[int64]int
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		var err error
		if rows, err = q.ListGroupRules(ctx, catalogdb.ListGroupRulesParams{Path: path, Vetted: vettedKeys(vetted)}); err != nil {
			return err
		}
		stars, err = ruleStars(ctx, q, ruleIDs(rows, func(r catalogdb.ListGroupRulesRow) int64 { return r.ID }))
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list rules of group %s: %v", path, err)
	}
	var libraries []views.GroupLibrary
	for _, row := range rows {
		ref := libraryRef(row.Owner, row.Name, row.OwnerAvatarUrl)
		if len(libraries) == 0 || libraries[len(libraries)-1].Library != ref {
			libraries = append(libraries, views.GroupLibrary{Library: ref})
		}
		last := &libraries[len(libraries)-1]
		last.Rules = append(last.Rules, views.RuleCard{
			Path: row.Path, Group: path, Title: row.Title, Impact: row.Impact, Version: version(row.Major, row.Minor, row.Patch),
			Stars: stars[row.ID],
		})
	}
	return libraries, nil
}

// Search returns one page of the vetted libraries' current rules that match query, best first, as store.Reader's
// Search does, matching groups by the names groups gives them. Its errors never include the query, which comes from a
// visitor.
func (s *Store) Search(ctx context.Context, vetted []domain.LibraryKey, groups []domain.CanonicalGroup, query domain.SearchQuery, limit, skip int) (views.SearchResults, error) {
	if limit < 1 || skip < 0 {
		return views.SearchResults{}, fmt.Errorf("search rules: limit %d is below 1 or skip %d below 0", limit, skip)
	}
	find, exclude := query.Terms()
	if len(find) == 0 {
		return views.SearchResults{NoWords: true}, nil
	}
	params := catalogdb.SearchRulesParams{
		Vetted: vettedKeys(vetted), MaxResults: int32(limit), Skip: int32(skip),
		CanonicalIds: make([]string, len(groups)), CanonicalNames: make([]string, len(groups)),
	}
	params.FindTerms, params.FindIdentifierTerms = termParams(find)
	params.ExcludeTerms, params.ExcludeIdentifierTerms = termParams(exclude)
	for i, g := range groups {
		params.CanonicalIds[i], params.CanonicalNames[i] = g.ID, g.Name
	}
	var rows []catalogdb.SearchRulesRow
	var searchable int64
	var stars map[int64]int
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		var err error
		if rows, err = q.SearchRules(ctx, params); err != nil {
			return err
		}
		if len(rows) == 0 {
			searchable, err = q.CountSearchableTerms(ctx, params.FindTerms)
			return err
		}
		stars, err = ruleStars(ctx, q, ruleIDs(rows, func(r catalogdb.SearchRulesRow) int64 { return r.ID }))
		return err
	})
	if err != nil {
		return views.SearchResults{}, fmt.Errorf("search rules: %v", err)
	}
	results := views.SearchResults{Results: make([]views.SearchResult, len(rows)), NoWords: len(rows) == 0 && searchable == 0}
	for i, row := range rows {
		results.Total, results.Complete = int(row.Total), int(row.Complete)
		results.Results[i] = views.SearchResult{
			Library: libraryRef(row.Owner, row.Name, row.OwnerAvatarUrl),
			Rule: views.RuleCard{
				Path: row.Path, Group: row.GroupPath, Title: row.Title, Impact: row.Impact,
				Version: version(row.Major, row.Minor, row.Patch), Stars: stars[row.ID],
			},
			WhenToRead: row.WhenToRead, WhenToReadHTML: row.WhenToReadHtml,
		}
		for _, ordinal := range row.Missing {
			results.Results[i].Missing = append(results.Results[i].Missing, find[ordinal-1].Text)
		}
	}
	return results, nil
}

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
	var page, unfiltered []catalogdb.ListRulesRow
	var searchable int64
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		var err error
		if params.StarRuleIds, params.StarCounts, err = countedStars(ctx, q, vetted, list.Group); err != nil {
			return err
		}
		if page, err = q.ListRules(ctx, params); err != nil || len(page) > 0 {
			return err
		}
		// Every row says what the list holds before its filters, so without one, read the first rule without them.
		if unfiltered, err = q.ListRules(ctx, firstUnfiltered(params)); err != nil || len(unfiltered) > 0 || params.MatchAll {
			return err
		}
		searchable, err = q.CountSearchableTerms(ctx, params.FindTerms)
		return err
	})
	if err != nil {
		return views.RuleResults{}, fmt.Errorf("list rules group=%q: %v", list.Group, err)
	}
	if len(page) == 0 && len(unfiltered) == 0 {
		return views.RuleResults{NoWords: !params.MatchAll && searchable == 0}, nil
	}
	results := views.RuleResults{Rows: make([]views.RuleRow, len(page))}
	summary := unfiltered
	if len(page) > 0 {
		summary = page
	}
	// Every row says the same of the list before its filters.
	first := summary[0]
	results.Unfiltered = int(first.Unfiltered)
	for i, owner := range first.LibraryOwners {
		results.LibraryCounts = append(results.LibraryCounts, views.LibraryCount{
			Library: libraryRef(owner, first.LibraryNames[i], first.LibraryAvatarUrls[i]),
			Vetted:  first.LibraryVetted[i], Rules: int(first.LibraryRules[i]),
		})
	}
	for i, row := range page {
		results.Total, results.Complete, results.Libraries = int(row.Total), int(row.Complete), int(row.Libraries)
		results.Rows[i] = ruleRow(row, find)
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

// firstUnfiltered returns params for the first rule of their list before its filters, whose row says what the list
// holds.
func firstUnfiltered(params catalogdb.ListRulesParams) catalogdb.ListRulesParams {
	params.Libraries, params.Impact, params.MinStars, params.Kind = []string{}, "", 0, ""
	params.MaxResults, params.Skip = 1, 0
	return params
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

// ruleRow describes a row of ListRules, naming each term of find the rule lacks.
func ruleRow(row catalogdb.ListRulesRow, find []domain.SearchTerm) views.RuleRow {
	r := views.RuleRow{
		Library: libraryRef(row.Owner, row.Name, row.OwnerAvatarUrl), Vetted: row.Vetted,
		Rule: views.RuleCard{
			Path: row.Path, Group: row.GroupPath, Title: row.Title, Impact: row.Impact,
			Version: version(row.Major, row.Minor, row.Patch), Stars: int(row.Stars),
		},
		Retired: row.Retired, GroupRules: int(row.GroupRules),
	}
	if row.ReplacedBy != "" {
		r.ReplacedBy = &views.RuleRef{Path: row.ReplacedBy, Title: row.ReplacementTitle}
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

func libraryRef(owner, name, avatarURL string) views.LibraryRef {
	return views.LibraryRef{Owner: owner, Name: name, OwnerAvatarURL: avatarURL}
}
