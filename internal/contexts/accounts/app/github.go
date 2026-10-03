// Read a visitor's GitHub account: their organizations, the repositories of theirs and their organizations' that publish
// a Code Rules library, and the projects that import one, kept as the account's snapshot; and the installations of the
// GitHub App that reads their private repositories.

package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/fabricahq/rulemart/internal/contexts/accounts/domain"
	"github.com/fabricahq/rulemart/internal/contexts/accounts/store"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// GitHubReader reads a visitor's GitHub account with their OAuth token, and a private repository with an
// installation's token. accounts/github.API implements it. A read refused for the token fails with
// domain.ErrGitHubTokenRefused.
type GitHubReader interface {
	// Organizations returns the logins of the token's user's organizations, at most domain.MaxOrganizations.
	Organizations(ctx context.Context, token string) ([]string, error)
	// Repositories returns owner's public repositories, an organization's when organization is true, the most recently
	// pushed first, at most limit, and whether owner has more.
	Repositories(ctx context.Context, token, owner string, organization bool, limit int) (repos []domain.GitHubRepository, more bool, err error)
	// RootEntries returns what the root of the repository's default branch holds, nothing for one the token can't see.
	RootEntries(ctx context.Context, token string, repo domain.Repository) (domain.RootEntries, error)
	// ReleaseTags returns the names of the repository's tags that start with release/.
	ReleaseTags(ctx context.Context, token string, repo domain.Repository) ([]string, error)
	// File returns the file at path in the repository's default branch, at most maxBytes, or found false.
	File(ctx context.Context, token string, repo domain.Repository, path string, maxBytes int) ([]byte, bool, error)
	// OrganizationRole returns the token's user's role in org, admin or member, or empty when they aren't a member.
	OrganizationRole(ctx context.Context, token, org string) (string, error)
}

// GitHubApp acts as the GitHub App that reads private repositories. accounts/github.App implements it.
type GitHubApp interface {
	// InstallURL returns the GitHub page where a visitor installs the app.
	InstallURL() string
	// Installation returns the account installation id is on, or fails with domain.ErrNoSuchInstallation.
	Installation(ctx context.Context, id int64) (domain.InstallationAccount, error)
	// InstallationToken returns a token that reads what installation id may, or fails with
	// domain.ErrNoSuchInstallation.
	InstallationToken(ctx context.Context, id int64) (string, error)
	// InstallationRepositories returns the private repositories an installation's token reads, the most recently
	// pushed first, at most limit, and whether it left some out.
	InstallationRepositories(ctx context.Context, token string, limit int) (repos []domain.GitHubRepository, more bool, err error)
	// WebhookChange returns the change to an installation a webhook delivery reports, after checking its signature. It
	// fails with domain.ErrBadSignature for a delivery GitHub didn't sign, and domain.ErrIgnoredEvent for one that changes
	// nothing Rulemart keeps.
	WebhookChange(ctx context.Context, event string, body []byte, signature string) (domain.InstallationChange, error)
}

// ErrNoApp reports that Rulemart has no GitHub App to read private repositories with.
var ErrNoApp = errors.New("Rulemart has no GitHub App to read private repositories with")

// ErrNotYourInstallation reports an installation of the GitHub App that is on neither the visitor's own account nor an
// organization they own, or that GitHub doesn't know.
var ErrNotYourInstallation = errors.New("the installation isn't on the visitor's account or an organization they own")

// ErrGitHubRead reports a read of a visitor's GitHub account that failed, such as when GitHub didn't answer: the
// snapshot returned with it says so, and keeps what an earlier read found.
var ErrGitHubRead = errors.New("Rulemart couldn't read the visitor's GitHub account")

// readTimeout bounds one read of a visitor's GitHub account, well within the web function's own timeout.
const readTimeout = 25 * time.Second

// readConcurrency is how many requests one read sends GitHub at once: enough that a read of domain.MaxRepositories
// repositories takes seconds, few enough to stay within GitHub's limits on concurrent requests.
const readConcurrency = 8

// GitHubAccounts reads visitors' GitHub accounts, keeps what it found as each account's snapshot, and records the
// installations of the GitHub App they read private repositories through.
type GitHubAccounts struct {
	Store store.Store
	// Sessions opens the GitHub token each session keeps.
	Sessions Sessions
	GitHub   GitHubReader
	// App reads private repositories where visitors install it; nil reads public repositories only.
	App GitHubApp
	// Now returns the time; nil is time.Now.
	Now func() time.Time
}

// PrivateRepositories reports whether visitors can let Rulemart read their private repositories: whether there's a
// GitHub App to install.
func (g GitHubAccounts) PrivateRepositories() bool { return g.App != nil }

// InstallURL returns the GitHub page where a visitor installs the GitHub App, or empty without one.
func (g GitHubAccounts) InstallURL() string {
	if g.App == nil {
		return ""
	}
	return g.App.InstallURL()
}

// Snapshot returns the account's snapshot, reading the visitor's GitHub account with session's token first when
// Rulemart has none, such as after signing in. It fails with ErrNoGitHubToken when the session keeps no token GitHub
// takes, and with an error wrapping ErrGitHubRead, beside the snapshot that says so, when the read failed.
func (g GitHubAccounts) Snapshot(ctx context.Context, account domain.Account, session domain.SessionToken) (domain.Snapshot, error) {
	snapshot, found, err := g.Store.Snapshot(ctx, account.ID)
	if err != nil || found {
		return snapshot, err
	}
	return g.read(ctx, account, session, domain.Snapshot{})
}

// Refresh reads the visitor's GitHub account again, unless a read began within domain.RefreshInterval, and returns the
// account's snapshot, failing as Snapshot does.
func (g GitHubAccounts) Refresh(ctx context.Context, account domain.Account, session domain.SessionToken) (domain.Snapshot, error) {
	snapshot, _, err := g.Store.Snapshot(ctx, account.ID)
	if err != nil {
		return domain.Snapshot{}, err
	}
	return g.read(ctx, account, session, snapshot)
}

// read reads the visitor's GitHub account with session's token, and keeps what it found as the account's snapshot,
// unless a read began within domain.RefreshInterval since the snapshot was last discarded, claimed before contacting
// GitHub so that requests arriving together read it once; then it returns the snapshot kept. When
// the read fails for a reason other than the token, it keeps previous, saying so, so pages show what an earlier read
// found and when, and the next read waits domain.RefreshInterval. When the account's access changes while it reads,
// such as when the visitor removes access, it reads again, at most maxReads times in all, without previous, which may
// hold what the visitor can no longer see.
func (g GitHubAccounts) read(ctx context.Context, account domain.Account, session domain.SessionToken, previous domain.Snapshot) (domain.Snapshot, error) {
	token, err := g.Sessions.GitHubToken(ctx, session)
	if err != nil {
		return previous, err
	}
	claimed, err := g.Store.ClaimRead(ctx, account.ID, g.now(), domain.RefreshInterval)
	if err != nil {
		return previous, err
	}
	if !claimed {
		// Another request read GitHub within the minute, or is reading it now: show what it kept, if it has finished.
		snapshot, _, err := g.Store.Snapshot(ctx, account.ID)
		return snapshot, err
	}
	for range maxReads - 1 {
		snapshot, err := g.readOnce(ctx, token, account, previous)
		if !errors.Is(err, errAccessChanged) {
			return snapshot, err
		}
		previous = domain.Snapshot{}
	}
	snapshot, err := g.readOnce(ctx, token, account, previous)
	if errors.Is(err, errAccessChanged) {
		return domain.Snapshot{ReadFailed: true}, fmt.Errorf("read GitHub accountID=%d: %w: %v", account.ID, ErrGitHubRead, err)
	}
	return snapshot, err
}

// maxReads bounds how many times read reads GitHub while the account's access keeps changing.
const maxReads = 3

// errAccessChanged reports that the account's access to private repositories changed while a read was under way, so
// what it read may hold what the visitor can no longer see, and it kept nothing.
var errAccessChanged = errors.New("the account's access to private repositories changed during the read")

// readOnce is one attempt of read, with the visitor's GitHub token. It notes the account's GitHub generation before
// anything else, and keeps what it read, or previous after a failure, only while the generation is unchanged; otherwise,
// or when it forgets an installation itself, it fails with errAccessChanged.
func (g GitHubAccounts) readOnce(ctx context.Context, token string, account domain.Account, previous domain.Snapshot) (domain.Snapshot, error) {
	generation, err := g.Store.GitHubGeneration(ctx, account.ID)
	if err != nil {
		return previous, err
	}
	installations, err := g.Store.Installations(ctx, account.ID)
	if err != nil {
		return previous, err
	}
	now := g.now()
	installations, err = g.stillPermitted(ctx, token, account, installations)
	var snapshot domain.Snapshot
	if err == nil {
		snapshot, err = g.scan(ctx, token, account.Login, installations)
	}
	if errors.Is(err, errAccessChanged) {
		return domain.Snapshot{}, err
	}
	if errors.Is(err, domain.ErrGitHubTokenRefused) {
		return previous, ErrNoGitHubToken
	}
	if err != nil {
		previous.ReadFailed = true
		saved, saveErr := g.Store.SaveSnapshot(ctx, account.ID, generation, previous)
		if saveErr != nil {
			return previous, saveErr
		}
		if !saved {
			return domain.Snapshot{}, errAccessChanged
		}
		return previous, fmt.Errorf("read GitHub accountID=%d: %w: %v", account.ID, ErrGitHubRead, err)
	}
	snapshot.ReadAt = now
	saved, err := g.Store.SaveSnapshot(ctx, account.ID, generation, snapshot)
	if err != nil {
		return previous, err
	}
	if !saved {
		return domain.Snapshot{}, errAccessChanged
	}
	return snapshot, nil
}

// stillPermitted returns installations, checking with the visitor's own token that they still own each organization one
// is on, as Install checked when they added it. A former owner, demoted or gone from the organization, is no longer
// permitted, so the installation is forgotten for their account, with its snapshot, and it fails with
// errAccessChanged.
func (g GitHubAccounts) stillPermitted(ctx context.Context, token string, account domain.Account, installations []domain.Installation) ([]domain.Installation, error) {
	revoked := false
	for _, installation := range installations {
		if g.App == nil || strings.EqualFold(installation.Account, account.Login) {
			continue
		}
		role, err := g.GitHub.OrganizationRole(ctx, token, installation.Account)
		if err != nil {
			return nil, err
		}
		if role == "admin" {
			continue
		}
		if err := g.Store.RemoveInstallation(ctx, account.ID, installation.ID); err != nil {
			return nil, err
		}
		revoked = true
	}
	if revoked {
		return nil, errAccessChanged
	}
	return installations, nil
}

// scan reads what the snapshot holds: login's organizations, then the public repositories of login and each
// organization, and the private repositories each installation reads, the domain.MaxRepositories most recently pushed of
// them, and in each, whether it publishes a library and whether it's a project. An installation GitHub no longer knows
// is forgotten, and the scan fails with errAccessChanged.
func (g GitHubAccounts) scan(ctx context.Context, token, login string, installations []domain.Installation) (domain.Snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	orgs, err := g.GitHub.Organizations(ctx, token)
	if err != nil {
		return domain.Snapshot{}, err
	}
	candidates, tokens, more, err := g.candidates(ctx, token, login, orgs, installations)
	if err != nil {
		return domain.Snapshot{}, err
	}
	snapshot := domain.Snapshot{Organizations: orgs, Truncated: more || len(candidates) > domain.MaxRepositories}
	candidates = candidates[:min(len(candidates), domain.MaxRepositories)]
	libraries := make([]*domain.PublishableRepository, len(candidates))
	projects := make([]*domain.Project, len(candidates))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(readConcurrency)
	for i, repo := range candidates {
		group.Go(func() error {
			var err error
			libraries[i], projects[i], err = g.inspect(groupCtx, tokens[repo.Installation], repo.Repository)
			return err
		})
	}
	if err := group.Wait(); err != nil {
		return domain.Snapshot{}, err
	}
	for i := range candidates {
		if libraries[i] != nil {
			snapshot.Libraries = append(snapshot.Libraries, *libraries[i])
		}
		if projects[i] != nil {
			snapshot.Projects = append(snapshot.Projects, *projects[i])
		}
	}
	slices.SortFunc(snapshot.Libraries, func(a, b domain.PublishableRepository) int { return compareRepositories(a.Repository, b.Repository) })
	slices.SortFunc(snapshot.Projects, func(a, b domain.Project) int { return compareRepositories(a.Repository, b.Repository) })
	return snapshot, nil
}

// candidates returns the repositories a scan may look into, each once, the most recently pushed first: the public
// repositories login and orgs own, then the private repositories installations read, with the token that reads each by
// its installation, 0 for the visitor's own token, and whether a listing left some out.
func (g GitHubAccounts) candidates(ctx context.Context, token, login string, orgs []string, installations []domain.Installation) (all []domain.GitHubRepository, tokens map[int64]string, more bool, err error) {
	owners := append([]string{login}, orgs...)
	lists := make([][]domain.GitHubRepository, len(owners)+len(installations))
	tokens = map[int64]string{0: token}
	var mu sync.Mutex
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(readConcurrency)
	for i, owner := range owners {
		group.Go(func() error {
			repos, left, err := g.GitHub.Repositories(groupCtx, token, owner, i > 0, domain.MaxRepositories)
			mu.Lock()
			more = more || left
			mu.Unlock()
			lists[i] = repos
			return err
		})
	}
	if g.App != nil {
		for i, installation := range installations {
			group.Go(func() error {
				repos, installationToken, left, err := g.installationRepositories(groupCtx, installation)
				mu.Lock()
				tokens[installation.ID] = installationToken
				more = more || left
				mu.Unlock()
				lists[len(owners)+i] = repos
				return err
			})
		}
	}
	if err := group.Wait(); err != nil {
		return nil, nil, false, err
	}
	seen := map[string]bool{}
	for _, list := range lists {
		for _, repo := range list {
			if key := strings.ToLower(repo.FullName()); !seen[key] {
				seen[key] = true
				all = append(all, repo)
			}
		}
	}
	slices.SortStableFunc(all, func(a, b domain.GitHubRepository) int { return b.PushedAt.Compare(a.PushedAt) })
	return all, tokens, more, nil
}

// installationRepositories returns the private repositories installation reads, the domain.MaxRepositories most
// recently pushed, each marked with it, the token that reads them, and whether it left some out. An installation GitHub
// no longer knows is forgotten, and the read fails with errAccessChanged, to read again without it.
func (g GitHubAccounts) installationRepositories(ctx context.Context, installation domain.Installation) ([]domain.GitHubRepository, string, bool, error) {
	token, err := g.App.InstallationToken(ctx, installation.ID)
	if errors.Is(err, domain.ErrNoSuchInstallation) {
		if err := g.Store.InstallationRemoved(ctx, installation.ID); err != nil {
			return nil, "", false, err
		}
		return nil, "", false, errAccessChanged
	}
	if err != nil {
		return nil, "", false, err
	}
	repos, more, err := g.App.InstallationRepositories(ctx, token, domain.MaxRepositories)
	if err != nil {
		return nil, "", false, err
	}
	for i := range repos {
		repos[i].Installation = installation.ID
	}
	return repos, token, more, nil
}

// inspect returns repo as a library when its root holds rule-library.yaml and it has a library release tag, and as a
// project when its provenance file parses, reading it with token. A provenance file Rulemart can't parse makes no
// project, rather than failing the read.
func (g GitHubAccounts) inspect(ctx context.Context, token string, repo domain.Repository) (*domain.PublishableRepository, *domain.Project, error) {
	root, err := g.GitHub.RootEntries(ctx, token, repo)
	if err != nil {
		return nil, nil, err
	}
	var library *domain.PublishableRepository
	if root.Manifest {
		tags, err := g.GitHub.ReleaseTags(ctx, token, repo)
		if err != nil {
			return nil, nil, err
		}
		if release := latestRelease(tags); release > 0 {
			library = &domain.PublishableRepository{Repository: repo, Release: release}
		}
	}
	var project *domain.Project
	if root.CodeRules {
		data, found, err := g.GitHub.File(ctx, token, repo, domain.ProvenancePath, domain.MaxProvenanceBytes)
		if err != nil {
			return nil, nil, err
		}
		if sources, err := domain.ParseProvenance(data); found && err == nil {
			project = &domain.Project{Repository: repo, Sources: sources}
		}
	}
	return library, project, nil
}

// latestRelease returns the largest library release number tags name, or 0 when none is a release tag.
func latestRelease(tags []string) int {
	latest := 0
	for _, tag := range tags {
		if n, err := coderules.ParseReleaseTag(tag); err == nil {
			latest = max(latest, n)
		}
	}
	return latest
}

// compareRepositories orders repositories by owner and name, without regard to case.
func compareRepositories(a, b domain.Repository) int {
	return cmp.Compare(strings.ToLower(a.FullName()), strings.ToLower(b.FullName()))
}

// Installations returns the installations of the GitHub App the account reads private repositories through.
func (g GitHubAccounts) Installations(ctx context.Context, accountID int64) ([]domain.Installation, error) {
	return g.Store.Installations(ctx, accountID)
}

// Install records that the account reads private repositories through installation id, which GitHub returned the
// visitor to Rulemart with, once GitHub confirms it's on the visitor's own account, or on an organization the visitor
// owns, as only an owner can install an app on all of an organization's repositories. Then it reads the visitor's GitHub
// account again, with the repositories the installation reads, as Refresh does. It fails with ErrNoApp without a GitHub
// App, and ErrNotYourInstallation for an installation on another account, or one GitHub doesn't know.
func (g GitHubAccounts) Install(ctx context.Context, account domain.Account, session domain.SessionToken, id int64) (domain.Snapshot, error) {
	if g.App == nil {
		return domain.Snapshot{}, ErrNoApp
	}
	on, err := g.App.Installation(ctx, id)
	if errors.Is(err, domain.ErrNoSuchInstallation) {
		return domain.Snapshot{}, fmt.Errorf("install installationID=%d: %w", id, ErrNotYourInstallation)
	}
	if err != nil {
		return domain.Snapshot{}, err
	}
	if err := g.checkInstalledBy(ctx, account, session, on); err != nil {
		return domain.Snapshot{}, fmt.Errorf("install installationID=%d: %w", id, err)
	}
	if err := g.Store.AddInstallation(ctx, account.ID, domain.Installation{ID: id, Account: on.Login}); err != nil {
		return domain.Snapshot{}, err
	}
	return g.read(ctx, account, session, domain.Snapshot{})
}

// checkInstalledBy reports whether the visitor may read through an installation on the account on: it's their own, or
// an organization they own, as their own token tells. It fails with ErrNotYourInstallation otherwise.
func (g GitHubAccounts) checkInstalledBy(ctx context.Context, account domain.Account, session domain.SessionToken, on domain.InstallationAccount) error {
	if !on.Organization {
		if on.ID != account.GitHubUserID {
			return ErrNotYourInstallation
		}
		return nil
	}
	token, err := g.Sessions.GitHubToken(ctx, session)
	if err != nil {
		return err
	}
	role, err := g.GitHub.OrganizationRole(ctx, token, on.Login)
	if errors.Is(err, domain.ErrGitHubTokenRefused) {
		return ErrNoGitHubToken
	}
	if err != nil {
		return err
	}
	if role != "admin" {
		return ErrNotYourInstallation
	}
	return nil
}

// ForgetInstallations stops reading private repositories for the account: it forgets every installation it reads
// through and discards its snapshot, so its next page reads only what its own token can. The app stays installed on
// GitHub until the visitor uninstalls it there.
func (g GitHubAccounts) ForgetInstallations(ctx context.Context, accountID int64) error {
	return g.Store.RemoveInstallations(ctx, accountID)
}

// Deliver acts on a delivery of the GitHub App's webhook: an installation removed is forgotten for every account, and
// one whose repositories changed discards their snapshots, so their next page reads GitHub again. It fails with
// ErrNoApp without a GitHub App, and as GitHubApp.WebhookChange does for a delivery it doesn't act on.
func (g GitHubAccounts) Deliver(ctx context.Context, event string, body []byte, signature string) error {
	if g.App == nil {
		return ErrNoApp
	}
	change, err := g.App.WebhookChange(ctx, event, body, signature)
	if err != nil {
		return err
	}
	if change.Removed {
		return g.Store.InstallationRemoved(ctx, change.ID)
	}
	return g.Store.InstallationChanged(ctx, change.ID)
}

func (g GitHubAccounts) now() time.Time {
	if g.Now == nil {
		return time.Now()
	}
	return g.Now()
}
