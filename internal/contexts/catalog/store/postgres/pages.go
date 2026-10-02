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

// Libraries returns the vetted libraries, ordered by owner and name without regard to case.
func (s *Store) Libraries(ctx context.Context, vetted []domain.LibraryKey) ([]views.LibraryCard, error) {
	var cards []views.LibraryCard
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		var err error
		cards, err = libraries(ctx, q, vetted)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("load libraries: %v", err)
	}
	return cards, nil
}

// OwnerLibraries returns the vetted libraries whose owner is login, matched without regard to case, ordered by name
// without regard to case.
func (s *Store) OwnerLibraries(ctx context.Context, vetted []domain.LibraryKey, login string) ([]views.LibraryCard, error) {
	var cards []views.LibraryCard
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		rows, err := q.ListOwnerLibraries(ctx, catalogdb.ListOwnerLibrariesParams{Vetted: vettedKeys(vetted), Login: login})
		if err != nil {
			return err
		}
		cards = make([]views.LibraryCard, len(rows))
		for i, row := range rows {
			cards[i] = views.LibraryCard{
				Owner: row.Owner, Name: row.Name, Description: row.Description, OwnerAvatarURL: row.OwnerAvatarUrl,
				Rules: int(row.RuleCount),
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load owner libraries login=%q: %v", login, err)
	}
	return cards, nil
}

// UnvettedLibraries returns the libraries listings name that vetted doesn't hold, ordered by owner and name without
// regard to case.
func (s *Store) UnvettedLibraries(ctx context.Context, vetted []domain.LibraryKey) ([]views.LibraryCard, error) {
	var cards []views.LibraryCard
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		rows, err := q.ListUnvettedLibraries(ctx, vettedKeys(vetted))
		if err != nil {
			return err
		}
		cards = make([]views.LibraryCard, len(rows))
		for i, row := range rows {
			cards[i] = views.LibraryCard{
				Owner: row.Owner, Name: row.Name, Description: row.Description, OwnerAvatarURL: row.OwnerAvatarUrl,
				Rules: int(row.RuleCount),
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load unvetted libraries: %v", err)
	}
	return cards, nil
}

// HomePage returns the vetted libraries, ordered by owner and name, and each group that holds current rules in them,
// as Groups returns them.
func (s *Store) HomePage(ctx context.Context, vetted []domain.LibraryKey) ([]views.LibraryCard, []views.LibraryGroup, error) {
	var cards []views.LibraryCard
	var groups []views.LibraryGroup
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		var err error
		if cards, err = libraries(ctx, q, vetted); err != nil {
			return err
		}
		groups, err = libraryGroups(ctx, q, vetted)
		return err
	})
	if err != nil {
		return nil, nil, fmt.Errorf("load home page: %v", err)
	}
	return cards, groups, nil
}

// libraries returns the vetted libraries, ordered by owner and name.
func libraries(ctx context.Context, q *catalogdb.Queries, vetted []domain.LibraryKey) ([]views.LibraryCard, error) {
	rows, err := q.ListLibraries(ctx, vettedKeys(vetted))
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

// LibraryPage returns the vetted library owner/name, matched without regard to case, with its groups, current rules,
// and retired rules. It fails with store.ErrNotFound when there's no such library.
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
		retired, err := q.ListRetiredRules(ctx, id)
		if err != nil {
			return err
		}
		links, err := ruleLinks(ctx, q, id)
		if err != nil {
			return err
		}
		ids := make([]int64, len(rules))
		for i, r := range rules {
			ids[i] = r.ID
		}
		stars, err := ruleStars(ctx, q, ids)
		if err != nil {
			return err
		}
		page = views.LibraryPage{Library: lib, Links: links}
		for _, r := range retired {
			page.Retired = append(page.Retired, views.RetiredRuleCard{
				Path: r.Path, Title: r.Title.String, LastVersion: version(r.Major, r.Minor, r.Patch), RetiredIn: int(r.RetiredIn),
				ReplacedBy: r.ReplacedBy.String,
			})
		}
		for _, g := range groups {
			page.Groups = append(page.Groups, views.Group{
				Path: g.Path, Description: g.Description, WhenToRead: g.WhenToRead, Rules: int(g.RuleCount),
			})
		}
		for _, r := range rules {
			page.Rules = append(page.Rules, views.RuleCard{
				Path: r.Path, Group: r.GroupPath, Title: r.Title, Impact: r.Impact, Version: version(r.Major, r.Minor, r.Patch),
				Stars: stars[r.ID],
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

// RulePage returns the vetted library owner/name, matched as LibraryPage matches it, and its rule at rulePath, current
// or retired, with every version, newest first. It fails with store.ErrNotFound when there's no such library or rule.
func (s *Store) RulePage(ctx context.Context, vetted []domain.LibraryKey, owner, name, rulePath string) (views.RulePage, error) {
	var page views.RulePage
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		var err error
		page, _, err = rulePage(ctx, q, vetted, owner, name, rulePath)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return views.RulePage{}, fmt.Errorf("load rule %s/%s/%s: %w", owner, name, rulePath, store.ErrNotFound)
	}
	if err != nil {
		return views.RulePage{}, fmt.Errorf("load rule %s/%s/%s: %v", owner, name, rulePath, err)
	}
	return page, nil
}

// RuleComparison returns the rule's page, as RulePage does, with the text of its versions from and to, read only when
// both are stored and hold at most maxBytes together. It fails with store.ErrNotFound when there's no such library or
// rule, or when either isn't a version of the rule.
func (s *Store) RuleComparison(ctx context.Context, vetted []domain.LibraryKey, owner, name, rulePath string, from, to coderules.RuleVersion, maxBytes int64) (views.RuleComparison, error) {
	comparison := views.RuleComparison{From: from, To: to}
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		page, stored, err := rulePage(ctx, q, vetted, owner, name, rulePath)
		if err != nil {
			return err
		}
		comparison.Page = page
		pair := textPair{}
		var found [2]bool
		for i, v := range page.Versions {
			if v.Version == from {
				pair.old, found[0] = stored[i], true
			}
			if v.Version == to {
				pair.new, found[1] = stored[i], true
			}
		}
		if !found[0] || !found[1] {
			return pgx.ErrNoRows
		}
		texts, err := compareTexts(ctx, q, []textPair{pair}, maxBytes)
		if err != nil {
			return err
		}
		comparison.Text = texts[0]
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return views.RuleComparison{}, fmt.Errorf("compare versions of rule %s/%s/%s: %w", owner, name, rulePath, store.ErrNotFound)
	}
	if err != nil {
		return views.RuleComparison{}, fmt.Errorf("compare versions of rule %s/%s/%s: %v", owner, name, rulePath, err)
	}
	return comparison, nil
}

// rulePage reads the rule at rulePath in the vetted library owner/name, current or retired, with every version, newest
// first, and what's stored of each version's text, in the same order. It returns pgx.ErrNoRows when there's no such
// library or rule.
func rulePage(ctx context.Context, q *catalogdb.Queries, vetted []domain.LibraryKey, owner, name, rulePath string) (views.RulePage, []storedText, error) {
	lib, id, err := library(ctx, q, vetted, owner, name)
	if err != nil {
		return views.RulePage{}, nil, err
	}
	r, err := q.GetRule(ctx, catalogdb.GetRuleParams{LibraryID: id, Path: rulePath})
	if err != nil {
		return views.RulePage{}, nil, err
	}
	versions, err := q.ListVersions(ctx, r.ID)
	if err != nil {
		return views.RulePage{}, nil, err
	}
	links, err := ruleLinks(ctx, q, id)
	if err != nil {
		return views.RulePage{}, nil, err
	}
	page := views.RulePage{Library: lib, Rule: views.Rule{
		Path: r.Path, Group: r.GroupPath, Title: r.Title.String, Impact: r.Impact.String,
		WhenToRead: r.WhenToRead.String, WhenToReadHTML: r.WhenToReadHtml, HTML: r.Html.String,
		Version: version(r.Major, r.Minor, r.Patch),
		Release: int(r.Release), PublishedAt: r.PublishedAt.Time,
	}, Links: links}
	if r.RetiredIn.Valid {
		page.Rule.Retirement = &views.Retirement{Release: int(r.RetiredIn.Int32), RetiredAt: r.RetiredAt.Time, Summaries: r.RetirementSummaries}
	} else {
		stars, err := ruleStars(ctx, q, []int64{r.ID})
		if err != nil {
			return views.RulePage{}, nil, err
		}
		page.Rule.Stars = stars[r.ID]
	}
	stored := make([]storedText, len(versions))
	for i, v := range versions {
		page.Versions = append(page.Versions, views.Version{
			Version: version(v.Major, v.Minor, v.Patch), Release: int(v.Release), PublishedAt: v.PublishedAt.Time,
			Change: coderules.Change(v.Change), Summaries: v.Summaries,
		})
		stored[i] = storedText{id: v.ID, release: int(v.Release), present: v.HasMarkdown, bytes: int64(v.MarkdownBytes)}
	}
	return page, stored, nil
}

// ruleLinks reads how every rule of the library library was replaced, in path order.
func ruleLinks(ctx context.Context, q *catalogdb.Queries, library int64) ([]views.RuleLink, error) {
	rows, err := q.ListRuleLinks(ctx, library)
	if err != nil {
		return nil, err
	}
	links := make([]views.RuleLink, len(rows))
	for i, row := range rows {
		links[i] = views.RuleLink{
			Path: row.Path, Title: row.LastTitle.String, RetiredIn: int(row.RetiredIn.Int32), ReplacedBy: row.ReplacedBy.String,
			FirstRelease: int(row.FirstRelease), FirstTitle: row.FirstTitle.String, LastTitle: row.LastTitle.String,
		}
	}
	return links, nil
}

// LibraryHistory returns the vetted library owner/name, matched as LibraryPage matches it, with its releases and every
// rule's versions. It fails with store.ErrNotFound when there's no such library.
func (s *Store) LibraryHistory(ctx context.Context, vetted []domain.LibraryKey, owner, name string) (views.LibraryHistory, error) {
	var history views.LibraryHistory
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		var err error
		history, _, err = libraryHistory(ctx, q, vetted, owner, name)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return views.LibraryHistory{}, fmt.Errorf("load library history %s/%s: %w", owner, name, store.ErrNotFound)
	}
	if err != nil {
		return views.LibraryHistory{}, fmt.Errorf("load library history %s/%s: %v", owner, name, err)
	}
	return history, nil
}

// ReleaseComparison returns the library's history, as LibraryHistory does, and the text of each pair of versions that
// pick chooses from it, keyed by the pair's key, read from the same snapshot. It reads the pairs in pick's order, each
// only when both texts are stored and hold, with the pairs before it, at most maxBytes.
func (s *Store) ReleaseComparison(ctx context.Context, vetted []domain.LibraryKey, owner, name string, pick func(views.LibraryHistory) []views.VersionPair, maxBytes int64) (views.LibraryHistory, map[string]views.ComparedText, error) {
	var history views.LibraryHistory
	texts := map[string]views.ComparedText{}
	err := s.read(ctx, func(q *catalogdb.Queries) error {
		var stored [][]storedText
		var err error
		history, stored, err = libraryHistory(ctx, q, vetted, owner, name)
		if err != nil {
			return err
		}
		picked := pick(history)
		pairs := make([]textPair, len(picked))
		for i, p := range picked {
			pairs[i] = textPair{old: stored[p.OldRule][p.OldVersion], new: stored[p.NewRule][p.NewVersion]}
		}
		compared, err := compareTexts(ctx, q, pairs, maxBytes)
		if err != nil {
			return err
		}
		for i, p := range picked {
			texts[p.Key] = compared[i]
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return views.LibraryHistory{}, nil, fmt.Errorf("compare releases of library %s/%s: %w", owner, name, store.ErrNotFound)
	}
	if err != nil {
		return views.LibraryHistory{}, nil, fmt.Errorf("compare releases of library %s/%s: %v", owner, name, err)
	}
	return history, texts, nil
}

// libraryHistory reads the vetted library owner/name with its releases and every rule's versions, and what's stored of
// each version's text, by rule and version in the history's order. It returns pgx.ErrNoRows when there's no such
// library.
func libraryHistory(ctx context.Context, q *catalogdb.Queries, vetted []domain.LibraryKey, owner, name string) (views.LibraryHistory, [][]storedText, error) {
	lib, id, err := library(ctx, q, vetted, owner, name)
	if err != nil {
		return views.LibraryHistory{}, nil, err
	}
	releases, err := q.ListReleases(ctx, id)
	if err != nil {
		return views.LibraryHistory{}, nil, err
	}
	rows, err := q.ListRuleHistories(ctx, id)
	if err != nil {
		return views.LibraryHistory{}, nil, err
	}
	history := views.LibraryHistory{Library: lib}
	for _, r := range releases {
		history.Releases = append(history.Releases, views.Release{
			Number: int(r.Number), TaggedAt: r.TaggedAt.Time, UpdatesSharedFiles: r.UpdatesSharedFiles,
		})
	}
	var stored [][]storedText
	for _, row := range rows {
		if n := len(history.Rules); n == 0 || history.Rules[n-1].Path != row.Path {
			history.Rules = append(history.Rules, views.RuleHistory{
				Path: row.Path, RetiredIn: int(row.RetiredIn.Int32), ReplacedBy: row.ReplacedBy.String,
				RetirementSummaries: row.RetirementSummaries,
			})
			stored = append(stored, nil)
		}
		r, n := &history.Rules[len(history.Rules)-1], len(stored)-1
		// Rows come oldest first, so the last one's title is the rule's newest.
		r.Title = row.Title.String
		r.Versions = append(r.Versions, views.Version{
			Version: version(row.Major, row.Minor, row.Patch), Release: int(row.Release), PublishedAt: row.PublishedAt.Time,
			Change: coderules.Change(row.Change), Summaries: row.Summaries, Title: row.Title.String,
		})
		stored[n] = append(stored[n], storedText{id: row.ID, release: int(row.Release), present: row.HasMarkdown, bytes: int64(row.MarkdownBytes)})
	}
	return history, stored, nil
}

// storedText is what the catalog stores of one version's text: whether it has it, and how large it is.
type storedText struct {
	// id is the version's row, and release the number of the library release that published it.
	id      int64
	release int
	present bool
	bytes   int64
}

// textPair is the stored text of two versions of a rule to compare, the older first.
type textPair struct {
	old, new storedText
}

// compareTexts returns, for each pair in order, the two versions' text, read when both are stored and together with
// the pairs read before them hold at most maxBytes.
func compareTexts(ctx context.Context, q *catalogdb.Queries, pairs []textPair, maxBytes int64) ([]views.ComparedText, error) {
	texts := make([]views.ComparedText, len(pairs))
	var ids []int64
	var spent int64
	for i, p := range pairs {
		texts[i].OldRelease, texts[i].NewRelease = p.old.release, p.new.release
		switch size := p.old.bytes + p.new.bytes; {
		case !p.old.present || !p.new.present:
			texts[i].State = views.TextMissing
		case spent+size > maxBytes:
			texts[i].State = views.TextTooLarge
		default:
			spent += size
			ids = append(ids, p.old.id, p.new.id)
		}
	}
	if len(ids) == 0 {
		return texts, nil
	}
	rows, err := q.ListMarkdown(ctx, ids)
	if err != nil {
		return nil, err
	}
	markdown := make(map[int64]string, len(rows))
	for _, row := range rows {
		markdown[row.ID] = row.Markdown
	}
	for i, p := range pairs {
		if texts[i].State == views.TextShown {
			texts[i].Old, texts[i].New = markdown[p.old.id], markdown[p.new.id]
		}
	}
	return texts, nil
}

// library returns the library owner/name that vetted holds or a listing names, and its catalog id, or pgx.ErrNoRows
// when there's none.
func library(ctx context.Context, q *catalogdb.Queries, vetted []domain.LibraryKey, owner, name string) (views.Library, int64, error) {
	row, err := q.GetLibrary(ctx, catalogdb.GetLibraryParams{Host: domain.GitHub, Owner: owner, Name: name, Vetted: vettedKeys(vetted)})
	if err != nil {
		return views.Library{}, 0, err
	}
	return views.Library{
		Vetted: row.Vetted, Owner: row.Owner, Name: row.Name, Description: row.Description, OwnerAvatarURL: row.OwnerAvatarUrl,
		LicenseExpression: row.LicenseExpression.String, LicenseFile: row.LicenseFile.String,
		LatestRelease: int(row.LatestRelease), LatestTaggedAt: row.LatestTaggedAt.Time,
		Groups: int(row.GroupCount), Rules: int(row.RuleCount),
	}, row.ID, nil
}

func version(major, minor, patch int32) coderules.RuleVersion {
	return coderules.RuleVersion{Major: int(major), Minor: int(minor), Patch: int(patch)}
}
