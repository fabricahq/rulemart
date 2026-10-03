package postgres_test

import (
	"context"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// The libraries a visitor and their organizations publish are the vetted and listed ones their logins own, in any
// case, each with its rules and the sum of their stars; a library only stored, and another owner's, are left out.
func TestDashboardListsTheLibrariesTheOwnersPublishWithTheirStars(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	first, second := c.account(t, 1), c.account(t, 2)
	c.star(t, vettedBoth, first, "acme", "backend", acmeErrors)
	c.star(t, vettedBoth, second, "acme", "backend", acmeErrors)
	c.star(t, vettedBoth, first, "acme", "backend", acmeRetry)

	got, err := c.web.Dashboard(ctx, vettedBoth, []string{"ACME", "stranger", "nobody"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Owned) != 1 || got.Owned[0].Library.FullName() != "acme/backend" || !got.Owned[0].Vetted ||
		got.Owned[0].Rules != 4 || got.Owned[0].Stars != 3 || got.Owned[0].AddedAt.IsZero() {
		t.Fatalf("owned %+v, want acme/backend with 4 rules and 3 stars, and not the unlisted stranger/rules", got.Owned)
	}

	c.resolve(t, c.list(t, first, "stranger", "rules"), "23")
	got, err = c.web.Dashboard(ctx, vettedBoth, []string{"stranger"}, nil)
	if err != nil || len(got.Owned) != 1 || got.Owned[0].Vetted || got.Owned[0].Stars != 0 {
		t.Errorf("once listed, owned %+v, %v; want stranger/rules, unvetted", got.Owned, err)
	}
}

// The libraries projects import come with each rule as it stands, current with its version or retired, so update counts
// compare them; one Rulemart neither vets nor lists is missing.
func TestDashboardReadsTheRulesOfTheImportedLibraries(t *testing.T) {
	c := newListingCatalog(t)
	got, err := c.web.Dashboard(context.Background(), vettedBoth, nil, []string{"Acme/Backend", "stranger/rules", "nobody/rules"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Imported) != 1 || got.Imported[0].Library.FullName() != "acme/backend" {
		t.Fatalf("imported %+v, want acme/backend alone", got.Imported)
	}
	rules := got.Imported[0].Rules
	if rules.Current[acmeErrors] != (coderules.RuleVersion{Major: 1}) || !rules.Retired["practices/testing/retry-forever"] || len(rules.Current) != 4 {
		t.Errorf("acme/backend's rules are %+v", rules)
	}
	if n := rules.Updates([]domain.PinnedVersion{{Path: "practices/testing/retry-forever", Version: coderules.FirstRuleVersion}}); n != 1 {
		t.Errorf("a pinned retired rule counts %d updates, want 1", n)
	}
}

// A star that counts toward no current rule of a vetted library, such as one on a rule retired without a replacement,
// or in a library that lost its vetting, is listed apart, and the account can remove it.
func TestUncountedStarsAreListedAndRemovable(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	account := c.account(t, 1)
	c.starAsOwner(t, account, "21", "practices/testing/retry-forever")
	c.star(t, vettedBoth, account, "acme", "backend", acmeErrors)
	c.star(t, vettedBoth, account, "Beta", "rules", "techs/go/name-packages-plainly")
	vettedAcme := vettedBoth[:1]

	got, err := c.web.UncountedStars(ctx, vettedAcme, account)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("uncounted %+v, want Beta's rule and acme's retired rule", got)
	}
	byPath := map[string]bool{}
	for _, s := range got {
		byPath[s.Library.FullName()+"/"+s.Path] = true
		if s.Path == "practices/testing/retry-forever" && (!s.Retired || !s.Vetted || s.ReplacedBy != "") {
			t.Errorf("the retired rule's star is %+v", s)
		}
		if s.Path == "techs/go/name-packages-plainly" && (s.Vetted || s.Retired || s.Title != "Name packages plainly") {
			t.Errorf("the unvetted library's star is %+v", s)
		}
	}
	if !byPath["acme/backend/practices/testing/retry-forever"] || !byPath["Beta/rules/techs/go/name-packages-plainly"] {
		t.Errorf("uncounted %v", byPath)
	}

	if err := c.web.RemoveStar(ctx, account, "acme", "BACKEND", "practices/testing/retry-forever"); err != nil {
		t.Fatal(err)
	}
	if err := c.web.RemoveStar(ctx, account, "acme", "backend", "practices/testing/retry-forever"); err == nil {
		t.Error("removed a star twice")
	}
	if got, _ := c.web.UncountedStars(ctx, vettedAcme, account); len(got) != 1 {
		t.Errorf("after removing one, uncounted %+v", got)
	}
	if counted := c.starredRules(t, vettedAcme, account); len(counted) != 1 {
		t.Errorf("the counted stars are %v, want return-errors alone", counted)
	}
}
