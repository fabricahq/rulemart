// Package postgres stores the catalog in Postgres: it replaces a library's rows in one transaction, and reads each
// page's rows from one snapshot. The catalog's SQL is in queries/, from which sqlc generates catalogdb.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres/generated/catalogdb"
	"github.com/fabricahq/rulemart/internal/platform/database"
)

// Store is the catalog in Postgres.
type Store struct {
	db *database.DB
}

// New returns a Store that reads and writes through db.
func New(db *database.DB) *Store {
	return &Store{db: db}
}

// ReplaceLibrary makes the catalog's rows for lib match it, in one transaction, and returns how many rows changed.
// It's safe to repeat, so the database may retry it after a failed connection.
func (s *Store) ReplaceLibrary(ctx context.Context, lib domain.Library) (int64, error) {
	var changed int64
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			w := writer{ctx: ctx, q: catalogdb.New(tx)}
			w.write(lib)
			changed = w.changed
			return w.err
		})
	})
	if err != nil {
		return 0, fmt.Errorf("write catalog host=%s repository=%s: %v", lib.Repository.Host, lib.Repository.ID, err)
	}
	return changed, nil
}

// Checkpoint returns where library was last fetched from and the tags of its stored releases, in one statement, so
// it reads one committed state. found is false when the catalog has no such library.
func (s *Store) Checkpoint(ctx context.Context, library domain.LibraryKey) (domain.Checkpoint, bool, error) {
	var rows []catalogdb.GetCheckpointRow
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		var err error
		rows, err = catalogdb.New(pool).GetCheckpoint(ctx, catalogdb.GetCheckpointParams{Host: library.Host, HostRepositoryID: library.RepositoryID})
		return err
	})
	if err != nil {
		return domain.Checkpoint{}, false, fmt.Errorf("read catalog checkpoint host=%s repository=%s: %v", library.Host, library.RepositoryID, err)
	}
	if len(rows) == 0 {
		return domain.Checkpoint{}, false, nil
	}
	checkpoint := domain.Checkpoint{CloneURL: rows[0].CloneUrl.String, Tags: domain.ReleaseTags{}}
	for _, row := range rows {
		if row.Number.Valid {
			checkpoint.Tags[int(row.Number.Int32)] = row.TagObjectID.String
		}
	}
	return checkpoint, true, nil
}

// writer applies one library's rows within a transaction. It stops at the first failed statement, keeping its
// error, and counts the rows each statement changed.
type writer struct {
	ctx     context.Context
	q       *catalogdb.Queries
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
func (w *writer) write(lib domain.Library) {
	w.writeLibrary(lib)
	numbers := make([]int32, len(lib.Releases))
	for i, r := range lib.Releases {
		numbers[i] = int32(r.Number)
		w.exec(fmt.Sprintf("upsert release/%d", r.Number), func() (int64, error) {
			return w.q.UpsertRelease(w.ctx, catalogdb.UpsertReleaseParams{
				LibraryID: w.library, Number: numbers[i], TagObjectID: optionalText(r.TagID), CommitID: r.CommitID,
				TaggedAt: pgtype.Timestamptz{Time: r.TaggedAt, Valid: true}, UpdatesSharedFiles: r.UpdatesSharedFiles,
			})
		})
	}
	w.readIDs()
	w.deleteStaleVersions(lib.Rules)
	groupPaths := make([]string, len(lib.Groups))
	for i, g := range lib.Groups {
		groupPaths[i] = g.Path
		w.exec("upsert group "+g.Path, func() (int64, error) {
			return w.q.UpsertGroup(w.ctx, catalogdb.UpsertGroupParams{
				LibraryID: w.library, Path: g.Path, Name: g.Name, Description: g.Description, WhenToRead: g.WhenToRead,
			})
		})
	}
	w.readIDs()
	rulePaths := make([]string, len(lib.Rules))
	for i, r := range lib.Rules {
		rulePaths[i] = r.Path
		w.writeRule(r)
	}
	w.readIDs()
	for _, r := range lib.Rules {
		w.writeVersions(r)
	}
	w.exec("delete stale rules", func() (int64, error) {
		return w.q.DeleteRulesExcept(w.ctx, catalogdb.DeleteRulesExceptParams{LibraryID: w.library, Paths: rulePaths})
	})
	w.exec("delete stale groups", func() (int64, error) {
		return w.q.DeleteGroupsExcept(w.ctx, catalogdb.DeleteGroupsExceptParams{LibraryID: w.library, Paths: groupPaths})
	})
	w.exec("delete stale releases", func() (int64, error) {
		return w.q.DeleteReleasesExcept(w.ctx, catalogdb.DeleteReleasesExceptParams{LibraryID: w.library, Numbers: numbers})
	})
}

// writeLibrary upserts the library on its host and repository ID, and keeps its catalog id.
func (w *writer) writeLibrary(lib domain.Library) {
	repo := lib.Repository
	w.exec("upsert library", func() (int64, error) {
		return w.q.UpsertLibrary(w.ctx, catalogdb.UpsertLibraryParams{
			Host: repo.Host, HostRepositoryID: repo.ID, Owner: repo.Owner, Name: repo.Name,
			Description: repo.Description, OwnerAvatarUrl: repo.OwnerAvatarURL,
			LicenseExpression: optionalText(lib.LicenseExpression), LicenseFile: optionalText(lib.LicenseFile),
			CloneUrl: optionalText(repo.CloneURL),
		})
	})
	if w.err != nil {
		return
	}
	id, err := w.q.GetLibraryID(w.ctx, catalogdb.GetLibraryIDParams{Host: repo.Host, HostRepositoryID: repo.ID})
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
// rewritten. A version is its rule and number: one that rewritten tags moved to another release survives, and its
// upsert moves it.
func (w *writer) deleteStaleVersions(rules []domain.Rule) {
	if w.err != nil {
		return
	}
	stored, err := w.q.ListVersionKeys(w.ctx, w.library)
	if err != nil {
		w.err = fmt.Errorf("list stored versions: %v", err)
		return
	}
	type key struct {
		path                string
		major, minor, patch int32
	}
	published := map[key]bool{}
	for _, r := range rules {
		for _, v := range r.Versions {
			published[key{r.Path, int32(v.Number.Major), int32(v.Number.Minor), int32(v.Number.Patch)}] = true
		}
	}
	for _, row := range stored {
		if !published[key{row.Path, row.Major, row.Minor, row.Patch}] {
			w.exec("delete stale version of "+row.Path, func() (int64, error) { return w.q.DeleteVersion(w.ctx, row.ID) })
		}
	}
}

// writeRule upserts a rule in its group.
func (w *writer) writeRule(r domain.Rule) {
	var retiredIn pgtype.Int8
	if r.RetiredIn != 0 {
		retiredIn = pgtype.Int8{Int64: w.releases[r.RetiredIn], Valid: true}
	}
	w.exec("upsert rule "+r.Path, func() (int64, error) {
		return w.q.UpsertRule(w.ctx, catalogdb.UpsertRuleParams{
			LibraryID: w.library, GroupID: w.groups[r.Group], Path: r.Path, RetiredInReleaseID: retiredIn,
			ReplacedBy: optionalText(r.ReplacedBy), RetirementSummaries: r.RetirementSummaries,
		})
	})
}

// writeVersions upserts a rule's versions, oldest first, so a version that stops being current loses its content
// before the new current version gains it.
func (w *writer) writeVersions(r domain.Rule) {
	for i, v := range r.Versions {
		params := catalogdb.UpsertVersionParams{
			LibraryID: w.library, RuleID: w.rules[r.Path], ReleaseID: w.releases[v.Release], Change: string(v.Change), Summaries: v.Summaries,
			Major: int32(v.Number.Major), Minor: int32(v.Number.Minor), Patch: int32(v.Number.Patch),
		}
		if c := r.Content; c != nil && i == len(r.Versions)-1 {
			params.Title, params.Impact = present(c.Title), present(c.Impact)
			params.ImpactDescription, params.WhenToRead = present(c.ImpactDescription), present(c.WhenToRead)
			params.Markdown, params.Html = present(c.Markdown), present(c.HTML)
		}
		w.exec(fmt.Sprintf("upsert %s %s", r.Path, v.Number), func() (int64, error) { return w.q.UpsertVersion(w.ctx, params) })
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
