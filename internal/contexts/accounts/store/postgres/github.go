// Store what Rulemart read of each account's GitHub account, and the installations of the GitHub App it reads private
// repositories through.

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/store/postgres/generated/accountsdb"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// Snapshot returns the account's GitHub snapshot, as store.Store describes.
func (s *Store) Snapshot(ctx context.Context, accountID int64) (domain.Snapshot, time.Time, bool, error) {
	var row accountsdb.GetSnapshotRow
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		var err error
		row, err = accountsdb.New(pool).GetSnapshot(ctx, accountID)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Snapshot{}, time.Time{}, false, nil
	}
	if err != nil {
		return domain.Snapshot{}, time.Time{}, false, fmt.Errorf("read GitHub snapshot accountID=%d: %v", accountID, err)
	}
	var record snapshotRecord
	if err := json.Unmarshal(row.Snapshot, &record); err != nil {
		return domain.Snapshot{}, time.Time{}, false, fmt.Errorf("read GitHub snapshot accountID=%d: decode it: %v", accountID, err)
	}
	return record.snapshot(), row.TriedAt.Time, true, nil
}

// SaveSnapshot keeps snapshot as the account's, as store.Store describes.
func (s *Store) SaveSnapshot(ctx context.Context, accountID int64, snapshot domain.Snapshot, triedAt time.Time) error {
	data, err := json.Marshal(newSnapshotRecord(snapshot))
	if err != nil {
		return fmt.Errorf("save GitHub snapshot accountID=%d: encode it: %v", accountID, err)
	}
	err = s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		return accountsdb.New(pool).SaveSnapshot(ctx, accountsdb.SaveSnapshotParams{
			AccountID: accountID, TriedAt: pgtype.Timestamptz{Time: triedAt, Valid: true}, Snapshot: data,
		})
	})
	if err != nil {
		return fmt.Errorf("save GitHub snapshot accountID=%d: %v", accountID, err)
	}
	return nil
}

// Installations returns the installations the account reads private repositories through, as store.Store describes.
func (s *Store) Installations(ctx context.Context, accountID int64) ([]domain.Installation, error) {
	var rows []accountsdb.ListInstallationsRow
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		var err error
		rows, err = accountsdb.New(pool).ListInstallations(ctx, accountID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("read GitHub installations accountID=%d: %v", accountID, err)
	}
	installations := make([]domain.Installation, len(rows))
	for i, row := range rows {
		installations[i] = domain.Installation{ID: row.InstallationID, Account: row.GithubAccount}
	}
	return installations, nil
}

// AddInstallation records the installation for the account and discards its snapshot, in one transaction.
func (s *Store) AddInstallation(ctx context.Context, accountID int64, installation domain.Installation) error {
	err := s.inTransaction(ctx, func(q *accountsdb.Queries) error {
		if err := q.AddInstallation(ctx, accountsdb.AddInstallationParams{
			AccountID: accountID, InstallationID: installation.ID, GithubAccount: installation.Account,
		}); err != nil {
			return err
		}
		return q.DeleteSnapshot(ctx, accountID)
	})
	if err != nil {
		return fmt.Errorf("add GitHub installation accountID=%d installationID=%d: %v", accountID, installation.ID, err)
	}
	return nil
}

// RemoveInstallations forgets the account's installations and discards its snapshot, in one transaction.
func (s *Store) RemoveInstallations(ctx context.Context, accountID int64) error {
	err := s.inTransaction(ctx, func(q *accountsdb.Queries) error {
		if err := q.DeleteAccountInstallations(ctx, accountID); err != nil {
			return err
		}
		return q.DeleteSnapshot(ctx, accountID)
	})
	if err != nil {
		return fmt.Errorf("remove GitHub installations accountID=%d: %v", accountID, err)
	}
	return nil
}

// InstallationRemoved forgets the installation for every account and discards their snapshots, in one statement.
func (s *Store) InstallationRemoved(ctx context.Context, id int64) error {
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		_, err := accountsdb.New(pool).DeleteInstallation(ctx, id)
		return err
	})
	if err != nil {
		return fmt.Errorf("forget GitHub installation installationID=%d: %v", id, err)
	}
	return nil
}

// InstallationChanged discards the snapshots of the accounts that read through the installation.
func (s *Store) InstallationChanged(ctx context.Context, id int64) error {
	err := s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		_, err := accountsdb.New(pool).DiscardInstallationSnapshots(ctx, id)
		return err
	})
	if err != nil {
		return fmt.Errorf("discard snapshots of GitHub installation installationID=%d: %v", id, err)
	}
	return nil
}

// inTransaction runs work in one transaction, committed when work returns nil. It's safe to repeat after a failed
// connection, since the transaction either committed nothing or everything.
func (s *Store) inTransaction(ctx context.Context, work func(*accountsdb.Queries) error) error {
	return s.db.Run(ctx, func(pool *pgxpool.Pool) error {
		return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error { return work(accountsdb.New(tx)) })
	})
}

// snapshotRecord is a snapshot as github_snapshots keeps it, in JSON. Its field names are the stored format, so a
// rename here needs a migration of the stored rows, or a read that accepts both.
type snapshotRecord struct {
	// ReadAt is nil when no read has succeeded.
	ReadAt        *time.Time      `json:"readAt,omitempty"`
	Organizations []string        `json:"organizations"`
	Libraries     []libraryRecord `json:"libraries"`
	Projects      []projectRecord `json:"projects"`
	Truncated     bool            `json:"truncated,omitempty"`
	Failure       string          `json:"failure,omitempty"`
}

type repositoryRecord struct {
	Owner   string `json:"owner"`
	Name    string `json:"name"`
	Private bool   `json:"private,omitempty"`
}

type libraryRecord struct {
	repositoryRecord
	Release int `json:"release"`
}

type projectRecord struct {
	repositoryRecord
	Sources []sourceRecord `json:"sources"`
}

type sourceRecord struct {
	Name    string       `json:"name"`
	Library string       `json:"library,omitempty"`
	Release int          `json:"release,omitempty"`
	Groups  []string     `json:"groups,omitempty"`
	Rules   []ruleRecord `json:"rules,omitempty"`
}

type ruleRecord struct {
	Path    string                `json:"path"`
	Version coderules.RuleVersion `json:"version"`
}

func newSnapshotRecord(s domain.Snapshot) snapshotRecord {
	record := snapshotRecord{
		Organizations: append([]string{}, s.Organizations...), Libraries: []libraryRecord{}, Projects: []projectRecord{},
		Truncated: s.Truncated, Failure: s.Failure,
	}
	if !s.ReadAt.IsZero() {
		record.ReadAt = &s.ReadAt
	}
	for _, l := range s.Libraries {
		record.Libraries = append(record.Libraries, libraryRecord{repositoryRecord: newRepositoryRecord(l.Repository), Release: l.Release})
	}
	for _, p := range s.Projects {
		project := projectRecord{repositoryRecord: newRepositoryRecord(p.Repository), Sources: []sourceRecord{}}
		for _, source := range p.Sources {
			sr := sourceRecord{Name: source.Name, Library: source.Library, Release: source.Release, Groups: source.Groups}
			for _, r := range source.Rules {
				sr.Rules = append(sr.Rules, ruleRecord{Path: r.Path, Version: r.Version})
			}
			project.Sources = append(project.Sources, sr)
		}
		record.Projects = append(record.Projects, project)
	}
	return record
}

func newRepositoryRecord(r domain.Repository) repositoryRecord {
	return repositoryRecord{Owner: r.Owner, Name: r.Name, Private: r.Private}
}

func (r repositoryRecord) repository() domain.Repository {
	return domain.Repository{Owner: r.Owner, Name: r.Name, Private: r.Private}
}

func (r snapshotRecord) snapshot() domain.Snapshot {
	s := domain.Snapshot{Organizations: r.Organizations, Truncated: r.Truncated, Failure: r.Failure}
	if r.ReadAt != nil {
		s.ReadAt = *r.ReadAt
	}
	for _, l := range r.Libraries {
		s.Libraries = append(s.Libraries, domain.PublishableRepository{Repository: l.repository(), Release: l.Release})
	}
	for _, p := range r.Projects {
		project := domain.Project{Repository: p.repository()}
		for _, source := range p.Sources {
			ds := domain.Source{Name: source.Name, Library: source.Library, Release: source.Release, Groups: source.Groups}
			for _, rule := range source.Rules {
				ds.Rules = append(ds.Rules, domain.PinnedRule{Path: rule.Path, Version: rule.Version})
			}
			project.Sources = append(project.Sources, ds)
		}
		s.Projects = append(s.Projects, project)
	}
	return s
}
