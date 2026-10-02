package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
	"github.com/fabricahq/rulemart/internal/platform/postgrestest"
)

// star stars owner/name for the account as the web function does, and fails t if it can't.
func (c listingCatalog) star(t *testing.T, accountID int64, owner, name string) {
	t.Helper()
	if _, err := c.web.Star(context.Background(), vettedBoth, accountID, owner, name); err != nil {
		t.Fatalf("star %s/%s for account %d: %v", owner, name, accountID, err)
	}
}

// stars returns how many stars pages count for each vetted library, by owner/name, and on acme/backend's own page.
func (c listingCatalog) stars(t *testing.T) (cards map[string]int, page int) {
	t.Helper()
	libraries, err := c.web.Libraries(context.Background(), vettedBoth)
	if err != nil {
		t.Fatal(err)
	}
	cards = map[string]int{}
	for _, lib := range libraries {
		cards[lib.Owner+"/"+lib.Name] = lib.Stars
	}
	acmePage, err := c.web.LibraryPage(context.Background(), vettedBoth, "acme", "backend")
	if err != nil {
		t.Fatal(err)
	}
	return cards, acmePage.Library.Stars
}

// An account stars a vetted library once, by any spelling of its name, and pages count each account's star.
func TestAnAccountStarsAVettedLibraryOnce(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	first, second := c.account(t, 1), c.account(t, 2)

	ref, err := c.web.Star(ctx, vettedBoth, first, "ACME", "Backend")
	if err != nil || ref.FullName() != "acme/backend" {
		t.Fatalf("got %+v, %v; want acme/backend as GitHub spells it", ref, err)
	}
	c.star(t, first, "acme", "backend")
	if cards, page := c.stars(t); cards["acme/backend"] != 1 || cards["Beta/rules"] != 0 || page != 1 {
		t.Fatalf("after one account starred acme twice: cards %v, page %d; want one star", cards, page)
	}
	c.star(t, second, "acme", "backend")
	if cards, page := c.stars(t); cards["acme/backend"] != 2 || page != 2 {
		t.Fatalf("after two accounts starred acme: cards %v, page %d; want two stars", cards, page)
	}
	for account, want := range map[int64]bool{first: true, second: true, c.account(t, 3): false} {
		if starred, err := c.web.Starred(ctx, account, "Acme", "BACKEND"); err != nil || starred != want {
			t.Errorf("account %d: starred %v, %v; want %v", account, starred, err, want)
		}
	}
}

// Unstarring removes only the account's own star, and unstarring what it hasn't starred changes nothing.
func TestUnstarringRemovesOnlyTheAccountsOwnStar(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	first, second := c.account(t, 1), c.account(t, 2)
	c.star(t, first, "acme", "backend")
	c.star(t, second, "acme", "backend")
	c.star(t, first, "Beta", "rules")

	for range 2 {
		if err := c.web.Unstar(ctx, second, "ACME", "backend"); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.web.Unstar(ctx, second, "beta", "rules"); err != nil {
		t.Fatal(err)
	}
	if err := c.web.Unstar(ctx, second, "nobody", "nothing"); err != nil {
		t.Fatalf("unstarring a library that isn't there: %v", err)
	}
	if cards, page := c.stars(t); cards["acme/backend"] != 1 || cards["Beta/rules"] != 1 || page != 1 {
		t.Fatalf("got cards %v, page %d; want the first account's stars on both", cards, page)
	}
	if starred, err := c.web.Starred(ctx, first, "acme", "backend"); err != nil || !starred {
		t.Errorf("the first account's star: %v, %v", starred, err)
	}
	if starred, err := c.web.Starred(ctx, second, "acme", "backend"); err != nil || starred {
		t.Errorf("the second account's star: %v, %v; want it gone", starred, err)
	}
}

// Only a vetted library can be starred: not a listed one, nor one the catalog doesn't have.
func TestOnlyAVettedLibraryCanBeStarred(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	account := c.account(t, 1)
	c.resolve(t, c.list(t, account, "stranger", "rules"), "23")

	for _, name := range [][2]string{{"stranger", "rules"}, {"nobody", "nothing"}, {"acme", ""}} {
		if _, err := c.web.Star(ctx, vettedBoth, account, name[0], name[1]); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("star %s/%s: got %v, want ErrNotFound", name[0], name[1], err)
		}
	}
	if stars, err := c.web.AccountStars(ctx, vettedBoth, account); err != nil || len(stars) != 0 {
		t.Fatalf("got %+v, %v; want no stars", stars, err)
	}
}

// An account's stars list its starred libraries, most recently starred first, and say which the release no longer
// vets and which of those a listing still names; the account can unstar those too.
func TestAccountStarsListTheAccountsLibrariesNewestFirst(t *testing.T) {
	c := newListingCatalog(t)
	ctx := context.Background()
	account, other := c.account(t, 1), c.account(t, 2)
	everyone := append([]domain.LibraryKey{{Host: domain.GitHub, RepositoryID: "23"}}, vettedBoth...)
	for _, name := range [][2]string{{"acme", "backend"}, {"stranger", "rules"}, {"Beta", "rules"}} {
		if _, err := c.web.Star(ctx, everyone, account, name[0], name[1]); err != nil {
			t.Fatal(err)
		}
	}
	c.star(t, other, "acme", "backend")
	// Star acme last, then Beta, then stranger, whatever order the statements ran in.
	postgrestest.Exec(t, c.connString, `UPDATE stars SET created_at = now() - CASE
		(SELECT l.owner FROM libraries l WHERE l.id = stars.library_id)
		WHEN 'acme' THEN interval '1 minute' WHEN 'Beta' THEN interval '2 minutes' ELSE interval '3 minutes' END
		WHERE account_id = $1`, account)
	c.resolve(t, c.list(t, other, "stranger", "rules"), "23")

	stars, err := c.web.AccountStars(ctx, vettedBoth, account)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		name           string
		stars          int
		vetted, listed bool
	}
	var got []row
	for _, s := range stars {
		if s.StarredAt.IsZero() {
			t.Errorf("%s has no star time", s.Library.Name)
		}
		got = append(got, row{s.Library.Owner + "/" + s.Library.Name, s.Library.Stars, s.Vetted, s.Listed})
	}
	want := []row{{"acme/backend", 2, true, false}, {"Beta/rules", 1, true, false}, {"stranger/rules", 1, false, true}}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("star %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
	if stars[0].Library.Description != "Rules by acme." || stars[0].Library.Rules == 0 {
		t.Errorf("acme's card: %+v", stars[0].Library)
	}

	if err := c.web.Unstar(ctx, account, "stranger", "rules"); err != nil {
		t.Fatal(err)
	}
	if stars, err := c.web.AccountStars(ctx, vettedBoth, account); err != nil || len(stars) != 2 {
		t.Fatalf("after unstarring the unvetted library: %+v, %v", stars, err)
	}
	if others, err := c.web.AccountStars(ctx, vettedBoth, other); err != nil || len(others) != 1 ||
		others[0].Library != (views.LibraryCard{Owner: "acme", Name: "backend", Description: "Rules by acme.", OwnerAvatarURL: acme.Repository.OwnerAvatarURL, Rules: others[0].Library.Rules, Stars: 2}) {
		t.Fatalf("the other account's stars: %+v, %v", others, err)
	}
}

// Deleting an account, as the web function does, removes its stars, so they stop counting.
func TestDeletingAnAccountRemovesItsStars(t *testing.T) {
	c := newListingCatalog(t)
	first, second := c.account(t, 1), c.account(t, 2)
	c.star(t, first, "acme", "backend")
	c.star(t, second, "acme", "backend")

	postgrestest.Exec(t, postgrestest.AsWebRole(t, c.connString), `DELETE FROM accounts WHERE id = $1`, first)

	if cards, page := c.stars(t); cards["acme/backend"] != 1 || page != 1 {
		t.Fatalf("got cards %v, page %d; want only the remaining account's star", cards, page)
	}
	var left int
	postgrestest.QueryRow(t, c.connString, fmt.Sprintf(`SELECT count(*) FROM stars WHERE account_id = %d`, first), &left)
	if left != 0 {
		t.Fatalf("the deleted account keeps %d stars", left)
	}
}
