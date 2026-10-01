// Read what the pages show from the catalog, limited to vetted libraries.

package web

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres/catalogdb"
	"github.com/fabricahq/rulemart/internal/platform/database"
)

// errNotFound reports a library or rule that isn't in the catalog, or isn't vetted.
var errNotFound = errors.New("not found")

// Store reads the catalog. It finds only the libraries in its vetted list.
type Store struct {
	db *database.DB
	// vetted holds the libraries pages may show, each as host:repository ID, as the queries match them.
	vetted []string
}

// NewStore returns a Store that reads through db and finds only the libraries in vetted.
func NewStore(db *database.DB, vetted []domain.LibraryKey) *Store {
	keys := make([]string, len(vetted))
	for i, library := range vetted {
		keys[i] = library.Host + ":" + library.RepositoryID
	}
	return &Store{db: db, vetted: keys}
}

// library is a vetted library, as every page about it describes it.
type library struct {
	// id is the library's catalog id.
	id int64
	// owner and name are spelled as GitHub spells them now.
	owner, name, description string
	// avatar is the owner's avatar URL; empty when GitHub reported none.
	avatar string
	// license and licenseFile are what the library declares; each is empty when it declares none.
	license, licenseFile string
	latestRelease        int
	latestAt             time.Time
}

// contents is what a library's page lists.
type contents struct {
	groups []catalogdb.ListGroupsRow
	rules  []catalogdb.ListCurrentRulesRow
}

// rule is a current rule as its page describes it.
type rule struct {
	catalogdb.GetRuleRow
	versions []catalogdb.ListVersionsRow
}

// readOnlySnapshot is how every page reads: all its queries see one committed state of the catalog, so an
// ingestion that commits midway can't mix two library releases on one page. Read-only also keeps a page from
// writing, while the functions still connect as the database's owner.
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

// libraries returns the vetted libraries, ordered by owner and name.
func (s *Store) libraries(ctx context.Context) ([]catalogdb.ListLibrariesRow, error) {
	var rows []catalogdb.ListLibrariesRow
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		var err error
		rows, err = q.ListLibraries(ctx, s.vetted)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list libraries: %v", err)
	}
	return rows, nil
}

// libraryPage returns the vetted library owner/name, matched without regard to case, with its groups and current
// rules. It fails with errNotFound when there's no such library.
func (s *Store) libraryPage(ctx context.Context, owner, name string) (library, contents, error) {
	var lib library
	var c contents
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		var err error
		if lib, err = s.library(ctx, q, owner, name); err != nil {
			return err
		}
		if c.groups, err = q.ListGroups(ctx, lib.id); err != nil {
			return err
		}
		c.rules, err = q.ListCurrentRules(ctx, lib.id)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return library{}, contents{}, fmt.Errorf("load library %s/%s: %w", owner, name, errNotFound)
	}
	if err != nil {
		return library{}, contents{}, fmt.Errorf("load library %s/%s: %v", owner, name, err)
	}
	return lib, c, nil
}

// rulePage returns the vetted library owner/name, matched as libraryPage matches it, and its current rule ruleID
// with every version, newest first. It fails with errNotFound when there's no such library or current rule.
func (s *Store) rulePage(ctx context.Context, owner, name, ruleID string) (library, rule, error) {
	var lib library
	var r rule
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		var err error
		if lib, err = s.library(ctx, q, owner, name); err != nil {
			return err
		}
		if r.GetRuleRow, err = q.GetRule(ctx, catalogdb.GetRuleParams{LibraryID: lib.id, Path: ruleID}); err != nil {
			return err
		}
		r.versions, err = q.ListVersions(ctx, r.ID)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return library{}, rule{}, fmt.Errorf("load rule %s/%s/%s: %w", owner, name, ruleID, errNotFound)
	}
	if err != nil {
		return library{}, rule{}, fmt.Errorf("load rule %s/%s/%s: %v", owner, name, ruleID, err)
	}
	return lib, r, nil
}

// library returns the vetted library owner/name, or pgx.ErrNoRows when there's none.
func (s *Store) library(ctx context.Context, q *catalogdb.Queries, owner, name string) (library, error) {
	row, err := q.GetLibrary(ctx, catalogdb.GetLibraryParams{Host: domain.GitHub, Owner: owner, Name: name, Vetted: s.vetted})
	if err != nil {
		return library{}, err
	}
	return library{
		id: row.ID, owner: row.Owner, name: row.Name, description: row.Description,
		avatar: row.OwnerAvatarUrl, license: row.LicenseExpression.String, licenseFile: row.LicenseFile.String,
		latestRelease: int(row.LatestRelease), latestAt: row.LatestTaggedAt.Time,
	}, nil
}
