// Read what the pages show from the catalog, limited to vetted libraries.

package site

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fabricahq/rulemart/internal/database"
	"github.com/fabricahq/rulemart/internal/site/sitedb"
)

// errNotFound reports a library or rule that isn't in the catalog, or isn't vetted.
var errNotFound = errors.New("not found")

// Store reads the catalog. It finds only the libraries in its vetted list.
type Store struct {
	db *database.DB
	// vetted holds the GitHub repository IDs of the libraries pages may show.
	vetted []int64
}

// NewStore returns a Store that reads through db and finds only the libraries whose GitHub repository IDs are in
// vetted.
func NewStore(db *database.DB, vetted []int64) *Store {
	return &Store{db: db, vetted: vetted}
}

// library is a vetted library, as every page about it describes it.
type library struct {
	githubID int64
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
	groups []sitedb.ListGroupsRow
	rules  []sitedb.ListCurrentRulesRow
}

// rule is a current rule as its page describes it.
type rule struct {
	sitedb.GetRuleRow
	versions []sitedb.ListVersionsRow
}

// libraries returns the vetted libraries, ordered by owner and name.
func (s *Store) libraries(ctx context.Context) ([]sitedb.ListLibrariesRow, error) {
	var rows []sitedb.ListLibrariesRow
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		var err error
		rows, err = sitedb.New(pool).ListLibraries(ctx, s.vetted)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list libraries: %v", err)
	}
	return rows, nil
}

// library returns the vetted library owner/name, matched without regard to case. It fails with errNotFound when
// there's none.
func (s *Store) library(ctx context.Context, owner, name string) (library, error) {
	var lib library
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		q := sitedb.New(pool)
		row, err := q.GetLibrary(ctx, sitedb.GetLibraryParams{Owner: owner, Name: name, Vetted: s.vetted})
		if err != nil {
			return err
		}
		lib = library{
			githubID: row.GithubID, owner: row.Owner, name: row.Name, description: row.Description,
			avatar: row.OwnerAvatarUrl, license: row.LicenseExpression.String, licenseFile: row.LicenseFile.String,
			latestRelease: int(row.LatestRelease), latestAt: row.LatestTaggedAt.Time,
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return library{}, fmt.Errorf("load library %s/%s: %w", owner, name, errNotFound)
	}
	if err != nil {
		return library{}, fmt.Errorf("load library %s/%s: %v", owner, name, err)
	}
	return lib, nil
}

// contents returns the groups and current rules of the library with GitHub repository ID libraryID.
func (s *Store) contents(ctx context.Context, libraryID int64) (contents, error) {
	var c contents
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		q := sitedb.New(pool)
		var err error
		if c.groups, err = q.ListGroups(ctx, libraryID); err != nil {
			return err
		}
		c.rules, err = q.ListCurrentRules(ctx, libraryID)
		return err
	})
	if err != nil {
		return contents{}, fmt.Errorf("list groups and rules githubID=%d: %v", libraryID, err)
	}
	return c, nil
}

// rule returns the current rule ruleID of the library with GitHub repository ID libraryID, with every version,
// newest first. It fails with errNotFound when the library has no such current rule.
func (s *Store) rule(ctx context.Context, libraryID int64, ruleID string) (rule, error) {
	var r rule
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		q := sitedb.New(pool)
		row, err := q.GetRule(ctx, sitedb.GetRuleParams{LibraryID: libraryID, RuleID: ruleID})
		if err != nil {
			return err
		}
		r.GetRuleRow = row
		r.versions, err = q.ListVersions(ctx, sitedb.ListVersionsParams{LibraryID: libraryID, RuleID: ruleID})
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return rule{}, fmt.Errorf("load rule githubID=%d rule=%q: %w", libraryID, ruleID, errNotFound)
	}
	if err != nil {
		return rule{}, fmt.Errorf("load rule githubID=%d rule=%q: %v", libraryID, ruleID, err)
	}
	return r, nil
}
