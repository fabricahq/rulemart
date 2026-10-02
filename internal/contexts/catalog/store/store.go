// Package store is the catalog's persistence contract: what the catalog's operations need from storage.
// store/postgres implements it.
package store

import (
	"context"
	"errors"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// Writer replaces what the catalog stores about a library, and reads back what ingestion needs to decide whether to
// replace it again.
type Writer interface {
	// ReplaceLibrary makes the catalog's rows for lib match it, in one transaction, and returns how many rows
	// changed. Rows that still exist keep their ids, and a row whose values didn't change isn't written, so
	// replacing a library with itself changes nothing.
	ReplaceLibrary(ctx context.Context, lib domain.Library) (changed int64, err error)
	// Checkpoint returns where the library was last fetched from, the tags of its stored releases, and whether any of
	// its stored versions lacks content, or found false when the catalog has no such library.
	Checkpoint(ctx context.Context, library domain.LibraryKey) (checkpoint domain.Checkpoint, found bool, err error)
}

// Reader reads what the catalog's pages show. Each read sees one committed state of the catalog, so a page never
// mixes two ingestions, and finds only the libraries in vetted.
type Reader interface {
	// HomePage returns the vetted libraries, ordered by owner and name without regard to case, and their groups, as
	// Groups returns them.
	HomePage(ctx context.Context, vetted []domain.LibraryKey) ([]views.LibraryCard, []views.LibraryGroup, error)
	// LibraryPage returns the vetted library owner/name, matched without regard to case, or ErrNotFound.
	LibraryPage(ctx context.Context, vetted []domain.LibraryKey, owner, name string) (views.LibraryPage, error)
	// RulePage returns the rule at rulePath in the vetted library owner/name, matched as LibraryPage matches it,
	// current or retired, or ErrNotFound.
	RulePage(ctx context.Context, vetted []domain.LibraryKey, owner, name, rulePath string) (views.RulePage, error)
	// RuleComparison returns the rule's page, as RulePage does, with the text of its versions from and to, read only
	// when both are stored and hold at most maxBytes together, or ErrNotFound when either isn't a version of the rule.
	RuleComparison(ctx context.Context, vetted []domain.LibraryKey, owner, name, rulePath string, from, to coderules.RuleVersion, maxBytes int64) (views.RuleComparison, error)
	// LibraryHistory returns the vetted library owner/name, matched as LibraryPage matches it, with its releases and
	// every rule's versions, or ErrNotFound.
	LibraryHistory(ctx context.Context, vetted []domain.LibraryKey, owner, name string) (views.LibraryHistory, error)
	// ReleaseComparison returns the library's history, as LibraryHistory does, and the text of each rule whose version
	// after release from differs from its version after release to, keyed by the rule's path: in path order, each
	// pair read only when both are stored and hold, with the pairs before it, at most maxBytes.
	ReleaseComparison(ctx context.Context, vetted []domain.LibraryKey, owner, name string, from, to int, maxBytes int64) (views.LibraryHistory, map[string]views.ComparedText, error)
	// Groups returns each group that holds current rules in a vetted library, once for each library that holds it,
	// in path order and then the library's owner and name, without regard to case.
	Groups(ctx context.Context, vetted []domain.LibraryKey) ([]views.LibraryGroup, error)
	// GroupRules returns the current rules of the group at path in each vetted library that holds it, by library in
	// owner and name order, and each library's in title order. It's empty when no vetted library holds the group.
	GroupRules(ctx context.Context, vetted []domain.LibraryKey, path string) ([]views.GroupLibrary, error)
	// Search returns the vetted libraries' current rules that match query, best first, at most limit of them, which
	// must be at least 1, with how many matched in all. A rule matches by its title, reading guidance, impact
	// description, and body, and by its group's name, together: the name groups gives a canonical group, and the name
	// part of any group's ID. It leaves each result's CanonicalGroup nil.
	Search(ctx context.Context, vetted []domain.LibraryKey, groups []domain.CanonicalGroup, query domain.SearchQuery, limit int) (views.SearchResults, error)
}

// ErrNotFound reports a library, rule, or rule version that isn't in the catalog, or a library that isn't vetted.
var ErrNotFound = errors.New("not found")
