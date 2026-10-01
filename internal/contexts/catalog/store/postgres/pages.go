// Read what the pages show from the catalog, each page from one snapshot, limited to vetted libraries.

package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres/generated/catalogdb"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// readOnlySnapshot is how every page reads: all its queries see one committed state of the catalog, so an
// ingestion that commits midway can't mix two library releases on one page. Read-only also keeps a page from
// writing, whatever role it connects as.
var readOnlySnapshot = pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}

// read runs fn with queries that all see one snapshot of the catalog, in a read-only transaction. fn may run again
// after a failed connection.
func (s *Store) read(ctx context.Context, fn func(*catalogdb.Queries) error) error {
	return s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		return pgx.BeginTxFunc(ctx, pool, readOnlySnapshot, func(tx pgx.Tx) error {
			return fn(catalogdb.New(tx))
		})
	})
}

// vettedKeys returns the vetted libraries as the queries match them: each as host:repository ID.
func vettedKeys(vetted []domain.LibraryKey) []string {
	keys := make([]string, len(vetted))
	for i, library := range vetted {
		keys[i] = library.Host + ":" + library.RepositoryID
	}
	return keys
}

// Libraries returns the vetted libraries, ordered by owner and name.
func (s *Store) Libraries(ctx context.Context, vetted []domain.LibraryKey) ([]views.LibraryCard, error) {
	var rows []catalogdb.ListLibrariesRow
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		var err error
		rows, err = q.ListLibraries(ctx, vettedKeys(vetted))
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list libraries: %v", err)
	}
	cards := make([]views.LibraryCard, len(rows))
	for i, row := range rows {
		cards[i] = views.LibraryCard{
			Owner: row.Owner, Name: row.Name, Description: row.Description, OwnerAvatarURL: row.OwnerAvatarUrl,
			Rules: int(row.RuleCount),
		}
	}
	return cards, nil
}

// LibraryPage returns the vetted library owner/name, matched without regard to case, with its groups and current
// rules. It fails with store.ErrNotFound when there's no such library.
func (s *Store) LibraryPage(ctx context.Context, vetted []domain.LibraryKey, owner, name string) (views.LibraryPage, error) {
	var page views.LibraryPage
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		lib, id, err := library(ctx, q, vetted, owner, name)
		if err != nil {
			return err
		}
		groups, err := q.ListGroups(ctx, id)
		if err != nil {
			return err
		}
		rules, err := q.ListCurrentRules(ctx, id)
		if err != nil {
			return err
		}
		page = views.LibraryPage{Library: lib}
		for _, g := range groups {
			page.Groups = append(page.Groups, views.Group{
				Path: g.Path, Description: g.Description, WhenToRead: g.WhenToRead, Rules: int(g.RuleCount),
			})
		}
		for _, r := range rules {
			page.Rules = append(page.Rules, views.RuleCard{
				Path: r.Path, Group: r.GroupPath, Title: r.Title, Impact: r.Impact, Version: version(r.Major, r.Minor, r.Patch),
			})
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return views.LibraryPage{}, fmt.Errorf("load library %s/%s: %w", owner, name, store.ErrNotFound)
	}
	if err != nil {
		return views.LibraryPage{}, fmt.Errorf("load library %s/%s: %v", owner, name, err)
	}
	return page, nil
}

// RulePage returns the vetted library owner/name, matched as LibraryPage matches it, and its current rule at
// rulePath with every version, newest first. It fails with store.ErrNotFound when there's no such library or
// current rule.
func (s *Store) RulePage(ctx context.Context, vetted []domain.LibraryKey, owner, name, rulePath string) (views.RulePage, error) {
	var page views.RulePage
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		lib, id, err := library(ctx, q, vetted, owner, name)
		if err != nil {
			return err
		}
		r, err := q.GetRule(ctx, catalogdb.GetRuleParams{LibraryID: id, Path: rulePath})
		if err != nil {
			return err
		}
		versions, err := q.ListVersions(ctx, r.ID)
		if err != nil {
			return err
		}
		page = views.RulePage{Library: lib, Rule: views.Rule{
			Path: r.Path, Group: r.GroupPath, Title: r.Title, Impact: r.Impact,
			WhenToRead: r.WhenToRead, HTML: r.Html, Version: version(r.Major, r.Minor, r.Patch),
			Release: int(r.Release), PublishedAt: r.PublishedAt.Time,
		}}
		for _, v := range versions {
			page.Versions = append(page.Versions, views.Version{
				Version: version(v.Major, v.Minor, v.Patch), Release: int(v.Release), PublishedAt: v.PublishedAt.Time,
				Change: coderules.Change(v.Change), Summaries: v.Summaries,
			})
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return views.RulePage{}, fmt.Errorf("load rule %s/%s/%s: %w", owner, name, rulePath, store.ErrNotFound)
	}
	if err != nil {
		return views.RulePage{}, fmt.Errorf("load rule %s/%s/%s: %v", owner, name, rulePath, err)
	}
	return page, nil
}

// library returns the vetted library owner/name and its catalog id, or pgx.ErrNoRows when there's none.
func library(ctx context.Context, q *catalogdb.Queries, vetted []domain.LibraryKey, owner, name string) (views.Library, int64, error) {
	row, err := q.GetLibrary(ctx, catalogdb.GetLibraryParams{Host: domain.GitHub, Owner: owner, Name: name, Vetted: vettedKeys(vetted)})
	if err != nil {
		return views.Library{}, 0, err
	}
	return views.Library{
		Owner: row.Owner, Name: row.Name, Description: row.Description, OwnerAvatarURL: row.OwnerAvatarUrl,
		LicenseExpression: row.LicenseExpression.String, LicenseFile: row.LicenseFile.String,
		LatestRelease: int(row.LatestRelease), LatestTaggedAt: row.LatestTaggedAt.Time,
	}, row.ID, nil
}

func version(major, minor, patch int32) coderules.RuleVersion {
	return coderules.RuleVersion{Major: int(major), Minor: int(minor), Patch: int(patch)}
}
