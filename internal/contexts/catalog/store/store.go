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
	// Libraries returns the vetted libraries, ordered by owner and name without regard to case.
	Libraries(ctx context.Context, vetted []domain.LibraryKey) ([]views.LibraryCard, error)
	// HomePage returns the vetted libraries, ordered by owner and name without regard to case, and their groups, as
	// Groups returns them.
	HomePage(ctx context.Context, vetted []domain.LibraryKey) ([]views.LibraryCard, []views.LibraryGroup, error)
	// LibraryPage returns the vetted library owner/name, matched without regard to case, or ErrNotFound.
	LibraryPage(ctx context.Context, vetted []domain.LibraryKey, owner, name string) (views.LibraryPage, error)
	// RulePage returns the rule at rulePath in the vetted library owner/name, current or retired, both matched without
	// regard to case, as LibraryPage matches the library, with how every rule of the library was replaced, or
	// ErrNotFound. The page's Rule.Path is the library's spelling.
	RulePage(ctx context.Context, vetted []domain.LibraryKey, owner, name, rulePath string) (views.RulePage, error)
	// RuleComparison returns the rule's page, as RulePage does, with the text of its versions from and to, read only
	// when both are stored and hold at most maxBytes together, or ErrNotFound when either isn't a version of the rule.
	RuleComparison(ctx context.Context, vetted []domain.LibraryKey, owner, name, rulePath string, from, to coderules.RuleVersion, maxBytes int64) (views.RuleComparison, error)
	// LibraryHistory returns the vetted library owner/name, matched as LibraryPage matches it, with its releases and
	// every rule's versions, or ErrNotFound.
	LibraryHistory(ctx context.Context, vetted []domain.LibraryKey, owner, name string) (views.LibraryHistory, error)
	// ReleaseComparison returns the library's history, as LibraryHistory does, and the text of each pair of versions
	// that pick chooses from it, keyed by the pair's key, read from the same snapshot: in pick's order, each pair read
	// only when both are stored and hold, with the pairs before it, at most maxBytes.
	ReleaseComparison(ctx context.Context, vetted []domain.LibraryKey, owner, name string, pick func(views.LibraryHistory) []views.VersionPair, maxBytes int64) (views.LibraryHistory, map[string]views.ComparedText, error)
	// Groups returns each group that holds current rules in a vetted library, once for each library that holds it,
	// in path order and then the library's owner and name, without regard to case.
	Groups(ctx context.Context, vetted []domain.LibraryKey) ([]views.LibraryGroup, error)
	// GroupRules returns the current rules of the group at path in each vetted library that holds it, by library in
	// owner and name order, and each library's in title order. It's empty when no vetted library holds the group.
	GroupRules(ctx context.Context, vetted []domain.LibraryKey, path string) ([]views.GroupLibrary, error)
	// Search returns one page of the vetted libraries' current rules that match query, best first: at most limit of
	// them, which must be at least 1, after the best skip, with how many matched in all. A rule matches when it holds
	// at least one of the query's terms to find and none of those to leave out, by its title, reading guidance,
	// impact description, body, library name, and group's name: the name groups gives a canonical group, and the name
	// part of any group's ID. A term that joins words with -, /, or : also matches the rule's ID, its group's, and its
	// library's owner and name. Rules that hold more of the terms, in more telling places, come first. It leaves each
	// result's CanonicalGroup nil.
	Search(ctx context.Context, vetted []domain.LibraryKey, groups []domain.CanonicalGroup, query domain.SearchQuery, limit, skip int) (views.SearchResults, error)
}

// ErrNotFound reports a library, rule, or rule version that isn't in the catalog, or a library that isn't vetted.
var ErrNotFound = errors.New("not found")
