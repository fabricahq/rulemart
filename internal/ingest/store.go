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
			w := writer{ctx: ctx, q: ingestdb.New(tx)}
			w.write(lib)
			changed = w.changed
			return w.err
		})
	})
	if err != nil {
		return 0, fmt.Errorf("write catalog host=%s repository=%s: %v", lib.repo.Host, lib.repo.ID, err)
	}
	return changed, nil
}

// writer applies one library's rows within a transaction. It stops at the first failed statement, keeping its
// error, and counts the rows each statement changed.
type writer struct {
	ctx     context.Context
	q       *ingestdb.Queries
	changed int64
	err     error
	// library is the library's catalog id, and releases, groups, and rules map each natural key to its row's id,
	// once the rows are written.
	library  int64
	releases map[int]int64
	groups   map[string]int64
	rules    map[string]int64
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

// write upserts lib's rows on their natural keys and deletes the ones its tags no longer publish. A row that still
// exists keeps its id, and each upsert changes a row only when its values differ, so unchanged tags write nothing.
// It writes in the order the foreign keys and the one-current-version index need: the library, then releases,
// stale versions before new ones, groups before the rules in them, and stale rows last, children before parents.
func (w *writer) write(lib library) {
	w.writeLibrary(lib)
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
	w.readIDs()
	w.deleteStaleVersions(lib.rules)
	groupPaths := make([]string, len(lib.groups))
	for i, g := range lib.groups {
		groupPaths[i] = g.id
		w.exec("upsert group "+g.id, func() (int64, error) {
			return w.q.UpsertGroup(w.ctx, ingestdb.UpsertGroupParams{
				LibraryID: w.library, Path: g.id, Name: g.meta.Name, Description: g.meta.Description,
				WhenToRead: g.meta.WhenToRead,
			})
		})
	}
	w.readIDs()
	rulePaths := make([]string, len(lib.rules))
	for i, r := range lib.rules {
		rulePaths[i] = r.id
		w.writeRule(r)
	}
	w.readIDs()
	for _, r := range lib.rules {
		w.writeVersions(r)
	}
	w.exec("delete stale rules", func() (int64, error) {
		return w.q.DeleteRulesExcept(w.ctx, ingestdb.DeleteRulesExceptParams{LibraryID: w.library, Paths: rulePaths})
	})
	w.exec("delete stale groups", func() (int64, error) {
		return w.q.DeleteGroupsExcept(w.ctx, ingestdb.DeleteGroupsExceptParams{LibraryID: w.library, Paths: groupPaths})
	})
	w.exec("delete stale releases", func() (int64, error) {
		return w.q.DeleteReleasesExcept(w.ctx, ingestdb.DeleteReleasesExceptParams{LibraryID: w.library, Numbers: numbers})
	})
}

// writeLibrary upserts the library on its host and repository ID, and keeps its catalog id.
func (w *writer) writeLibrary(lib library) {
	w.exec("upsert library", func() (int64, error) {
		return w.q.UpsertLibrary(w.ctx, ingestdb.UpsertLibraryParams{
			Host: lib.repo.Host, HostRepositoryID: lib.repo.ID, Owner: lib.repo.Owner, Name: lib.repo.Name,
			Description: lib.repo.Description, OwnerAvatarUrl: lib.repo.OwnerAvatarURL,
			LicenseExpression: optionalText(lib.licenseExpression), LicenseFile: optionalText(lib.licenseFile),
		})
	})
	if w.err != nil {
		return
	}
	id, err := w.q.GetLibraryID(w.ctx, ingestdb.GetLibraryIDParams{Host: lib.repo.Host, HostRepositoryID: lib.repo.ID})
	if err != nil {
		w.err = fmt.Errorf("read library id: %v", err)
		return
	}
	w.library = id
}

// readIDs reads the ids of the library's stored releases, groups, and rules by their natural keys.
func (w *writer) readIDs() {
	if w.err != nil {
		return
	}
	releases, err := w.q.ListReleaseIDs(w.ctx, w.library)
	if err != nil {
		w.err = fmt.Errorf("read release ids: %v", err)
		return
	}
	groups, err := w.q.ListGroupIDs(w.ctx, w.library)
	if err != nil {
		w.err = fmt.Errorf("read group ids: %v", err)
		return
	}
	rules, err := w.q.ListRuleIDs(w.ctx, w.library)
	if err != nil {
		w.err = fmt.Errorf("read rule ids: %v", err)
		return
	}
	w.releases, w.groups, w.rules = map[int]int64{}, map[string]int64{}, map[string]int64{}
	for _, r := range releases {
		w.releases[int(r.Number)] = r.ID
	}
	for _, g := range groups {
		w.groups[g.Path] = g.ID
	}
	for _, r := range rules {
		w.rules[r.Path] = r.ID
	}
}

// deleteStaleVersions deletes the stored versions that rules don't publish, such as after a library's tags were
// rewritten, so the upserts that follow never meet a stale row on either of a version's unique keys.
func (w *writer) deleteStaleVersions(rules []rule) {
	if w.err != nil {
		return
	}
	stored, err := w.q.ListVersionKeys(w.ctx, w.library)
	if err != nil {
		w.err = fmt.Errorf("list stored versions: %v", err)
		return
	}
	type key struct {
		path                         string
		release, major, minor, patch int32
	}
	published := map[key]bool{}
	for _, r := range rules {
		for _, v := range r.versions {
			published[key{r.id, int32(v.release), int32(v.version.Major), int32(v.version.Minor), int32(v.version.Patch)}] = true
		}
	}
	for _, row := range stored {
		if !published[key{row.Path, row.Release, row.Major, row.Minor, row.Patch}] {
			w.exec("delete stale version of "+row.Path, func() (int64, error) { return w.q.DeleteVersion(w.ctx, row.ID) })
		}
	}
}

// writeRule upserts a rule in its group.
func (w *writer) writeRule(r rule) {
	var retiredIn pgtype.Int8
	if r.retiredIn != 0 {
		retiredIn = pgtype.Int8{Int64: w.releases[r.retiredIn], Valid: true}
	}
	w.exec("upsert rule "+r.id, func() (int64, error) {
		return w.q.UpsertRule(w.ctx, ingestdb.UpsertRuleParams{
			LibraryID: w.library, GroupID: w.groups[r.group], Path: r.id, RetiredInReleaseID: retiredIn,
			ReplacedBy: optionalText(r.replacedBy),
		})
	})
}

// writeVersions upserts a rule's versions, oldest first, so a version that stops being current loses its content
// before the new current version gains it.
func (w *writer) writeVersions(r rule) {
	for i, v := range r.versions {
		params := ingestdb.UpsertVersionParams{
			RuleID: w.rules[r.id], ReleaseID: w.releases[v.release], Change: string(v.change), Summaries: v.summaries,
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
