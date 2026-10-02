package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store/postgres"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/platform/database/databasetest"
	"github.com/fabricahq/rulemart/internal/platform/postgrestest"
)

// listingCatalog is a catalog of acme, beta, and stranger, where acme and beta are vetted, read and listed as the web
// function does, and checked as the worker does.
type listingCatalog struct {
	// web connects as the web function's role, and worker as the worker's.
	web, worker *postgres.Store
	// connString is the database's, as its owner.
	connString string
}

// newListingCatalog stores acme, beta, and stranger in a new database, as newLibraries does.
func newListingCatalog(t *testing.T) listingCatalog {
	t.Helper()
	db, connString := databasetest.New(t)
	writer := postgres.New(db)
	for _, lib := range []domain.Library{acme, beta, stranger} {
		if _, err := writer.ReplaceLibrary(context.Background(), lib); err != nil {
			t.Fatal(err)
		}
	}
	return listingCatalog{
		web:        postgres.New(databasetest.AsWebRole(t, connString)),
		worker:     postgres.New(databasetest.AsWorkerRole(t, connString)),
		connString: connString,
	}
}

// account adds an account for GitHub user githubID, and returns its ID.
func (c listingCatalog) account(t *testing.T, githubID int) int64 {
	t.Helper()
	var id int64
	postgrestest.QueryRow(t, c.connString, fmt.Sprintf(
		"INSERT INTO accounts (github_user_id, github_login, avatar_url) VALUES (%d, 'user-%d', '') RETURNING id", githubID, githubID), &id)
	return id
}

// list lists owner/name for the account, and fails t if it can't.
func (c listingCatalog) list(t *testing.T, accountID int64, owner, name string) int64 {
	t.Helper()
	id, err := c.web.CreateListing(context.Background(), vettedBoth, accountID, owner, name)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// resolve records, as the worker, that the listing names GitHub repository repositoryID.
func (c listingCatalog) resolve(t *testing.T, id int64, repositoryID string) {
	t.Helper()
	if err := c.worker.ResolveListing(context.Background(), id, domain.LibraryKey{Host: domain.GitHub, RepositoryID: repositoryID}); err != nil {
		t.Fatal(err)
	}
}

// strangerRef is how pages name the stranger/rules library.
var strangerRef = views.LibraryRef{Owner: "stranger", Name: "rules", OwnerAvatarURL: stranger.Repository.OwnerAvatarURL}

// A listed library has its own pages, which say it isn't vetted, and the unvetted list names it; nothing across
// libraries shows it: not the libraries, the groups, a group's rules, or search.
func TestAListedLibraryHasUnvettedPagesAndStaysOutOfBrowsingAndSearch(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	c.resolve(t, c.list(t, c.account(t, 1), "stranger", "rules"), "23")

	page, err := c.web.LibraryPage(ctx, vettedBoth, "Stranger", "RULES")
	if err != nil || page.Library.Vetted || page.Library.FullName() != "stranger/rules" {
		t.Fatalf("got %+v, %v; want stranger/rules, unvetted", page.Library, err)
	}
	if vettedPage, err := c.web.LibraryPage(ctx, vettedBoth, "acme", "backend"); err != nil || !vettedPage.Library.Vetted {
		t.Fatalf("got %+v, %v; want acme/backend, vetted", vettedPage.Library, err)
	}
	if rule, err := c.web.RulePage(ctx, vettedBoth, "stranger", "rules", "techs/go/use-go"); err != nil || rule.Library.Vetted {
		t.Fatalf("rule page: got %+v, %v", rule.Library, err)
	}
	if history, err := c.web.LibraryHistory(ctx, vettedBoth, "stranger", "rules"); err != nil || history.Library.Vetted {
		t.Fatalf("history: got %+v, %v", history.Library, err)
	}
	unvetted, err := c.web.UnvettedLibraries(ctx, vettedBoth)
	if err != nil || len(unvetted) != 1 || unvetted[0].Owner != "stranger" || unvetted[0].Rules != 2 {
		t.Fatalf("unvetted libraries: got %+v, %v", unvetted, err)
	}

	libraries, err := c.web.Libraries(ctx, vettedBoth)
	if err != nil || len(libraries) != 2 {
		t.Errorf("libraries: got %+v, %v; want acme and beta", libraries, err)
	}
	home, groups, err := c.web.HomePage(ctx, vettedBoth)
	if err != nil || len(home) != 2 {
		t.Errorf("home: got %+v, %v; want acme and beta", home, err)
	}
	allGroups, err := c.web.Groups(ctx, vettedBoth)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range append(groups, allGroups...) {
		if g.Library == strangerRef {
			t.Errorf("groups name the listed library: %+v", g)
		}
	}
	goRules, err := c.web.GroupRules(ctx, vettedBoth, "techs/go")
	if err != nil {
		t.Fatal(err)
	}
	for _, lib := range goRules {
		if lib.Library == strangerRef {
			t.Errorf("a group's rules name the listed library")
		}
	}
	if ids := sourceIDs(search(t, c.web, "retry")); slices.Contains(ids, "stranger/rules:practices/testing/retry-everything") {
		t.Errorf("search found the listed library's rule: %q", ids)
	}
}

// Removing a listing hides its library again, and the unvetted list forgets it, though the catalog still stores it.
func TestRemovingAListingHidesItsLibrary(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	account := c.account(t, 1)
	id := c.list(t, account, "stranger", "rules")
	c.resolve(t, id, "23")

	if err := c.web.RemoveListing(ctx, account, id); err != nil {
		t.Fatal(err)
	}

	if _, err := c.web.LibraryPage(ctx, vettedBoth, "stranger", "rules"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("library page: got %v, want store.ErrNotFound", err)
	}
	if unvetted, err := c.web.UnvettedLibraries(ctx, vettedBoth); err != nil || len(unvetted) != 0 {
		t.Errorf("unvetted libraries: got %+v, %v; want none", unvetted, err)
	}
}

// A vetted library that's also listed is vetted: its pages say so, and it isn't in the unvetted list.
func TestAVettedLibraryStaysVettedWhenListed(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	c.resolve(t, c.list(t, c.account(t, 1), "stranger", "rules"), "23")
	vettedAll := append(slices.Clone(vettedBoth), domain.LibraryKey{Host: domain.GitHub, RepositoryID: "23"})

	page, err := c.web.LibraryPage(ctx, vettedAll, "stranger", "rules")
	if err != nil || !page.Library.Vetted {
		t.Fatalf("got %+v, %v; want stranger/rules, vetted", page.Library, err)
	}
	if unvetted, err := c.web.UnvettedLibraries(ctx, vettedAll); err != nil || len(unvetted) != 0 {
		t.Errorf("unvetted libraries: got %+v, %v; want none", unvetted, err)
	}
	if libraries, err := c.web.Libraries(ctx, vettedAll); err != nil || len(libraries) != 3 {
		t.Errorf("libraries: got %+v, %v; want all three", libraries, err)
	}
}

// A repository is listed once: a listing by the same name in another case, or by the name of a library a listing
// names, or of a vetted library, is refused, naming the library when the catalog stores it.
func TestCreateListingRefusesARepositoryListedOrVettedAlready(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	first, second := c.account(t, 1), c.account(t, 2)
	// The lister gave stranger's old name; the catalog stores its library as stranger/rules.
	c.resolve(t, c.list(t, first, "stranger", "old-rules"), "23")
	c.list(t, first, "someone", "pending")

	for name, test := range map[string]struct {
		owner, name string
		want        store.ListingConflict
	}{
		"the name a listing gave, in another case": {"Stranger", "OLD-RULES", store.ListingConflict{Library: views.LibraryRef{Owner: "stranger", Name: "rules"}}},
		"the name of a library a listing names":    {"stranger", "rules", store.ListingConflict{Library: views.LibraryRef{Owner: "stranger", Name: "rules"}}},
		"a listing not checked yet":                {"someone", "pending", store.ListingConflict{}},
		"a vetted library":                         {"acme", "backend", store.ListingConflict{Vetted: true, Library: views.LibraryRef{Owner: "acme", Name: "backend"}}},
	} {
		t.Run(name, func(t *testing.T) {
			for _, check := range []func() error{
				func() error { return c.web.CheckListing(ctx, vettedBoth, second, test.owner, test.name) },
				func() error {
					_, err := c.web.CreateListing(ctx, vettedBoth, second, test.owner, test.name)
					return err
				},
			} {
				var conflict *store.ListingConflict
				if err := check(); !errors.As(err, &conflict) || *conflict != test.want {
					t.Errorf("got %v (%+v), want %+v", err, conflict, test.want)
				}
			}
		})
	}
	if err := c.web.CheckListing(ctx, vettedBoth, second, "someone", "else"); err != nil {
		t.Errorf("a new repository: got %v", err)
	}
}

// An account holds at most domain.MaxAccountListings unvetted listings; a vetted one doesn't count, and removing one
// frees its place.
func TestCreateListingKeepsAnAccountWithinItsLimit(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	account, other := c.account(t, 1), c.account(t, 2)
	var ids []int64
	for i := range domain.MaxAccountListings {
		ids = append(ids, c.list(t, account, "someone", fmt.Sprintf("rules-%d", i)))
	}

	if err := c.web.CheckListing(ctx, vettedBoth, account, "someone", "one-more"); !errors.Is(err, store.ErrAccountListingLimit) {
		t.Fatalf("check: got %v, want store.ErrAccountListingLimit", err)
	}
	if _, err := c.web.CreateListing(ctx, vettedBoth, account, "someone", "one-more"); !errors.Is(err, store.ErrAccountListingLimit) {
		t.Fatalf("create: got %v, want store.ErrAccountListingLimit", err)
	}
	c.list(t, other, "someone", "for-another-account")
	// The first listing turns out to be acme's, which is vetted.
	c.resolve(t, ids[0], "21")
	c.list(t, account, "someone", "one-more")
	if err := c.web.RemoveListing(ctx, account, ids[1]); err != nil {
		t.Fatal(err)
	}
	c.list(t, account, "someone", "another")
}

// Rulemart holds at most domain.MaxUnvettedListings unvetted listings, from every account, including deleted ones.
func TestCreateListingKeepsRulemartWithinItsLimit(t *testing.T) {
	c := newListingCatalog(t)
	postgrestest.Exec(t, c.connString, `INSERT INTO listings (host, owner, name)
		SELECT 'github', 'someone', 'rules-' || n FROM generate_series(1, $1::int) AS n`, domain.MaxUnvettedListings)

	_, err := c.web.CreateListing(context.Background(), vettedBoth, c.account(t, 1), "someone", "one-more")

	if !errors.Is(err, store.ErrListingsFull) {
		t.Fatalf("got %v, want store.ErrListingsFull", err)
	}
}

// Listings at once can't pass a limit together: the lock makes each check see the ones before it.
func TestCreateListingKeepsTheLimitUnderConcurrentListings(t *testing.T) {
	c := newListingCatalog(t)
	account := c.account(t, 1)
	var wg sync.WaitGroup
	var mu sync.Mutex
	listed, refused := 0, 0
	for i := range 3 * domain.MaxAccountListings {
		wg.Go(func() {
			_, err := c.web.CreateListing(context.Background(), vettedBoth, account, "someone", fmt.Sprintf("rules-%d", i))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				listed++
			case errors.Is(err, store.ErrAccountListingLimit):
				refused++
			default:
				t.Errorf("listing %d: %v", i, err)
			}
		})
	}
	wg.Wait()

	if listed != domain.MaxAccountListings || refused != 2*domain.MaxAccountListings {
		t.Fatalf("listed %d and refused %d, want %d and %d", listed, refused, domain.MaxAccountListings, 2*domain.MaxAccountListings)
	}
}

// An account's listings page shows each listing's state, newest first.
func TestAccountListingsShowEachListingsState(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	account, other := c.account(t, 1), c.account(t, 2)
	vettedOne := c.list(t, account, "acme", "old-backend")
	c.resolve(t, vettedOne, "21")
	listed := c.list(t, account, "stranger", "rules")
	c.resolve(t, listed, "23")
	if err := c.worker.RecordListingCheck(ctx, listed, "the repository is unreachable"); err != nil {
		t.Fatal(err)
	}
	failed := c.list(t, account, "someone", "not-a-library")
	c.resolve(t, failed, "99")
	if err := c.worker.RecordListingCheck(ctx, failed, "the repository has no release tags"); err != nil {
		t.Fatal(err)
	}
	checking := c.list(t, account, "someone", "new")
	c.list(t, other, "someone", "elses")

	got, err := c.web.AccountListings(ctx, vettedBoth, account)

	if err != nil {
		t.Fatal(err)
	}
	want := []views.AccountListing{
		{ID: checking, Owner: "someone", Name: "new", State: domain.ListingChecking},
		{ID: failed, Owner: "someone", Name: "not-a-library", RepositoryID: "99", State: domain.ListingFailed, Failure: "the repository has no release tags"},
		{ID: listed, Owner: "stranger", Name: "rules", RepositoryID: "23", State: domain.ListingListed, Library: strangerRef, Failure: "the repository is unreachable"},
		{ID: vettedOne, Owner: "acme", Name: "old-backend", RepositoryID: "21", State: domain.ListingVetted,
			Library: views.LibraryRef{Owner: "acme", Name: "backend", OwnerAvatarURL: acme.Repository.OwnerAvatarURL}},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i, w := range want {
		g := got[i]
		if g.ListedAt.IsZero() || g.CheckedAt.IsZero() != (w.Failure == "") {
			t.Errorf("listing %d: listed at %v, checked at %v", i, g.ListedAt, g.CheckedAt)
		}
		g.ListedAt, g.CheckedAt = time.Time{}, time.Time{}
		if g != w {
			t.Errorf("listing %d is %+v, want %+v", i, g, w)
		}
	}
}

// An account removes and retries only its own listings, and retries only one whose check failed.
func TestRemoveAndRetryActOnlyOnTheAccountsOwnListings(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	account, other := c.account(t, 1), c.account(t, 2)
	id := c.list(t, account, "someone", "rules")

	if err := c.web.RetryListing(ctx, account, id); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("retry before a failure: got %v, want store.ErrNotFound", err)
	}
	if err := c.worker.RecordListingCheck(ctx, id, "broken"); err != nil {
		t.Fatal(err)
	}
	if err := c.web.RetryListing(ctx, other, id); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("retry by another account: got %v, want store.ErrNotFound", err)
	}
	if err := c.web.RemoveListing(ctx, other, id); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("remove by another account: got %v, want store.ErrNotFound", err)
	}
	if err := c.web.RetryListing(ctx, account, id); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if listings, err := c.web.AccountListings(ctx, vettedBoth, account); err != nil || listings[0].State != domain.ListingChecking || listings[0].Failure != "" {
		t.Fatalf("after retrying: got %+v, %v", listings, err)
	}
	if err := c.web.RemoveListing(ctx, account, id); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := c.web.RemoveListing(ctx, account, id); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("remove again: got %v, want store.ErrNotFound", err)
	}
}

// Deleting an account keeps its listings, no longer linked to it, and their libraries listed.
func TestDeletingAnAccountKeepsItsListings(t *testing.T) {
	c := newListingCatalog(t)
	account := c.account(t, 1)
	c.resolve(t, c.list(t, account, "stranger", "rules"), "23")

	postgrestest.Exec(t, postgrestest.AsWebRole(t, c.connString), "DELETE FROM accounts WHERE id = $1", account)

	var unattributed int
	postgrestest.QueryRow(t, c.connString, "SELECT count(*) FROM listings WHERE account_id IS NULL", &unattributed)
	if unattributed != 1 {
		t.Fatalf("%d unattributed listings, want 1", unattributed)
	}
	if page, err := c.web.LibraryPage(context.Background(), vettedBoth, "stranger", "rules"); err != nil || page.Library.Vetted {
		t.Fatalf("got %+v, %v; want stranger/rules, unvetted", page.Library, err)
	}
}

// The worker reads a listing, resolves it to a repository no other listing names, and records each check.
func TestTheWorkerResolvesAListingAndRecordsItsChecks(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	account := c.account(t, 1)
	id := c.list(t, account, "Stranger", "Rules")
	other := c.list(t, account, "stranger", "renamed-rules")

	listing, found, err := c.worker.Listing(ctx, id)
	if err != nil || !found {
		t.Fatalf("got %+v, %v, %v", listing, found, err)
	}
	if listing.Resolved() || listing.Ingested || listing.FullName() != "Stranger/Rules" {
		t.Errorf("before resolving: got %+v", listing)
	}
	c.resolve(t, id, "23")
	if listing, _, _ = c.worker.Listing(ctx, id); listing.Library.RepositoryID != "23" || !listing.Ingested {
		t.Errorf("after resolving: got %+v", listing)
	}
	if err := c.worker.ResolveListing(ctx, other, domain.LibraryKey{Host: domain.GitHub, RepositoryID: "23"}); !errors.Is(err, store.ErrAlreadyListed) {
		t.Errorf("resolving another listing to the same repository: got %v, want store.ErrAlreadyListed", err)
	}
	if _, found, err := c.worker.Listing(ctx, 1_000_000); found || err != nil {
		t.Errorf("a missing listing: got found %v, %v", found, err)
	}
	if err := c.worker.RecordListingCheck(ctx, id, ""); err != nil {
		t.Fatal(err)
	}
	if listings, _ := c.web.AccountListings(ctx, vettedBoth, account); listings[1].CheckedAt.IsZero() || listings[1].Failure != "" {
		t.Errorf("after a check: got %+v", listings[1])
	}
}

// The poll checks every listing that isn't vetted, except one that failed before its library ever ingested.
func TestListingsToCheckLeaveOutVettedAndNeverIngestedFailures(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	account := c.account(t, 1)
	pending := c.list(t, account, "someone", "pending")
	vettedOne := c.list(t, account, "acme", "backend-old")
	c.resolve(t, vettedOne, "21")
	failing := c.list(t, account, "stranger", "rules")
	c.resolve(t, failing, "23")
	neverIngested := c.list(t, account, "someone", "broken")
	for _, id := range []int64{failing, neverIngested} {
		if err := c.worker.RecordListingCheck(ctx, id, "broken"); err != nil {
			t.Fatal(err)
		}
	}

	got, err := c.worker.ListingsToCheck(ctx, vettedBoth)

	if err != nil || !slices.Equal(got, []int64{pending, failing}) {
		t.Fatalf("got %v, %v; want %v", got, err, []int64{pending, failing})
	}
}
