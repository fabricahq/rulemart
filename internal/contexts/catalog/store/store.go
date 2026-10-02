// Package store is the catalog's persistence contract: what the catalog's operations need from storage.
// store/postgres implements it.
package store

import (
	"context"
	"errors"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// Writer replaces what the catalog stores about a library, and reads back what ingestion needs to decide whether to
// replace it again.
type Writer interface {
	// ReplaceLibrary makes the catalog's rows for lib match it, in one transaction, and returns how many rows
	// changed. Rows that still exist keep their ids, and a row whose values didn't change isn't written, so
	// replacing a library with itself changes nothing.
	ReplaceLibrary(ctx context.Context, lib domain.Library) (changed int64, err error)
	// Checkpoint returns where the library was last fetched from and the tags of its stored releases, or found false
	// when the catalog has no such library.
	Checkpoint(ctx context.Context, library domain.LibraryKey) (checkpoint domain.Checkpoint, found bool, err error)
}

// Reader reads what the catalog's pages show. Each read sees one committed state of the catalog, so a page never
// mixes two ingestions, and finds only the libraries in vetted.
type Reader interface {
	// Libraries returns the vetted libraries, ordered by owner and name without regard to case.
	Libraries(ctx context.Context, vetted []domain.LibraryKey) ([]views.LibraryCard, error)
	// LibraryPage returns the vetted library owner/name, matched without regard to case, or ErrNotFound.
	LibraryPage(ctx context.Context, vetted []domain.LibraryKey, owner, name string) (views.LibraryPage, error)
	// RulePage returns the current rule at rulePath in the vetted library owner/name, matched as LibraryPage
	// matches it, or ErrNotFound.
	RulePage(ctx context.Context, vetted []domain.LibraryKey, owner, name, rulePath string) (views.RulePage, error)
}

// ErrNotFound reports a library or rule that isn't in the catalog, isn't vetted, or is retired.
var ErrNotFound = errors.New("not found")
