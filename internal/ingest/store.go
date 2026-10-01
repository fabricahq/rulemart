// Write a library's catalog rows in one transaction.

package ingest

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fabricahq/rulemart/internal/database"
	"github.com/fabricahq/rulemart/internal/ingest/ingestdb"
)

// Store writes the catalog.
type Store struct {
	db *database.DB
}

// NewStore returns a Store that writes through db.
func NewStore(db *database.DB) *Store {
	return &Store{db: db}
}

// replace makes the catalog's rows for lib match it, in one transaction, and returns how many rows changed. It's
// safe to repeat, so the database may retry it after a failed connection.
func (s *Store) replace(ctx context.Context, lib library) (int64, error) {
	var changed int64
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			w := writer{ctx: ctx, q: ingestdb.New(tx), library: lib.repo.ID}
			w.write(lib)
			changed = w.changed
			return w.err
		})
	})
	if err != nil {
		return 0, fmt.Errorf("write catalog githubID=%d: %v", lib.repo.ID, err)
	}
	return changed, nil
}

// writer applies one library's rows within a transaction. It stops at the first failed statement, keeping its
// error, and counts the rows each statement changed.
type writer struct {
	ctx     context.Context
	q       *ingestdb.Queries
	library int64
	changed int64
	err     error
}

// exec runs one statement unless an earlier one failed, adding the rows it changed.
func (w *writer) exec(what string, statement func() (int64, error)) {
	if w.err != nil {
		return
	}
	rows, err := statement()
	if err != nil {
		w.err = fmt.Errorf("%s: %v", what, err)
		return
	}
	w.changed += rows
}

// write upserts lib's rows and deletes the ones its tags no longer publish, in the order the foreign keys and the
// one-current-version index need: releases before the rules and versions that refer to them, stale versions before
// new ones, and stale releases last.
func (w *writer) write(lib library) {
	w.exec("upsert library", func() (int64, error) {
		return w.q.UpsertLibrary(w.ctx, ingestdb.UpsertLibraryParams{
			GithubID: w.library, Owner: lib.repo.Owner, Name: lib.repo.Name, Description: lib.repo.Description,
			OwnerAvatarUrl: lib.repo.OwnerAvatarURL, LicenseExpression: optionalText(lib.licenseExpression),
			LicenseFile: optionalText(lib.licenseFile),
		})
	})
	numbers := make([]int32, len(lib.releases))
	for i, r := range lib.releases {
		numbers[i] = int32(r.record.Release)
		w.exec("upsert "+r.tag, func() (int64, error) {
			return w.q.UpsertRelease(w.ctx, ingestdb.UpsertReleaseParams{
				LibraryID: w.library, Number: numbers[i], CommitID: r.commit.Hash.String(),
				TaggedAt: pgtype.Timestamptz{Time: r.taggedAt, Valid: true},
			})
		})
	}
	w.deleteStaleVersions(lib.rules)
	ruleIDs := make([]string, len(lib.rules))
	for i, r := range lib.rules {
		ruleIDs[i] = r.id
	}
	w.exec("delete stale rules", func() (int64, error) {
		return w.q.DeleteRulesExcept(w.ctx, ingestdb.DeleteRulesExceptParams{LibraryID: w.library, RuleIds: ruleIDs})
	})
	groupIDs := make([]string, len(lib.groups))
	for i, g := range lib.groups {
		groupIDs[i] = g.id
		w.exec("upsert group "+g.id, func() (int64, error) {
			return w.q.UpsertGroup(w.ctx, ingestdb.UpsertGroupParams{
				LibraryID: w.library, GroupID: g.id, Name: g.meta.Name, Description: g.meta.Description,
				WhenToRead: g.meta.WhenToRead,
			})
		})
	}
	w.exec("delete stale groups", func() (int64, error) {
		return w.q.DeleteGroupsExcept(w.ctx, ingestdb.DeleteGroupsExceptParams{LibraryID: w.library, GroupIds: groupIDs})
	})
	for _, r := range lib.rules {
		w.writeRule(r)
	}
	w.exec("delete stale releases", func() (int64, error) {
		return w.q.DeleteReleasesExcept(w.ctx, ingestdb.DeleteReleasesExceptParams{LibraryID: w.library, Numbers: numbers})
	})
}

// deleteStaleVersions deletes the stored versions that rules don't publish, such as after a library's tags were
// rewritten.
func (w *writer) deleteStaleVersions(rules []rule) {
	if w.err != nil {
		return
	}
	stored, err := w.q.ListVersionKeys(w.ctx, w.library)
	if err != nil {
		w.err = fmt.Errorf("list stored versions: %v", err)
		return
	}
	published := map[ingestdb.ListVersionKeysRow]bool{}
	for _, r := range rules {
		for _, v := range r.versions {
			published[ingestdb.ListVersionKeysRow{
				RuleID: r.id, Release: int32(v.release),
				Major: int32(v.version.Major), Minor: int32(v.version.Minor), Patch: int32(v.version.Patch),
			}] = true
		}
	}
	for _, key := range stored {
		if !published[key] {
			w.exec("delete stale version of "+key.RuleID, func() (int64, error) {
				return w.q.DeleteVersion(w.ctx, ingestdb.DeleteVersionParams{LibraryID: w.library, RuleID: key.RuleID, Release: key.Release})
			})
		}
	}
}

// writeRule upserts a rule and its versions, oldest first, so a version that stops being current loses its content
// before the new current version gains it.
func (w *writer) writeRule(r rule) {
	retiredIn := pgtype.Int4{Int32: int32(r.retiredIn), Valid: r.retiredIn != 0}
	w.exec("upsert rule "+r.id, func() (int64, error) {
		return w.q.UpsertRule(w.ctx, ingestdb.UpsertRuleParams{
			LibraryID: w.library, RuleID: r.id, GroupID: r.group, RetiredIn: retiredIn, ReplacedBy: optionalText(r.replacedBy),
		})
	})
	for i, v := range r.versions {
		params := ingestdb.UpsertVersionParams{
			LibraryID: w.library, RuleID: r.id, Release: int32(v.release), Change: string(v.change), Summaries: v.summaries,
			Major: int32(v.version.Major), Minor: int32(v.version.Minor), Patch: int32(v.version.Patch),
		}
		if c := r.content; c != nil && i == len(r.versions)-1 {
			params.Title, params.Impact = present(c.title), present(c.impact)
			params.ImpactDescription, params.WhenToRead = present(c.impactDescription), present(c.whenToRead)
			params.Markdown, params.Html = present(c.markdown), present(c.html)
		}
		w.exec(fmt.Sprintf("upsert %s %s", r.id, v.version), func() (int64, error) { return w.q.UpsertVersion(w.ctx, params) })
	}
}

// optionalText stores text, or NULL when it's empty.
func optionalText(text string) pgtype.Text {
	return pgtype.Text{String: text, Valid: text != ""}
}

// present stores text, even when it's empty.
func present(text string) pgtype.Text {
	return pgtype.Text{String: text, Valid: true}
}
