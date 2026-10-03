// Read what a visitor's dashboard shows of the catalog: the libraries they and their organizations publish, and the
// rules of the libraries their projects import.

package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres/generated/catalogdb"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// Dashboard returns the libraries owners publish and the libraries names names, as store.Reader describes, from one
// snapshot.
func (s *Store) Dashboard(ctx context.Context, vetted []domain.LibraryKey, owners, names []string) (views.Dashboard, error) {
	var dashboard views.Dashboard
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		var err error
		if dashboard.Owned, err = ownedLibraries(ctx, q, vetted, owners); err != nil {
			return err
		}
		dashboard.Imported, err = importedLibraries(ctx, q, vetted, names)
		return err
	})
	if err != nil {
		return views.Dashboard{}, fmt.Errorf("load dashboard owners=%d libraries=%d: %v", len(owners), len(names), err)
	}
	return dashboard, nil
}

// ownedLibraries returns the libraries owners publish, each with its stars: the sum of its current rules', for a vetted
// library.
func ownedLibraries(ctx context.Context, q *catalogdb.Queries, vetted []domain.LibraryKey, owners []string) ([]views.OwnedLibrary, error) {
	rows, err := q.ListOwnedLibraries(ctx, catalogdb.ListOwnedLibrariesParams{Vetted: vettedKeys(vetted), Owners: lowercase(owners)})
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	var vettedIDs []int64
	for _, row := range rows {
		if row.Vetted {
			vettedIDs = append(vettedIDs, row.ID)
		}
	}
	rules, err := q.ListCurrentRuleIDs(ctx, vettedIDs)
	if err != nil {
		return nil, err
	}
	stars, err := ruleStars(ctx, q, ruleIDs(rules, func(r catalogdb.ListCurrentRuleIDsRow) int64 { return r.ID }))
	if err != nil {
		return nil, err
	}
	totals := map[int64]int{}
	for _, r := range rules {
		totals[r.LibraryID] += stars[r.ID]
	}
	owned := make([]views.OwnedLibrary, len(rows))
	for i, row := range rows {
		owned[i] = views.OwnedLibrary{
			Library: libraryRef(row.Owner, row.Name, row.OwnerAvatarUrl), Vetted: row.Vetted, Rules: int(row.RuleCount),
			Stars: totals[row.ID], AddedAt: row.AddedAt.Time,
		}
	}
	return owned, nil
}

// importedLibraries returns the libraries names names, each with its rules as they stand.
func importedLibraries(ctx context.Context, q *catalogdb.Queries, vetted []domain.LibraryKey, names []string) ([]views.ImportedLibrary, error) {
	if len(names) == 0 {
		return nil, nil
	}
	rows, err := q.ListNamedLibraries(ctx, catalogdb.ListNamedLibrariesParams{Vetted: vettedKeys(vetted), Names: lowercase(names)})
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	ids := make([]int64, len(rows))
	imported := make([]views.ImportedLibrary, len(rows))
	at := map[int64]int{}
	for i, row := range rows {
		ids[i], at[row.ID] = row.ID, i
		imported[i] = views.ImportedLibrary{
			Library: libraryRef(row.Owner, row.Name, row.OwnerAvatarUrl), Vetted: row.Vetted,
			Rules: domain.RuleStates{Current: map[string]coderules.RuleVersion{}, Retired: map[string]bool{}},
		}
	}
	states, err := q.ListRuleStates(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, r := range states {
		rules := imported[at[r.LibraryID]].Rules
		if r.Retired {
			rules.Retired[r.Path] = true
		} else {
			rules.Current[r.Path] = coderules.RuleVersion{Major: int(r.Major), Minor: int(r.Minor), Patch: int(r.Patch)}
		}
	}
	return imported, nil
}

// lowercase returns texts in lowercase, as the queries compare them.
func lowercase(texts []string) []string {
	lower := make([]string, len(texts))
	for i, t := range texts {
		lower[i] = strings.ToLower(t)
	}
	return lower
}
