// Read what checkout resolves a browser's cart against, for the web function.

package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres/generated/catalogdb"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

var _ store.Carts = (*Store)(nil)

// CartLibraries returns the libraries items name, with the rules of the groups they name, from one snapshot, as
// store.Carts describes.
func (s *Store) CartLibraries(ctx context.Context, vetted []domain.LibraryKey, items []domain.CartItem) ([]views.CartLibrary, error) {
	var libraries []views.CartLibrary
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		found, err := q.FindCartLibraries(ctx, catalogdb.FindCartLibrariesParams{
			Vetted: vettedKeys(vetted), Host: domain.GitHub, FullNames: cartFullNames(items),
		})
		if err != nil {
			return fmt.Errorf("find libraries: %v", err)
		}
		libraries = make([]views.CartLibrary, len(found))
		ids := make([]int64, len(found))
		byID := map[int64]*views.CartLibrary{}
		var groups []string
		for i, row := range found {
			libraries[i] = views.CartLibrary{
				Library: views.LibraryRef{Owner: row.Owner, Name: row.Name, OwnerAvatarURL: row.OwnerAvatarUrl},
				Vetted:  row.Vetted, LatestRelease: int(row.LatestRelease), LatestCommit: row.LatestCommit,
			}
			ids[i], byID[row.ID] = row.ID, &libraries[i]
			groups = append(groups, cartGroups(row.ID, row.Owner+"/"+row.Name, items)...)
		}
		// One character more than a cart shows tells a title that's cut from one that fits.
		rules, err := q.ListCartRules(ctx, catalogdb.ListCartRulesParams{LibraryIds: ids, Groups: groups, TitleRunes: views.MaxCartTitleRunes + 1})
		if err != nil {
			return fmt.Errorf("list rules: %v", err)
		}
		for _, row := range rules {
			lib := byID[row.LibraryID]
			lib.Rules = append(lib.Rules, views.CartRule{
				Path: row.Path, Group: row.GroupPath, Title: cartTitle(row.Title), Version: version(row.Major, row.Minor, row.Patch),
				RetiredIn: int(row.RetiredIn),
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load cart items=%d: %v", len(items), err)
	}
	return libraries, nil
}

// cartTitle returns title as a cart shows it: cut to views.MaxCartTitleRunes characters, with an ellipsis, when it's
// longer.
func cartTitle(title string) string {
	runes := []rune(title)
	if len(runes) <= views.MaxCartTitleRunes {
		return title
	}
	return strings.TrimRight(string(runes[:views.MaxCartTitleRunes-1]), " ") + "…"
}

// cartFullNames returns the libraries items name, as FindCartLibraries matches them: owner/name in lowercase.
func cartFullNames(items []domain.CartItem) []string {
	names := make([]string, len(items))
	for i, item := range items {
		names[i] = strings.ToLower(item.FullName())
	}
	return names
}

// cartGroups returns the groups items name of the library fullName, whose ID is id, as ListCartRules matches them:
// the ID, a slash, and the group's path in lowercase.
func cartGroups(id int64, fullName string, items []domain.CartItem) []string {
	var groups []string
	for _, item := range items {
		if strings.EqualFold(item.FullName(), fullName) {
			groups = append(groups, strconv.FormatInt(id, 10)+"/"+strings.ToLower(item.Group()))
		}
	}
	return groups
}
