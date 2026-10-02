// Read what pages across libraries show: the vetted libraries' groups, one group's rules in each library, and search.

package postgres

import (
	"context"
	"fmt"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres/generated/catalogdb"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// Groups returns each group that holds current rules in a vetted library, once for each library that holds it, in
// path order and then the library's owner and name.
func (s *Store) Groups(ctx context.Context, vetted []domain.LibraryKey) ([]views.LibraryGroup, error) {
	var groups []views.LibraryGroup
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		var err error
		groups, err = libraryGroups(ctx, q, vetted)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("load groups: %v", err)
	}
	return groups, nil
}

// libraryGroups returns each group that holds current rules in a vetted library, as Groups does.
func libraryGroups(ctx context.Context, q *catalogdb.Queries, vetted []domain.LibraryKey) ([]views.LibraryGroup, error) {
	rows, err := q.ListVettedGroups(ctx, vettedKeys(vetted))
	if err != nil {
		return nil, fmt.Errorf("list groups: %v", err)
	}
	groups := make([]views.LibraryGroup, len(rows))
	for i, row := range rows {
		groups[i] = views.LibraryGroup{
			Path: row.Path, Library: libraryRef(row.Owner, row.Name, row.OwnerAvatarUrl), Rules: int(row.RuleCount),
		}
	}
	return groups, nil
}

// GroupRules returns the current rules of the group at path in each vetted library that holds it, by library in owner
// and name order, and each library's in title order.
func (s *Store) GroupRules(ctx context.Context, vetted []domain.LibraryKey, path string) ([]views.GroupLibrary, error) {
	var rows []catalogdb.ListGroupRulesRow
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		var err error
		rows, err = q.ListGroupRules(ctx, catalogdb.ListGroupRulesParams{Path: path, Vetted: vettedKeys(vetted)})
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
	params.FindTerms, params.FindIdentifiers = termParams(find)
	params.ExcludeTerms, params.ExcludeIdentifiers = termParams(exclude)
	for i, g := range groups {
		params.CanonicalIds[i], params.CanonicalNames[i] = g.ID, g.Name
	}
	var rows []catalogdb.SearchRulesRow
	var searchable int64
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		var err error
		if rows, err = q.SearchRules(ctx, params); err != nil || len(rows) > 0 {
			return err
		}
		searchable, err = q.CountSearchableTerms(ctx, params.FindTerms)
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
				Version: version(row.Major, row.Minor, row.Patch),
			},
			WhenToRead: row.WhenToRead,
		}
		for _, ordinal := range row.Missing {
			results.Results[i].Missing = append(results.Results[i].Missing, find[ordinal-1].Text)
		}
	}
	return results, nil
}

// termParams returns terms' queries and identifier flags, in step, as search's parameters.
func termParams(terms []domain.SearchTerm) (queries []string, identifiers []bool) {
	queries, identifiers = make([]string, len(terms)), make([]bool, len(terms))
	for i, t := range terms {
		queries[i], identifiers[i] = t.Query, t.Identifier
	}
	return queries, identifiers
}

func libraryRef(owner, name, avatarURL string) views.LibraryRef {
	return views.LibraryRef{Owner: owner, Name: name, OwnerAvatarURL: avatarURL}
}
