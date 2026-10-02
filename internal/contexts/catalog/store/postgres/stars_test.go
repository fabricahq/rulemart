package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
	"github.com/fabricahq/rulemart/internal/platform/postgrestest"
)

const (
	acmeRetry  = "practices/testing/verify-retry-limits"
	acmeErrors = "techs/go/return-errors"
)

// star stars the rule at rulePath of owner/name for the account as the web function does, and fails t if it can't.
func (c listingCatalog) star(t *testing.T, vetted []domain.LibraryKey, accountID int64, owner, name, rulePath string) {
	t.Helper()
	if _, err := c.web.Star(context.Background(), vetted, accountID, owner, name, rulePath); err != nil {
		t.Fatalf("star %s/%s/%s for account %d: %v", owner, name, rulePath, accountID, err)
	}
}

// starAsOwner stars the rule at rulePath of the library with the GitHub repository ID for the account, as the
// database's owner, as if the account starred it before a release retired it.
func (c listingCatalog) starAsOwner(t *testing.T, accountID int64, repositoryID, rulePath string) {
	t.Helper()
	postgrestest.Exec(t, c.connString, `INSERT INTO rule_stars (account_id, rule_id)
		SELECT $1, r.id FROM rules r JOIN libraries l ON l.id = r.library_id WHERE l.host_repository_id = $2 AND r.path = $3`,
		accountID, repositoryID, rulePath)
}

// ruleStars returns how many stars the library owner/name's page counts on each of its current rules, by path.
func (c listingCatalog) ruleStars(t *testing.T, vetted []domain.LibraryKey, owner, name string) map[string]int {
	t.Helper()
	page, err := c.web.LibraryPage(context.Background(), vetted, owner, name)
	if err != nil {
		t.Fatal(err)
	}
	stars := map[string]int{}
	for _, r := range page.Rules {
		stars[r.Path] = r.Stars
	}
	return stars
}

// starredRules returns the account's stars as AccountStars lists them: each rule as owner/name/path, then " as " and
// the rule the account starred in its place, if any.
func (c listingCatalog) starredRules(t *testing.T, vetted []domain.LibraryKey, accountID int64) []string {
	t.Helper()
	starred, err := c.web.AccountStars(context.Background(), vetted, accountID)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, s := range starred {
		entry := s.Library.FullName() + "/" + s.Rule.Path
		if s.StarredAs != "" {
			entry += " as " + s.StarredAs
		}
		if s.StarredAt.IsZero() {
			t.Errorf("%s has no star time", entry)
		}
		got = append(got, entry)
	}
	return got
}

// chainLibrary returns a library at GitHub repository id, owner/rules, of two releases, whose n rules techs/go/r00,
// techs/go/r01, and so on each replaced the one before: release 2 retired every rule but the last, which is current,
// each replaced by the next. It also holds techs/go/dropped, which release 2 retired with no replacement.
func chainLibrary(id, owner string, n int) domain.Library {
	path := func(i int) string { return fmt.Sprintf("techs/go/r%02d", i) }
	retired := func(path, replacedBy string) domain.Rule {
		return domain.Rule{Path: path, Group: "techs/go", RetiredIn: 2, ReplacedBy: replacedBy,
			RetirementSummaries: []string{"Retire it."},
			Versions:            []domain.Version{{Number: v(1, 0, 0), Release: 1, Change: coderules.ChangeNew, Summaries: []string{"Add the rule."}}}}
	}
	rules := []domain.Rule{retired("techs/go/dropped", "")}
	for i := range n - 1 {
		rules = append(rules, retired(path(i), path(i+1)))
	}
	rules = append(rules, rule(path(n-1), "Return errors", "When a function fails.", "Wrap each error."))
	lib := newLibrary(id, owner, "rules", []domain.Group{goGroup}, rules...)
	lib.Releases = append(lib.Releases, domain.Release{Number: 2, CommitID: strings.Repeat("2", 40), TaggedAt: day(2)})
	return lib
}

// An account stars a current rule of a vetted library once, by any spelling of the library and the rule, and every
// read that lists the rule counts each account's star.
func TestAnAccountStarsACurrentRuleOnce(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	first, second, third := c.account(t, 1), c.account(t, 2), c.account(t, 3)

	if _, err := c.web.Star(ctx, vettedBoth, first, "ACME", "Backend", "Techs/Go/Return-Errors"); err != nil {
		t.Fatal(err)
	}
	c.star(t, vettedBoth, first, "acme", "backend", acmeErrors)
	if got := c.ruleStars(t, vettedBoth, "acme", "backend"); got[acmeErrors] != 1 || got[acmeRetry] != 0 {
		t.Fatalf("after one account starred return-errors twice: %v; want one star on it", got)
	}
	c.star(t, vettedBoth, second, "acme", "backend", acmeErrors)

	if got := c.ruleStars(t, vettedBoth, "acme", "backend"); got[acmeErrors] != 2 {
		t.Errorf("the library's page counts %v, want two stars on return-errors", got)
	}
	page, err := c.web.RulePage(ctx, vettedBoth, "acme", "backend", acmeErrors)
	if err != nil || page.Rule.Stars != 2 {
		t.Errorf("the rule's page counts %d, %v; want 2", page.Rule.Stars, err)
	}
	groups, err := c.web.GroupRules(ctx, vettedBoth, "techs/go")
	if err != nil || len(groups) == 0 || groups[0].Rules[0].Path != acmeErrors || groups[0].Rules[0].Stars != 2 {
		t.Errorf("the group's page lists %+v, %v; want acme's return-errors with two stars", groups, err)
	}
	for _, r := range search(t, c.web, "errors").Results {
		if want := map[bool]int{true: 2, false: 0}[r.Rule.Path == acmeErrors]; r.Rule.Stars != want {
			t.Errorf("search counts %d stars on %s, want %d", r.Rule.Stars, r.Rule.Path, want)
		}
	}
	for account, want := range map[int64]bool{first: true, second: true, third: false} {
		if starred, err := c.web.Starred(ctx, account, "Acme", "BACKEND", "techs/go/RETURN-errors"); err != nil || starred != want {
			t.Errorf("account %d: starred %v, %v; want %v", account, starred, err, want)
		}
	}
}

// A star is the account's first when the account had none and now has it: not a repeat of it, not a second star, and
// again once unstarring leaves the account none. Another account's stars don't count.
func TestAStarIsFirstOnlyWhenTheAccountHadNone(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	account, other := c.account(t, 1), c.account(t, 2)
	c.star(t, vettedBoth, other, "acme", "backend", acmeRetry)

	for _, step := range []struct {
		what  string
		star  bool
		rule  string
		first bool
	}{
		{"the first star", true, acmeErrors, true},
		{"starring it again", true, acmeErrors, false},
		{"a second star", true, acmeRetry, false},
		{"unstarring the first", false, acmeErrors, false},
		{"starring the first again beside the second", true, acmeErrors, false},
		{"unstarring the first", false, acmeErrors, false},
		{"unstarring the second", false, acmeRetry, false},
		{"a star after unstarring every one", true, acmeErrors, true},
	} {
		if !step.star {
			if err := c.web.Unstar(ctx, vettedBoth, account, "acme", "backend", step.rule); err != nil {
				t.Fatalf("%s: %v", step.what, err)
			}
			continue
		}
		first, err := c.web.Star(ctx, vettedBoth, account, "acme", "backend", step.rule)
		if err != nil || first != step.first {
			t.Errorf("%s: first %v, %v; want %v", step.what, first, err, step.first)
		}
	}
}

// Only a current rule of a vetted library can be starred or unstarred: not a retired rule, a rule of a listed
// library, or a library or rule the catalog doesn't have.
func TestOnlyACurrentRuleOfAVettedLibraryCanBeStarred(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	account := c.account(t, 1)
	c.resolve(t, c.list(t, account, "stranger", "rules"), "23")

	for _, rule := range [][3]string{
		{"acme", "backend", "practices/testing/retry-forever"},
		{"stranger", "rules", "techs/go/use-go"},
		{"nobody", "nothing", acmeErrors},
		{"acme", "backend", "techs/go/missing"},
		{"acme", "backend", ""},
	} {
		if _, err := c.web.Star(ctx, vettedBoth, account, rule[0], rule[1], rule[2]); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("star %s/%s/%s: got %v, want ErrNotFound", rule[0], rule[1], rule[2], err)
		}
		if err := c.web.Unstar(ctx, vettedBoth, account, rule[0], rule[1], rule[2]); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("unstar %s/%s/%s: got %v, want ErrNotFound", rule[0], rule[1], rule[2], err)
		}
	}
	var stars int
	postgrestest.QueryRow(t, c.connString, `SELECT count(*) FROM rule_stars`, &stars)
	if stars != 0 {
		t.Fatalf("%d stars stored, want none", stars)
	}
}

// A library that loses its vetting but stays listed keeps its stars stored, uncounted: its page and its rules' pages
// count none, and once it's vetted again, they count again.
func TestAListedLibraryCountsNoStarsUntilItsVettedAgain(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	account := c.account(t, 1)
	c.star(t, vettedBoth, account, "acme", "backend", acmeErrors)
	onlyBeta := vettedBoth[1:]
	listing, err := c.web.CreateListing(ctx, onlyBeta, account, "acme", "backend")
	if err != nil {
		t.Fatal(err)
	}
	c.resolve(t, listing, "21")

	if got := c.ruleStars(t, onlyBeta, "acme", "backend"); got[acmeErrors] != 0 {
		t.Errorf("unvetted, the library's page counts %v, want no stars", got)
	}
	page, err := c.web.RulePage(ctx, onlyBeta, "acme", "backend", acmeErrors)
	if err != nil || page.Library.Vetted || page.Rule.Stars != 0 {
		t.Errorf("unvetted, the rule's page counts %d, vetted %v, %v; want 0, unvetted", page.Rule.Stars, page.Library.Vetted, err)
	}

	if got := c.ruleStars(t, vettedBoth, "acme", "backend"); got[acmeErrors] != 1 {
		t.Errorf("vetted again, the library's page counts %v, want one star on return-errors", got)
	}
	if page, err := c.web.RulePage(ctx, vettedBoth, "acme", "backend", acmeErrors); err != nil || page.Rule.Stars != 1 {
		t.Errorf("vetted again, the rule's page counts %d, %v; want 1", page.Rule.Stars, err)
	}
}

// Unstarring removes only the account's own star, and unstarring what it hasn't starred changes nothing.
func TestUnstarringRemovesOnlyTheAccountsOwnStar(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	first, second := c.account(t, 1), c.account(t, 2)
	c.star(t, vettedBoth, first, "acme", "backend", acmeErrors)
	c.star(t, vettedBoth, second, "acme", "backend", acmeErrors)
	c.star(t, vettedBoth, first, "Beta", "rules", "techs/go/name-packages-plainly")

	for range 2 {
		if err := c.web.Unstar(ctx, vettedBoth, second, "ACME", "backend", acmeErrors); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.web.Unstar(ctx, vettedBoth, second, "beta", "rules", "techs/go/name-packages-plainly"); err != nil {
		t.Fatal(err)
	}

	if got := c.ruleStars(t, vettedBoth, "acme", "backend"); got[acmeErrors] != 1 {
		t.Errorf("acme counts %v, want the first account's star on return-errors", got)
	}
	if got := c.ruleStars(t, vettedBoth, "Beta", "rules"); got["techs/go/name-packages-plainly"] != 1 {
		t.Errorf("Beta counts %v, want the first account's star", got)
	}
	if starred, err := c.web.Starred(ctx, second, "acme", "backend", acmeErrors); err != nil || starred {
		t.Errorf("the second account's star: %v, %v; want it gone", starred, err)
	}
}

// A star stays on the rule it was given to, and once a release retires the rule, counts toward the current rule its
// chain of replacements reaches. Each account counts once however many rules of the chain it starred, and unstarring
// the current rule takes away every star of the account's that counts toward it. A star on a rule retired without a
// replacement counts toward nothing.
func TestStarsOnRetiredRulesCountTowardTheirReplacement(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	if _, err := c.worker.ReplaceLibrary(ctx, chainLibrary("24", "chain", 3)); err != nil {
		t.Fatal(err)
	}
	vetted := append([]domain.LibraryKey{{Host: domain.GitHub, RepositoryID: "24"}}, vettedBoth...)
	first, second, third := c.account(t, 1), c.account(t, 2), c.account(t, 3)
	c.starAsOwner(t, first, "24", "techs/go/r00")
	c.star(t, vetted, first, "chain", "rules", "techs/go/r02")
	c.starAsOwner(t, second, "24", "techs/go/r01")
	c.starAsOwner(t, third, "24", "techs/go/dropped")

	if got := c.ruleStars(t, vetted, "chain", "rules"); got["techs/go/r02"] != 2 {
		t.Fatalf("the current rule counts %v, want both accounts once", got)
	}
	for account, want := range map[int64]bool{first: true, second: true, third: false} {
		if starred, err := c.web.Starred(ctx, account, "chain", "rules", "techs/go/r02"); err != nil || starred != want {
			t.Errorf("account %d: starred %v, %v; want %v", account, starred, err, want)
		}
	}
	for account, want := range map[int64][]string{
		first:  {"chain/rules/techs/go/r02"},
		second: {"chain/rules/techs/go/r02 as techs/go/r01"},
		third:  {},
	} {
		if got := c.starredRules(t, vetted, account); !slices.Equal(got, want) {
			t.Errorf("account %d lists %q, want %q", account, got, want)
		}
	}

	if err := c.web.Unstar(ctx, vetted, second, "chain", "rules", "techs/go/r02"); err != nil {
		t.Fatal(err)
	}
	if got := c.ruleStars(t, vetted, "chain", "rules"); got["techs/go/r02"] != 1 {
		t.Errorf("after the second account unstarred: %v, want only the first's", got)
	}
	if got := c.starredRules(t, vetted, second); len(got) != 0 {
		t.Errorf("the second account still lists %q", got)
	}
}

// A star counts toward the current rule its chain reaches within domain.MaxReplacements rules, as pages follow a
// chain, and no further: it reads as starred, lists, and is unstarred from the current rule only within that bound,
// and beyond it stays stored.
func TestAStarCountsOnlyWithinTheReplacementsPagesFollow(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	if _, err := c.worker.ReplaceLibrary(ctx, chainLibrary("24", "chain", domain.MaxReplacements+2)); err != nil {
		t.Fatal(err)
	}
	vetted := append([]domain.LibraryKey{{Host: domain.GitHub, RepositoryID: "24"}}, vettedBoth...)
	within, beyond := c.account(t, 1), c.account(t, 2)
	current := fmt.Sprintf("techs/go/r%02d", domain.MaxReplacements+1)
	c.starAsOwner(t, within, "24", "techs/go/r01")
	c.starAsOwner(t, beyond, "24", "techs/go/r00")

	if got := c.ruleStars(t, vetted, "chain", "rules"); got[current] != 1 {
		t.Errorf("the current rule counts %d stars, want only the one %d replacements away", got[current], domain.MaxReplacements)
	}
	if got := c.starredRules(t, vetted, within); !slices.Equal(got, []string{"chain/rules/" + current + " as techs/go/r01"}) {
		t.Errorf("the star within reach lists %q", got)
	}
	if got := c.starredRules(t, vetted, beyond); len(got) != 0 {
		t.Errorf("the star beyond reach lists %q", got)
	}
	for account, want := range map[int64]bool{within: true, beyond: false} {
		if starred, err := c.web.Starred(ctx, account, "chain", "rules", current); err != nil || starred != want {
			t.Errorf("account %d: starred %v, %v; want %v", account, starred, err, want)
		}
	}

	for _, account := range []int64{within, beyond} {
		if err := c.web.Unstar(ctx, vetted, account, "chain", "rules", current); err != nil {
			t.Fatal(err)
		}
	}
	if got := c.ruleStars(t, vetted, "chain", "rules"); got[current] != 0 {
		t.Errorf("after unstarring, the current rule counts %d stars, want none", got[current])
	}
	for account, want := range map[int64]int{within: 0, beyond: 1} {
		var stored int
		postgrestest.QueryRow(t, c.connString, fmt.Sprintf(`SELECT count(*) FROM rule_stars WHERE account_id = %d`, account), &stored)
		if stored != want {
			t.Errorf("after unstarring, account %d keeps %d stars, want %d", account, stored, want)
		}
	}
}

// An account's stars list the rules they count toward, most recently starred first, each with its stars, and leave
// out a rule of a library the release no longer vets.
func TestAccountStarsListTheAccountsRulesNewestFirst(t *testing.T) {
	c := newListingCatalog(t)
	account, other := c.account(t, 1), c.account(t, 2)
	everyone := append([]domain.LibraryKey{{Host: domain.GitHub, RepositoryID: "23"}}, vettedBoth...)
	c.star(t, everyone, account, "acme", "backend", acmeErrors)
	c.star(t, everyone, account, "stranger", "rules", "techs/go/use-go")
	c.star(t, everyone, account, "Beta", "rules", "techs/go/name-packages-plainly")
	c.star(t, everyone, account, "acme", "backend", acmeRetry)
	c.star(t, everyone, other, "acme", "backend", acmeErrors)
	// Star return-errors last, then Beta's rule, then verify-retry-limits, whatever order the statements ran in.
	postgrestest.Exec(t, c.connString, `UPDATE rule_stars SET created_at = now() - CASE
		(SELECT r.path FROM rules r WHERE r.id = rule_stars.rule_id)
		WHEN 'techs/go/return-errors' THEN interval '1 minute' WHEN 'techs/go/name-packages-plainly' THEN interval '2 minutes'
		ELSE interval '3 minutes' END
		WHERE account_id = $1`, account)

	if got, want := c.starredRules(t, vettedBoth, account), []string{
		"acme/backend/" + acmeErrors, "Beta/rules/techs/go/name-packages-plainly", "acme/backend/" + acmeRetry,
	}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	starred, err := c.web.AccountStars(context.Background(), vettedBoth, account)
	if err != nil {
		t.Fatal(err)
	}
	if want := (views.StarredRule{
		Library: views.LibraryRef{Owner: "acme", Name: "backend", OwnerAvatarURL: acme.Repository.OwnerAvatarURL},
		Rule: views.RuleCard{Path: acmeErrors, Group: "techs/go", Title: "Return errors with context", Impact: "HIGH",
			Version: v(1, 0, 0), Stars: 2},
		StarredAt: starred[0].StarredAt,
	}); !reflect.DeepEqual(starred[0], want) {
		t.Errorf("got %+v, want %+v", starred[0], want)
	}
	if got := c.starredRules(t, vettedBoth, c.account(t, 3)); len(got) != 0 {
		t.Errorf("an account without stars lists %q", got)
	}
}

// Deleting an account, as the web function does, removes its stars, so they stop counting.
func TestDeletingAnAccountRemovesItsStars(t *testing.T) {
	c := newListingCatalog(t)
	first, second := c.account(t, 1), c.account(t, 2)
	c.star(t, vettedBoth, first, "acme", "backend", acmeErrors)
	c.star(t, vettedBoth, second, "acme", "backend", acmeErrors)

	postgrestest.Exec(t, postgrestest.AsWebRole(t, c.connString), `DELETE FROM accounts WHERE id = $1`, first)

	if got := c.ruleStars(t, vettedBoth, "acme", "backend"); got[acmeErrors] != 1 {
		t.Fatalf("got %v, want only the remaining account's star", got)
	}
	var left int
	postgrestest.QueryRow(t, c.connString, fmt.Sprintf(`SELECT count(*) FROM rule_stars WHERE account_id = %d`, first), &left)
	if left != 0 {
		t.Fatalf("the deleted account keeps %d stars", left)
	}
}
